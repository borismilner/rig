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
