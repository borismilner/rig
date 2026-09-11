package daemon

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/internal/instance"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// upEstate is upDaemon with a claimed estate name, which the shared helper has
// no reason to carry.
func upEstate(t *testing.T, name string) string {
	t.Helper()
	// Short deliberately: sun_path is 108 bytes and t.TempDir under a long
	// TMPDIR silently exceeds it, failing as EINVAL.
	dir, err := os.MkdirTemp("", "rige")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "s")

	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := instance.Acquire(filepath.Join(dir, "p"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })

	d, err := New(Config{Version: "test-build", Wire: "v1", Estate: name, Lock: lock})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); _ = d.Serve(ctx, l) }()
	t.Cleanup(func() { cancel(); <-done })
	return sock
}

func estateOf(t *testing.T, sock string) *rigv1.EstateResponse {
	t.Helper()
	resp := &rigv1.EstateResponse{}
	if err := dial(t, sock).Call(ctx5(t), "rig.estate", &rigv1.EstateRequest{}, resp); err != nil {
		t.Fatalf("rig.estate: %v", err)
	}
	return resp
}

// Boris: "The peers (all agents) will know which rig instance is which and will
// know when to be talking to which and for what purposes." An agent could not
// know that: XDG_RUNTIME_DIR is inherited, so the estate was something an agent
// is placed in and never something it reads.
func TestEstateAnswersItsNameAndItsDerivedRole(t *testing.T) {
	for _, tc := range []struct {
		name string
		want rigv1.EstateRole
	}{
		{"production", rigv1.EstateRole_ESTATE_ROLE_PRODUCTION},
		{"development", rigv1.EstateRole_ESTATE_ROLE_DEVELOPMENT},
		{"", rigv1.EstateRole_ESTATE_ROLE_UNNAMED},
	} {
		label := tc.name
		if label == "" {
			label = "unnamed"
		}
		t.Run(label, func(t *testing.T) {
			got := estateOf(t, upEstate(t, tc.name))
			if got.GetName() != tc.name {
				t.Errorf("name is %q, want %q", got.GetName(), tc.name)
			}
			if got.GetRole() != tc.want {
				t.Errorf("role is %v, want %v", got.GetRole(), tc.want)
			}
			if got.GetDaemonVersion() != "test-build" {
				t.Errorf("daemon_version is %q, want the daemon's own build",
					got.GetDaemonVersion())
			}
			if got.GetWire() != "v1" {
				t.Errorf("wire is %q, want v1", got.GetWire())
			}
			if got.GetSemanticsGen() != selfDeclaration().SemanticsGen {
				t.Errorf("semantics_gen is %d, want rig's own declared %d",
					got.GetSemanticsGen(), selfDeclaration().SemanticsGen)
			}
		})
	}
}

// AN UNNAMED ESTATE MUST NEVER ANSWER UNSPECIFIED, and this is the assertion
// the enum was reshaped for.
//
// proto3 cannot tell an unset scalar from a zero one. If unnamed WERE the zero,
// a daemon carrying the field and failing to set it would render as "this is an
// unnamed estate" and an agent would branch on it - and unnamed reads as
// ephemeral and disposable where production does not. So the zero keeps section
// 21's "nothing was said" and the identity fact has a number of its own.
func TestAnUnnamedEstateIsAFactAndNotAnAbsence(t *testing.T) {
	got := estateOf(t, upEstate(t, ""))
	if got.GetRole() == rigv1.EstateRole_ESTATE_ROLE_UNSPECIFIED {
		t.Fatal("an unnamed estate answered UNSPECIFIED, which is what a " +
			"daemon that forgot to set the field also answers. The two must " +
			"never be the same value")
	}
	if got.GetRole() != rigv1.EstateRole_ESTATE_ROLE_UNNAMED {
		t.Fatalf("role is %v, want UNNAMED", got.GetRole())
	}
}

// SECTION 14, RULED: rig.estate is UNSCOPED and every caller kind gets the full
// answer. It is the only rig method whose answer does not depend on who is
// asking, because it aggregates nothing - every field is a fact about the
// daemon and none is data belonging to another principal.
//
// THIS IS THE PROPERTY A LATER FIELD COULD QUIETLY DESTROY, which is why it is
// asserted rather than left to the comment. A registered program is the caller
// that would notice first: it is the one kind whose reads are scoped.
func TestEstateIsUnscopedSoAScopedCallerGetsTheSameAnswer(t *testing.T) {
	sock := upEstate(t, "production")

	unscoped := estateOf(t, sock)

	// A registered program: the caller kind whose other reads ARE filtered.
	scoped := &rigv1.EstateResponse{}
	p := program(t, sock, "shelf")
	if err := p.Call(ctx5(t), "rig.estate", &rigv1.EstateRequest{}, scoped); err != nil {
		t.Fatalf("a registered program was refused rig.estate: %v", err)
	}

	if !proto.Equal(unscoped, scoped) {
		t.Fatalf("a scoped caller got a different estate answer:\n  unscoped %v\n  scoped   %v\n"+
			"rig.estate aggregates nothing and must not depend on who is asking",
			unscoped, scoped)
	}
}

// The role cannot be sent, so the two can never disagree. That is structural
// rather than enforced: EstateRequest carries nothing at all, so there is no
// second place to write a role from.
func TestTheRoleCannotBeSentByAClient(t *testing.T) {
	if n := (&rigv1.EstateRequest{}).ProtoReflect().Descriptor().Fields().Len(); n != 0 {
		t.Fatalf("EstateRequest has %d fields; it must have none, so a client "+
			"can never send a role and the derivation has exactly one home", n)
	}
}
