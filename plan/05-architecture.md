## 5. Architecture

### 5a. The shape

```
  ┌────────────────────────────────────────────────────────────────────┐
  │  rigd - the daemon. One process, no GUI, no TUI, upgrades alone    │
  │                                                                    │
  │  registry     who is registered, what they declared                │
  │  config       layers, schema, provenance, live push                │
  │  store        location, migrations, backup, integrity              │
  │  secrets      keyring, namespaced per program, never recorded      │
  │  observe      log + trace + metric ingest, merge, query            │
  │  control      start, stop, invoke, health, supervision             │
  │  schedule     one scheduler for the whole estate                   │
  │  bus          events between programs, by grant                    │
  │  rules        who may run what, and what must ask first            │
  │  hosted       in-house programs with no binary of their own (5j)   │
  │  surfaces     CLI · MCP · HTTP · window · tray · toast · URL       │
  └───────────────────────▲────────────────────────────────────────────┘
                          │  one unix socket, length-prefixed protobuf frames
                          │  supply ▲   control ▼
   ┌──────────────────────┼──────────────────────┬───────────────┐
 shelf                  graft                  archi           nudge
 business logic         business logic         business logic  business
 + ~200 line stub       + ~200 line stub       + ~200 line stub  logic
```

### 5b. What each surface is

| Surface | What it is | Reached by |
|---|---|---|
| **CLI** | `rig shelf reindex --since 7d`, with `--help` and completion generated from the declared argument schema | A terminal, a script, a Makefile |
| **MCP** | Every declared command is an MCP tool on a server rig exposes. An agent drives the whole estate through one server | Claude Code, and any other agent |
| **HTTP** | `POST /v1/shelf/reindex` on `127.0.0.1`, same schema, same auth | curl, scripts, other machines later |
| **Window** | A rail of programs, a pane each, generated or embedded | A mouse and a keyboard |
| **Tray** | One icon for the whole estate, per-program status and commands | Always-on presence |
| **Toast** | A notification with the command's own actions inside it | Attention when something finishes |
| **Palette** | `Ctrl+Space` over every command in every program | Keyboard-first work |
| **Schedule** | Any command becomes a cron entry with one config line | Time |
| **URL** | `rig://shelf/search?q=...` | Links, other apps, the browser |
| **Bus** | One program's event fires another's command | Automation between programs |

**A declaration never names one of these.** A command declares properties (§5e); each surface
declares which properties it can carry, plus a rig-side predicate over them - `slack.include =
"effects != destructive && duration < 30s"` - living in that surface's own config schema,
defaulted by the surface author and overridable by Boris in one place. Adding a row to this
table is therefore a change to rig and never to a program, which is the whole compounding
claim in §1 and the only version of it that survives a surface being added later.

### 5c. What lives where

| Concern | rig | The program |
|---|---|---|
| Config file format, layering, provenance, live reload | ✔ | declares a schema |
| Where data lives, migrations, backup, integrity, retention | ✔ | owns its schema and its queries |
| Secrets storage | ✔ | names the keys it needs |
| Log collection, merging, rotation, viewing, shipping | ✔ | emits records |
| Traces, metrics, health surfaces | ✔ | emits spans |
| Every UI: window, panes, settings, toasts, tray | ✔ | declares schema, or serves its own HTML |
| Process lifecycle, restart, supervision | ✔ | responds to health checks |
| Scheduling, event routing, workflows | ✔ | declares events and commands |
| Packaging, updates, install | ✔ | ships a binary and a manifest |
| Authorization: which caller may run which command | ✔ | declares `effects` and danger, nothing more |
| Redaction: what must never be recorded | ✔ enforces | declares which fields are sensitive |
| Coverage: whether this is the whole program | renders it | ✔ declares it honestly |
| **Business logic** | | ✔ **all of it** |

### 5d. The client is a dumb pipe

The one piece of rig code that lives inside a program has to be as close to frozen as it can be,
because every symbol in it is a rebuild nobody can avoid later. So it does exactly five things:

1. Find the socket.
2. Frame messages, and stamp every mutating call with a client-generated request id.
3. Reconnect when rig restarts, presenting the session token it already holds.
4. Tolerate absence: a bounded outbound queue, backoff to a deadline, and one typed
   `unavailable` error (§5g).
5. Hand over the bytes of the resolved snapshot when the program asks for them, without
   interpreting them (§5g).

**It carries no semantics.** It does not know what a notification looks like, what config keys
mean, how a UI is described or what commands exist. All of that is data negotiated at connect
time. This is the Language Server Protocol lesson: the editor holds a dumb client, every bit of
intelligence is server-side, and servers upgrade freely.

**READ "NO SEMANTICS" AS THE SENTENCE AFTER IT DEFINES IT: A BAN ON SCHEMA
KNOWLEDGE, NOT ON TRANSPORT BEHAVIOUR.** Duties 3 and 4 above are reconnection,
backoff, a bounded queue and a typed `unavailable`, and §5g classifies all four
as **mechanism** and sizes them at "about fifteen lines in the stub". The line
§5d draws is **policy versus mechanism** - the 300-line fallback §5g deleted was
policy - and none of the five duties crosses it.

**This is written down because a comment in the stub itself asserted the
opposite and three seats acted on it.** `client/surface.go` read the ban as
covering retry, backoff and queueing, which are duties this section assigns to
the stub by name. A worker checked the section and found the inversion. **The
cost of a false constraint is higher than a gap: a gap invites investigation,
while a false constraint closes it** - here it nearly produced a specification
for a new home for the tolerant client, which §5d and §5g already home in the
stub.

**AND ONE GENUINE TENSION INSIDE THIS SECTION, NOW RESOLVED.** Duty 2 says stamp
every **mutating** call, while the paragraph above says the stub does not know
**what commands exist**. Deciding mutating-ness needs declared effects, which is
exactly the schema knowledge this section forbids it. **Ruled 2026-09-11: the
stub stamps EVERY call.** That needs no schema knowledge, satisfies duty 2's
intent because no mutating call is ever left unstamped, and costs one string per
frame. A stamped read-only call is ignored by the window rather than mishandled,
since the window keys on method. **The alternative - the caller computes
mutating-ness and passes a flag down - makes duty 2 conditional on the caller
and leaves every non-CLI program with no id at all**, which defeats the duty for
precisely the callers this section exists to serve.

