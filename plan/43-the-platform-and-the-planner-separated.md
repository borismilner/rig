## 43. The platform and the planner, separated

✅ **Q1 AND Q4 ANSWERED BY BORIS 2026-09-24: THE STORE STAYS IN rig, THE
PLANNER IS `docket`.** Put to him as three questions with §43's own
recommendations first, on his *"We need it split ASAP"*; he took all three.
**The third reversed §44's order: the split comes FIRST, and S1, S4, S3 land
after it.** Q3, the MVP sentence, is the one row still his. §50 is the build.

⛔ Question 2 was first answered by §44, 2026-09-19 (services first) and
reversed by him on 2026-09-24 (split first); §44 carries both.

**RULED BY BORIS, 2026-09-19.** He raised it unprompted at the start of the
session, asked for it to be sized, and ruled before anything moved. The three
statements are quoted in the order he made them, verbatim from the terminal.

> *"I think I've deviated from my plan of making `rig` a shared platform for my
> in-house developed programs. I think I've added the project management
> feature into it when it should have been an external program that uses
> `rig`."*

> *"How much effort is to take the planning functionality that is supposed to
> replace logbook out of rig into a dedicated project and have it use `rig` for
> all the platform services it offers as planned to begin with?"*

> *"Before doing anything - I want you to persist the plan going-forward to
> refocus `rig` on being the platform services and to separate the planned into
> a standalong project that uses `rig` to the max."*

**The spelling is his and is kept.** Correcting a quotation is falsifying
evidence, and this project has already found two invented ones and one
misattributed requirement. *"the planned"* reads as *the planner*; that reading
is this seat's and it is one sentence for him to correct.

---

### ⛔ WHAT THIS OVERTURNS, AND IT IS HIS OWN RULING

**§39 rests on a ruling he made on 2026-09-12, and this reverses it.** B17 was
put to him as four options with the consequence of each. **One of the four was
an adopting program, and he refused it**, choosing rig carrying the mechanism.
He then widened it three times in the same exchange:

> *"I think rig should contain as much as possible since this is a single
> source of truth we can perfect, instead of perfecting general instructions
> across many different CLAUDE.md"*

**The argument recorded as deciding WHERE it lived** was *"rig sees every
session; an adopting program sees only its adopters."* `DECISIONS.md`
2026-09-12 carries the whole entry.

⛔ **THE 2026-09-12 RULING IS SUPERSEDED ON THE PLACEMENT QUESTION AND ON
NOTHING ELSE.** What it settled about the record itself - two co-equal
consumers, progress written during the work rather than composed afterwards,
nothing ever lost - survives intact and is still the specification. **What moves
is the address, not the design.**

---

### The line this restores was already drawn in §29, against a different surface

**§29 dropped Walkthroughs from the AgentBox port on exactly this reasoning**,
ruled 2026-09-11, and nobody ever turned it on §39:

> *"That is a product built on a platform, not platform, and it is the largest
> single surface dropped."*

**Assignments went out of the same door in the same ruling:** *"Starting an
agent session a human can open and take over is not rig's job."* **Project and
case management is on the same side of that line and was admitted anyway**,
which is the deviation he named.

---

### What rig IS, restated

| rig carries | the planner carries |
|---|---|
| configuration, storage, secrets, logging, tracing | the project and case model |
| the window, the tray, toasts, speech | briefs, backlogs, decisions, requirements |
| starting, stopping, invoking, scheduling | the markdown importers |
| the CLI, MCP and pane projections | whatever replaces the logbook |
| seats, presence, leases, the blackboard | its own views, served through rig |

**The test that decides a disputed row: does it name a program, a project, or a
plan?** rig may name none of the three. §29 non-goal 1 is untouched by this
section and is restored to its unnarrowed reading.

⛔ **THE HALF OF §39 THAT DOES NOT MOVE IS PROGRESS AND SESSION TELEMETRY.** His
2026-09-12 argument still holds for it and holds for nothing else: **a
progress stream that only adopters emit leaves holes exactly where a session is
in trouble.** rig sees every session; the planner never will. **So `progress`
stays a platform verb and the planner consumes it.** The brief that reads those
steps is the planner's.

---

### ⛔ WHAT "USES rig TO THE MAX" REQUIRES, AND MOST OF IT IS NOT BUILT

