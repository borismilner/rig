## 44. What rig owes itself before the planner leaves

**RULED BY BORIS, 2026-09-19**, the same day as §43 and immediately after it.
This is the ORDER decision §43 left open, and he took the option §43 named as
matching his own sentence.

> *"I want to complete missing services/capabilities in `rig` before we split
> out the planner. It must support the storage aspect, logging and everything
> else `rig` is using that in should not have implemented if it had `rig`."*

**The spelling is his and is kept.** *"that in should not have implemented"*
reads as *that it should not have implemented*.

⛔ **THIS ANSWERS §43's OPEN QUESTION 2 AND LEAVES 1, 3 AND 4 OPEN.** M11 comes
first. Whether the record store stays in rig, what the MVP sentence becomes,
and the planner's name are all still his.

---

### ⛔ THE TEST IS HIS, IT IS MECHANICAL, AND IT IS WHAT MAKES THIS LIST FINITE

**His sentence contains a rule, not just two examples:** *"everything else rig
is using that it should not have implemented if it had rig."*

> **A service is owed if rig USES it today. It is not owed if rig merely
> PROMISES it.**

**That is checkable by grep and it cuts the list from thirteen services to
five.** It is the reason this section is short and the reason it can be
finished. **A seat that widens it to everything §1 promises is building a
different thing**, and §38's swiss-knife rule says why that is wrong: best at
what it carries, and it does not grow new capabilities to be broader.

---

### The audit, run 2026-09-19 against the tree at `e12c946`

**§1's own sentence is the service list**, quoted from the top of this
specification: *"configuration, storage, secrets, logging, tracing,
notifications, a window, a tray - and it controls them: starting, stopping,
invoking, scheduling and wiring them to each other."* Every row below was
measured, not read.

| Service | Offered to programs? | What rig does for ITSELF | Owed? |
|---|---|---|---|
| ⛔ **storage** | **NO** | ⛔ **two hand-rolled stores, 959 lines, TWO engines and TWO migration runners** | ⛔ **YES** |
| ⛔ **backup / restore** | **NO** | ⛔ **a hand `cp`**, done by a seat this very day | ⛔ **YES** |
| ⛔ **logging** | **NO** | `slog` to stderr, swept up by the journal | ⛔ **YES** |
| ⛔ **configuration** | **NO** | three flags in `cmd/rigd/main.go` and some `os.Getenv` | ⛔ **YES** |
| ⛔ **migrations** | **NO** | part of storage, and written **twice** | ⛔ **YES** |
| tracing | NO | **nothing.** No span, no exporter | no - rig does not use it |
| secrets | NO | **nothing.** rig holds no secret | no - rig does not use it |
| scheduling | NO | **nothing.** No `time.Ticker`, no cron in the tree | no - rig does not use it |
| wiring, the event bus | NO | **nothing.** No `Publish`, no `Subscribe` anywhere | no - rig does not use it |
| notifications, toasts | NO | **nothing** | no - rig does not use it |
| starting / stopping | NO | **systemd units. `rigd` execs nothing** | no - rig does not use it |
| invoking | **YES** | its own | built |
| a window | **YES**, at the pre-kit tier | `rigwindow` | built |
| a tray | **YES** | one icon | built |

⛔ **THE HEADLINE FINDING, AND IT IS THE CASE FOR THIS WHOLE SECTION IN ONE
LINE: rig HAS IMPLEMENTED STORAGE TWICE AND OFFERS IT TO NOBODY.**
`internal/record/store.go` is 661 lines on `modernc.org/sqlite` versioned with
`PRAGMA user_version`; `internal/coord/store.go` is 298 lines on `bbolt` with
its own `migrations` map. **Two engines, two runners, two sets of open-and-
migrate rules, in one daemon, for one user.** An adopting program gets neither.

**The second finding is smaller and it is the one that costs today:** a seat
backed up his production store with `cp` before re-seeding it, because
`rig backup` does not exist. **§7 already specifies it and M11 already carries
it.** This is a planned capability that would have solved it, which is the form
§37 asks reports to take.

---

### The five services, specified

⛔ **THE ACCEPTANCE TEST FOR EVERY ONE OF THEM IS THE SAME, AND IT IS NOT "THE
SERVICE EXISTS".**

> **A service is DONE when rig's own hand-rolled version is DELETED and rig is
> the service's first consumer.**

**Why the test is written this way:** a service that ships beside the private
implementation it was meant to replace is a second thing to maintain, and this
project has recorded the shape before - a check that passes by not checking, a
green that could not have gone red. **A `git grep` proving the old path is gone
is the evidence; the service compiling is not.**

