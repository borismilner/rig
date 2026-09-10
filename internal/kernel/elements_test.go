package kernel_test

import (
	"strings"
	"testing"

	"github.com/boris-milner/rig/internal/kernel"
)

// R7: using an element rig does not serve fails at REGISTRATION.
//
// The alternative fails at render, in front of the user, on the one surface
// whose entire product is presentation. This is the test that keeps R7 from
// being enforced by review, which is how R1-R8 were enforced until 2026-09-10.
func TestAnElementRigDoesNotServeRefusesTheRegistration(t *testing.T) {
	d := good("shelf")
	d.Elements = []string{"rigTable", "rigSparkline"}

	err := d.Validate()
	if err == nil {
		t.Fatal("a declaration naming an element rig does not serve was accepted")
	}
	if !strings.Contains(err.Error(), "rigSparkline") {
		t.Fatalf("the refusal does not name the offending element: %v", err)
	}
	// A program author fixing a generated declaration one error per run is a
	// program author who stops generating it, so the refusal names the set.
	for _, served := range kernel.KitElements() {
		if !strings.Contains(err.Error(), served) {
			t.Errorf("the refusal does not name %q, so an author cannot fix it in one run: %v",
				served, err)
		}
	}
}

// The three names Boris approved on 2026-09-11, and the list is what a
// declaration may draw from.
func TestTheServedElementsAreTheApprovedThree(t *testing.T) {
	got := kernel.KitElements()
	want := []string{"rigPanel", "rigTable", "rigToolbar"}
	if len(got) != len(want) {
		t.Fatalf("the served set is %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("the served set is %v, want %v", got, want)
		}
	}

	d := good("shelf")
	d.Elements = want
	if err := d.Validate(); err != nil {
		t.Fatalf("a declaration naming every served element was refused: %v", err)
	}
}

// A program that serves no page of its own declares no elements, and that is
// not an incomplete declaration. R3's field is what a program uses, and using
// none is a real answer - unlike coverage or semantics_gen, where section 5e
// refuses silence because an absence there would carry a permissive meaning.
func TestDeclaringNoElementsIsFine(t *testing.T) {
	d := good("snapper")
	d.Elements = nil
	if err := d.Validate(); err != nil {
		t.Fatalf("a program that serves no page was refused: %v", err)
	}
}

// An empty string is not a name rig serves, so it is refused like any other.
// A generated declaration with a hole in it fails at registration rather than
// reaching a page.
func TestAnEmptyElementNameIsRefused(t *testing.T) {
	d := good("shelf")
	d.Elements = []string{""}
	if err := d.Validate(); err == nil {
		t.Fatal("an empty element name was accepted")
	}
}
