package daemon

import (
	"google.golang.org/protobuf/proto"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// ProbeCommand is the one command name rig owns inside every program's
// namespace, and it is the one call that carries a typed payload rather than
// declared JSON arguments.
//
// `rig ping shelf` sends `shelf.ping` with a PingRequest and proves the round
// trip with a nonce it generated, which is the M0 gate. Everything else a
// program declares travels as CallRequest/CallResponse, validated against the
// declared schema.
//
// THIS FORK IS A DEFECT AND IT IS WRITTEN DOWN RATHER THAN HIDDEN. Two
// reasons it has to go:
//
//  1. PingRequest and CallRequest are wire-compatible - both are one `bytes`
//     field at tag 1 - so a nonce decodes cleanly as `args` and would be
//     validated as JSON. The exemption below is therefore by NAME, because
//     the bytes cannot tell rig which message it is holding.
//  2. A reserved command id inside a program's own namespace collides with
//     what the program may declare. `ping` is rig's liveness probe, so it
//     belongs on rig's own method - `rig.ping` with the program as an
//     argument - not on the program's.
//
// Changing it moves the M0 gate's method name, so it is a deliberate change
// with its own commit, not a tidy-up.
const ProbeCommand = "ping"

// callArgs reads the JSON arguments out of a request frame.
//
// A frame whose payload is not a CallRequest is a caller error, not a
// daemon error: the arguments come back empty and validation refuses it
// against the declared schema, which produces a message naming the command
// rather than a proto complaint naming a field number.
func callArgs(f *rigv1.Frame) []byte {
	var req rigv1.CallRequest
	if err := proto.Unmarshal(f.GetPayload(), &req); err != nil {
		return nil
	}
	return req.GetArgs()
}

// validateArgs is the other half of the boundary: what the caller sent has to
// match what the program declared (PLAN.md sections 5e, 9).
//
// It runs AFTER authorisation, deliberately. A caller a house rule refuses
// should not learn whether its arguments would have been accepted, and the
// authorization floor is not conditional on the call being well-formed.
func (d *Daemon) validateArgs(from *conn, f *rigv1.Frame, program, command string) bool {
	if command == ProbeCommand {
		return true
	}
	if err := d.kernel.ValidateArgs(program, command, callArgs(f)); err != nil {
		from.fail(f.GetStreamId(), rigv1.Code_CODE_INVALID, err.Error())
		d.log.Debug("arguments refused at the boundary",
			"method", f.GetMethod(), "err", err)
		return false
	}
	return true
}
