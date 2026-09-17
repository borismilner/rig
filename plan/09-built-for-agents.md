## 9. Built for agents

An agent is a first-class user of rig, not an afterthought bolted onto the CLI. Boris runs
agents constantly, so the estate being legible to one is worth as much as it being legible to
him.

### ⛔ EVERY FACILITY AN AGENT NEEDS, AND FINDING OUT WHAT THEY ARE IS AN ASSIGNMENT

**BORIS, 2026-09-17, verbatim:** *"AI agents will have all the facilities they
need or may need for their conveninence in `rig` - and you are responsible on
finding out what those facilities are and consolidate them into our development
plan."*

⛔ **THIS IS AN OPEN-ENDED ASSIGNMENT TO THE TEAM-LEAD SEAT, NOT A FEATURE
REQUEST, AND IT IS THE FIRST OF ITS KIND IN THIS PROJECT.** He has not named the
facilities; he has made naming them somebody's job. **So the deliverable is a
SURVEY consolidated into this section**, and an empty answer is a failure of the
assignment rather than an absence of requirements.

**WHAT IT IS NOT: a licence to invent.** §38's standing rule still binds - grep
the plan first, and *"this planned capability would have solved it"* beats
proposing a new mechanism. **The evidence base this project already has is
better than a brainstorm:** every seat is ARMED (§37), and the 2026-09-17 attack
alone produced armed reports naming the argv cliff on prose, the missing
enumeration, the four-copy step vocabulary, the `in`-versus-`refs` key, the
shared destructive scratchpad and the unqueryable field set. **Those are
measured agent friction, with receipts. Start there, not from imagination.**

| # | Requirement | Whose |
|---|---|---|
| **A0** | **AGENTS GET EVERY FACILITY THEY NEED OR MAY NEED, and the lead OWNS discovering what those are and consolidating them HERE** | **his** |

### ⛔ AN AGENT'S WORKING NOTES LIVE IN rig, AND NOTHING IS EVER LOST

**BORIS, 2026-09-17, verbatim, recorded the turn he said it:** *"The AI agent
working with `rig` should manage its scratch-pads in `rig` so that nothing is
ever lost. It can associate its scratch-pads to anything in `rig` it wants, for
example a work-item, a project, a task, anything it wants, it can also set any
tags it wants for scratchpats for concenience and you can expand this
requirement of mine to make it as convenient as possible for agents to work
with, it can even be called more properly than a scratch-pad. I want to make AI
agents first-class-citizens."*

⛔ **HE INVITED THE EXPANSION EXPLICITLY, SO WHAT FOLLOWS IS PART HIS AND PART
THE LEAD'S, AND THE SPLIT IS MARKED.** The name, the shape and the conveniences
below are the lead's proposal; **A1 to A4 are his.**

| # | Requirement | Whose |
|---|---|---|
| **A1** | **AN AGENT'S WORKING NOTES ARE RECORDS IN rig.** *"So that nothing is ever lost"* is the acceptance test, not the motivation | **his** |
| **A2** | **A NOTE ASSOCIATES TO ANYTHING** - a work-item, a project, a task, *"anything it wants"* | **his** |
| **A3** | **AN AGENT SETS ANY TAGS IT WANTS** | **his** |
| **A4** | **AGENTS ARE FIRST-CLASS CITIZENS**, which is the clause the other three serve | **his** |
| **A5** | **THE NAME IS `working-note`, NOT `scratchpad`** | lead's, and it is his call to overrule |
| **A6** | **THE AGENT ASKS FOR ITS OWN NOTES BACK ON RESUME** | lead's expansion |

**WHY `working-note` RATHER THAN `scratchpad`**, since he invited a better
name: a scratchpad is by definition the thing you throw away, and his acceptance
test is that **nothing is ever lost**. The word fights the requirement. ⛔ **And
it must NOT reuse §39's existing `note` kind**, which is a PROJECT note with
`priority` and `about`, rendered in the brief's section 3 and a case's
`attention_n` list. **An agent's working notes are high-volume and would flood
the surface a human reads** - same word, different lifecycle, and merging them
is how the brief becomes unreadable.

### ⛔ THIS REQUIREMENT SITS ON TOP OF FOUR DEFECTS THE 2026-09-17 ATTACK FILED, AND IT CANNOT BE BUILT WELL OVER ANY OF THEM

**This is the honest state, not a reason to defer: his requirement is the best
forcing function the attack's findings have.** Every one of these is filed.

