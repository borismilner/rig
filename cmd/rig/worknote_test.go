package main

import (
	"testing"

	"github.com/borismilner/rig/internal/worknote"
)

// ⛔ THE CLIENT MAY NOT IMPORT internal/worknote, BECAUSE THAT WOULD LINK THE
// SQLITE DRIVER INTO cmd/rig: worknote imports internal/record, and section 22
// puts the driver in rigd alone. So worknote.go carries two copies, and this
// test is the only thing that keeps them equal to their originals. A TEST file
// may import the package, which is why the pin can exist at all.
func TestTheClientsWorkNoteConstantsMatchThePackage(t *testing.T) {
	if workNoteStamp != worknote.Stamp {
		t.Errorf("workNoteStamp = %q, worknote.Stamp = %q", workNoteStamp, worknote.Stamp)
	}
	if workNoteMaxLimit != worknote.MaxLimit {
		t.Errorf("workNoteMaxLimit = %d, worknote.MaxLimit = %d", workNoteMaxLimit, worknote.MaxLimit)
	}
}
