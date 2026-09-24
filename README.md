<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="design/readme/banner-dark.svg">
  <img alt="rig - the platform every in-house program runs on" src="design/readme/banner-light.svg" width="900">
</picture>

</div>

## What it is

**One daemon that every in-house program and every agent session on this
machine leans on.** A program declares its commands once and gets a CLI, MCP
tools and a window pane; a session gets a record, lessons, queues and toasts.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="design/readme/projection-dark.svg">
  <img alt="Six programs declare once into rig, which projects the declaration onto nine surfaces" src="design/readme/projection-light.svg" width="900">
</picture>

## Quick start

```sh
git clone https://github.com/borismilner/rig && cd rig
make deploy
```

`make deploy` builds and installs every part (the daemon `rigd`, the client
`rig`, the window and tray `rigwindow`) from the latest `main`, restarts them,
and checks that the version answering is the one it built. It refuses a tree
with uncommitted changes or a HEAD that is not `origin/main` (`FORCE=1`
overrides). It needs Go, plus cgo with GTK and WebKitGTK and Node for the
window.
`make help` lists every other target.

```sh
rig estate                    # which daemon this shell reached
rig apps list                 # every registered program
rig notify success "hello"    # a toast at the tray's corner
```

## What a program gets

A program connects to `$XDG_RUNTIME_DIR/rig/rigd.sock`, says hello with a
declaration, and answers its own commands. rig never runs code inside it.

| From one declaration | |
|---|---|
| a CLI | `rig <program> <command>`, generated help and completion, `--json` on every answer and error |
| agents | every command through the MCP door (`list`, `describe`, `invoke`); a promoted one as its own tool |
| a pane | generated from the declaration, or a page you serve, themed by the window |
| errors | one `Status` shape: code, message, precondition, actual, fix |

Go links `client` and `proto/rig/v1`; any other language speaks the wire from
the proto files. **[docs/programs.md](docs/programs.md)** is the contract, with
Go and Python examples in `examples/`, `client/clienttest` for testing against
a real daemon.

## What an agent session gets

| | CLI | MCP |
|---|---|---|
| **Toasts** in five severities: info, success, warning, error, urgent. Copy and close on every toast, pin on any that would close on its own; filed in the record first; Do Not Disturb never holds back urgent | `rig notify`, `rig dnd` | |
| **Lessons**: written once, searched with SQLite FTS5, answered as snippets, never whole documents | `rig knowledge` | `knowledge_search`, `knowledge_get`, `knowledge_add` |
| **The record**: append-only, versioned records with typed links, retract, delete and replace | `rig record` | `record_*` |
| **Progress**: a work item started, blocked or finished | `rig progress step` | `progress_step` |
| **Queues**: claim under a lease, heartbeat, requeue once the worker is observed dead; at-least-once, with a mandatory idempotency key | `rig queue` | |
| **Peers**: who else is here, what each is for and doing; leases with fencing tokens | `rig peers` | `announce`, `set_activity`, `list_agents` |

Everything in the table is also a `rig.*` verb on the socket.
`docs/orientation.md` is the checked inventory of what exists and what does
not yet.

## Measured before it was designed

The daemon shape only works if a socket round trip is cheap. Taken on one
laptop, 2026-09-10, Go 1.27.1, two real processes:

| Transport | Per operation | Rate |
|---|---|---|
| Direct Go function call, what an imported library gives you | 0.6 ns | 1.6G/s |
| Unix socket round trip, 64 B | **6.2 µs** | 161k/s |
| Unix socket round trip, 4 KB | 7.8 µs | 128k/s |
| Unix socket round trip, 64 KB | 19.0 µs | 53k/s |
| Round trip with JSON encode and decode, 81 B | 11.4 µs | 87k/s |
| One-way write, 256 B log record | **561 ns** | 1.8M/s |
| Shared memory, spin-wait round trip | 143 ns | 7.0M/s |

Three decisions fell out of that table:

- **The daemon costs nothing that matters.** A config read, a notification, a
  command invocation, a health check, thousands of times over at 6 µs, is noise.
- **Logging does not need shared memory.** A plain socket write sustains 1.8M
  records per second. Nothing here will produce a thousandth of that. Shared
  memory is 43x faster and stays unbuilt until a workload asks for it.
- **gRPC is not bought.** Its server on a unix socket measured **+9.80 MiB
  resident** for HTTP/2 framing a local socket does not need. Protobuf stays,
  hand-framed behind a length prefix.

Re-take them on your own machine with `make bench-ipc`. The absolute numbers
move with the hardware and the load; the ratios the decisions rest on do not.

**With one exception, and it is the row carrying an architectural decision: the
gRPC number is NOT produced by `make bench-ipc`.** `cmd/ipcbench` contains no
gRPC code at all. Every other row here reproduces within a few percent, verified
row by row on 2026-09-11; the +9.80 MiB was measured separately and is not
re-runnable from this repository. **Said here because "re-take them" over a
table whose most consequential row cannot be re-taken is the kind of claim this
project keeps catching in its own instruments.**

### What it costs on disk

Measured and held by `make bench-size`, which fails a build that grows a
binary without a recorded reason (`size-ratchet.json`):

| Binary | Bytes |
|---|---|
| `rigd` | 16,638,215 |
| `rig` | 7,074,055 |
| `rigwindow` | 14,291,976 |

## Where the plan lives

`PLAN.md` indexes the specification: fifty sections in `plan/`, ordered into
seventeen milestones. M0 is tagged; M1 and the MCP door of M2 are largely
built. The planner (projects, work items, the brief) moved out to
[docket](https://github.com/borismilner/docket), which reaches rig over the
socket like any other program. `design/visual-system.html` is the live visual
system: one file, open it in a browser.

## Layout

```
docs/orientation.md  start here: what exists today, where it lives
docs/programs.md     integrating a program: Go, Python and any other language
examples/            a Go and a Python program to copy
PLAN.md              the generated index of the specification
plan/                the specification, one file per section (50)
cmd/                 rigd, rig, rigwindow, the fake programs, the gates
internal/            the daemon: kernel, wire, record store, MCP, estates
client/              the stub a program embeds
proto/, schema/      the wire schema and the JSON schema generated from it
frontend/            the window's Svelte app
packaging/           systemd user units
tools/               installer, plan splitter, contrast audit
design/              the visual system, live in one HTML file
  theme.js             the engine: a small config object to the whole token set
  visual-system.src.html   markup and CSS. Edit this, not the built file
  build.py             inlines the scripts into visual-system.html
  readme-art.mjs       the artwork on this page
cmd/ipcbench/        the transport benchmark behind the table above
Makefile             the targets the milestones are demonstrated with
```

## Non-goals

- rig does not run business logic. A program id appearing in rig's code is a bug.
- rig is not required. Every program still works without it, at reduced service.
- rig does not replace a program's own CLI. `shelf` still works as `shelf`.
- rig does not own data. Programs own their data; rig owns the plumbing around it.
- rig does not hot-upgrade itself. Measured at 1.9 ms of benefit against four
  defect classes. Notice, restart, resume.
- rig does not host anything we did not write. Third-party code is a separate
  process, or it is not run.

## The name

A rig is a platform. A rig is your whole setup. Rigging is the ropes and tackle
that control a ship. To rig up is to assemble. Four meanings, all correct.