**Its surface is enumerated, budgeted and reported.** The public symbols are listed in one file
with a count asserted in `make ci`, and every connection reports `stub_build` alongside the wire
version. That last part matters: a defect in the pipe is fixed in code the daemon cannot reach,
so rig must at least be able to name every program still carrying an old one rather than
guessing. `rig doctor` lists them.

There is a **generated convenience layer** on top, giving typed methods for ergonomics. It is
optional and versioned. If it goes stale, the program still works, because rig accepts every
wire version it has ever shipped.

### 5e. Registration: what a program declares

Once, at connect. This is the whole contract from the program's side, and it is data: no rig
code runs inside the program to produce it.

```
identity        id, name, version, icon, description
coverage        full | partial (default partial), with a note
semantics_gen   integer. Pins what every declared name means, for this
                program, for its lifetime (§21)
services        which rig services this program uses. Unused services are
                never initialised for it and cost it nothing (§5k)
preamble        the one document an agent must read before touching this
                program. Optional, markdown, served as its own MCP resource
commands        id, title, argument schema, properties (below), examples
config          JSON Schema for everything it can be told, with defaults
state           named values rig may read and display
data            queryable collections: column schema, filters, sorts
events          topics it publishes, with payload schemas
ui              nothing (generated), or a URL rig should proxy and embed
capabilities    what it needs: secrets by name, paths, network, other
                programs' events
health          how to check it and how often
hosted          false (its own binary) | true (compiled into rigd, §5j)
```

**A command declares properties, never surfaces.** This is the change that makes a surface added
later free, and it is the single most important correction in this document.

| Property | What it says | Mandatory |
|---|---|---|
| `effects` | `read-only`, `writes-files`, `network`, `destructive`, `drives-input` | **yes** |
| `idempotent` | Whether re-running is safe. Decides retry, replay and schedule coalescing | **yes** |
| `sensitive` | JSON pointers into arguments and results that must never be recorded (§15) | **yes**, may be empty |
| `interactive` | Needs a human in the loop while it runs | yes |
| `streams` | Emits a stream rather than one result | yes |
| `needs_display` | Draws on screen and is meaningless without one | yes |
| `duration` | Order of magnitude: instant, seconds, minutes, hours | yes |
| `confirms` | Asks before doing the irreversible part | yes |
| `shape` | `unary`, `stream`, `interactive-stream`. What a result *is* | yes |
| `summary`, `description`, `examples`, `returns` | For a reader who has never seen the program (§9) | yes |
| `dry_run`, `cost`, `preconditions`, `promote` | Planning and promotion hints | no |

**Surfaces declare requirements, and rig computes the projection.** Each surface states which
properties it can carry, plus a rig-side predicate over them - `slack.include = "effects !=
destructive && duration < 30s"` - living in that surface's own config schema, defaulted by its
author and overridable by Boris in one place. No program is ever touched to add or remove a
surface.

`snapper.capture-region` is the worked example. It declares `needs_display: true`,
`interactive: true`, `duration: seconds`. The scheduler declares that it can satisfy neither
`needs_display` nor `interactive`, so the command is not schedulable - and nobody wrote
`snapper` anywhere inside rig to get that answer. The plan's previous shape, where a command
named its surfaces, made the surface set a closed vocabulary inside the registration schema and
killed the compounding claim in §1 outright.

**No property has a default that carries a safety meaning.** `effects`, `idempotent` and
`sensitive` are mandatory and a registration missing any of them is refused. A field whose
absence means "safe" can never have its default changed without lying about every declaration
written before the change. Where a default genuinely must exist, it is pinned by
`semantics_gen`.

**The declaration is generated, not hand-written.** Measured: one real archi command written
exactly as this section and §9 require is **160 non-blank lines of JSON**, and archi has 28
deduplicated operations. The 100-line budget in §3 is per program of *hand-written Go*; the
declaration itself comes out of the program's existing command definitions, and `rig verify`
checks the artefact.

Everything after `identity` is optional except `coverage`, `semantics_gen` and the mandatory
command properties. A program that declares only `commands` still gets a CLI, an MCP tool, an
HTTP route, a palette entry, a tray item and a schedulable job - each one wherever its declared
properties allow it.

### 5f. Transport

**One socket for the whole daemon**, at `$XDG_RUNTIME_DIR/rig/rigd.sock`, mode 0600. A program's
channel is a multiplexed stream on the connection it already holds. There is no per-program
path: a socket named after a program turns the runtime directory into a second copy of the
registry that `ls` enumerates and `stat` polls for presence, and a mode bit is a uid instrument
being asked to enforce a client boundary (§14).

**Exactly one `rigd` per ESTATE, and it is enforced rather than assumed.** **The boundary is the
runtime directory, not the uid, and §37 is where that is specified** - one user runs at most two
named estates, production and development, and the sentence that used to stand here said "per
user" while the mechanism below has always been per directory. **Do not "fix" the mechanism to
match the old sentence:** every test in this repository starts an estate of its own, and a
genuinely per-uid lock makes rig undevelopable. `rigd` takes an
exclusive `flock` on `$XDG_RUNTIME_DIR/rig/rigd.pid` before it binds, and a second instance
exits with the incumbent's pid rather than unlinking the socket and taking over. **This is the
mechanism §16's entire argument rests on** - "every coordination operation passes through a
single serialization point, which makes them linearizable by construction" - and it was a
premise with no implementation anywhere in the document. Two daemons over one state tree give
two serialization points, two WALs and two lock namespaces, silently, and every property §16
proves is false for as long as it lasts. The window for that is not hypothetical: it is the
whole of M0 to M7, when the daemon is started by hand many times a day. **M0**, with a chaos
test that starts two and asserts the second refuses.

