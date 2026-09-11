package daemon

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/wire"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// The example the program's own author wrote. fix_command may only ever be
// this, never a string rig assembled: see firstExample in internal/kernel.
const atlasExample = "rig atlas reindex --since 7d"

// atlasDeclaration is destructiveDeclaration's neighbour and exists because
// that one declares no examples. Without an example the kernel leaves
// fix_command empty by design, so a test built on it could never tell a
// daemon that drops the field from one that never had it.
func atlasDeclaration(id string) *rigv1.Declaration {
	d := testDeclaration(id)
	reindex := proto.Clone(d.GetCommands()[0]).(*rigv1.Command)
	reindex.Id = "reindex"
	reindex.Title = "Reindex"
	reindex.Effects = rigv1.Effects_EFFECTS_WRITES_FILES
	reindex.Args = []byte(`{"type":"object","additionalProperties":false,` +
		`"required":["since"],"properties":{"since":{"type":"string"}}}`)
	reindex.Examples = []string{atlasExample}
	reindex.Summary = "Rebuild the index"
	reindex.Description = "Rebuilds the index from the tree."
	reindex.Returns = "The number of items indexed."

	d.Commands = append(d.Commands, reindex)
	return d
}

// atlas connects a program whose one interesting command declares BOTH a
// schema and an example, which is what makes all four of section 9's fields
// reachable in a single refusal.
func atlas(t *testing.T, sock string) {
	t.Helper()
	c := dial(t, sock)
	c.Handle(func(method string, _ []byte) (proto.Message, error) {
		return &rigv1.CallResponse{Result: []byte(`{"ran":"` + method + `"}`)}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Hello(ctx, atlasDeclaration("atlas")); err != nil {
		t.Fatal(err)
	}
}

// callStatus makes one call that is expected to fail and returns the Status
// the CALLER received - the far side of a real unix socket, not the kernel's
// own return value.
func callStatus(t *testing.T, c *client.Client, method string, args []byte) *rigv1.Status {
	t.Helper()
	var resp rigv1.CallResponse
	err := c.Call(ctx5(t), method, &rigv1.CallRequest{Args: args}, &resp)
	if err == nil {
		t.Fatalf("%s with args %q was accepted", method, args)
	}
	var ce *client.CallError
	if !errors.As(err, &ce) {
		t.Fatalf("the refusal did not arrive as a CallError: %T %v", err, err)
	}
	return ce.Status
}

// TestAStructuredRefusalSurvivesTheSocket is the daemon-level half of M2
// slice 1. internal/kernel proves the refusal is BUILT with section 9's four
// fields; this proves they are still there after the frame has been
// marshalled, written to a unix socket, read back and unmarshalled.
//
// The assertion is equality against the kernel's own refusal rather than
// against literal strings. A test that hard-codes the wording locks the
// message instead of the plumbing, and it goes red when someone improves a
// sentence - which is the failure that teaches a reader to edit the test.
func TestAStructuredRefusalSurvivesTheSocket(t *testing.T) {
	sock, d := upDaemon(t, nil)
	atlas(t, sock)

	const method = "atlas.reindex"
	args := []byte(`{"since":7}`)

	// What the kernel produces in-process, with no wire involved.
	direct := d.kernel.ValidateArgs("atlas", "reindex", args)
	want, ok := kernel.AsRefusal(direct)
	if !ok {
		t.Fatalf("the kernel refused without structure: %v", direct)
	}
	if want.Precondition == "" || want.Actual == "" ||
		want.Fix == "" || want.FixCommand == "" {
		t.Fatalf("the fixture cannot prove the wire: the kernel left a field "+
			"empty (%+v)", want)
	}

	got := callStatus(t, dial(t, sock), method, args)

	for _, f := range []struct {
		name      string
		got, want string
	}{
		{"message", got.GetMessage(), want.Error()},
		{"precondition", got.GetPrecondition(), want.Precondition},
		{"actual", got.GetActual(), want.Actual},
		{"fix", got.GetFix(), want.Fix},
		{"fix_command", got.GetFixCommand(), want.FixCommand},
	} {
		if f.got != f.want {
			t.Errorf("%s did not survive the socket:\n got %q\nwant %q",
				f.name, f.got, f.want)
		}
	}
	if got.GetCode() != rigv1.Code_CODE_INVALID {
		t.Errorf("a schema refusal arrived as %v", got.GetCode())
	}
	if got.GetFixCommand() != atlasExample {
		t.Errorf("fix_command is not the program's own example: %q",
			got.GetFixCommand())
	}
}

// TestAnAbsentPreconditionArrivesAbsent locks the other half of "optional by
// design". The kernel deliberately names NO precondition when the arguments
// are not JSON at all - there is no field to point at, and inventing one
// would be a guess wearing a measurement's clothes (internal/kernel/args.go).
//
// The surface that renders this prints nothing for an absent field, so a
// daemon that filled it with a placeholder would produce a line that is
// false. Only a caller-side assertion catches that.
func TestAnAbsentPreconditionArrivesAbsent(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	atlas(t, sock)

	got := callStatus(t, dial(t, sock), "atlas.reindex", []byte(`not json`))

	if got.GetPrecondition() != "" {
		t.Errorf("a precondition was invented for a non-JSON payload: %q",
			got.GetPrecondition())
	}
	if got.GetActual() != "" {
		t.Errorf("an actual state was invented for a non-JSON payload: %q",
			got.GetActual())
	}
	// The two that ARE knowable without parsing anything.
	if got.GetFix() == "" {
		t.Error("no fix was offered for a payload that is not JSON")
	}
	if got.GetFixCommand() != atlasExample {
		t.Errorf("fix_command is not the program's own example: %q",
			got.GetFixCommand())
	}
}

// TestEveryRefusalShapeCarriesItsStructure proves the plumbing is not
// specific to the schema failure the test above uses. An undeclared command
// is refused earlier, by a different branch of ValidateArgs, and section 9
// asks the same four fields of it. A daemon that only carried the structure
// for one shape would pass the test above and still tell an agent nothing
// about the mistake it is most likely to make.
func TestEveryRefusalShapeCarriesItsStructure(t *testing.T) {
	sock, d := upDaemon(t, nil)
	atlas(t, sock)

	direct := d.kernel.ValidateArgs("atlas", "nosuch", nil)
	want, ok := kernel.AsRefusal(direct)
	if !ok {
		t.Fatalf("an undeclared command was refused without structure: %v", direct)
	}

	got := callStatus(t, dial(t, sock), "atlas.nosuch", nil)

	for _, f := range []struct {
		name      string
		got, want string
	}{
		{"precondition", got.GetPrecondition(), want.Precondition},
		{"actual", got.GetActual(), want.Actual},
		{"fix", got.GetFix(), want.Fix},
		{"fix_command", got.GetFixCommand(), want.FixCommand},
	} {
		if f.got != f.want {
			t.Errorf("%s did not survive the socket:\n got %q\nwant %q",
				f.name, f.got, f.want)
		}
	}
	// The one field an agent acts on hardest: what the program DOES declare.
	if !strings.Contains(got.GetActual(), "reindex") {
		t.Errorf("the refusal does not name what atlas declares: %q",
			got.GetActual())
	}
}

// TestAPlainErrorStillProducesARefusalFrame covers failErr's other branch.
//
// THAT BRANCH HAS NO REACHABLE CALLER TODAY, and this test exists because of
// it rather than in spite of it. failErr has exactly one call site
// (internal/daemon/args.go), it passes kernel.ValidateArgs' error, and every
// non-nil return from ValidateArgs is a *kernel.RefusalError - so the `if ok`
// guard is defensive. The contract it defends is that the structure is an
// ADDITION and never a condition of reporting a failure, which is exactly
// what a second call site would rely on without checking. Locking it now
// costs one test; discovering it was never true costs a silent dropped
// refusal.
//
// It drives the frame codec over net.Pipe rather than a unix socket: the
// socket is proved above, and what is under test here is the branch, not the
// transport.
func TestAPlainErrorStillProducesARefusalFrame(t *testing.T) {
	server, caller := net.Pipe()
	t.Cleanup(func() { _ = server.Close(); _ = caller.Close() })

	c := &conn{w: wire.NewConn(server), log: slog.New(slog.DiscardHandler)}
	go c.failErr(7, rigv1.Code_CODE_INTERNAL, errors.New("a plain error"))

	f, err := wire.NewConn(caller).ReadFrame()
	if err != nil {
		t.Fatalf("no frame arrived: %v", err)
	}
	if f.GetKind() != rigv1.FrameKind_FRAME_KIND_ERROR {
		t.Fatalf("a failure arrived as %v", f.GetKind())
	}
	if f.GetStreamId() != 7 {
		t.Errorf("the failure landed on stream %d", f.GetStreamId())
	}
	st := f.GetStatus()
	if st.GetMessage() != "a plain error" {
		t.Errorf("the sentence was lost: %q", st.GetMessage())
	}
	if st.GetCode() != rigv1.Code_CODE_INTERNAL {
		t.Errorf("the code was lost: %v", st.GetCode())
	}
	// Nothing was known, so nothing is claimed.
	if st.GetPrecondition() != "" || st.GetActual() != "" ||
		st.GetFix() != "" || st.GetFixCommand() != "" {
		t.Errorf("structure was invented for an error that carried none: %+v", st)
	}
}
