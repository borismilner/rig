# rig

The platform every in-house program runs on. It supplies the infrastructure they all need -
configuration, storage, secrets, logging, tracing, notifications, a window, a tray - and it
controls them: starting, stopping, invoking, scheduling and wiring them to each other.

**No in-house program imports rig.** They talk to it over a socket. That is the whole point: rig
upgrades on its own, and every program gets the improvement without being rebuilt.

The name: a rig is a platform. A rig is also your whole setup. Rigging is the ropes and tackle
that control a ship. And to rig something up is to assemble it. Four meanings, all correct.

Status: planning. Nothing built. Written 2026-09-10.
**Revised 2026-09-10, after a ten-seat adversarial attack on this document.** Every fix that
survived verification is applied; §31 records what changed and what it cost.
Last verified: 2026-09-10 (Go 1.27.1 installed, Wails v3 beta.19, IPC costs measured on this
laptop, see section 4).

---

## 1. The idea, in one line

**An app declares what it can do, once. rig projects that onto every way anyone might reach it.**

```
       shelf    graft    archi   snapper   nudge   grabbit
         └────────┴────────┴────────┴────────┴────────┘
                            │  declares once:
                            │  commands · config schema · state · data · events
                            ▼
                 ┌───────────────────────┐
                 │          rig          │
                 │  one registry, one    │
                 │  truth, one contract  │
                 └───────────────────────┘
                            │  projected onto every surface
   ┌──────┬──────┬──────┬───┴──┬──────┬──────┬──────┬──────┐
  GUI    tray   CLI    MCP    HTTP  palette cron   URL   toast
  pane   item  subcmd  tool   route  entry   job  scheme action
   │      │      │      │      │      │      │      │      │
 window hotkey terminal agents scripts keys  time  links  phone
```

The right-hand row is free. An app writes nothing to get a CLI subcommand, an MCP tool, a tray
entry, a schedulable job or a button inside a toast. It declared its commands; rig did the rest.

**And it compounds.** Add a new surface to rig in 2028 - a voice interface, a phone app, a new
UI - and every program already has it, without being touched.

---

## 2. Decisions locked

- **Name:** rig. CLI `rig`, daemon `rigd`, module `github.com/boris-milner/rig`. The UI shell
  UI shell component is **the window**. `turret` was chosen for it after the lathe turret that
  carries many tools and rotates the right one into place, and dropped on 2026-09-10: the
  product is rig, one name is enough, and a second name for a component only Boris ever opens
  buys nothing. §27 assumption 5 is resolved by this line. The command is `rig window`.
- **Two binaries, not one.** `rigd` is the daemon; `rig` is the CLI and TUI. Measured: keeping
  the terminal stack out of the daemon recovers **8.89 MiB resident and 27 ms of cold start**,
  and it is a build-graph change only (§17). **Minus an unmeasured keyring term**: that rung
  bundled go-keyring with four TUI libraries, and §22 puts go-keyring in the daemon because
  §13's per-program scoping is unenforceable anywhere else.
- **No imports.** An in-house program does not link any rig code beyond a dumb pipe (§5d). All
  behaviour lives in the daemon, so all behaviour upgrades without a rebuild.
- **One socket, binary frames, and no gRPC.** Measured at 6.2µs round trip and 1.8M one-way
  records/second on this laptop (§4), which is far more than anything here needs. A gRPC server
  on a unix socket costs **9.80 MiB resident** before it carries a byte, which is 58% of the
  whole footprint budget for HTTP/2 machinery a local socket does not need. So: protobuf
  messages, hand-framed, length-prefixed. No shared memory, no io_uring, no zero-copy codec.
  That complexity is not bought either.
- **Two directions.** Apps call rig for supply (config, storage, notify, UI). rig calls apps for
  control (start, stop, invoke, query, reload). Both over the same connection.
- **Declarative wins.** Where an app *describes* something, rig can improve it forever. Where an
  app *calls* something, that call is frozen. So the contract is biased hard toward description.
- **Commands declare properties; surfaces declare requirements.** A declaration never names a
  surface. It says what the command *is* - interactive, streaming, needs a display, how long it
  runs, whether it confirms - and each surface says what it can carry. rig computes the
  projection. This is the only construction under which a surface added in 2028 matches a
  declaration written in 2026, and it costs one enum today (§5e).
- **Adoption is per service, and coverage is declared.** A program adopts one service at a time
  and declares only the commands worth projecting. Every declaration carries `coverage`, which
  defaults to `partial`, and every surface renders it, because a surface that implies
  completeness lies (§5k).
- **A program may run inside rig, and pays four prices for it.** Most in-house programs are
  separate binaries. A hosted plugin is compiled into `rigd`, declares and projects identically,
  and gives up independent upgrade, crash containment, OS-level capability enforcement and a
  zero footprint contribution to do it (§5j).
- **The client tolerates rig's absence; it does not reimplement rig.** There is no second
  implementation of config, storage, secrets or logging inside any program. The stub reconnects,
  queues, and returns one typed `unavailable` error. rig writes an already-resolved snapshot to
  disk for the few things that must survive its absence (§5g). This replaces the 300-line
  fallback this plan carried before the attack, which was policy in the one component that can
  never be upgraded.
- **No hot upgrade.** Measured, both paths built: once clients are told rig is going, the
  handover's entire marginal benefit over a plain restart is **1.9 ms per upgrade**, which at
  weekly upgrades is 5.2 seconds a year. It was priced at four defect classes, including a
  silent replay path less safe than `kill -9`. Lifecycle notice plus a sub-100 ms restart
  delivers the requirement instead (§5g, §18).
- **There is an authorization layer.** `house rules` says which kinds of caller may run which
  kinds of command, and which must ask first. It lives in the kernel's invoker, so no surface can
  forget it, and the default rule set carries only the two `url` rules, so nothing already
  working breaks (§13).
- **Operating is a credential, not a uid, and reading is a different credential.** Without the
  first correction every client on a one-user machine satisfies the operator predicate, which
  is exactly how the compound secret leak worked. Without the second, the fix locks Boris's own
  agents out of the estate they exist to read (§14, §15). `introspect` arrives in the
  environment of what he starts; `operate` is **asked for, not carried** (§14).
- **Nothing sensitive is recorded.** Redaction is declared at registration as JSON pointers and
  compiled once into byte spans over the wire encoding: **82.5 ns** in the hot path, against
  3100 ns for redacting at record time. Anything the `secrets` service returns is never recorded
  at all, only the key name (§15).
- **Time is CLOCK_BOOTTIME, and every deadline is absolute.** Lease, budget and expiry arithmetic
  in the daemon runs on boottime, stored as `boot_id + deadline`, never as a remaining TTL and
  never on the client's clock. This laptop suspends nightly and Go's monotonic clock does not
  advance across suspend (§16).
- **Storage: rig manages, the app opens.** rig owns location, migrations, backup, integrity and
  retention; the app opens the file and runs its own queries at full speed. Also an assumption
  in §27.
- **The peers service supersedes AgentBox.** rig builds the best inter-client coordination
  substrate it can (§16), proves it against four gates, and only then do the agents move off
  AgentBox. AgentBox keeps running untouched until that happens. Decided 2026-09-10.
- **rig drives the desktop, and the driver is its own binary.** AgentBox's synthetic input comes
  over as `hand`, a program that declares its commands like any other, in `cmd/righand` and not
  in `rigd` (§5m). It is the **fourth** binary, after `rigd`, `rig` and `rigwindow`, and the line
  above about two binaries is untouched by it: that decision is about keeping the terminal stack
  out of the daemon, not a cap on how many processes rig ships. The test each new one has to pass
  is the window's - a dependency the daemon must not link - and this is the second thing to pass
  it. Measured: the X11 dependency alone is **1,015,911 bytes**, and the successor
  backend is a C library, so a daemon that absorbed the driver would need cgo - which is the same
  argument §17 already makes for keeping the window out. The consequence is the point: because
  `righand` is reachable only through rig, the display becomes the **one lease in this estate a
  fencing token can genuinely fence**, which §16 says of no other and now states as an
  exception.
- **History is buffered and never fsynced.** Losing under a second of history to a power cut is
  acceptable and was agreed; durability is what makes event logging expensive, so it is not
  bought. A process crash loses at most one flush interval, because the kernel already holds the
  rest.
- **An agent Boris runs may read every part of rig.** Introspection is complete for it, not
  scoped: the estate-wide views, the full capability map, any client's history, every trace,
  every config resolution. It is acting for him, and a picture of the estate that is silently
  partial is worse to him than no picture. §14 said the opposite by implication and named the
  contradiction without deciding it - "seven things that sat outside the old six-view list, one
  of which §9 tells an agent to read first". This line decides it. The credential splits in two
  (§14): **reading everything and acting on everything are different grants**, and only the
  first is handed out by default. Decided 2026-09-10.
- **Linux and X11 first**, on this laptop. Nothing knowingly non-portable, and no cross-platform
  claim until it is tested.
- **Dependencies:** best library for the job, newest published version, **and a measured binary
  cost**. Hand-rolling needs a product reason; so does any dependency that moves the size budget
  in §17, because in Go resident memory tracks binary size.

---

## 3. The maximals, made measurable

Boris's brief across this session: maximally expandable, maximally configurable, highly
introspectable, maximally tested, bullet-proof robust, and infrastructure that upgrades
independently of the programs using it. Adjectives do not pass or fail. Each below is a test.

### Upgrades independently - the defining one

| The test it has to pass |
|---|
| Ship a new rig with a redesigned toast, a new settings UI and a new log viewer. No in-house program is rebuilt, and all of them show the new behaviour on next connect |
| A program compiled today against wire v1 still works against a rig built three years from now. Enforced by **one frozen conformance fixture per wire major**, built the day that major ships, archived with its vendored source so it can be rebuilt in 2035, and retained forever. Each fixture asserts a recorded **behaviour transcript** - which default applied, whether a confirm fired, what each enum decoded to - not merely a successful round trip |
| A surface added in 2028 gets a correct projection of a declaration written in 2026, because the declaration never named a surface. Proven by adding a surface whose requirements no registered program was written against, and touching no program |
| Every wire major carries, in this plan, the date it moves from *supported* to *frozen*. `rig doctor` names every connected program still speaking a frozen major |
| Adding a whole new surface to rig (a new UI, a phone client, a voice interface) gives every already-registered program that surface with zero code change. Proven once by adding the HTTP surface after the CLI surface, touching no app |
| The client stub's public surface is enumerated in one file and does not grow without an explicit decision recorded in this plan. Every symbol in it is a future rebuild that cannot be avoided |

### Expandable

| The test it has to pass |
|---|
| A new program registers and becomes fully reachable - CLI, TUI, MCP, tray, HTTP, palette, schedule - with **zero frontend code and no rig-specific logic**. The declaration is data, and it is budgeted per command, not per program: one real archi command written exactly as §5e and §9 require measured **160 lines of JSON**, and archi has 28 operations. So the hand-written budget is under 100 lines of Go, the declaration is generated from the program's existing command definitions, and `rig verify` checks the generated artefact |
| A program that adopts exactly one service works, is not degraded for adopting one, and says so: `coverage: partial` travels with it onto every surface |
| A hosted plugin (§5j) and an external program pass the same conformance battery, from the same test package, with no branch anywhere in the suite |
| Everything the window can do, the terminal can do. Asserted by a test that enumerates the registry's views and fails if one has no TUI renderer |
| Adding a service or a surface to rig gives every already-registered program that capability with no program rebuilt. Proven once by adding HTTP after CLI, touching no app |
| Registration is data, not code. A program written in Python or Rust registers the same way, proven by a reference implementation in each, kept building in CI |
| Nothing in rig mentions a program by name. `make lint` fails on the grep |

### Configurable

| The test it has to pass |
|---|
| Every setting of rig or any program is declared as JSON Schema and the settings UI is generated. There is no hand-written settings form in the codebase |
| `rig config origin <key>` prints the winning layer, its value, and every layer that lost with its value. No setting is ever unexplained |
| Config changes apply live. The program is pushed the new value, validates it, and accepts or rejects with a reason shown in the UI |
| `rig config export` writes one file that reproduces **the caller's** machine exactly, every program it is permitted to see included. Reproducing the whole machine is an estate-wide read and needs `introspect` (§14), which an agent Boris runs holds |

### Introspectable

| The test it has to pass |
|---|
| Every call in both directions is recorded: caller, callee, method, duration, status, size, trace id. Live in the UI and exportable |
| For any program: pid, uptime, restarts with the reason for each, RSS, CPU, wire version, health history, last 1000 log lines, live goroutine dump |
| One trace covers an action end to end across both processes, whichever surface started it - a click, a CLI call, an MCP call from an agent, a scheduled fire |
| `rig doctor` reports the health of the whole installation in one screen and exits non-zero if anything is wrong. Its product is an estate-wide aggregate, so it is a granted surface - **readable with `introspect`**, which an agent holds (§14) |
| Every answer about a program states its coverage. An agent that asks what a program can do is told, in the same answer, whether that list is everything |
| `rig loose-ends` reports what stopped pointing at anything after a declaration changed: schedule entries, bus rules, capability grants, tray entries, promoted tools, `rig://` routes, config overrides |
| No answer drawn from history is given without its coverage. A gap caused by sampling, ring overwrite or segment eviction is rendered, not silently omitted |
| **An agent holding `introspect` can read every part of rig** (§2, §14): the full capability map, every program, every client and its whole history, every trace, every config resolution with its losers, the schedule, the audit log, the notification centre. Asserted positively - a scoped answer where a complete one was owed is a failure |
| **An agent can answer "what is this machine doing right now" in one call, without knowing what to ask.** `query` reaches state, data, logs, traces and history; a whole-estate snapshot is one of its shapes, not nine calls an agent has to know to make |
| **Everything a surface renders, an agent can obtain as data.** No view computes something it does not also return, so an agent is never reduced to reading a screenshot of a screen it cannot query |
| The one thing no reader gets is the thing that was never written: a declared-`sensitive` value, or anything the `secrets` service returned. Absent, not filtered - and the difference is stated in the answer rather than left to look like an empty field |

### Reliable and tested

| The test it has to pass |
|---|
| `kill -9` rig at any instant. Every program keeps running. On restart every program reconnects and no state is lost |
| `kill -9` any program at any instant. rig never blinks, the failure is shown with its reason, and the restart budget governs what happens next |
| A program that hangs, floods, leaks, lies about its schema or ignores cancellation is contained. `fakeapp` does all six on a flag and each has a test |
| No call anywhere is without a deadline. Enforced by a lint rule, not by review |
| Coverage: 90% statements on `internal/`, 100% on the wire contract and the client stub |
| Timing behaviour - backoff, budgets, deadlines, debounce, schedules - is tested with `testing/synctest`, so those tests are deterministic and take microseconds |
| Every UI claim was exercised in a real window with a real keyboard before being called done |
| Two **ungranted** clients on one daemon cannot see each other. Asserted by **observational equivalence over two worlds**: the whole battery runs against W1={A} and W2={A,B} with B exercising every primitive, and A's transcript - responses, error codes, ids and tokens issued, ordering - must be identical. A difference is a failure unless it is on an enumerated, reviewed list of permitted disclosures. The battery covers non-surface observation too: the runtime directory, the config tree, the state tree, the process table |
| The same battery, re-run with A holding `introspect`, asserts the opposite: A sees B **completely**. A view that quietly returns the scoped answer to a principal holding the read grant fails this run, and it is ranked as a defect exactly as high as a leak (§14) |
| No secret reaches the history. `secrets.get` is a call and a call's arguments and results are recorded, so the test asserts a known token appears in no segment, in no encoding, at any sampling rate |
| A caller with no grant cannot run a destructive command. House rules are enforced in the invoker, so one test covers every surface, present and future |
| Idle footprint stays inside the budget in §17. `make bench-idle` fails the build on a regression |
| The daemon binary does not grow by accident. `make bench-size` records `rigd`'s size and fails a PR that adds more than 1 MB unless the budget line in §17 is edited in the same commit. In Go, this is the budget that causes the RSS budget |
| Every module compiles out, and compiles in **alone**. `make modules-matrix` builds kernel-plus-one for every module in turn; `make build-minimal` is the N=0 case, which builds, runs and serves the wire with no service and no surface. Both analyzers run under every tag set CI builds |

---

## 4. Measured: what the wire actually costs

Run on this laptop, 2026-09-10, Go 1.27.1, two real processes. The full harness is in the repo
as `cmd/ipcbench` so the numbers can be re-taken on any machine.

| Transport | Per operation | Rate |
|---|---|---|
| Direct Go function call (what an imported library gives) | 0.6 ns | 1.6G/s |
| Unix socket round trip, 64 B payload | **6.2 µs** | 161k/s |
| Unix socket round trip, 4 KB | 7.8 µs | 128k/s |
| Unix socket round trip, 64 KB | 19.0 µs | 53k/s |
| Round trip with JSON encode and decode, 81 B | 11.4 µs | 87k/s |
| One-way socket write, 256 B log record | **561 ns** | 1.8M/s |
| Shared memory, spin-wait round trip | 143 ns | 7.0M/s |

**What this decides.**

- The daemon architecture costs nothing that matters. A config read at startup, a notification, a
  command invocation, a health check: all of them at 6µs, thousands of times over, is noise.
- **Logging does not need shared memory.** A plain socket write is 561ns and sustains 1.8M
  records per second. Nothing here will produce a thousandth of that.
- **Shared memory is not built.** It is 43x faster and there is no workload that needs it. If one
  ever appears, the number above is what justifies adding it, and not before.
- A binary codec is worth having over JSON (5µs of the 11.4µs is the codec), but it is a
  preference, not a requirement.
- **gRPC is not bought.** Its server on a unix socket measured **+9.80 MiB resident** for
  HTTP/2 framing, flow control and stream machinery that a single local socket does not need.
  The framing this plan actually requires is a length prefix, and §17 is where that 9.80 MiB
  went. Protobuf stays; gRPC goes.

---

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

**Exactly one `rigd` per user, and it is enforced rather than assumed.** `rigd` takes an
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

## 6. Configuration

Layers, lowest to highest, with the winner recorded per key:

```
  built-in defaults
  < /etc/rig/rig.toml
  < ~/.config/rig/rig.toml
  < the program's declared defaults
  < ~/.config/rig/apps/<id>.toml
  < environment (RIG_*)
  < command-line flags
  < a runtime override set in the UI
```

- Every key is schema-declared, so the settings UI is generated. No hand-written forms.
- `rig config origin <key>` prints the winner and every loser, with file and line.
- Changes apply live: rig validates, pushes, and shows accepted, rejected with the program's own
  reason, or needs-restart with a one-click restart.
- A change set is validated whole before any of it is sent, so nothing lands half-applied.
- `rig config export` and `rig config diff` reproduce and compare **the caller's** machine.
  Reproducing the whole machine is an estate-wide read and therefore needs `introspect`
  (§14), which an agent Boris runs holds.
- **Every resolution is written to a snapshot on disk**, already merged, with provenance, the
  database path assigned and the schema version migrated to. That file is what a program reads
  when rig is unreachable (§5g), and it is the reason there is exactly one implementation of
  config resolution.
- **A renamed key reports its losers.** `rig loose-ends` (§8) names every override that stopped
  applying, because the silent version of this bug is invisible to `rig config origin`: the
  override does not lose, it simply no longer exists under that spelling.

**The visual system is configuration, not code.** Faces, base size, type scale, tracking, line
height, corner radius, density, the hue family's lightness, chroma and rotation, the neutral
surface ladder and whether anything animates are all declared under `ui.theme` as JSON Schema,
layered and live-pushed like every other setting. `design/theme.js` is the reference
implementation and `design/visual-system.html` is the live editor over it; the export box there
emits exactly this fragment.

Two rules travel with it, because a theme is the one setting a user can break the product with:

- **The hues are generated in oklch**, six evenly spaced at one lightness and one chroma, so a
  family stays a family when any parameter moves.
- **The neutrals are solved, not chosen.** `--border`, `--fg-dim` and `--fg-faint` are searched
  to the dimmest value that still clears their WCAG target on *every* surface they can land on.
  rig refuses to apply a token set that fails, naming the token and the ground, so a theme
  cannot silently produce an unreadable product.

**`ui.theme`, as schema.** This is the whole surface - there is no second set of knobs hidden in
code, and `design/theme.js` implements exactly this object.

```jsonc
{ "$id": "rig://schema/ui.theme", "type": "object", "additionalProperties": false,
  "properties": {
    "faces":  { "type": "object", "additionalProperties": false, "properties": {
      "display": {"type":"string"}, "ui": {"type":"string"}, "mono": {"type":"string"} } },
    "type":   { "type": "object", "additionalProperties": false, "properties": {
      "base":       {"type":"number","minimum":12,"maximum":22,"default":16,"unit":"px"},
      "scale":      {"type":"number","minimum":1.10,"maximum":1.45,"default":1.26},
      "lineHeight": {"type":"number","minimum":1.2,"maximum":1.9,"default":1.55},
      "uiTight":    {"type":"number","minimum":-0.04,"maximum":0.02,"default":-0.011,"unit":"em"},
      "dispTight":  {"type":"number","minimum":-0.06,"maximum":0.02,"default":-0.024,"unit":"em"} } },
    "shape":  { "type": "object", "additionalProperties": false, "properties": {
      "radius":  {"type":"number","minimum":0,"maximum":26,"default":12,"unit":"px"},
      "density": {"type":"number","minimum":0.7,"maximum":1.4,"default":1.0},
      "gut":     {"type":"number","minimum":0.8,"maximum":3.0,"default":1.6,"unit":"rem"} } },
    "hues":   { "type": "object", "additionalProperties": false, "properties": {
      "members": { "type":"array","minItems":6,"maxItems":9,"items": {
        "type":"object","required":["name","angle","role","identity","anchored"],
        "properties": {
          "name":     {"type":"string","pattern":"^[a-z][a-z0-9-]{1,15}$"},
          "angle":    {"type":"number","minimum":0,"exclusiveMaximum":360},
          "role":     {"enum":["bad","warn","good","progress","info","-"]},
          "identity": {"type":"boolean","description":"false = no program may own it"},
          "anchored": {"type":"boolean","description":"true = the optimiser may not move it"} } } },
      "rotate": {"type":"number","minimum":-180,"maximum":180,"default":0},
      "dark":   {"$ref":"#/$defs/lc","default":{"L":0.800,"C":0.098}},
      "light":  {"$ref":"#/$defs/lc","default":{"L":0.470,"C":0.110}} } },
    "surfaces": { "type":"object","additionalProperties":false,"properties": {
      "hue":    {"type":"number","minimum":0,"exclusiveMaximum":360,"default":252},
      "chroma": {"type":"number","minimum":0,"maximum":0.06,"default":0.022},
      "dark":   {"$ref":"#/$defs/ladder"}, "light": {"$ref":"#/$defs/ladder"} } },
    "motion": {"type":"boolean","default":true,
               "description":"forced false under prefers-reduced-motion, never the other way"} },
  "$defs": {
    "lc":     {"type":"object","additionalProperties":false,"properties":{
                 "L":{"type":"number","minimum":0.2,"maximum":0.95},
                 "C":{"type":"number","minimum":0,"maximum":0.22}}},
    "ladder": {"type":"object","additionalProperties":false,
               "required":["bg","bg2","panel","glow","tint","fg"],
               "description":"oklch L per surface. Every surface is its own knob: one step cannot express a light theme, where panel goes UP toward white and the recessed grounds go DOWN away from it",
               "properties":{"bg":{"type":"number"},"bg2":{"type":"number"},
                 "panel":{"type":"number"},"glow":{"type":"number"},
                 "tint":{"type":"number"},"fg":{"type":"number"}}} } }
```

**Three things the schema cannot express, and rig enforces them after validation:**

1. `--border`, `--border-2`, `--fg-dim` and `--fg-faint` are **not in the schema at all**. They
   are solved from the ladder, so there is no spelling of a failing border.
2. **Text tokens are solved against every surface text lands on, `tint` included; boundary
   tokens against the four a boundary sits on.** The two sets are different, and solving a text
   token against the wrong one is how a 3.95:1 reaches a page that audits clean.
3. Rejection names the token *and the ground it failed on*, because "contrast too low" without
   the ground is not actionable.

---

## 7. Storage

rig owns everything about a database except what is in it.

| rig owns | The program owns |
|---|---|
| Where the file lives, and telling the program | Its schema |
| Running migrations before the program starts | Writing the migrations |
| Backup, restore, retention, integrity check | Its queries and transactions |
| A browsable viewer in the UI | Its own indexes |
| Reporting size, growth and last backup | |

The program opens the file directly, so a query costs what SQLite costs and not what a socket
costs. The tedious half - migration running, backup, integrity, retention - upgrades
independently, which is the half that is copy-pasted fifteen times today.

Nothing may hold the only copy of anything in SQLite. Files on disk are the record; the database
is an index and is rebuildable. **That rule binds rig's own storage too**, and §15 breaks it
unless the history's interning dictionary travels inside the segments rather than in the index -
see §15, where the plan's previous advice to delete the index and rescan destroyed data.