Length-prefixed protobuf frames, hand-framed, bidirectional, multiplexed streams. **No gRPC**:
its server on a unix socket measured +9.80 MiB resident for HTTP/2 machinery a local socket does
not need, which is 58% of the whole footprint budget (§17). Numbers in §4 say a plain socket is
enough.

- Control plane, request/response: 6.2µs. Config, commands, queries, health.
- Data plane, one-way: 561ns. Logs, traces, metrics, progress events.
- Large payloads: 64KB frames at 19µs is 3.4 GB/s effective. Fine for a table of 100k rows.
- Every frame carries a trace context so a span crosses the process boundary.
- **Every mutating call carries a client-generated request id.** rig dedups against a bounded
  window persisted in the WAL and returns *the original response*, never a fresh application.
  Without it, one retried call after a timeout is enough to make §16's linearizability claim
  false, and the retry cannot live in the stub because the stub is forbidden semantics (§5d).
- **Every connection carries a session token that survives reconnect.** On reconnect rig either
  resumes the session, with leases and subscriptions intact, or answers `SESSION_DEAD`, at which
  point the client knows exactly what it lost. Silence is not an answer either way.
- **Every proto enum reserves `*_UNSPECIFIED = 0`** and an unknown enum value is refused at the
  daemon boundary rather than decoded to zero. Without this a new `effects` value falls through
  to zero on an old binary and a destructive command reports itself read-only. Lint gate, §21.

### 5g. When rig is not running

**The client tolerates rig's absence. It does not reimplement rig.** The 300-line fallback this
plan carried before the attack was a second, un-upgradable, unobservable implementation of
config layering, storage paths, secret sources and log sinks, compiled into every program - that
is policy, in the one component §5d requires to be semantics-free, and it cannot be patched from
the daemon. It is deleted. Three mechanisms replace it.

**1. A tolerant client.** A reconnect loop with backoff to a deadline, a bounded outbound queue,
and one typed `unavailable` error. Mechanism only: no layering, no merging, no answers.

**2. A resolved snapshot on disk.** Every time rig resolves configuration it writes the
already-merged result to a known path, with provenance comments, the database path it assigned,
and the schema version it migrated to. When rig is unreachable the stub reads that one file and
hands over the bytes. There is still exactly one implementation of config resolution, and it
lives in the daemon. About fifteen lines in the stub.

Secrets are deliberately not snapshotted - that would be plaintext on disk - so they are
unavailable while rig is, and a program declares whether it can run without them.

**3. Lifecycle notices**, so the client knows whether to wait and for how long. A control frame
on the existing connection, best effort by definition: a SIGKILLed rig sends nothing and the
client sees EOF.

| State | Meaning | What the client does |
|---|---|---|
| `READY` | Serving | Normal |
| `DRAINING` + `back_in` | Going down deliberately, expected back | Hold requests in the bounded queue, wait, resume |
| `UPGRADING` + `back_in` | Going down and straight back on the same socket | Same, tighter window, reconnect with the session token |
| `GOING_AWAY` | Going down and not coming back | Stop waiting. Refuse new work with an honest message |
| EOF, no notice | Crashed | Unknown. Retry with backoff to a deadline, then treat as `GOING_AWAY` |

Every notice carries a reason string, so the program's log and the terminal say *why* they are
waiting rather than only that they are.

**"Coming back up" cannot be a push, and this plan says so.** Once the connection is gone rig has
nobody to notify. The honest mechanism is client-side retry bounded by the deadline in the notice
it already received, plus a **readiness handshake on reconnect**: rig sends `HELLO` carrying its
version, wire version, whether this is the same instance, whether the session was restored, and
explicitly what was lost - which in-flight commands, which leases. For a client that was never
connected, a socket that exists and accepts is the whole signal.

**The window draws the degraded state, not the daemon.** A toast saying "rig is restarting"
cannot be drawn by the thing that is restarting. This is where §17's separate-window rule pays
off a second time: the window process survives a daemon restart, so the tray icon gains a fourth
state beyond ok/degraded/stopped - **detached**, meaning the window is up and the daemon is not.

**What a program actually loses while rig is down**, stated plainly, because the previous table
claimed more than it could deliver:

| Capability | rig up | rig down |
|---|---|---|
| Config | Served, live reload, provenance | The resolved snapshot, read once, no reload |
| Storage | Managed location, migrations run before start | The snapshot names the path; **a program that has never started under rig does not start** |
| Secrets | From the keyring | Unavailable. The program declared whether it can run without them |
| Logs | Collected, merged, viewable | The program's own stderr. rig collects nothing it did not see |
| Notifications, tray, window, palette, schedule | Present | Absent |
| **Business logic** | **Works** | **Works, if its declared preconditions are met** |

That last row is the honest version. A migrated `nudge` whose rig unit failed used to boot into
eight hours as a headless process with no interface at all, and call it success; now it says
`unavailable`, with a reason, and `rig doctor` can see it.

### 5h. Extending rig itself

rig has to be extensible in two directions, and they are different problems with the same
answer: both are driven off the one registry.

```
              ┌─────────────────────────────────┐
   SERVICES   │            registry             │   SURFACES
   what rig   │  what every program declared    │   how the estate
   gives      └─────────────────────────────────┘   is reached
      │                     ▲   ▲                          │
      ├─ config             │   │              CLI ────────┤
      ├─ store              │   │              MCP ────────┤
      ├─ secrets            │   │             HTTP ────────┤
      ├─ observe            │   │           window ────────┤
      ├─ schedule           │   │             tray ────────┤
      ├─ bus                │   │            toast ────────┤
      ├─ centre             │   │          palette ────────┤
      ├─ peers              │   │             cron ────────┤
      ├─ events     ← new   │   │              URL ────────┤
      ├─ queue      ← open  │   │            Slack ────────┤ ← new
      ├─ cache      ← open  │   │            voice ────────┤ ← new
      └─ machines   ← open  │   │              TUI ────────┘ ← new
```

**A service extends what rig gives programs.** It implements one interface: a name, a config
schema, a lifecycle, and a set of methods exposed on the wire. Adding one gives every program a
new capability the moment they ask for it, with no program rebuilt.

