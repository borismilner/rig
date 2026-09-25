package meta

import (
	"context"
	"encoding/json"

	"github.com/borismilner/rig/internal/kernel"
)

// VerbTool routes one of rig's own verbs that the agent door carries without a
// hand-written tool of its own (plan/09, "The MCP door covers everything rig
// does"). The tool an agent sees is named for the verb; this is the route
// under it.
const VerbTool Tool = "verb"

// Verb is one of rig's own verbs as the agent door offers it.
//
// Input is a JSON Schema for the arguments, derived from the wire's request
// message so the tool and the wire cannot describe two different shapes.
type Verb struct {
	Tool        string
	Command     string
	Title       string
	Description string
	Effects     kernel.Effects
	Idempotent  kernel.Tristate
	Input       json.RawMessage
}

// Verbs is the optional half of Invoker that runs rig's own verbs for the
// connection it belongs to.
//
// OPTIONAL AND RESOLVED PER CALL, Mailbox's shape: a surface with no daemon
// under it has no verbs to run and offers no tools for them.
//
// NO METHOD TAKES A PRINCIPAL OR A SEAT, for Roster's reason: the
// implementation is per connection, and the verb runs as that connection's
// principal and seat, through the same authorisation a terminal meets.
type Verbs interface {
	Verbs() []Verb

	// CallVerb runs one verb with its arguments as JSON and answers the
	// wire's reply as JSON.
	CallVerb(ctx context.Context, command string, args []byte) ([]byte, error)
}

// Verbs lists the verbs this surface carries, empty with no daemon under it.
func (s *Server) Verbs() []Verb {
	if v, ok := s.invoker.(Verbs); ok {
		return v.Verbs()
	}
	return nil
}

// verb answers one bridged verb. The reply rides on Result, invoke's field,
// because it is the same thing: what running a command returned.
func (s *Server) verb(ctx context.Context, who kernel.Principal, r Request) (Answer, error) {
	out, err := s.rosterAnswer(who, r.Tool)
	if err != nil {
		return Answer{}, err
	}
	v, ok := s.invoker.(Verbs)
	if !ok {
		out.Unavailable = append(out.Unavailable, "rig's own verbs")
		return out, nil
	}
	res, err := v.CallVerb(ctx, r.Command, r.Args)
	if err != nil {
		return Answer{}, err
	}
	out.Result = res
	return out, nil
}
