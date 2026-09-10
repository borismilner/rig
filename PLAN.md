# rig

The platform every in-house program runs on. It supplies the infrastructure they all need -
configuration, storage, secrets, logging, tracing, notifications, a window, a tray - and it
controls them: starting, stopping, invoking, scheduling and wiring them to each other.

**No in-house program imports rig.** They talk to it over a socket. That is the whole point: rig
upgrades on its own, and every program gets the improvement without being rebuilt.

The name: a rig is a platform. A rig is also your whole setup. Rigging is the ropes and tackle
that control a ship. And to rig something up is to assemble it. Four meanings, all correct.

Status: planning. Nothing built. Written 2026-09-10.
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

- **Name:** rig. Binary `rig`, daemon `rig`, module `github.com/boris-milner/rig`. The UI shell
  component inside it keeps the name **turret**, because a lathe turret carries many tools and
  rotates the right one into place, which is what that component does.
- **No imports.** An in-house program does not link any rig code beyond a dumb pipe (§5d). All
  behaviour lives in the daemon, so all behaviour upgrades without a rebuild.
- **One socket, binary frames.** Measured at 6.2µs round trip and 1.8M one-way records/second
  on this laptop (§4), which is far more than anything here needs. No shared memory, no
  io_uring, no zero-copy codec. That complexity is not bought.
- **Two directions.** Apps call rig for supply (config, storage, notify, UI). rig calls apps for
  control (start, stop, invoke, query, reload). Both over the same connection.
- **Declarative wins.** Where an app *describes* something, rig can improve it forever. Where an
  app *calls* something, that call is frozen. So the contract is biased hard toward description.
- **rig degrades, it never blocks.** An app whose rig is not running still starts and still
  works, with reduced service (§5g). This is an assumption Boris can flip; see §27.
- **Storage: rig manages, the app opens.** rig owns location, migrations, backup, integrity and
  retention; the app opens the file and runs its own queries at full speed. Also an assumption
  in §27.
- **The peers service supersedes AgentBox.** rig builds the best inter-client coordination
  substrate it can (§16), proves it against four gates, and only then do the agents move off
  AgentBox. AgentBox keeps running untouched until that happens. Decided 2026-09-10.
- **History is buffered and never fsynced.** Losing under a second of history to a power cut is
  acceptable and was agreed; durability is what makes event logging expensive, so it is not
  bought. A process crash loses at most one flush interval, because the kernel already holds the
  rest.
- **Linux and X11 first**, on this laptop. Nothing knowingly non-portable, and no cross-platform
  claim until it is tested.
- **Dependencies:** best library for the job, newest published version. Hand-rolling needs a
  product reason.

---

## 3. The maximals, made measurable

Boris's brief across this session: maximally expandable, maximally configurable, highly
introspectable, maximally tested, bullet-proof robust, and infrastructure that upgrades
independently of the programs using it. Adjectives do not pass or fail. Each below is a test.

### Upgrades independently - the defining one

| The test it has to pass |
|---|
| Ship a new rig with a redesigned toast, a new settings UI and a new log viewer. No in-house program is rebuilt, and all of them show the new behaviour on next connect |
| A program compiled today against wire v1 still works against a rig built three years from now. Enforced by a golden-wire test that runs an archived binary against HEAD in CI |
| Adding a whole new surface to rig (a new UI, a phone client, a voice interface) gives every already-registered program that surface with zero code change. Proven once by adding the HTTP surface after the CLI surface, touching no app |
| The client stub's public surface is enumerated in one file and does not grow without an explicit decision recorded in this plan. Every symbol in it is a future rebuild that cannot be avoided |

### Expandable

| The test it has to pass |
|---|
| A new program registers and becomes fully reachable - CLI, TUI, MCP, tray, HTTP, palette, schedule - in under 100 lines and zero frontend code |
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
| `rig config export` writes one file that reproduces the machine exactly, every program included |

### Introspectable

| The test it has to pass |
|---|
| Every call in both directions is recorded: caller, callee, method, duration, status, size, trace id. Live in the UI and exportable |
| For any program: pid, uptime, restarts with the reason for each, RSS, CPU, wire version, health history, last 1000 log lines, live goroutine dump |
| One trace covers an action end to end across both processes, whichever surface started it - a click, a CLI call, an MCP call from an agent, a scheduled fire |
| `rig doctor` reports the health of the whole installation in one screen and exits non-zero if anything is wrong |

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
| Two clients on one daemon cannot see each other. Asserted by running every surface against a two-client fixture and failing if either sees the other |
| Idle footprint stays inside the budget in §17. `make bench-idle` fails the build on a regression |
| Every module compiles out. `make build-minimal` builds, runs and serves the wire with no service and no surface |

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

