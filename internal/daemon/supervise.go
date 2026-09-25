package daemon

import (
	"errors"

	"google.golang.org/protobuf/proto"

	"github.com/borismilner/rig/internal/supervise"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

// Section 18's supervision, served. internal/supervise is the whole mechanism
// and stays stdlib-only because the tray links it, so this file is the
// translation to the wire and the two decisions the mechanism cannot make:
// WHICH connection is the supervised child, and WHO may report its health.

// verbRestart is the one supervision verb named in three files.
const verbRestart = "restart"

// serveSupervise dispatches rig.up, rig.stop, rig.restart, rig.health and a
// program's own rig.health.report.
func (d *Daemon) serveSupervise(c *conn, f *rigv1.Frame, command string) {
	if d.super == nil {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:         rigv1.Code_CODE_UNAVAILABLE,
			Message:      "rig." + command + ": this daemon supervises nothing",
			Precondition: "rigd was started with a supervisor",
			Actual:       "this daemon was built without one, which only a test does",
			Fix:          "run the rigd binary rather than a daemon built in-process",
		})
		return
	}
	switch command {
	case "health.report":
		d.serveHealthReport(c, f)
	case "up":
		var req verbsv1.UpRequest
		if !unmarshalOr(c, f, command, &req) {
			return
		}
		st, err := d.super.Up(req.GetPrograms()...)
		if err != nil {
			c.failErr(f.GetStreamId(), superviseCode(err), err)
			return
		}
		c.reply(f.GetStreamId(), &verbsv1.UpResponse{Programs: healthToWire(st)})
	case "stop", verbRestart:
		var program string
		act := d.super.Stop
		if command == verbRestart {
			var req verbsv1.RestartRequest
			if !unmarshalOr(c, f, command, &req) {
				return
			}
			program, act = req.GetProgram(), d.super.Restart
		} else {
			var req verbsv1.StopRequest
			if !unmarshalOr(c, f, command, &req) {
				return
			}
			program = req.GetProgram()
		}
		if err := act(program); err != nil {
			c.failErr(f.GetStreamId(), superviseCode(err), err)
			return
		}
		st, err := d.super.Health(program)
		if err != nil {
			c.failErr(f.GetStreamId(), superviseCode(err), err)
			return
		}
		one := healthToWire(st)[0]
		if command == verbRestart {
			c.reply(f.GetStreamId(), &verbsv1.RestartResponse{Program: one})
			return
		}
		c.reply(f.GetStreamId(), &verbsv1.StopResponse{Program: one})
	case "health":
		var req verbsv1.HealthRequest
		if !unmarshalOr(c, f, command, &req) {
			return
		}
		st, err := d.super.Health(req.GetPrograms()...)
		if err != nil {
			c.failErr(f.GetStreamId(), superviseCode(err), err)
			return
		}
		supervise.SortStatus(st)
		c.reply(f.GetStreamId(), &verbsv1.HealthResponse{Programs: healthToWire(st)})
	}
}

// serveHealthReport is a supervised program saying how it is doing.
//
// ⛔ THE PROGRAM IS THE CONNECTION's, AND ONLY THE CHILD RIG LAUNCHED MAY
// SPEAK FOR IT. The request carries no id. A connection that registered under
// a declared name but is not the process the supervisor started (a second
// copy run by hand) is refused: its marker is not evidence about the child,
// and accepting it would let a healthy impostor keep a stalled child HEALTHY.
func (d *Daemon) serveHealthReport(c *conn, f *rigv1.Frame) {
	var req rigv1.HealthReportRequest
	if !unmarshalOr(c, f, "health.report", &req) {
		return
	}
	id, ok := d.supervisedChild(c)
	if !ok {
		c.failStatus(f.GetStreamId(), &rigv1.Status{
			Code:         rigv1.Code_CODE_DENIED,
			Message:      "rig.health.report: this connection is not a program rig supervises",
			Precondition: "the caller said hello as a declared program, from the process rig started for it",
			Actual:       "an unregistered connection, an undeclared program, or a process rig did not start",
			Fix:          "declare the program in programs.json and start it with rig up",
		})
		return
	}
	if err := d.super.Observe(id, supervise.Report{
		Marker: req.GetMarker(), Waiting: req.GetWaiting(), Parked: req.GetParked(),
	}); err != nil {
		c.failErr(f.GetStreamId(), superviseCode(err), err)
		return
	}
	c.reply(f.GetStreamId(), &rigv1.HealthReportResponse{})
}

