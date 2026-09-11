package kernel_test

import (
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
)

// TestASuccessorAndACollisionAreTheSameRefusal records section 34 gap 3 as a
// hole nobody has ruled on. It LOCKS what happens today; it does not change
// it.
//
// Two sessions legitimately occupy one role for a period: a program hands off
// to its successor, and for as long as the briefing lasts BOTH exist and both
// answer. The successor registering the program's name while the predecessor
// still holds it is, today, refused as a duplicate - internal/kernel's
// registry compares owner session ids and nothing else.
//
// THE FINDING IS NOT THE REFUSAL, IT IS THAT NOTHING TELLS THE TWO CASES
// APART. A successor of the same program and two unrelated programs racing
// for one name produce refusals of the same shape, from the same branch,
// differing only in the owner's rendered identity - which the caller cannot
// interpret, because it has never seen the holder's session id and has no way
// to learn it. So no client can decide whether to wait, retry or fail.
//
// Deliberately NOT added here: a handing-off state, a generation, a seat.
// All three are specification, and writing them as code would put the
// specification where nobody reviews it as one.
func TestASuccessorAndACollisionAreTheSameRefusal(t *testing.T) {
	decl := withCommands("shelf", map[string]kernel.Effects{
		"reindex": kernel.EffectsWritesFiles,
	})

	// A succession: the same program, a second session, while the first is
	// still registered and still answering.
	succession := func() error {
		k := kernel.New()
		if _, err := k.Register(programPrincipal("shelf"), decl); err != nil {
			t.Fatalf("register the predecessor: %v", err)
		}
		successor := programPrincipal("shelf")
		successor.SessionID = "s-shelf-successor"
		successor.PID = 4242
		_, err := k.Register(successor, decl)
		return err
	}

	// A collision: two unrelated programs that happen to want one name.
	collision := func() error {
		k := kernel.New()
		if _, err := k.Register(programPrincipal("shelf"), decl); err != nil {
			t.Fatalf("register the holder: %v", err)
		}
		intruder := programPrincipal("shelf")
		intruder.SessionID = "s-somebody-else"
		intruder.PID = 9999
		_, err := k.Register(intruder, withCommands("shelf",
			map[string]kernel.Effects{"nothing-alike": kernel.EffectsReadOnly}))
		return err
	}

	sErr, cErr := succession(), collision()
	if sErr == nil {
		t.Fatal("a successor registered over a live predecessor, so this test " +
			"no longer describes the code")
	}
	if cErr == nil {
		t.Fatal("an unrelated program took a held name")
	}

	// The refusal today, pinned so a change to it is a decision.
	const want = `kernel: program "shelf" is already registered by `
	for name, err := range map[string]error{"succession": sErr, "collision": cErr} {
		if !strings.HasPrefix(err.Error(), want) {
			t.Errorf("the %s refusal is not the one this test pins:\n got %q\nwant prefix %q",
				name, err.Error(), want)
		}
	}

	// THE ASSERTION THAT IS THE POINT, and it is stronger than "similar": the
	// two refusals are BYTE-IDENTICAL. Nothing about the newcomer reaches the
	// message - not its session, not its pid, not whether it declared the same
	// commands as the holder or an unrelated set. The only identity in the
	// sentence is the HOLDER's, which is the same in both cases because the
	// holder is the same program. A caller receiving this string cannot tell
	// which case it is in, and neither can a person reading the log.
	if sErr.Error() != cErr.Error() {
		t.Errorf("the two refusals now differ, so something distinguishes "+
			"them:\nsuccession %q\ncollision  %q", sErr, cErr)
	}

	// And the identity that IS there is the holder's session, which a caller
	// has never seen and cannot compare against anything it knows.
	if !strings.Contains(sErr.Error(), "session s-shelf") {
		t.Errorf("the refusal does not name the holder at all: %q", sErr)
	}
	if strings.Contains(sErr.Error(), "s-shelf-successor") {
		t.Errorf("the refusal now names the successor, which would give a "+
			"caller something to compare: %q", sErr)
	}

	// Nothing structured travels with it. Section 9 wants a failed call to
	// carry the precondition, the actual state and the fix; this refusal is
	// prose, so a client has a sentence and nothing to branch on. This is the
	// half a specification could close without deciding anything about
	// succession at all.
	if _, ok := kernel.AsRefusal(sErr); ok {
		t.Error("the refusal now carries structure, so the prose-only finding " +
			"this test records is stale")
	}
}

// TestTheHolderIsNamedByASessionTheCallerHasNeverSeen states the reason the
// identity in the refusal cannot be used to tell succession from collision,
// as an assertion rather than as a comment.
//
// A registering program knows its OWN session id and nothing else: session
// ids are minted per connection by the daemon and never travel. So the one
// field that differs between the two refusals is one the caller has no way
// to interpret.
func TestTheHolderIsNamedByASessionTheCallerHasNeverSeen(t *testing.T) {
	k := kernel.New()
	decl := withCommands("shelf", map[string]kernel.Effects{
		"reindex": kernel.EffectsWritesFiles,
	})
	holder := programPrincipal("shelf")
	if _, err := k.Register(holder, decl); err != nil {
		t.Fatalf("register: %v", err)
	}

	newcomer := programPrincipal("shelf")
	newcomer.SessionID = "s-newcomer"
	_, err := k.Register(newcomer, decl)
	if err == nil {
		t.Fatal("the second registration was accepted")
	}
	if !strings.Contains(err.Error(), holder.SessionID) {
		t.Fatalf("the refusal does not name the holding session: %q", err)
	}
	if strings.Contains(err.Error(), newcomer.SessionID) {
		t.Fatalf("the refusal names the CALLER's session, which would give it "+
			"something to compare: %q", err)
	}
}
