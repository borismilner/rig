## 22. Tech stack

Versions verified 2026-09-10.

| Concern | Choice | Version |
|---|---|---|
| Language | Go | 1.27.1 |
| Wire | protobuf messages, hand-framed on a unix socket | google.golang.org/protobuf v1.36.x. **Not gRPC**: measured +9.80 MiB resident for HTTP/2 machinery a local socket does not need (§17) |
| Schema | JSON Schema 2020-12: santhosh-tekuri/jsonschema (validate). Emission needs no library: `cmd/schemagen` walks the proto descriptors | v6.0.3 |
| Config | knadh/koanf/v2 | v2.3.6 |
| CLI | spf13/cobra | v1.10.2 |
| Store | modernc.org/sqlite | v1.59.0. **ADOPTED - in `go.mod` today, and it is the single largest dependency rig has: +5,001,319 bytes**, measured. **ANSWERED BY B28, 2026-09-16** - confirmed over bbolt and over a hand-rolled store, and it survived a 100x scale arm at 613us flat. **bleve was REFUSED on footprint.** `modernc` and not `mattn/go-sqlite3`: the cgo driver **builds clean with `CGO_ENABLED=0` and panics on first use**, so the gate cannot catch it |
| Coordination store: leases, epoch, CAS | **REMOVED 2026-10-01**: `go.etcd.io/bbolt` left `go.mod`; coord is on `modernc.org/sqlite` through `internal/store` (§48 decision 1, decision 0251) | Measured on `rigd` at the swap, the same flags at `0d944f0` and `0b76df5`: **18,235,552 to 17,911,968 bytes, -323,584**, against the +352,256 measured when it was adopted. The three live `coord.db` files were converted once; the converter is `cmd/coordconvert` at `8aa9a1c` |
| Boot-time clock | `golang.org/x/sys/unix`, promoted indirect -> direct | v0.47.0 - **moved up by the sqlite add, not by a decision of ours**. ⛔ **ONE VERSION ON THIS ROW, DELIBERATELY:** naming the superseded pin beside it makes `depscheck` treat the row as offering a choice and it STOPS COMPARING THE ROW ALTOGETHER - measured here, the uncompared count went 14 -> 15 and the row went green by no longer being checked. **ADOPTED - in `go.mod` today. +4,096 bytes on rigd** - one page, because rigd already links `x/sys/cpu`. `unix.ClockGettime(CLOCK_BOOTTIME)` is the only clock that excludes suspend, and §16's deadlines are stored absolute as boot id + boottime deadline |
| Record ids | `github.com/google/uuid`, promoted indirect -> direct | v1.6.0. **ADOPTED 2026-09-16** for §39's record ids, granted by the lead as a one-line direct promotion. **UUIDv7 and not v4: the ordering is the entire reason** - a record stream sorts by id. `project` and `case` keep SLUGS per §39's id scheme, so this does not mint every id. **No new module enters the graph** - it was already `// indirect`. **Binary cost is UNMEASURED and honestly so: no shipped binary links `internal/record` yet**, so there is nothing to weigh; it arrives with the wire surface and is attributed there. **`cmd/rig` and `internal/daemon` link `internal/record` as of 2026-09-24** (`rig record put/get/query/history/link/unlink/refs/retract/delete/replace` ships), so this cost is no longer unattributed and the row needs a measured number |
| Timer schedules (§52 Q2) | `github.com/robfig/cron/v3`, parse only: rig keeps its own realtime timerfd and never runs cron's scheduler | v3.0.1. **ADOPTED 2026-10-01** by §52 slice 3 under decision 0255 (Q2, both syntaxes). Measured on `rigd`, the same flags against `f60483a`: **18,014,471 to 18,116,871 bytes, +102,400**, which includes the timer code itself. No dependencies of its own |
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
| Toast content | shiki | 4.4.3 |
| Markdown in the window | `marked`, **as a LEXER and never as a parser** | 18.0.12, pinned exactly. ⛔ `marked.parse()` returns an HTML STRING, which would have to be injected and then sanitised forever; `marked.lexer()` returns a token TREE the view walks into Svelte markup, so §38's "no injection-shaped string building" holds by construction rather than by a check. Candidates weighed 2026-09-18: `snarkdown` (1 KB, no tables, HTML strings only - fails the 348 bodies that hold a table), `markdown-it` (~3x, plugin model nothing needs), `micromark` (events not a tree, needs a second package). Raw HTML inside a body renders as its own characters: 56 bodies contain a `<` and every one MENTIONS markup rather than intending it |
| Program icons in the window | Lucide (`lucide`, ISC), imported one icon at a time | 1.47.0, pinned exactly. `Identity.icon` names one; a name is never markup (§11) |
| Testing | stdlib, testing/synctest, go-cmp | v0.7.0 |
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
`glamour`, `huh`, `go-cmp` and `systray` are **all named here and
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

**The two rows added 2026-09-16 - bbolt and `x/sys` - are what that marking
should look like, and they are the first rows to arrive carrying it.** Each
names ADOPTED, says the manifest it is in, and carries the byte cost measured
**on `rigd` rather than over a hello-world**, which is the number that decides
anything: bbolt is +1,186,587 bytes over an empty binary and +352,256 on top of
rigd, because rigd has already paid for the runtime, fmt and reflect that
larger figure includes. **A row without the on-binary number is not evidence of
adoption, it is a citation.** Both were measured against the `rigd` at
`f921a04`, whose 10,998,023 bytes match the size ratchet exactly - so the
baseline is known not to have drifted under the measurement.
**bbolt's row has since read REMOVED (2026-10-01, S1 slice 4)**; it
keeps its on-binary numbers, which is what a removal row should look like
too.


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

