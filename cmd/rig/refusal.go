package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/paths"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// The structured failure, rendered (PLAN.md sections 9 and 10).
//
// Section 9: "A failed call never returns prose. It returns a structured
// error: what failed, which precondition, what the actual state was, and what
// would fix it - including, where it exists, the exact command that fixes
// it." Section 10 then requires --json on "every command, every view and
// every error", returning exactly what the MCP tool returns.
//
// EVERY ERROR is the part this file takes literally. A refusal the daemon
// sent and a failure rig produced on its own render through one shape, not
// two: an agent that has to tell prose from an object by looking at which
// thing failed has not been given a machine surface. So there are two
// producers below and one renderer, and the renderer is what --json and the
// terminal both go through.
//
// Two renderings, one object. A person gets the sentence they already got,
// with whichever fields are present underneath it. An agent that passed
// --json gets the object, because slice 3 hands that same shape to MCP and a
// CLI-shaped envelope here would make the two surfaces disagree on the day
// MCP lands.

// ---- the code sets, and why they cannot collide ---------------------------

// codeLocal prefixes every code rig mints for a failure of its own.
//
// The constraint is that rig's codes must never collide with the daemon's,
// and the cheapest way to hold it is to make it true by construction rather
// than by review: every code the daemon can send is a value of the wire's
// Code enum and is spelled CODE_*, so a code spelled RIG_* is disjoint from
// all of them AND from every one added to that enum later.
//
// TestRigsOwnCodesCannotCollideWithTheDaemons walks the enum descriptor and
// proves it, so the guarantee survives someone naming a rig code CODE_
// something out of habit.
const codeLocal = "RIG_"

const (
	// codeNoDaemon: rig could not reach rigd at all. The most useful
	// structured error rig has, because it is the one an agent hits first and
	// the only one whose fix command is certain.
	codeNoDaemon = codeLocal + "NO_DAEMON"

	// codeNoSuchProgram: the registry rig read does not name that program.
	// Distinct from the daemon's CODE_NOT_FOUND, which is the daemon's own
	// routing answer - this one is rig reading a snapshot.
	codeNoSuchProgram = codeLocal + "NO_SUCH_PROGRAM"

	// codeNoSuchCommand: that program is there and its declaration does not
	// name that command. Separate from the one above because the fix differs:
	// a missing program is started, a missing command is declared, and
	// section 5k's coverage is what tells the two apart.
	codeNoSuchCommand = codeLocal + "NO_SUCH_COMMAND"

	// codeBadArgument: argv did not parse, or contradicted itself, before
	// anything left this process.
	codeBadArgument = codeLocal + "BAD_ARGUMENT"

	// codeTimeout: rig gave up waiting. NOT the daemon's CODE_DEADLINE, and
	// the difference is the whole reason this is its own code: CODE_DEADLINE
	// says the call missed the deadline the daemon supervises, this says the
	// client ran out of patience and the call may still be running.
	codeTimeout = codeLocal + "TIMEOUT"

	// codeBadResult: an answer arrived and is not what the declaration
	// promised. The program's defect, reported rather than swallowed.
	codeBadResult = codeLocal + "BAD_RESULT"
)

// ---- the two producers ----------------------------------------------------

// refusal is a wire refusal that knows how the caller asked to see it.
type refusal struct {
	*client.CallError
	asJSON bool
}

// rigError is a failure rig produced itself, wearing the shape a daemon
// refusal wears.
//
// The fields are filled only where rig actually knows the answer. Section 9's
// value is that a precondition is TRUE when set, and rig inventing one for
// its own failures would be the same defect as a daemon inventing one.
type rigError struct {
	object jsonStatus
	asJSON bool
	cause  error
}

func (e *rigError) Error() string { return e.object.Message }

// Unwrap keeps whatever chain the failure already had. cmdDown decides that a
// dial failure is a successful stop by matching two syscall errnos through
// errors.Is, and that has to keep working after the error gains a shape.
func (e *rigError) Unwrap() error { return e.cause }

// local builds one. The sentence goes in Message, so an error rig already
// words well keeps its wording and gains structure rather than being
// rewritten into it.
func local(s jsonStatus) error { return &rigError{object: s} }

