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
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// The structured refusal, rendered (PLAN.md sections 9 and 10, M2 slice 1).
//
// Section 9: "A failed call never returns prose. It returns a structured
// error: what failed, which precondition, what the actual state was, and what
// would fix it - including, where it exists, the exact command that fixes
// it." The wire carries those four fields and the kernel fills them. This
// file is the half that puts them in front of whoever asked.
//
// Two renderings, one object. A person gets the sentence they already got,
// with the fields underneath it. An agent that passed --json gets the Status
// itself, because section 10 requires --json to return "exactly what the MCP
// tool returns" and slice 3 hands that same shape to MCP. A CLI-shaped
// envelope here would make the two surfaces disagree on the day MCP lands,
// which is the specific thing section 10 exists to prevent.

// refusal is a wire refusal that knows how the caller asked to see it.
//
// The mode has to travel with the error rather than be re-derived where it is
// printed: --json is parsed per command, against that command's own flag set,
// and a second parser in main would be a second place for `--json=false` and
// `--` to be got wrong.
type refusal struct {
	*client.CallError
	asJSON bool
}

// asRefusal marks a wire refusal with the caller's mode and leaves every other
// error exactly as it was.
//
// A local failure - no socket, bad flag, a program that answered something
// that is not JSON - has no Status behind it, so there is nothing structured
// to render and inventing one would be prose wearing an object's clothes.
func asRefusal(err error, asJSON bool) error {
	var ce *client.CallError
	if errors.As(err, &ce) {
		return &refusal{CallError: ce, asJSON: asJSON}
	}
	return err
}

// call is the only place in this package that reaches the wire.
//
// Every surface rig has returns its errors through here, so a refusal renders
// the same whether it came from `rig ping`, `rig apps`, `rig down` or a
// declared command. TestEveryWireCallInThisPackageGoesThroughCall is what
// stops the next one being added beside it instead of through it.
func call(ctx context.Context, c *client.Client, method string,
	in, out proto.Message, asJSON bool,
) error {
	return asRefusal(c.Call(ctx, method, in, out), asJSON)
}

// report is the one place a failure reaches the user, and it owns the choice
// of channel.
//
// Human mode writes to stderr, because there the result channel is prose for
// a person and a failure is not a result. --json writes the object to STDOUT,
// because in that mode the object IS the answer: section 10 promises --json
// on "every command, every view and every error", and an answer that moves to
// another channel when it is bad forces the caller to parse prose on the one
// path that most needs parsing. The exit status stays non-zero either way, so
// nothing reads a refusal as success.
//
// Anything that is not a wire refusal keeps the old behaviour in both modes -
// see the gap named on jsonStatus.
func report(stdout, stderr io.Writer, err error) {
	var r *refusal
	if errors.As(err, &r) && r.asJSON {
		if encErr := writeJSONStatus(stdout, r.Status); encErr == nil {
			return
		}
		// Falling through to prose rather than printing nothing: a caller
		// whose pipe has gone is better told on stderr than told nothing.
	}
	fmt.Fprintln(stderr, "rig: "+errorText(err))
}

// errorText is what a person reads: the sentence, then whichever of the four
// fields the daemon actually set.
func errorText(err error) string {
	var r *refusal
	if !errors.As(err, &r) {
		return err.Error()
	}
	return r.detail()
}

// detail renders only what is present, and that is a rule rather than a
// tidiness.
//
// The proto says so in as many words: all four fields are optional and
// absence means nothing, which is the opposite of a declaration's mandatory
// properties. A label with nothing after it reads as "rig looked and found
// none", and rig did not look - the daemon simply did not say. So an absent
// field gets no line at all, and a refusal that set none of them prints
// exactly what it printed before this file existed.
func (r *refusal) detail() string {
	var b strings.Builder
	b.WriteString(r.Error())
	for _, f := range []struct{ label, value string }{
		{"precondition", r.Status.GetPrecondition()},
		{"actual", r.Status.GetActual()},
		{"fix", r.Status.GetFix()},
		{"fix command", r.Status.GetFixCommand()},
	} {
		// Whitespace counts as absent. A field holding a space is not a fact
		// the daemon measured, and a label with blanks after it makes the
		// same claim an empty one would.
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
// Nothing stops a daemon putting one there - the schema validator's own text
// already carries newlines in `message` - and a second line at the left
// margin reads as a field of its own with a missing label, which is worse
// than a long line.
func wrapUnder(value string) string {
	return strings.ReplaceAll(value, "\n", "\n"+strings.Repeat(" ", fieldColumn))
}

// jsonStatus is the object --json returns for a failed call.
//
// It is the Status and nothing else. No CLI envelope, and deliberately no
// `method`: a field the CLI carries and the MCP tool does not is exactly the
// parallel rendering section 10 forbids, and the caller typed the method.
//
// Hand-written rather than protojson for two reasons. protojson varies its
// whitespace between runs on purpose, which is a poor property for a surface
// rig promises to agents; and its default names are lowerCamelCase where the
// proto field names are the contract every other surface will use.
// TestTheJSONObjectCoversEveryFieldOfStatus is what stops this struct drifting
// behind the proto when a field is added on the daemon side.
//
// KNOWN GAP, written here rather than discovered later: this covers errors
// that carry a Status. A local failure - rigd not running, a flag rig could
// not parse - still prints prose on stderr under --json. Section 10 says
// "every error", so that is real and it is not this slice: giving those a
// code is a decision about what rig's own failures are called on the wire,
// and inventing one here would put a second vocabulary beside the daemon's.
type jsonStatus struct {
	Code         string `json:"code"`
	Message      string `json:"message,omitempty"`
	Precondition string `json:"precondition,omitempty"`
	Actual       string `json:"actual,omitempty"`
	Fix          string `json:"fix,omitempty"`
	FixCommand   string `json:"fix_command,omitempty"`
}

// writeJSONStatus emits one Status as the indented object --json promises.
func writeJSONStatus(w io.Writer, s *rigv1.Status) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(jsonStatus{
		Code:         s.GetCode().String(),
		Message:      s.GetMessage(),
		Precondition: s.GetPrecondition(),
		Actual:       s.GetActual(),
		Fix:          s.GetFix(),
		FixCommand:   s.GetFixCommand(),
	})
}