**This is the part that decides the order of the work.** Measured 2026-09-19
against the tree at `53c41d3`.

| Service the planner needs | Milestone | State today |
|---|---|---|
| registration and declaration | M1 | **built** |
| CLI projection, `rig <app> <cmd>` | M1 | **built** |
| MCP promotion, one tool per promoted command | M2 | **built** |
| invoke, `rig call` | M1 | **built** |
| a pane it serves itself | M1a | **built**, at the pre-kit tier |
| seats, presence, peers | M7 cherry-pick | partial |
| ⛔ **storage** | **M11** | ⛔ **NOT BUILT** |
| ⛔ **configuration** | **M4** | ⛔ **NOT BUILT** |
| logging and tracing | M5 | not built |
| secrets | M11 | not built |

⛔ **rig SERVES NO STORAGE TO PROGRAMS, AND THE PLANNER IS ALMOST ENTIRELY A
STORE.** `grep` over `proto/rig/v1/wire.proto` finds no storage verb, no config
verb and no secret verb; the only persistent surface on that wire is the record
itself, which is what is leaving.

**So "to the max" is a requirement on rig before it is a requirement on the
planner.** A planner extracted today would carry its own SQLite and adopt rig
for the projections only, which is the opposite of what he asked for.

---

### The seam, measured rather than estimated

**Run 2026-09-19 in a scratch copy, by splitting the package and reading the
compiler rather than the source.** The working tree was not touched.

| Measurement | Result |
|---|---|
| intra-repo packages `internal/record` imports | **one**, `internal/paths` |
| store half compiled alone | **0 errors** |
| symbols the planning half needs from the store | **6**, replaced by a 15-line alias file |
| symbols found on the wrong side | **3** - `stronglyConnected`, `KindProgress`, `LinkPartOf` |
| files found on the wrong side | **2** - `fence.go` and `friendly.go` are planning |

**Re-measured 2026-09-24 at `abb1ed0`, §50:** every number above holds, and one
reads differently under this section's own ruling that `progress` stays in rig -
two of the 14 methods are `Step` and `Stream` in `progress.go`, so **12 move and
2 stay**. §50 carries the file-by-file classification and the moves in order.

⛔ **CORRECTED 2026-09-24 BY THE BUILD OF §50 MOVES 1 AND 2 (the `seam`
seat, rig `2fedfaa`, `b50bf42`, `f0682cf`), by splitting the package for real
and reading the compiler:** the six symbols above are **seven** (`notesAbout`);
the alias file is **11 symbols in 20 lines**, not 6 in 15; `lateststeps.go` is
a **store** file (SQL over rig's schema), so **8 methods move, not 12**; and
`stronglyConnected` is needed on **both** sides. The 2026-09-19 numbers stand
as what that scratch split measured; these are what a real one found.

⛔ **THE ONE REAL COUPLING: 14 METHODS ON `*Store` ARE DEFINED IN PLANNING
FILES** - eight in `brief.go`, four in `lateststeps.go`, two in `progress.go` -
**and they reach the store's unexported internals** (`briefContainer`,
`latestSteps`, `itemsWithANote`, `blocksAmong`, `blockedBy`,
`coarseCitations`).

**That is the architectural finding stated in code:** the application grew into
the platform's private state. **The importers do not have it.** `backlog.go`,
`plan.go`, `decisions.go`, `sections.go`, `section.go`, `fence.go` and
`friendly.go` are 2,754 lines with zero references to `Store` and move
untouched.

**The volume, so nobody re-derives it:** about 38,000 lines including tests -
35% of the Go tree and 52% of the frontend. **The two cuts under consideration
cost nearly the same**, because tests and frontend dominate either way, so the
seam is chosen on architecture and never on size.

---

### ⛔ OPEN, AND A SEAT MAY NOT INVENT AN ANSWER TO ANY OF THESE

**He ruled the separation. He ruled none of the following**, and each changes
what gets built.

