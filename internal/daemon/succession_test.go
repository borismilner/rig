package daemon

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/client"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// helloErr connects, declares "shelf", and returns whatever the daemon
// answered. The name is fixed rather than a parameter: every caller here is
// asking what happens when the SAME name is claimed twice, and a parameter
// only ever passed one value is a parameter that invites a reader to look
// for the case that varies it.
func helloErr(t *testing.T, sock string) error {
	t.Helper()
	c, err := client.Dial(sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	c.Handle(func(method string, _ []byte) (proto.Message, error) {
		return &rigv1.CallResponse{Result: []byte(`{"ran":"` + method + `"}`)}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = c.Hello(ctx, testDeclaration("shelf"))
	return err
}

// TestASuccessorIsRefusedAsADuplicateOverTheWire is the daemon-level half of
// section 34 gap 3. It LOCKS what the wire says today; it changes nothing.
//
// A program handing off to its successor is two sessions occupying one role
// for as long as the briefing lasts. Both exist, both answer, and the
// successor has to register the name before the predecessor can let it go.
// Today that registration is refused outright as a duplicate.
//
// WHAT THIS PINS, because the lead is writing the specification against it:
//
//   - the CODE on the wire is CODE_DENIED
//   - the MESSAGE is the kernel's, prose, naming the holder's session and pid
//   - NOTHING STRUCTURED TRAVELS WITH IT. The hello path calls fail, not
//     failErr, so precondition, actual, fix and fix_command are all empty -
//     and this is exactly the refusal section 9 would want them on, because
//     "wait for the holder to go" and "pick another name" are different fixes
//     and the wire cannot express either.
//
// The consequence for a client is the whole finding: a successor and an
// unrelated program racing for one name receive the same code and a message
// that differs in nothing the caller can interpret, so no client can decide
// between waiting, retrying and failing.
//
// Deliberately NOT added: a handing-off state, a generation, a seat.
func TestASuccessorIsRefusedAsADuplicateOverTheWire(t *testing.T) {
	sock, _ := upDaemon(t, nil)

	if err := helloErr(t, sock); err != nil {
		t.Fatalf("the predecessor could not register: %v", err)
	}

	// The successor, on its own connection, while the predecessor is still
	// connected and still answering.
	err := helloErr(t, sock)
	if err == nil {
		t.Fatal("a successor registered alongside a live predecessor, so this " +
			"test no longer describes the code")
	}

	var ce *client.CallError
	if !errors.As(err, &ce) {
		t.Fatalf("the refusal did not arrive as a CallError: %T %v", err, err)
	}
	st := ce.Status

	if st.GetCode() != rigv1.Code_CODE_DENIED {
		t.Errorf("the duplicate refusal arrives as %v, and this test pinned "+
			"CODE_DENIED", st.GetCode())
	}
	if !strings.Contains(st.GetMessage(), "already registered by") {
		t.Errorf("the refusal is not the kernel's duplicate refusal: %q",
			st.GetMessage())
	}
	// The daemon has a SECOND duplicate message - "is already connected",
	// from its own routing map - and it is not the one that fires. The
	// kernel's registration guard is reached first. Pinned because a
	// specification that changes one of them has to know there are two.
	if strings.Contains(st.GetMessage(), "already connected") {
		t.Errorf("the routing map's refusal now fires first: %q", st.GetMessage())
	}

	// The half a specification could close without ruling on succession at
	// all: section 9 says a failed call never returns prose, and this one
	// does.
	for _, f := range []struct{ name, got string }{
		{"precondition", st.GetPrecondition()},
		{"actual", st.GetActual()},
		{"fix", st.GetFix()},
		{"fix_command", st.GetFixCommand()},
	} {
		if f.got != "" {
			t.Errorf("%s is now set on the duplicate refusal (%q), so the "+
				"prose-only finding this test records is stale", f.name, f.got)
		}
	}
}

// TestTheRefusalDoesNotSayWhetherWaitingWouldHelp is the consequence stated
// as an assertion rather than as a comment.
//
// The two things a client could do differ completely - a successor should
// wait for the predecessor to finish handing off, an unrelated program should
// take another name - and the refusal contains no word that separates them.
// The check is deliberately crude: if any of these ever appears, somebody has
// started answering the question and this test should be rewritten against
// what they decided.
func TestTheRefusalDoesNotSayWhetherWaitingWouldHelp(t *testing.T) {
	sock, _ := upDaemon(t, nil)
	if err := helloErr(t, sock); err != nil {
		t.Fatalf("the predecessor could not register: %v", err)
	}
	err := helloErr(t, sock)
	if err == nil {
		t.Fatal("the second registration was accepted")
	}

	for _, word := range []string{
		"retry", "wait", "handing off", "handoff", "succession",
		"transient", "temporary",
	} {
		if strings.Contains(strings.ToLower(err.Error()), word) {
			t.Errorf("the refusal now says %q, so it has begun to distinguish "+
				"a succession from a collision: %v", word, err)
		}
	}
}