// noDaemon is the failure an agent hits first, and the one section 9
// describes best: the precondition is nameable, the actual state is
// checkable, and the fix has a command that always works.
//
// It replaces the "is rigd running? start it with: rigd" hint that six call
// sites appended by hand. The hint was the fix line all along, so it moves
// into the field rather than being repeated inside the sentence.
func noDaemon(cause error) error {
	actual := "nothing is listening on the rig socket"
	if sock, err := paths.Socket(); err == nil {
		actual = "nothing is listening on " + sock
	}
	return &rigError{cause: cause, object: jsonStatus{
		Code:         codeNoDaemon,
		Message:      cause.Error(),
		Precondition: "the rig daemon is running",
		Actual:       actual,
		Fix:          "start rigd, then run this again",
		FixCommand:   "rigd",
	}}
}

// badArgumentf is the short form for a failure with nothing structured to say
// yet, and every one of those is argv's fault so far.
//
// It exists so converting a call site is never a reason to invent a
// precondition: a code and the sentence the site already had is a complete,
// honest answer, and it is already strictly better than prose because --json
// can carry it. A second short form belongs here when a second code needs
// one, not before.
func badArgumentf(format string, a ...any) error {
	return local(jsonStatus{Code: codeBadArgument, Message: fmt.Sprintf(format, a...)})
}

// ---- the one wire entry point ---------------------------------------------

// call is the only place in this package that reaches the wire.
//
// Every surface rig has returns its errors through here, so a refusal renders
// the same whether it came from `rig ping`, `rig apps`, `rig down` or a
// declared command. TestEveryWireCallInThisPackageGoesThroughCall is what
// stops the next one being added beside it instead of through it.
//
// It does not take the output mode. A refusal starts in human mode and the
// command that parsed --json stamps it on the way out, which is what inMode
// does - so the prose surfaces (help, completion) need no mode at all and
// cannot get it wrong.
func call(ctx context.Context, c *client.Client, method string,
	in, out proto.Message,
) error {
	err := c.Call(ctx, method, in, out)
	if err == nil {
		return nil
	}
	var ce *client.CallError
	if errors.As(err, &ce) {
		return &refusal{CallError: ce}
	}
	// Not a refusal: the call never got an answer. A deadline is rig's own
	// patience running out rather than the daemon's supervision deadline, and
	// section 18 makes those two different facts.
	if errors.Is(err, context.DeadlineExceeded) {
		return local(jsonStatus{
			Code:         codeTimeout,
			Message:      err.Error(),
			Precondition: method + " answers before rig stops waiting",
			Actual:       "no answer arrived, and the call may still be running",
			Fix:          "wait longer with --timeout, or ask rigd what the program is doing",
			FixCommand:   "rig apps list",
		})
	}
	return err
}

// inMode stamps the caller's output mode onto whatever a command failed with.
//
// Named-return plus defer at each command that parses --json, so a failure
// produced anywhere between argv and the socket is rendered the way the
// caller asked without every one of those returns carrying a bool.
func inMode(err error, asJSON bool) error {
	var r *refusal
	if errors.As(err, &r) {
		r.asJSON = asJSON
		return err
	}
	var l *rigError
	if errors.As(err, &l) {
		l.asJSON = asJSON
		return err
	}
	return err
}

// ---- the one renderer -----------------------------------------------------

// report is the one place a failure reaches the user, and it owns the choice
// of channel.
//
// Human mode writes to stderr, because there the result channel is prose for
// a person and a failure is not a result. --json writes the object to STDOUT,
// because in that mode the object IS the answer: section 10 promises --json
// on every error, and an answer that moves to another channel when it is bad
// forces the caller to parse prose on the one path that most needs parsing.
// The exit status stays non-zero either way, so nothing reads a failure as
// success.
func report(stdout, stderr io.Writer, err error) {
	obj, asJSON, structured := shape(err)
	if structured && asJSON {
		if encErr := writeJSONStatus(stdout, obj); encErr == nil {
			return
		}
		// Falling through to prose rather than printing nothing: a caller
		// whose pipe has gone is better told on stderr than told nothing.
	}
	fmt.Fprintln(stderr, "rig: "+errorText(err))
}

