package client

// The stub's public surface, enumerated (PLAN.md section 3).
//
// Every symbol here is a future rebuild that cannot be avoided: a program
// links this package, and anything exported from it is a promise to every
// program that ever links it. So the set is written down, asserted by
// surface_test.go, and does not grow without a decision recorded in the plan.
//
//	Connect() (*Client, error)          dial rig at its well-known socket
//	Dial(socket string) (*Client, error)  dial a known path
//	Client                              one connection
//	  Call(ctx, method, in, out) error  one request, one answer
//	  Hello(ctx, program, version)      the program handshake
//	  Handle(Handler)                   answer what rig routes here
//	  Done() <-chan struct{}            closed when the connection ends
//	  Err() error                       why it ended
//	  Close() error
//	Handler                             the callback Handle takes
//	CallError                           a wire-level failure
//	  Error() string
//	  Code() rigv1.Code
//
// What is deliberately NOT here: no schema knowledge, and no interpretation
// of what a method means. The stub frames bytes; it does not know what a
// notification looks like, what config keys mean, how a UI is described or
// what commands exist. Those four are section 5d's own list.
//
// CORRECTED 2026-09-11, and the previous wording was inverted. It read "no
// retry, no backoff, no queue, no schema knowledge", citing section 5d as
// forbidding all four. Section 5d requires three of them. Its five duties
// include "3. Reconnect when rig restarts, presenting the session token it
// already holds" and "4. Tolerate absence: a bounded outbound queue, backoff
// to a deadline, and one typed unavailable error (section 5g)" - so the
// sentence claimed the plan barred what the plan asks for.
//
// The line section 5d draws is POLICY, not transport behaviour. Its "carries
// no semantics" is scoped by the sentence immediately after it, quoted above.
// Section 5g settles it in its own words: the tolerant client is "mechanism
// only: no layering, no merging, no answers", and the resolved-snapshot reader
// is "about fifteen lines in the stub". What section 5g deleted was a 300-line
// fallback that re-implemented config layering - "that is policy, in the one
// component section 5d requires to be semantics-free".
//
// The correction is recorded rather than made quietly because this comment was
// load-bearing: it was read as the specification by later work, and the
// tolerant client was scoped as needing "a home that does not exist yet" on
// its authority. The home is this package, and always was.

// Surface is the enumeration above, in a form a test can check.
//
// Ordered, so a diff on this slice is the decision section 3 asks to be
// recorded rather than a set that quietly re-sorts.
var Surface = []string{
	"CallError",
	"CallError.Code",
	"CallError.Error",
	"Client",
	"Client.Call",
	"Client.Close",
	"Client.Done",
	"Client.Err",
	"Client.Handle",
	"Client.Hello",
	"Connect",
	"Dial",
	"Handler",
}