### ⛔ The library audit, 2026-09-24, for Boris's "best of the absolute best" ruling (§12's header quotes it)

Every direct dependency, what it is for, the named alternatives, and the verdict. **Measured**
means a number taken on this date; **reasoned** means the verdict rests on the alternatives'
known properties and was not re-measured here, and says so.

| Dependency | For | Alternatives | Verdict |
|---|---|---|---|
| `google.golang.org/protobuf` | the wire | gogo/protobuf (dead), vtprotobuf (generated fast marshal on top of this) | **keep.** Reasoned. vtprotobuf is the one worth measuring: §48 found serialising, not transport, dominates a large answer. **For the lead**: a measured trial on `record.query` pages |
| `modernc.org/sqlite` | the record store | mattn/go-sqlite3 (cgo), ncruces/go-sqlite3 (wasm) | **keep.** Pure Go keeps `rigd` free of cgo (§17, §22). Measured today: 13% of daemon CPU under the soak, the rest is syscalls and scheduling |
| `go.etcd.io/bbolt` | the lease and epoch store | SQLite (already linked) | **REMOVED 2026-10-01** by S1 slice 4, -323,584 bytes on `rigd` (row above) |
| `github.com/wailsapp/wails/v3` (beta) | the window | webview/webview_go, gotk4 by hand, Tauri (Rust) | **keep, with the beta risk named.** It is the only Go option with typed bindings and a maintained GTK4/WebKitGTK-6 backend; its Linux tray cannot report the icon's position (measured in source: `bounds()` returns an empty rect) |
| `fyne.io/systray` | the tray process | Wails' own tray (needs GTK in the tray process), getlantern/systray (unmaintained) | **keep.** It keeps the tray process free of GTK and WebKit, the footprint ruling's reason for the separate process |
| `github.com/modelcontextprotocol/go-sdk` | the MCP door | mark3labs/mcp-go | **keep.** The official SDK, maintained with the spec |
| `github.com/santhosh-tekuri/jsonschema/v6` | argument schemas | xeipuuv/gojsonschema (draft-07, unmaintained), qri-io/jsonschema | **keep.** Full 2020-12 and the strictest of the three |
| `github.com/google/uuid` | UUIDv7 record ids | gofrs/uuid | **keep.** Either would do; this one was already indirect (row above) |
| `github.com/robfig/cron/v3` | §52 timer schedules: the five-field cron line and @daily | adhocore/gronx, netresearch/go-cron (a maintained fork), go-co-op/gocron (a whole scheduler, built on robfig), a parser by hand | **keep.** Measured +102,400 bytes with the timer code. The de facto parser, stable since 2020 and with no dependencies; its last release is old, which a parser of a fixed grammar tolerates. A parser by hand is §38's named failure. The fork is the swap if a defect needs fixing |
| `golang.org/x/sys` | SO_PEERCRED, CLOCK_BOOTTIME, getsid | syscall (frozen) | **keep.** Required |
| `go.uber.org/goleak` (test only) | goroutine-leak checks | NumGoroutine counts | **keep.** Adopted today; linked into no binary |
| `github.com/godbus/dbus/v5` | the toast fallback to `org.freedesktop.Notifications` (§12), in `rigwindow` | exec of `notify-send` (a binary that may be absent), gdbus | **keep.** Promoted indirect to direct 2026-09-24: `fyne.io/systray` already links it into `rigwindow`, so no new module enters the graph. The one D-Bus library Go has that is maintained |
| `golang.org/x/mod` | `cmd/tagcheck`: reading release tags as Go reads them (`semver`) | a hand-written semver regex | **keep.** Promoted indirect to direct 2026-09-24: it was already in the module graph, it is the Go toolchain's own reading of a version, and `tagcheck` is linked into no shipped binary |
| `@wailsio/runtime` | the window's bridge | none: it is Wails' own | **keep** |
| `svelte`, `vite`, `@sveltejs/vite-plugin-svelte`, `tailwindcss`, `@tailwindcss/vite` | the window's frontend | React, Solid; webpack | **keep.** Reasoned: Svelte 5 compiles to the smallest runtime of the three, which is the footprint ruling's question |
| `marked` | markdown in panes | markdown-it, micromark | **keep.** Reasoned: the smallest and fastest of the three for trusted input |
| `lucide` | program icons in the rail and dashboard (§11) | phosphor, tabler, heroicons | **keep.** ISC, no dependencies, the widest set, and tree-shaken per icon so the bundle carries only the thirty offered |
| `@playwright/test` | the contrast gates | puppeteer | **keep.** It drives the real browser the gates need |
| `typescript`, `tslib`, `@tsconfig/svelte`, `prettier`, `prettier-plugin-svelte`, `vitest`, `json-schema-to-typescript` | build and test tools | - | **keep.** Dev-only; none ships in a binary |

**Performance leaks, measured 2026-09-24:** goleak now runs at the end of the daemon and client
test packages and found two leaks, both fixed (`Daemon.Close`, a test's dialer). The daemon soak
(`RIG_SOAK=3m`) ran 68,078 rounds with goroutines 5 -> 5 and heap 1,170 KiB -> 1,212 KiB. Under
load the CPU profile is syscalls 15%, futex 10%, SQLite 13%; in-use heap 2 MB. `rigd` idle: 17.3 MB
resident, 10 ms of CPU in 12 s.
