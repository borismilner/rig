package daemon

import (
	"github.com/boris-milner/rig/internal/kernel"
	rigv1 "github.com/boris-milner/rig/proto/rig/v1"
)

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
			readOnly("estate", "Estate",
				"Say which estate this is",
				"Answers the estate's name and role, this daemon's build, the wire major and rig's own semantic generation. The one rig method whose answer does not depend on who is asking.",
				"The estate name and role, the daemon build, the wire major and rig's own semantic generation."),
			readOnly("programs", "Programs",
				"List what this principal may reach",
				"Reads the registry through the calling principal's own view.",
				"Every program this principal may see."),

			// READ-ONLY, AND THE WORD IS DOING WORK RATHER THAN BEING
			// GENEROUS. The effects ladder is about what a call does OUTSIDE
			// rig - files, network, destruction, driving input - and minting
			// is none of them. It is also read-only in the literal sense
			// here: the token is minted at ACCEPT, for every connection, so
			// this method reads the one the caller already has rather than
			// creating anything.
			//
			// IDEMPOTENT FOR THE SAME REASON, and the house argument for it
			// is already in this file: `down` is idempotent because
			// "idempotence is about the state left behind, not about whether
			// the reply is identical". Asking twice returns the same token
			// and leaves the same state; a resume that succeeds leaves the
			// same state as the resume before it.
			// PRESENCE, AND ALL THREE ARE READ-ONLY BY THE LADDER'S OWN
			// DEFINITION rather than by generosity. The effects ladder is
			// about what a call does OUTSIDE rig - files, network,
			// destruction, driving input - and a roster entry is none of
			// them: it touches no file, opens no socket and dies with the
			// connection that made it. This is the identical argument
			// `session` writes down two entries below, and the two must not
			// drift: if a level for "mutates rig's own state" is ever added,
			// both move together or the ladder has two meanings.
			//
			// IDEMPOTENT ON THE HOUSE DEFINITION - "about the state left
			// behind, not about whether the reply is identical". Announcing
			// twice with the same purpose leaves one row saying that purpose,
			// and re-announcing into a seat this connection already holds
			// deliberately does not move the generation.
			readOnly("announce", "Announce",
				"Say what this session is for, and see who else is here",
				"Records this connection's purpose and activity on the estate's roster, optionally taking a named SEAT, and returns the crew as it stands. A seat is a role that outlives its occupants; each occupancy gets a generation, so a peer can tell whether the session it is addressing is still the one it meant. Refused if the seat is held by a live peer.",
				"This peer's own row including its generation, the whole crew, and whether the roster is known to be incomplete."),
			readOnly("activity", "Activity",
				"Say what this session is doing right now",
				"Replaces this connection's activity line, and optionally moves its seat to HANDING_OFF. Cheap and non-blocking. Requires an earlier announce: an activity with no purpose above it is the unsupervisable row the seat mechanism exists to prevent.",
				"This peer's updated row."),
			readOnly("peers", "Peers",
				"Read the estate's roster",
				"Who is here, what each is for, what each is doing, which seat each holds and at which generation. Presence is connection state, so a peer that is listed is a peer whose connection is open.",
				"The crew, and whether the roster is known to be incomplete."),

			readOnly("session", "Session",
				"Say which session this connection carries",
				"Answers this connection's session token, or resumes a session named by one. Section 5f says every connection carries a token that survives reconnect; a terminal, an agent and a script never handshake, so this is the message on which they receive it.",
				"The session token, and whether a resume was honoured."),

			// SECTION 39's RECORD VERBS.
			//
			// ⛔ EVERY ONE OF THEM MUST BE DECLARED, AND THE REASON IS IN
			// DeclareSelf's own comment: without a declaration rig's methods
			// are the one surface that never reaches the rules table, and an
			// unresolvable ref resolves to EffectsCeiling - so ANY RULE AT
			// ALL would stop rig answering. An undeclared verb is not a
			// missing row, it is rig refusing itself.
			//
			// `record.refs` IS HERE FROM 2026-09-16 LATE. It was held out while
			// its reverse lookup was unbuilt - declaring it then would have
			// been the wire-that-lies this section still refuses for
			// standard.stamp and project.gate - and it landed with the slice
			// that serves it, exactly as that rule intends.
			readOnly("record.get", "Record get",
				"Read one record, at head or at a version",
				"Answers one record. Version 0 means HEAD rather than version zero, which no record has - the first write is version 1.",
				"The record at the version asked for."),
			readOnly("record.query", "Record query",
				"Find records by kind in a project",
				"Answers the HEAD of every record of a kind in a project. This is section 39's \"indexed\".",
				"Every matching record at its current version."),
			readOnly("record.history", "Record history",
				"Read every version of one record",
				"Answers every version of one record, oldest first, each with the provenance of the write that made it.",
				"One record's versions, oldest first."),
			readOnly("record.refs", "Record refs",
				"What points AT this record",
				"Walks the reverse edges and answers what cites, blocks or is part of this record - section 39's \"correlated\", and the direction files cannot go. The walk is bounded by depth and says which depth it ANSWERED, flags a partial answer rather than returning a short list that looks complete, and reports any cycle it crossed by name without resolving it. It stays inside the record's own project unless asked to cross, which is a performance property rather than a preference.",
				"What points at the record, each edge naming its kind, title and the record it arrived through."),
			readOnly("project.brief", "Project brief",
				"The derived answer to what is going on here",
				"Derives next-up work in execution order, what is blocked and on whom, any blocks cycle by name, and the must-read set for this session. Nothing here is stored and no seat writes prose: the whole answer is computed at read time. Asking for it MARKS the must-read set delivered to this session, which is the one side effect on this surface and is per-session state rather than anything written to the record.",
				"The project's derived state, in the view the caller asked for."),

			// THE FOUR WRITERS, AND THEY ARE NOT readOnly.
			//
			// The effects ladder is about what a call does OUTSIDE rig, and
			// the test self.go already states for presence is three clauses:
			// touches no file, opens no socket, dies with the connection. A
			// record write FAILS the first and the third - it is SQLite on
			// disk and it outlives the process - so read-only would be a
			// false declaration rather than a generous one.
			//
			// ⛔ AND NO NEW LADDER LEVEL IS NEEDED, WHICH IS WHY `announce`
			// AND `session` DO NOT MOVE. The "mutates rig's own state" level
			// predicted above would be for something that mutates WITHOUT
			// touching a file - that is the roster, and it is already
			// covered. The record store IS a file, so an existing rung is
			// honest.
			//
			// Idempotent is picked PER VERB. One value carrying all four was
			// the mistake this comment exists to stop.
			{
				ID: "record.put", Title: "Record put",
				Effects: kernel.EffectsWritesFiles,

				// NOT idempotent: an empty id MINTS one, so two identical
				// calls leave TWO records. The compare-and-swap makes a
				// retry SAFE, which is a different property from leaving
				// the same state behind.
				Idempotent: kernel.No,

				Sensitive:    []string{},
				Interactive:  kernel.No,
				Streams:      kernel.No,
				NeedsDisplay: kernel.No,
				Duration:     kernel.DurationInstant,
				Confirms:     kernel.No,
				Shape:        kernel.ShapeUnary,
				Summary:      "Create a record, or supersede one",
				Description:  "Writes a record and returns the version written. The caller may name the version it believes is current, and a losing compare-and-swap is refused as a conflict rather than overwriting. Provenance - who wrote it, from which seat, at which epoch - is the daemon's and is never read off the request.",
				Returns:      "The record as written, at its new version.",
			},
			{
				ID: "progress.step", Title: "Progress step",
				Effects: kernel.EffectsWritesFiles,

				// NOT idempotent: it APPENDS. Two calls leave two steps,
				// and section 39 makes the stream the state, so collapsing
				// them would change what the item's live state IS.
				Idempotent: kernel.No,

				Sensitive:    []string{},
				Interactive:  kernel.No,
				Streams:      kernel.No,
				NeedsDisplay: kernel.No,
				Duration:     kernel.DurationInstant,
				Confirms:     kernel.No,
				Shape:        kernel.ShapeUnary,
				Summary:      "Append one step to a work item's stream",
				Description:  "Records that a work item was started, blocked or finished, written at a boundary DURING the work rather than at the end. An absent stream reads as UNKNOWN, never as nothing happened.",
				Returns:      "The step as written.",
			},
			{
				ID: "record.link", Title: "Record link",
				Effects: kernel.EffectsWritesFiles,

				// Idempotent on the house definition - the state left
				// behind is the same however many times it is asked.
				Idempotent: kernel.Yes,

				Sensitive:    []string{},
				Interactive:  kernel.No,
				Streams:      kernel.No,
				NeedsDisplay: kernel.No,
				Duration:     kernel.DurationInstant,
				Confirms:     kernel.No,
				Shape:        kernel.ShapeUnary,
				Summary:      "Write a typed edge between two records",
				Description:  "Asserts a typed, directed edge. The type set is closed at section 39's eight names and an unknown one is refused by name with the value quoted back.",
				Returns:      "Nothing. The edge either exists afterwards or the call was refused.",
			},
			{
				ID: "record.unlink", Title: "Record unlink",
				Effects:    kernel.EffectsWritesFiles,
				Idempotent: kernel.Yes,

				Sensitive:    []string{},
				Interactive:  kernel.No,
				Streams:      kernel.No,
				NeedsDisplay: kernel.No,
				Duration:     kernel.DurationInstant,
				Confirms:     kernel.No,
				Shape:        kernel.ShapeUnary,
				Summary:      "Remove a typed edge between two records",
				Description:  "Removes an edge. Removing one that is not there leaves the same state behind, which is why this is idempotent.",
				Returns:      "Nothing. The edge is absent afterwards either way.",
			},

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