| Open | Why it is not answerable from what he said |
|---|---|
| ✅ **ANSWERED 2026-09-24: STAYS.** does the generic record store stay in rig as a platform service, or go with the planner? | Put to him as *Stays in rig* (rig keeps kinds, versions, links, the `record.*` verbs and `progress`; the planner declares ITS kinds and reads and writes over the wire) against *Goes with the planner*. He chose STAYS. §50 was drafted for it |
| ✅ **ANSWERED TWICE. does M11 come before the extraction?** | 2026-09-19: yes, §44. **2026-09-24: NO - split first**, the reversal named to him as a reversal of his own order; the planner leaves on rig's existing record store over the wire and adopts S1, S4, S3 as each lands |
| ⛔ **what happens to the MVP definition?** | §39's MVP is *"being able to use `rig` to work on `rig` with respect to the project/case management"*. **That sentence now describes the planner, not rig.** `READINESS.txt` is the team-lead's and is the only artefact allowed to answer it |
| ✅ **ANSWERED 2026-09-24: `docket`.** the planner's name, and its repository | `~/me/projects/docket`, module `github.com/borismilner/docket`, notes at `logbook/projects/docket/` by the standing convention. `cmd/docket`, M1a's second fake application, is renamed to free the name. `slate` was the alternative offered |

#### Seat recommendations, 2026-09-23 evening. NOT ANSWERS: §45's rule is that his question is parked WITH a recommendation and with what proceeds meanwhile

**He said to do as much as possible without him (§45, 2026-09-23). The three
rows are still his**, and each is one word from him. A recommendation here is
one seat's reading of the evidence named beside it, nothing more.

| Row | Recommendation | Evidence | Proceeds meanwhile |
|---|---|---|---|
| **Q1 - the record store** | **STAYS in rig, as a platform layer over S1.** The generic mechanism - kinds, versions, links, compare-and-swap, provenance, the `record.*` verbs - is rig's; **the planner declares ITS kinds** (project, case, work item, decision) and takes its importers, `brief.go`, `lateststeps.go`, the views | `progress` is already ruled a platform verb above, and a progress record has to land somewhere rig owns. §39's *two co-equal consumers* was the design from 2026-09-12 and is untouched by the placement ruling. **The 14 coupled methods in the seam table are exactly what B108 is pricing**: if `project.brief` composes over the wire at an acceptable cost, STAYS is buildable; if not, §44 already owes a batched projection verb that pushes the joins down, and the answer is still STAYS | B108 runs; S1's specification (the next section written) is drafted for STAYS and says so in its first line, so GOES costs one section rewrite and no code |
| **Q3 - the MVP sentence** | **His words unchanged, re-addressed:** *use the planner, running on rig, to work on rig with respect to project/case management*, and the tray test unchanged. **`READINESS.txt` carries the draft**, being the only artefact allowed to answer it | §39's sentence names the OUTCOME he wants, and the outcome did not move; only the program that delivers it did. The tray test is a deployment test and no seam touches it (above) | nothing waits on it; it changes what `READINESS.txt` counts, not what is built |
| **Q4 - the name and the repository** | **`docket`**: *a list of cases to be heard*, which is what the program is. One repository at `~/me/projects/docket`, module `github.com/borismilner/docket`, notes at `logbook/projects/docket/` by the standing convention until it retires the logbook (§16). **Collision:** `cmd/docket` is M1a's second FAKE application and would be renamed; **`slate`** is the alternative that collides with nothing | The in-house names are one plain word each (`shelf`, `graft`, `archi`, `nudge`). The extraction specification cannot be written without a module path, so this is the first of the three the build actually blocks on | the extraction specification is drafted with `<planner>` as a placeholder and is one substitution from final |

---

### What this does NOT change

- ⛔ **The tray acceptance test stands, unmoved.** *"I'll know we reached MVP
  when I'll see the production icon on my system-tray both during this session
  and after I reboot the machine so I know it is properly deployed."* It is a
  deployment test and no seam touches it.
- **§39's design survives** - the nouns, the provenance, the grain, the
  retention exemption, the two co-equal consumers. It is being re-addressed,
  not re-decided.
- **§38's four standing rules are untouched**, and the swiss-knife rule now
  reads on rig with more force rather than less: best at what it carries, and
  it does not grow new capabilities to be broader.
- **The logbook replacement is still the goal.** It becomes the planner's job
  rather than rig's, and the testable definition in §16 is unchanged: when it
  lands, the convention document, the setup script, the symlinks and the
  two-repo commit rule all disappear.