---

## 5. Architecture

### 5a. The shape

```
  ┌──────────────────────────────────────────────────────────────┐
  │  rig daemon  (one process, one binary, upgrades alone)       │
  │                                                              │
  │  registry     who is registered, what they declared          │
  │  config       layers, schema, provenance, live push          │
  │  store        location, migrations, backup, integrity        │
  │  secrets      keyring, namespaced per program                │
  │  observe      log + trace + metric ingest, merge, query       │
  │  control      start, stop, invoke, health, supervision       │
  │  schedule     one scheduler for the whole estate             │
  │  bus          events between programs, by grant              │
  │  surfaces     CLI · MCP · HTTP · turret · tray · toast · URL │
  └───────────────────────▲──────────────────────────────────────┘
                          │  unix socket, length-prefixed frames
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
| **turret** | The window: a rail of programs, a pane each, generated or embedded | A mouse and a keyboard |
| **Tray** | One icon for the whole estate, per-program status and commands | Always-on presence |
| **Toast** | A notification with the command's own actions inside it | Attention when something finishes |
| **Palette** | `Ctrl+Space` over every command in every program | Keyboard-first work |
| **Schedule** | Any command becomes a cron entry with one config line | Time |
| **URL** | `rig://shelf/search?q=...` | Links, other apps, the browser |
| **Bus** | One program's event fires another's command | Automation between programs |

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
| **Business logic** | | ✔ **all of it** |

### 5d. The client is a dumb pipe

The one piece of rig code that lives inside a program has to be as close to frozen as it can be,
because every symbol in it is a rebuild nobody can avoid later. So it does exactly four things:

1. Find the socket.
2. Frame messages.
3. Reconnect when rig restarts.
4. Fall back when rig is absent (§5g).

**It carries no semantics.** It does not know what a notification looks like, what config keys
mean, how a UI is described or what commands exist. All of that is data negotiated at connect
time. This is the Language Server Protocol lesson: the editor holds a dumb client, every bit of
intelligence is server-side, and servers upgrade freely.

There is a **generated convenience layer** on top, giving typed methods for ergonomics. It is
optional and versioned. If it goes stale, the program still works, because rig accepts every
wire version it has ever shipped.

### 5e. Registration: what a program declares

Once, at connect. This is the whole contract from the program's side.

```
identity      id, name, version, icon, description
commands      id, title, argument schema, danger level, cancellable,
              which surfaces it should appear on
config        JSON Schema for everything it can be told, with defaults
state         named values rig may read and display
data          queryable collections: column schema, filters, sorts
events        topics it publishes, with payload schemas
ui            nothing (generated), or a URL rig should proxy and embed
tray          which commands belong in the tray
capabilities  what it needs: secrets by name, paths, network, other
              programs' events
health        how to check it and how often
```

Everything after `identity` is optional. A program that declares only `commands` still gets a
CLI, an MCP tool, an HTTP route, a palette entry, a tray item and a schedulable job.

### 5f. Transport

One unix socket per program, at `$XDG_RUNTIME_DIR/rig/<id>.sock`. Length-prefixed protobuf
frames, bidirectional, multiplexed streams. Numbers in §4 say this is enough.

- Control plane, request/response: 6.2µs. Config, commands, queries, health.
- Data plane, one-way: 561ns. Logs, traces, metrics, progress events.
- Large payloads: 64KB frames at 19µs is 3.4 GB/s effective. Fine for a table of 100k rows.
- Every frame carries a trace context so a span crosses the process boundary.

### 5g. When rig is not running

The stub carries a small local fallback so a program is never blocked by an absent platform.

| Capability | rig up | rig down |
|---|---|---|
| Config | Served, live reload, provenance | Read straight off disk, no reload |
| Logs | Collected, merged, viewable | Written to stderr and a local file |
| Storage | Managed location, migrations run | Default path, no migration check |
| Secrets | From the keyring | From the environment, or refused |
| Notifications | Toast in the corner | One line on stderr |
| Tray, window, palette, schedule | Present | Absent |
| **Business logic** | **Works** | **Works** |