### rig backs itself up, or the rule above is a rule for other programs

The row two tables up sells backup, restore, retention and integrity to fifteen programs, and
until this section nothing said what happens to rig's own state. Two of the things it holds
are records **of record** with no source to rebuild from - the audit log and the notification
centre - so for those the rule above is not merely unmet, it is unmeetable by rescanning.

| rig's own state | Rebuildable from | Backed up |
|---|---|---|
| Config tree | nothing - it is hand-edited source | **yes**, and it is the one thing whose loss cannot be worked around |
| History segments | nothing, but they age out in 30 days anyway | no. Retention already says they are disposable |
| **Audit log** | nothing. It is the record | **yes** |
| **Notification centre** | nothing. §12 makes it the record of record | **yes** |
| Registry, peers WAL, resolved snapshots | live registration and re-resolution | no. Rebuilt at start |
| A program's database | the program's own files, per the rule above | already covered, per program |

**One command, and it is the same one programs get.** `rig backup` writes the four rows marked
yes to one archive; `rig restore` puts them back on a machine where `rigd` has never run. That
is the missing half of M15's "fresh machine to a working rig in one command" - which as
written installs the software and carries none of the history. **M11**, beside the per-program
backup it already ships, because building it twice is the alternative.

---

## 8. Observability

rig is where "highly introspectable" is actually delivered, and it is delivered once for
everything rather than fifteen times badly.

| View | Shows |
|---|---|
| Programs | Every one: pid, uptime, restarts with reasons, RSS, CPU, wire version, health history |
| Calls | Live call log both directions: method, duration, status, size, trace id, expandable payload |
| Traces | One action as a span tree across processes, whatever surface started it. Exports OTLP |
| Logs | Merged, one colour per program, level and text filters, follow, jump to the trace |
| Events | A live tap on the bus: publisher, topic, payload, who received, who was denied |
| Config | Every key, the winning layer and every losing value. Searchable |
| Schedule | What is due, what fired, what failed, what it cost |
| Audit | Every action taken anywhere, by whom, through which surface. One log for the estate, with its coverage stated (§5k) |
| Loose ends | What stopped pointing at anything after a declaration changed. See below |
| Doctor | Versions, drift, orphaned registrations, quarantined programs, capability warnings, disk, permissions, stub build vintages, programs on a frozen wire major |

Every view above states its coverage, and none of them answers a question about history without
saying what it could not see (§15).

### `rig doctor` names every dependency, including the ones that are missing

The Doctor row above is what rig can see about *itself*. It says nothing about what rig needs
**from the machine**, and rig degrades against roughly a dozen external things that are
individually optional and collectively the difference between a working estate and a puzzling
one.

**A degradation nobody can see is the failure mode.** §5g already promises a capability can be
`unavailable` with a reason that `rig doctor` can see; §12 already promises `rig doctor` says
whether the frameless toast or the freedesktop fallback is live. Those are two instances of a
rule that was never stated once, and stating it once is what makes the rest of the plan's
"degrades gracefully" claims checkable.

**Three classes, and the distinction is what makes the output usable:**

| Class | If it is missing | Examples |
|---|---|---|
| **Required** | rig refuses to start and names which one | `$XDG_RUNTIME_DIR`, a writable socket directory, the store path |
| **Optional** | a **named** feature degrades to a **named** fallback | speech engine, audio player, keyring, a compositor with an ARGB visual |
| **Build-time** | nothing at runtime; reported only under `--dev` | `protoc`, `wails3`, node, Chrome for the contrast gate |

**Every optional row prints four things:** what is missing, what stops working, what happens
instead, and the one command that installs it. `keyring: not found` has told a person less than
they knew before they ran it.

**Kernel facilities are dependencies too, and they were being assumed.** Each was probed on this
laptop on 2026-09-10 and each gets a row, because each is a silent absence rather than a loud
one:

| Facility | Used by | Here |
|---|---|---|
| PSI read side | `disk.pressure`, `memory.pressure` (§5h) | present |
| PSI **triggers** | nothing, deliberately | **rejected, `EINVAL`** - which is why rig polls (§5h) |
| `pidfd_open` | `process.exited` (§5h) | present |
| `PIDFD_INFO_EXIT` | would give a non-child's exit code | **absent** - which is why the code is `unknown` (§5h) |
| inotify `max_user_watches` | `fs.changed` (§5h) | 524288 |
| inotify `max_queued_events` | `fs.changed` (§5h) | **16384**, which one `git checkout` overruns |

**`rig doctor --json` carries the same content**, because an agent asking why a capability is
missing needs the answer in a form it can act on, and §14 already grants `introspect` for
exactly that.

**The check ships with `doctor` at M5, and every later milestone adds its own rows rather than
a second checker**: M8 the compositor and the tray, M9 the speech engine and the audio player,
M11 the keyring. A milestone that introduces an optional dependency and no doctor row has not
finished.

### Loose ends

When a declaration changes, rig reports what in the rest of the estate stopped pointing at
anything: schedule entries, bus rules, capability grants, tray entries, promoted MCP tools,
`rig://` routes, config overrides. `rig loose-ends` on the CLI, a view in the window and the
TUI, and a line in `rig doctor`.

It is cheap - the registry already holds both sides - and it catches a real bug class the rest
of the plan cannot. A renamed config key does not make its override *lose*; the override simply
stops existing under that spelling, so `rig config origin` shows no loser and the setting
silently reverts to a default.

### What a recorded call actually costs

§15's budget covers the history append alone. This section mandates two more recordings per
call, and the plan used to contradict itself about the total:

| Recording | Measured | Mandated by |
|---|---|---|
| History append, one writer | 110.8 ns | §15 |
| OpenTelemetry span | 1019 ns, and a *rejected* span still costs 195.6 ns | this section |
| `slog` record | 627 ns | this section |
| **Total per call** | **1757 ns** | 28% on top of §4's 6.2µs wire |

The budget in §15 is stated against that total, not against one third of it. Sampling does not
rescue the span, because a rejected span is not free.

---

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

## 10. The terminal client

Everything rig can do is reachable from a terminal, and the terminal is not the poor relation of
the window. A headless server, an SSH session and a container get the whole product.

**Three faces, because scripts, humans and agents want different things.**

| Face | Shape | For |
|---|---|---|
| `rig <app> <cmd>` | One shot, no interaction, exit codes, `--json` on everything | Scripts, Makefiles, cron, agents |
| `rig shell` | Line-oriented, completion, history, inline `describe`, `?` help | A human exploring, fast |
| `rig tui` | Full screen, panes, live tails, generated forms | A human working, or watching |

### `rig tui`

Mirrors every view the window has, because both are surfaces reading the same registry. Same
data, different renderer.

```
┌ rig ───────────────────────────────────────────────── 6 up  0 degraded ┐
│ ● shelf     │ shelf                                      ● ok  1.2.0   │
│ ● graft     ├────────────────────────────────────────────────────────  │
│ ▲ dispatch  │ commands   reindex · search · stats · gc                 │
│ ● archi     │ config     8 keys, 2 overridden                          │
│ ○ snapper   │ storage    412 MB · migrated 19 · backed up 2h ago       │
│ ● nudge     │ health     ok, checked 4s ago                            │
│             │                                                          │
│ [1] progs   │ 14:22:03 shelf  index  scanned 1,204 files               │
│ [2] logs    │ 14:22:03 graft  run    nightly-audit started             │
│ [3] calls   │ 14:22:04 shelf  index  done in 812ms                     │
│ [4] traces  │                                                          │
│ [5] config  ├────────────────────────────────────────────────────────  │
│ [6] sched   │ :  invoke shelf.reindex --since 7d                       │
└─────────────┴──────────────────────────────────── ? help   q quit ─────┘
```

### The forms come from the same schema as the window

A command's argument schema renders as a Svelte form in the window and as a `huh` form in the
terminal. One declaration, two renderers, and a program that adds an argument gets both. This is
the renderer architecture from §5h proving itself on its first real case.

### What agents actually use

Being honest about which face serves whom: **a full-screen TUI is hostile to an agent** - ANSI
escapes, redraws, cursor state. So the agent affordance in the terminal is not the TUI, it is:

- `--json` on **every** command, every view and every error, returning exactly what the MCP tool
  returns
- `rig describe <anything>` with the full declaration, examples and effects
- Structured errors that name the failed precondition and the command that fixes it
- `rig shell --batch --json`, which reads commands on stdin and writes one JSON object per line,
  for a scripted session that needs state

An agent that only knows **list**, **describe** and **invoke** can operate the entire estate.

**Those are the MCP TOOL names, and this line used to write them as CLI verbs -
`rig --json list` and so on - which do not exist.** Corrected 2026-09-11, found
by typing the line rather than by reading it. `rig list` today dispatches as a
program named `list`; there is no `describe` and no `invoke` verb at all.

**The CLI spelling and the MCP tool name are DIFFERENT SURFACES OF ONE
OPERATION, and §10's rule is about the OBJECT, not the spelling.** *"`--json`
returns exactly what the MCP tool returns"* constrains what comes back, and
deliberately does not require the two surfaces to be typed the same way - an MCP
tool name is a flat identifier in a tool list, and a CLI is a verb over a
namespaced estate.

| Operation | MCP tool | CLI today | CLI when M2 lands |
|---|---|---|---|
| enumerate the estate | `list` | `rig apps list` | `rig apps list` |
| one thing in full | `describe` | **does not exist** | `rig describe <anything>`, above |
| run a command | `invoke` | `rig <app> <cmd>` | `rig <app> <cmd>` |

**`invoke` is the one worth noticing: the CLI has had it since M1 and it is not
spelled like a verb at all**, because `rig shelf reindex` is the whole point of
the front door. An agent reading this plan for CLI syntax would have gone
looking for a command that was never going to exist.

**These three are M2 slices 2 and 3**, so the surfaces arrive together and the
table's last column is a commitment rather than a description.

### rig's own failures carry a code, and it cannot collide with the daemon's

**`--json` on every error means EVERY error, including the ones the daemon never
saw.** rigd unreachable, argv that did not parse, a client-side timeout: these
returned prose while every daemon refusal returned an object, which is the
defect §10's own rule exists to prevent. **The error most worth structuring is
"rigd is not running", because it is the one an agent is likeliest to meet
first.**

**Two vocabularies, disjoint by construction rather than by care.** Every code
the daemon can send is a value of the wire `Code` enum and is spelled `CODE_*`.
rig's own are spelled `RIG_*`. **So the sets cannot collide however many codes
either side adds later, and neither side has to check** - which is the
difference between a convention and a mechanism.

| Code | Means |
|---|---|
| `RIG_NO_DAEMON` | rig could not reach rigd at all. **Carries the socket path as `actual` and the best `fix_command` rig owns** |
| `RIG_NO_SUCH_PROGRAM` | the registry rig read does not name that program. **No `fix_command`**: rig cannot start a program, and §9 would rather say nothing than print a line that does not run |
| `RIG_NO_SUCH_COMMAND` | that program's declaration does not name that command. **Carries coverage in `actual`** (§5k), because "it declares ping, purge, reindex, and its coverage is partial" is what tells a caller whether to look again or look elsewhere |
| `RIG_BAD_ARGUMENT` | argv did not parse, or contradicted itself, before anything left rig |
| `RIG_TIMEOUT` | rig stopped waiting, **and the call may still be running** |
| `RIG_BAD_RESULT` | an answer arrived that the declaration did not promise |

**`RIG_TIMEOUT` is deliberately NOT the daemon's deadline code, and collapsing
them would lose the distinction that matters.** A daemon deadline says the call
missed the deadline the daemon supervises. `RIG_TIMEOUT` says the client ran out
of patience **and the work may still be in flight.** For an agent deciding
whether to retry, those are opposite facts, and a retry against the first is
safe where a retry against the second is a duplicate execution.

### The rule that keeps the terminal first-class

**A view ships in the TUI when its data lands.** Once the window exists at M1a, no view ships in
one without the other. The TUI is written as a surface plugin like any other, so it costs one
module rather than a parallel product.

**And the claim is scoped, because the unscoped version is false.** "Everything the window can
do, the terminal can do" holds for **generated** panes - the ones rig draws from a declared
schema - and cannot hold for embedded ones. archi's pane is an iframe over a hand-built SVG
canvas; there is no TUI renderer for it and there can be none, and writing one would put archi's
business logic inside rig, which §5h calls a bug. Six of eleven programs in §25 are tier-two.

So the test in §3 asserts: **every generated view has a TUI renderer, and every pane a program
draws itself declares a terminal fallback** - the commands, state and data behind it, reachable
and runnable from the terminal even though the canvas is not drawable there. A program that
offers such a pane and no fallback fails `rig verify`. **That covers the kit tier as well as
the embedded one**, and it has to say so: §11 has three tiers, this test used to name two, and
a kit element cannot carry a fallback of its own because the fallback is per pane and only the
program knows what is behind it (§5h R2).

---

## 11. The window

The UI shell inside rig. Its visual system is a separate piece of work (§23 M1a) because for a
program whose whole job is presenting other programs, the visual design *is* the product. It is
built and measured: `design/visual-system.html`, engine at `design/theme.js`.

- **One window.** A left rail of registered programs, a context bar, a pane, a status strip.
  Chrome budget is a number that gets defended, not a feeling.
- **One tray icon** for the whole estate, replacing the six that exist today. Per-program status,
  badge, and commands runnable with no window open. A stopped program is still listed, with the
  reason and a Start entry.
- **Three pane tiers**, and the middle one exists because the outer two leave a gap.
  *Generated*: the program declared a schema and rig renders forms, tables, actions, progress,
  detail and status - and the program never names a widget (§5h). *Kit*: the program serves its
  own HTML and composes it from rig's own elements. *Embedded*: the program serves its own HTML
  and brings its own components, getting the token set and nothing more.
- **The gap the kit closes, and the version of this claim that was measured and found false.**
  The tier exists because a program serving its own HTML gets tokens and stops there, so one
  visual system cannot reach it. What this section used to say - that `archi`, `dispatch` and
  `snapper` each rebuild a table, a toolbar, an empty state and a spinner and get all four
  subtly wrong - is not true of any of them (§5h). **The kit's adopters are the fake
  applications of §23 until a real program asks for it.**
- **The shell is achromatic.** `design/theme.js` ships **seven** hues of which **five** are
  ownable, and a program with no identity hue stays achromatic, which is what the shell is
  anyway. The owned hue is the only saturated colour on screen while you are in it. A host that
  wears the colour of whatever it is holding.
- **Healthy is the absence of colour.** Six programs, most idle, most of the time, so `ok` is
  drawn as a hollow tick and a hue appears only when something wants you. A stopped program
  keeps its place and goes dashed, so the rail never re-orders under your hand.
- **The window survives a daemon restart.** It is a separate process (§17), which is what makes
  the lifecycle notice in §5g drawable at all: the tray gains a fourth state, **detached**,
  meaning the window is up and `rigd` is not.
- **The visual system is config** (§6). Faces, sizes, scale, radius, density, the hue family's
  parameters and whether anything animates are declared, layered and live-pushed, and rig
  refuses a token set that fails its contrast targets. Built and measured:
  `design/visual-system.html`, with `design/theme.js` as the engine.

## 12. Toasts

A notification is a designed surface, not a line handed to the desktop. Frameless, always-on-top,
transparent webview windows rig owns and styles. X11 with GNOME Shell as compositor is confirmed
present on this laptop, so an ARGB visual and a real backdrop blur are available.

- Full CSS and the theme tokens, spring physics, live-updating bodies, syntax-highlighted code,
  sparklines, images, markdown.
- **Inline actions wired to the program's own commands.** Answering does not open a window.
- Stacking with collapse; hover fans the deck out.
- Never steals focus, never eats a click meant for what is underneath.
- Dwell scales with reading time: roughly 10s plus 5s per line. Hover pauses.
- **Nothing is ever only a toast.** Everything lands in a notification centre, searchable, with
  the action still live. That is what makes auto-dismiss safe.
- Do Not Disturb and a fullscreen-focus rule. Suppressed toasts go to the centre, never dropped.
- **Do Not Disturb never suppresses an `ask` that gates a command** (§13a, §14). It suppresses
  notices; a question the caller is blocked on is not one. Stated here as well as in §14
  because whoever builds this surface reads this section, and a conformance item asserts it
  rather than leaving it as prose.
- Falls back to `org.freedesktop.Notifications` where the frameless window is unavailable, and
  `rig doctor` says which is live.
- **The notification centre is a service, not part of this surface** (§5h). It is the record of
  record, so `make build-minimal` must not be able to drop it, and the window, the TUI and the
  CLI all read the same one.

### Speech: the same notification, said out loud

> "AgentBox has the `say` functionality that uses kokoro-say. I want to take it to the optimum
> and make it a feature in rig." - Boris, 2026-09-10

**Speech is a renderer on the notification centre, not a new surface.** The `voice` box in
§5h's diagram is an *input* surface - reaching the estate by talking to it - and this is the
opposite direction. Putting speech on the centre is what makes it free: every program that
already sends a notification can be heard without being rebuilt, which is §1's compounding
claim again, and a program that wants to be heard rather than seen sets one field.

**An item speaks only if it carries a `speak` line.** Never the title, never the body, never a
heuristic that shortens one into the other. That rule is AgentBox's and it is why its speech is
usable rather than exhausting: what gets said is something a program wrote *in order to be
said*, and keeping "worth saying" the program's judgement is the whole reason it works.

**The engine contract is a long-lived process, and that is the design rather than an
optimisation.** One line of UTF-8 per utterance on stdin, raw little-endian 16-bit mono PCM on
stdout, and the process stays open between lines. Measured with kokoro-82M on this machine:
**loading the model costs ~3s and an utterance ~375ms**, so a process per sentence puts every
notification seconds behind the thing it is about. rig holds the engine open and pipes it into
a player.

| Piece | Rule |
|---|---|
| Engine | Resolved from config, `kokoro-say` by default. Anything satisfying the stdin/PCM contract works, which is how piper stays a valid answer |
| Player | `pw-play`, `paplay`, `aplay`, `play`, in that order, resolved at startup |
| cgo | **None**, and the daemon holds no audio device open (§22) |
| Residency | A voice model is ~100 MB resident. The pipeline is released after a quiet spell and rebuilt on demand, and **M9 adds its row to §17's table** rather than leaving it off the estate's all-day cost |
| Queue | Bounded, oldest dropped past the bound. Twenty notifications at once must not become a two-minute monologue still reading out the first |
| Do Not Disturb | Suppresses speech exactly as it suppresses a toast - **and never suppresses an `ask` that gates a command** (§14). Same conformance assertion, one line lower |
| Rate | 24 kHz for kokoro against piper's 22.05 kHz. The engine reports its own rate; no caller hard-codes one |
| Missing engine or player | A named degradation in `rig doctor` (§8), never an error. rig without a voice still toasts |

**Where "the optimum" is more than a port.** Each of these is something the centre gives it
that a standalone `say` binary cannot:

- **One queue for the estate.** Fifteen programs speaking through one arbiter cannot talk over
  each other. Today each caller races every other caller for the same sound device.
- **Severity and Do Not Disturb are already decided** for the toast. Speech inherits that
  decision instead of growing a second, differently-wrong copy of it.
- **Everything spoken is in the centre**, searchable, with its action still live. §12's
  "nothing is ever only a toast" applies unchanged, and it is what makes a missed utterance
  recoverable - which speech needs more than toasts do, because there is no scrollback for
  sound.
- **It is auditable**: what was said, on whose behalf, through which surface, in the one log.
- **Voice, speed and language are declared config keys** (§6), so they are generated into the
  settings UI and pushed live like everything else, rather than being three environment
  variables a person has to know about.

**It lands at M9, with the centre it renders, and it cannot land earlier** - there is nothing
to be a renderer *of* until M9. That also satisfies the sequencing that was asked for: M6 has
rig starting with the session (§5l), M9 has it speaking, and neither is a special case built to
make the ordering work.

**The AgentBox cutover is M16 and is not part of M9.** Repointing every agent's instructions
from AgentBox's `speak` to rig's is a change to a dozen instruction files and an estate-wide
behaviour change, so it belongs with the rest of the AgentBox supersession - after rig's speech
has survived M12's pilot week of daily use. Shipping the renderer and repointing the estate are
two decisions, and collapsing them is how the second one gets made by accident.

---

## 13. Capabilities and trust

Deny by default. A program declares what it needs at registration, rig enforces it, and every
denial is logged with a trace id and shown.

| Capability | Enforced by |
|---|---|
| `secrets` | Keyring reads scoped to the named keys, namespaced per program |
| `storage` | A path the program cannot escape |
| `network` | An allowlist, via a network namespace where available, otherwise at the proxy and honestly labelled best-effort |
| `filesystem` | Declared paths only, mode-checked, landlock where the kernel supports it |
| `subscribe` | The bus refuses a subscription without a grant |
| module-contributed | Every service and surface registers the capability names it grants (§5h). The kernel checks a grant against a set it never had to know in advance - the previous enum literally listed `tray`, `notify`, `schedule`, three module names inside the kernel |

Capabilities are enforced where enforcement is real and labelled best-effort where it is not; the
UI says which per program. Widening a capability needs an explicit confirmation, so an edited
manifest cannot quietly gain access. **Widening `sensitive` needs the same confirmation**, since
after §15 that declaration is security-relevant.

**A hosted program (§5j) gets in-process enforcement only.** Path confinement, network
namespaces and landlock are per-process instruments and there is no second process. The Programs
view says so per program rather than implying a boundary that is not there.

### 13a. house rules - the authorization floor

**There is no authorization language anywhere else in this plan.** rig carries `effects` and a
danger level from the registry, and the principal from the connection, and before this section
it never put the two together. Its answer used to be §9's "an agent can refuse or confirm on its
own" - which is the restraint living inside the thing being restrained.

`house rules` is a small table: which kinds of caller may run which kinds of command, and which
must ask first.

```
[[rules]]
caller  = "agent"           # agent | terminal | window | script | program
                            # | schedule | bus | url | any
effects = "destructive"     # at least as dangerous as this, per the declared property
action  = "confirm"         # allow | confirm | deny
```

**`effects` in a rule is a floor, not an equality.** It matches a command whose declared
effects are **at least as dangerous** as the value named, so a rule written against
`writes-files` covers `destructive` too. Equality was the first reading and it cannot express
the one default this section ships: a rule naming `read-only` would match read-only calls and
nothing else, so the dangerous call - the only call the rule exists for - falls through to the
empty set and is allowed. The kernel already orders the enum, for exactly this.

**When more than one rule matches, the most restrictive wins: `deny` over `confirm` over
`allow`.** The table needs this the day it ships, because the `url` default below is two rules
that both match the same call. First-match makes the outcome depend on the order lines happen
to sit in a file, and most-specific makes a narrow `allow` beat a broad `deny`; both fail open,
and this is the authorization floor.

**The four clauses below were unanswered until 2026-09-11**, and the fourth is
an authority hole that was **demonstrated in this tree, not inferred from a term
sweep.**

| Clause | What it requires |
|---|---|
| **Evaluated on the exact effective arguments that will execute** | with no re-parsing between the decision and the dispatch. §13a matches on the pair `(caller, effects)` and arguments were never matched at all. A decision taken against one argument set and dispatched with another has authorised something nobody decided |
| **Evaluation cannot error and cannot block, and any failure is `deny`** | a rules table that throws is a table that fails open. This is the same reasoning that refused first-match and most-specific above, applied to the evaluator rather than to the ordering |
| **A decision names its matching rule, its config layer, and the remedy** | `origin` carries `rule`/`elevation` and the rule id today. **The layer and the remedy are not recorded**, so an operator reading a refusal cannot tell which file to edit |

#### A program must not be able to weaken its own effects unobserved

**`effects` is declared by the program, and house rules match on that
declaration.** So a rule written as `(agent, destructive) -> confirm` stops
matching the moment that command presents itself as `writes-files`. **No rule is
violated and no denial is logged**, because the pair simply stopped matching.
`effects` being *"a floor, not an equality"* widens it: one step down the enum
drops every rule written at or above the old level.

**The reachable path is a RECONNECT, and this was corrected on 2026-09-11 after
being got wrong.** The first statement of this hole said a live session
re-registers and the entry is replaced. **That is true of the kernel and not
reachable from the daemon**, which refuses a second handshake on a live
connection outright. The path that actually exists is ordinary: the connection
closes, the close deregisters the program, and it reconnects declaring the same
command one level weaker. **A deploy is enough. No hostile program and no race.**

**That makes it harder to close, not easier**, and the difficulty is the
specification: the old declaration is gone by the time the new one arrives, so
**closing this means remembering a declaration past the connection that made
it.** A comparison at registration time has nothing to compare against unless
rig keeps the prior declaration deliberately.

**The observability half is separate, and nothing covers it today.** rig logs
**decisions**, not changes of declaration. There is no event class for *"the
pair this command presents has changed"*, so an operator watching for refusals
sees only a call that was allowed. The restart appears in the log as a
deregistration and a fresh registration, and **neither line says that a rule
which had been gating that command no longer reaches it.**

