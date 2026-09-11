package daemon

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"github.com/boris-milner/rig/internal/kernel"
	"github.com/boris-milner/rig/internal/meta"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// The daemon is what runs a command for the four meta tools. Asserted here so
// the interface and the implementation cannot drift apart silently: the
// signature that matters is the one carrying the principal, and a compile
// error is the only thing that notices it changing.
var _ meta.Invoker = (*Daemon)(nil)

// Invoke runs one declared command as one principal, and it is the whole of
// meta.Invoker.
//
// It goes down Daemon.call, which is the path the wire goes down, so a caller
// arriving through the MCP server meets section 13a's floor rather than a
// second copy of it. That is what call was extracted for: "a new surface
// SPLITS the existing path and reuses its floor; it never grows a second one."
//
// The principal is an argument rather than daemon state because the daemon
// answers every caller. An MCP server holds ONE socket connection for MANY
// callers, so a principal held anywhere but the call itself is a principal
// that can be stale for the caller in front of it.
func (d *Daemon) Invoke(
	ctx context.Context, who kernel.Principal, program, command string, args []byte,
) ([]byte, error) {
	payload, err := proto.Marshal(&rigv1.CallRequest{Args: args})
	if err != nil {
		return nil, fmt.Errorf(
			"daemon: could not encode arguments for %s.%s: %w", program, command, err)
	}

	reply, bad := d.call(ctx, caller{
		who:    who,
		method: program + "." + command,
		args:   payload,
		// requestID is deliberately empty. Section 4's dedup window keys on a
		// CLIENT-generated id that is stable across a retry, and an in-process
		// caller has none to offer. Minting one here would produce an id that
		// is unique per attempt, which is the opposite of what the window
		// needs: it would make every retry look like a new call.
	}, program, command)
	if bad != nil {
		return nil, bad.error()
	}

	// A program's own refusal is not rig's, and it arrives as an ERROR frame
	// rather than as a failure of the call. It still has to reach the agent
	// with section 9's structure intact, so it is rebuilt rather than
	// flattened to a sentence.
	if reply.GetKind() == rigv1.FrameKind_FRAME_KIND_ERROR {
		return nil, statusError(program, command, reply.GetStatus())
	}

	var res rigv1.CallResponse
	if err := proto.Unmarshal(reply.GetPayload(), &res); err != nil {
		return nil, fmt.Errorf(
			"daemon: %s.%s answered with something that is not a CallResponse: %w",
			program, command, err)
	}
	return res.GetResult(), nil
}

// statusError turns a Status back into an error, keeping section 9's four
// fields so an in-process caller is told what the wire would have told it.
//
// Section 35: acceptance is not a contract, retrievability is. A refusal that
// reaches one surface complete and another as a bare sentence is the same
// defect one level down.
func statusError(program, command string, st *rigv1.Status) error {
	if st == nil {
		return fmt.Errorf("%s.%s failed and said nothing about why", program, command)
	}
	msg := st.GetMessage()
	if msg == "" {
		msg = fmt.Sprintf("%s.%s failed with %s", program, command, st.GetCode())
	}
	return &kernel.RefusalError{
		Err:          errors.New(msg),
		Precondition: st.GetPrecondition(),
		Actual:       st.GetActual(),
		Fix:          st.GetFix(),
		FixCommand:   st.GetFixCommand(),
	}
}
