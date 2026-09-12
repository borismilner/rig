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