**Both halves are required.** Detecting the weakening without emitting an event
leaves the operator blind; emitting the event without keeping the prior
declaration leaves nothing to emit.

#### No path to invocation may drop the caller, and a signature that omits it is the defect

**§13a's floor lives in the kernel's invoker for one stated reason: "no surface
can forget it, and a surface added in 2028 is covered by rules written in
2026."** That guarantee is structural, and it holds only while every path to an
invocation still carries a principal when it arrives.

**Found 2026-09-11 while building M2's meta layer: an internal interface between
the meta tools and the daemon took the program, the command and the arguments -
and no principal.** The layer above it had one and used it for a visibility
check, then dropped it. **So whatever implemented that interface was handed
nothing to authorise with.** The floor was not bypassed; it was made
*unreachable* by a type signature.

**This is the sharpest form of the defect §13a exists to prevent**, and it is
worth stating as its own rule because it does not look like a security bug from
either side: the caller believed it had authorised, the implementer had nothing
to authorise with, and no rule was violated because no rule was consulted.

| The rule | |
|---|---|
| **Every function on a path to invocation carries the principal.** A signature that cannot express the caller is a defect in the signature | An interface whose shape makes forgetting compulsory defeats a floor that exists so nobody has to remember |
| **One authorisation path, not one per surface.** A new surface SPLITS the existing path and reuses its floor; it never grows a second one | Two floors is how they diverge, and the one that diverges is whichever was added last |
| **A surface that mints a principal is walked against §14's caller table as part of the change** | already §14's rule, and this is the case it was written for |

**Why it was not exploitable when it was found, stated so the fix is not
mis-timed:** at M2 the only caller through that layer connects over the socket
as an ordinary unregistered client and gets everything §14's table grants such a
client. **One principal, no scoping, no gap.**

**The fuse is the HTTP surface.** §14 specifies an HTTP client as *"its bearer
principal's scopes, never `introspect`"* - a genuinely narrower principal, and
the first one that would reach an invocation with its scope dropped. **So this
is settled BEFORE that surface is built, not during it**, which is what §14's
walk-the-table rule already requires.

#### The confirmation channel is out of band from the caller

**§14 and §13a route `confirm` to a window, toast or terminal and bind the
answer to "that one call, not the connection". That is the hard half and it is
right.** The clauses below are the rest, and each was checked against the client
surface on 2026-09-11 rather than assumed.

| Clause | State today |
|---|---|
| **No argument, flag, header or second call can satisfy a confirmation** | see the four channels below. **Three exist unreserved and one does not exist at all** |
| **Deny when no human is reachable. Not queue, and not wait** | absent. A confirmation that queues until somebody appears is an approval with a delay on it |
| **Approval is bound to one invocation by id and argument digest** | **there is no id on the client side.** `request_id` exists on the wire and the daemon dedups against a bounded window, but nothing in the client or the CLI ever sets it, so the window never fires from any rig surface today |
| **The prompt is composed by the daemon from structured data**, with caller-supplied text shown only as marked, escaped, secondary content | absent. A prompt a caller can write is a prompt a caller can forge |

**The four channels, measured:**

| Channel | State |
|---|---|
| **Flag** | rig reserves exactly three flag names on a call - `json`, `timeout`, `args`. **The rest of the flag namespace is the PROGRAM's**: one flag is generated per declared schema property, so a program declaring a property named `confirm`, `yes`, `force` or `approve` gets that flag generated, completed and accepted, with no comment from rig. **The reservation list is the lever and no confirmation word is in it** |
| **Argument** | **wider than the flag channel and it defeats a flag reservation on its own.** `--args '<json>'` passes the whole argument object verbatim, schema-validated but flag-free. A confirmation expressed as an argument is satisfied in one call and no reserved-name check would see it |
| **Header** | **does not exist on either side.** The client call takes no metadata and the wire frame has no header or metadata map. **Nothing to close today**, and this clause exists to gate the day one is added rather than to repair anything |
| **Second call** | the id that would bind an approval is unset, above |

#### `confirms` is declared, rendered to a human, and consumed by nothing

**rig currently tells a user that a command confirms before acting, while rig
confirms nothing.** `Command.confirms` is a tristate the program declares about
itself. The kernel validates only that it is not unsaid; **no other reader
exists in the tree.** Generated `--help` prints *"it declares that it confirms
before acting"* and `rig apps list --json` carries `"confirms": true`.

**Demonstrated 2026-09-11, not read off the source:** the reference program
declares a `purge` command `destructive` with `confirms` yes, its generated help
says so, and invoked with no terminal it returns success and purges. **No
prompt, no refusal, no channel.**

**This is conflation #16 arriving on its own** - `confirmation` and
`authorization` conflated outright - and it is the sharper form of it, because a
conflation inside a document misleads a reader while **this one makes rig state
something false to a user.** The wording is the tell and it is doing real work:
*"it declares that it confirms"* is honest about the provenance and is read as a
guarantee.

**RULED 2026-09-11: `confirms` is an INPUT rig acts on, not a program's claim
about itself.** Delegated by Boris with the instruction to prefer robustness and
usefulness, and both point the same way here.

**A command declaring `confirms` is treated exactly as if a house rule had
matched it at `confirm`**, and it composes with the rules table through the
most-restrictive-wins rule already stated above. That single sentence is the
whole mechanism, and it is why this is the robust reading rather than the
ambitious one:

- **A declaration can only ever ADD a confirmation, never remove one.**
  Most-restrictive-wins makes `confirms: no` unable to weaken a rule, so the
  field cannot become a second lever on the authorization floor. **The
  weakening hole above does not reopen through this door**, and it would have
  if the composition rule were anything else.
- **It uses machinery that exists.** §13a already produces `confirm` as an
  action and already routes it. Nothing new is introduced at the boundary.
- **The alternative is strictly less useful.** Reading `confirms` as a claim
  means changing the rendering to disown it and leaving rig with a declared
  field nothing consumes - which is the state being repaired.
- **It fails closed.** A program that says it confirms gets a confirmation. The
  failure mode of getting this wrong is an extra prompt, not an unguarded
  destructive call.

**The rendering stops being false the moment this is built**, because *"it
declares that it confirms before acting"* becomes a description of an input rig
honours rather than a claim rig merely repeats.

**The last three are not connections, and that is why they are in the enum.** §14 says every
*connection* carries a principal, and a scheduled fire, a bus-triggered invocation and a
`rig://` URL are none of them - so without these three values the rule the owner most needs,
"the scheduler may not run destructive commands unattended", cannot be written at all. Each
mints a principal at the point of invocation: `schedule` from the schedule entry's owner,
`bus` from the rule's owner, and `url` from nothing, because a URL arrives from the browser.

**`url` is therefore denied above read-only and confirmed at it - which is two rules, and both
ship with the enum.**

```
[[rules]]
caller  = "url"
effects = "writes-files"
action  = "deny"

[[rules]]
caller  = "url"
effects = "read-only"
action  = "confirm"
```

A `rig://` scheme registered on the desktop is reachable from any web page, email or
chat message; §29's "the threat model is a mistake in our own code, not an adversary" was
written about hosted programs and does not cover an inbound handler that anything can address.
A destructive call arriving over `rig://` matches both rules - the first through the floor
above - and the precedence rule resolves it to `deny`. **It is written as two rules rather than
as prose because "read-only plus confirm" is not something the grammar can say in one**, and a
default that cannot be expressed in the grammar it ships beside is a default nobody can audit.
The URL surface itself is M13. **The enum ships at M1**, because it lives in the kernel's
invoker and the whole point of that placement is that a surface added in 2028 is covered by
rules written in 2026 - which only holds if the vocabulary can name it.

**A rule is a pair, and a wrapping verb splits it.** `effects` comes from the registry, per
command, declared by the program; `caller` comes from the invocation. A verb whose subject is
another command separates the two onto different objects - and one ships at M7:
`rig peers run --lease=NAME -- make deploy` (§16). Match on the outer verb and there is no
declared `effects`, because `rig peers run` has no registry entry. Match on the wrapped command
and the caller is no longer the principal that arrived. **Whichever half the invoker looks at it
loses the other, so the pair never completes and, against an empty default set, the call is
allowed.** The rule the owner most needs - "the scheduler may not run destructive commands
unattended" - is silently unwritable for exactly the callers it was written for.

**So the invoker matches on the effective pair, resolved once at the boundary.** `caller` is the
principal that arrived; `effects` is the union over every registered command the call will
actually run. A wrapping verb contributes no effects of its own and **cannot mask the effects it
wraps**. This is a property of the invoker, so it holds for a wrapping verb added in 2028 the
same way the caller enum does.

**A wrapped command rig cannot resolve counts as `destructive` for matching.** `make deploy` is
not a registered command and rig cannot know what it does. This does not change what happens
when no rule exists - an empty set still matches nothing - but it means a rule that names
`destructive` fires on the one case where the invoker cannot see what it is authorising, rather
than silently missing it. Unresolvable and harmless is a combination only the program can
declare, and it has not.

- **It lives in the kernel's invoker**, so no surface can forget it and a surface added in 2028
  is covered by rules written in 2026. One test covers every surface, present and future.
- **The default rule set is the two `url` rules and nothing else**, so nothing already working
  breaks the day it lands: no surface mints a `url` principal before M13, and every other
  caller starts unmatched. An unmatched call is allowed, which is what makes the table safe to
  land early - and is why the M1 demo writes a rule rather than relying on a default.
- **It costs a registered program nothing.** Both fields it matches on are already declared.
- `confirm` routes through the same `ask` primitive the peers service uses (§16), so the
  question reaches whoever is actually present - window, toast, terminal or phone.
- **Two kinds of ask, and until now the plan had one failure mode.** A **gating** ask fails
  *closed*: an elevation nobody answers is denied and recorded (§14), because the alternative
  is estate-wide action taken by default. A **disambiguating** ask fails to *none*: it parks,
  the way §9's `interactive-stream` run parks rather than fails, and the caller proceeds
  without the thing it asked about. "Which of these three continuations?" (§16) must not be
  *denied* when nobody answers - denying it strands a replacement that could have started
  cold. Every `ask` declares which kind it is, and **the default is gating**, because that is
  the half where guessing wrong is unsafe.
- Every decision is written to the audit log with **`origin`**, which is `rule` or
  `elevation`, and the rule id when there is one. An elevation is not triggered by a house
  rule - it is triggered by the call needing estate-wide authorisation (§14) - so an `origin`
  that can only name a rule has no value for the commonest `confirm` the system will produce,
  and the log cannot then answer the first question anyone asks of it. One more enum value
  now; a schema migration later.
- With that, "why was I asked" and "why was that allowed" are answerable from the record.

---

## 14. Isolation

**Default deny, at three boundaries.** A user of rig is not aware that other users exist.

| Boundary | Between | How |
|---|---|---|
| **Instance** | Unix users | One daemon per uid. One socket at `$XDG_RUNTIME_DIR/rig/rigd.sock`, mode 0600, separate config, state and storage trees. Nothing is shared, including the tray, the window and the notification centre |
| **Session** | Clients of one daemon: agents, terminals, the window, scripts | Every connection carries a principal - uid, client kind, client id, session id - and belongs to one or more **scopes**. Every kernel view is filtered by scope set, **unless the principal holds `introspect`** (below - and it is not presented, it is decided
when the connection is made). Enforced against accident and against a merely buggy client; see the floor stated below for what it is not enforced against |
| **Program** | Programs | Capabilities (§13): secrets namespaced, storage pathed, events by grant, no cross-program read of anything |

**A client sees itself and the programs it may reach. Nothing else.** It cannot enumerate other
clients, cannot see their commands, cannot receive their events and does not appear in their
views. Two agents working in the same repository, through the same daemon, are invisible to each
other unless both opt in.

**That is the ordinary API, and it has exactly one door out of it: `introspect` (below).** The
default is deny because a program should not read another program's events by accident; it is
not deny because Boris should be kept out of his own machine. An agent he runs holds
`introspect` and sees all of it. The isolation this section builds is between *programs and
unprivileged clients*, and the proof below is stated over that population, not over a
principal that was handed the read grant on purpose.

### The kernel exposes no unscoped accessor at all

The previous version of this section listed six views that get filtered, and claimed in the same
paragraph that a new surface cannot leak. An allowlist of six is exactly how a seventh leaks.

**Every registry read is a method on a principal, the principal is in the type, and an unscoped
read is unrepresentable rather than discouraged.** A house analyzer - `no registry handle
outside the kernel` - sits beside the existing `no program id in rig code`.

**The corollary the plan was missing: any surface whose product is an estate-wide aggregate is
by definition a granted surface**, and needs one of the two grants below. That is the
capability map, `config export`, `doctor`, the palette, the tray, the notification centre and
the schedule - seven things that sat outside the old six-view list, one of which §9 tells an
agent to read first. **Which grant is the whole question**, and the earlier version of this
section answered it with the only credential it had, making every one of those seven closed to
an agent. Reading them takes `introspect`; changing anything through them takes `operate`.

### The operator is not a uid - and not a credential either

§15 used to grant the clients view to "the uid that runs the daemon", and the Instance
boundary above puts one daemon per uid. A unix socket carries no credential but `SO_PEERCRED`, so **every client that could
connect already satisfied that predicate** - and `rig history --client=X` would then hand any
agent another program's secrets (§15). It is the highest-ranked finding of the 2026-09-10
attack and it was invisible to every seat individually.

The fix went through two wrong drafts before the right one, and both were wrong the same way,
so the reasoning is worth keeping:

| Draft | Mechanism | How it broke |
|---|---|---|
| 1 | A token minted at startup into a 0600 file | Every client runs as this uid, so every client could read it. Any program could grant itself the estate |
| 2 | A token in `RIG_INTROSPECT`, exported by Boris's shell | An environment is inherited by a shell's **whole descendant tree** - `make`, an npm postinstall, every in-house program he starts by hand. And it is minted per daemon start, so after the first `rigd` restart every long-lived shell holds a stale one and is silently downgraded, with no way to rewrite it from outside |

**Both drafts tried to push a secret to the right set of processes.** The set was never the
problem. rig already knows which connections are programs, because it registered them.

### `introspect` is decided at connect time, not distributed

**What it grants**, stated before how it is decided: **every read**. The estate-wide views, the
full capability map, `config export`, `doctor`, the schedule, the notification centre, any
client's history, any trace. It grants **no action at all** - that is `operate`, below.

- `introspect` is **not a token, a file or an environment variable**. There is nothing to
  deliver, nothing to store and nothing to steal. It is decided when a connection is made:
    * a connection that completes the **program registration** handshake (§5) is a program,
      and is scoped for the life of that connection;
    * every other connection on the local socket from this uid is a client of Boris's, and
      reads everything.
- **HTTP is excluded.** A connection with no unix peer mints its principal from a bearer
  token per client kind - `MintPrincipal(evidence)`, below - and gets no scopes at all,
  `introspect` included.

Nothing is inherited, so a build script descended from Boris's shell gets no more and no less
than the shell did - and a program gets nothing, however it was started. Nothing is minted per
daemon start, so a restart cannot silently downgrade a window that has been open for a week.
Nothing is persisted, so there is no file to find.

**The one limit, stated because a boundary nobody writes down is a boundary nobody tests:** a
program that opens a *second* connection and declines to register on it is a client of Boris's
and reads everything. That is the same floor the file and the environment had, but reaching it
now takes deliberate action rather than inheritance - which is the standard §13 sets for
capability enforcement, labelled the same way. **No program acquires the estate by accident, or
by being merely buggy.** Genuinely enforced against a hostile local process: only the uid
boundary and redaction.

This is also the only version of the mechanism a **hosted** program (§5j) can be held to. A
hosted plugin is compiled into `rigd` and shares its environment, so no environment scrub could
ever have covered it; its *connection* is registered like any other program's, so the predicate
covers it with no special case. Conformance item 23 asserts it, and item 24 holds the hosted
class to it (§19).

### Every new principal gets walked against this table

The 2026-09-10 sweep found the same defect four times - the grant reaching a caller kind nobody
was thinking about - and diagnosed the cause correctly: **reasoning about a grant from the
caller it was written for, and not from the others.** The artefact that prevents it is a list of
the callers. Anything that adds a principal, a grant or a scope is walked against all of it, and
the walk is part of the change, not a review comment:

| Caller | Has a connection | What it gets |
|---|---|---|
| An agent Boris runs | yes, unregistered | everything. This is the motivating caller, and the one that misleads |
| A terminal, the window, the tray | yes, unregistered | everything |
| A script, a `make` target, anything descended from his shell | yes, unregistered | everything, and it inherited nothing to get there |
| A registered program, spawned or hosted | yes, registered | its own scope only |
| An HTTP client | no unix peer | its bearer principal's scopes, never `introspect` |
| A scheduled fire, a house rule, an internal timer | **none** | rig's own principal. It has no connection to decide from, so it is named explicitly or it is refused (§13a) |

### `operate` is an authorisation decision per call, not a grant at all

The first draft handed `operate` to "the process that launched `rigd`" - which under M15's
autostart is systemd, so `rig stop shelf` from a terminal, the tray's Start entry and the crash
panel's manual restart were all unreachable. The second draft kept a file for "non-interactive
callers - systemd units, `make deploy`", and recreated the same defect one row lower: `make
deploy` typed in a terminal descends from Boris's shell, not from the daemon's launcher, so the
row promised a file to a caller that provably cannot read it.

**So `operate` stops being a credential.** Estate-wide action - stopping a program another
client owns, revoking a grant, breaking a lease, editing config outside the caller's scope - is
an authorisation decision taken per call, which is exactly what §13a already is:

| Caller | How the call is authorised |
|---|---|
| Any interactive surface - terminal, tray, window, an agent | `confirm`, which routes to window, toast or terminal (§12, §13a). The answer authorises **that one call**, not the connection |
| Anything unattended - a systemd unit rig installs, `make deploy`, a scheduled fire | A **house rules** entry naming the caller and the command. Declared, auditable, reviewable in one file, and no secret exists to leak |

That deletes the second file, the second boolean and the distribution problem in one move.
**One boolean remains on the principal struct** - `scoped`, set by registration - and there is
no other way to become either kind of caller.

There is no new elevation vocabulary: `confirm` already routes to window, toast or terminal.
Authorisation is per call, so nothing holds standing estate-wide power for hours. And the audit
log already records every `confirm` decision, so an estate-wide action is audited for free -
with `origin: elevation` distinguishing it from `origin: rule` (§13a).

**The question must name what it is authorising, and reach the caller that asked.** A toast
that says only "stop shelf?" is answered by whoever is looking at the screen, in the belief it
came from the terminal in front of them - and §12's toast deliberately does not steal focus.
So the prompt carries, always: the **principal** and client kind, the **pid**, the
**command**, and the **arguments** as they will be invoked. And it routes to the requesting
caller's own surface first - an interactive caller is asked where it is, and only an
unattended caller's question goes to whoever is present. The audit log records the decision
afterwards (§13a); this is what makes the decision itself attributable at the moment it is
taken.

**An unanswered elevation is denied, and the denial is recorded.** Estate-wide action fails
closed: an agent at 3am that asks to stop a program another client owns, with nobody present
and no house rule naming it, is refused rather than queued. Do Not Disturb never suppresses an
`ask` that gates a command (§12) - it suppresses notices, and this is not one.

**A lease-holder can still be asked.** §16 says a lock is never held across a question to a
human, and rig refuses the combination - which would refuse the motivating case, `rig peers run
--lease=deploy -- make deploy` needing to stop a program. The exception, and it reuses existing
machinery: **an elevation or `confirm` that gates the lease-holder's own call freezes expiry on
every lease that caller holds for the duration of the question**, the same freeze §16 already
applies after a suspend. The rule §16 states is about a lock blocking *someone else* on an
answer, and here nobody else is blocked.

**Why complete reading is safe.** The 2026-09-10 attack's highest-ranked finding was that
`SO_PEERCRED` made every client an operator, and that `rig history --client=X` would then hand
any agent **another program's secrets**. That was a finding about *what the history contains*,
and §15 fixed it at the source: anything `secrets` returns is never recorded, and every field
declared `sensitive` is blanked before the write. **Redaction is the precondition for complete
introspection.** With it, the worst an introspecting agent can read is everything Boris could
read himself - the requirement, not the leak. **That holds for the paths that cross the
history's write, which is where redaction happens**, so §15 states the invariant every other
estate-wide read path has to satisfy; a path that satisfies neither half of it leaks while
passing run 3. Without it, no mechanism would have been safe
enough - so M1 ships the connect-time predicate, and complete history reading is gated on M5's
redaction spans (§23).
### Scopes, so peers is not a hole in the filter

`announce` must return the crew, so under the old design the kernel needed a peers special case
at the very enforcement point that was supposed to make leaks impossible - and §3's two-client
test would have had to fail the moment peers compiled in.

Instead the kernel offers one general mechanism: `MintPrincipal(evidence)` and
`JoinScope(principal, scope, grant)`. Peers becomes an ordinary service that creates a crew
scope and joins consenting principals to it. The visibility it needs is **granted data, not a
code branch**, which is what §14 always intended by "invisible to each other unless both opt in".

`MintPrincipal` also answers a question no section previously did: **a surface with no unix peer
has to mint a principal from something.** HTTP hits this at M2. Evidence is a bearer token
issued per client kind, and an unauthenticated HTTP request gets a principal with no scopes
rather than the daemon's own.

### Written down, and therefore testable: what a client may infer

A boundary nobody wrote down is a boundary nobody tests.

- Acquiring a contended lease reveals that *a* peer exists. Accepted, and stated.
- `try_lock` returning BUSY reveals the same. Accepted.
- Deadlock refusal names **the leases the caller holds or requested**, never the peers. The
  named cycle goes to the clients view only, which needs `introspect` (§14) - so an agent
  debugging a stuck `make deploy` can see the cycle, and the blocked peer still cannot.
- Counters are scoped: fencing tokens monotonic **per lease**, blackboard revisions **per
  namespace**. Never one global sequence, which leaks the rate of other clients' activity.
- A crew is created by a principal and joined only through a handle it hands out. Joining an
  unknown crew name returns the same error as joining one that does not exist.
- A colliding program id from a principal that cannot see the incumbent returns exactly the
  error it would get for an id it is not permitted to use - indistinguishable from "not
  allowed", never "already exists". An error that distinguishes those two is an oracle.

### The proof: observational equivalence over two worlds

§3's old test was a *content* predicate - run every surface against a two-client fixture and
fail if either sees the other - and every channel above except one is a **differential**
channel, invisible to it. Worse: with the operator bug, the fixture's expected result for
`rig clients` was "B is visible", so the test asserted the breach.

The property is stated as observational equivalence instead. Run the whole battery against
**W1 = {A}** and **W2 = {A, B}**, with B exercising every primitive, and require A's complete
transcript - responses, error codes, ids and tokens issued, ordering - to be identical. A
difference is a failure unless it appears on the enumerated, reviewed list above. The battery
covers non-surface observation too: the runtime directory, the config tree, the state tree, the
process table.

This is the same machinery §16 already buys for deterministic simulation at M7. It just has to
be pointed at isolation as well as at linearizability.

**A is a client with no grants. That is the whole population the property is stated over**,
and saying so is not a weakening - a property whose population is unstated is a property that
gets quietly falsified the first time a legitimate reader is added. So the battery runs three
ways, and the third is the one that catches the bug this design could actually have:

| Run | A holds | Required outcome |
|---|---|---|
| 1 | nothing | A's transcript is identical in W1 and W2. Isolation holds |
| 2 | `introspect` | A sees B completely, in every view, **with no gap that §15's coverage log does not declare**, and no silent filtering. A missing row whose absence the coverage log explains is a pass; a missing row it does not explain is a failure here, the same way an extra row is a failure in run 1. The assertion is on the coverage entry, not on the row - §15 loses records through four declared paths, so a completeness test with no such clause is false on the first busy day |
| 3 | `introspect` | Every value B declared `sensitive`, and everything B's `secrets` calls returned, is absent from A's transcript - at any sampling rate, in any encoding |

Run 2 is the requirement in §2 made testable, and it fails loudly rather than
degrading: **a view that quietly returns the scoped answer to a principal holding
`introspect` is a defect of the same rank as a leak.** Run 3 is why run 2 is safe, and it is
§3's existing no-secret-reaches-the-history test pointed at the reader instead of the log.

## 15. Who is using rig, and what they did

§14 says clients cannot see each other. **A holder of `introspect` can see all of them, and
that includes every agent Boris runs** (§2). The asymmetry is deliberate: isolation is between
clients, not between rig and the people and agents working for its owner.

Both halves of that sentence are corrections, and they were made a day apart. `introspect` is
decided at connect time (§14), not a uid and not a token - because
without that correction every client on a one-user machine reads everything by accident, and
`rig history --client=X` hands over another program's secrets. And it is deliberately **not**
the same grant as acting estate-wide, because the version of the fix that made this section
safe also made it useless to the reader it exists for.

### The clients view

| Column | For every connected client |
|---|---|
| Who | Kind (agent, terminal, window, script, program), pid, command line, uid, session id |
| Since | Connected at, last active, wire version, stub build |
| Holding | Leases held, crew membership, subscriptions, capabilities granted |
| Waiting | What it is blocked on, and for how long |
| Doing | Calls in flight right now, with elapsed time |
| Rate | Calls per second, bytes, error rate, denied capability attempts |

Selecting a client opens its history: every call it made, when, with what arguments, how long it
took, what came back - **minus everything declared sensitive**, which was never recorded at all.
The same view exists in the window, the TUI, the CLI (`rig clients`, `rig history --client=X`)
and over MCP, because they are all surfaces over one registry, and all of them require
`introspect` (§14). **An agent Boris runs holds it**, so `rig history --client=X` is a question
an agent may ask about any client, including another agent - that is §2's decision, and the
redaction above is what makes it safe rather than the scoping that used to sit here.

**Looking is itself an event.** Opening another client's history is written to the audit log.
On a single-user machine that is close to pointless, but it costs nothing and it means the rule
is the same rule when it stops being a single-user machine.

**Coalesced, because the reader is usually an agent.** Written for a person opening one
history, per-read auditing turns the log into a transcript of whatever agent is sweeping the
estate (§9). The record is therefore **one entry per principal, per view, per minute, with a
count and the widest range touched** - which answers "who read what, when" exactly as well and
does not drown the thing it is auditing. A grant being *presented* is always its own
uncoalesced entry, so the interesting event - a principal becoming an introspector - is never
buried in a count.

### Redaction, or the history is an exfiltration channel

**`secrets.get` is a call, and this section records what every call returned, verbatim.** With
no redaction anywhere, a keyring token lands in an unencrypted segment kept for thirty days, and
`rig history` hands it over. Conformance item 15 does not catch it: that tests the *program's*
logs, not rig's own history.

Redaction is therefore part of the **registration declaration**, not of the logger:

- A command declares `sensitive`: a list of JSON pointers into its arguments and its results.
  Mandatory, may be empty (§5e).
- At registration rig compiles that list once per `(method, wire version)` into a **byte-span
  list over the wire encoding**. The hot path blanks known offsets: **82.5 ns, zero
  allocations**, inside budget. Decoding the payload to redact it at record time costs
  **3100 ns**, which is 15x the whole budget - so the declaration is the only affordable place
  for this.
- **Redaction is a write-side mechanism, and this plan has been stating it as though it were a
property of reading.** The compiled span list blanks known offsets on the way *into* the
history, so every estate-wide read that goes through the history is safe by construction. A read
path that does not go through the history is not, and §14's sentence - "the worst an
introspecting agent can read is everything Boris could read himself" - is true only of the paths
that cross that write. Every conformance run in §14 would pass while such a path leaked, because
run 3 reads the transcript and the leak is not in the transcript.

**The invariant, stated once so that a new read path has to satisfy it rather than notice it.**
Every estate-wide read path either:

1. renders its payload through **the same compiled span list** as the history write; or
2. declares its payload **wholly sensitive** - existence, size, age and owner readable
   estate-wide, contents never.

§16's continuation slots already take (2) and say so in their own text. Anything new picks one
explicitly, in writing, and **a third option does not exist**: a second access-control door is
precisely what §14 says it does not have.

**Why (2) has to be available at all.** `sensitive` is JSON pointers into arguments and results
(§5e), so a payload that is neither - a live outbound stream, an opaque blob a program owns -
has no pointer space in which the declaration could be made. For those, (2) is the only
mechanism there is, and a design that reaches for (1) and finds no pointer must take (2) rather
than conclude the question does not apply.

**Anything the `secrets` service returns is never recorded at all**, only the key name. A
  field that is not declared sensitive *is* recorded, so the default for a secret cannot be left
  to a declaration.
- The declaration is now security-relevant, so widening it needs the same explicit confirmation
  §13 requires for widening a capability.

### History has to be nearly free, and it can be

You gave the permission that makes this cheap: **losing recent entries to a crash or a power cut
is acceptable.** Durability is what makes event logging expensive, so not needing it changes the
design completely.

```
  call happens
      │
      ▼
  per-client ring buffer      sharded, so the cursor and the interner are
      │                       uncontended by construction, not by luck
      │  flush armed BY THE FIRST RECORD, then 250 ms or 256 KB
      ▼
  write(2) to a segment file  NO fsync, ever
      │                       the kernel now owns it
      │  on rotation
      ▼
  zstd the closed segment     + a self-describing header
                              + a per-segment column summary
                              + an offset index (rebuildable)