| Blocker | Why it lands exactly here |
|---|---|
| ⛔ **B60** - `record put` takes its body through **argv only** | **A working note is PROSE.** The one input path is the one that cannot carry it. This is A1's hard blocker |
| ⛔ **B65** - query has **no field predicate** | **So A3's tags are DECORATION.** An agent can set any tag and nothing can select on one. Tags that cannot be queried are worse than no tags, because they look like an index |
| ⛔ **B64** - three of ten kinds are **write-only** | A new kind inherits the same fate: nothing renders it, so a note is reachable only by an id you already have |
| ⛔ **SWEEP-4** - ids are **global and unscoped** | Human ids (`B1`) collide with generated note ids by construction. **A note must mint its own id**, never take a human one |

⛔ **AND THE ONE NOBODY HAS FILED, WHICH IS THE MOST DIRECTLY FATAL TO A4.**
S1+S4 measured the live store: **78 records, ONE distinct seat, SEVENTY-EIGHT
distinct sessions.** Every `record put` minted its own session. **So *"show me
everything I wrote this session"* - the single most natural question an agent
asks of its own notes - is unanswerable by construction.** Provenance records a
login and a moment, and can group neither. **First-class citizenship means the
WHO and the WHEN-TOGETHER work, and today neither does.**

### THE CONVENIENCES, WHICH ARE THE PART HE ASKED ME TO EXPAND

- **Write must accept prose from a file or stdin**, not argv. B60, and it is the
  first thing.
- ⛔ **APPEND MUST BE CHEAPER THAN SUPERSEDE.** An agent adds to a note many
  times in one task. `Put`'s versioning now suppresses identical content (rig
  `0ed7324`), but an append that rewrites the whole body still mints a version
  per keystroke-sized change. **Decide whether a working note is a growing
  document or a stream of small records BEFORE building it** - the two answer
  *"what did I think at 14:00"* completely differently.
- **Association is `part-of` to any id**, which §39 already has and already
  uses. **A2 needs no new link type**, which is the right answer under §38.
- ⛔ **RETRIEVAL IS THE HALF THAT MAKES IT WORTH ANYTHING.** *"Nothing is ever
  lost"* is satisfied by a write-only pit; **what he means is that he can get it
  back.** That needs the field predicate (B65) and it probably needs full text -
  which is **B28, the store search, where `bleve` and SQLite FTS5 are both
  already named as candidates and no search has been run.**
- **A6, on resume:** §39 already specifies a delivery mechanism - the
  `must_read` set, which a brief returns AND marks delivered. **An agent being
  handed its own prior working notes on resume is that mechanism pointed at the
  agent rather than at the project**, and it is how *"nothing is ever lost"*
  closes the loop rather than just filling a table.

✅ ⛔ **AND THE THING WORTH SAYING PLAINLY: THIS REQUIREMENT IS THE PRODUCER THE
ATTACK SAID WAS MISSING.** B64's finding is that **nothing in the workflow
produces records** - eleven rulings on 2026-09-17 and zero in rig. **An agent
writing its working notes into rig as it works is a producer that runs
continuously and needs no discipline to remember**, which is exactly what a
ruling-per-decision does not. **If one thing on this list gets built, this is
the one that changes the eleven-to-zero.**

### The scaling problem, solved first

Fifteen programs with twenty commands each is three hundred tools. Handing an agent three
hundred tool definitions destroys its context before it has done anything. So rig does not do
that.

| Tier | What the agent sees | When |
|---|---|---|
| **Meta tools** | Four, always: `list`, `describe`, `invoke`, `query` | Always loaded. Costs almost nothing |
| **Promoted tools** | Individual commands raised to first-class tools | The ones marked `promote` in the declaration, plus the ones this agent actually uses often |
| **Everything else** | Reached through `describe` then `invoke` | On demand, one round trip |

`list` returns the estate at whatever depth is asked for. `describe` returns one thing in full.
`invoke` runs it. `query` reads state, data, logs, traces and history. Four tools reach
everything, and an agent pays for detail only where it needs detail.

### What every command declares so an agent can use it well

Beyond the argument schema, a declaration carries the things an agent has to guess at otherwise:

| Field | Why an agent needs it |
|---|---|
| `summary`, `description` | Plain language, written for a reader who has never seen the program |
| `examples` | Two or three real invocations with real values. This is the single highest-value field |
| `effects` | `read-only`, `writes-files`, `network`, `destructive`. **Mandatory**, and rig - not the agent - decides what may run (§13) |
| `idempotent` | Whether re-running is safe. Decides retry behaviour |
| `dry_run` | Whether the command can be simulated. rig exposes `--dry-run` wherever this is true |
| `cost` | Rough duration and whether it spends money. Lets an agent plan |
| `preconditions` | What must be true. Checked before the call, so failure is early and explained |
| `returns` | The output schema, so a result can be used rather than parsed out of prose |
| `shape` | `unary`, `stream` or `interactive-stream`. `returns` alone models one request and one response, which `graft run` is not: it emits frames for twenty minutes and may stop to ask a human. Without a shape, four surfaces invent four different treatments of one command and the conformance suite forbids the only correct one |