That fallback is the one part of the stub that duplicates daemon behaviour, and therefore the
one part that cannot upgrade independently. It is kept deliberately stupid for that reason:
about 300 lines, no policy, no cleverness.

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
      ├─ observe            │   │           turret ────────┤
      ├─ schedule           │   │             tray ────────┤
      ├─ bus                │   │            toast ────────┤
      ├─ queue      ← new   │   │          palette ────────┤
      ├─ cache      ← new   │   │             cron ────────┤
      └─ watch      ← new   │   │              URL ────────┤
                            │   │            Slack ────────┤ ← new
                            │   │            voice ────────┤ ← new
                            │   │              TUI ────────┘ ← new
```

**A service extends what rig gives programs.** It implements one interface: a name, a config
schema, a lifecycle, and a set of methods exposed on the wire. Adding one gives every program a
new capability the moment they ask for it, with no program rebuilt. Candidates already visible:
a job queue, a cache, a file-watcher, an HTTP client with shared retry and rate limits, a
template renderer, a lock manager, a diff service.

**A surface extends how the estate is reached.** It implements one interface: it reads the
registry and projects it. It never talks to a program directly, only through the registry and
the invoker, which is what keeps surfaces from multiplying the contract. Adding one gives every
already-registered program that surface for free. That is the compounding property, and it is
worth more than any individual surface.

**A renderer extends how a declared thing is drawn.** A new widget type in the generated UI - a
map, a waveform, a calendar - is a renderer registered against a schema shape. Programs that
already declare that shape get it without changing.

All three can be **in-tree** (compiled into rig, for the ones that are core) or **out-of-tree**
(a separate process rig supervises, speaking the same wire in the opposite direction). Out-of-tree
is the same machinery a program uses, pointed the other way, so there is one contract to test
rather than two.

**The rule that keeps this honest:** a service or surface that needs to know a program's
identity to work is not a service or surface. It is business logic in the wrong place, and the
lint rule that bans program ids in rig code catches it.

### 5i. Modularity, made testable rather than claimed

"Modular" is an adjective until something fails when it stops being true. Four rules, each with
a test behind it.

| Rule | The test |
|---|---|
| **The kernel is small.** Registry, wire, principal, invoker, supervision. Nothing else | The kernel's public API is enumerated in one file and reviewed on every change |
| **No module imports another module.** Services and surfaces talk through the kernel, never to each other | `make modules` runs a layering analyzer that fails the build on a cross-import |
| **Every module can be compiled out.** `make build-minimal` produces a kernel-only daemon | It builds, it runs, it serves the wire, and its footprint is measured |
| **A module knows no program's name.** A service or surface that needs one is business logic in the wrong place | The same lint that bans program ids in rig code |

Modules are declared in one registration file, in-tree or out-of-tree, and the out-of-tree case
uses the same wire a program uses, pointed the other way. One contract, tested once.

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
- `rig config export` and `rig config diff` reproduce and compare a machine.

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
is an index and is rebuildable.

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
| Audit | Every action taken anywhere, by whom, through which surface. One log for the estate |
| Doctor | Versions, drift, orphaned registrations, quarantined programs, capability warnings, disk, permissions |

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
| `effects` | `read-only`, `writes-files`, `network`, `destructive`. An agent can refuse or confirm on its own |
| `idempotent` | Whether re-running is safe. Decides retry behaviour |
| `dry_run` | Whether the command can be simulated. rig exposes `--dry-run` wherever this is true |
| `cost` | Rough duration and whether it spends money. Lets an agent plan |
| `preconditions` | What must be true. Checked before the call, so failure is early and explained |
| `returns` | The output schema, so a result can be used rather than parsed out of prose |

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

### Discovery is a resource, not a guess

rig serves one MCP resource that is the whole capability map of the estate, versioned and
diffable. An agent reads it once and knows everything that exists. When a program registers a
new command, the map changes, and no agent needs updating.

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

An agent that only knows `rig --json list`, `rig --json describe X` and `rig --json invoke X`
can operate the entire estate.

### The rule that keeps the terminal first-class

**A view ships in the TUI when its data lands.** Once the window exists at M7, no view ships in
one without the other. The TUI is written as a surface plugin like any other, so it costs one
module rather than a parallel product.

---

## 11. turret: the window

The UI shell inside rig. Its visual system is a separate piece of work (§23 M8) because for a
program whose whole job is presenting other programs, the visual design *is* the product.

- **One window.** A left rail of registered programs, a context bar, a pane, a status strip.
  Chrome budget is a number that gets defended, not a feeling.
- **One tray icon** for the whole estate, replacing the six that exist today. Per-program status,
  badge, and commands runnable with no window open. A stopped program is still listed, with the
  reason and a Start entry.
- **Two pane tiers.** Generated: the program declared schema and rig renders forms, tables,
  actions, progress, detail and status. Embedded: the program serves its own HTML and rig
  proxies it into a themed iframe with a typed bridge.
- **The shell is achromatic.** Each program owns one hue from a uniform six-hue family, and that
  is the only saturated colour on screen while you are in it. A host that wears the colour of
  whatever it is holding.

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
- Falls back to `org.freedesktop.Notifications` where the frameless window is unavailable, and
  `rig doctor` says which is live.

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
| `tray`, `notify`, `schedule` | Requests from a program that did not declare them are dropped and reported |

Capabilities are enforced where enforcement is real and labelled best-effort where it is not; the
UI says which per program. Widening a capability needs an explicit confirmation, so an edited
manifest cannot quietly gain access.

---

## 14. Isolation

**Default deny, at three boundaries.** A user of rig is not aware that other users exist.

| Boundary | Between | How |
|---|---|---|
| **Instance** | Unix users | One daemon per uid. Socket at `$XDG_RUNTIME_DIR/rig/` mode 0600, separate config, state and storage trees. Nothing is shared, including the tray, the window and the notification centre |
| **Session** | Clients of one daemon: agents, terminals, the window, scripts | Every connection carries a principal - uid, client kind, client id, session id. The registry view, the event stream, notifications, in-flight commands, the call log and the audit log are all filtered to that principal |
| **Program** | Programs | Capabilities (§13): secrets namespaced, storage pathed, events by grant, no cross-program read of anything |

**A client sees itself and the programs it may reach. Nothing else.** It cannot enumerate other
clients, cannot see their commands, cannot receive their events and does not appear in their
views. Two agents working in the same repository, through the same daemon, are invisible to each
other unless both opt in.

**The filtering is in the kernel, not in each surface.** A surface receives an already-scoped
view, so a new surface cannot leak by forgetting to filter. That is the only way this stays true
as surfaces multiply, and it is asserted by a test that runs every surface against a two-client
fixture and fails if either sees the other.

---

## 15. Who is using rig, and what they did

§14 says clients cannot see each other. **You can see all of them.** That asymmetry is
deliberate: isolation is between clients, not between rig and its owner. The uid that runs the
daemon gets a privileged operator view, and every other principal gets the scoped one.

### The operator view

| Column | For every connected client |
|---|---|
| Who | Kind (agent, terminal, window, script, program), pid, command line, uid, session id |
| Since | Connected at, last active, wire version |
| Holding | Leases held, crew membership, subscriptions, capabilities granted |
| Waiting | What it is blocked on, and for how long |
| Doing | Calls in flight right now, with elapsed time |
| Rate | Calls per second, bytes, error rate, denied capability attempts |

Selecting a client opens its history: every call it made, when, with what arguments, how long it
took, what came back. The same view exists in the window, in the TUI, on the CLI (`rig clients`,
`rig history --client=X`) and over MCP, because they are all surfaces over one registry.

**Looking is itself an event.** Opening another client's history is written to the audit log.
On a single-user machine that is close to pointless, but it costs nothing and it means the rule
is the same rule when it stops being a single-user machine.

### History has to be nearly free, and it can be

You gave the permission that makes this cheap: **losing recent entries to a crash or a power cut
is acceptable.** Durability is what makes event logging expensive, so not needing it changes the
design completely.

```
  call happens
      │
      ▼
  ring buffer in memory        fixed byte ceiling, lock-free append
      │                        reads are free, no disk involved
      │  every 250 ms, or 256 KB, whichever first
      ▼
  write(2) to a segment file   NO fsync, ever
      │                        the kernel now owns it
      │  on rotation
      ▼
  zstd the closed segment      plus an offset index in SQLite