```

**Six things that keep it cheap, and each one was wrong in at least one way before.**

- **No fsync in the hot path.** The whole reason it is affordable, and a locked decision.
- **Sharded per client, merged at flush.** A single shared ring measured 2.1x over budget on the
  mean at ten clients and 5.1x at p99; with the obvious RWMutex-map interner, 33x over. The
  budget below is stated at p99 with sixteen concurrent clients, because a single-writer mean is
  not a number anyone experiences.
- **Interned, fixed-width records**, with the **dictionary written into the segment**, not the
  index. A segment holds the integer 42, not `peers.lease.acquire`; if that mapping lives only
  in the index then "delete the index, it rebuilds by scanning" - which this plan used to
  advise - destroys data and breaks §7's locked rule. Each segment carries a header of the
  id→name pairs first referenced in it, appended at rotation from data already in memory. A
  segment then decodes standalone, ids are scoped to the segment that defines them, so a rename
  coexists with its old spelling instead of colliding. Client ids are interned **per principal**,
  not per connection, so a Makefile loop does not mint 100k entries.
- **The flush timer is armed by the first record**, not run on an interval. An idle daemon has no
  history timer at all, which is where §17's wakeup budget went: a 250 ms ticker alone measured
  **45.5 wakeups/second** against a stated budget of one.
- **Sampling under load, with the rate recorded**, so counts stay correct.
- **Retention by size, age AND rate.** 500 MB holds 10.56M records; ten programs health-checking
  once a second collapses "thirty days" to 12.2 days, and one client at the wire's own 161k
  calls/s unlinks everything in 66 seconds. A per-client rate ceiling is enforced before the
  size ceiling, so one noisy client cannot evict the estate's record.

### The coverage log, so a gap is visible rather than silent

Four separate paths lose records - sampling engaging, the ring overwriting, a segment being
unlinked, a segment failing to close cleanly - and without this they are indistinguishable from
"nothing happened", which makes the whole log useless for the one question it exists to answer.

One small append-only object: `(from_ts, to_ts, client_id or *, cause, retained/total)`, written
on the cold path whenever any of those four occurs. **Every view in §8 and §15 renders a gap
band and refuses to answer "every call it made" without stating its coverage.**

**The audit log and the peers timeline are never sampled and have their own budget.** They are
not the same kind of object as a call log and must not share its eviction.

### Querying thirty days without nine cores

500 MB of zstd is 2.0 GB raw; at a measured 1197 MB/s/core, a full scan is 1.7 s of core time,
so the old "< 200 ms over 30 days" needed 8.5 cores. A segment is therefore not the unit of
reading. On rotation, alongside the blob and the index, rig writes a **column summary**: the set
of client ids present, the set of method ids, min/max timestamp, error count, and a bitmap of
which client appears in which one-second bucket. A few KB per 64 MB segment, computed from data
already in hand.

### The budget

| What | Target | Basis |
|---|---|---|
| History append, **p99 at 16 concurrent clients** | **< 200 ns** | measured 110.8 ns single, 334.9 ns unsharded at 8 |
| **Whole recorded call**: history + span + slog | **< 2 µs** | measured 1757 ns (§8). The old budget bounded one third of the cost |
| Redaction, per sensitive field | **< 100 ns** | measured 82.5 ns as compiled spans |
| Worst-case history lost to a power cut | **< 1 s** | unchanged |
| Memory: all history buffers | **2 MB**, configurable | was 8 MB, which was 41% of the whole RSS budget (§17) |
| Memory: interning dictionary, resident | **512 KB** | new line; it was previously unbudgeted |
| Disk, default | **500 MB** call log, **plus** a separate audit + timeline budget | oldest dropped first, with a coverage entry |
| **Filtered** 30-day query | **< 200 ms** | what the column summary delivers |
| **Unfiltered** 30-day scan | **seconds, streaming** | the truth, and it is fine |

All of them are asserted by `make bench-idle` and a history-specific benchmark, so a regression
fails the build rather than being noticed a year later. Index rebuild is an explicit background
task with history served degraded-but-labelled meanwhile, so it cannot blow §17's cold start.

## 16. The peers service

Isolation is the default (§14); coordination is a capability you opt into. This service is
**the best inter-agent coordination substrate we can build**, and once it is finished and
proved, every agent on this machine moves off AgentBox onto it. Decided 2026-09-10.

That is a high bar, so this section says what clears it.

### The structural advantage: one daemon means no consensus

There is exactly one rig per user. Every coordination operation therefore passes through a
single serialization point, which makes them **linearizable by construction**. No Raft, no
Paxos, no split brain, no clock skew, no quorum. Distributed coordination is hard because of
consensus; rig does not need consensus, so it can spend the entire budget on semantics and
observability instead. That is why this can be better than the general-purpose tools.

**Except that one serialization point orders *applied* operations, and linearizability is about
the client's view.** A `barrier.arrive()` that times out and is retried once releases a barrier
of nine with eight agents present. The fix is not in this service: it is the request id and the
session token on the wire (§5f), so every mutating call is exactly-once from the client's side
and every future primitive gets it free. Without those two fields the claim above is false on
any retry, and §5d forbids the stub from fixing it.

Durability comes from a write-ahead log: coordination state survives a daemon restart, and
clients reconcile on reconnect rather than losing their place.

### Time, named, because a laptop suspends

Every deadline in this service is **absolute, on `CLOCK_BOOTTIME`**, stored in the WAL as
`boot_id + boottime_deadline`. Never a remaining TTL, which a restart silently extends; never
the client's clock; never wall time, which a timezone change moves.

Go's monotonic clock does not advance across suspend, and this laptop suspends nightly, so the
two readings of "the same lease" differ by hours depending on whether rig restarted. A changed
`boot_id` means the machine rebooted, which is a different and simpler case.

**A resume grace epoch.** rig detects a gap larger than one TTL (BOOTTIME against MONOTONIC, or
logind's `PrepareForSleep`) and freezes expiry for one full TTL after resume, during which every
holder must re-present its token before its first guarded operation. Without it, a suspend
expires the whole estate's leases at once, on the machine this is built for.

§20 mandates `testing/synctest` for all timing, and synctest **cannot model suspend**. So this
class needs a real-clock test with an injected clock jump, or the Simulated gate below is
structurally blind to it.

### The primitives

**Presence.** What AgentBox does today, kept because it works.

| Call | What it does |
|---|---|
| `announce` | State a purpose and current activity; receive the crew as it stands |
| `activity` | Update what you are doing now. Cheap, non-blocking |
| `list` | Who is here, their purpose, activity, state, and what they hold or await |

**Mutual exclusion, done correctly.** This is where AgentBox is improved on rather than copied.

| Primitive | Why it is here |
|---|---|
| **Leases, not locks** | Every hold has a TTL and must be renewed. A dead holder expires automatically. There is no orphan state to detect and no human to break a lock |
| **Fencing tokens, scoped to what they can actually fence** | The classic bug: holder A stalls, its lease expires, B acquires, A wakes and writes anyway. Each acquisition returns a token, monotonic **per lease**, and every rig-mediated write must present it. **But a token can only fence writes rig mediates**, and every lease in this estate guards something rig does not own - git, a deploy, the VM, the desktop. The token is rejected while `make deploy` runs on regardless. Said plainly here, because the unqualified sentence will otherwise be quoted back later. **One exception now exists and it is narrow**: the display, once rig owns the only driver that can reach it (§5m). It is an exception because the resource is reached only through rig, which stays false of git, the deploy and the VM |
| **A liveness witness, and two-step expiry** | This is what makes the line above safe. `acquire` takes a witness: a pid or pidfd rig can poll, a cgroup, or the literal `unwitnessed`, recorded in the WAL beside the token. TTL expiry moves the lease to **ORPHANED, not FREE**; ORPHANED becomes FREE only when the witness is observed dead. An `unwitnessed` lease needs an explicit break, which is a recorded human action - AgentBox already works this way and discarding it would be a regression |
| **`rig peers run --lease=NAME -- make deploy`** | The primitive that makes the honest path the easy path. rig owns the child process, so the witness is exact and, critically, **expiry can kill the writer**. It is the only real fence for a resource rig does not own, which is all of them |
| **Read/write leases** | Many readers or one writer, because "everyone waits for everyone" is how a crew stops working |
| **Deadlock detection** | rig holds the wait-for graph and can see a cycle. It refuses the acquisition that would close one, naming the cycle, rather than letting two agents wait forever |
| **Semaphores** | At most N agents doing the expensive thing at once |
| **Barriers** | N agents wait until all arrive, then all proceed |
| **Leader election** | Exactly one agent runs the migration, and the others know who it is |

**Shared state.**

| Primitive | Why it is here |
|---|---|
| **Versioned blackboard** | Compare-and-swap on a key, with a revision per change. **A key may also be CLAIMED, with an owner and a liveness witness** - see "A claim has an owner and a liveness witness" below, which is the half of this primitive the estate actually uses |
| **Multi-key transactions** | Claim three things or none. Single-key CAS cannot express "divide this work" safely |
| **Watches with a cursor** | Subscribe from a revision. A client that reconnects gets what it missed instead of a gap it cannot detect. **One global revision, transaction-granular delivery**, so a multi-key claim is never observed half-applied |

**Work distribution**, which is what "divide the work so nobody doubles" actually needs.

| Primitive | Why it is here |
|---|---|
| **Claimable queues, at-least-once and it says so** | Claim a task under a lease, heartbeat it, and it is requeued if you die. **Duplicates are possible**: a task carries a mandatory `idempotency_key` and the consumer contract is replay-safety, or the task runs through the witnessed path above so requeue can kill the stalled worker before re-offering it. Requeue is the same two-step machine: EXPIRED → witness dead → REQUEUEABLE; witness alive → ORPHANED. One mechanism, three findings |
| **`duplicate_execution` is an event, not an error** | A stale `complete` is recorded on the timeline, so the post-mortem view can answer the question this primitive creates. Detecting it in the simulator is not the same as seeing it in production |
| **Rendezvous** | Hand a result to a named successor and park until it is collected |
| **Signals** | `post` and `await`. Park with nothing burned until a peer wakes you. Replaces every poll loop and every "check back in five minutes". **This is the primitive; the contract is "The message contract, from queued to acted-on" below** - five delivery states, a cursor with `gap: true`, topic families, and what happens to a queued message when the recipient dies |

**The human is a peer.** `ask` puts a question to whoever is actually present, routed to the
window, a toast, the terminal or a phone. That is AgentBox's most valuable single idea and it
is kept whole.

### What the agent-parallelism pass added, 2026-09-11

**Why this subsection exists.** Boris, 2026-09-11: the mechanisms that matter
are the ones that let **AI agents work in parallel** on one machine, so that rig
can run like AgentBox and the agents can then migrate off AgentBox onto rig.
**Not at parity - above it.** And explicitly *"not all AgentBox are must-have in
rig, some of its features have no place in the rig"*, so nothing below is here
because AgentBox has it; each is here because an agent working beside another
agent fails without it.

**The bar is this section's own**, set by the lease above: a primitive that does
not say what happens when the holder dies has been mentioned, not specified. The
five below are ordered by
`logbook/projects/rig/taxonomy-parity-cross-2026-09-11.md`, which crosses the
blind mechanism taxonomy against the AgentBox parity pass. **Three of the five
appear in both lists independently**, and the cross records which.

#### 1. A claim has an owner and a liveness witness

**The blackboard above has compare-and-swap, a revision per change, multi-key
transactions and watches with a cursor. It has no owner**, so a session that
dies holding `claims/chunk-3` leaves a chunk nobody will ever finish and nothing
says so. **The estate divides work with exactly this key shape**, so the half of
claiming that rig specified with a witness (the lease-backed queue) is not the
half anybody uses.

| | |
|---|---|
| **A key may be claimed, not merely written** | `own` records the claiming session **and its liveness witness** - the same witness the lease takes: a pid, a pidfd, a cgroup, or the literal `unwitnessed` |
| **A read reports the owner's liveness, not just the value** | **The same two-step as the lease, and deliberately the same words: OWNED, then ABANDONED when the witness is observed dead. Never silently FREE.** A claim whose owner is gone is visible as abandoned rather than indistinguishable from a healthy one |
| **Taking over an abandoned claim is one CAS write** at the version just read. No break, no human, no second mechanism |
| **An `unwitnessed` claim needs a recorded human break**, exactly as an unwitnessed lease does |
| **Values are NEVER trimmed** | The deliberate difference from signals. **Retention on a claim table hands one chunk to two agents**, which is the failure the whole primitive exists to prevent |
| **The cap refuses a NEW key rather than evicting an existing claim** | Under pressure, an eviction is indistinguishable from a completion to every reader. Refusing is loud; evicting is silent and wrong |

**Two opposite policies over the same event already exist in one daemon, and
nothing states which is intended where.** Measured 2026-09-11 by reading every
piece of state `internal/` holds:

- **The single-instance claim is engineered to VANISH on holder death.** It is
  an `flock` living on an open descriptor, so `kill -9` releases it and leaves
  no stale claim behind. Death frees it, deliberately.
- **The registry's name claims are engineered to be REFUSED to a successor.**
  A second registration of one identity is denied while the holder lives, and
  the holder's liveness is the connection.

**Both are defensible and they are opposites.** One says a dead holder's claim
should evaporate; the other says a claim should outlive the moment and be
defended. **This is gap 3 meeting gap 6**: the disposition of a claim on holder
death is the single most consequential decision in this design, and rig has made
it twice, differently, without writing either down.

**The rule this section adopts:** a claim that exists to prevent two actors
doing one thing at once (the single instance) may evaporate on death, because a
dead actor is not doing the thing. **A claim that carries identity a successor
needs (a registered name, a seat, a chunk of divided work) goes ORPHANED and is
inheritable, never silently free and never permanently refused.** The
distinction is whether anything downstream has to find the claim again.

**Why this is one mechanism and not two.** rig currently splits claiming: the
work queue claims **under a lease** with a heartbeat and a witness, and the
blackboard claims **a key** with CAS and no liveness at all. That split is why
the gap was invisible - the witness exists in the document, just not on the
surface anyone uses. **The witness belongs to the act of claiming, wherever it
happens.**

#### 2. The message contract, from queued to acted-on

**Signals were one sentence.** `post` and `await` is the primitive; the contract
is below. With a recipient that can be confidently wrong and can repeat what it
was corrected on, **the gap between "delivered" and "acted on" is where
multi-agent coordination actually fails**, and no existing wording distinguishes
them.

**Five states, and who owns each transition:**

| State | Owned by | Means |
|---|---|---|
| `queued` | the daemon | accepted and durable. **`delivered: 0` means nobody was parked, NOT that anything was lost** |
| `delivered` | the daemon | handed to a recipient's open subscription |
| `read` | the recipient | returned from an `await` into the recipient's own context |
| `acknowledged` | the recipient, **explicitly** | the recipient states it understood. Never inferred from delivery |
| `acted-on` | the recipient, **explicitly, and it carries the outcome** | the only state a sender may plan against |

**Nothing promotes a message on the recipient's behalf.** A sender that needs
`acted-on` asks for it and waits for it; a sender that does not, does not. The
failure being designed out is a sender treating `delivered` as agreement.

| | |
|---|---|
| **A cursor per subscriber, and `gap: true`** | When the cursor is older than retention the batch **cannot** be complete. **Treat what you were tracking as UNKNOWN, never as not having happened** - a silently incomplete batch is how two agents come to believe they each own one chunk. This is the lease's own best sentence applied to messages, and this section set that bar itself |
| **Everything since the cursor arrives in ONE batch** | Three events that fired while an agent was editing are one wake-up, not three missed ones |
| **Topic families** | a trailing `*` is a prefix, so one waiter covers a fan-out without naming its members in advance |
| **Directed messaging** | every seat names a private topic, so "message that agent" is an ordinary post and needs no second mechanism |
| **A bounded payload, and a named anti-pattern** | a payload where a pointer belongs. The bound is stated, and a message over it is refused rather than truncated |
| **The rider** | when an agent's area gains or loses a peer, or a lease it holds is broken, or a claim it owns is reclaimed, **the news is appended to the result of whatever call it makes next.** It needs no subscription and no decision to listen, which is the point: **it reaches an agent that did not know to ask.** A watch and a signal both require having already decided the thing was interesting |

**What happens to a queued message when the recipient dies, which is the
question nothing answered:** it is queued against the **seat**, not the session.
A session's death does not destroy it and a successor claiming that seat
receives it. **A message queued to a session that has no seat dies with the
session, and the sender is told so** rather than the message expiring silently.

#### 3. Seats, generations, and `HANDING_OFF` as a published state

**This is the one place rig overtakes AgentBox rather than catching up, because
AgentBox has no seats either.** The estate runs multi-seat teams today on a
naming convention over a blackboard key, a `state` field every seat agrees to
honour, and a signal topic every seat must spell identically. **None of that is
a mechanism. All of it is discipline, enforced by nothing.**

| Concept | Definition |
|---|---|
| **Seat** | a named role, independent of who occupies it. **The addressable identity.** Senders address seats; the daemon resolves to the current occupant |
| **Generation** | one occupancy of a seat by one session. Monotonic per seat. **A message carries the generation it was addressed to**, so a receiver can tell it is not the session the sender believes it is talking to - which is a check no amount of care can do today |
| **Lineage** | the chain of generations. A successor can prove it is the successor; a stranger cannot claim to be |

**The seat states, and the transitions:**

| State | Means | Sending to it |
|---|---|---|
| `VACANT` | nobody holds it | refused, and the sender is told the seat exists and is empty - not that the name is unknown |
| `LIVE` | one session holds it | delivered |
| `HANDING_OFF(successor)` | **two sessions legitimately occupy one seat**, the predecessor briefing the successor | **queued for the successor, not delivered to the closing session.** This is the whole reason the state is published |
| `ORPHANED` | the occupant's witness is observed dead | queued against the seat, delivered when it is next claimed. **Not discarded** |

**`HANDING_OFF` is legitimate and rig must stop reading it as a fault.** §5
refuses a second registration of one identity with `CODE_DENIED`, which is
correct for two unrelated programs racing for a name and **wrong for a
succession**.

**Measured 2026-09-11, by building both cases and comparing the strings: the two
refusals are BYTE-IDENTICAL.** Not similar - identical. The message carries the
**holder's** rendered identity and nothing whatever about the newcomer: not its
session, not its pid, not whether it declared the same commands as the holder or
a completely unrelated set. So a client has one code and one sentence, **and
neither varies with the case it is in.** It cannot choose between waiting for a
predecessor to finish closing, retrying, and failing outright - and neither can
a person reading the log.

**The identity that IS in the message is not a discriminator either.** It is the
holder's session id, minted per connection by the daemon and never sent to
anybody, so no caller has ever seen it. It is a label, not something a newcomer
can compare itself against.

**Two further facts a specification here has to know**, both pinned by test:
there are **two** duplicate refusals in the tree - the kernel's registration
guard and the daemon's routing map - and only the kernel's is reachable on this
path, so a change to one must account for the other. And **the prose half is
separable and cheaper**: §9 says a failed call never returns prose, and this
refusal returns exactly that, with all four structured fields empty. *"Wait for
the holder to go"* and *"take another name"* are different fixes and the wire
expresses neither. **That can be closed without ruling on succession at all**,
and it should be, because it is useful under either ruling.

**Death is the normal termination here, not the exception**, so `ORPHANED` is
the state a seat spends real time in and its behaviour is load-bearing rather
than a tidy-up path.

#### 4. Retraction

**An agent takes back an item it posted, before the human or the peer acts on
it.** Absent today under every wording.

**The reason it is a correctness property and not a convenience:** a warning
waits on a supervisor's screen until it is dismissed. **A "build failed" whose
build has since been fixed is a false statement rig is making on the agent's
behalf**, and it is worse than no notification at all because the supervisor
acts on it. §12's auto-dismiss is the *timer* half; the poster withdrawing a
claim that has become false is the half that was missing.

**A retraction returns what actually happened to the item** - never posted, seen
but not acted on, or already acted on - because those are three different
situations for the agent and only the daemon knows which.

#### 5. A lease name is registered with the resource it protects

**Two actors guarding one resource under two names are both unlocked and
neither finds out.** Non-deterministic actors invent plausible names as a matter
of course, so this is the normal case rather than the careless one.

**This estate has already paid the bill**: one session took `rig-makefile` and
another `repo:rig-shared-build` for the same file, and the file was found dirty
mid-edit. **Nothing else in the mechanism backlog has a recorded local
failure.**

| | |
|---|---|
| **A name is registered with what it protects**, and is discoverable before use. Asking "what guards this path" is a call, not a convention in a document |
| **An unregistered name is not silently honoured.** What the daemon does with one is stated: it is granted and **flagged as unregistered to both the holder and to `rig doctor`**, because refusing it outright breaks every ad-hoc use and honouring it silently is the bug |
| **Overlap is tested by SCOPE INTERSECTION, not name equality** | two names over one path collide and rig says so. Name equality is precisely the test that failed |

#### 6. Durability of an agent's own work

**The default outcome of every agent session today is that its findings die with
it**, and that is treated as a discipline asked of the author rather than as a
mechanism. It is a mechanism. An agent's context IS its memory, and the estate's
supervised sessions end by budget exhaustion far more often than by finishing.

| | |
|---|---|
| **A durable location is assigned and SEEDED BEFORE the work begins** | not chosen by the worker when it decides it has something worth keeping. A worker that has to decide where to write has already lost the case where it dies before deciding |
| **Checkpoints at PHASE BOUNDARIES** | not at the end, and **never in a termination handler.** A handler does not run on `kill -9`, on an out-of-budget stop, or on the container going away - which are the three ways this actually ends |
| **Checkpoint writes are atomic** | a half-written checkpoint is worse than none, because it reads as a record |
| **Work not recorded is UNKNOWN, never done** | the reader of a checkpoint store is told which phases have records and which do not, and an absent record is never rendered as an absent phase |
| **Child work is persisted independently of its parent** | a spawned worker's findings must not die because the session that spawned it did. Parent closure is a supervised event with obligations, not a cascade |

#### 7. At risk, and the escalation ladder

**A session that knows it is nearly out of context or budget is the only actor
that can say so.** Nothing else on the machine can see it: not the supervisor,
not the human, not a peer. So there is no monitoring answer to this, only a
declaration one.

| | |
|---|---|
| **A session declares AT RISK with a reason and an estimate** | budget, context, a deadline it will not meet. The reason matters because the remedies differ |
| **The ladder, and each rung names who decides** | **continue** (the session) → **checkpoint now** (the session) → **hand off to a successor** (the session, and it is the rung that must not need a human) → **stop** (a human, or the hard floor below) |
| **A hard floor, below which rig acts without asking** | because the rung that needs a human is the rung a dying session cannot reach. At the floor, rig checkpoints and marks the seat `ORPHANED` itself |
| **Announce-with-a-countdown, and it proceeds unless stopped** | the one interaction shape that costs the supervising human nothing when they agree, which is the common case. It is the ladder's escalation primitive and not a separate feature |

**Why this is ranked below durability and not above it.** At-risk detection
without durable work is a session announcing its own death and taking its
findings with it anyway. **Durability makes the announcement worth making.**

### Continuation slots, so an agent can hand off to itself

An agent that is about to reset its context - because the context is nearly full, not because
the work is done - needs to leave a note for the process that replaces it. Today that note goes
to the logbook: a path decision, a schema decision, a commit. That is right for work worth
keeping and wrong for a note whose whole lifetime is the minute between one context and the
next.

**A continuation slot is the versioned blackboard with a scope of one. It is not a new
service.** Same compare-and-swap, same revision per change, same watches with a cursor. What it
adds is three rules, and each exists because the obvious design fails without it:

| Rule | Why it is a rule and not a convention |
|---|---|
| **A slot carries a label, and the label is mandatory** | The replacement finds its slot by listing candidates and asking (below). `7f3a2b · 14:22 · 41 KB` cannot be chosen from, and an optional label is always missing exactly when there are three rows to pick between |
| **Existence is readable estate-wide; the payload is not** | A slot's label, age and owner are ordinary introspectable state (§14). Its contents are declared `sensitive`, so they are absent from every other reader's answer by the mechanism §15 already uses - not filtered by a second access-control door, which §14 does not have |
| **A slot expires, and the expiry is visible before it fires** | Two-step expiry, the same as a lease above. An abandoned reset otherwise leaves a row forever, and by the third week the question has eleven candidates and stops being read |

**How the replacement finds its slot, which is the only genuinely hard part.** A respawned agent
is a **new connection with a new client id**: §5's session token survives a *reconnect*, and a
context reset is not one - the token died with the context that held it. So nothing the old
agent knew can name the slot, and the replacement does not try to reconstruct it. **It lists
what is waiting and asks.** That is deliberately the mechanism the owner's own resume flow
already uses over the logbook - scan every candidate, newest first, with its title, and ask one
question when they conflict. Nothing is derived from the cwd, so two agents in one checkout do
not collide by convention, which is the failure that killed the two earlier designs.

**This inverts the isolation requirement, and the inversion is the point.** "Each agent's
clipboard is private" and "a respawned agent can list what is waiting" are only jointly
satisfiable one way: **isolate the values, not the existence.** Per-connection isolation would
hand the replacement an empty list, because it is a different client than the one that wrote -
so the listing is scoped to the uid, and privacy comes from the payload being `sensitive`
rather than from the row being hidden.

**What this does not claim.** It does not avoid a disk write - `rigd` writes slots to the WAL,
so they survive a daemon restart, which is the property that makes them worth trusting at all.
What it avoids is the *agent* having to choose a path, invent a schema and make a commit for a
note it expects to consume in ninety seconds. That is the whole benefit and it is smaller than
"no persistence", which is why it is written here rather than sold as a headline.

**On the CLI it is `rig continue`**, listing slots with label, age and owner, and `--take` to
claim one. Not `rig loose-ends`, which is taken and means declarations that stopped pointing at
anything (§8).

### Safety defaults, because agents get killed mid-operation

- Every lease has a TTL, and an absolute one on `CLOCK_BOOTTIME`. There is no infinite hold.
- Every `await` has a deadline. There is no infinite block.
- A lock is never held across a question to a human. rig refuses the combination. **One
  exception:** a `confirm` or an elevation that gates the lease-holder's *own* call is not a
  question across a lock - nobody else is blocked on the answer. Expiry is frozen on every
  lease that caller holds for the duration of the question, the same freeze applied after a
  suspend below (§13a, §14).
- Every primitive's crash semantics are written down and tested, not inferred.
- **What a contended acquisition reveals about other clients is written down in §14** and
  accepted there, rather than discovered later: BUSY proves a peer exists, and a refused
  acquisition names leases, never peers.

### Observability of coordination, which is where it wins

Nothing else in this class shows you what is happening. rig does, on every surface.

| View | Shows |
|---|---|
| Crew | Everyone, their purpose, activity, what they hold, what they await |
| Wait-for graph | Live, drawn. A cycle is visible before it is a mystery |
| Contention | Which lease is hot, who waited longest, queue depths over time |
| Timeline | Every coordination event, replayable, filterable by peer |
| Post-mortem | "Why was this agent blocked for four minutes" answered from the record, without anyone having been watching |

### What "fully proved" has to mean

The cutover from AgentBox does not happen on a feeling. Four gates, all of them tests.

| Gate | The proof |
|---|---|
| **Specified** | Lease, fencing and transaction semantics written as a model, with property tests generated against it |
| **Simulated** | Deterministic simulation testing: thousands of randomised interleavings with injected crashes, restarts and slow clients, asserting linearizability. Seed printed on failure so any violation reproduces exactly |
| **Adversarial** | The specific attacks: two holders of one exclusive lease, a stale fence token accepted, a watch that misses a revision, a claimed task run twice, a deadlock not detected. Each is a named test that must fail before the fix and pass after |
| **Shadowed** | Replaces "baked", which had two readings and the plan stated neither. During the shadow period **AgentBox remains the sole authority**: every coordination call still goes to it, and rig receives the same request in the same order and records what it *would* have decided. Divergence then means something, and there is never a second lock namespace over one resource. Cutover flips the authority, not the traffic - and **the dual-write path is in M7's ships column**, because it is the gate's only apparatus and it was previously in nobody's milestone |
| **Fenced** | The fifth gate, and the only one that tests the boundary rather than rig's own model. For every class of guarded external resource, name the enforcement mechanism - rig-owned pid, pidfd witness, cgroup kill - and prove by test that a stalled holder's **work stops**, not merely that its token is rejected. Without this the other four are self-referential |

**Simulated is only adequate if it injects the right failures**: lost replies (apply the
operation, drop the response, watch the client retry), a clock jump across suspend, and a
restart mid-transaction. Add to Adversarial: *an operation whose reply was lost is applied
exactly once when the client retries.*

Only after all five does the agent tooling point at rig. AgentBox stays running and untouched
until then, and stays available afterwards until a month has passed with no regression.

---

## 17. Footprint

rig is resident all day. A platform that costs the machine something noticeable has taken back
what it gave. The budget is numbers, and `make bench-idle` fails the build when one is missed.

**One number in this section is currently a claim about `rigd`, not about rig.** The tray must
run with no window open (§11), the toast layer is a frameless webview (§12), and the daemon
links no webview and stays cgo-free (§22) - so a third always-resident process is implied by
those three lines and named in none of them. It has no lifecycle, no autostart entry, no crash
policy and **no row in the table below**, which means the number that actually matters - what
the estate costs all day - is unmeasured while the number that is measured looks like an
answer. **Decided at M8, before the tray ships**, and the decision is between a named third
process with its own budget row, cgo in the daemon for a Linux systray, or dropping the
frameless toast for the freedesktop fallback §12 already carries. Recorded here rather than at
M9, because M9 is after the tray.

**The previous budget was set without measurement and three of its seven lines were
unreachable.** §22's own dependency list, assembled into a daemon that does nothing, measured
**43.43 MiB resident, 43 wakeups/second, 0.150% CPU** - with no rig code in it at all. What
follows is built from that ladder rather than from a wish.

### Where the bytes went, measured

| Rung | Idle RSS | Delta | In the budget? |
|---|---|---|---|
| bare Go, `time.Sleep` only | 1.88 MiB | - | yes |
| + gRPC server on a unix socket | 11.68 | **+9.80** | **no. gRPC is not bought** (§2, §5f) |
| + `modernc.org/sqlite` **linked, never opened** | 13.22 | +1.54 | yes. Laziness cannot reclaim a link |
| + that database opened, WAL, 10k rows | 17.96 | +4.74 | **transient only** - migrations open, run, close |
| + OTel trace + metric + OTLP exporters | 23.58 | +5.62 | yes |
| + koanf, cobra, jsonschema, ring, timers, 10 programs | 34.54 | +10.96 | yes, with the ring cut from 8 MB to 2 |
| + bubbletea, huh, glamour, lipgloss, keyring | 43.43 | **+8.89** | **mostly no - that is `rig`, not `rigd`** (§2). But **keyring is in this rung and belongs to `rigd`** (§22), so this line is not yet a clean split and `make bench-size` must separate it |

**Two structural facts the old budget did not know.** Go RSS tracks binary size, because text
and rodata pages are resident - so an RSS budget without a **binary-size budget** is an RSS
budget with no cause. And a single 1 Hz ticker in Go floors at **5.60 wakeups/second**, so
"< 1 per second" was never reachable by any amount of care.

### The budget

| What | Budget | Measured by |
|---|---|---|
| **`rigd` binary size** | **< 20 MB, and ratcheted** | `make bench-size`. A PR adding more than 1 MB fails unless this line is edited in the same commit. This is the budget that causes the next one. **Edited at M1 slice 5: 6.29 -> 7.75 MB**, and the whole 1.45 MB is `santhosh-tekuri/jsonschema` v6, which §22 pins. It has to be in the daemon rather than the client, because a program cannot trust a caller to have validated its own arguments (§5e). 39% of the budget spent at M1 of 16, and the next surface that wants a library this size is the one to argue with |
| Daemon idle, resident memory | **< 20 MB** | `make bench-idle`, after 60s quiet. Reachable only with gRPC and the TUI out: 1.88 + framing + 1.54 + 5.62 + ~3 + 2 |
| Daemon idle, CPU | **< 0.1%** | same |
| **Wakeups above the unavoidable timer floor** | **< 2 per second** | same. The floor is measured and printed beside the number, because budgeting against zero is budgeting against Go |
| Wakeups with no program connected | **< 1 per second** | same. Reachable: bare Go with no timer measured 0.00, and the history flush is armed by the first record (§15), not by an interval |
| Overhead per registered program | **< 500 KB** | `make bench-scale` at 1, 10 and 50. Measured 23 KiB, 21x headroom - this line has never been the problem |
| Cold start to first command served | **< 100 ms** | measured in CI. Daemon-only measured 83 ms; daemon plus TUI was 115.81 ms, which is why they are two binaries |
| A no-op command, end to end | **< 10 ms** | the wire is 6.2µs of it (§4); the rest is process |
| A fully recorded call | **< 2 µs** | history + span + slog, §15 |
| The window, when closed | **zero** | it is not the same process |
| **`righand`, when nothing is being driven** | **zero** | same argument, and the same mechanism. `rig hand` starts it, a released lease can stop it, and the X11 dependency measured at **1,015,911 bytes** is never linked into `rigd` at all (§5m) |

**A warning about the gates themselves:** the problem is entirely fixed cost, so `bench-scale`
will stay green forever while `bench-idle` is the one that fails. Do not read a green scale
benchmark as evidence of anything.

**Eight rules that deliver those numbers.**

- **Two binaries.** `rigd` is the daemon; `rig` is the CLI and TUI. Recovers 8.89 MiB and 27 ms
  of cold start, and it is a build-graph change only.
- **The window is a separate process.** The daemon has no GUI dependency, does not link Wails,
  and does not link a webview. `rig window` starts it; closing it returns every byte.
  This also confines the Wails beta to a process that can crash without touching anything, and
  keeps the daemon cross-compilable with no cgo. It is also what makes the lifecycle notice in
  §5g drawable.
- **No gRPC.** Hand-framed length-prefixed protobuf on one socket (§5f).
- **`GOMEMLIMIT` and `GOGC` are set explicitly**, in the unit file and in the Makefile. Neither
  appeared anywhere in this plan or the Makefile before, which means the Go heap was being left
  to a default nobody chose.
- **One memory arena, one owner.** The history ring, log and trace buffers share a single
  declared ceiling rather than three independent ones; the old 8 MB history line alone was 41%
  of the whole RSS budget.
- **Nothing polls.** Everything is epoll-driven. Timers are coalesced onto one wheel - measured:
  ten coalesced tickers cost 8.60 wakeups/s against 5.60 for one, so the rule works and is
  worth far less than the floor it sits on.
- **Programs and services are lazy.** A program with `autostart = lazy` is not started until a
  surface needs it; a service nobody has used is registered but not initialised, and §5k makes
  that real by having programs declare which services they use. Linked-but-unopened cost is not
  reclaimable this way, which is why sqlite is a budget line rather than a laziness win.
- **A regression fails CI.** The budget is a test, not an aspiration.

## 18. Supervision and failure

- **Register:** handshake, wire version check, declaration validated against its schema,
  capabilities granted, then the program appears on every surface. A failure at any step is a
  quarantine with the reason, not a retry loop.
- **Start:** rig builds the child's environment rather than inheriting its own. **No grant
  travels in it, because no grant exists as a variable** - `introspect` is decided from the
  connection (§14), so there is nothing to scrub and nothing a future reader can reintroduce
  by forgetting to. What the constructed environment *does* carry is an allowlist, written
  down because a constructed environment silently governs the estate and an empty one breaks
  four subsystems:

  | Variable | Without it |
  |---|---|
  | `DBUS_SESSION_BUS_ADDRESS` | `go-keyring` cannot reach the secret store (§22), and §12's D-Bus toast fallback is dead |
  | `DISPLAY`, `WAYLAND_DISPLAY`, `XAUTHORITY` | every `needs_display` command fails, including §5e's own worked example `snapper.capture-region` |
  | `SSH_AUTH_SOCK` | a program that shells out to git over ssh hangs on a passphrase nobody can answer |
  | `TZ`, `LANG`, `LC_ALL` | timestamps and collation drift from every surface that renders them, and `TZ` feeds the DST hole in §33 |
  | `HOME`, `PATH`, `XDG_RUNTIME_DIR`, `XDG_*_HOME` | nothing resolves - a program cannot find its own socket, config or cache |
  | `RIG_*` | the program's own registration handles: socket path, program id, wire version. Never a grant |

  Everything else is dropped, and the allowlist is config, resolved through §11 like anything
  else, so a program needing one more variable declares it rather than getting the parent's
  whole environment back. Conformance item 22 already reads the child's environment; it
  asserts the positive half in the same test.
- **Health:** on the interval, with a timeout. Three failures is degraded, five is a restart.
- **Restart:** exponential backoff inside a budget. Exceeding it is quarantine - a visible state
  with the full history and a manual restart, never a silent disappearance.
- **Crash:** the pane becomes a panel with the exit status, the last 200 log lines, the restart
  decision and the countdown. Nothing else changes.
- **Hang:** every call has a deadline. A program that misses them is degraded, then restarted. A
  hung program can never block a rig goroutine, and a lint rule keeps it that way.
- **Flood:** per-program rate limits on events, logs and notifications, with the drop count shown.
- **rig dies:** every program keeps running, tolerating the absence (§5g). On restart all
  reconnect, present their session token, and are told explicitly what was lost. In-flight
  commands are marked interrupted with partial output kept, and are **never silently replayed** -
  a retry is the client's decision, made against the `idempotent` property and deduplicated by
  the request id on the wire (§5f).
- **Shutdown:** the lifecycle notice first (§5g), then SIGTERM, grace period, then SIGKILL.
  Programs rig did not start are never killed. A `DRAINING` notice moves every held lease to
  ORPHANED rather than FREE, so a restart cannot hand one resource to two holders (§16).
- **Upgrade:** stop, swap the binary, start. **No handover.** Measured against a built handover
  path with fifteen connections and real fd inheritance: worst client gap 4.4-5.0 ms plain
  against 2.5-3.4 ms handed over, and the handover's 30-45 refused dials are removed by the
  notice alone. The entire marginal benefit is **1.9 ms per upgrade**, or 5.2 seconds a year at
  weekly upgrades, against four defect classes - a replay path less safe than `kill -9`, a
  fencing counter that breaks in the 0.86 ms serialise window, an unversioned second wire
  format, and value that is highest exactly when the state struct changes daily.
- **A hosted program panics** (§5j): the invoker recovers it, quarantines that program with the
  stack and the reason, and keeps serving. Twice inside the restart budget and it stays
  quarantined.
- **A missed schedule fire.** The laptop suspends nightly, so this is the normal case rather
  than the exception, and cron's answer (drop it) and anacron's (fire them all) are both wrong
  here. The policy is decided by the command's own `idempotent` property: **idempotent** - all
  missed fires coalesce into exactly one run at resume; **not idempotent** - nothing fires,
  and the misses are reported in the schedule view with their times and a one-click run. Either
  way the coverage log records the gap (§15), and expiry is frozen for one TTL after resume
  (§16).


### The supervisor as a machine, not a description

**Everything above this line is prose, and that was the finding.** §34 named
§18 as the calibration point at the wrong end: five state names that appear only
in §5's candidate section, no state set, no transition table, no owner, no
statement of what is illegal. **The consequence is not cosmetic** - §5's
"declared state machines, if the supervisor can be its first client" cannot be
tested while the supervisor is sentences, because there is nothing to express.

**The states, and this is the set. There are no others.**

| State | Means |
|---|---|
| `STARTING` | rig has launched the child and it has not completed the handshake |
| `HEALTHY` | registered, and showing evidence of progress (below) |
| `DEGRADED` | reachable, and failing its health definition |
| `RESTARTING` | inside the restart budget, backing off |
| `QUARANTINED` | out of budget, or failed registration. **Visible, with history, and it stays until a human acts** |

**The transitions, with the trigger and who owns causing it.**

| From | To | Trigger | Caused by |
|---|---|---|---|
| - | `STARTING` | launch | rig, or a program connecting unbidden |
| `STARTING` | `HEALTHY` | handshake and declaration validated | the program |
| `STARTING` | `QUARANTINED` | any registration step fails | rig. **Not a retry loop** |
| `HEALTHY` | `DEGRADED` | three health failures, or a missed deadline | rig's health check |
| `DEGRADED` | `HEALTHY` | health recovers | rig's health check |
| `DEGRADED` | `RESTARTING` | five failures | rig's restart budget |
| `RESTARTING` | `STARTING` | backoff elapses, budget remains | rig's restart budget |
| `RESTARTING` | `QUARANTINED` | budget exhausted | rig's restart budget |
| any | `QUARANTINED` | a hosted program panics twice in budget (§5j) | the invoker |
| `QUARANTINED` | `STARTING` | **a human, and only a human** | manual restart |

**The illegal transitions, and what happens on one.** Everything not in the
table above is illegal. **An attempted illegal transition is not ignored and not
best-guessed: it is recorded as a supervisor fault and the program is
quarantined**, because a supervisor that cannot account for its own state is the
one component whose confusion is not survivable. This is the row the prose could
never have: *inferable* transitions are exactly the ones nobody checks are total.

**Four actors can move a program and they are named here** because "three
failures is degraded" hid that: the health check, the restart budget, the
invoker's panic recovery, and a human. **A `DRAINING` notice moves leases, not
program states** (§16) - it is not a fifth actor, and reading it as one is the
mistake this row prevents.

### Health is evidence of progress, not evidence of responsiveness

**"Responding" is the wrong health definition for the actor this estate
actually supervises.** A service that answers is working. **An agent session
that answers may be idle, stuck, looping, or parked on a question nobody can
see** - every agent-specific failure is responsive, so a liveness check passes
through all of them.

| | |
|---|---|
| **Health is evidence of PROGRESS** | a renewal carries a progress marker. **A renewal emitted by a background thread proves the process runs and nothing else**, which is the check that cannot fail and therefore cannot help |
| **Idle-and-not-blocked is UNHEALTHY, with a threshold** | not a rest state. A session doing nothing and waiting for nothing is the characteristic failure, and it looks identical to diligence from outside |
| **A waiting session declares WHAT it waits for** | so waiting is distinguishable from stuck. Without it the two are the same observation, and one of them is fine |
| **Parked on a question nobody has seen is its own state** | it is neither progress nor a fault, and rendering it as either loses the only action that helps, which is showing the question to a human |

**This is the same defect as the roster's `partial`, and as four measurement
bugs this repository has already shipped:** a check that cannot distinguish
"nothing is wrong" from "the check did not run". **Being told a program is
responsive answers a question nobody asked.**

### What survives what: the state ownership matrix

**§18 promises programs "are told explicitly what was lost". That is the
promise; this is the mechanism.** Scattered per-subsystem prose means no reader
sees the whole thing and programs guess.

**Six events reduce to three, and the reduction is a finding rather than a
convenience.** An *upgrade* is stop-swap-start with no handover, and neither
version number is ever compared (§21), so **an upgrade is exactly a daemon
restart**. A *reboot* is a daemon restart in which every program restarts too. A
*program restart* is holder death for everything that program put in. So the
columns are: **daemon restart**, **holder death**, **seat succession**.

| State class | Daemon restart | Holder death | Seat succession |
|---|---|---|---|
| Declaration | **lost; the PROGRAM rebuilds it** by re-registering on reconnect | lost with the connection | n/a - programs do not occupy seats |
| Lease | **survives**, with expiry frozen one TTL past resume (§16) | **ORPHANED, not FREE**, until the witness is observed dead | held leases transfer to the successor generation |
| Coordination claim | **survives** | **ABANDONED, inheritable by one CAS write** | transfers |
| Seat | **survives** | **ORPHANED**, and the seat outlives every occupant | the point of the mechanism |
| Blackboard key, durable class | **survives** | survives; ownership goes abandoned if claimed | n/a |
| Blackboard key, session class | lost | lost | lost |
| Subscription | **lost; the CLIENT rebuilds it** from its cursor, and `gap: true` if retention has moved past it | lost | the successor resubscribes from the seat's cursor |
| Queued message | **survives**, queued against the seat | **survives** - this is what queueing against a seat rather than a session buys | **delivered to the successor** |
| Pending confirmation | **DENIED, never carried across** | denied | denied. **An approval outliving the invocation that asked for it is the hole §13a closes** |
| Detached operation | survives; output buffered | survives - it is rig's child, not the caller's | reattachable by the successor |
| Schedule entry | **survives**; missed fires resolve by `idempotent` (above) | survives | n/a |
| Config | **survives**; re-resolved through §6's layers | n/a | n/a |
| Single-instance claim | released and retaken | **released by design** - the flock lives on the descriptor | n/a |

**Every cell has an answer, "nothing survives" included, and the side that
rebuilds what is not preserved is named where anything does.** That naming is
the half that makes the table usable: a program that knows a declaration is lost
and that rebuilding it is *its* job needs no negotiation on reconnect.

**What is true TODAY is a different document and deliberately so:**
`logbook/projects/rig/state-ownership-today-2026-09-11.md`, measured by reading
every piece of state `internal/` holds. **The headline: nothing rig holds
survives anything, and nine of these thirteen classes do not exist yet.** The
two tables are meant to diverge - one is the target, the other is the distance
to it - and **merging "does not exist" with "exists and is lost" is the thing
that would make both useless.**

---

## 19. The conformance suite

`rig verify ./program` is the gate, and `pkg/rigtest` is the same battery as a Go package so a
program's own CI runs it. This is what makes the contract real.

1. Handshake, wire version negotiation, refusal on mismatch
2. Declaration is valid against the registration schema and claims nothing undeclared
3. Health responds under load and while a command is hung
4. Config schema is valid JSON Schema 2020-12 and round-trips unchanged
5. Config rejection carries a reason a human can act on
6. Every command's argument schema validates, and bad arguments are refused, not half-run
7. Cancellation honoured within 1s, with a terminal event on every path
8. 100 concurrent invocations stay correct and bounded
9. Deadlines honoured: a call given 100ms returns or errors within 200ms
10. Data paging is stable under concurrent writes; subscriptions drop nothing
11. Every undeclared capability call is denied and logged
12. Fuzzed inputs on every message do not panic the program
13. SIGTERM shuts down cleanly inside the grace period
14. SIGKILL mid-command leaves no corrupt state
15. Log hygiene: no secrets, no unbounded lines, valid JSON where claimed
16. **Golden wire: every retained fixture, not the last release.** One frozen conformance
    fixture per wire major, built the day that major ships and archived with its vendored
    source, run against HEAD. Each asserts a recorded **behaviour transcript** - which default
    applied, whether a confirm fired, what each enum decoded to - because a successful round
    trip proves nothing about meaning
17. Runs correctly with rig absent: the tolerant client, the resolved snapshot, one typed
    `unavailable` error, and no second implementation of anything (§5g)
18. **Declaration completeness:** `effects`, `idempotent` and `sensitive` present on every
    command, `coverage` present, `semantics_gen` present. A missing safety field is a refusal,
    not a default
19. **Nothing sensitive is recorded.** A known token passed through a declared-sensitive field
    appears in no segment, in no encoding, at any sampling rate
20. **Renderer fidelity:** every declared schema shape either has a renderer that can express
    it, or an explicit terminal fallback. Asserting that *a* renderer exists is not the test
21. **House rules are enforced in the invoker**, proven by running one denied command through
    every surface from one test
22. **A started program's environment is the constructed allowlist and nothing more**, read
    from `/proc/self/environ` inside a program rig started (§18). Both halves are asserted:
    every variable §18 lists is present and correct, and **no variable outside the list
    survives**, including any `RIG_*` name that is not a registration handle. No grant can
    appear here because no grant exists as a variable (§14) - the item stands so that a future
    reader who reintroduces one fails a test rather than a review
23. **A registered program is scoped on its own connection, and stays scoped.** From inside a
    started program: every estate-wide view returns its own scope only, for the life of the
    connection, and no handshake, reconnect or capability grant changes that (§14). This is
    the item a hosted program must pass, and the reason the predicate is on the connection
    rather than on the environment
24. **A program's declared element list is complete and resolvable** (§5h R3). Every name in it
    exists in the kit at the declared generation, and a name that does not **refuses the
    registration** rather than reaching a page. Item 18 asserts declaration completeness for
    `effects`, `idempotent`, `sensitive`, `coverage` and `semantics_gen`; the element list is
    the sixth and was enforced by review until 2026-09-10
25. **Every kit element renders, in the window, at every generation rig still serves**, and the
    pane holding it declares a terminal fallback (item 20's disjunction, applied to a tier that
    has no declared schema shape). **Asserting that the element exists is not the test**: it is
    rendered and read back. This item is what stops R2 being satisfied by a stub
26. **A hosted program passes items 1-25 unchanged**, from the same package, with no branch in
    the suite. Item 22 is the one that would have been impossible under the old design - a
    hosted plugin is compiled into `rigd` and shares its environment, so it is exempted from
    the allowlist half and held to item 23 instead, which is the assertion that actually
    binds it (§5j)

`fakeapp` is the misbehaving reference program and ships in the repo. It hangs, crashes, leaks,
floods, lies about its schema, ignores cancellation and returns garbage, each on a flag.

**It also behaves, once, in the hardest way any program will.** `fakeapp longrun` is the
`interactive-stream` case from §9: it runs for as long as it is told, emits 20,000 frames with
a cursor, stops in the middle to ask a permission question that has to be answerable from a
toast, reports a per-run cost as a typed field, and parks rather than fails while it waits.
That case exists so the shape is proved before the program that needs it is written - the
alternative is discovering the contract's gaps from the program, which is when they are
expensive.

---

## 20. Testing strategy

| Layer | Approach |
|---|---|
| Wire contract | 100% coverage, table-driven, plus the golden-wire compatibility test |
| Supervisor and scheduler | `testing/synctest` for all timing. Deterministic, microseconds, no `time.Sleep` in any test |
| Chaos | 10000 randomised kill / hang / flood / restart sequences against `fakeapp`, seed printed on failure |
| Suspend | A real-clock test with an injected `CLOCK_BOOTTIME` jump. `synctest` cannot model suspend, and this laptop suspends nightly, so the mandate above would otherwise make the whole class untestable |
| Isolation | Observational equivalence over two worlds (§14), including the runtime directory, the config tree, the state tree and the process table |
| Authorisation | Two assertions §14 and §12 both rely on and neither could test as prose. **Do Not Disturb does not suppress a gating `ask`:** with DND on, a command needing `confirm` still reaches a surface and still blocks, while a lifecycle notice in the same run goes to the centre (§12, §13a). **An elevation prompt names its caller:** the rendered question carries principal, client kind, pid, command and arguments, and an interactive caller's question arrives on that caller's own surface rather than on whichever is present (§14) |
| Introspection | §14's **three-run battery**, and it lives here rather than in §19's program-side suite: it is a property of *rig* over several clients, and the program under test is refused `introspect` and has no peers to be complete about. Run 1 ungranted A sees nothing of B; **run 2 A sees B completely - a scoped answer where a complete one was owed fails as hard as a leak**, modulo the coverage log; run 3 no `sensitive` value surfaces at any sampling rate |
| Fuzz | Registration parser, JSON Schema inputs, frame decoding, the postMessage bridge |
| CLI | testscript golden transcripts for every command including failures |
| Generated UI | Golden snapshot per schema shape |
| Frontend | Vitest on the bridge and stores; Playwright driving the real window - click, Esc, Tab, Enter, tray, theme switch, crash and recovery |
| Contrast | Measured in a real browser, **both themes** - the token set is generated (§6), so the gate asserts the generator's output, not one static page. A failing ratio fails the build. **`make contrast` is the gate and CI runs it**; until 2026-09-10 it invoked a tool that had never existed and CI never called it, so the rule was prose |
| Contrast at element scale | **Every kit element, on every ground text lands on, in both themes** (§5h R5). Three things the page-level gate does not see, each found by measurement on 2026-09-10 and each now asserted: a **focus ring** is non-text contrast and owes 3:1 against the surface behind it, not 4.5:1 against nothing; **`opacity` and `color-mix` are invisible to a DOM audit**, which reads the declared colour and not the painted one, so anything dimmed that way is sampled from pixels; and the ground set is the **five** in `theme.js`'s `textGrounds`, tint included, not the four the catalogue draws |
| Integration | The pilot runs the real `shelf` binary in CI, not a mock |
| Gates | 90% on `internal/`, 100% on wire and stub, no call without a deadline, no program id in rig code, no registry handle outside the kernel, no meaningful enum zero, kernel and stub symbol budgets, `make modules-matrix` green, `make bench-size` within the ratchet |

---

## 21. Versioning

- The wire is versioned by major in the path. rig serves **every** wire version it has ever
  shipped; that is the promise that makes independent upgrade real.
- **One frozen conformance fixture per wire major**, built the day that major ships, archived
  with its full vendored source tree so it can be rebuilt in 2035, and **retained forever**. The
  previous plan archived only the last release, and "N works against N+1" repeated is not
  "1 works against N".
- **One fixture's shape is declared unproven, and this is where that has to be said.** The
  `interactive-stream` case (§9, §19) is modelled on `graft run`, a program that does not
  exist yet, and its hardest row - an inbound question on an outbound stream - has no working
  implementation anywhere in the estate: the graft session measured Claude Code 2.1.267
  emitting *no* permission event under `--permission-prompt-tool`, and agentbox runs
  `bypassPermissions` for the same reason. **Retained forever applies to a fixture, not to a
  guess**, so this one may be corrected once a real answered prompt exists, and the correction
  reads as planned rather than as a broken promise. Everything else in §9's five rows is
  ordinary and stands.
- **Each fixture asserts a behaviour transcript**, recorded at ship time: which default applied,
  whether a confirm fired, what each enum decoded to. A round-trip test passes while a new
  `effects` value silently decodes to zero on an old binary and a destructive command reports
  itself read-only.
- **Meaningful enum zero is banned.** Every proto enum reserves `*_UNSPECIFIED = 0`; an unknown
  value is a hard refusal at the daemon boundary, never a zero-value fall-through. Lint gate,
  §20.
- Capability differences are negotiated at connect: the program says what it supports, rig uses
  what it has, and the gap is visible in the Programs view rather than a failure.
- The stub's public surface is enumerated in one file with a symbol budget in `make ci`, and
  every connection reports its `stub_build`, so rig can name every program carrying an old pipe.

### The version that is not the wire: `semantics_gen`

"Old declarations keep parsing" is not the property that matters. Turning `network` from a
best-effort proxy into a real namespace breaks a 2027 declaration that parses perfectly - the
bytes are fine and the meaning moved. Nothing in a wire major can express that.

So a registration carries **one integer, `semantics_gen`**, distinct from the wire major. It
pins, for that program, for its lifetime: what every capability name means, what every
declaration default is, and which JSON Schema dialect and validator behaviour apply to its
schemas. rig carries a generation table instead of pretending meanings are immutable. It is the
cheapest option available, and the only version number that can ever be *retired* - once no
registration claims a generation, its row goes.

**Measured 2026-09-11: neither version number is ever COMPARED, so "upgrade"
is today exactly "daemon restart".** Two findings, from reading every use:

- **The wire version is announced and never compared.** It is read in one place
  and written into the handshake response. An old program and a new daemon
  transact regardless.
- **`semantics_gen` is validated and never compared against a previous
  registration.** Validation refuses a non-positive value, registration logs it,
  and the projection carries it - but nothing compares one registration's
  generation to the last one's, **which is the comparison the field's name
  implies.**

**So the generation table above is specified and unbuilt**, and the state
ownership matrix has two cells it cannot fill for that reason: "what does an
upgrade do that a restart does not" is undecidable from the code because today
the answer is "nothing". **Whether it should stay nothing is an open question
and it is Boris's**, because it decides whether a program can be running against
a meaning that has moved underneath it.

### The escape hatch, which costs nothing now and everything later

Serving every wire version forever, with no stated end, quietly grows the test matrix without
limit and leaves old programs hollowing out unannounced. There are 2 hits for deprecation
language in this document and neither is a policy. Three sentences fix it:

1. **A support window ships with the version.** Every wire major declares, in this plan, on the
   day it ships, the date it moves from *supported* to *frozen*. A date, not a feeling.
2. **Two states, explicitly.** *Supported* means new services are reachable and the full
   conformance suite applies. *Frozen* means it still connects and existing behaviour still
   works, and it is permanently excluded from new services and new conformance items. That caps
   the matrix at (supported majors x 22), which is the number that has to stay small.
3. **`rig doctor` names every connected program still speaking a frozen major**, and the
   Programs view carries the state. The alarm already exists; it just has to be wired.

Nothing is ever switched off. A frozen major keeps working forever - it simply stops growing,
in public, on a schedule.

## 22. Tech stack

Versions verified 2026-09-10.

| Concern | Choice | Version |
|---|---|---|
| Language | Go | 1.27.1 |
| Wire | protobuf messages, hand-framed on a unix socket | google.golang.org/protobuf v1.36.x. **Not gRPC**: measured +9.80 MiB resident for HTTP/2 machinery a local socket does not need (§17) |
| Schema | JSON Schema 2020-12: santhosh-tekuri/jsonschema (validate). Emission needs no library: `cmd/schemagen` walks the proto descriptors | v6.0.3 |
| Config | knadh/koanf/v2 | v2.3.6 |
| CLI | spf13/cobra | v1.10.2 |
| Store | modernc.org/sqlite | v1.58.0 |
| MCP | modelcontextprotocol/go-sdk | latest at M2 |
| Tracing, metrics | OpenTelemetry Go | v1.46.0 |
| Logging | log/slog | stdlib |
| Secrets | zalando/go-keyring | v0.2.8 |
| Desktop | Wails v3 | v3.0.0-beta.19, confined to one package, pinned exactly |
| Synthetic input | `jezek/xgb` with `xproto` and `xtest`, in `cmd/righand` only | v1.3.1. Measured at **+1,015,911 bytes** over a hello-world, which is why it is its own binary and not a daemon dependency (§5m). Named successor: **libei** through the XDG Desktop Portal `RemoteDesktop` interface, which is cgo and is the other half of that reason |
| Tray | Wails v3 systray, `fyne.io/systray` v1.12.2 as fallback | |
| Terminal UI | charmbracelet bubbletea + bubbles + lipgloss | v1.3.10 / v1.0.0 / v1.1.0 |
| Terminal forms | charmbracelet huh, rendered from the same JSON Schema as the window | v1.0.0 |
| Terminal markdown | charmbracelet glamour | v1.0.0 |
| TUI testing | charmbracelet x/exp/teatest, golden frames | latest |
| Frontend | Svelte 5 + TypeScript + Vite + Tailwind v4 | 5.57 / 7.0 / 8.2 / 4.3 |
| Toast motion | motion | 13.2.0 |
| Toast content | shiki, marked | 4.4.3 / 18.0.12 |
| Testing | stdlib, testing/synctest, go-cmp, testscript | v0.7.0 / v1.16.0 |
| Frontend testing | Vitest, Playwright | 5.0.0 / 1.63.0 |
| Frontend build plugin | `@sveltejs/vite-plugin-svelte` | 7.3.0 |
| Schema to TypeScript | `json-schema-to-typescript`, so the window's types come from the same schema `cmd/schemagen` emits | 16.0.0 |
| Formatting | `prettier` with `prettier-plugin-svelte` | 3.9.6 / 4.1.1 |
| TypeScript runtime helpers | `tslib` | 2.8.1 |
| Lint | golangci-lint plus three house analyzers: no program id in rig code, no registry handle outside the kernel, no meaningful enum zero | |
| Runtime tuning | `GOMEMLIMIT` and `GOGC` set explicitly in the unit file and the Makefile | neither appeared anywhere before |

**This table is the INTENDED stack, and most of it is unbuilt. That is not a
defect and the distinction is not currently drawn.**

**Measured 2026-09-11 against `go.mod`:** of the Go rows above, only the
protobuf runtime and the JSON Schema validator are actually present. `koanf`,
`cobra`, `sqlite`, OpenTelemetry, `go-keyring`, `xgb`, `bubbletea`, `lipgloss`,
`glamour`, `huh`, `go-cmp`, `testscript` and `systray` are **all named here and
absent from every manifest** - because the sections that adopt them are not
built yet.

**So `make deps-check` gates ONE DIRECTION, and that direction is the correct
one.** It compares every pinned dependency in `go.mod` and `package.json`
**against this table**, catching a dependency that ships without a row. **The
reverse check would be wrong rather than merely strict**: it would redden on
every row this plan has not reached, which is most of them.

**This paragraph originally claimed the one-directionality was a defect and the
`koanf` row was its fourth instance. That was written from one row, and
measuring the other fifteen overturned it** - one absent row is a finding, and
fifteen is a document doing its job. The correction is kept rather than
silently replaced, because the reasoning that produced it is the estate's own
recurring bug pattern pointed at the wrong target, and that is worth seeing.

**What IS missing is cheaper and more useful than a reverse gate: this table
does not say which rows are ADOPTED and which are INTENDED.** A reader cannot
tell "rig depends on this" from "rig will depend on this", and neither can a
tool. **Marking the adopted rows is what would let a reverse check exist at
all**, and it is owed before anyone builds one.

**Two binaries** (§17): `cmd/rigd` links none of the terminal stack; `cmd/rig` links
bubbletea, huh, glamour and lipgloss and never the daemon's internals. `make bench-size`
attributes every dependency's contribution to each.

**`rigd` owns the keyring, and the split above used to say the opposite.** §5a puts `secrets`
inside the daemon and §13 enforces "keyring reads scoped to the named keys, namespaced per
program" - which is only enforceable if the process holding the scope is the one making the
read. A client that reaches the Secret Service directly is a client that can read any key in
it, and the compound leak §31 records comes back by a second route. So go-keyring is a daemon
dependency, and the CLI reaches secrets the same way every other client does: over the socket.

**That costs part of a number §17 quotes.** Its ladder measures `bubbletea + huh + glamour +
lipgloss + keyring` as one **+8.89 MiB** rung and attributes the whole of it to `rig`.
go-keyring's own share was never separated, so the recovery §2 claims is **8.89 MiB minus an
unmeasured keyring term**. `make bench-size` must split that rung before the M11 budget line
is treated as met, and until it does the figure is a design target rather than a measurement -
which is the exact defect §31 says the last round of budgets had.

---

## 23. Milestone build order

Ordered so the first useful thing arrives in week one and nothing speculative is built before the
program that needs it. Each ends green, committed, and demonstrated.

| M | Name | Ships | The demo that closes it |
|---|---|---|---|
| M0 | Skeleton | Repo, module, Makefile, CI, lint with all three analyzers, **`cmd/rigd` and `cmd/rig` as separate binaries**, the socket, **the single-instance `flock` (§5f)**, the hand-framed protobuf codec, `GOMEMLIMIT`/`GOGC`, `make bench-size`, `make modules-matrix`, `fakeapp` | `make ci` green; `rig ping fakeapp` round-trips; **a second `rigd` refuses to start and names the incumbent's pid**; `make bench-ipc` reproduces §4; `make bench-size` records the ratchet's starting number |
| **M1** | **Register and the CLI** | Registration with declared **properties** and the computed projection, the registry, argument schemas, coverage, `semantics_gen`, **the connect-time `introspect` predicate and per-call `operate` (§14)** - and note the
dependency, because §14's safety argument turns on it: the predicate ships here, and
**complete history reading is gated on M5's compiled redaction spans**, so between M1 and M5
an introspecting client reads the registry and the live views but not a recorded transcript.
The call log itself lands at M5, so the exposure is thin in practice; it is written down so
the ordering cannot be changed later by someone who never reads §14 - **house rules in the invoker**, `rig <app> <cmd>`, generated `--help`, completion, `--json` everywhere | **The week-one product.** `rig shelf reindex` from any terminal. One front door, no GUI. **One house rule is written, and a `destructive` command is then refused with the refusal naming the pair it matched** - the wording before this said "with no grant is refused", which the authorisation model does not do: §14 disowns `operate` as a grant, and §13a's unmatched call is allowed. The same test covers every surface added later |
| **M1a** | **The shell, and two fake applications that use it** | The window shell - rail, context bar, pane, status strip - over the visual system in `design/` as live `ui.theme` config; **`make contrast` repaired and wired into CI** before any of it is drawn; and **two fake applications, each modelled on a real program's measured markup**, serving their own HTML and composing it from the first kit elements they turn out to need (§5h) | **Two fake programs in one rail, drawn by rig, and the element list is whatever those two actually needed rather than whatever this document guessed.** The theme is changed from the settings UI and an unreadable set is refused by a gate that runs |
| M2 | MCP and HTTP | The four meta tools, promotion, the capability-map resource **with coverage per program**, the per-program preamble, HTTP routes with **minted principals**, structured errors | An agent runs a real command in a real program through one MCP server, having written nothing - and is told, in the same answer, that its picture of that program is partial |
| M3 | Terminal client | `rig shell` with completion and inline describe, the `rig tui` frame, `huh` forms from declared schemas, `--batch --json` | Every registered command discoverable and runnable from the TUI, by a person who read no docs |
| M4 | Config, and the event stream | Layers, schema, provenance, live push, validate, export, diff, and its TUI view; **the `events` service with `config.changed` as its first kind, because the live push is the general mechanism built once (§5h)** | `rig config origin` explains a surprising value; a change applies live with no restart - **and a second program that armed `config.changed` and shares no code with the first is told in the same instant** |
| M5 | Observability | Log, trace and metric ingest, merge, query, the call log, **compiled redaction spans**, the **coverage log**, segment-embedded dictionaries, column summaries, `rig logs`, `rig loose-ends`, `rig doctor` **with its dependency check - required, optional and build-time, every optional degradation naming what stops working and what happens instead (§8)**, `disk.pressure` and `memory.pressure` polled from the PSI read side (§5h), TUI views | One MCP call traced end to end across two processes and read back in the TUI; a known secret passed through a declared-sensitive field appears in no segment; **and `rig doctor` on a machine with the audio player removed names the player, what stops working, the fallback and the command that installs it** |
| M6 | Control, supervision, and rig starting by itself | Start, stop, restart, health, budgets, quarantine, **lifecycle notices**, the **tolerant client and the resolved snapshot**, reconnect with a session token, request-id dedup; **the `systemd --user` unit with no `ExecStop` (§5l)**; and three event kinds that fall out of being the supervisor - `system.resumed`, `program.health` and `process.exited` (§5h) | `kill -9` in a loop both ways, plus a deliberate restart under load with **zero refused dials and zero silent replays**. **Log out and back in and rig is already serving.** A suspend and resume publishes `system.resumed` to an armed program; a killed process publishes `process.exited` **with its code where rig is the parent and the literal `unknown` where it is not** |
| M7 | Peers | Presence, leases with **witnesses and two-step expiry**, `rig peers run`, fencing tokens per lease, read/write, semaphores, barriers, election, **continuation slots** (§16), versioned blackboard with multi-key transactions, watches with a cursor, claimable queues, rendezvous, signals, `ask`, **the AgentBox dual-write shadow path**, the crew, wait-for graph, contention and timeline views | The simulation suite green over 10000 seeded interleavings with injected crashes, **lost replies and a suspend clock jump**; every adversarial test passing; a stalled holder's `make deploy` actually stops |
| **M7a** | **The hand** | `cmd/righand` as a fourth binary, eleven declared commands, the typed step array with the terse text form as sugar over it, the window lock re-checked before every event, the per-stroke XKB group lock, the park latch, the display lease witnessed by the hand's own pid and **fenced on every call**, and the HANDS OFF strip as the window's rendering of a held lease (§5m) | **An agent fills a form in a window it did not open, and Boris takes the desktop back mid-script.** The strip is up for exactly the length of the run and goes down on expiry with nobody releasing anything. A second agent asking to drive is told who holds it. A house rule denying `(program, drives-input)` refuses a program's script while Boris's own terminal still drives. **And the release tag `2026.7.3` types as `2026.7.3` with a second keyboard layout selected**, which is the regression this whole section exists to keep from coming back |
| M8 | The tray, and the window over real programs | Embedded mode over the localhost SPAs the estate already serves, one tray icon with the **detached** state, **the decision on who hosts the tray and the toast layer, with its §17 budget row** (§17), and **the first real adopter of the element kit, which is where R1 closes and R4's generations begin (§5h)** | One tray icon where six were, real programs in the rail M1a built, and one of them serving its own HTML through kit elements that a fake application shook out first. **`make bench-idle` covers every resident rig process, not only `rigd`** |
| M9 | Toasts, and speech | The frameless toast, severities, springs, stacking, live bodies, inline actions, the centre, Do Not Disturb, D-Bus fallback; **speech as a renderer on the centre (§12) - one estate-wide queue, a long-lived engine, engine and player resolved at startup, its §17 budget row, and its `rig doctor` rows** | A command answered from inside a toast with no window open. **Two programs notify at once and are heard one after the other rather than over each other; Do Not Disturb silences both while an `ask` that gates a command still speaks; and with the engine uninstalled rig still toasts and `rig doctor` says why it is quiet** |
| M10 | Generated UI | Forms, tables, actions, progress, detail, status from declared schema, in both window and TUI | `nudge` gets a complete pane and a complete TUI view with zero frontend code |
| M11 | Storage and secrets | Managed location, migration runner, backup, integrity, retention, browser; keyring with per-program namespaces, **in `rigd`** (§22); **`rig backup` and `rig restore` for rig's own state** (§7) | A program's migration runs before it starts, and its backup restores. **A fresh machine restores the audit log, the notification centre and the config tree from one archive** |
| M12 | Pilot: shelf | shelf entirely on rig: config, storage, logs, tray, pane, TUI, commands on every surface. A week of daily use | shelf loses its own tray icon and loses no capability |
| M13 | Palette, search, schedule, bus, URL | Cross-program palette, federated search, one scheduler **with the missed-fire policy (§18)**, events with grants, `fs.changed` from the file-watcher source and `grant.revoked` (§5h), `rig://` | graft finishing a run triggers a shelf reindex, with the grant visible and revocable; and an overnight suspend coalesces four missed idempotent reindexes into one |
| M14 | Hardening | Chaos at full size, fuzz corpora, cgroups and landlock, security review, coverage to target | The chaos suite green over 10000 iterations |
| M15 | Packaging and updates | `.deb`, desktop entry, the signed update channel for rig and programs, self-update. **The autostart unit itself moved to M6** (§5l); what is left here is packaging it | Fresh machine to a working rig with three programs in one command, **and `rig restore` carries the state M11 backed up** |
| M16 | Estate migration and the AgentBox cutover | The rest of the estate, in the order in §25, then the agent tooling repointed from AgentBox to the peers service - **and every agent's instructions repointed from AgentBox's `speak` to rig's speech (§12), which is a decision M9 deliberately does not make** | Every in-house program reachable from one CLI, one TUI, one tray, one window, one MCP server |

