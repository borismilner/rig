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
| M8 | The tray, and the window over real programs | Embedded mode over the localhost SPAs the estate already serves, one tray icon with the **detached** state, **the decision on who hosts the tray and the toast layer, with its §17 budget row** (§17), **the tray icon's own design, which is Boris's standing requirement** (§11 - *"eye-catching and engaging"*, his words, and it is not satisfied by the visual system's version of that request), and **the first real adopter of the element kit, which is where R1 closes and R4's generations begin (§5h)** | One tray icon where six were, real programs in the rail M1a built, and one of them serving its own HTML through kit elements that a fake application shook out first. **The icon is measured at 22px against light and dark shell panels, in all four states including `detached`, rather than approved at display size.** **`make bench-idle` covers every resident rig process, not only `rigd`** |
| M9 | Toasts, and speech | The frameless toast, severities, springs, stacking, live bodies, inline actions, the centre, Do Not Disturb, D-Bus fallback; **speech as a renderer on the centre (§12) - one estate-wide queue, a long-lived engine, engine and player resolved at startup, its §17 budget row, and its `rig doctor` rows** | A command answered from inside a toast with no window open. **Two programs notify at once and are heard one after the other rather than over each other; Do Not Disturb silences both while an `ask` that gates a command still speaks; and with the engine uninstalled rig still toasts and `rig doctor` says why it is quiet** |
| M10 | Generated UI | Forms, tables, actions, progress, detail, status from declared schema, in both window and TUI | `nudge` gets a complete pane and a complete TUI view with zero frontend code |
| M11 | Storage and secrets | Managed location, migration runner, backup, integrity, retention, browser; keyring with per-program namespaces, **in `rigd`** (§22); **`rig backup` and `rig restore` for rig's own state** (§7) | A program's migration runs before it starts, and its backup restores. **A fresh machine restores the audit log, the notification centre and the config tree from one archive** |
| M12 | Pilot: shelf | shelf entirely on rig: config, storage, logs, tray, pane, TUI, commands on every surface. A week of daily use | shelf loses its own tray icon and loses no capability |
| M13 | Palette, search, schedule, bus, URL | Cross-program palette, federated search, one scheduler **with the missed-fire policy (§18)**, events with grants, `fs.changed` from the file-watcher source and `grant.revoked` (§5h), `rig://` | graft finishing a run triggers a shelf reindex, with the grant visible and revocable; and an overnight suspend coalesces four missed idempotent reindexes into one |
| M14 | Hardening | Chaos at full size, fuzz corpora, cgroups and landlock, security review, coverage to target | The chaos suite green over 10000 iterations |
| M15 | Packaging and updates | `.deb`, desktop entry, the signed update channel for rig and programs, self-update. **The autostart unit itself moved to M6** (§5l); what is left here is packaging it | Fresh machine to a working rig with three programs in one command, **and `rig restore` carries the state M11 backed up** |
| M16 | Estate migration and the AgentBox cutover | The rest of the estate, in the order in §25, then the agent tooling repointed from AgentBox to the peers service - **and every agent's instructions repointed from AgentBox's `speak` to rig's speech (§12), which is a decision M9 deliberately does not make** | Every in-house program reachable from one CLI, one TUI, one tray, one window, one MCP server |

### M3 TO M6 ARE CHERRY-PICKED, RULED BY BORIS 2026-09-11

**His words, choosing between three shapes put to him the day M2 closed:**
*"M3-M6 cherry-picked for only what M7 and §37 actually need."*

**This does not renumber anything and does not cancel anything.** M3, M4, M5
and M6 keep their numbers, their contents and their demos. What changes is
WHEN the parts below are built: the listed items move ahead of M7 because M7
or §37 cannot be finished without them, and everything else in those four
milestones waits.

**The ruling this follows from is his earlier one**, that the agent-facing path
gets perfected first. M2 was the half of that which was one slice from done and
it closed on 2026-09-11. M7 is the other half.

#### What is KEPT, with what needs it

| From | Kept | Needed by | State today |
|---|---|---|---|
| **M6** | **A client-generated request id on every mutating call** | **M7, and §16 says so in as many words**: without the request id and the session token *"the claim above is false on any retry"*, because a `barrier.arrive()` that times out and is retried releases a barrier of nine with eight agents present | **THE FIELD EXISTS, THE MECHANISM DOES NOT - CORRECTED 2026-09-11.** `request_id` is wire field 4, plumbed both directions since M0 and set by nothing. **There is no dedup in the tree at all**, so the superseded "the daemon dedups against a bounded window" was false rather than half true. §5f puts the window in the **WAL**, which lands at M7, so **M6 owes the CLIENT half only** - a client-generated id on every call - and the window arrives with the WAL. See §14's clause row on binding approval to one invocation, which carries the same correction. **Cited by behaviour rather than by line: this reference used to be a line number and rotted twice in one evening** |
| **M6** | **A session token that survives a reconnect** | same sentence of §16, and §37 precondition 4 | **NOT ON THE WIRE AT ALL.** `grep session proto/rig/v1/wire.proto` returns nothing. A proto change, so it batches with anything else in flight there |
| **M6** | **The tolerant client** (§5g): reconnect loop with backoff to a deadline, bounded outbound queue, one typed `unavailable` | §37 precondition 4 - *a restart is survivable and distinguishable from a blip*. An agent whose daemon restarts mid-development must reconcile rather than fail | not built |
| **M6** | **The `systemd --user` unit with no `ExecStop`** (§5l) | **§37 precondition 5**, and nothing else. Production must still be serving after a logout or the agents cannot depend on it | not built. **The unit sets no `XDG_RUNTIME_DIR`**, which is what makes it unable to reach `development` - see §37 |

#### What WAITS, and why each is genuinely off the path

| Deferred | Why it is not needed |
|---|---|
| **M3 entirely** - `rig shell`, the `rig tui` frame, `huh` forms | **agents do not use a TUI.** M7's own contention and timeline views are TUI views and defer with it; they are observability OF the coordination service, not part of it |
| **M4's config layers, provenance, diff and the `events` service** | **M7's signals and watches are its own primitives**, not subscriptions on the config bus. §16 gives `post`/`await` and a watch with one global revision and a cursor. The rider - news appended to the result of whatever call you make next - needs no subscription at all, which is its whole point |
| **M5 entirely** - ingest, the call log, redaction spans, `rig doctor`, `rig logs` | none of it is reachable from a coordination primitive, and §14 already gates history reading behind M5's redaction spans rather than the other way round |
| **M6's start, stop, restart, health, budgets, quarantine and lifecycle notices** | supervision of OTHER programs. M7 coordinates agents that are already running |
| **The three supervisor event kinds, `system.resumed` included** | **THE ROW STANDS AND ITS REASON IS REPLACED, 2026-09-11.** It used to defer as "supervision of OTHER programs", decided for all three on one sentence without checking §5h's basis for any. That is wrong for `system.resumed`: its basis is *"rig owns `boot_id` and `CLOCK_BOOTTIME`"*, and §16's resume grace epoch is a LEASE-CORRECTNESS mechanism, not supervision. **The deferral survives on other grounds** - M7 detects a suspend gap by comparing `CLOCK_BOOTTIME` against `CLOCK_MONOTONIC` and needs no event kind to do it. Only a PROGRAM being *told* about a resume needs the event, and no program needs that before M7 |
| **M6's resolved snapshot on disk** | a config-availability mechanism. Coordination state has its own durability - see the correction below |

#### THE CORRECTION THIS RULING FORCED, and it moves a §37 precondition

**§16 gives the coordination service its own write-ahead log:** *"Durability
comes from a write-ahead log: coordination state survives a daemon restart, and
clients reconcile on reconnect rather than losing their place."* Continuation
slots say the same thing about themselves - `rigd` writes slots to the WAL.

**So M7's WAL is the first persistent state rig holds, and §37 precondition 2
was anchored to M5 on the assumption that storage arrives there.** It does not
arrive there first. **Precondition 2 now bites at M7**, and its failure is the
one already written down: a development estate writing into production's store
with no error at all - except that the store in question is the coordination
WAL, so what leaks across is leases, claims and the blackboard rather than
logs.

**The specification written on 2026-09-11 is what the WAL path must use:**
`$XDG_STATE_HOME/rig/estates/<name>/`, and an unnamed estate gets none. **An
ephemeral estate has no WAL**, which is consistent rather than awkward: §18
already says nothing rig holds survives a restart for callers who did not claim
a name, and every test in this repository starts an unnamed estate.

**v1 is M0 through M13.**

**M1a is inserted rather than numbered, and that is deliberate.** The GUI moved early on
2026-09-10 because a fake application that uses the shell is the only demonstration of it that
exists before a real program is asked to change, and because the element kit had been specified
for two years' time against an inventory nobody had built. Renumbering M2-M16 to make room
would have moved **85 milestone references**, moved §24's M3 gate off the milestone it is
anchored to, and rewritten the numbering under a session that was mid-M1 at the time. A letter
costs one line of explanation; a renumber costs every one of those references.

---
