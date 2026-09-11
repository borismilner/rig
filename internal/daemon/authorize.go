package daemon

import (
	"context"
	"fmt"

	"github.com/boris-milner/rig/internal/kernel"
)

// caller is everything the authorization floor needs about one invocation,
// with no connection anywhere in it.
//
// The floor used to take a *conn and a *rigv1.Frame. That is why an in-process
// surface could not reach it at all: a call that never touched the socket has
// neither, so the daemon could not implement meta.Invoker without either
// bypassing the floor or growing a second one. Section 13a now states the rule
// this type exists to satisfy - "every function on a path to invocation
// carries the principal, and a signature that cannot express the caller is a
// defect in the signature" - and the floor was never bypassed here, it was
// made unreachable by a type.
//
// Holding the method and the args rather than the frame is what makes it
// serve both: the wire path already has a frame to read them from, and the
// in-process path has no frame to build.
type caller struct {
	who kernel.Principal

	// method is <program>.<command>. It reaches the decision log and the
	// gating question, both of which name the call to a person.
	method string

	// args is the payload exactly as it will be delivered. Section 14 requires
	// a gating prompt to carry the arguments as they will be invoked, so
	// losing them here would make the prompt forgeable by omission.
	args []byte

	// requestID is the caller's own id, stable across a retry, which rig
	// dedups against a bounded window (section 4). It is not the floor's
	// input - it travels with the invocation, and a surface that cannot
	// produce one sends it empty, exactly as every rig surface does today.
	requestID string
}

// Question is a gating ask, carrying everything section 14 says a prompt must
// carry: the principal and its client kind, the pid, the command, and the
// arguments as they will be invoked.
//
// A toast that says only "stop shelf?" is answered by whoever is looking at
// the screen, in the belief it came from the terminal in front of them.
type Question struct {
	Principal kernel.Principal
	Decision  kernel.Decision

	// Method is <program>.<command>, and Args is the payload exactly as it
	// will be delivered. Rendering it for a person is the surface's job, not
	// the boundary's; the boundary's job is to not lose it.
	Method string
	Args   []byte
}

// Asker routes a gating question to whoever is actually present - window,
// toast, terminal or phone (PLAN.md sections 12, 13a).
//
// It is an interface here and nil at M1, because no surface exists to answer
// on yet. Nil is not a hole: an unanswered gating question is denied, which
// is the behaviour section 13a specifies for the half where guessing wrong is
// unsafe. Every surface that can ask arrives later and implements this.
type Asker interface {
	Ask(ctx context.Context, q Question) (bool, error)
}

// authorize is the authorization boundary, and every routed call passes
// through it exactly once (PLAN.md section 13a).
//
// It answers with the decision it took and whether the call may proceed. The
// decision is the record: an error return means the invoker could not reason
// about the call at all, which is not the same as refusing it.
//
// rig's own methods reach this too, from serveSelf, and that was a gap until
// they did: serveSelf was the one surface that never met the floor, which is
// what section 13a's "no surface can forget it" forbids.
//
// The methods are rig.hello, rig.ping and rig.programs. (An earlier version
// of this comment said rig.apps; that is the CLI verb, `rig apps list`, and
// no such method exists on the wire.)
//
// rig does NOT do it "the way every other program does", because it cannot:
// SelfID is refused to every registration in two independent places, and
// Register takes a principal because a declaration belongs to the connection
// that made it - rig has no connection to itself. Its declaration is held
// beside the registry by Kernel.DeclareSelf, where only the invoker's read
// reaches it, so rig resolves to declared effects while never appearing as a
// program.
//
// hello is the one method that does not come through here, and that is not an
// exemption: it MINTS the principal, so there is no pair to match before it.
func (d *Daemon) authorize(
	ctx context.Context, from caller, program, command string,
) (kernel.Decision, bool, error) {
	refs := []kernel.Ref{{Kind: kernel.RefCommand, Program: program, Command: command}}

	dec, err := d.kernel.Authorize(from.who, refs)
	if err != nil {
		return dec, false, err
	}

	switch dec.Action {
	case kernel.ActionAllow:
		// Section 13a wants every decision recorded, and section 15 is where
		// that becomes an audit log at M5. Until then an allow is debug and a
		// refusal is a warning, because a line per read-only call at info
		// buries the ones that matter.
		d.logDecision(from, dec, true)
		return dec, true, nil

	case kernel.ActionDeny:
		d.logDecision(from, dec, false)
		return dec, false, nil

	case kernel.ActionConfirm:
		return d.confirm(ctx, from, dec)

	default:
		return dec, false, fmt.Errorf(
			"daemon: house rules produced %s for %s, which is not an action",
			dec.Action, dec.Pair)
	}
}

// confirm routes a gating question, and denies when nobody can answer it.
//
// Section 13a: a gating ask fails CLOSED. An elevation nobody answers is
// denied and recorded, because the alternative is estate-wide action taken by
// default. The disambiguating kind, which parks instead, is not this - it
// arrives with the peers service.
func (d *Daemon) confirm(
	ctx context.Context, from caller, dec kernel.Decision,
) (kernel.Decision, bool, error) {
	if d.ask == nil {
		dec.Reason = fmt.Sprintf("%s, and no surface could ask: a gating "+
			"question nobody answers is denied", dec.Reason)
		d.logDecision(from, dec, false)
		return dec, false, nil
	}

	answered, err := d.ask.Ask(ctx, Question{
		Principal: from.who,
		Decision:  dec,
		Method:    from.method,
		Args:      from.args,
	})
	if err != nil {
		dec.Reason = fmt.Sprintf("%s, and the question could not be put: %v",
			dec.Reason, err)
		d.logDecision(from, dec, false)
		return dec, false, nil
	}
	if !answered {
		dec.Reason += ", and the answer was no"
		d.logDecision(from, dec, false)
		return dec, false, nil
	}

	// The answer authorises THIS call and nothing else. Nothing is stored on
	// the connection, so the next call asks again.
	d.logDecision(from, dec, true)
	return dec, true, nil
}

// logDecision writes what was decided and why.
//
// The fields are the ones "why was I asked" and "why was that allowed" need:
// the pair, the action, the origin, the rule if one fired, and the principal
// that arrived. Section 15's recorded call log at M5 replaces this with
// something queryable; the vocabulary is the same either way.
func (d *Daemon) logDecision(from caller, dec kernel.Decision, allowed bool) {
	args := []any{
		"method", from.method,
		"pair", dec.Pair.String(),
		"action", dec.Action.String(),
		"origin", dec.Origin.String(),
		"principal", from.who.String(),
	}
	if dec.Rule != "" {
		args = append(args, "rule", dec.Rule)
	}
	if allowed {
		d.log.Debug("house rules allowed the call", args...)
		return
	}
	d.log.Warn("house rules refused the call", append(args, "reason", dec.Reason)...)
}
