package daemon

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/robfig/cron/v3"
	"golang.org/x/sys/unix"

	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// Section 52's timers. Boris ruled that rig owns the clock (Q1, decision
// 0255): only rig knows when the machine slept, so only rig can fire a timer
// correctly across a suspend.
//
// ONE TIMERFD ON CLOCK_REALTIME, ABSOLUTE, ARMED FOR THE EARLIEST FIRE. A Go
// timer runs on the monotonic clock, which stops while the machine sleeps, so
// a fire due at 07:00 and armed at 23:00 would come hours late. The kernel
// fires an absolute realtime timer at resume if its time passed, and
// TFD_TIMER_CANCEL_ON_SET wakes it when the wall clock is set, so the loop
// recomputes then too. Nothing wakes while nothing is due.
//
// IN MEMORY, AS THE BUS IS (E7). A program arms its timers each time it
// starts, and arming the same name and schedule again changes nothing. A
// timer outlives its program's connection, so a fire while the program
// restarts waits in the ring for it.

const (
	maxTimersPerProgram = 64
	maxTimerName        = 64
	maxTimerSchedule    = 128
	// maxMissedCount bounds the count of missed fires, so a schedule every
	// second across a month's sleep does not count for minutes.
	maxMissedCount = 100_000
)

// minTimerEvery bounds "every" at cron's own grain. Every fire takes a slot
// in the one ring all waiters share, so a program ticking each second would
// push everyone else's events out of it within the hour. A var so a test
// need not wait a minute.
var minTimerEvery = time.Minute

type armedTimer struct {
	name, schedule string
	sched          cron.Schedule
	next           time.Time
}

// timerDesk holds every program's timers. Its zero value is ready to use;
// fd is set while the loop runs.
type timerDesk struct {
	mu    sync.Mutex
	armed map[string]map[string]*armedTimer // owner, then name
	fd    int                               // the timerfd plus one; 0 while no loop runs
	now   func() time.Time                  // nil is time.Now
}

func (t *timerDesk) clock() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

// parseSchedule reads Q2's two syntaxes: "every 15m" and "at 09:00", or a
// cron line in standard five fields or a descriptor such as @daily.
func parseSchedule(s string) (cron.Schedule, error) {
	if len(s) > maxTimerSchedule {
		return nil, fmt.Errorf("is over %d bytes", maxTimerSchedule)
	}
	if rest, ok := strings.CutPrefix(s, "every "); ok {
		d, err := time.ParseDuration(strings.TrimSpace(rest))
		if err != nil {
			return nil, fmt.Errorf("every takes a duration such as 15m or 2h: %w", err)
		}
		if d < minTimerEvery {
			return nil, fmt.Errorf("every %s is under the floor of %s", d, minTimerEvery)
		}
		return cron.Every(d), nil
	}
	if rest, ok := strings.CutPrefix(s, "at "); ok {
		at, err := time.Parse("15:04", strings.TrimSpace(rest))
		if err != nil {
			return nil, errors.New("at takes a 24-hour local time such as 09:00")
		}
		return cron.ParseStandard(fmt.Sprintf("%d %d * * *", at.Minute(), at.Hour()))
	}
	return cron.ParseStandard(s)
}

func badTimerName(name string) string {
	if name == "" || len(name) > maxTimerName {
		return fmt.Sprintf("a timer's name is 1 to %d bytes", maxTimerName)
	}
	for _, r := range name {
		if !unicode.IsPrint(r) || unicode.IsSpace(r) {
			return "a timer's name is printable and has no spaces"
		}
	}
	return ""
}

func (t *timerDesk) arm(owner, name, schedule string, sched cron.Schedule) (*registryv1.Timer, bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.armed == nil {
		t.armed = map[string]map[string]*armedTimer{}
	}
	mine := t.armed[owner]
	if mine == nil {
		mine = map[string]*armedTimer{}
		t.armed[owner] = mine
	}
	old, had := mine[name]
	if had && old.schedule == schedule {
		return timerOut(old), false, nil
	}
	if !had && len(mine) >= maxTimersPerProgram {
		return nil, false, fmt.Errorf("%s already has %d timers, the most one program may arm", owner, maxTimersPerProgram)
	}
	next := sched.Next(t.clock())
	if next.IsZero() {
		return nil, false, errors.New("that schedule never comes due")
	}
	at := &armedTimer{name: name, schedule: schedule, sched: sched, next: next}
	mine[name] = at
	t.rearm()
	return timerOut(at), had, nil
}

func (t *timerDesk) disarm(owner, name string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.armed[owner][name]; !ok {
		return false
	}
	delete(t.armed[owner], name)
	t.rearm()
	return true
}

func (t *timerDesk) list(owner string) []*registryv1.Timer {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []*registryv1.Timer
	for _, at := range t.armed[owner] {
		out = append(out, timerOut(at))
	}
	slices.SortFunc(out, func(a, b *registryv1.Timer) int { return strings.Compare(a.GetName(), b.GetName()) })
	return out
}

func timerOut(at *armedTimer) *registryv1.Timer {
	return &registryv1.Timer{Name: at.name, Schedule: at.schedule, NextUnixNano: at.next.UnixNano()}
}

// timerFire is one fire owed to one program.
type timerFire struct {
	owner string
	fired *registryv1.TimerFired
}

