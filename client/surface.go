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
// What is deliberately NOT here, because section 5d forbids the stub from
// carrying semantics: no retry, no backoff, no queue, no schema knowledge,
// no interpretation of what a method means. The stub frames bytes and waits.

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
