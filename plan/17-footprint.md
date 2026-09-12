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

### THE FOOTPRINT GATE IS OFF, 2026-09-12, BY BORIS

**`make ci` no longer runs `bench-size`.** The budget below stands as a design
target and nothing enforces it automatically any more.

**Why, and the measurement is the argument.** `cmd/sizeratchet` compared
`now > was` against a number recorded at the previous build, so it had **zero
headroom by construction** and fired on any growth at all. Presence (§16 row 1)
grew five binaries by one or two 4,096-byte pages each - entirely generated
wire types that every binary links whether it calls them or not - and the gate
fired exactly as it would have for a twenty-megabyte dependency. **The only
available response was to re-record the number, so every firing was answered by
an override and the control refused nothing.** Boris raised it directly:
*"Do you really need to manage size-ratchet.json?"*

**WHAT THIS COSTS, said plainly rather than buried.** Nothing now catches a
dependency that quietly adds megabytes to `rigd`. **`make bench-size` still
works and still prints the table**; it is a thing a seat runs deliberately, not
a thing the build runs for it. §22's dependency bar and §38b's search
obligation are what remain, and both are judgement rather than a gate.

**A ceiling with headroom was offered and NOT chosen**, so a seat must not
reintroduce one as a gate without asking again.

### The budget

| What | Budget | Measured by |
|---|---|---|
| **`rigd` binary size** | **< 20 MB, and ratcheted** | `make bench-size`. A PR adding more than 1 MB fails unless this line is edited in the same commit. This is the budget that causes the next one. **Edited at M1 slice 5: 6.29 -> 7.75 MB**, and the whole 1.45 MB is `santhosh-tekuri/jsonschema` v6, which §22 pins. It has to be in the daemon rather than the client, because a program cannot trust a caller to have validated its own arguments (§5e). 39% of the budget spent at M1 of 16, and the next surface that wants a library this size is the one to argue with. **Edited again at M2 slice 3: 7.75 -> 10.90 MB**, and the whole 3.09 MB is `modelcontextprotocol/go-sdk` and the tree it brings - `google/jsonschema-go`, `segmentio/encoding`, `segmentio/asm`, `yosida95/uritemplate`, `golang.org/x/oauth2`, `sync` and `time`. **It is in `rigd` because §37's transport ruling put the MCP server there, on its own socket, and no version of that ruling leaves this row where it was.** **54% of the budget spent at M2 of 16**, and that is the number to read rather than the delta: 39% at M1 and 54% at M2 is a trajectory, and two more surfaces of this size do not fit. **The next library over 1 MB is an argument to have with §2, not a ratchet row to raise** |
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