**Nine ship inside v1** and each has a milestone: `config` (M4), `store` and `secrets` (M11),
`observe` (M5), `schedule` and `bus` (M13), the notification `centre` (M9), `peers` (M7) and
`events` (M4).
The centre and peers are services rather than parts of a surface, for the reason stated below,
and the diagram says so because a picture that omits them undersells what the wire carries.

**Three are marked `← open`, which means no milestone owns them.** A job queue, a cache and
declared state machines are drawn because they are plausible, not because they
are planned - and drawing an unowned box as though it were scheduled is how a plan lies to its
own reader. Also named and not drawn: an HTTP client with shared retry and rate limits, a
template renderer, a lock manager, a diff service. **Nothing here ships without a program that
adopts it** - the rule is stated here, and §5k covers per-service adoption and declared
coverage rather than this gate - and §24's M3 gate is where each is either given a
milestone or struck.

**`voice` in the surfaces column is voice *input*, and it stays unowned.** Speech output - rig
saying a line out loud - is not that box. It is a renderer on the notification centre and it
does have a milestone (§12, M9). The two share a word and nothing else, and drawing them as one
box is how a surface acquires a milestone it has not earned.

### rig is an event source, and a program arms what it wants to hear

§2 locks two directions: apps call rig for supply, rig calls apps for control - start, stop,
invoke, query, reload. **Nothing in that list is rig saying that something happened**, and rig
sits on more of the estate than any program can see. The `events` service closes that
direction: a program arms the kinds it cares about, by pattern, and receives them on the
connection it already holds.

**It adds no mechanism. It collapses three that already exist and are each hand-rolled.**

| Already required | Today | Becomes |
|---|---|---|
| §6's config live push | a bespoke push path | `config.changed` |
| §5g's lifecycle notice and its reason string | a second bespoke path | `rig.stopping` |
| §18's health transitions | computed, never emitted | `program.health` |

**That is also what clears this section's own adoption gate.** Two of those three are milestone
requirements before any of this ships, so the service has adopters on the day it lands rather
than a hope of one. It arrives at **M4** with `config.changed` as its first kind, because
config is the first push that is required and building it as the general mechanism costs what
building it once for config costs. M6 then adds kinds, not machinery.

**Four kinds are worth naming, and each is one only rig can emit:**

| Kind | Why no program can know it | Ships |
|---|---|---|
| `config.changed` | rig owns the layers and the provenance (§6) | M4 |
| `system.resumed` | rig owns `boot_id` and `CLOCK_BOOTTIME` (§16) | M6 |
| `program.health` | rig is the supervisor (§18) | M6 |
| `grant.revoked` | rig owns the rules, and the alternative is a failure (§13a) | M13 |

**`system.resumed` fixes a defect that is live in every program today.** §2 locks every
deadline to boottime because this laptop suspends nightly. rig is the only component that
knows the clock jumped eight hours, so every cache, timer and *last run at* in the estate is
wrong every morning and no program can detect it. One kind, and nothing is rebuilt.

**`program.health` retires the dependency graph this plan nearly needed.** A program that must
not start before graft arms `program.health` on graft and waits. Same outcome as a declared
inter-program dependency list with a derived start order, without either of them existing.

**A subscription is a filter, never a trigger.** An event says what happened and carries no
command. The `bus` already fires a command from an event and §13a already mints the principal
for it, so an event that could invoke would be a second and weaker invoker. The two compose
the way `schedule` and the candidate below do.

**It costs the stub nothing** (§5d). The pipe's public symbols are budgeted and asserted in
`make ci`; an arming is data negotiated at connect, which is what the pipe already does with
every other declaration.

### The file-watcher is a source on `events`, not a service of its own

§26's question 10 asks which program adopts each `← open` service. For the file-watcher the
answer is that **none has to**: rig watches paths declared in its own config and publishes
`fs.changed`, so its first consumers are `events` and `bus` rather than a program that would
have to be talked into it. It is struck as a service and lands as a kind at M13 with the bus.

**Two properties are load-bearing, because a watch that misses silently is worse than no
watch.** inotify hands out no revision, so §16's answer for the blackboard - *subscribe from a
revision, and a client that reconnects gets what it missed rather than a gap it cannot detect*
- is not available here. Measured on this laptop, 2026-09-10: `max_user_watches` is 524288, so
watch exhaustion is not the constraint it is assumed to be, but **`max_queued_events` is
16384**, which a single `git checkout` overruns. The kernel then sets `IN_Q_OVERFLOW`, and the
lost events are neither recoverable nor enumerable.

So: a rescan-and-diff on start and after every overflow, with the gap declared in §15's
coverage log rather than absent. Debouncing is the `idempotent` rule §18 already applies to a
missed schedule fire and it composes unchanged, which matters because one editor save is three
to five inotify events.

### Pressure is a source too, and this kernel will not wake us for it

§33 banked a finding this answers: no disk budget, and ENOSPC missing from the coverage log's
causes. rig already sets a 500 MB call-log budget and already drops the oldest with a coverage
entry (§15), so it is already the component that learns first that the machine is running out
of something. `disk.pressure` and `memory.pressure` publish that instead of keeping it.

**This is the weaker claim, and it is written as the weaker claim.** The four kinds above are
things only rig *can* know. Any program could read `/proc/pressure` itself; the argument here
is that fifteen of them doing it is the defect, not that they are unable to.

**Measured on this laptop, 2026-09-10, kernel 7.0.0-31-generic:**

| Probe | Result |
|---|---|
| `CONFIG_PSI=y`, `CONFIG_PSI_DEFAULT_DISABLED` unset | present |
| reading `/proc/pressure/{cpu,memory,io}` | works: `some` and `full`, at avg10/60/300 |
| **writing a PSI trigger** - four valid formats, `O_RDWR` and `O_WRONLY` | **every one rejected, `EINVAL`** |

**So there is no wake-on-threshold here, and the plan must not assume one.** rig polls the read
side on the health interval it already runs and computes the transition itself, exactly as §18
computes a health transition rather than being told about one. That is one more reason the
event belongs to rig: one poller, not fifteen.