#### S1. Storage - the service, one engine, one migration runner

**Milestone: M11.** §7 and §22 carry it.

| Clause | What it means |
|---|---|
| **a wire surface** | open, read, write, transact, on `proto/rig/v1/wire.proto`. There is no storage verb on that wire today |
| **one engine** | ⛔ **the two-engine split is a finding, not a design.** `modernc.org/sqlite` is ruled by B28 after a search that was run. **bbolt's presence in `internal/coord` predates that ruling and has never been re-argued** |
| **one migration runner** | schema version read, forward steps applied, version stamped, all in one transaction. **Both current runners already do this correctly and separately** - the service is the third writing of code that exists twice |
| **per-program namespace** | a program's data is its own. §29's *"rig does not own a PROGRAM's data"* is the constraint: rig owns the plumbing, not the contents |
| **the migration runs before the program starts** | M11's own demo clause, unchanged |
| ⛔ **rig's two stores become its first two consumers** | **this is the acceptance test.** `internal/record/store.go` and `internal/coord/store.go` stop opening a database |

#### S2. Backup and restore

**Milestone: M11.** §7 names `rig backup` and `rig restore` for rig's own
state.

- **A whole-estate archive**, restorable onto a fresh machine.
- ⛔ **It must cover the record store**, which §7 exempts from every retention
  rung precisely because *"nothing is ever lost"* is the property §39 exists to
  provide. **A store with no backup does not have that property**, and today it
  has no backup.
- **The acceptance test:** the `cp` in a seat's shell history is replaced by a
  command, and a restore is demonstrated onto a throwaway estate.

#### S3. Logging - ingest, not a logger

**Milestone: M5.** §8 carries it.

- **Programs send log records to rig**; rig merges, stores and serves them.
- ⛔ **`rig logs` is the read side and it is what makes this worth doing** -
  today rig's own logs are in the journal, which is a per-unit view of a
  multi-process estate.
- ⛔ **M5's compiled redaction spans are a DEPENDENCY, not a nicety.** §14
  gates complete history reading behind them, and §13's sensitive-field
  declarations mean nothing until something enforces them. **A log service
  shipped without redaction puts a declared-sensitive value in a segment**,
  which is the one failure §8's own demo is written to catch.
- **The acceptance test:** `rigd`'s `slog` handler writes into rig, `rig logs`
  reads it back, and a known secret through a declared-sensitive field appears
  in no segment.

#### S4. Configuration

**Milestone: M4.** §6 carries it.

- **Layers, schema, provenance, validate, export, diff**, and live push.
- ⛔ **rig's own three flags are the first consumer**, which is a small surface
  deliberately: `--log-level`, `--estate` and `--version` are the whole of
  rig's configuration today and that makes the first consumer cheap to convert
  and easy to prove.
- ⛔ **`rig config origin` explaining a surprising value is M4's demo and it
  stays.** It is also the one clause that cannot be met by a flag parser, which
  is why this is a service rather than a library.
- **NOT in scope here: the `events` service.** M4 bundles `config.changed` with
  the bus, and **rig uses no bus today**, so the live-push half fails his test.
  Flagged so the bundling does not smuggle it in.

#### S5. The migration runner

**Folded into S1 rather than numbered separately**, and named here because it
is the part that is written twice and the part whose failure is silent.
`internal/record/store.go` says so about itself: *"THIS IS THE ONE REQUIREMENT
IN THE SET WHOSE FAILURE IS SILENT."*

---

### ⛔ WHAT IS DELIBERATELY OUT, AND WHY EACH IS OUT ON HIS OWN TEST

**Recorded so a later seat does not read the absence as an oversight**, which
is this project's named missing-row failure.

| Out | Because |
|---|---|
| **tracing** (M5) | rig emits no span. **It ships with the logging milestone and must not be assumed into S3** |
| **secrets** (M11) | rig holds no secret. It ships with the storage milestone and is a separate clause |
| **scheduling** (M13) | no ticker and no cron anywhere in the tree |
| **the event bus** (M4/M13) | no `Publish` or `Subscribe` exists. ⛔ **THE PLANNER WILL WANT `record.changed` FOR A LIVE WINDOW, AND THAT IS A PLANNER WANT RATHER THAN RIG'S OWN USE.** It fails his test and is flagged for him rather than folded in |
| **notifications and toasts** (M9) | rig notifies nobody today |
| **supervision** (M6) | `rigd` execs nothing; systemd starts it. **Different from the M6 items already cherry-picked** for §37, which stay where they are |

---

### ⛔ WHY THE SERVICES EXIST, AND HOW THEY ARE CALLED. BORIS, 2026-09-19.