// due collects every fire owed at now and moves each timer to its next
// time. A timer that fell due more than once while nobody looked fires once,
// counting the rest as missed (Q3). Called with mu held.
func (t *timerDesk) due(now time.Time) []timerFire {
	var out []timerFire
	for owner, mine := range t.armed {
		for _, at := range mine {
			if at.next.After(now) {
				continue
			}
			var missed uint32
			for n := at.sched.Next(at.next); !n.IsZero() && !n.After(now) && missed < maxMissedCount; n = at.sched.Next(n) {
				missed++
			}
			out = append(out, timerFire{owner: owner, fired: &registryv1.TimerFired{
				Name: at.name, DueUnixNano: at.next.UnixNano(), Missed: missed,
			}})
			at.next = at.sched.Next(now)
		}
	}
	slices.SortFunc(out, func(a, b timerFire) int {
		return int(min(max(a.fired.GetDueUnixNano()-b.fired.GetDueUnixNano(), -1), 1))
	})
	return out
}

// rearm points the timerfd at the earliest fire, or at never. Called with mu
// held.
func (t *timerDesk) rearm() {
	if t.fd == 0 {
		return
	}
	var earliest time.Time
	for _, mine := range t.armed {
		for _, at := range mine {
			if !at.next.IsZero() && (earliest.IsZero() || at.next.Before(earliest)) {
				earliest = at.next
			}
		}
	}
	when := unix.Timespec{Sec: math.MaxInt32}
	if !earliest.IsZero() {
		// A time already past would read as zero, which DISARMS a timerfd:
		// one nanosecond after the epoch fires at once instead.
		when = unix.NsecToTimespec(max(earliest.UnixNano(), 1))
	}
	_ = unix.TimerfdSettime(t.fd-1, unix.TFD_TIMER_ABSTIME|unix.TFD_TIMER_CANCEL_ON_SET,
		&unix.ItimerSpec{Value: when}, nil)
}

// startTimers runs the firing loop for as long as ctx lives.
func (d *Daemon) startTimers(ctx context.Context) {
	fd, err := unix.TimerfdCreate(unix.CLOCK_REALTIME, unix.TFD_NONBLOCK|unix.TFD_CLOEXEC)
	if err != nil {
		d.log.Warn("timers are off: no timerfd", "err", err)
		return
	}
	f := os.NewFile(uintptr(fd), "rigd-timers-timerfd")
	d.timers.mu.Lock()
	d.timers.fd = fd + 1
	d.timers.rearm()
	d.timers.mu.Unlock()
	go func() {
		<-ctx.Done()
		d.timers.mu.Lock()
		d.timers.fd = 0
		d.timers.mu.Unlock()
		_ = f.Close()
	}()
	go func() {
		buf := make([]byte, 8)
		for {
			if _, err := f.Read(buf); err != nil && !errors.Is(err, unix.ECANCELED) {
				if ctx.Err() == nil {
					d.log.Warn("timers stopped: the timerfd failed", "err", err)
				}
				return
			}
			d.timers.mu.Lock()
			for _, fire := range d.timers.due(d.timers.clock()) {
				d.events.publishRigTo("timer.fired", fire.fired, fire.owner)
			}
			d.timers.rearm()
			d.timers.mu.Unlock()
		}
	}()
}

// serveTimer answers timer.arm, timer.disarm and timer.list. A timer belongs
// to the registered program that armed it, read off the connection.
func (d *Daemon) serveTimer(c *conn, f *rigv1.Frame, command string) {
	id := f.GetStreamId()
	owner := c.name()
	if owner == "" {
		c.failStatus(id, &rigv1.Status{
			Code:         rigv1.Code_CODE_DENIED,
			Message:      "rig." + command + ": only a registered program has timers",
			Precondition: "the caller registered with hello; its timers fire to it as timer.fired",
			Actual:       "this connection is not a registered program",
			Fix:          "arm the timer from the program that acts on it",
		})
		return
	}
	switch command {
	case "timer.arm":
		var req registryv1.TimerArmRequest
		if !unmarshalOr(c, f, command, &req) {
			return
		}
		if bad := badTimerName(req.GetName()); bad != "" {
			c.fail(id, rigv1.Code_CODE_INVALID, "rig.timer.arm: "+bad)
			return
		}
		sched, err := parseSchedule(req.GetSchedule())
		if err != nil {
			c.failStatus(id, &rigv1.Status{
				Code:         rigv1.Code_CODE_INVALID,
				Message:      fmt.Sprintf("rig.timer.arm: schedule %q %v", req.GetSchedule(), err),
				Precondition: `"every 15m", "at 09:00", a five-field cron line, or @hourly, @daily, @weekly, @monthly`,
				Fix:          "write the schedule in one of those forms; times are local",
			})
			return
		}
		t, replaced, err := d.timers.arm(owner, req.GetName(), req.GetSchedule(), sched)
		if err != nil {
			c.fail(id, rigv1.Code_CODE_INVALID, "rig.timer.arm: "+err.Error())
			return
		}
		c.reply(id, &registryv1.TimerArmResponse{Timer: t, Replaced: replaced})
	case "timer.disarm":
		var req registryv1.TimerDisarmRequest
		if !unmarshalOr(c, f, command, &req) {
			return
		}
		c.reply(id, &registryv1.TimerDisarmResponse{Disarmed: d.timers.disarm(owner, req.GetName())})
	case "timer.list":
		c.reply(id, &registryv1.TimerListResponse{Timers: d.timers.list(owner)})
	}
}
