package daemon

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/borismilner/rig/internal/coord"
)

// Whether Boris is at the desk, so a program can pick its moment: a question
// put to an empty chair waits, one put while he works lands. rig keeps it in
// the shared key rig.desk, {"state":"active|idle|locked","since":RFC 3339},
// and every change posts shared.rig.desk, so a program reads it with
// shared.get and waits on it with events.wait. No key means rig cannot tell.
//
// HOW IT IS NOTICED, AT NO COST WHILE NOTHING HAPPENS. GNOME says both
// halves on the session bus: the screen lock's ActiveChanged, and Mutter's
// idle monitor, which fires an idle watch once the input has been quiet for
// deskIdleAfterMs and a user-active watch at the next touch. rig sets watches
// and waits for signals; it never polls.

const (
	deskKey = "rig.desk"
	// deskIdleAfterMs is how long the input stays quiet before rig.desk
	// says idle: five minutes, in the milliseconds Mutter counts.
	deskIdleAfterMs uint64 = 5 * 60 * 1000

	deskActive = "active"
	deskIdle   = "idle"
	deskLocked = "locked"

	idleMonitorDest  = "org.gnome.Mutter.IdleMonitor"
	idleMonitorPath  = "/org/gnome/Mutter/IdleMonitor/Core"
	screenSaverDest  = "org.gnome.ScreenSaver"
	screenSaverPath  = "/org/gnome/ScreenSaver"
	idleWatchFired   = idleMonitorDest + ".WatchFired"
	screenSaverState = screenSaverDest + ".ActiveChanged"
)

// rigOwnKey says a shared key is rig's own, which no seat may write.
func rigOwnKey(key string) bool { return strings.HasPrefix(key, "rig.") }

// deskState is rig.desk's value.
type deskState struct {
	State string `json:"state"`
	Since string `json:"since"`
}

// setDesk writes rig.desk when the state changed, and posts it.
func (d *Daemon) setDesk(state string, at time.Time) {
	pre, found, err := d.leases.SharedGet(deskKey)
	if err != nil {
		d.log.Warn("rig.desk not read", "err", err)
		return
	}
	var was deskState
	if found && json.Unmarshal(pre.Value, &was) == nil && was.State == state {
		return
	}
	value, _ := json.Marshal(deskState{State: state, Since: at.UTC().Format(time.RFC3339)})
	var expected uint64
	if found {
		expected = pre.Version
	}
	v, applied, err := d.leases.SharedSet(deskKey, value, expected, "", coord.NoWitness(), "rig")
	if err != nil || !applied {
		d.log.Warn("rig.desk not written", "state", state, "err", err)
		return
	}
	d.publishJSON("shared."+deskKey, sharedChange{Key: deskKey, Change: "set", Version: v.Version, By: "rig"})
}

// startDeskWatch follows the screen lock and the idle monitor for as long as
// ctx lives. A desktop without them leaves rig.desk unset, and says so.
func (d *Daemon) startDeskWatch(ctx context.Context) {
	bus, err := dbus.ConnectSessionBus()
	if err != nil {
		d.log.Warn("rig.desk is off: no session bus", "err", err)
		return
	}
	idle := bus.Object(idleMonitorDest, idleMonitorPath)
	saver := bus.Object(screenSaverDest, screenSaverPath)
	var locked bool
	var quiet uint64
	if err := saver.Call(screenSaverDest+".GetActive", 0).Store(&locked); err != nil {
		d.log.Warn("rig.desk is off: no screen lock to read", "err", err)
		_ = bus.Close()
		return
	}
	if err := idle.Call(idleMonitorDest+".GetIdletime", 0).Store(&quiet); err != nil {
		d.log.Warn("rig.desk is off: no idle monitor to read", "err", err)
		_ = bus.Close()
		return
	}
	for _, m := range [][]dbus.MatchOption{
		{dbus.WithMatchObjectPath(idleMonitorPath), dbus.WithMatchInterface(idleMonitorDest), dbus.WithMatchMember("WatchFired")},
		{dbus.WithMatchObjectPath(screenSaverPath), dbus.WithMatchInterface(screenSaverDest), dbus.WithMatchMember("ActiveChanged")},
	} {
		if err := bus.AddMatchSignal(m...); err != nil {
			d.log.Warn("rig.desk is off: the bus refused a match", "err", err)
			_ = bus.Close()
			return
		}
	}
	signals := make(chan *dbus.Signal, 16)
	bus.Signal(signals)

	var idleWatch, activeWatch uint32
	if err := idle.Call(idleMonitorDest+".AddIdleWatch", 0, deskIdleAfterMs).Store(&idleWatch); err != nil {
		d.log.Warn("rig.desk is off: no idle watch", "err", err)
		_ = bus.Close()
		return
	}
	// awaitTouch arms the one-shot watch that fires at the next input.
	awaitTouch := func() {
		if err := idle.Call(idleMonitorDest+".AddUserActiveWatch", 0).Store(&activeWatch); err != nil {
			d.log.Warn("rig.desk: no user-active watch", "err", err)
		}
	}
	away := quiet >= deskIdleAfterMs
	if away {
		awaitTouch()
	}
	state := func() string {
		switch {
		case locked:
			return deskLocked
		case away:
			return deskIdle
		}
		return deskActive
	}
	d.setDesk(state(), time.Now())

	go func() {
		defer func() { _ = bus.Close() }()
		for {
			select {
			case <-ctx.Done():
				return
			case s, ok := <-signals:
				if !ok {
					return
				}
				switch s.Name {
				case screenSaverState:
					if len(s.Body) == 1 {
						locked, _ = s.Body[0].(bool)
					}
				case idleWatchFired:
					if len(s.Body) < 1 {
						continue
					}
					id, _ := s.Body[0].(uint32)
					switch id {
					case idleWatch:
						away = true
						awaitTouch()
					case activeWatch:
						away = false
					default:
						continue
					}
				default:
					continue
				}
				d.setDesk(state(), time.Now())
			}
		}
	}()
}