**Free space and stall pressure are different numbers and both are needed.** PSI reports how
long tasks stalled on IO. It says nothing about a filesystem at 98% that is not stalling at
all. `disk.pressure` carries both, per watched path, and the paths default to the ones rig
already owns: the call log, the store, the socket directory.

It lands at **M5** with the observability budgets it exists to defend, and its first consumer is
rig's own retention rather than a program that had to be talked into it.

### The process monitor knows that something died, and only sometimes why

The ask was for a program to be told when a process it cares about terminates or crashes, "and
maybe if possible even know their exit code". The answer has two halves and the second is a
constraint rather than a feature:

| The process | Death | Exit code |
|---|---|---|
| rig started it (§18) | yes, immediately | **yes** - `wait4`, because rig is the parent |
| rig did not start it | yes, immediately | **no** |

**Measured on this laptop, 2026-09-10.** `pidfd_open` (syscall 434) against a pid rig is *not*
the parent of succeeds, and `poll` on that descriptor woke on the process's death after 1.00s
of a 1.0s sleep - so death notification for an arbitrary pid is exact and costs no polling.
`waitid(P_PIDFD, …, WEXITED|WNOWAIT)` on that same descriptor returns **ECHILD**, and
`/usr/include/linux/pidfd.h` on this machine defines `PIDFD_NONBLOCK` and nothing else: no
`PIDFD_GET_INFO`, no `PIDFD_INFO_EXIT`. There is no unprivileged route to the exit status of a
process you did not fork.

**That asymmetry is the argument, not the embarrassment.** A program that wants the exit code
registers the process with rig and lets rig start it, which is what §18 already does. A program
that only needs to know a pid is gone gets that for anything on the machine.

`process.exited` carries the code where rig has it and the literal `unknown` where it does not.
**A field that is sometimes silently zero is worse than a field that is sometimes absent** - a
watcher cannot tell a clean exit from an unobservable one, and zero is the answer that reads as
success.

It lands at **M6** with supervision, where the parent half is free, and its first consumers are
§18's crash policy and the tray's crash panel.

### Declared state machines, if the supervisor can be its first client

The one `← open` candidate with a design rather than a name, written down because the argument
for it and the condition on it are both specific.

**The argument for is the compounding one, and it is strong.** A state machine hand-rolled
inside a program gets nothing from rig. A *declared* one gets the generated pane showing the
current state and the legal transitions from it (§10), every transition in `observe` with a
trace (§15), a `confirm` on any transition declared destructive (§13a), and a TUI view - all of
it for free, because that is what a declaration buys here.

**The argument against is that the transition table is the easy part.** The hard parts are
side effects on transition, a retried transition applying twice, and a process dying
mid-transition; and rig already owns all three - §4's request-id dedup returns *the original
response* rather than reapplying, §16 has the WAL, absolute deadlines on `CLOCK_BOOTTIME` and
fencing tokens. So the service is a thin layer over machinery that exists, and a thin layer is
worth building only if something real adopts it.

**The condition, and it is decidable now rather than at M13: §18's supervisor is the pilot.**
rig hard-codes three state machines already - the supervisor (starting, healthy, degraded,
restarting, quarantined), the lease lifecycle with its two-step expiry (§16), and the crash
panel's countdown. §5h's own rule is that shared vocabulary lives in the service that owns it.
**If the supervisor cannot be expressed in this service, the service is not general enough to
exist** and the candidate is struck. That test costs a design session, not a milestone.

**The ceiling, stated because this is one step from a workflow engine.** States, guards and
transitions are declarable; **nothing that schedules is.** A timed transition is a `schedule`
entry firing an ordinary transition, so the two services compose rather than one absorbing the
other. Retries, compensation and sub-machines are out of scope by decision, not by omission -
that is Temporal inside `rigd`, and it fails §17's footprint budget and §20's chaos matrix at
the same time.

**A surface extends how the estate is reached.** It implements one interface: it reads the
registry and projects it. It never talks to a program directly, only through the registry and
the invoker, which is what keeps surfaces from multiplying the contract. Adding one gives every
already-registered program that surface for free. That is the compounding property, and it is
worth more than any individual surface.

**A renderer extends how a declared thing is drawn.** A new widget type in the generated UI - a
map, a waveform, a calendar - is a renderer registered against a schema shape. Programs that
already declare that shape get it without changing.

**A renderer publishes a capability profile**, and this is what stops "one schema, two
renderers" being a lie. The program declares *semantics only* - types, constraints, relations,
cardinality - and never a widget or a hint. Each renderer publishes the JSON Schema constructs
it can express, so a renderer that cannot draw an array of objects says so, in its own artefact,
and the conformance test asserts **fidelity** rather than the existence of a renderer. Where a
human genuinely wants a different widget, the override is rig-side config against the schema
shape, never a field in the program's declaration.

### The element kit, so a program that draws itself still looks like rig

A renderer draws something the program **declared**. A **kit element** is the other direction: a
program serving its own HTML asks rig for a table, a toolbar, an empty state, a skeleton - and
gets rig's, in rig's theme, behaving the way that element behaves on every other surface.

**A program picks a subset and fits it to its own layout. It does not get to invent one.** That
split is the whole design: *which* elements to use is the program's choice, and how each one
looks and behaves is rig's. It does not weaken the rule above - a program still never names a
widget for **declared** data. It names elements only for the HTML it serves itself, which today
gets no help at all.

**The first version of this tier was attacked on 2026-09-10 and did not survive**, and the two
things that killed it are the two this version is built to avoid.

**First, it had no adopter.** It was pitched on `archi`, `dispatch` and `snapper` each
reimplementing a table, a toolbar, an empty state and a spinner "and all four subtly wrong".
Measured: `archi` has zero tables, `dispatch` has one call site, `snapper` serves no HTML at
all, and **no program in the estate has a spinner** while two of them have 114 skeletons. Six of
those twelve claims did not exist. **So the adopters are now the fake applications of §23**,
which is the only way this tier gets exercised before a real program is asked to change.