**v1 is M0 through M13.**

**M1a is inserted rather than numbered, and that is deliberate.** The GUI moved early on
2026-09-10 because a fake application that uses the shell is the only demonstration of it that
exists before a real program is asked to change, and because the element kit had been specified
for two years' time against an inventory nobody had built. Renumbering M2-M16 to make room
would have moved **85 milestone references**, moved §24's M3 gate off the milestone it is
anchored to, and rewritten the numbering under a session that was mid-M1 at the time. A letter
costs one line of explanation; a renumber costs every one of those references.

---

## 24. The gates

Two places where the plan stops and asks rather than continuing.

**After M3.** At that point `rig <app> <cmd>` works from a terminal, every command is an MCP tool
an agent can call, and the TUI makes all of it explorable. That is a large fraction of the total benefit for a small fraction of the
total work. The question gets asked honestly: is the GUI worth the remaining twelve milestones,
or are the front door and the terminal enough? A platform nobody stopped to question is how months disappear.

**M1a makes this gate answerable instead of rhetorical, and that is the point of moving it
early.** By M3 the shell exists and two fake applications are running in it, so the question is
asked in front of the thing rather than in front of a description of it. **What M1a cannot
answer is this gate**, and the distinction has to be held: fake applications show that the GUI
*works*, and this gate asks whether it is *worth the remaining milestones*. A demonstration
that looks good is the most likely way this gate gets passed rather than taken.