**Stated while this section was being written, and it is the reason the five
services are worth the work at all:**

> *"The idea is that `rig` will control and develop and improve the
> technologies behind those capabilities it offers the different projects so
> they do not have to do it on their own, and they will call this functionality
> as already defined in my requirements by using best possible ways of
> interactions with rig to allow for the absolutely best performance."*

⛔ **HE IS RIGHT THAT IT IS ALREADY DEFINED, AND IT IS DEFINED IN TWO PLACES.**
Grepped rather than remembered, which is §38's rule about proposing what
already exists.

**The WHY is §3, "Upgrades independently", which §3 itself calls the defining
maximal:**

> *"Ship a new rig with a redesigned toast, a new settings UI and a new log
> viewer. No in-house program is rebuilt, and all of them show the new
> behaviour on next connect."*

**and §1's own second paragraph:** *"No in-house program imports rig. They talk
to it over a socket. That is the whole point: rig upgrades on its own, and
every program gets the improvement without being rebuilt."* **His sentence is
that requirement restated, so nothing new is recorded here - the citation is.**

**The HOW is §4 and §5's two-plane split**, and it is measured rather than
argued:

| Plane | Cost | What uses it |
|---|---|---|
| **control**, request/response | **6.2 µs** | config, commands, queries, health |
| **data**, one-way | **561 ns** | logs, traces, metrics, progress |
| **bulk**, 64 KB frames | 19 µs, 3.4 GB/s effective | large payloads |

#### What that binds, service by service

| Service | Plane | Already ruled? |
|---|---|---|
| **S3 logging** | **data, one-way** | ✅ **YES, by name.** §4: *"Logging does not need shared memory. A plain socket write is 561ns and sustains 1.8M records per second. Nothing here will produce a thousandth of that"* |
| **S4 config** | **control** | ✅ **YES, by name.** §4: *"A config read at startup ... at 6µs, thousands of times over, is noise"* |
| **S2 backup** | **bulk** | ✅ **YES.** 64 KB frames at 3.4 GB/s effective |
| ⛔ **S1 storage** | **control, PROVISIONALLY** | ⛔ **NO. THIS IS THE GAP HIS SENTENCE OPENS** |

#### ⛔ S1 IS THE ONE THE EXISTING NUMBERS DO NOT COVER, AND §4 SAYS WHAT TO DO

**Every workload §4 priced is occasional: a config read at startup, a
notification, a command, a health check.** ⛔ **Storage is not occasional.** The
planner is almost entirely a store, and a brief derivation that today runs as
in-process SQL becomes N round trips at 6.2 µs each the moment the store is on
the other side of a socket.

**§4 already pre-authorises the method and forbids guessing at it:**

> *"Shared memory is not built. It is 43x faster and there is no workload that
> needs it. If one ever appears, the number above is what justifies adding it,
> and not before."*

⛔ **SO S1 OWES A MEASUREMENT BEFORE IT OWES A DESIGN**, and the harness
already exists - `cmd/ipcbench`, which is how §4's own table was produced.

| What must be measured | Why it decides the shape |
|---|---|
| **`project.brief` end to end, in-process versus over the socket** | It is the heaviest real read this estate has, and it already exists to measure |
| **the round-trip COUNT a brief costs**, not only the per-call cost | 6.2 µs is noise at ten calls and is 6 ms at a thousand. **The count is the number that decides, and nobody has it** |
| **whether a batched or streamed query collapses that count** | The cheapest fix by far, and it needs no new transport |
| **shared memory, last** | §4 ruled it out until a workload justifies it. **This may be the first one. It may equally be answered by batching, which is why batching is measured first** |

⛔ **A SEAT THAT PICKS A TRANSPORT BEFORE RUNNING THAT MEASUREMENT IS
GUESSING**, and §4 exists precisely because this project prices things instead.
**`BACKLOG.md` B108 carries it, and it is a prerequisite of B103 rather than a
follow-on.**

---

### ⛔ THE ENGINE IS rig's PRIVATE CHOICE, AND EVERY SERVICE HAS A VISUAL HALF. BORIS, 2026-09-19.

**Sent immediately after the performance requirement, and it decides S1's API
shape rather than merely describing it:**

> *"The idea is that the projects will be agnostic to whether the storage used
> is SQLite or Postgres or anything else, they just consume `rig` capabilities
> as a service. This allows perfecting both visual and non-visual aspects of
> all of these services all in-house projects will consume."*

#### ⛔ WHAT IT RULES OUT, BEFORE ANYBODY PROPOSES IT

