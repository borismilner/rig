package daemon

import (
	"github.com/borismilner/rig/internal/kernel"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
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
	leaseWriter := func(id, title string, idempotent kernel.Tristate, summary, description, returns string) kernel.Command {
		c := readOnly(id, title, summary, description, returns)
		c.Effects = kernel.EffectsWritesFiles
		c.Idempotent = idempotent
		return c
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
			readOnly("describe", "Describe",
				"One program or command in full, as the MCP tool answers it",
				"Answers the object the MCP describe tool returns for the same program and command, rendered by the daemon so the CLI's --json and the MCP tool cannot disagree. Reads through the calling principal's own view.",
				"The MCP describe tool's JSON object, as bytes."),

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
			// ⛔ DECLARED BECAUSE IT MOVED, NOT BECAUSE IT WORKS. plan/50
			// decision 4, narrowed 2026-09-24: the derivation left rig for the
			// docket program, and section 21 is a ruling - rig serves every
			// wire version it has ever shipped - so the arm could not vanish
			// from v1. It stays declared and answers a structured refusal
			// naming the command that replaced it; the seven brief messages
			// stay in wire.proto until the next major.
			//
			// ⛔ THE PROSE BELOW IS THE REFUSAL AND NOT THE OLD DERIVATION.
			// Leaving the original sentence here would make this declaration
			// the last place in rig still claiming rig derives a brief, which
			// is the failure the whole move exists to end - and a declaration
			// is the one surface a caller reads BEFORE it calls.
			readOnly("project.brief", "Project brief",
				"Moved to the docket program; this arm refuses and says where",
				"Answers a refusal rather than a brief. The derivation moved to the docket program, which reaches this store over the socket like every other program here; rig keeps the record store and the record verbs the brief is derived on top of. The arm stays on this wire version because rig serves every version it has ever shipped, so a caller that asks is told where the answer went rather than being told there is no such method.",
				"A refusal naming the program that derives the brief, with the command to ask it instead."),

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
				ID: "backup.create", Title: "Backup create",

				// It writes a file and nothing else: the store is read
				// through VACUUM INTO, which does not change a row.
				Effects: kernel.EffectsWritesFiles,

				// NOT idempotent: the name carries a UTC stamp, so two
				// calls leave TWO archives. That is the intended shape -
				// a backup that overwrote the previous one would destroy
				// a backup in the act of taking one.
				Idempotent: kernel.No,

				Sensitive:    []string{},
				Interactive:  kernel.No,
				Streams:      kernel.No,
				NeedsDisplay: kernel.No,

				// Seconds, not instant: VACUUM INTO rewrites the whole
				// database compacted and then gzip runs over the result.
				Duration: kernel.DurationSeconds,

				// No confirmation: it only ever ADDS a file, and the one
				// file it could have destroyed is refused by name.
				Confirms:    kernel.No,
				Shape:       kernel.ShapeUnary,
				Summary:     "Archive this estate's own persistent state",
				Description: "Takes a transactional snapshot of the record store and writes one .tar.gz under the user's state directory, carrying a manifest and the snapshot. The caller names no path: the daemon chooses it from the estate name and the clock, and answers with where it landed. A copy of record.db taken with cp is NOT a backup, because the store runs in WAL mode and a committed write lives in record.db-wal until a checkpoint folds it in.",
				Returns:     "Where the archive landed, its size and SHA-256, and the schema version, record count and head count of the snapshot inside it.",
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

			// ⛔ B77's THREE, AND THEIR EFFECTS DIFFER FROM EACH OTHER, which
			// is the whole reason they are declared separately. A house rule
			// denying destructive calls must catch `record.delete` and must NOT
			// catch `record.retract` - a retraction destroys nothing - and a
			// single declaration covering all three would have forced one
			// answer onto two different facts. Same mistake this file already
			// records about idempotence being picked per verb.
			{
				ID: "record.retract", Title: "Record retract",
				Effects: kernel.EffectsWritesFiles,

				// Idempotent: the record ends up withdrawn either way. The
				// SECOND call keeps the first withdrawal's provenance and says
				// so, which is about what the answer carries rather than about
				// the state left behind.
				Idempotent: kernel.Yes,

				Sensitive:    []string{},
				Interactive:  kernel.No,
				Streams:      kernel.No,
				NeedsDisplay: kernel.No,
				Duration:     kernel.DurationInstant,
				Confirms:     kernel.No,
				Shape:        kernel.ShapeUnary,
				Summary:      "Withdraw a record, keeping its id and its history",
				Description:  "Marks a record withdrawn. It leaves every brief and every query; its id and every version of it survive, and record.get still answers, saying that it was retracted and why. Not a delete: nothing is destroyed. Anyone may retract any record.",
				Returns:      "The withdrawal - reason, provenance, and whether the record was already withdrawn by somebody else.",
			},
			{
				ID: "record.delete", Title: "Record delete",

				// ⛔ DESTRUCTIVE, AND IT IS THE SECOND THING rig DECLARES THAT
				// IS. It removes a record, every version of it, and every edge
				// touching it, and Boris ruled the edges are dropped rather
				// than the delete refused. Declaring this as a plain write
				// would let a rule denying destructive calls pass the only verb
				// in the record store that loses data.
				Effects: kernel.EffectsDestructive,

				// Idempotent about the state left behind: the record is absent
				// either way. A second call is a NotFound refusal, which is a
				// property of the answer rather than of the state.
				Idempotent: kernel.Yes,

				Sensitive:    []string{},
				Interactive:  kernel.No,
				Streams:      kernel.No,
				NeedsDisplay: kernel.No,
				Duration:     kernel.DurationInstant,
				Confirms:     kernel.No,
				Shape:        kernel.ShapeUnary,
				Summary:      "Remove a record, its versions and every edge touching it",
				Description:  "Destroys a record and its whole history, and drops every edge at either end of it - ruled by Boris over refusing while anything cites it. The answer names every edge dropped and counts the versions removed, and a dry run shows the same account without writing. Anyone may delete any record.",
				Returns:      "What it took: the version count and every dropped edge, named.",
			},
			{
				ID: "record.replace", Title: "Record replace",
				Effects: kernel.EffectsWritesFiles,

				// NOT idempotent: the first call moves the inbound edges and
				// withdraws the loser; a second finds the loser already
				// withdrawn and no edges left to move, so it leaves the same
				// state but the two calls did different things. The honest
				// answer for a verb whose report is its output.
				Idempotent: kernel.No,

				Sensitive:    []string{},
				Interactive:  kernel.No,
				Streams:      kernel.No,
				NeedsDisplay: kernel.No,
				Duration:     kernel.DurationInstant,
				Confirms:     kernel.No,
				Shape:        kernel.ShapeUnary,
				Summary:      "Put a different record in the place of an existing one",
				Description:  "The duplicate case: two ids hold one fact. Every edge pointing at the loser is moved onto the survivor, the loser is withdrawn pointing at it, and record.get on the old id still says where the fact went. A supersede cannot express this, because a supersede keeps the id.",
				Returns:      "Every inbound edge in one of three buckets - moved, merged into an edge the survivor already had, or dropped as a would-be self-edge - and the withdrawal.",
			},

			// SECTION 16's LEASES. The list is a read; the four writers keep
			// their state in the estate's bbolt file and it outlives the
			// process, so they are file writes on the same argument the
			// record writers make above. None is destructive: a break
			// refuses a lease still inside its deadline, so it only ever
			// frees one whose holder has already let it lapse.
			// SECTION 40's KNOWLEDGE SECTION. Search and get read; add writes a
			// lesson into the estate's store, a file, so it is a file write on
			// the record writers' argument.
			readOnly("knowledge.search", "Knowledge search",
				"Find lessons other sessions already learned",
				"Searches the estate's lessons by the words given and answers, best first, a title, a one-line summary and a snippet per hit - never a body, so consulting costs a few hundred bytes. No query syntax is interpreted. Consult it before a deep dive: another session may have done the research.",
				"Up to 20 hits, best first, each with an id to fetch."),
			readOnly("knowledge.get", "Knowledge get",
				"Read one lesson whole",
				"Answers one lesson by id, body and provenance included. Fetch only the hit that fits.",
				"The lesson."),
			leaseWriter("knowledge.add", "Knowledge add", kernel.No,
				"Write a lesson once, for every agent and person on this estate",
				"Writes a lesson: a title, a one-line summary that searches show, a body with the detail, and one-word tags. For lessons of great importance to many users, not for every note. Attributed to the caller's seat.",
				"The lesson as written, with its id."),

			readOnly("lease.list", "Lease list",
				"Every lease in the estate, with its owner's liveness",
				"Answers every lease this estate knows about, evaluated now: held, orphaned or free, who holds or last held it, how it is witnessed, whether the witness was observed dead, and whether it needs a recorded break. Expiry is derived on read, never swept.",
				"Every lease, by name."),
			leaseWriter("lease.acquire", "Lease acquire", kernel.No,
				"Take a named lease for a TTL, witnessed by the caller's own process",
				"Takes the lease for the caller's seat, witnessed by the pid the socket reports for the caller, or declared unwitnessed. Refused with the incumbent's full status when somebody else holds it or it is orphaned. Re-acquiring your own is the reconnect path and issues a new token.",
				"The handle: name, holder, token, epoch and the time left."),
			leaseWriter("lease.renew", "Lease renew", kernel.No,
				"Extend a lease you hold",
				"Extends the lease against its token and epoch. An orphaned lease is renewable by its own holder, which is why orphaned is not free. A handle from before a daemon restart is fenced by its epoch.",
				"The handle with its new deadline."),
			leaseWriter("lease.release", "Lease release", kernel.Yes,
				"Give a lease back",
				"Frees the lease against its token and epoch. The token stays monotonic per lease, so an old handle can never match again.",
				"Nothing. The lease is free afterwards or the call was refused."),
			leaseWriter("lease.break", "Lease break", kernel.Yes,
				"Free an orphaned lease by a recorded human action",
				"The only way an unwitnessed orphan ever becomes free. Records the caller's seat as who broke it and requires a reason. Refused for a lease still inside its deadline.",
				"Nothing. The lease is free afterwards, naming who broke it and why."),

			// SECTION 16's QUEUES. The list is a read; push, claim and
			// complete keep their state in the estate's bbolt file beside
			// the leases, so they are file writes on the leases' argument.
			// Complete is not idempotent: a second completion is refused.
			readOnly("queue.list", "Queue list",
				"A queue's unfinished tasks, or every queue's name",
				"Answers a queue's tasks not yet done, oldest first, each ready, claimed or orphaned with its claim lease evaluated now, and how many are done. With no queue it names every queue.",
				"The tasks and the done count, or the queue names."),
			leaseWriter("queue.push", "Queue push", kernel.Yes,
				"Add a task to a queue, under a mandatory idempotency key",
				"Adds a task with an idempotency key and a payload. The same key again answers the existing task, done or not, and never queues it twice; the same key over a different payload is refused. Delivery is at-least-once, so the key is what makes a replay harmless.",
				"The task, and whether it was a duplicate."),
			leaseWriter("queue.claim", "Queue claim", kernel.No,
				"Take the oldest ready task under a lease, witnessed by the caller's process",
				"Claims the oldest ready task for the caller's seat as a lease named queue/<queue>/<id>. Heartbeat it with lease.renew; release it unfinished with lease.release. Past its deadline it is orphaned while the worker lives and ready again once the worker is observed dead.",
				"The task and the claim's handle."),
			leaseWriter("queue.complete", "Queue complete", kernel.No,
				"Finish a claimed task",
				"Marks the task done against its claim's token and epoch, and releases the claim. A worker whose claim moved on is refused, and its work is the duplicate the idempotency key is for.",
				"The finished task."),

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
func estateRole(name string) registryv1.EstateRole {
	switch name {
	case "":
		return registryv1.EstateRole_ESTATE_ROLE_UNNAMED
	case "production":
		return registryv1.EstateRole_ESTATE_ROLE_PRODUCTION
	case "development":
		return registryv1.EstateRole_ESTATE_ROLE_DEVELOPMENT
	default:
		return registryv1.EstateRole_ESTATE_ROLE_UNSPECIFIED
	}
}