```

**What that actually costs you in a failure**, stated precisely because "buffered" is vague:

| Failure | What is lost |
|---|---|
| `kill -9 rig`, or a panic | Only the in-memory batch: at most 250 ms of history. The kernel already holds everything written before that |
| Power cut or kernel panic | The in-memory batch plus whatever the page cache had not flushed. Seconds, not minutes |
| Disk full | The oldest segments are dropped first, and rig says so rather than stopping |

**Six things that keep it cheap.**

- **No fsync in the hot path.** This is the whole reason it is affordable, and it is a locked
  decision rather than an oversight.
- **Interned, fixed-width records.** Method names, program ids, client ids and error codes are
  dictionary-encoded integers, not repeated strings. Roughly a tenth of the size of JSON and far
  faster to write and scan.
- **Ring buffer with a byte ceiling.** Memory is bounded before disk is involved, so a flood
  costs a fixed amount and drops the oldest rather than growing.
- **Sampling under load.** Past a configured rate, records are sampled and **the sampling rate is
  recorded with them**, so counts stay correct even though not every entry is kept.
- **Retention by both size and age**, enforced by dropping whole segments, which is one unlink
  rather than a compaction.
- **The index is rebuildable.** SQLite holds offsets into the segments. Delete it and it is
  reconstructed by scanning. Nothing holds the only copy of anything.

### The budget

| What | Target |
|---|---|
| Cost of recording one call, amortised | **< 200 ns** |
| Worst-case history lost to a power cut | **< 1 s** |
| Memory ceiling for all history buffers | **8 MB**, configurable |
| Disk ceiling, default | **500 MB**, configurable, oldest dropped first |
| Cost of a history query over 30 days | **< 200 ms** |

All five are asserted by `make bench-idle` and a history-specific benchmark, so a regression
fails the build rather than being noticed a year later.

---

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

Durability comes from a write-ahead log: coordination state survives a daemon restart, and
clients reconcile on reconnect rather than losing their place.

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
| **Fencing tokens** | The bug that breaks every naive distributed lock: holder A stalls, its lease expires, B acquires, A wakes and writes anyway. Each acquisition returns a monotonically increasing token, and every guarded write must present it. rig rejects a stale token. **Without this, a lock is a suggestion** |
| **Read/write leases** | Many readers or one writer, because "everyone waits for everyone" is how a crew stops working |
| **Deadlock detection** | rig holds the wait-for graph and can see a cycle. It refuses the acquisition that would close one, naming the cycle, rather than letting two agents wait forever |
| **Semaphores** | At most N agents doing the expensive thing at once |
| **Barriers** | N agents wait until all arrive, then all proceed |
| **Leader election** | Exactly one agent runs the migration, and the others know who it is |

**Shared state.**

| Primitive | Why it is here |
|---|---|
| **Versioned blackboard** | Compare-and-swap on a key, with a revision per change |
| **Multi-key transactions** | Claim three things or none. Single-key CAS cannot express "divide this work" safely |
| **Watches with a cursor** | Subscribe from a revision. A client that reconnects gets what it missed instead of a gap it cannot detect |

**Work distribution**, which is what "divide the work so nobody doubles" actually needs.

| Primitive | Why it is here |
|---|---|
| **Claimable queues** | Claim a task under a lease, heartbeat it, and it is automatically requeued if you die. This is the correct shape for handing work to agents that can be killed |
| **Rendezvous** | Hand a result to a named successor and park until it is collected |
| **Signals** | `post` and `await`. Park with nothing burned until a peer wakes you. Replaces every poll loop and every "check back in five minutes" |

**The human is a peer.** `ask` puts a question to whoever is actually present, routed to the
window, a toast, the terminal or a phone. That is AgentBox's most valuable single idea and it
is kept whole.

### Safety defaults, because agents get killed mid-operation

- Every lease has a TTL. There is no infinite hold.
- Every `await` has a deadline. There is no infinite block.
- A lock is never held across a question to a human. rig refuses the combination.
- Every primitive's crash semantics are written down and tested, not inferred.

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
| **Baked** | Run in parallel with AgentBox on real agent sessions for a fortnight, with both recording. Any divergence is a bug in one of them and gets found before anything is switched off |

Only after all four does the agent tooling point at rig. AgentBox stays running and untouched
until then, and stays available afterwards until a month has passed with no regression.

---

## 17. Footprint

rig is resident all day. A platform that costs the machine something noticeable has taken back
what it gave. The budget is numbers, and `make bench-idle` fails the build when one is missed.

| What | Budget | Measured by |
|---|---|---|
| Daemon idle, resident memory | **< 20 MB** | `make bench-idle`, after 60s quiet |
| Daemon idle, CPU | **< 0.1%** | same |
| Wakeups at idle | **< 1 per second** | same. This is the one that costs battery |
| Overhead per registered program | **< 500 KB** | `make bench-scale`, at 1, 10 and 50 programs |
| Cold start to first command served | **< 100 ms** | measured in CI |
| A no-op command, end to end | **< 10 ms** | the wire is 6.2µs of it (§4); the rest is process |
| The window, when closed | **zero** | it is not the same process |

**Six rules that deliver those numbers.**

- **The window is a separate process.** The daemon has no GUI dependency, does not link Wails,
  and does not link a webview. `rig turret` starts the window; closing it returns every byte.
  This also confines the Wails beta to a process that can crash without touching anything, and
  keeps the daemon cross-compilable with no cgo.
- **Nothing polls.** Everything is epoll-driven. Health checks are timers, and timers are
  coalesced onto one wheel so ten programs do not mean ten wakeups.
- **Programs are lazy.** `autostart = lazy` means a program is not started until a surface
  actually needs it, and may be stopped again when idle if it declares that it can be.
- **Services are lazy.** A service nobody has used is registered but not initialised. The
  storage service does not open a database until someone asks for one.
- **Observability is bounded.** Logs and traces are ring-buffered with a byte ceiling, spilled
  to disk, and sampled under load. Introspection must never be the reason the machine is slow.
- **A regression fails CI.** The budget is a test, not an aspiration.

---

## 18. Supervision and failure

- **Register:** handshake, wire version check, declaration validated against its schema,
  capabilities granted, then the program appears on every surface. A failure at any step is a
  quarantine with the reason, not a retry loop.
- **Health:** on the interval, with a timeout. Three failures is degraded, five is a restart.
- **Restart:** exponential backoff inside a budget. Exceeding it is quarantine - a visible state
  with the full history and a manual restart, never a silent disappearance.
- **Crash:** the pane becomes a panel with the exit status, the last 200 log lines, the restart
  decision and the countdown. Nothing else changes.
- **Hang:** every call has a deadline. A program that misses them is degraded, then restarted. A
  hung program can never block a rig goroutine, and a lint rule keeps it that way.
- **Flood:** per-program rate limits on events, logs and notifications, with the drop count shown.
- **rig dies:** every program keeps running on its fallback. On restart, all reconnect and
  re-register. In-flight commands are marked interrupted with partial output kept.
- **Shutdown:** SIGTERM, grace period, then SIGKILL. Programs rig did not start are never killed.

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
16. Golden wire: an archived binary from the last release still works against HEAD
17. Runs correctly with rig absent, on the fallback

`fakeapp` is the misbehaving reference program and ships in the repo. It hangs, crashes, leaks,
floods, lies about its schema, ignores cancellation and returns garbage, each on a flag.

---

## 20. Testing strategy

| Layer | Approach |
|---|---|
| Wire contract | 100% coverage, table-driven, plus the golden-wire compatibility test |
| Supervisor and scheduler | `testing/synctest` for all timing. Deterministic, microseconds, no `time.Sleep` in any test |
| Chaos | 10000 randomised kill / hang / flood / restart sequences against `fakeapp`, seed printed on failure |
| Fuzz | Registration parser, JSON Schema inputs, frame decoding, the postMessage bridge |
| CLI | testscript golden transcripts for every command including failures |
| Generated UI | Golden snapshot per schema shape |
| Frontend | Vitest on the bridge and stores; Playwright driving the real window - click, Esc, Tab, Enter, tray, theme switch, crash and recovery |
| Contrast | Measured in a real browser both themes, asserted against WCAG. A failing ratio fails the build |
| Integration | The pilot runs the real `shelf` binary in CI, not a mock |
| Gates | 90% on `internal/`, 100% on wire and stub, no call without a deadline, no program id in rig code |

---

## 21. Versioning

- The wire is versioned by major in the path. rig serves **every** wire version it has ever
  shipped; that is the promise that makes independent upgrade real.
- Capability differences are negotiated at connect: the program says what it supports, rig uses
  what it has, and the gap is visible in the Programs view rather than a failure.
- The stub's public surface is enumerated in one file. Adding to it requires a recorded decision,
  because it is a rebuild nobody can avoid later.
- The golden-wire test runs an archived binary against HEAD in CI every release.

---

## 22. Tech stack

Versions verified 2026-09-10.

| Concern | Choice | Version |
|---|---|---|
| Language | Go | 1.27.1 |
| Wire | protobuf over a unix socket | google.golang.org/grpc v1.83.2 |
| Schema | JSON Schema 2020-12: santhosh-tekuri/jsonschema (validate), invopop/jsonschema (emit) | v6.0.3 / v0.14.0 |
| Config | knadh/koanf/v2 | v2.3.6 |
| CLI | spf13/cobra | v1.10.2 |
| Store | modernc.org/sqlite | v1.58.0 |
| MCP | modelcontextprotocol/go-sdk | latest at M2 |
| Tracing, metrics | OpenTelemetry Go | v1.46.0 |
| Logging | log/slog | stdlib |
| Secrets | zalando/go-keyring | v0.2.8 |
| Desktop | Wails v3 | v3.0.0-beta.19, confined to one package, pinned exactly |
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
| Lint | golangci-lint plus two house analyzers | |

---

## 23. Milestone build order

Ordered so the first useful thing arrives in week one and nothing speculative is built before the
program that needs it. Each ends green, committed, and demonstrated.

| M | Name | Ships | The demo that closes it |
|---|---|---|---|
| M0 | Skeleton | Repo, module, Makefile, CI, lint with both analyzers, the daemon, the socket, the frame codec, `rig` CLI, `fakeapp` | `make ci` green; `rig ping fakeapp` round-trips; `make bench-ipc` reproduces §4 |
| **M1** | **Register and the CLI** | Registration, the registry, argument schemas, `rig <app> <cmd>`, generated `--help`, shell completion, `--json` everywhere | **The week-one product.** `rig shelf reindex` from any terminal. One front door, no GUI |
| M2 | MCP and HTTP | The four meta tools, promotion, the capability-map resource, HTTP routes, structured errors | An agent runs a real command in a real program through one MCP server, having written nothing |
| M3 | Terminal client | `rig shell` with completion and inline describe, the `rig tui` frame, `huh` forms from declared schemas, `--batch --json` | Every registered command discoverable and runnable from the TUI, by a person who read no docs |
| M4 | Config | Layers, schema, provenance, live push, validate, export, diff, and its TUI view | `rig config origin` explains a surprising value; a change applies live with no restart |
| M5 | Observability | Log, trace and metric ingest, merge, query, the call log, `rig logs`, `rig doctor`, TUI views for all of it | One MCP call from an agent traced end to end across two processes and read back in the TUI |
| M6 | Control and supervision | Start, stop, restart, health, budgets, quarantine, graceful shutdown, reconnect | `kill -9` in a loop both ways: rig survives a program dying, programs survive rig dying |
| M7 | Peers | Presence, leases with fencing tokens, read/write, semaphores, barriers, election, versioned blackboard with multi-key transactions, watches with a cursor, claimable queues, rendezvous, signals, `ask`. The crew, wait-for graph, contention and timeline views | The deterministic simulation suite green over 10000 seeded interleavings with injected crashes, and every adversarial test passing |
| M8 | turret: window and tray | The rail, panes, embedded mode over the localhost SPAs six programs already serve, one tray icon, the visual system | One window, one tray, three programs in a rail. Six tray icons become one |
| M9 | Toasts | The frameless toast, severities, springs, stacking, live bodies, inline actions, the centre, Do Not Disturb, D-Bus fallback | A command answered from inside a toast with no window open |
| M10 | Generated UI | Forms, tables, actions, progress, detail, status from declared schema, in both window and TUI | `nudge` gets a complete pane and a complete TUI view with zero frontend code |
| M11 | Storage and secrets | Managed location, migration runner, backup, integrity, retention, browser; keyring with per-program namespaces | A program's migration runs before it starts, and its backup restores |
| M12 | Pilot: shelf | shelf entirely on rig: config, storage, logs, tray, pane, TUI, commands on every surface. A week of daily use | shelf loses its own tray icon and loses no capability |
| M13 | Palette, search, schedule, bus, URL | Cross-program palette, federated search, one scheduler, events with grants, `rig://` | graft finishing a run triggers a shelf reindex, with the grant visible and revocable |
| M14 | Hardening | Chaos at full size, fuzz corpora, cgroups and landlock, security review, coverage to target | The chaos suite green over 10000 iterations |
| M15 | Packaging and updates | `.deb`, desktop entry, autostart, signed update channel for rig and programs, self-update | Fresh machine to a working rig with three programs in one command |
| M16 | Estate migration and the AgentBox cutover | The rest of the estate, in the order in §25, then the agent tooling repointed from AgentBox to the peers service | Every in-house program reachable from one CLI, one TUI, one tray, one window, one MCP server |

