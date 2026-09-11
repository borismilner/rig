package kernel

import (
	"errors"
	"fmt"
	"strings"
)

// RefusalError is a refusal that can say more than a sentence.
//
// PLAN.md section 9: "A failed call never returns prose. It returns a
// structured error: what failed, which precondition, what the actual state
// was, and what would fix it - including, where it exists, the exact command
// that fixes it." Section 10 then requires --json to return exactly what the
// MCP tool returns, so this is not a CLI nicety - it is the one error object
// every M2 surface renders.
//
// It is an ordinary error first. Everything that already handles a kernel
// error keeps working, errors.As reaches the structure when a caller wants
// it, and a refusal that cannot fill the fields is still a valid refusal.
//
// EVERY FIELD BUT THE MESSAGE IS OPTIONAL, and that is the whole discipline.
// The value of Precondition is that it is TRUE when set. A refusal that
// cannot name the condition it checked leaves it empty; an invented one is
// worse than a blank, because a surface renders it with the same authority
// as a measured one and an agent acts on it.
type RefusalError struct {
	// The sentence, and the only field always present. Wrapped rather than
	// copied so errors.Is and %w keep working through a RefusalError.
	Err error

	// The condition that had to hold, stated positively - "since matches
	// ^[0-9]+[dhm]$", not "since was wrong".
	Precondition string

	// What was found instead. Positively too, and specific: the value, the
	// missing path, the count.
	Actual string

	// What would fix it, in words, for a human reading a terminal.
	Fix string

	// Runnable as written, or empty. NEVER prose - Fix is where the sentence
	// goes. A surface may put this behind a button or run it after a confirm,
	// so anything here that is not a real command is a defect and not a hint.
	FixCommand string
}

func (f *RefusalError) Error() string { return f.Err.Error() }

func (f *RefusalError) Unwrap() error { return f.Err }

// refusalf builds a RefusalError whose message is formatted, so a call site reads like
// the fmt.Errorf it replaces and the extra fields are added deliberately.
func refusalf(format string, a ...any) *RefusalError {
	return &RefusalError{Err: fmt.Errorf(format, a...)}
}

// with returns the RefusalError with its structured fields set. Separate from refusalf
// so the message and the structure are visibly two decisions: a refusal that
// can only manage the sentence simply does not call this.
func (f *RefusalError) with(precondition, actual, fix, fixCommand string) *RefusalError {
	f.Precondition = precondition
	f.Actual = actual
	f.Fix = fix
	f.FixCommand = fixCommand
	return f
}

// AsRefusal reaches the RefusalError inside an error chain, reporting whether there was
// one. Surfaces call this; nothing in the kernel needs it.
func AsRefusal(err error) (*RefusalError, bool) {
	return errors.AsType[*RefusalError](err)
}

// pointer renders a JSON pointer path the way section 9's worked example
// writes it - "shelf.index.path", not "/shelf/index/path" - because the
// example an agent is told to act on is dotted and the two must not drift.
// An empty path is the document itself and says so.
func pointer(parts []string) string {
	if len(parts) == 0 {
		return "the arguments"
	}
	return strings.Join(parts, ".")
}