⛔ **SQL OVER THE WIRE IS REFUSED.** It is the obvious cheapest storage service
and it fails this requirement outright: **a program that sends SQL is coupled
to the dialect, and therefore to the engine, and rig could never change it.**
The same argument refuses a thin `database/sql` proxy and any surface that
leaks a cursor, a table name or a driver error.

**What is required instead: an abstract data API** - put, get, query by field,
transact, watch - **that rig maps onto whatever engine it has chosen today.**
§39's record verbs are the working precedent: nothing in `record.put` or
`record.query` names SQLite, and the engine is already swappable behind them.

⛔ **AND THIS RE-FRAMES THE TWO-ENGINE FINDING RATHER THAN CANCELLING IT.** The
question stops being *"pick one engine forever"* and becomes *"the engine is
rig's private choice and may change with no program rebuilt"*. **That is §3's
defining maximal applied to storage**, and it means the SQLite/bbolt split is
still a defect - two private choices where one is owed - but a defect rig can
fix at any time once no program can see it.

#### ⛔ AND IT COLLIDES WITH THE PERFORMANCE REQUIREMENT, WHICH IS THE REAL DESIGN PROBLEM OF S1

**His two statements, one turn apart, pull against each other and both bind:**

| | Requires |
|---|---|
| **agnostic** | the program cannot express its own query in the engine's language |
| **absolutely best performance** | the program must not pay a round trip per row |

⛔ **SO THE QUERY API MUST BE EXPRESSIVE ENOUGH TO PUSH THE WORK DOWN.** A
program that cannot write SQL and cannot push a filter, a join or an aggregate
is left doing N+1 round trips at 6.2 µs each, **which is the exact failure
B108 exists to measure.** The two requirements are satisfiable together and
only by a deliberately-designed query surface; they are not satisfiable by a
key-value get.

**`project.brief` is the worked example and it is already built**, which is why
B108 measures it: it is the heaviest real read in this estate, it aggregates
across kinds, and §39 already records that one of its four SQL formulations
was **582x** worse than the one taken. **A brief the planner has to assemble
client-side is that 582x arriving through the API instead of through the SQL.**

#### The visual half, and it is already specified per service

**His *"both visual and non-visual"* is §3's Expandable maximal**, quoted:
*"A new program registers and becomes fully reachable - CLI, TUI, MCP, tray,
HTTP, palette, schedule - with **zero frontend code and no rig-specific
logic**."*

| Service | Its visual half | Already specified |
|---|---|---|
| **S1 storage** | a store browser | M11 names it |
| **S2 backup** | restore, visible and driveable | §7 |
| **S4 config** | **the generated settings UI.** §3: *"There is no hand-written settings form in the codebase"* | M4, §6 |
| **S3 logging** | `rig logs` and its TUI views | M5, §8 |

⛔ **THE VISUAL HALF IS NOT A SECOND PROJECT AND IT IS NOT DEFERRED TO M10.**
§3's test is that the surface is generated from the declaration, so a service
whose data is declared correctly **gets its view for free** - and a service
that needs a hand-written view has got its declaration wrong. **That is the
check to run on each of the four, not a separate milestone.**

---

### Order, and the reason it is this order

| | Service | Why here |
|---|---|---|
| **1** | **S1 storage** | ⛔ **Everything else needs somewhere to put bytes.** The log service stores segments, config stores layers, backup archives a store. Doing it first means each of the others has one store to consume rather than a fourth hand-rolled one |
| **2** | **S2 backup** | **Immediately after S1 and before anything else writes into it.** A store gains a backup before it gains more writers, not after |
| **3** | **S4 config** | Smaller than logging, and its first consumer is three flags. **It is the cheapest proof that the dogfooding test works at all** |
| **4** | **S3 logging** | Largest, and it carries the redaction dependency. **Last because it is the only one whose failure is a leak rather than a gap** |

⛔ **S1 IS ALSO WHERE THE TWO-ENGINE QUESTION GETS SETTLED**, and that is a
ruling somebody owes before writing code, not during. §38's reuse rule binds:
name the candidates, and *"I did not find one"* is only an answer after a
search that can be described. **B28 already ran that search for the record
store and its answer was `modernc.org/sqlite` alone.**

---

### What this does not change

- **§43 stands in full.** This is its order, not a revision of it.
- **The tray acceptance test is untouched**, and it does not depend on any of
  these five.
- ⛔ **The MVP sentence is STILL OPEN.** §43's question 3 is not answered by
  this section, and `READINESS.txt` remains the only artefact allowed to say
  what any of it costs.
- **No planner code moves until S1 to S4 land**, which is his instruction read
  literally: *"complete missing services/capabilities in rig BEFORE we split
  out the planner."*