**`interactive-stream` has one worked case, and it is the hardest one in the estate.** `graft
run` was specified on 2026-09-10 by the session planning it, and it asks for all five of these
in one command:

| What it does | What the contract has to carry |
|---|---|
| Runs for minutes to hours | A deadline that is not a timeout (§18's hang rule cannot fire on a long-running command that is behaving) |
| Emits tens of thousands of frames | Backpressure and a cursor, not a socket that buffers 20k frames for a client that scrolled away |
| **Stops mid-stream to ask a human a permission question** | An inbound question on an outbound stream, answerable **from the toast** and not only from the window (§12), and a run that is *parked* rather than failed while it waits |
| Carries per-run dollar cost | Cost as a typed field on the frame, not a line of log text a surface has to parse back out |
| Needs a 20,000-frame scrubbable timeline | A generated pane that renders a range of a stream, not a table of all of it |

**If `interactive-stream` survives that, it survives everything else here.** It is therefore
the conformance suite's case for the shape (§19), and `fakeapp` grows a command that does all
five, so the shape is tested before graft exists rather than discovered by it.

**Row three is a design target, not an observed shape** - flagged because the whole table is
derived from a program with no code. `interactive-stream` reads as "the stream carries the
question and the answer", and graft's actual permission path is not that: it is
`--permission-prompt-tool` pointing at an out-of-band MCP server, so the question leaves
through a side channel and the outbound stream is *quieter* under the bridge, not richer.
Measured against Claude Code 2.1.267 by the session planning graft: with that flag set the CLI
emits no permission-denied event at all, and an unanswered request does not error - it runs to
a 1800s idle timeout (1,805,940 ms measured) and ends `subtype: success`, `is_error: false`,
`permission_denials` empty. **Anything treating a completed stream as a satisfied interaction
reads that as a pass**, which is a trap for the invoker as much as for the fixture. agentbox
hit the same wall and runs `bypassPermissions` because it does not speak the stream-json
permission protocol. Two in-house data points and neither has an answered prompt, so §21
declares this fixture correctable.

**And one thing nobody owns:** the run-transcript scrubber. graft's plan assigns it to rig's
pane surface; §10 and §11 here assume the program brings its own view. Each assigned it to the
other, so it is unbuilt by both. It is not v1 work - graft is deferred past M13 - but it is
recorded so the next reader of either plan finds it rather than the gap.

**A program declares a preamble, not just commands.** One document an agent must read before
touching that program, served as its own MCP resource and returned by `describe` on the program
rather than on a command. archi's own ordered list of what matters in AI integration begins
"the doctrine is handed over first, before the format, before the tools" - and with per-command
metadata only, that cannot be expressed, so archi keeps its own MCP server and the estate ends
with two.

### Errors an agent can act on

A failed call never returns prose. It returns a structured error: what failed, which
precondition, what the actual state was, and **what would fix it** - including, where it exists,
the exact command that fixes it. An agent that gets "shelf.index.path does not exist; run `rig
shelf init` or set the key" does not need a human.

### Everything that happened is queryable

`query` reaches the same data the introspection views show: logs, traces, the call log, config
provenance, schedule history, the audit log. So an agent can answer "why did the nightly reindex
fail" from rig alone, with the trace, the log lines and the config that was in effect at the
time, rather than asking Boris to look.

### Nothing is hidden from an agent that is not hidden from Boris

The point of §2's decision, stated once so no later section has to re-derive it. An agent he
runs holds `introspect` (§14) and the answer to every question below is *yes*:

| Can an agent read... | |
|---|---|
| every program, its pid, restarts, health history, log tail, goroutine dump | yes |
| **every other client**, including another agent, its calls in flight and its whole history | yes |
| every trace end to end, across both processes, whatever surface began it | yes |
| every config key with its winning layer and all its losers | yes |
| the complete capability map, not the reachable subset | yes |
| the schedule, the audit log, the notification centre, `doctor`'s whole output | yes |
| the coverage of each of those answers, so a gap is visible rather than absent | yes |

**One exception, and it is an absence rather than a filter.** A value a command declared
`sensitive`, and anything the `secrets` service returned, was never written (§15). An agent
does not get "permission denied" for these; it gets an answer that says the field exists and
was never recorded. That distinction matters: a filtered field invites an agent to go looking
for another route to it, and a field that does not exist ends the search. Reading a secret is
`secrets.get` with a grant, which is a different door and an audited one.

**Where that distinction actually comes from, because the obvious answer is wrong.** §15's hot
path *blanks known byte offsets* at 82.5 ns - and a blanked span decodes as an empty value,
indistinguishable from a field that was legitimately empty. A blank cannot say "this exists
and was never recorded". Putting a tombstone in the encoding would say it, at the cost of a
wire change and a re-measurement, and it is not worth either. **The answer carries the
declaration's `sensitive` pointer list instead** - the registry already holds it, per command,
per `semantics_gen` - so a reader intersects the pointers with the record and gets the
distinction from metadata that was never on the hot path. The blanking stays exactly as
measured, and no per-record byte is spent on saying what the declaration already said.

**Three limits of this, stated rather than claimed away:**

- **A program never receives `introspect`.** rig scrubs it from the environment of every
  program it starts (§14), and it is refused to a connection that registered as a program. A
  program that goes and reads it out of an agent's `/proc/<pid>/environ` would get it, and
  nothing here stops that - same-uid is the floor on Linux. Not defended against, because
  every program in this estate is his own code; written down so the boundary is not mistaken
  for a stronger one, the way §13 labels best-effort capability enforcement.
- **Reading is audited by grant use, coalesced, not per read.** §15's "looking is itself an
  event" was written for a human opening one client's history. An agent sweeping the estate
  would turn the audit log into its own transcript, so the record is one entry per principal
  per view per minute, with a count - and that is stated in §15's coverage line rather than
  left for someone to discover as a gap.
- **Complete does not mean instant.** §15's history query budget governs; a thirty-day sweep is
  bounded by that section's numbers, not by this one's promise.

### rig itself is reachable FROM the agent surface, and this had to be said

**ADDED 2026-09-16, after the first four-clause pass against §37's bar measured
that it was not true.** An agent dialling the MCP socket could not reach a single
one of rig's own commands: `invoke` and `describe` both refused `rig` as *"no
program \"rig\" is visible to this caller"*, while the same command answered
normally from a terminal.

**THE REQUIREMENT: an agent can ask rig which estate it is in, and gets an
answer that distinguishes two estates running the same programs.** It is asked
through `query`, section 9's tool for asking rig about rig. No fifth tool, no
second resource, and **no new invoke target** - which is the part the first
draft of this requirement got wrong.

**THIS PARAGRAPH REPLACED A STRONGER ONE THE SAME DAY IT WAS WRITTEN, and the
first version is kept here because the mistake is the instructive part.** It
said: *"every command rig declares of itself is reachable by an agent through
the ordinary meta tools, on the same terms a terminal reaches it"*, to be
delivered by resolving `SelfID` before the program lookup on the agent-facing
read, exactly as the invoker's own read already does. It is a clean-looking
symmetry and it is **unsafe**:

| | |
|---|---|
| **rig declares `down`, and declares it `EffectsDestructive`** | "Every command" is eight commands, and one of them stops the daemon. That change hands `rig.down` to every MCP agent in the estate **in the same diff, silently, with no line of it mentioning `down`** |
| **It was already pinned against** | `TestRigIsNotAnInvokeTargetOnAnySurface`, written 2026-09-12 and marked FOUND BY DEMONSTRATION, asserts the widest possible principal cannot reach rig through the program projection. The requirement would have been delivered by deleting a test whose comment predicts this exact change |
| **It would not have worked anyway** | `Daemon.call` routes to a program's own connection. **rig has no connection to itself**, so a resolved `rig.estate` moves the failure from "not visible" to "not connected". A symmetry that cannot execute |

**Holding rig beside the map is not an asymmetry to be tidied away. It is the
mechanism**, and it carries two jobs at once: `SelfID` is refused to every
registration in two independent places, so no program can shadow rig's
namespace, **and** rig's destructive command has no invoke surface to be
reached through. **A fix that makes the surface look regular destroys both.**

**AND THE REFUSAL MUST NAME THE RIGHT CAUSE.** *"is not visible to this
caller"* is visibility-shaped, and the caller in the measured case was an
introspecting principal that sees everything there is. **An agent acting on
that text goes looking for a grant it already holds, cannot be given, and
would be the wrong fix if it could.** Where the truth is structural, the
refusal says so and names what does answer - which is section 9's own "errors
an agent can act on" applied to the one error an agent was most likely to act
on wrongly.

**WHY THE CAPABILITY MAP COULD NOT CARRY THIS, which is the reason a new
answer was needed at all.** Its version is a **digest of content**, deliberately
- "comparable between two daemons", so it survives a restart and can be compared
across boxes. **The consequence is that two estates holding the same programs
produce the SAME version.** An agent telling production from development by
comparing digests gets a match **in exactly the case that matters**: two
estates of one project, running the same software. The estate's identity is
not derivable from anything the map returns, and that is why it is a field
rather than an inference.

**A SECOND MCP RESOURCE WAS REFUSED, and the reason is a property of the SDK
rather than a preference.** B16 measured that the Go SDK stamps every resource
read publicly cacheable and a handler cannot opt out. **For "which estate am I
in", a cache is the worst possible property**: an agent that dialled
development can be served a cached production answer, and the failure is
invisible at the point of use.

**WHAT query DOES WITH A SUBJECT IT CANNOT REACH: it does NOT refuse, and an
earlier draft of this section said the opposite.** That draft recorded
"`query` MAY NOT ANSWER A SUBJECT IT DID NOT UNDERSTAND" and called the
fall-through a defect. **The reading was wrong twice.** An unrecognised subject
never returned the registry - the condition simply did not match, and the
answer came back with an empty program list. And refusing is pinned against by
`TestQueryDoesNotRefuseASubjectItCannotReach`: *"a refusal would tell an agent
the subject does not exist. Saying where to look next is the difference."*
**Subjects are free-form prose at M2** - the pinned example is *"why did the
nightly reindex fail"* - so an unmatched subject usually means query cannot
reach the source yet, not that the agent mistyped a keyword. `unavailable` is
the answer, and it was already correct.

**WHAT IS STILL OWED, and it is the real half of that misreading.** The answer
says what query CANNOT read and **never says what it CAN**. An agent asking for
the estate in prose lands in the unmatched branch and is told about logs.
**The subject vocabulary belongs in the tool's own description**, which is what
an agent reads BEFORE calling - not in the answer, which it reads after.


**THE MOTIVATING CALLER IS THE ONE THAT WAS BROKEN, which is why this is in §9
rather than in a defect list.** §37 calls the unregistered agent Boris runs "the
motivating caller" and gives it full access. **A terminal holding a method that
caller cannot reach is this specification upside down**, and nothing in §9 said
out loud that it must not be - so it happened, was shipped, and was found by a
demonstration rather than by a reader.

### Discovery is a resource, not a guess

rig serves one MCP resource that is the whole capability map of the estate, versioned and
diffable. An agent reads it once and knows everything that exists. When a program registers a
new command, the map changes, and no agent needs updating.

**It carries coverage per program** (§5k), so an agent knows its picture is incomplete before it
reasons from it - the alternative is an agent confidently reporting that shelf has three
commands. Its product is an estate-wide aggregate, so it is a granted surface: a client with no
grant gets the map of what it may reach, **and an agent Boris runs holds `introspect` and gets
all of it** (§2, §14). The earlier version of this line made the map an operator surface and
left §9 telling agents to read a document §14 would not serve them.

**The version is a DIGEST OF THE PROJECTION THAT PRINCIPAL WAS GIVEN, never a
counter on the registry. This is a scope property, not a performance one.**

**The demo above puts a scoped caller and an introspecting one side by side
reading DIFFERENT maps at the SAME INSTANT. A registry counter gives both the
SAME version**, so anything caching on that version serves one caller the
other's estate - **a scope leak arriving through a cache key rather than through
the filter**, which is the one route the scope filter itself cannot defend. The
version must therefore be a function of the bytes this principal actually
received.

**And a digest survives what a counter does not.** Nothing rig holds survives a
daemon restart (§18), so a counter restarts at zero while the estate it
describes is unchanged - and two daemons serving identical estates would
disagree. A digest is a function of content alone, so it is stable across a
restart and comparable between daemons.

**Three properties the digest must have, each easy to omit and each with a
failure that looks like nothing:**

| Property | Without it |
|---|---|
| **The depth is folded in** | one estate at two depths shares a version. **Only visible on an EMPTY estate**, because with programs registered the depths differ in content anyway - and an empty estate is every fresh daemon, so it is the first case a user meets |
| **Commands are sorted by id** | declaration order is what a program happened to write, not something it declared, so the version changes when nothing did |
| **Every value is length-prefixed** | a digest over concatenated fields cannot tell `ab` + `c` from `a` + `bc`. **Only reachable between ADJACENT fields**, so a test of this property that picks two fields with others between them cannot fail |


---
