# rig - orientation

Read this first. It says what rig is, what exists today, where each piece
lives, and where to go deeper. It was last checked against commit `8bdcba8`
(2026-09-24, after the planner split) and every "built" row was checked
against the code, not the plan.
When this file and the code disagree, the code wins and this file gets fixed.

## What rig is, in five lines

rig is the platform every in-house program runs on: one daemon per user.
It supplies what every program needs (configuration, storage, secrets,
logging, notifications, a window, one tray) and it controls the programs.
**No program imports rig.** Programs talk to it over a unix socket, so rig
upgrades and every program gets the improvement without a rebuild.
**A program declares what it can do once, and rig projects that onto every
surface**: CLI, MCP, window, tray, toast, cron, palette, URL.

```
   shelf   graft   archi   snapper   docket   ...
     └───────┴───────┴────────┴────────┘
                    │ declare once: commands, schema, state, events
                    ▼
             ┌──────────────┐
             │     rigd     │  one registry, one truth, one contract
             └──────────────┘
                    │ projected onto every surface
   CLI · MCP · window pane · tray · toast · cron · palette · URL · HTTP
```

## Who it is for, as ruled

| Scope | Comes first |
|---|---|
| rig overall | Boris, as creator and user |
| agents working on projects | agents first: "the perfect environment to thrive in" |
| second objective | Boris can introspect everything of importance, and interact |

Ruled 2026-09-17, `plan/01`. Token cost is a first-order constraint for any
agent-facing surface.

**Writing a program for rig?** Start with `docs/programs.md` and
`examples/`: the wire, the handshake, the declaration, errors, versioning and
panes, for Go and for any other language.

## What exists today

State legend: **built** = in the code and tested. **partial** = some of it
is in the code. **specified** = designed in `plan/`, not built. **out** =
ruled a non-goal.

### Processes and binaries

| Binary | What it is | State |
|---|---|---|
| `rigd` | the daemon: kernel, wire, registry, house rules, record store, coordination store, MCP socket | built |
| `rig` | the client: `rig <program> <command>`, generated help and completion, `--json` everywhere | built |
| `rigwindow` | the tray icon and the window (Wails v3, Svelte). The tray is its own WebKit-free process and spawns `rigwindow --window` as a child on demand, so a closed window holds no renderer (`cf337e6`) | partial |
| `fakeapp` | the reference program the conformance suite drives | built |
| `ledger`, `abacus`, `lantern` | fake adopting programs that prove the pane tiers: ledger and abacus the kit tier, lantern the embedded tier (`tools/f8-demo.sh` shows all three tiers) | built |
| `gate`, `depscheck`, `footprint`, `sizeratchet`, `schemagen`, `pkgdeb`, `ipcbench` | build gates and tools: lint gate, dependency audit, binary-size ratchet, schema generation, `.deb` packaging, the IPC benchmark | built |

### The client, `rig --help`

| Command | What it does | State |
|---|---|---|
| `rig <app> <cmd>` | run a command a program declared, with its declared flags, `--timeout`, `--args '<json>'` | built |
| `rig apps list` | what every program declared, as this client may see it | built |
| `rig describe <app> [<cmd>]` | one thing in full; `--json` prints the MCP describe object the daemon renders | built |
| `rig ping <program>` | round trip through rigd | built |
| `rig estate` | which estate this shell reached | built |
| `rig peers` | who else is here, their purpose and activity | built |
| `rig record put/get/query/history/link/unlink/refs` | the record store; `query` pages under the 1 MiB frame | built |
| `rig record retract/delete/replace` | withdraw a record (history kept), destroy one, or supersede one | built |
| `rig progress step <item>` | append one step to a work item's stream | built |
| `rig backup` / `rig restore --estate` | archive and restore an estate, offline, `--force` moves the old aside | built |
| `rig down`, `rig version`, `rig completion <sh>` | stop the daemon, versions, shell completion | built |
| `rig mcp` | stdin/stdout bridge to the daemon's MCP socket, for an agent host | built |

### Wire methods served by rigd