**After M8.** One window and one tray exist over programs that were not modified. If that is
enough, M10 and beyond wait for a program that genuinely needs them.

**The gate is answered on whether a real program adopted the kit, and what that cost it in
hours** - not on the fake applications, which were built to use it and therefore cannot fail to.
M10's main product is panes that look like one system; if a real adopter got that from the kit
at M8, the honest question becomes whether generated panes are worth M10 at all. **If no real
program has adopted by M8, that is the answer**, and the kit is struck with the shell kept:
the shell is M1a's and does not depend on it.

### Both gates are anchored to a milestone, and nothing else fires them

| Gate | Asked on | Answered where |
|---|---|---|
| **M3** | the day M3 lands. **M4 does not start until the answer is written** | `DECISIONS.md`, in writing |
| **M8** | the day M8 lands. **M9 does not start until the answer is written** | `DECISIONS.md`, in writing |

The M3 answer is one of three, and it names what it cuts: *continue*, *stop at M3*, or
*continue with a cut list*. **This project carries no dates.** A gate fires when the milestone
before it lands and not otherwise, so the thing that stops it being passed rather than taken is
the block on the next milestone, not a calendar.

---

## 25. Migration order

**The unit of progress is a service, not a program** (§5k). "shelf takes config and
notifications" is a valid milestone; a program is never blocked waiting to adopt everything, and
its declaration says `coverage: partial` until it does. The order below is the order programs
*start*, not the order they finish.

