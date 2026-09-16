package meta_test

import (
	"context"
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/meta"
)

// The window sentence, verbatim. It is a constant here rather than a substring
// spelled out at each use BECAUSE THE TEST IS ABOUT IT APPEARING IN EVERY
// BRANCH: a test that wrote it out three times could drift into checking three
// different sentences and still pass.
const windowSentence = "rig remembers departures for the last"

// departedEstate is one registered program that has since gone.
//
// It goes through Register and Deregister rather than reaching into the
// registry, because the tombstone is written BY Deregister and a test that
// planted one would pass against a Deregister that wrote nothing.
func departedEstate(t *testing.T) *kernel.Kernel {
	t.Helper()
	k := estate(t, kernel.CoverageFull)
	k.Deregister("s-shelf")
	return k
}

func refusal(t *testing.T, k *kernel.Kernel, who kernel.Principal, program string) string {
	t.Helper()
	_, err := meta.New(k, nil).Answer(context.Background(), who,
		meta.Request{Tool: meta.Describe, Program: program})
	if err == nil {
		t.Fatalf("describe %q: wanted a refusal, got none", program)
	}
	return err.Error()
}

// TestAGoneProgramIsNamedAsGoneRatherThanAsHIDDEN is row 0's clause 2, the
// half that was buildable without a change to the wire.
//
// FOUND BY DEMONSTRATION, 2026-09-16: a holder was killed on a live daemon and
// `describe` answered "is not visible to this caller" - the same sentence a
// caller outside its scope gets. A dead program and a permission boundary call
// for OPPOSITE actions: one is a crash to react to, the other is an access
// request to file. Answering both with one sentence sends an agent after a
// grant that would not have helped.
func TestAGoneProgramIsNamedAsGoneRatherThanAsHIDDEN(t *testing.T) {
	got := refusal(t, departedEstate(t), agent(), "shelf")

	for _, want := range []string{
		"was registered here and left at",
		"A grant will not bring it back",
		windowSentence,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("refusal does not say %q:\n%s", want, got)
		}
	}
	// AND IT MUST NOT REVERT TO THE OLD CLAIM. The wording that failed the
	// clause asserted scope; a refusal that named the departure AND kept the
	// old sentence would read as both at once.
	if strings.Contains(got, "outside this caller's scope") {
		t.Errorf("a departed program is still being blamed on scope:\n%s", got)
	}
}

// TestTheWindowSentenceIsInBOTHBRANCHES is a SECURITY case, not a wording one.
//
// If rig said "it remembers departures for the last N" only when it HAS a
// tombstone, then the PRESENCE OF THAT SENTENCE WOULD ITSELF BE THE TOMBSTONE
// - every caller would learn which programs departed regardless of scope, and
// the side channel would be in the prose rather than in the data, where a test
// on the returned struct could never see it.
//
// It also earns its place on its own: without it, absence is ambiguous all
// over again. No tombstone means never-registered OR expired, and the caller
// cannot tell which. With it, silence past N is interpretable rather than
// evidence.
func TestTheWindowSentenceIsInBOTHBRANCHES(t *testing.T) {
	k := departedEstate(t)

	gone := refusal(t, k, agent(), "shelf")
	never := refusal(t, k, agent(), "never-registered")

	for name, got := range map[string]string{"gone": gone, "never registered": never} {
		if !strings.Contains(got, windowSentence) {
			t.Errorf("the %s branch omits the window:\n%s", name, got)
		}
	}
}

// TestATombstoneDoesNotREVEALAProgramTheCallerCouldNotSee is the invariant the
// whole mechanism stands on: A TOMBSTONE IS A PROJECTION AND IT INHERITS THE
// DEAD PROGRAM'S SCOPE.
//
// Without it, register nothing, wait, and read off every program that ever ran
// in the estate - an enumeration oracle wearing the shape of a helpfulness
// feature. Section 14 already refuses to let "you may not see shelf" tell a
// caller that shelf exists, and an unfiltered tombstone undoes that in one
// step.
//
// THE ASSERTION IS THAT TWO ANSWERS ARE THE SAME, which is the only form that
// catches it: a caller who could not see the program alive must be told what a
// caller asking about a name nobody ever registered is told, down to the word.
func TestATombstoneDoesNotREVEALAProgramTheCallerCouldNotSee(t *testing.T) {
	k := departedEstate(t)
	stranger := kernel.Principal{
		UID: 1000, Kind: kernel.KindProgram,
		ClientID: "docket", SessionID: "s-docket", PID: 3,
	}

	gone := refusal(t, k, stranger, "shelf")
	never := refusal(t, k, stranger, "never-registered")

	if strings.Contains(gone, "was registered here and left at") {
		t.Fatalf("the tombstone leaked to a caller who could not see it alive:\n%s", gone)
	}
	// Compared with the program name taken out, because the refusal quotes
	// what it was asked for and that is not a leak.
	if a, b := strings.ReplaceAll(gone, "shelf", "X"),
		strings.ReplaceAll(never, "never-registered", "X"); a != b {
		t.Errorf("a caller out of scope can tell the two apart:\n gone: %s\nnever: %s", a, b)
	}
}
