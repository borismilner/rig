## 50. The planner extraction skeleton

**Written 2026-09-24 without him, on his 2026-09-23 instruction** (*"Find the
plan to separate the planner from `rig` and follow it. I want you to do as
much as possible on your own without bothering me!"*). It is the §45 step-4
document for B99 in SKELETON form: what leaves rig, what stays, the seam
re-measured, the moves in order, and the acceptance test stated before any
of it starts. ⛔ **It is not yet a specification a subagent can build from,
and the reason is stated rather than hidden:** three of its rows wait on his
word (Q1, Q4 and the order gate below), and every other row is the seat's
call, marked as such. A row he reshapes is edited here, not argued in a
handoff.

> **In one line: the project and case model, its importers, its brief and its
> views move to a program called `<planner>` that talks to rig over the socket
> like every other in-house program; the record store, the `record.*` verbs
> and `progress.step` stay in rig as platform.** `<planner>` is a placeholder
> until Q4 is answered, and this section is one substitution from naming it.

---

### ⛔ WHAT WAITS ON HIM, AND WHAT THIS SECTION ASSUMES MEANWHILE

| Waits on | Assumed here | If he rules otherwise |
|---|---|---|
| **§43 Q1** - the record store stays in rig or goes with the planner | **STAYS**, §43's recommendation: the generic mechanism is rig's, the planner declares its kinds | moves 2, 4 and 7 below are rewritten and acceptance A changes shape; the importers and the views move either way |
| **§43 Q4** - the planner's name and repository | `<planner>`, one plain word; `~/me/projects/<planner>`, module `github.com/boris-milner/<planner>` | one substitution across this section. §43 recommends `docket`, else `slate` |
| **§43 Q3** - the MVP sentence | untouched here; `READINESS.txt` is the only artefact allowed to answer it | nothing in this section changes |
| **§44's order gate** - S1, S2, S4 and S3 land first | **no planner code moves before B103, B104, B105 and B106 are struck.** §44: *"no planner code moves until S1 to S4 land"* | he can reorder in one word; the moves below do not change, only when they start |

**The moves below are written so that Q1 STAYS is the default and GOES is a
named delta**, because §48 (S1) is already drafted for STAYS and says so in
its first line. Two drafts for two answers would be the breadth §45 forbids.

---

### The seam, re-measured 2026-09-24 at `abb1ed0`

**§43 measured it on 2026-09-19 at `53c41d3` by splitting the package in a
scratch copy. This is the same seam read from the tree today**, so the numbers
a future seat starts from are current rather than inherited.

| Measurement | 2026-09-19 | 2026-09-24 |
|---|---|---|
| intra-repo packages `internal/record` imports | one, `internal/paths` | **one, `internal/paths`** |
| `*Store` methods defined in planning files | 14 (8 + 4 + 2) | **14 (8 + 4 + 2), and two of them do not move - below** |
| importer files with zero `Store` references | 7, 2,754 lines | **7, 2,668 lines** |
| symbols on the wrong side (B102) | 3 and 2 files | unchanged; re-proved by the compiler at move 1 |
| files in the package that did not exist | - | `snapshot.go`, S2's, store side |

⛔ **CORRECTION TO §43's SEAM TABLE, AND IT IS §43's OWN RULING APPLIED TO ITS
OWN COUNT.** The two `*Store` methods in `progress.go` are `Step` and `Stream`,
the write and read path of `progress.step`. **§43 rules that `progress` stays a
platform verb.** So of the 14 methods in planning files, **12 move** (eight in
`brief.go`, four in `lateststeps.go`) and **2 stay**; `progress.go` is a store
file carrying two planning constants, which is B102's finding and not a third
category.

**Every non-test file in `internal/record`, classified.** The classification is
the seat's; the line counts are measured.

| Side | Files | Lines | `*Store` methods |
|---|---|---|---|
| **STORE - stays in rig** | `store.go`, `records.go`, `control.go`, `links.go`, `refs.go`, `snapshot.go`, `progress.go` | **2,658** | 26 |
| **IMPORTERS - move untouched** | `backlog.go`, `plan.go`, `decisions.go`, `sections.go`, `section.go`, `fence.go`, `friendly.go` | **2,668** | 0 |
| **COUPLED - move after B100** | `brief.go`, `lateststeps.go` | **1,646** | 12, reaching the store's unexported internals |

**The planner-shaped code OUTSIDE the package**, which §43 sized at about
38,000 lines including tests and this table names file by file:

| Where | What | Lines | Goes or stays |
|---|---|---|---|
| `cmd/rigseed/` | the seeder: drives `rig record put` over the wire through the `rig` binary, and imports `internal/record` **only for the parsers and the kind constants** | 2,685 (4,944 with tests) | **goes, whole.** It is already a program talking to rig over the socket; only its import moves with the importers |
| `cmd/rig/brief.go` | the `brief` verb and its rendering | 2,116 | **goes**, becomes `<planner> brief`, reachable as `rig <planner> brief` through M1's CLI projection |
| `cmd/rig/record.go` | the ten `record.*` verbs and `progress` | 2,881 | **stays** under Q1 STAYS |
| `internal/daemon/record.go` | serves the `record.*` verbs, `progress.step` and `project.brief` | 1,118 | **splits**: `serveProjectBrief` (line 786 onward) goes; the rest stays |
| `internal/daemon/mcp_records.go` | the record tools on the MCP door and `project_brief` | 432 | **splits** the same way |
| `proto/rig/v1/wire.proto` | 30 of 73 messages are record-shaped; **7 are the brief's** (`ProjectBriefRequest`, `ProjectBriefResponse`, `BriefSectionStatus`, `BriefNote`, `BriefHealth`, `GoverningRecord`, `ClosedItem`) | - | the brief's seven leave rig's wire - **see the §21 row below** |
| `frontend/src/lib/` | `Records.svelte`, `PlanVsExec.svelte`, `ProjectCaseGui.svelte`, `ItemRow.svelte`, `brief.ts`, `records.ts`, `itemtype.ts` and two test files | **3,939 of 8,104 (49%)** | **go**, as the planner's pane. `Dashboard.svelte`, `Sections.svelte` and `Waffle.svelte` import them today and are the frontend's own seam |

---

### The design, decided as far as it can be without him

⛔ **EVERY ROW IS THE SEAT'S CALL UNLESS IT CITES A RULING.**

| # | Decision | Why | What it rules out |
|---|---|---|---|
| **1** | **The planner imports nothing of rig.** `go list -deps ./...` in its module names no `github.com/boris-milner/rig` package | the front matter of this specification: *"No in-house program imports rig. They talk to it over a socket."* §5 | a shared Go module; a `replace` directive; copying `internal/wire` |
| **2** | **The 12 coupled methods become `record.query` calls with §48's `fields` projection**, assembled in the planner at B108's measured cost | B108: serialising the answer is 3.555 ms, 69x the socket, so projection beats batching; §44's *"the query API must push the work down"*. **The interface B100 gives `Store` is a stepping stone that proves the seam compiles, not the end state** | a `project.brief` served by rig on the planner's behalf; a brief that pays N+1 round trips |
| **3** | **`project.brief` leaves `self.go` and the brief's seven messages leave `wire.proto`** | §43: rig may name no program, project or plan, and a brief names all three | keeping a planner verb on rig's wire "for compatibility" |
| **4** | ⛔ **`project.brief` on wire v1 is §21-frozen and cannot simply vanish:** *"rig serves every wire version it has ever shipped."* **Recommendation: v1 keeps the verb name and answers it by invoking the planner's registered `brief` command (M1's invoke), and the message set is dropped at the next wire major** | §21 is a ruling and this section does not overturn it; forwarding costs one invoke and keeps every frozen conformance fixture green | deleting the arm from v1; a v2 opened for this alone |
| **5** | **The planner declares its kinds** - project, case, work item, requirement, decision, note, section - **at registration, and rig's store validates a record against the declaring program's kinds rather than against constants of its own** | §43 Q1 STAYS: *"the planner declares ITS kinds"*; §39's nouns survive as a declaration, which is §3's Expandable maximal applied to the store | rig's `records.go` carrying `KindWorkItem`; a store that knows what a backlog is |
| **6** | **History stays in rig.** The planner's first commit names the rig sha it was cut from, and moved files are copied, not rewritten | rig's history is the evidence this project keeps citing; a filter-repo would make every `PLAN.md section N` citation in the logbook resolve against a rewritten tree | `git filter-repo`; a subtree split that leaves two histories of one file |
| **7** | **The planner's views are served through rig's pane tier** (M1a, built at the pre-kit tier) **and rig's own window keeps only platform data**: peers, presence, deployment, the estate, settings | §43's table: *"its own views, served through rig"*; §3: *"zero frontend code and no rig-specific logic"* | `Records.svelte` staying in `cmd/rigwindow` because it is already there |
| **8** | **`rigseed` moves whole and keeps its name until the planner retires the logbook** (§43: the logbook replacement is now the planner's job) | it already drives rig over the wire; §16's testable definition of "the logbook is replaced" is unchanged and is the planner's to meet | a seeder in rig for documents rig may not name |
| **9** | **The extraction is one subagent's, in named slices, one commit each, after S1 to S4** | §45's loop; `COORDINATION.md`'s one-build-at-a-time rule | the lead doing it by hand; two seats splitting one seam |

---

### The moves, in order, and what proves each one

**Each move is a slice a subagent lands with one commit, and each has a check
the compiler or `git grep` runs rather than a reader.** Moves 1 and 2 are
B102 and B100, which `BACKLOG.md` already argues are worth doing whichever way
Q1 falls; they may run before the gate opens because they move nothing out of
rig.

| # | Move | Proof |
|---|---|---|
| **1** | **B102: three symbols and two files to the right side** inside `internal/record`. `stronglyConnected` to `refs.go`; `KindProgress`, `LinkPartOf` to `records.go`; `fence.go`, `friendly.go` marked as importers | `go build ./...` and `go test ./internal/record/` unchanged; `git diff --stat` shows motion only |
| **2** | **B100: an exported interface for what the brief needs**, the 12 methods rewritten against it | the planning files compile with `Store` replaced by the interface in a scratch split; the six unexported names §43 lists no longer appear in `brief.go` or `lateststeps.go` |
| **3** | **The repository:** `~/me/projects/<planner>`, `go.mod`, the seven importers and `cmd/rigseed` copied in, decision 6's cut-sha in the first commit | `go list -deps` names no rig package (acceptance A); `rigseed --check` exits 0 from the new repo against the same estate |
| **4** | **The brief over the wire:** `brief.go` and `lateststeps.go` rewritten as `record.query` calls with `fields` (decision 2); registered as `<planner> brief` | acceptance C, the diff of two briefs |
| **5** | **`cmd/rig/brief.go` leaves**; `rig <planner> brief` answers through M1's projection; `project.brief` becomes decision 4's forward | `cmd/rig/refusal_verbs_test.go` green; `rig brief` prints the refusal that names the new command |
| **6** | **Kinds as a declaration** (decision 5); `records.go` loses its planner constants | `git grep -n 'KindWorkItem\|KindRequirement\|KindDecision\|KindNote'` in rig returns nothing outside `plan/` |
| **7** | **The pane:** the nine frontend files move; `Dashboard`, `Sections` and `Waffle` lose their imports | `make test-window` 0, `make contrast-window` clean, and the window shows platform data only |
| **8** | **The daemon and the door:** `serveProjectBrief` and `project_brief` leave; decision 4's forward arm replaces them | `internal/daemon` tests green; the MCP door lists the planner's promoted tool and not rig's |
| **9** | **`bench-size` re-recorded with the shrink attributed** - gen 22 attributed `rigd`'s growth to the store arriving; the brief's leaving is the first row that goes down | `make bench-size` rows for `rig`, `rigd` and `rigwindow` re-recorded under `rig-makefile` in the same commit |

---

### Acceptance, restated so it can be checked rather than read

**§44's form: a thing is DONE when rig's own version of it is DELETED, proven
by `git grep`, and §45's: the test is stated before the seat starts.**

| | Test | Command |
|---|---|---|
| **A** | **the planner imports nothing of rig** | `go list -deps ./... \| grep -c github.com/boris-milner/rig` prints **0** in the planner's module |
| **B** | **rig names no program, project or plan** (§43's test for a disputed row) | `git grep -l -E 'BacklogParse\|PlanParse\|DecisionParse\|serveProjectBrief\|ProjectBriefRequest\|KindWorkItem' -- ':!plan/' ':!*.md'` prints nothing in rig |
| **C** | ⛔ **THE DEMONSTRATION: the same store, two briefs, no difference.** `rig backup` on production (S2); `rig restore --estate demo` under a private `XDG_STATE_HOME`; the OLD `rig brief` from a binary built at the pre-extraction sha and the NEW `rig <planner> brief` both run against `demo` | `diff <(old) <(new)` is **empty**. The restore is the fixture, so production is never touched, and the check is a byte comparison rather than a reading |
| **D** | **the seeder still agrees with the documents from its new home** | `rigseed --check` exits **0** from the planner's repo against `demo` |
| **E** | **both repositories gate green** | `make ci` 0 and `make lint` 0 in rig; the planner's equivalents 0 |
| **F** | **the tray test is untouched** | the production icon, during the session and after a reboot - §39, unchanged |

⛔ **C IS THE TEST, AND IT IS WHY S2 COMES BEFORE THE SPLIT.** §44 puts backup
second so that a store gains a backup before it gains more writers; this
section adds the second reason: **the extraction's acceptance test is a restore
of production onto a throwaway estate**, and it cannot be run until `rig
restore` exists. B104 is that command.

---

### Files a future seat owns, in skeleton form

**The binding table is `COORDINATION.md`'s and it is written before the first
write, not here.** This is the shape it will take, so the rows can be checked
for completeness against the tables above when the time comes.

| Path | Fate | Note |
|---|---|---|
| `internal/record/{backlog,plan,decisions,sections,section,fence,friendly}.go` + tests | **move** | untouched |
| `internal/record/{brief,lateststeps}.go` + tests | **move, rewritten** | decision 2 |
| `internal/record/{store,records,control,links,refs,snapshot,progress}.go` + tests | **stay** | decision 5 edits `records.go` only |
| `cmd/rigseed/` | **move, whole** | decision 8 |
| `cmd/rig/brief.go` + tests, `testdata/exec/brief-*.golden` | **move** | move 5 |
| `internal/daemon/record.go`, `mcp_records.go` | **split** | one arm each leaves; the seat's grant is those arms and nothing else in the lead's files |
| `internal/daemon/self.go` | **one block** | `project.brief` becomes decision 4's forward |
| `proto/rig/v1/wire.proto`, `wire.pb.go` | **seven messages** | at the next wire major only, §21 |
| `frontend/src/lib/` nine files; `Dashboard`, `Sections`, `Waffle` | **move; edit** | move 7 |
| `~/me/projects/<planner>/` | **NEW** | the seat's, whole |
| `logbook/projects/<planner>/` | **NEW** | by the standing convention until the planner retires it |

---

### What this section does not change

- **§43, §44 and §45 stand in full.** This is §43's build shape, in §44's
  order, under §45's gate. **Nothing moves before S1 to S4 land and before he
  answers Q1 and Q4**; a seat that starts move 3 without both has taken back
  the decisions §45 exists to give him. Moves 1 and 2 are the exception, and
  only because they move nothing out of rig.
- **§39's design survives** as the planner's declaration: the nouns, the
  provenance, the grain, the two co-equal consumers, the retention exemption.
  `progress` stays rig's, as §43 rules.
- **The tray acceptance test is untouched.** It is a deployment test and no
  move here reaches it.
- **§21 is not overturned.** Decision 4 keeps every shipped wire version
  served; the brief's messages leave at a major, not before.
- **Deferral is never deletion**, §43. The eight services §44 orders after
  the split are still rig's to build.
- **The capability after B104 is his to choose.** This section prepares; it
  approves nothing and spawns nothing.