**v1 is M0 through M13.**

---

## 24. The gates

Two places where the plan stops and asks rather than continuing.

**After M3.** At that point `rig <app> <cmd>` works from a terminal, every command is an MCP tool
an agent can call, and the TUI makes all of it explorable. That is a large fraction of the total benefit for a small fraction of the
total work. The question gets asked honestly: is the GUI worth the remaining twelve milestones,
or are the front door and the terminal enough? A platform nobody stopped to question is how months disappear.

**After M8.** One window and one tray exist over programs that were not modified. If that is
enough, M10 and beyond wait for a program that genuinely needs them.

---

## 25. Migration order

Each program keeps working standalone throughout, and gives up its own tray icon when it is
migrated. **State** is what was on disk on 2026-09-10, because rig's value is a function of how
many of these are worth reaching.

| Order | Program | State on 2026-09-10 | Why here |
|---|---|---|---|
| 1 | `shelf` | 45 commits, built, used daily | The pilot. Designs the contract against something real |
| 2 | `dispatch` | 45 commits, built | Already Wails v3; proves an existing app becoming a pane |
| 3 | `nudge` | 11 commits, built | Tiny and tray-only. Designs the generated UI and proves the 100-line claim |
| 4 | `sigs` | 113 commits | The second generated-UI program, so it is designed against two |
| 5 | `snapper` | 197 commits, built | Native capture stays its own window; history and settings come in |
| 6 | `graft` | 2 commits, planned 2026-09-10 | **Its M14 (its own Wails window, tray, badge, .desktop, icon) should be struck now and replaced with registering with rig, while it is still unbuilt** |
| 7 | `archi` | 237 commits, no built binary | The richest frontend under the theme bridge |
| 8 | `grabbit` | 100 commits, stalled since 2026-08-11 | A stalled project is the best test of whether rig makes finishing cheaper |
| 9 | `devtool` | 65 commits | The endpoint of the argument: unbundle it, each utility its own program |
| 10 | `romsort`, `dedup` | 7 and 2 commits | Deferred until they are real programs |