Each program keeps working standalone throughout, and gives up its own tray icon when its tray
adoption lands. **State** is what was on disk on 2026-09-10, because rig's value is a function
of how many of these are worth reaching.

**One row already moved, and the direction it moved is the warning.** `graft` was written in as
adopter 6 on the morning of 2026-09-10 and was out of the order by that afternoon, because it
is now waiting on rig rather than adopting it. Nothing here is load-bearing until it is built
and used; the advocate's item 3 asks for this table to be re-checked before M12, and this is
what re-checking looks like.

| Order | Program | State on 2026-09-10 | Why here |
|---|---|---|---|
| 1 | `shelf` | 45 commits, built, used daily | The pilot. Designs the contract against something real |
| 2 | `dispatch` | 45 commits, built | Already Wails v3; proves an existing app becoming a pane |
| 3 | `nudge` | 11 commits, built | Tiny and tray-only. Designs the generated UI, and is the honest test of the hand-written budget in §3 - the *declaration* is generated, and one rich archi command alone measured 160 lines of JSON |
| 4 | `sigs` | 113 commits | The second generated-UI program, so it is designed against two |
| 5 | `snapper` | 197 commits, built | Native capture stays its own window; history and settings come in |
| - | `graft` | 2 commits, **deferred 2026-09-10** | **Not an adopter, and not counted as one.** Its own window/tray milestone was struck the day this row was written, and Boris then deferred graft entirely until rig v1 and the client stub exist. It is a *consumer* of rig's schedule, not a contributor to it: counting it here would let rig's payoff borrow a program that is waiting on rig |
| 7 | `archi` | 237 commits, two built binaries (13M, 19M) | The richest frontend under the theme bridge |
| 8 | `grabbit` | 100 commits, stalled since 2026-08-11 | A stalled project is the best test of whether rig makes finishing cheaper |
| 9 | `devtool` | 65 commits | The endpoint of the argument: unbundle it, each utility its own program |
| 10 | `romsort`, `dedup` | 7 and 2 commits | Deferred until they are real programs |

---

## 26. Open questions

1. **Does a transparent, frameless, always-on-top, focus-refusing webview work under GNOME Shell
   on X11 from Wails v3 beta?** Load-bearing for §12 and unverified. Compositor and ARGB visual
   confirmed present on this laptop; what is not confirmed is whether Wails reaches the
   override-redirect and input-shape hints. Decided at **M9** by building one first. Fallback:
   the toast layer becomes its own small GTK4 or Gio process rig drives.
2. **Does the Wails v3 beta tray behave on X11 here?** Decided at **M8**. `fyne.io/systray` is
   the fallback and six programs already use it.
3. **Generated forms: `@sjsf/form` or hand-rolled?** Decided at M8. The table, action and
   progress surfaces are ours either way.
4. **How hard is network capability enforcement worth making?** Namespaces are real enforcement
   and real complexity. M12 decides; until then it is declared and honestly labelled.
5. **Does `snapper`'s capture window belong in rig's window at all?** Probably not, and the
   plan assumes not. Note that this is now *answered by the declaration* rather than by naming
   a program: `capture-region` declares `needs_display` and `interactive`, and the pane surface
   states what it can carry (§5e).
6. **Which programs, if any, are worth hosting inside `rigd` rather than as binaries?** (§5j)
   Nothing is hosted until one is actually cheaper that way. Decided per program, and the four
   costs are stated so the decision is not made by accident.
7. **What is the first wire major's support window?** §21 requires a date on the day v1 ships.
   It does not exist yet because v1 has not shipped.
8. **Can §18's supervisor be expressed as a declared state machine?** (§5h) The whole case for
   a state machine service rests on this, and it is answerable from the design rather than from
   code. If yes, the service is real and gets a milestone at the §24 gate; if no, it is struck
   and the three machines rig hard-codes stay hard-coded. Nothing else adopts it today, so
   there is no second candidate to fall back on.
9. **Which real program adopts the element kit, and what does it cost that program?**
   (§5h, §11) **Asked and answered once already, and the answer was none.** Measured
   2026-09-10: `archi` has zero tables, `dispatch` has one call site and its own table has no
   behaviour to lose, `snapper` serves no HTML at all, and replacing working UI with rig's is
   effort with no feature at the end of it - the same reason §25's stalled programs are
   stalled. The kit's adopters are now M1a's fake applications, which cannot answer this
   question because they were written to use it. **The question stays open until a real
   program adopts or M8 arrives, and if M8 arrives first the kit is struck** (§24).
10. **Do the three `← open` services survive the §24 gate?** A job queue, a cache and state
   machines are drawn in §5h and owned by no milestone. §5h says nothing ships without a
   program that adopts it, so the question is really "which program", and for all three there
   is no answer yet. **The file-watcher was the fourth and is now answered**: it needed no
   program adopter, because it is a source on the `events` service rather than a service a
   program calls, and it lands at M13 with the bus (§5h).

11. ~~**Does `effects` need a fifth value for driving input?**~~ **Decided yes and shipped**,
   while the wire was still unfrozen, because §21 makes a new value a hard refusal at an older
   daemon's boundary and the cheap moment is before the first major. `EFFECTS_DRIVES_INPUT = 5`
   sits above `destructive`, so a rule can say "may delete its own files, may not type into my
   windows". **It also found a defect it would otherwise have caused**: the invoker floored an
   unresolvable ref at the `destructive` literal, which stopped being the top of the order, so an
   opaque call would have slipped under a rule denying the new level. There is now an
   `EffectsCeiling` constant and a test that fails when a named value appears above it.
12. **Can a redaction pointer address every element of an array?** (§15, §5m) Every
   declared-sensitive field in this plan is at a fixed path, and `/steps/*/text` is not: the
   number of steps is known per call. Whether the compiled-span construction expresses a wildcard
   at 82.5 ns, or at all, decides whether the hand can log a redacted script or must fall back to
   declaring the whole `steps` array sensitive and recording only its length and each step's op.
   The fallback is what AgentBox does today, so nothing is lost by taking it, but the answer
   changes §15 rather than §5m and should be measured there.

---

## 27. Assumptions made without asking

Flip any of these with a sentence and the plan changes accordingly.

| # | Assumed | The alternative, and what it costs |
|---|---|---|
| 1 | ~~A program with no rig degrades but still runs~~ **Resolved 2026-09-10, and the middle path won.** The stub reimplements nothing; it tolerates absence, reads a snapshot rig wrote, and a program whose declared preconditions are unmet says `unavailable` with a reason instead of running headless for eight hours (§5g) | The two rejected ends: a 300-line un-upgradable copy of the platform inside every binary, or refusing to start and making rig a hard dependency |
| 2 | **rig manages storage, the program opens it** (§7) | Everything over RPC: one audit point and a swappable engine, but ~6µs per query and a contract that must keep up with SQL. Or storage stays entirely with the program, which leaves migrations copy-pasted fifteen times |
| 3 | **rig owns config, storage, secrets, GUI, observability, lifecycle, scheduling and updates** | You named config, storage and GUI. The rest is my proposal; say which to drop |
| 4 | **Programs stay separate binaries by default**, and hosting one inside `rigd` is a deliberate per-program choice (§5j). Partly flipped 2026-09-10 at Boris's instruction | Hosting everything makes rig a monolith and forfeits the defining maximal; hosting nothing loses the case where an extra process is the larger cost |
| 5 | ~~`turret` survives as the name of the UI component~~ **Resolved 2026-09-10: it does not.** The product is rig and the window is the window | Keeping a second name for a component only its owner opens, at the cost of every reader having to learn it |

---

## 28. Repository, versioning and the Makefile

### Versioned, three ways, and they are not the same number

| Version | Governs | Changes when |
|---|---|---|
| **Product** | The `rig` binary. Semver, git tag `v1.4.2` | Any release |
| **Wire** | The contract programs speak. Major only, in the path | A breaking contract change, which rig then serves *alongside* every previous version, forever |
| **Registration schema** | The shape of what a program declares | Independently, and old declarations keep parsing |
| **Semantics generation** | What the declared names *mean*: capability meanings, declaration defaults, schema dialect | Whenever any of those changes meaning. Old registrations stay pinned to the generation they declared, and a generation nobody claims is retired (§21) |

`rig version` prints all three plus the build SHA and date. A program's `rig version` mismatch
is never an error, only a line in the Programs view.

### Git

- One repository, `github.com/boris-milner/rig`, git from the first commit.
- Conventional commits, `type(scope): description`, imperative, under 72 characters.
- Every milestone in §23 ends in a tagged commit with the demo recorded in the message.
- `CHANGELOG.md` generated from the commit log at release, never hand-edited.
- Release is a tag; CI builds, runs the full gate, and publishes the artefacts.
- The wire contract lives in `proto/` and generated code is checked in, so a clone builds with
  no code generation step and a diff shows contract changes plainly.

### The Makefile is the interface to the repository

Every important operation has a target, `make help` lists them all with descriptions parsed from
the file itself, and CI runs the same targets a human does. There is no operation documented in
a README that is not a target, because a documented command drifts and a target does not.

| Group | Targets |
|---|---|
| Build | `build` `build-rigd` `build-rig` `install` `run` `dev` `clean` |
| Test | `test` `test-unit` `test-race` `test-chaos` `test-e2e` `fuzz` `cover` |
| Quality | `lint` `fmt` `vet` `contrast` `verify` `audit` `modules` `modules-matrix` `build-minimal` |
| Generate | `generate` `proto` `schema` `types` `docs` |
| Measure | `bench` `bench-ipc` `bench-idle` `bench-scale` `bench-size` `profile` |
| Operate | `doctor` `logs` `apps` `up` `down` |
| Ship | `deps-check` `tidy` `release` `package` `ci` |

---

## 29. Non-goals

- rig does not run business logic. Ever. A program id appearing in rig's code is a bug.
- rig is not required. Every program works without it, at reduced service.
- rig does not host third-party or untrusted programs. The trust model is real; the threat model
  is a mistake in our own code, not an adversary.
- rig does not replace any program's own CLI. `shelf` still works as `shelf`.
- rig does not own data. Programs own their data; rig owns the plumbing around it.
- **rig does not hot-upgrade itself.** Measured at 1.9 ms of marginal benefit per upgrade
  against four defect classes (§18). Notice, restart, resume.
- **rig does not host anything we did not write.** A hosted program (§5j) is compiled into
  `rigd` at build time, because Go has no working dynamic loading and because an in-process
  plugin has no capability boundary. Third-party code is a separate process or it is not run.
- rig does not decide what a program's declaration *means*. It records what was declared, pins
  the generation it was declared under, and refuses what is missing.
- **rig does not carry AgentBox's whole surface across the M16 cutover.** Four surfaces are
  OUT, ruled 2026-09-11, so the supersession in §16 is a deliberate narrowing rather than an
  accident discovered at cutover. The reasoning is in
  `logbook/projects/rig/agentbox-parity-2026-09-11.md`; the rulings are:
  - **Walkthroughs** - a durable step-by-step code review with a persistent board, TL;DRs,
    domains, prose-to-code binds and a glossary. That is a product built on a platform, not
    platform, and it is the largest single surface dropped.
  - **Assignments** - summoning an agent session on a schedule with the whole toolbox. rig
    projects `cron` from a declaration (§5), which covers running a *command* on a schedule.
    **Starting an agent session a human can open and take over is not rig's job.**
  - **Artifacts** - running interactive HTML so a human can answer with a number, a shape or
    a selection. §11's window and §12's toasts are rig's answer to "ask a human something".
  - **`request_review`** - a blocking diff review returning approved plus a comment. The
    small sibling of walkthroughs, and OUT for the same reason.

---

## 30. Name

A rig is a platform. A rig is your whole setup. Rigging is the ropes and tackle that control a
ship. To rig up is to assemble. Binary `rig`, socket at `$XDG_RUNTIME_DIR/rig/`, config at
`~/.config/rig/`. There is no second name inside it: the window is the window, and `turret` -
after the lathe turret that carries many tools and rotates the right one into place - was
considered for it and dropped on 2026-09-10.


---

## 31. What the attack changed

Ten adversarial seats were run against this document at `73d7ff4` on 2026-09-10, one per
failure surface, plus one pointed the other way at features the shape makes possible. All ten
returned; every citation in the attack record was independently re-checked and held. The record
is `logbook/projects/rig/attacks/2026-09-10-rig-architecture.md`; each seat's full findings, and
the four measurement harnesses, are in `logbook/projects/rig/agent-work/`.

**Verdict: the architecture survived, and the fixes were substantial.** Nothing found argued for
abandoning it. The daemon shape, the projection model and the single-serialization-point
advantage all held. What failed was almost entirely the layer below: budgets set without
measurement, enforcement mechanisms that could not detect what they promised to prevent, and one
security composition that no single surface owned.

### The finding that mattered most, because no seat could see it alone

**Any agent could read any secret.** History recorded what every call returned, verbatim, with
zero redaction in 1104 lines (`grep -ci redact` returned 0). `secrets.get` is a call. And every
client satisfied the operator predicate, because it was "the uid that runs the daemon" and there
is one daemon per uid. So one `rig history --client=X` handed over another program's token. The
storage half and the isolation half were each a finding of their own; the harm existed only in
their product.

### What changed

| Area | Was | Now |
|---|---|---|
| Projection | A command named the surfaces it appears on | Commands declare **properties**, surfaces declare requirements, rig computes it (§5e) |
| The stub | A 300-line un-upgradable copy of config, storage, secrets and logging | Tolerant client + resolved snapshot + lifecycle notices (§5g) |
| Hot upgrade | A locked requirement | **Not built.** Measured at 1.9 ms marginal benefit against four defect classes (§18) |
| Authorization | Absent. Zero such language in the document | `house rules`, in the kernel's invoker (§13a) |
| The operator | The uid running the daemon | **Nothing is minted at all.** `introspect` is decided at connect time from whether the connection registered as a program, and `operate` is an authorisation decision per call (§14). Two rejected drafts - a 0600 file, then an environment variable - are recorded there so neither is reached for again |
| Redaction | Absent | Declared at registration, compiled to byte spans, 82.5 ns (§15) |
| Leases | A TTL and a fencing token | A liveness witness, two-step expiry, `rig peers run`, and an honest statement of what a token can fence (§16) |
| Time | Unnamed | `CLOCK_BOOTTIME`, absolute deadlines, a resume grace epoch (§16) |
| Wire compatibility | One archived binary, round-trip asserted | A frozen fixture per major kept forever, behaviour transcripts, banned enum zero, `semantics_gen`, and a support window with a date (§21) |
| Isolation proof | A two-client content test | Observational equivalence over two worlds (§14) |
| Footprint | Seven budget lines, three unreachable | Rebuilt from a measured 12-rung ladder, plus a binary-size ratchet (§17) |
| Modularity gates | Two of four could not fail | `modules-matrix`, symbol budgets, analyzers under every tag set and on the frontend (§5i) |
| Adoption | Implicitly all-or-nothing | Per service, `coverage: partial` by default (§5k) |
| Where a program runs | Always its own binary | Hosted plugins exist, with four costs stated (§5j) |

### What was not attacked

The peers service's *feature list* (as opposed to its semantics), the wire numbers in §4, the
visual system, and the plan's own milestone ordering. The `advocate` pass has since run
(`logbook/projects/rig/advocate/2026-09-10-post-fix.md`); the blind-spot sweep over this
applied diff was run afterwards and is recorded in the attack file.

---

## 32. What full introspection changed, 2026-09-10

A requirement arrived after the attack and after the advocate pass: **rig must be fully
introspectable to the agents Boris runs.** It is not a new feature - §3 already called
introspection a maximal and §9 already called an agent a first-class user - but it decides a
contradiction the attack created and named without resolving.

**The contradiction.** The attack's highest-ranked finding made the operator a credential
rather than a uid, correctly. §14 then made every estate-wide view an operator surface -
seven of them - and observed in its own text that "one of which §9 tells an agent to read
first". §9 kept telling agents to read the capability map; §14 had just made sure they could
not. Both sections were individually right and jointly wrong, which is precisely the failure
mode of ten seats each attacking one surface.

| Was | Is |
|---|---|
| One operator credential, minted to a path only the launching process gets | **Two grants and no credential.** `introspect` is **decided at connect time** from whether the connection registered as a program; `operate` is an authorisation decision per call, through `confirm` for anyone present and a named `house rules` entry for anything unattended. Nothing is minted, distributed, inherited or persisted. Two drafts that did distribute something - a 0600 file, then `RIG_INTROSPECT` in the environment - are recorded in §14 with how each broke, and §33 has the sweep that killed the second |
| Estate-wide views closed to every agent | Open to any agent Boris runs, completely - the full map, any client's history, every trace, every config resolution |
| §3's isolation test: "two clients cannot see each other" | Three runs. Ungranted A sees nothing of B; A holding `introspect` sees **all** of B and a missing row is a failure; and no run at any sampling rate surfaces a `sensitive` value |
| "Looking is an event", one audit entry per read | Coalesced per principal, per view, per minute, with a count. Presenting a grant stays uncoalesced |

**The argument that makes it safe is one the plan already owned.** §15 does not filter secrets
out of the history; it never writes them. So the worst an introspecting agent can read is
everything Boris could read himself. **Redaction is the precondition for complete
introspection**, and the two features are complements rather than a trade.

**What this deliberately does not do.** It does not widen what an agent may *run* - `house
rules` (§13a) is untouched and a destructive command still needs its grant. It does not make
programs introspectors: a connection that registered as a program is scoped for its life, and
there is no variable, file or token for a program to acquire instead.

**Found by the blind-spot sweep, an hour after the first draft, and then again an hour after
the second.** Round 1 caught two defects while the change was still uncommitted: the grant
delivered as a uid-readable file, and `operate` distributed to a process no human surface
descends from. **Both are the same mistake** - reasoning about the grant from the reader it was
written for, and not from the other four kinds of caller.

The second draft made that same mistake four more times, and round 2 is what caught it: the
environment reaches a shell's whole descendant tree and goes stale on restart (D1, D2, D3), a
hosted program is the one class no environment scrub can cover (D4), `make deploy` cannot read
the launcher's file (D9), and §12 never carried the Do Not Disturb exception §14 relied on
(D13). **The durable lesson is narrower than "run a sweep":** anything that adds a principal,
a grant or a scope gets walked against the full caller table, which is why §14 now has one.
The record is
`logbook/projects/rig/agent-work/attack-2026-09-10-s9-blindspot-sweep/FINDINGS.md`.

**And one thing it costs, written down rather than argued away.** The session boundary in
§14's table was previously enforced for every client; it is now enforced against accident and
against a merely buggy client, and a program that opens a second connection and declines to
register on it reads everything. That is the floor of same-uid isolation on Linux and no
arrangement of tokens beats it. Three things remain genuinely enforced against a hostile local
process: the uid boundary, `operate`, and redaction. **The first draft of this change put the
grant in a 0600 file, which would have made the session boundary a comment** - it is recorded
here because the next person to simplify the delivery will reach for exactly that file.

---

## 33. What the blind-spot sweep changed, 2026-09-10

An eleventh seat ran after the fixes and after the advocate, pointed at what the other ten did
not look at rather than at more of what they did. It returned **19 findings**; the record is
`logbook/projects/rig/agent-work/attack-2026-09-10-s9-blindspot-sweep/FINDINGS.md`.

**Its verdict on the method is the part that generalises.** Ten seats drove ten surfaces to
local optima, so the residue is entirely in the joints - and the joints got *worse*, because
each fix moved cost or risk across a boundary its seat did not own. The footprint fix exported
the GUI to processes nobody budgeted; the isolation fix put its key somewhere every program
could read it. The class no single-surface seat can produce at all is the class with no
surface: backup, disk, format versioning, uninstall, accessibility, unattended operation.

### Applied

| # | Was | Now |
|---|---|---|
| B1 | `introspect` in a uid-readable file, so any program could read every other program's history | Nothing is delivered. It is **decided at connect time** from whether the connection registered as a program (§14). Went through an environment-variable draft first, which round 2 killed |
| B3 | `operate` reached only whatever launched `rigd` - systemd - so no human surface could act | Not a credential at all: `confirm` per call for anyone present, a named `house rules` entry for anything unattended (§14). The unattended *token* survived the first fix and round 2's D9 killed that too |
| B4 | The `house rules` caller vocabulary named only connections | `schedule`, `bus` and `url` added, each minting a principal; `url` defaults to read-only plus confirm (§13a) |
| B5 | §5a put `secrets` in the daemon; §22 linked go-keyring only into the CLI | `rigd` owns the keyring. §17's 8.89 MiB rung is now a design target until `bench-size` splits it (§17, §22) |
| B6 | "Exactly one rig per user" - §16's load-bearing premise - with zero enforcement | `flock` on a pidfile before bind, at M0, with a chaos test (§5f) |
| B10 | §7's rule binds rig's own storage; nothing backed rig up | `rig backup` / `rig restore`, four rows of state named, at M11 (§7) |
| B18 | The client stub "does exactly four things" over a list of five, against a §3 gate | Corrected (§5d) |
| B2 | §17 measured `rigd` and called it rig, while three sections imply an unnamed resident GUI process | The gap and the three-way decision are stated, and M8 owns it (§17, §23) |

### Round 2, over the introspection change itself

The sweep ran twice. The second pass was pointed at the full-introspection rewrite (§32) an
hour after it landed, and returned 15 findings against text written the same day. All are
applied:

| # | Was | Now |
|---|---|---|
| D1, D2, D3 | `introspect` delivered in `RIG_INTROSPECT` from Boris's shell - underspecified, inherited by every descendant of a shell, and stale after the first daemon restart with no way to rewrite it | **Nothing is delivered.** Decided at connect time from whether the connection registered as a program (§14) |
| D4 | Conformance item 22 said "items 1-21", so the environment test excluded hosted programs - the one class running inside `rigd` | Items reordered: 22 is the environment allowlist, 23 the connection predicate, and **24 is the hosted program passing 1-23** (§19) |
| D5 | Item 24 asserted a property of *rig* from inside the *program-side* suite, over peers a single-program run does not have | Moved to §20's Introspection row, beside §14's three-run battery |
| D6 | Run 2's "a missing row is a failure" collided head-on with §15's four declared lossy paths | The assertion is on the **coverage entry**, not the row: a gap the coverage log declares is a pass (§14) |
| D7 | §9 promised a field that says it "exists and was never recorded"; §15 blanks bytes, and a blank cannot say that | The answer carries the declaration's `sensitive` **pointer list** from the registry. No wire change, and 82.5 ns still holds (§9) |
| D8 | §16 forbids holding a lock across a question to a human, which forbade the motivating elevation case | Stated exception: gating the lease-holder's **own** call freezes its lease expiry for the duration (§16) |
| D9 | The unattended `operate` path promised a file to `make deploy`, which descends from Boris's shell and cannot read it - B3 recreated one row lower | `operate` stops being a credential. A named `house rules` entry authorises unattended callers (§14) |
| D10 | "rig builds the child's environment" named no contents, silently breaking the keyring, the D-Bus fallback and every `needs_display` command | The allowlist is enumerated, is config, and conformance item 22 asserts both halves (§18) |
| D11 | §15 and §31 still described the credential as "a token minted into a 0600 file" - the design §14 exists to reject | Both corrected (§15, §31) |
| D12 | Every elevation is audited "with the rule that fired", and no rule fires on an elevation | `origin` is `rule` or `elevation` (§13a) |
| D13 | The Do Not Disturb exception lived in §14, which needs it, not §12, which implements it | Stated in §12 too, as a conformance assertion (§12) |
| D14 | Nothing bound an elevation prompt to the call that caused it, so a background agent's request could be answered as if it came from the terminal in view | The prompt carries principal, client kind, pid, command and arguments, and routes to the requesting caller's surface first (§14) |
| D15 | M1 shipped the grants; M5 ships the redaction that §14 calls their precondition | M1's row states the dependency: complete history reading is gated on M5 (§23) |

**Round 1's B1 and B3 are withdrawn as fixed** in the same file, and the ten other round-1
findings stand unchanged in the bank below.

### Banked for the M3 gate, deliberately

Twelve findings are real and none of them changes M0 or M1: no version on any on-disk format
while the wire is versioned forever; two filesystem registry mirrors §5f's fix left open; no
disk budget and ENOSPC missing from the coverage log's causes - **partly answered since, because `disk.pressure` at M5 gives rig free space and stall pressure per watched path (§5h); the budget policy itself is still owed**; no policy for `ask` at 3am
beyond the one §14 now sets for `operate`; accessibility absent except contrast and reduced
motion; two budgets whose own cited measurements fail them; no uninstall and no update
rollback; the keyring's availability off a desktop session; a "phone" surface promised four
times and specified nowhere; no scheduler library and no DST rule; no default health interval,
which silently sets three budgets in three sections; and the id namespace an earlier seat
accepted and nobody wrote.

**They are banked rather than dropped**, and §24's M3 gate is where they are
answered - because the honest reading of a twelve-item list on an unbuilt plan is that it is
evidence about the plan's size, not a queue of chores.

### What the sweep did not attack

The measurements themselves - quoted numbers were reconciled against each other, not re-run.
Localisation. `design/theme.js`, so §6's claim that its schema is exactly what the engine
implements is unverified by anything but the engine. Multi-seat in the strong sense. And the
four things §31 already lists.


---

## 34. What the mechanism pass changed, 2026-09-11

An expert enumerated, **blind to this document and to the source**, the mechanisms
infrastructure of this class must have. 507 mechanisms in 33 categories; the record is
`logbook/projects/rig/mechanism-taxonomy-2026-09-11.md` and the brief that produced it is
`mechanism-expert-brief-2026-09-11.md`. The blindness is the method: a list written after
reading this document reproduces this document, and the gaps are precisely what anchoring
hides.

**The subject is narrower and harder than "programs".** Boris, 2026-09-11: *"user-programs
is meant for example AI agents - in fact it's mainly AI agents coordinating, locking,
communicating, syncing and all that."* §1's "user" is the unix user; the **adopting
programs** this section is about are autonomous agent sessions. They differ from services
in ways that change the mechanism rather than the tuning:

| The actor | What it changes |
|---|---|
| **Death is the normal termination** - a bounded context or a usage budget, not a crash. Frequent, and not an error | A lease design whose correctness depends on holders releasing is wrong at the centre, not at the edge |
| **It usually dies holding something** | ORPHANED vs FREE is the most consequential single decision in the coordination design |
| **It hands off to a successor, and both exist for a period** | "Alive" does not mean "will still be here". Two occupants of one role is legitimate, not a duplicate-registration fault |
| **Names recycle in shape** | A message can reach a session that is not the one the sender believes it is addressing |
| **It spawns children whose work dies with it** | Parent closure is a supervised event with obligations |
| **It is non-deterministic, and can repeat a claim it was corrected on** | A command served by an agent is not a function. Provenance must travel with a claim |
| **Its context is its memory, and that is not durable** | Durability of an agent's own work is a mechanism, not a discipline asked of the author |
| **A human supervises several at once and acts only on what he can see** | Visibility and reachability are mechanisms, not conveniences |
| **Idleness is a failure** | "Healthy" cannot mean "responding". Progress is the health signal |

### The bar this section is measured against

**A requirement that names a mechanism without stating its failure semantics has mentioned
it, not specified it.** §16's lease is the calibration point at one end - it says what
happens when the holder dies (ORPHANED, not FREE), what a resume does to expiry, and what a
`DRAINING` notice does to every held lease. §18's supervisor is the calibration point at the
other: five state names, no state set, no transition table, no owner, described entirely in
prose.

### Confirmed, and cheap to say so

| | Where | |
|---|---|---|
| Holder-death disposition of a lease | §16 | **The strongest thing in this document.** Two-step expiry, ORPHANED before FREE, a liveness witness, and an `unwitnessed` lease needing a recorded human break |
| Suspend-aware time | §16 | Absolute deadlines on `CLOCK_BOOTTIME`, `boot_id` stamping, a resume grace epoch, and the admission that `synctest` cannot model suspend |
| Advisory exclusion described honestly | §16 | "A token can only fence writes rig mediates", stated plainly *"because the unqualified sentence will otherwise be quoted back later"* |
| Snapshot and deltas atomically | §16 | "One global revision, transaction-granular delivery", so a multi-key claim is never observed half-applied |

### The foundational gaps, each traceable to a mechanism id

**Absent means the concept is not present under any wording** - each row was checked by
reading, not only by term count, and the term counts are given where they are the evidence.

| # | Gap | ids | State |
|---|---|---|---|
| 1 | **Naming is not part of the synchronisation mechanism.** Two actors guarding one resource under two names are both unlocked and neither finds out. Non-deterministic actors invent plausible names as a matter of course. **This estate has already paid for it**: one session took `rig-makefile` and another `repo:rig-shared-build` for the same file, and the file was found dirty mid-edit | J23, M4, K9 | **absent.** `well-known name`, `lease name`, `unregistered name`, `scope intersection`: 0 occurrences each. Needs: names registered with the resource they protect and discoverable before use; what the daemon does with an unregistered name; and overlap tested by **scope intersection**, not name equality |
| 2 | **Seats, generations and the misaddressed successor.** `generation` appears 13 times in this document and every one is a kit or semantics generation; `seat` appears 9 times and every one is a reviewer in §31's attack. **The concept of a role occupied by a succession of sessions does not exist here** | K1, K4, K6, C15 | **absent, and it has already shaped a design.** §16's continuation slots solve "find my note" by listing-and-asking *because there is no seat identity to address* - "nothing the old agent knew can name the slot". With seats and generations the successor could be handed its slot |
| 3 | **HANDING_OFF as a published state.** `handoff`, `handing-off`, `succession`, `overlap`: 0 occurrences. Two sessions legitimately occupy one role for a period; today that reads as a duplicate registration, which §5 refuses with `CODE_DENIED` | K2, K3, K6, K11 | **absent** |
| 4 | **Acknowledgement, and who owns confirming a message landed.** `acknowledg`, `acted-on`, `delivered-unread`: 0 occurrences. With a recipient that can be confidently wrong and can repeat what it was corrected on, the gap between "delivered" and "acted on" is where multi-agent coordination actually fails | L2, L3, L4, L5 | **absent.** §16 has Signals (`post`/`await`) and Rendezvous; neither distinguishes queued, delivered, read, acknowledged and acted-on, and neither says what happens to a queued message when the recipient dies |
| 5 | **Authority attenuation along delegation and spawn chains.** `attenuat`, `confused deputy`, `on behalf of` as an authority notion, `delegat`: 0 occurrences. §13 and §14 are entirely about authority and this is not in them | AC4, AC5, K14 | **absent**, and it is the one gap in a security-adjacent area. An agent spawning a child that calls rig has no stated authority relationship to it |
| 6 | **The state ownership matrix.** §18 says programs "are told explicitly what was lost", which is the promise; the table is the mechanism. Scattered per-subsystem prose means no reader sees the whole thing and programs guess | H3 | **partial.** Needs one table: a row per class of state a session can put into the daemon (declaration, lease, claim, seat, blackboard key by lifetime class, subscription, queued message, pending confirmation, detached operation, schedule, config) against every event it can survive (program restart, daemon restart, upgrade, reboot, holder death, seat succession), **with an answer in every cell including "nothing survives"**, and which side rebuilds what is not preserved |
| 7 | **Idleness as a failure, and progress as the health signal.** §18 measures health by responsiveness and treats idle as a rest state. **Every agent-specific failure is responsive**: idle, stuck, looping, parked on a question nobody sees | O1, O3, O4 | **absent.** Needs: health defined by evidence of progress; idle-and-not-blocked named as unhealthy with a threshold; a waiting session declaring **what** it waits for, so waiting is distinguishable from stuck; and renewal carrying a progress marker rather than being emitted by a background thread |
| 8 | **Durability of an agent's own work.** `checkpoint`: 0 occurrences. Treated nowhere as a mechanism, and the default outcome of every session is that its findings die with it | N1, N2, N3, N4, N5 | **absent.** Needs: a durable location assigned and seeded **before** the work begins; checkpointing at phase boundaries rather than at the end or in a termination handler; atomic checkpoint writes; **work not recorded is UNKNOWN, never done**; and child work persisted independently of its parent |
| 9 | **Intent records and successor re-issue.** `intent record`, `re-issue`, `ambiguous outcome`: 0 occurrences. A call whose reply was lost has an unknown outcome, and a successor cannot safely redo its predecessor's work without the record | U2, F3, Q1 | **absent.** §16 names the request id and session token that make a call exactly-once *from the client's side*; nothing survives the client |
| 10 | **At-risk detection and the escalation ladder.** `at risk`, `remaining budget`, `escalation`: 0 occurrences. A session that knows it is nearly out of context is the only actor that can say so | O2, O5 | **absent** |
| 11 | **"What a program must never assume", stated explicitly.** 0 occurrences | D6 | **absent.** A contract that never says what is *not* guaranteed is read optimistically |
| 12 | **The confirmation channel being out of band from the caller.** §14 and §13a route `confirm` to a window, toast or terminal and bind the answer to *"that one call, not the connection"*, which is the hard half and is right. **The rest is unstated**: `out of band` 0, `argument digest` 0, what happens when **no human is reachable** 0, replay-binding 0 | AB1, AB2, AB3, AB4 | **partial.** Needs: no argument, flag, header or second call can satisfy a confirmation; **deny when no human is reachable, not queue and not wait**; approval bound to one invocation by id and argument digest; and the prompt composed by the daemon from structured data with caller text shown only as marked, escaped, secondary content |

### The three counts the taxonomy asks for

| Count | |
|---|---|
| `foundational` mechanisms absent | **10 absent and 2 partial, of 12 examined.** Anything above zero is load-bearing. **This is a floor, not a count of 183** - the twelve were reached by triage from Part V's 22 entries and from the categories a density measurement pointed at, not by walking all 183. The earlier wording, "11 of 183", was both wrong arithmetic and a triage presented as an audit |
| Named but not defined to the bar | **CLOSED 2026-09-11.** All 22 of Part V's entries are adjudicated: four confirmed present (above), twelve became the gap rows, §13a's conflict resolution is `partial`, and **V13, V15, V18 and V20 are adjudicated in §36**. Record of the first seventeen: `logbook/projects/rig/count2-part-v-audit-2026-09-11.md`. **This count was open when §34 was written and the section said so; saying so is what let it be finished rather than forgotten** |
| Part IV confusion pairs this document conflates | **2 of 25.** #16 `confirmation` / `authorization` conflated outright, #15 `policy denial` / `precondition` / `validation` / `unconfirmed` partially. Record: `logbook/projects/rig/conflation-audit-2026-09-11.md` |

**The second count is open, and saying so is the point.** A pass that reported
only what it finished would be the same defect it is auditing.

### Added by the count-2 pass: house rules are not fully specified

**§13a states the conflict-resolution rule and states it well** - *"the most restrictive
wins: `deny` over `confirm` over `allow`"*, with first-match and most-specific both named
and refused because *"both fail open"*. A term sweep scored this section zero on every word
the taxonomy uses for it, and **the sweep was wrong**: the plan is ahead of the probe,
stating `deny-overrides` in its own vocabulary. That false positive is why every row in
this section was read before it was written.

**Four clauses of the same mechanism are unanswered**, and the fourth is the one that
matters:

| Clause | State |
|---|---|
| Evaluated on the exact effective **arguments** that will execute, with no re-parsing between decision and dispatch | **absent.** §13a matches on the pair `(caller, effects)`; arguments are never matched |
| Evaluation cannot error or block, and any failure is **deny** | **absent** |
| A decision names its matching rule, its **config layer** and the **remedy** | **partial.** `origin` carries `rule`/`elevation` and the rule id; layer and remedy are not recorded |
| **A program re-declaring a command with weaker `effects` is detected rather than trusted** | **absent** |

**The last row is an authority hole, and it is the same shape as the two already queued for
Boris.** `effects` is declared by the program, and house rules match on that declaration.
Nothing says what happens when a program re-registers a command at a *weaker* level - so a
rule written as `(agent, destructive) -> confirm` stops matching the moment the program
re-declares that command as `writes-files`. **No rule is violated and no denial is logged**,
because the pair simply stopped matching. `effects` being *"a floor, not an equality"* widens
it: one step down the enum drops every rule written at or above the old level.

**Traceable to** V17 and category AA, and it belongs beside gap 5 (authority attenuation):
both are authority leaking because nobody stated who may change the input to the decision.

### What this pass has not done

The `important` (290) and `situational` (34) mechanisms are recorded and not consolidated.
The AgentBox parity yardstick - a second, deliberately independent enumeration of what this
estate already has and must not lose - is
`logbook/projects/rig/agentbox-parity-2026-09-11.md`. **It has now been crossed with the
taxonomy: `logbook/projects/rig/taxonomy-parity-cross-2026-09-11.md`, and §35 carries the
result.** The twelve gaps above are listed in the order they were found, which is not a
ranking; **§35 is the ranking.** Four of its surfaces are already ruled out (below).

### Ruled out, 2026-09-11, by Boris

**Walkthroughs, assignments, artifacts and `request_review` are OUT** - 18 of AgentBox's
~40 tools. Dropped deliberately and on the record rather than discovered at the M16
cutover. See §29.

## 35. What the agent-parallelism pass changed, 2026-09-11

**Boris's steer, which is a ranking instruction and not new scope:** the
mechanisms that matter are the ones that help **AI agents work in parallel** on
one machine, so that rig can run like AgentBox and the agents can then be moved
off AgentBox onto rig - *"the rig takes all this functionality to the highest
possible level of quality, robustness and utilization benefit."*

**And the bound on it, stated the same day:** *"we decided to not make perfect
parity to AgentBox, some of its features have no place in the rig."* **Nothing
is imported because AgentBox has it.** Each mechanism below is here because an
agent working beside another agent fails without it, and the IN/OUT judgement
for every candidate is recorded in the cross.

### The ranking, and what it is ranked by

§34 lists twelve gaps in the order they were found and does not claim that is a
ranking. **This is the ranking.** It comes from crossing the blind mechanism
taxonomy against the AgentBox parity pass - two lists produced independently and
deliberately so, where **a mechanism appearing in both is not arguable**.
Record: `logbook/projects/rig/taxonomy-parity-cross-2026-09-11.md`.

| Rank | Mechanism | In both lists? | Landed |
|---|---|---|---|
| 1 | A claim has an owner and a liveness witness | **yes** | §16 |
| 2 | The message contract, queued to acted-on | **yes** | §16 |
| 3 | Seats, generations, `HANDING_OFF` as a published state | **no - see below** | §16 |
| 4 | Retraction of a posted item | **yes** | §16 |
| 5 | A lease name registered with the resource it protects | no | §16 |
| 6 | Health is evidence of progress; idle-and-not-blocked is unhealthy | **yes** | §18 |
| 7 | Durability of an agent's own work | no | §16 |
| 8 | At-risk detection and the escalation ladder | partial | §16 |
| 9 | The state ownership matrix | no | §18 |

**§18 also gained the thing that BLOCKED something else.** The supervisor was
described entirely in prose, which is why §5's "declared state machines, if the
supervisor can be its first client" could never be tested: there was nothing to
express. It now has a state set, a transition table with the trigger and the
owner of each, a statement of what is illegal and what happens on one, and the
four actors that can move a program named. **That test is now runnable**, and it
was costed at a design session rather than a milestone.

**Rank 3 is absent from the parity pass because AgentBox has no seats either**,
and that is the finding rather than a weakness in the pass. The estate runs
multi-seat teams today on a naming convention over a blackboard key and a signal
topic every seat must spell identically - **discipline, enforced by nothing.**
It is therefore the one place rig overtakes rather than catches up.

**The symmetric caution, kept because the ranking is worthless without it:**
ranks 1, 2 and 4 are things AgentBox **already does** and rig had not specified.
Overtaking on rank 3 while losing those at the M16 cutover would be a net loss
to every agent on the machine. **The floor comes first; rank 3 is what the floor
is for.**

### What was PROVED in code rather than argued, and by whom

**Every claim in this section that describes rig's behaviour today was run.**
The three findings that changed what was written:

| Finding | What it changed |
|---|---|
| **A succession and a name collision are BYTE-IDENTICAL refusals.** Both cases built and the strings compared directly; the message names the holder and says nothing about the newcomer | §16's rank 3. The first draft said the two "produce the same error"; the measurement made it exact, and turned up that the holder's session id in the message is a label no caller has ever seen rather than a discriminator |
| **The weaker-effects path is a RECONNECT, not a live re-declaration.** The daemon refuses a second handshake on a live connection, so the reachable path is close, deregister, reconnect one level weaker - an ordinary deploy | §13a. **The first statement of this hole was wrong about the path**, which mattered: the old declaration is gone by the time the new one arrives, so closing it means remembering a declaration past the connection that made it. Harder than the version that was written first |
| **`confirms` is declared, rendered to a human, and consumed by nothing.** The reference program's `purge` declares `destructive` with `confirms` yes, its help says so, and it runs to completion with no terminal: no prompt, no refusal, no channel | §13a gained its own subsection. This is conflation #16 in its sharpest form - **rig stating something false to a user**, rather than a document misleading a reader |

### A declared field that no surface can return is a DEFECT, and there are two

**Found twice on one day, by two different seats, in two unrelated fields.**
That is a class, not a coincidence.

| Field | What happens today |
|---|---|
| `confirms` | declared, validated, **rendered to a human as a guarantee**, and consumed by nothing. Ruled above: it becomes an input rig acts on |
| `preamble` | declared, validated, stored since M1 - and **there is no wire message on any path that can carry it back to a caller.** The daemon-to-caller message is the declaration minus the preamble, so it was accepted and dead |

**The rule, because two instances is enough to state one:** a field the
declaration accepts must be reachable by some caller on some surface, or it is
not a field - it is a validated place to put something that goes nowhere.
**Acceptance is not a contract; retrievability is.**

**And it is a conformance item rather than a review habit** (§19): the suite
already drives a reference program, so a declaration that sets every optional
field and a projection that has to return them is a test a machine can run.
**Neither of these two was going to be caught by reading**, and both were found
only because somebody went to USE the field.

### What this pass has NOT done, and it is deliberate

- **Ranks 6 to 9 are specified nowhere yet.** Rank 9's data is in hand and the
  other three are not started. Listing them ranked and unbuilt is the honest
  state; a pass that reported only what it finished would be the same defect
  §34 exists to audit.
- **The `important` (290) and `situational` (34) taxonomy tiers are still not
  consolidated**, unchanged from §34.
- **Nothing here rules on `confirms`.** Two options are live - a program's claim
  about itself, or an input rig acts on - and **only the current state is ruled
  out**, because a declared-and-unconsumed field shown to a user is a false
  statement whichever way the question goes.

## 36. The four remaining Part V entries, adjudicated 2026-09-11

**§34's second count was open and said so.** Seventeen of Part V's 22 entries
were adjudicated there; **V13, V15, V18 and V20 remained.** They are closed
here, which closes count 2. Record of the method and of the first seventeen:
`logbook/projects/rig/count2-part-v-audit-2026-09-11.md`.

**Each verdict is per CLAUSE, not per entry**, because an entry answering four
of seven clauses is `partial` and the missing clauses are the backlog - and
recording only entry-level verdicts is how "mentions it" passes for "specifies
it".

### V13 - bounded queues and the gap signal: now `partial`, was absent

**Answered by §16's message contract**: a cursor per subscriber, `gap: true`
when the cursor has fallen behind retention, everything since the cursor in one
batch, and a bounded payload that is **refused rather than truncated**.

**Two clauses remain and they are ruled here, because leaving them open is the
robustness hole:**

- **A full queue REFUSES the post; it does not drop the oldest.** Dropping the
  oldest is silent, and it discards precisely the message that has been waiting
  longest - which correlates with the recipient being in trouble. A refusal is
  loud and the poster can act on it.
- **A slow consumer is a health signal, not a queue policy.** A subscriber
  falling behind is reported as such, on the same surface that reports a session
  idle-and-not-blocked (§18). **The queue does not quietly compensate for a
  consumer that has stopped consuming**, because compensating is what makes the
  failure invisible until the buffer is gone.

### V15 - single daemon and epoch handles: now `partial`

**The single-instance claim exists** and behaves correctly - an `flock` on an
open descriptor, so a dead daemon's claim is released rather than left stale.

**The epoch half is ABSENT and it is a robustness gap, not a nicety.** Measured
2026-09-11: nothing rig holds survives a restart, and a client's declaration is
rebuilt by re-registering. **But a client cannot currently tell a daemon restart
from a network blip** - it reconnects, re-registers, and is not told that the
thing it reconnected to is a different process with none of its state.

**Ruled: the daemon publishes an epoch, and every handle carries it.** A handle
presented across an epoch boundary is refused as stale, naming the epoch rather
than failing generically. §18's matrix says what is lost; **the epoch is what
makes a client able to ASK rather than infer.** Inference here is the same class
of defect as a check that cannot tell "nothing is wrong" from "it did not run".

### V18 - session identity is not connection identity: now specified

**Measured: today they are the same thing.** Session ids are minted per
connection by the daemon and never travel to any caller.

**§16's seats make that wrong**, and the contradiction has to be resolved
rather than left for whoever notices it: a **seat outlives every connection**,
a **generation** is one occupancy, and a **connection** is shorter than both.
Three lifetimes, and collapsing any two of them loses something:

| Identity | Lives as long as | Addressable by a peer |
|---|---|---|
| Seat | the role exists | **yes - this is the address** |
| Generation | one session's occupancy | yes, to prove who you are talking to |
| Connection | one socket | **no.** It is transport, and no peer should ever hold one |

**A peer never holds a connection identity**, which is the clause that was
missing and the reason the byte-identical refusal was not obviously wrong: the
one identity in that message is a connection-scoped session id that no caller
has ever seen.

### V20 - projection totality: now specified, and it lands on work in flight

**A projection must be TOTAL**: every piece of state it covers is either
represented or explicitly named as not representable. A projection that silently
omits is a projection that lies, and it lies specifically to the introspecting
caller who cannot check.

| Clause | Ruled |
|---|---|
| **State not representable in the projection is NAMED as such** | never omitted. The caller is told the projection has a hole, and where |
| **Sanitisation is declared, not incidental** | a field removed for a scoped caller is reported as withheld, not as absent. **Absent and withheld are different facts** and only one of them means "ask for more access" |
| **The projection states its own coverage** | which is §5k's coverage principle applied to rig itself, and the same rule that refused `rig doctor` at M1 |

**This is live work, not a future clause.** M2 slice 2 is the registry
projection and it is being built now; slice 4's capability map is the same rule
at a larger scale, where *"a scoped caller and an introspecting one read
different maps at the same instant"* is exactly the withheld-versus-absent
distinction above.