`rig.hello`, `rig.ping`, `rig.programs`, `rig.estate`, `rig.session`,
`rig.announce`, `rig.activity`, `rig.peers`, `rig.down`,
`rig.record.{put,get,query,history,link,unlink,refs,replace,retract,delete}`,
`rig.progress.step`, `rig.backup.create`, and `rig.project.brief`, which
only refuses and names docket: rig serves every wire version it has shipped,
so the method stays until the next major (plan/50 decision 4). Framing is protobuf behind a length
prefix on a unix socket. gRPC was measured (+9.8 MiB resident) and rejected.

### The MCP door

| Tools | What | State |
|---|---|---|
| `list`, `describe`, `invoke`, `query` | the four meta tools: reach every command of every program without loading 300 definitions | built |
| `capabilities` | the capability map | built |
| `announce`, `set_activity`, `list_agents` | presence | built |
| `record_put/get/query/history/link/unlink/refs/replace/retract/delete`, `progress_step` | the record store | built |
| promoted tools | individual commands raised to first-class tools | specified |

### Platform services

| Service | What it does | State | Spec |
|---|---|---|---|
| registry and projection | a program's declaration becomes CLI, MCP and help | built for CLI and MCP; other surfaces specified | §1, §5 |
| house rules | the authorization floor every call passes | built | §13 |
| estates | `production`, `development`, `staging`: separate sockets and stores | built | §37 |
| record store | kinds, versions, links, refs, query with paging, retract, progress streams (SQLite) | built | §39, §48 |
| backup and restore | whole-estate archive including the WAL | built | §46 |
| presence | announce, activity, peers | built | §16 |
| leases with a liveness witness | TTL leases, pid witness, two-step expiry; `rig.lease.*` on the wire, listed by `rig peers` | built | §16 |
| claimable work queues | claim under a lease, heartbeat, requeue once the worker is observed dead, mandatory idempotency key, at-least-once; `rig.queue.*`, `rig queue push/list`, `examples/queueworker` | built | §16 |
| fencing, barriers, semaphores, signals, deadlock detection | the rest of the peers service | specified | §16 |
| working notes for agents | never lost, tagged, linked, handed back on resume | specified | §9 |
| knowledge-sharing section | lessons written once, searched with SQLite FTS5 so an agent never reads them whole; `rig.knowledge.*`, `knowledge_*` MCP tools, `rig knowledge` | built | §40 |
| configuration | layers, schema, provenance, live push | specified (S1 drafted) | §6, §47 |
| logging, tracing, metrics | ingest, query, redaction at write time | specified (S3 drafted) | §8, §49 |
| operator ledger | who used what, redacted | specified | §15 |
| secrets | keyring-backed, never recorded | specified | §11 M11 |
| supervision | start, stop, health as evidence of progress, restart budgets | partial (window supervisor only) | §18 |
| the window | three pane tiers: generated, kit, embedded | partial | §11 |
| one tray | one icon for every program | built for rig itself | §11 |
| toasts and speech | frameless toasts, severities, inline actions | specified | §12 |
| palette, search, scheduler, URL scheme | cross-program surfaces | specified | M13 |
| packaging and updates | `.deb`, signed update channel | partial (`pkgdeb`) | M15 |

### Milestones

`plan/23` orders seventeen milestones, M0 to M16. **M0 is tagged**
(`v0.0.0-m0`). M1 (wire, registry, CLI) and the M2 MCP door are largely in
the code. M16 is the estate migration and the AgentBox cutover, where
AgentBox's agents move onto rig's peers service.

`logbook/projects/rig/READINESS.txt` is the only file allowed to say how far
rig is from being developed with rig. Ask it, not this file.

## The planner left rig on 2026-09-24

Project and case management (projects, work items, decisions, notes, the
brief) was ruled out of rig (`plan/43`) and moved to **docket**,
`~/me/projects/docket` (`github.com/borismilner/docket`, private). docket
talks to rig through the `client` stub like any program, and imports nothing
else of rig. **The record store and the `record.*` verbs stay in rig.**