---

## 26. Open questions

1. **Does a transparent, frameless, always-on-top, focus-refusing webview work under GNOME Shell
   on X11 from Wails v3 beta?** Load-bearing for §12 and unverified. Compositor and ARGB visual
   confirmed present on this laptop; what is not confirmed is whether Wails reaches the
   override-redirect and input-shape hints. Decided at M7 by building one first. Fallback: the
   toast layer becomes its own small GTK4 or Gio process rig drives.
2. **Does the Wails v3 beta tray behave on X11 here?** Decided at M6. `fyne.io/systray` is the
   fallback and six programs already use it.
3. **Generated forms: `@sjsf/form` or hand-rolled?** Decided at M8. The table, action and
   progress surfaces are ours either way.
4. **How hard is network capability enforcement worth making?** Namespaces are real enforcement
   and real complexity. M12 decides; until then it is declared and honestly labelled.
5. **Does `snapper`'s capture window belong in turret at all?** Probably not, and the plan
   assumes not.

---

## 27. Assumptions made without asking

Flip any of these with a sentence and the plan changes accordingly.

| # | Assumed | The alternative, and what it costs |
|---|---|---|
| 1 | **A program with no rig degrades but still runs** (§5g) | Refuse to start instead: zero duplicate code and a tiny stub, but rig becomes a hard dependency of everything - the single point of failure that was explicitly rejected earlier |
| 2 | **rig manages storage, the program opens it** (§7) | Everything over RPC: one audit point and a swappable engine, but ~6µs per query and a contract that must keep up with SQL. Or storage stays entirely with the program, which leaves migrations copy-pasted fifteen times |
| 3 | **rig owns config, storage, secrets, GUI, observability, lifecycle, scheduling and updates** | You named config, storage and GUI. The rest is my proposal; say which to drop |
| 4 | **Programs stay separate binaries** | The alternative is that they become modules of one program, which is a different design entirely |
| 5 | **`turret` survives as the name of the UI component** | It can just be called "the window" |

---

## 28. Repository, versioning and the Makefile

### Versioned, three ways, and they are not the same number

| Version | Governs | Changes when |
|---|---|---|
| **Product** | The `rig` binary. Semver, git tag `v1.4.2` | Any release |
| **Wire** | The contract programs speak. Major only, in the path | A breaking contract change, which rig then serves *alongside* every previous version, forever |
| **Registration schema** | The shape of what a program declares | Independently, and old declarations keep parsing |

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
| Build | `build` `install` `run` `dev` `clean` |
| Test | `test` `test-unit` `test-race` `test-chaos` `test-e2e` `fuzz` `cover` |
| Quality | `lint` `fmt` `vet` `contrast` `verify` `audit` |
| Generate | `generate` `proto` `schema` `types` `docs` |
| Measure | `bench` `bench-ipc` `profile` |
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

---

## 30. Name

A rig is a platform. A rig is your whole setup. Rigging is the ropes and tackle that control a
ship. To rig up is to assemble. Binary `rig`, socket at `$XDG_RUNTIME_DIR/rig/`, config at
`~/.config/rig/`. The window component inside it is `turret`, after the lathe turret that carries
many tools and rotates the right one into place.