**Second, the inventory was taken from a sentence rather than from the estate.** The census that
should have set it: `clinic` 143 tables, `pull-report` 18, `devtool` 4, `minibot` 4, `archi` 2,
`dispatch` 1. **A fake application that is written to fit the kit proves nothing**, so each one
is modelled on a real program's measured markup and the kit is what has to bend.

| # | Requirement | Why it is a requirement and not a preference |
|---|---|---|
| R1 | **The inventory is open while the only adopters are fake, and closes on the first real one.** Until then adding an element is a normal change | A closed inventory bought protection for programs that had not adopted, and cost the ability to change the kit at all. The fragmentation it guarded against has not happened: `dispatch` and `devtool` already share a byte-identical component set that rig did not supply |
| R2 | **One renderer, plus the pane-level terminal fallback §10 already requires of the program** | A renderer draws a *declared* thing, and a kit element declares nothing, so there is no input a second renderer could consume. Conformance item 20 says every declared schema shape has a renderer **or** a fallback; it is a disjunction, about the generated tier, and it does not reach an element |
| R3 | **The element list is a registration declaration** - a field in §5e beside `services` - and config may subtract from it, never add | One authority, so the check has somewhere to run. Routing it through §6 put four layers above the program's own declaration and made the failure land at render, which R7 exists to prevent |
| R4 | **The kit is unversioned until the first real adopter, and carries a generation from that day** | Serving every generation forever protects a program that never asked, and there is no such program yet. Applied now it would freeze the kit against the one thing it needs, which is to change while the fake applications are still finding its shape |
| R5 | **Every element passes the contrast gate on every surface it can land on, in both themes.** The gate is `make contrast`, and it runs in CI | §20 asserts this for the generated token set. An element is where a token actually meets text. **`make contrast` currently invokes a tool that has never existed and CI never calls it**, so this requirement is a promise about a gate that has to be built first |
| R6 | **No element knows which program is holding it.** It takes data and emits events | §5h's own rule: a service or surface that needs a program's identity to work is business logic in the wrong place. Measured and it holds: the hue reaches an element as a custom property on the root, so the element reads a token and never learns an id |
| R7 | **Using an element rig has removed fails at registration**, which follows from R3 rather than from a bridge | The alternative fails at render, in front of the user, on the one surface whose entire product is presentation |
| R8 | **The kit is a section of `design/visual-system.html`**, each element beside the fake application that uses it | Section 09 shows renderers beside the declaration *that produced them*; an element has no declaration, so it is shown beside its caller instead. The page has nine sections and **no navigation of any kind** - no nav, no anchors, no ids - which is fixed before a tenth is added |

**Every requirement above gets a test or it is deleted.** R1-R8's first version had none: no
conformance item and no §20 row mentioned an element or the kit, so eight rules were enforced
by review. **They now have four**: §19 item 24 checks the declared element list, item 25 renders
every element at every generation rig serves, §20's element-scale row measures contrast where a
token actually meets text, and `make contrast` runs in CI. **R3, R5 and R7 are tested by those
and R2 by item 25.** R1, R4, R6 and R8 are still enforced by review, and each says so above
rather than implying otherwise. A requirement with no test is a preference with a bold heading,
which §3 and §5i both already say in their own headings.

**What the kit is not.** It is not a second theme surface - there is one token set and the
element list selects from it rather than adding to it. It is not a widget hint on declared data,
which §5h forbids above and which would fragment the generated tier to fix the embedded one. And
it is not a framework: an element is markup, tokens and behaviour, with no opinion about how the
program builds the page around it. **Behaviour is the half with no definition anywhere** - focus
order, keyboard handling, what a click does - and the fake applications are where it gets one.

**Modules contribute capability names; the kernel does not enumerate them.** §13's capability
set was literally `tray`, `notify`, `schedule` - three module names hard-coded inside the
kernel's enforcement point, so adding a surface meant editing the kernel before the surface
existed. A module registers the capabilities it grants, and the kernel checks grants against a
set it never had to know in advance.

All four kinds can be **in-tree** (compiled into `rigd`) or **out-of-tree** (a separate process
rig supervises, speaking the same wire in the opposite direction). Out-of-tree is the same
machinery a program uses, pointed the other way, so there is one contract to test rather than
two.

**Go has no working dynamic loading, so "hot-reloadable" and "out-of-tree" are the same
statement.** An in-tree module changes only when `rigd` is rebuilt. This plan says that rather
than implying a free choice, and it is the reason a hosted program (§5j) forfeits independent
upgrade.

**The notification centre is a service, not part of the toast surface.** It is the record of
record for everything that was ever notified, and §12 promises nothing is ever only a toast. A
surface that owns it means `make build-minimal` silently drops that record; a service means the
toast surface, the window, the TUI and the CLI all read one thing.

**The rule that keeps this honest:** a service or surface that needs to know a program's
identity to work is not a service or surface. It is business logic in the wrong place, and the
lint rule that bans program ids in rig code catches it.

### 5i. Modularity, made testable rather than claimed

"Modular" is an adjective until something fails when it stops being true. Six rules, each with a
test behind it, and the tests are the point - the previous set had two rules whose gates could
not fail.

| Rule | The test |
|---|---|
| **The kernel is small, and it is mechanisms.** Registry, wire, principal and scope, invoker (with streams), supervision, config resolution, capability check, module lifecycle | The kernel's public API is enumerated in one file with a **symbol budget asserted in `make ci`**, exactly as §3 already does for the client stub. Growth costs a recorded decision, not a nod |
| **The kernel holds no domain type.** No `Severity`, no `LogRecord`, no `Lease` | Shared vocabulary lives in the service that owns it, and another module reaches it as a *renderer of that service* - a directed edge the analyzer is taught to allow. Without this rule, the cheapest way to share anything is always to move it into the kernel |
| **No module imports another module.** Services and surfaces talk through the kernel | `make modules` runs a layering analyzer that fails the build on a cross-import, **runs under every tag set CI builds**, and rejects the untyped service locator that made the previous analyzer blind: modules receive kernel-owned interfaces injected at construction from a declared dependency list |
| **Every module compiles in alone.** Not merely out | `make modules-matrix` builds kernel-plus-one for every module in turn. `make build-minimal` is the N=0 case. One build tag proves only that coupled modules are removed together |
| **A module knows no program's name** | The same lint that bans program ids in rig code, extended to the frontend: a program id special-case in Svelte used to pass `make ci` untouched |
| **A module's dependencies are data** | The declared list the analyzer and `modules-matrix` both read, from which init ordering is derived rather than assumed |

