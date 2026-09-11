package daemon

import (
	"google.golang.org/protobuf/proto"

	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

// ProbeCommand is the command name rig uses to probe a program's liveness,
// and it is the one call that carries a typed payload rather than declared
// JSON arguments.
//
// `rig ping shelf` sends `rig.ping` with a PingRequest naming shelf, and the
// daemon probes shelf as `shelf.ping` on its own connection. Everything else
// a program declares travels as CallRequest/CallResponse, validated against
// the declared schema.
//
// THE FORK IS HALF CLOSED, and which half matters:
//
//  1. CLOSED. The probe is no longer a reserved command id inside every
//     program's namespace on the wire the CALLER speaks. It is rig's own
//     method with the program as an argument, so nothing reading a command
//     list has to tell a declared command from rig's probe. That was the half
//     the window could not draw.
//  2. STILL OPEN, and it is why the exemption below is by NAME. PingRequest
//     and CallRequest are wire-compatible - both are one `bytes` field at
//     tag 1 - so on the daemon-to-program hop a nonce still decodes cleanly
//     as `args` and would be validated as JSON. The bytes cannot tell rig
//     which message it is holding, and no amount of moving the method name
//     fixes that. Closing it properly means the probe carrying a payload the
//     schema path cannot mistake for arguments.
//
// The programs still ANSWER `<program>.ping`, and a caller sending that form
// still routes, so an older rig keeps working.
const ProbeCommand = "ping"

// callArgs reads the JSON arguments out of a request frame.
//
// A frame whose payload is not a CallRequest is a caller error, not a
// daemon error: the arguments come back empty and validation refuses it
// against the declared schema, which produces a message naming the command
// rather than a proto complaint naming a field number.
func callArgs(payload []byte) []byte {
	var req rigv1.CallRequest
	if err := proto.Unmarshal(payload, &req); err != nil {
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
//
// It RETURNS the refusal rather than writing it, because the refusal has two
// destinations now: a frame on the wire path, and an error on the in-process
// one. The kernel's error carries section 9's structure either way - which
// surface renders it is the surface's business, not the boundary's.
func (d *Daemon) validateArgs(from caller, program, command string) error {
	if command == ProbeCommand {
		return nil
	}
	if err := d.kernel.ValidateArgs(program, command, callArgs(from.args)); err != nil {
		d.log.Debug("arguments refused at the boundary",
			"method", from.method, "err", err)
		return err
	}
	return nil
}