// estateRole derives an estate's role from its name, AT REPLY TIME, and it is
// the only place that derivation exists (PLAN.md section 37).
//
// THE DERIVATION LIVES HERE RATHER THAN IN THE CALLER, and that is the whole
// argument for carrying both fields on the wire. With only a name, every
// consumer would compute `name == "production"` for itself - every agent, every
// script, outside this repository and uncountable - and reopening the name set
// would break all of them silently. Here it is computed once and reopening the
// set breaks nothing. Because the role is never stored and never accepted as
// input, there is no second place for the two to disagree from.
//
// An unrecognised non-empty name cannot happen today: cmd/rigd refuses anything
// outside the closed set before the daemon is built. It is answered UNSPECIFIED
// rather than guessed at, because "nothing was said" is the honest answer to a
// name this function does not understand, and inventing a role for it would be
// the guess that section 21's zero exists to prevent.
func estateRole(name string) rigv1.EstateRole {
	switch name {
	case "":
		return rigv1.EstateRole_ESTATE_ROLE_UNNAMED
	case "production":
		return rigv1.EstateRole_ESTATE_ROLE_PRODUCTION
	case "development":
		return rigv1.EstateRole_ESTATE_ROLE_DEVELOPMENT
	default:
		return rigv1.EstateRole_ESTATE_ROLE_UNSPECIFIED
	}
}
