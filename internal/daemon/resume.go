package daemon

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// Section 52's system.resumed: the one fact no program can learn for itself,
// that the machine slept and every timer measured on the monotonic clock is
// now hours behind (plan/05, "system.resumed fixes a defect that is live in
// every program today").
//
// HOW IT IS NOTICED, AT NO COST WHILE NOTHING HAPPENS. A timerfd on
// CLOCK_REALTIME armed with TFD_TIMER_CANCEL_ON_SET is cancelled by the
// kernel whenever the wall clock is set, and timekeeping_resume cancels it on
// every resume. So the daemon blocks on one read and wakes only then; there
// is no polling loop. A cancel is either a resume or a clock change, and the
// two are told apart by how far CLOCK_BOOTTIME ran ahead of CLOCK_MONOTONIC,
// which is exactly the time spent asleep.

// resumeFloor is the shortest sleep reported. Below it the gap is noise from
// reading two clocks one after the other.
const resumeFloor = 2 * time.Second

// clocks is one reading of the two clocks whose difference is sleep.
type clocks struct{ boot, mono time.Duration }

func readClocks() (clocks, error) {
	var b, m unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &b); err != nil {
		return clocks{}, err
	}
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &m); err != nil {
		return clocks{}, err
	}
	return clocks{boot: time.Duration(b.Nano()), mono: time.Duration(m.Nano())}, nil
}

// slept is how long the machine was suspended between two readings.
func slept(from, to clocks) time.Duration {
	return (to.boot - from.boot) - (to.mono - from.mono)
}

// clockCancels returns a function that blocks until the wall clock is set or
// the machine resumes, and a closer. The function answers nil for either,
// and an error once closed or broken.
func clockCancels() (wait, closer func() error, err error) {
	fd, err := unix.TimerfdCreate(unix.CLOCK_REALTIME, unix.TFD_NONBLOCK|unix.TFD_CLOEXEC)
	if err != nil {
		return nil, nil, fmt.Errorf("timerfd: %w", err)
	}
	f := os.NewFile(uintptr(fd), "rigd-resume-timerfd")
	// Far enough ahead never to expire; it exists to be cancelled.
	never := unix.ItimerSpec{Value: unix.Timespec{Sec: math.MaxInt32}}
	arm := func() error {
		return unix.TimerfdSettime(fd, unix.TFD_TIMER_ABSTIME|unix.TFD_TIMER_CANCEL_ON_SET, &never, nil)
	}
	if err := arm(); err != nil {
		_ = f.Close()
		return nil, nil, fmt.Errorf("arming the timerfd: %w", err)
	}
	buf := make([]byte, 8)
	wait = func() error {
		_, err := f.Read(buf)
		if !errors.Is(err, unix.ECANCELED) {
			if err == nil {
				err = errors.New("the timerfd expired, which it is armed never to do")
			}
			return err
		}
		return arm()
	}
	return wait, f.Close, nil
}

// watchResume publishes system.resumed after every suspend until ctx ends.
// wait blocks until the next clock cancel; read reads the clocks.
func (d *Daemon) watchResume(ctx context.Context, wait func() error, read func() (clocks, error)) {
	last, err := read()
	if err != nil {
		d.log.Warn("system.resumed is off: the clocks cannot be read", "err", err)
		return
	}
	for {
		if err := wait(); err != nil {
			if ctx.Err() == nil {
				d.log.Warn("system.resumed is off: the resume signal failed", "err", err)
			}
			return
		}
		now, err := read()
		if err != nil {
			d.log.Warn("system.resumed is off: the clocks cannot be read", "err", err)
			return
		}
		gone := slept(last, now)
		last = now
		if gone < resumeFloor {
			continue // the wall clock was set; nobody slept
		}
		d.log.Info("the machine resumed", "slept", gone.Round(time.Second))
		d.events.publishRig("system.resumed", &registryv1.SystemResumed{SleptMs: uint64(gone.Milliseconds())}) //nolint:gosec // at least resumeFloor
	}
}

// startResumeWatch runs watchResume on the kernel's signal for as long as ctx
// lives. A machine without it loses only this event, and says so.
func (d *Daemon) startResumeWatch(ctx context.Context) {
	wait, closer, err := clockCancels()
	if err != nil {
		d.log.Warn("system.resumed is off", "err", err)
		return
	}
	go func() {
		<-ctx.Done()
		_ = closer()
	}()
	go d.watchResume(ctx, wait, readClocks)
}