| Moved to docket | Run it as |
|---|---|
| the brief | `docket brief <project>`, or `rig docket brief --project <p>` while `docket serve` is registered |
| the seeder that loads plan, backlog and decisions into the store | `go build -o /tmp/rigseed ./cmd/rigseed` in docket, then `--check`. `go run` reports exit 2 as 1 |
| the importers and the planner's pane | `docket pane` |

The split passed all six acceptance tests on 2026-09-24, two of them on a
restore of production (`plan/50`). Ideas about briefs, work items or
backlogs go to docket, not here. **The seeder never retracts:** a renamed
heading leaves its old record behind, cleared by `rig record retract` and
`rig record unlink`.

## Non-goals, and what they rule out

- No business logic. A program id in rig's code is a bug.
- rig is not required. Every program works without it, at reduced service.
- No third-party or untrusted programs; third-party code is a separate
  process or not run.
- No hot upgrade. Notice, restart, resume.
- **Out of rig on purpose**, from AgentBox: walkthroughs, assignments,
  artifacts, `request_review` (ruled 2026-09-11, products not platform).

## Where things live

| Path | What |
|---|---|
| `PLAN.md` | the index of the specification; generated, do not hand-edit |
| `plan/01..50` | the specification, one file per section; cite by section number |
| `cmd/` | every binary above |
| `internal/kernel`, `internal/wire` | the daemon core and the framing |
| `internal/daemon` | method handlers |
| `internal/record` | the record store |
| `internal/coord` | leases, witnesses, the boot-time clock |
| `internal/mcpserver` | the MCP door |
| `internal/estate`, `internal/instance`, `internal/paths` | estates, single instance, XDG paths |
| `internal/backup` | backup and restore |
| `internal/analysis` | the house analyzers `make lint-house` runs (for example `nocontextfree`) |
| `internal/meta` | the meta tools behind `list`, `describe`, `capabilities`: the roster, estate answers, tombstones for departed programs |
| `client/` | the dumb stub a program embeds; the only rig code inside a program |
| `proto/`, `schema/` | the wire schema and generated JSON schema |
| `frontend/` | the window's Svelte app |
| `design/` | the visual system: `visual-system.html` is live, `theme.js` generates the tokens |
| `packaging/` | systemd user units (`rigd.service` runs `--estate=production`) |
| `tools/` | installer, plan splitter (`plansplit.py`), contrast audit, theme gate, window restart |
| `size-ratchet.json` | the binary-size ceilings `make bench-size` holds every binary to |

## Build, run, test

```sh
make help          # every target
make build           # rigd, rig, fakeapp, ledger, abacus into build/
make build-rigwindow # the window (needs the frontend built)
make ci              # fmt-check, vet, lint-house, test-race, schema-check,
                     #   deps-check, theme-gate
make lint            # golangci-lint plus the house analyzers; NOT in ci
make bench-size      # the binary-size ratchet; NOT in ci, run it by hand
make run             # rigd in the foreground, debug logs, unnamed estate
make redeploy        # rebuild, install and restart daemon, client and window
make bench-ipc     # re-take the transport numbers
make contrast      # the contrast gate over the visual system
rig version        # what the running binaries carry
```

Tests never touch a live estate; a test that opens a real store is a bug
(found and fixed 2026-09-24, `a9c7ab8` and `54b0cf3`). Any daemon you start by
hand needs a private `XDG_STATE_HOME` AND `XDG_RUNTIME_DIR`, or it reaches
production. `make build-all` cross-compiles for release; it is not the local
build.

## Where to go next

| Question | Go to |
|---|---|
| what is rig supposed to do | `PLAN.md`, then the numbered section |
| has Boris already ruled on this | `logbook/projects/rig/DECISIONS.md`, then `plan/31..38` |
| what to build next | `logbook/projects/rig/BACKLOG.md` |
| how close to self-hosting | `logbook/projects/rig/READINESS.txt` |
| current state and resume point | `logbook/projects/rig/HANDOFF.md` |
| who owns which files right now | `logbook/projects/rig/COORDINATION.md` |
| what similar projects exist | `logbook/projects/rig/prior-art-2026-09-24.md` |