// shape reduces any error to the object --json emits, and says whether there
// was one and how the caller asked to see it.
func shape(err error) (obj jsonStatus, asJSON, structured bool) {
	var r *refusal
	if errors.As(err, &r) {
		s := r.Status
		return jsonStatus{
			Code: s.GetCode().String(),
			// The daemon's own sentence, NOT r.Error(): the object is the
			// Status, and the method and code the human line prefixes are
			// already fields of it.
			Message:      s.GetMessage(),
			Precondition: s.GetPrecondition(),
			Actual:       s.GetActual(),
			Fix:          s.GetFix(),
			FixCommand:   s.GetFixCommand(),
		}, r.asJSON, true
	}
	var l *rigError
	if errors.As(err, &l) {
		return l.object, l.asJSON, true
	}
	return jsonStatus{}, false, false
}

// errorText is what a person reads: the sentence, then whichever of the four
// fields was actually set.
//
// A rig failure keeps the exact sentence it has today, with no code prefixed.
// The daemon's line already carries `method: CODE_X:` and rig's does not, so
// the two stay distinguishable in a terminal without changing every error
// message rig has ever printed.
func errorText(err error) string {
	obj, _, structured := shape(err)
	if !structured {
		return err.Error()
	}
	sentence := err.Error()
	return detail(sentence, obj)
}

// detail renders only what is present, and that is a rule rather than a
// tidiness.
//
// The proto says so in as many words: all four fields are optional and
// absence means nothing, which is the opposite of a declaration's mandatory
// properties. A label with nothing after it reads as "rig looked and found
// none", and rig did not look - whoever produced the failure simply did not
// say. So an absent field gets no line at all.
func detail(sentence string, obj jsonStatus) string {
	var b strings.Builder
	b.WriteString(sentence)
	for _, f := range []struct{ label, value string }{
		{"precondition", obj.Precondition},
		{"actual", obj.Actual},
		{"fix", obj.Fix},
		{"fix command", obj.FixCommand},
	} {
		// Whitespace counts as absent. A field holding a space is not a fact
		// anybody measured, and a label with blanks after it makes the same
		// claim an empty one would.
		value := strings.TrimSpace(f.value)
		if value == "" {
			continue
		}
		// The seven-space continuation is this package's existing shape for a
		// second line under `rig: ` - see lookup's "it declares:" and the
		// "is rigd running?" hint.
		fmt.Fprintf(&b, "\n       %-13s %s", f.label, wrapUnder(value))
	}
	return b.String()
}

// fieldColumn is the column a field's value starts in: seven spaces of
// continuation, the thirteen-wide label, and the space after it.
const fieldColumn = 7 + 13 + 1

// wrapUnder keeps a value that arrived with a newline in it under its own
// label.
//
// Nothing stops a producer putting one there - the schema validator's own
// text already carries newlines in `message` - and a second line at the left
// margin reads as a field of its own with a missing label, which is worse
// than a long line.
func wrapUnder(value string) string {
	return strings.ReplaceAll(value, "\n", "\n"+strings.Repeat(" ", fieldColumn))
}

// ---- the object -----------------------------------------------------------

// jsonStatus is the object --json returns for a failure, whoever produced it.
//
// For a wire refusal it is the Status and nothing else: no CLI envelope, and
// deliberately no `method`, because a field the CLI carries and the MCP tool
// does not is exactly the parallel rendering section 10 forbids. A rig
// failure fills the same shape, with a RIG_ code so a reader can tell which
// side of the socket answered.
//
// Hand-written rather than protojson for two reasons. protojson varies its
// whitespace between runs on purpose, which is a poor property for a surface
// rig promises to agents; and its default names are lowerCamelCase where the
// proto field names are the contract every other surface will use.
// TestTheJSONObjectCoversEveryFieldOfStatus is what stops this struct
// drifting behind the proto when a field is added on the daemon side.
type jsonStatus struct {
	Code         string `json:"code"`
	Message      string `json:"message,omitempty"`
	Precondition string `json:"precondition,omitempty"`
	Actual       string `json:"actual,omitempty"`
	Fix          string `json:"fix,omitempty"`
	FixCommand   string `json:"fix_command,omitempty"`
}

// writeJSONStatus emits one object as the indented JSON --json promises.
func writeJSONStatus(w io.Writer, obj jsonStatus) error {
	if obj.Code == "" {
		obj.Code = rigv1.Code_CODE_UNSPECIFIED.String()
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(obj)
}