Modules are declared in one registration file, in-tree or out-of-tree, and the out-of-tree case
uses the same wire a program uses, pointed the other way. One contract, tested once.

### 5j. Hosted programs: an in-house program with no binary of its own

Most in-house programs are separate binaries that talk to rig over the socket. **Some are
better as plugins**: things we write, program-shaped, that run inside `rigd` rather than as a
process of their own. A hosted program declares exactly what an external one declares, is
projected onto exactly the same surfaces, and passes exactly the same conformance battery from
the same test package. The only difference is where it runs.

That difference is not free, and the plan states the price rather than implying there is none.

| Property | External program | Hosted program |
|---|---|---|
| Upgrades independently of rig | **yes** - the defining maximal (§3) | **no.** Rebuilt with `rigd`, always |
| Crash containment | `kill -9` it; rig never blinks (§18) | A panic must be recovered by the invoker; an unrecovered one takes the daemon with it |
| Capability enforcement | Real: path confinement, network namespace, landlock (§13) | **In-process only.** There is no OS boundary to enforce against, and §13's table says so per program |
| Contribution to the footprint budget | Zero. It is another process | Its entire dependency tree links into `rigd` and lands in the binary-size budget (§17) |
| Who may write one | Anyone whose program speaks the wire, in any language | Us, in Go, at build time. Go has no working dynamic loading (§5h) |

**The rules that make it safe enough to offer:**

- `hosted: true` in the declaration, so every surface, the clients view and `rig doctor` can
  say which programs have no process of their own.
- **Panic to quarantine.** The invoker recovers a panic from a hosted program, quarantines that
  program with the stack and the reason exactly as §18 quarantines a crashing process, and keeps
  serving. A hosted program that panics twice inside the restart budget stays quarantined until
  a manual start.
- **Its own goroutine budget and deadline.** A hosted program is subject to the same rule as any
  call: no work without a deadline, and it can never block a kernel goroutine.
- **A CI check on the binary.** `make bench-size` attributes the delta, so a plugin's
  dependencies cannot quietly spend the daemon's size budget.
- **It is still not business logic in rig.** The lint that bans program ids in rig code applies
  to the kernel, the services and the surfaces - never to a hosted program, which is a program
  and is allowed to know its own name. `make modules-matrix` builds `rigd` with each hosted
  program in and out.

This partly flips §27 assumption 4. Programs stay separate binaries **by default**, and hosting
is a deliberate choice made per program, for small things where an extra process is the larger
cost.

### 5k. Adoption is per service, and coverage is declared

> "When creating an in-house application, obviously, not all functionality will be using rig,
> but everything that is beneficial is going to use it." - Boris, 2026-09-10

rig is a la carte. A program adopts one service at a time, declares only the commands worth
projecting, and pays nothing for what it does not use. This fits the no-imports design exactly:
there is no framework to buy into, no lifecycle to hand over, and no all-or-nothing migration. A
program that uses rig for notifications alone is a legitimate rig program.

**What it stresses, and this is the part the plan was missing.** "Declare once, projected
everywhere" quietly assumed a program declares everything. It will not. So every surface shows a
*partial* view, and a surface that implies completeness lies.

| Consequence | What the plan therefore requires |
|---|---|
| `rig shelf --help` lists 3 commands; shelf has 20 | No surface may imply completeness |
| An agent asks what shelf can do and gets 3 | **The capability map carries coverage per program**, so an agent knows its picture is incomplete before it reasons from it (§9). This is the sharpest one |
| The audit log covers 3 of 20 commands | "One audit log for the estate" carries the caveat, and the coverage log (§15) records it |
| The clients view shows partial activity | §15 states what it cannot see |

- `coverage` is `full` or `partial`, with an optional note. **The default is `partial`**, because
  the honest default is the conservative one.
- A program declares which rig **services** it uses. An unused service is not initialised for it
  and costs it nothing, which is also how §17's laziness rule stops being a hope.
- §3 gains the maximal: a program that adopts exactly one service works and is not degraded for
  adopting one.
- §25's migration order becomes **per service**. "shelf takes config and notifications" is a
  valid milestone, and full migration is not the unit of progress.

### 5l. rig starts with the session, and its unit must not carry an `ExecStop`

An agent cannot call a daemon that was never started. §5g's tolerant client is designed around
rig being *temporarily* absent; it is not an answer to rig being absent because nothing ever
launched it. AgentBox is already started this way, and the estate is about to depend on rig at
least as heavily.

**A `systemd --user` unit, `PartOf=graphical-session.target` and `WantedBy=` the same.** rig
needs the session bus and the display for §12's toasts and §11's tray, so the graphical session
is the right parent rather than `default.target`. `Restart=on-failure`, `RestartSec=2`, so a
crash comes back rather than leaving the estate without its platform.

**Do not give it an `ExecStop`, and this is not a style note.** AgentBox's unit carries the
scar in a comment. Its `ExecStop` was `agentbox quit`; agentbox is single-instance by flock and
auto-spawns on first use, so `agentbox daemon` prints *already running* and **exits 0** when one
is up. systemd sees a `Type=simple` main process exit successfully, considers the service
finished, and runs `ExecStop` - which killed the healthy daemon that was already serving.
Enabling the unit left the desktop with no agentbox at all.

**rig has the identical shape.** §5f is a single-instance `flock`, §5g auto-spawns on the first
client call, and §18's SIGTERM path is the same graceful drain a stop command would use. So the
unit needs no stop verb, and adding one recreates a defect that has already been paid for once
on this machine.

**One wrinkle, inherited and accepted:** start the unit while a daemon is already up and the
unit reports inactive while that daemon keeps serving. At login - the case the unit exists for -
nothing is running yet, so it starts one and stays active.

