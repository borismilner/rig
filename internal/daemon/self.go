package daemon

import "github.com/boris-milner/rig/internal/kernel"

// selfDeclaration is rig declaring its own commands the way every program
// does, so the invoker has declared effects to match on instead of treating
// rig's own methods as unresolvable.
//
// rig.hello is NOT here, and that is not an omission. hello is the handshake
// that MINTS the principal; there is no (caller, effects) pair to match
// before it, because `caller` is what hello establishes. It is prior to the
// floor rather than exempt from it.
func selfDeclaration() kernel.Declaration {
	readOnly := func(id, title, summary, description, returns string) kernel.Command {
		return kernel.Command{
			ID: id, Title: title,
			Effects:      kernel.EffectsReadOnly,
			Idempotent:   kernel.Yes,
			Sensitive:    []string{},
			Interactive:  kernel.No,
			Streams:      kernel.No,
			NeedsDisplay: kernel.No,
			Duration:     kernel.DurationInstant,
			Confirms:     kernel.No,
			Shape:        kernel.ShapeUnary,
			Summary:      summary,
			Description:  description,
			Returns:      returns,
		}
	}
	return kernel.Declaration{
		Identity:     kernel.Identity{ID: kernel.SelfID, Name: kernel.SelfID, Version: "0"},
		Coverage:     kernel.CoverageFull,
		SemanticsGen: 1,
		Commands: []kernel.Command{
			readOnly("ping", "Ping",
				"Round-trip a nonce",
				"Echoes the nonce it was given, and probes a named program.",
				"The nonce, the program id and its version."),
			readOnly("programs", "Programs",
				"List what this principal may reach",
				"Reads the registry through the calling principal's own view.",
				"Every program this principal may see."),

			// The first thing rig declares about itself that is not read-only,
			// and the properties are the point rather than paperwork: this is
			// the declaration a house rule matches on, so getting `effects`
			// wrong here is what would let a rule denying destructive calls
			// fail to catch the most destructive call rig has.
			{
				ID: "down", Title: "Down",
				Effects: kernel.EffectsDestructive,

				// Idempotent because the ESTATE ends up stopped either way.
				// The second call gets no answer - there is nothing left to
				// answer it - but idempotence is about the state left behind,
				// not about whether the reply is identical, and the CLI is
				// built to treat "already stopped" as success for exactly
				// this reason.
				Idempotent: kernel.Yes,

				Sensitive:   []string{},
				Interactive: kernel.No,
				Streams:     kernel.No,

				NeedsDisplay: kernel.No,

				// Instant: the daemon replies, then stops. What follows the
				// reply is closing connections already known to be
				// interruptible (section 18 gives every call a deadline and
				// the stub survives rig going away), not work the caller
				// waits on.
				Duration: kernel.DurationInstant,

				// No. There IS no surface to confirm through at M1 - Asker is
				// nil, so a confirm is denied as an unanswered gating question
				// - and declaring Yes here would make `rig down` undeniable-by
				// -accident: the honest reading of Confirms is "this command
				// asks the caller first", and it does not. The protection is
				// the destructive effects level above, which a house rule can
				// act on today.
				Confirms: kernel.No,

				Shape:       kernel.ShapeUnary,
				Summary:     "Stop the daemon",
				Description: "Stops this rigd, the one serving the socket in this XDG_RUNTIME_DIR. It is scoped by that directory and needs no estate name.",
				Returns:     "The pid and version of the daemon that agreed to stop.",
			},
		},
	}
}

// declareSelf hands rig's own declaration to the kernel at startup, so the
// invoker resolves rig.ping and rig.programs to declared effects instead of
// leaving them unresolvable.
func declareSelf(k *kernel.Kernel) error { return k.DeclareSelf(selfDeclaration()) }
