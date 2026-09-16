package client

// The stub's public surface, enumerated (PLAN.md section 3).
//
// Every symbol here is a future rebuild that cannot be avoided: a program
// links this package, and anything exported from it is a promise to every
// program that ever links it. So the set is written down, asserted by
// surface_test.go, and does not grow without a decision recorded in the plan.
//
// SHAPES, NOT NAMES, since 2026-09-16. The list below carried bare names
// until then, which locked the NAME Client.Call while leaving its signature
// free: changing what it takes or returns broke every program that links the
// stub AND passed the gate, because the name was still there. It also listed
// no struct fields at all, while cmd/rig builds a CallError by naming Method
// and Status, so retyping either was a breaking change nothing checked. Both
// are enumerated with their shape now. PARAMETER NAMES ARE NOT PART OF IT:
// Go has no named arguments, so renaming a parameter breaks nobody, and
// locking one would spend a recorded decision on an edit that costs nothing.
//
// THE LIST BELOW IS THE ONLY COPY. This comment used to carry a prose version
// beside it, and the prose rotted: it described Hello as taking a program and
// a version long after Hello took a declaration and returned a response. Two
// copies of a contract means one of them is wrong with nothing to say which,
// and the copy a test can check is the one that survives.
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
	"CallError struct",
	"CallError.Code() rigv1.Code",
	"CallError.Error() string",
	"CallError.Method string",
	"CallError.Status *rigv1.Status",
	"Client struct",
	"Client.Call(context.Context, string, proto.Message, proto.Message) error",
	"Client.Close() error",
	"Client.Done() <-chan struct{}",
	"Client.Err() error",
	"Client.Handle(Handler)",
	"Client.Hello(context.Context, *rigv1.Declaration) (*rigv1.HelloResponse, error)",
	"Connect() (*Client, error)",
	"Dial(string) (*Client, error)",
	"Handler func(string, []byte) (proto.Message, error)",
}
