## 9. Built for agents

An agent is a first-class user of rig, not an afterthought bolted onto the CLI. Boris runs
agents constantly, so the estate being legible to one is worth as much as it being legible to
him.

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