// supervisedChild names the declared program this connection is, when it is
// the very process the supervisor launched for it. The pid comes off the
// socket's peer credentials, never off the request.
func (d *Daemon) supervisedChild(c *conn) (string, bool) {
	if d.super == nil || !c.scoped.Load() {
		return "", false
	}
	id := c.name()
	st, err := d.super.Health(id)
	if err != nil || len(st) != 1 || st[0].PID == 0 {
		return "", false
	}
	return id, st[0].PID == c.principal().PID
}

// supervisedHello completes STARTING's handshake for a supervised child. A
// program nobody declared, or one rig did not start, is none of the
// supervisor's business and registers exactly as before.
func (d *Daemon) supervisedHello(c *conn) {
	id, ok := d.supervisedChild(c)
	if !ok {
		return
	}
	if err := d.super.Registered(id); err != nil {
		// A second hello from a child already past STARTING is not a fault
		// worth refusing the connection over; the table said no and the
		// history keeps why.
		d.log.Info("supervised program registered outside STARTING", "program", id, "err", err)
	}
}

func unmarshalOr(c *conn, f *rigv1.Frame, command string, m proto.Message) bool {
	if err := proto.Unmarshal(f.GetPayload(), m); err != nil {
		c.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, command+": "+err.Error())
		return false
	}
	return true
}

func superviseCode(err error) rigv1.Code {
	var fault *supervise.FaultError
	switch {
	case errors.Is(err, supervise.ErrNoSuchProgram):
		return rigv1.Code_CODE_NOT_FOUND
	case errors.As(err, &fault):
		return rigv1.Code_CODE_CONFLICT
	default:
		return rigv1.Code_CODE_INTERNAL
	}
}

var programStateWire = map[supervise.State]verbsv1.ProgramState{
	supervise.StateStarting:    verbsv1.ProgramState_PROGRAM_STATE_STARTING,
	supervise.StateHealthy:     verbsv1.ProgramState_PROGRAM_STATE_HEALTHY,
	supervise.StateDegraded:    verbsv1.ProgramState_PROGRAM_STATE_DEGRADED,
	supervise.StateRestarting:  verbsv1.ProgramState_PROGRAM_STATE_RESTARTING,
	supervise.StateQuarantined: verbsv1.ProgramState_PROGRAM_STATE_QUARANTINED,
}

func healthToWire(in []supervise.Status) []*verbsv1.ProgramHealth {
	out := make([]*verbsv1.ProgramHealth, 0, len(in))
	for _, s := range in {
		h := &verbsv1.ProgramHealth{
			Id: s.ID, State: programStateWire[s.State], Pid: int32(s.PID), //nolint:gosec // a pid fits
			Failures: clampU32(s.Failures), Restarts: clampU32(s.Restarts),
			Marker: s.Marker, Waiting: s.Waiting, Parked: s.Parked,
		}
		if !s.Since.IsZero() {
			h.SinceUnixNano = s.Since.UnixNano()
		}
		if s.LastExit != nil {
			h.LastExit = s.LastExit.String()
		}
		if !s.NextAttempt.IsZero() {
			h.NextAttemptUnixNano = s.NextAttempt.UnixNano()
		}
		for _, e := range s.History {
			h.History = append(h.History, &verbsv1.ProgramEvent{
				AtUnixNano: e.At.UnixNano(), From: programStateWire[e.From],
				To: programStateWire[e.To], Trigger: e.Trigger.String(),
				Actor: e.Actor.String(), Note: e.Note,
			})
		}
		out = append(out, h)
	}
	return out
}

func clampU32(n int) uint32 {
	if n < 0 {
		return 0
	}
	return uint32(min(n, int(^uint32(0))))
}