**It lands at M6, not M15.** M15 keeps the `.deb`, the desktop entry and the signed update
channel. What moves earlier is the twelve-line unit, because every milestone after M6 assumes
rig is up, because §12's toasts and §11's tray are not features of a daemon somebody has to
remember to start, and because §14's `operate` argument is already written against "under M15's
autostart is systemd" - a sentence that is load-bearing several milestones before the milestone
it names.

**AND IT LANDS IN A NEW TOP-LEVEL `packaging/`, ruled 2026-09-12.** This section
specified the unit and said nothing about where the file goes, so a reader with
only the specification could not find it. **Not `cmd/rigd/`:** a `cmd/` tree
builds a binary, and the unit is an artefact the build installs rather than
compiles, so putting it there makes the one directory that means *a Go main
package* also mean *and some data files*. `packaging/` is where M15's `.deb` and
desktop entry arrive, so the unit is not relocated a second time. **The
`COORDINATION.md` ownership row was written before the directory existed**, which
is what stops it becoming the absent-row hole that `tools/`, `cmd/ledger`,
`README.md` and `schema/` each were. `DECISIONS.md` carries the argument.

### 5m. The hand: rig drives the desktop, and that is what makes one lease real

AgentBox can move the pointer, click, drag, scroll and type on Boris's own display, as real
synthetic input that no application can distinguish from a hand on the hardware. It is the only
tool in that daemon that *acts* on the desktop instead of putting something in front of him and
waiting, and it works. What follows is that capability brought over, and the several places
where rig's own model makes it better rather than merely relocated.

**The service is named `hand`, and the verb is `rig hand`.** One syllable, the thing it is, and
it does not collide with `rig window`, which draws.

#### It is a program, not a service inside `rigd`, and the reason is measured

Measured: a Go hello-world at `-trimpath -s -w` is 1,507,488 bytes and the same binary plus
`jezek/xgb` with `xproto` and `xtest` is 2,523,399, so **the X11 dependency is +1,015,911
bytes**. That is 0.97 MB, under §17's 1 MB trip wire, and it would take `rigd` from 39% to 44%
of its budget at M1 of 16.

**The size is the smaller reason.** §17's rule for the window is that the daemon links no
display dependency and stays cgo-free, and the successor backend below is a C library: XTEST is
pure Go so the daemon *could* carry it today, and the day the hand moves a daemon that had
absorbed the driver would need cgo. So `cmd/righand` is a fourth binary, after `rigd`, `rig` and
the window. §2's "two binaries, not one" is about where the terminal stack lives and is not a
cap; `rigwindow` already ships outside that count for this same reason, and **the rule that
generalises out of both cases is that a dependency the daemon must not link gets a process, not
a build tag.** §29's first non-goal holds either way: rig learns that a program called `hand`
declares eleven commands, and no program id appears in its code.

#### And that is what turns §16's most careful sentence from a caveat into an exception

§16 says fencing tokens guard resources rig does not own - "git, a deploy, the VM, the desktop"
- and says it plainly because the unqualified version will be quoted back. This is the
quote-back, and it narrows rather than contradicts.

**The desktop stops being one of those the moment rig owns the only driver that reaches it.**
`righand` binds no socket, is reachable only through an invocation from `rigd`, and is started
by rig or not at all, so every event crosses rig's boundary and an expired lease *refuses* the
next step rather than failing to discourage it. The classic stall - A's lease expires, B
acquires, A wakes and writes anyway - is closed here because A's write comes back through the
process that knows the lease is gone. The token is therefore checked on **every call** rather
than once at acquisition, and the lease is witnessed by `righand`'s own pid, which makes §16's
two-step expiry exact. **It does not generalise**: it works only because the resource is reached
solely through rig, which stays false of git, the deploy and the VM.

#### The four things it decides, and the two it does not

**The design lives in `logbook/projects/rig/hand.md`**, per `logbook/CONVENTION.md`
and the still-open advocate item asking this document to stop growing. The
declaration table, the six carried-over traps with the defect each prevents, and
the backend comparison are all there. What has to be here is what a later reader
cannot re-derive:

- **The contract is a typed array of step objects, and the terse one-step-per-line
  form is a parser in `rig` that produces one.** AgentBox's notation is good and it
  survives as sugar, but a line-oriented string cannot be validated by the daemon,
  generated into `--help`, completed, addressed by a redaction pointer or projected
  onto a surface added later, and §5e gives all five to a schema for free. Same
  shape as M1 slice 5's flag sugar: the schema is real, the notation is convenience.
- **Typed text is declared sensitive, not specially handled.** `sensitive:
  ["/text", "/steps/*/text"]`, compiled to byte spans like anything else (§15).
  AgentBox logs a script's shape and never its text, hand-rolled in the one place
  that needed it; rig already owns the general form.
- **Two commands are not what they look like, and both errors were made here
  first.** `hold` is not `read-only`: acquiring the lease changes nothing in the
  world, but a rule that denies driving while allowing read-only lets a program
  take the display and never give it back, so **gating the drive while leaving the
  grab open gates nothing**. And `windows` returns every window title on screen -
  document names, ticket numbers, browser tabs - which is genuinely read-only and
  is also the shape of the compound leak §31 records, so its **result** carries §15
  pointers.
- **The backend never appears in a declaration.** X11 through XTEST today, which §2
  already licenses; **libei through the XDG Desktop Portal `RemoteDesktop`
  interface** is the named successor, and its consent prompt is an OS-enforced
  version of the strip rather than a window rig draws over its own screen.
  `/dev/uinput` is rejected outright: it is blind to windows, so the window lock
  cannot be built on it, and a backend that cannot say what it is about to type
  into is a different and worse feature. `rig doctor` (§8) is where the backend
  appears by name.

**What it does not decide** is §26 question 12: whether a redaction pointer can
address every element of an array. Question 11, whether `effects` needed a value
of its own for driving input, is decided and shipped - `drives-input`, above
`destructive`, with an `EffectsCeiling` constant guarding the level the invoker
assumes for anything it cannot resolve.

---
