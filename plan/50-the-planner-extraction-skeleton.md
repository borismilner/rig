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

### ⛔ RULED BY BORIS, 2026-09-24: THE SPLIT IS THE NEXT CAPABILITY, AND IT IS ASAP

**Said in reply to the B104 certification report, which ended by asking
whether to start B114 or hold for the nine batched rows:**

> *"We need it split ASAP; Do all needed to make it happen ASAP!"*

**What that settles, and what the seat did with it the same turn:**

| | |
|---|---|
| **§45's "next capability"** | **CHOSEN: the extraction itself (B99), ahead of S1, S4 and S3.** B114 and B115 yield to it; B114 stays owed before any restore in anger, and acceptance C below is a restore |
| **§44's order gate** | ✅ **ANSWERED THE SAME TURN: SPLIT FIRST.** Put to him as a reversal of his own 2026-09-19 order, with Q1 (STAYS) and Q4 (`docket`) beside it; he took all three recommendations. S1, S4 and S3 land after the split and the planner adopts each as it arrives |
| **"If possible, work in parallel!"** | his next message, while the `seam` seat was spawning. **Several seats at once on disjoint files**, §45's last open row answered; the tracks are the schedule table below |
| **what runs without waiting** | **moves 1 and 2 (B102, B100) as the `seam` seat; the `cmd/docket` rename as the `rename` seat; move 3, the repository, as the `docket` seat** - all spawned 2026-09-24. Moves 4 to 9 are specified below and start as their inputs land |
| **what does not change** | the two commands stay his; nothing is pushed; §45's loop still hands the build to a subagent from a written specification |

**RULED AGAIN 2026-09-24 07:30, on the generation-27 brief that offered the
B116 shape as a choice:** *"Apply all and be as pro-active as possible, we
don't have this account for much longer so I need you to do as much as
possible with the time left."* **What it settles:** the lead's picks in this
section (B116 by paging, decision 4 narrowed, four tracks at once, F no
longer waiting on E) proceed without a further ask; each is recorded here
and in `DECISIONS.md` so he can reverse any of them in one word.

> **In one line: the project and case model, its importers, its brief and its
> views move to a program called `docket` that talks to rig over the socket
> like every other in-house program; the record store, the `record.*` verbs
> and `progress.step` stay in rig as platform.** He named it `docket` on
> 2026-09-24; `cmd/docket`, M1a's second fake application, is renamed to
> free the name.

---

### ⛔ WHAT WAITS ON HIM, AND WHAT THIS SECTION ASSUMES MEANWHILE

| Waits on | Assumed here | If he rules otherwise |
|---|---|---|
| **§43 Q1** - the record store stays in rig or goes with the planner | ✅ **ANSWERED 2026-09-24: STAYS.** The generic mechanism is rig's, the planner declares its kinds | moves 2, 4 and 7 below are rewritten and acceptance A changes shape; the importers and the views move either way |
| **§43 Q4** - the planner's name and repository | ✅ **ANSWERED 2026-09-24: `docket`**; `~/me/projects/docket`, module `github.com/borismilner/docket` | one substitution across this section. §43 recommends `docket`, else `slate` |
| **§43 Q3** - the MVP sentence | untouched here; `READINESS.txt` is the only artefact allowed to answer it | nothing in this section changes |
| **§44's order gate** - S1, S2, S4 and S3 land first | ✅ **REVERSED BY HIM 2026-09-24: split first.** S2 had landed anyway (B104). §44 as written said *"no planner code moves until S1 to S4 land"* and carries the reversal at its top | he can reorder in one word; the moves below do not change, only when they start |

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
platform verb.** So of the 14 methods in planning files, ~~12 move~~ **8 move
(`brief.go`) and 6 stay** - ⛔ **CORRECTED 2026-09-24 BY THE SEAM SEAT'S
SCRATCH SPLIT:** every read in `lateststeps.go` is SQL over rig's schema, so it
is a STORE file the planner cannot carry; its four methods stay beside
`progress.go`'s two. Three more corrections from the same proof: §43's six
unexported names are SEVEN (`notesAbout` was a fourth raw `s.db` read in
`brief.go`); the alias file the planning half needs is **11 symbols, 20 lines**,
not 6 and 15 (brief sections 10-12 each reach a kind constant); and
`stronglyConnected` is needed on BOTH sides, so docket duplicates it (about 70
lines, standard library) at move 4. Full record: the seat's FINDINGS,
`agent-work/build-plan-50-moves-1-and-2/`.

**Every non-test file in `internal/record`, classified.** The classification is
the seat's; the line counts are measured.

| Side | Files | Lines | `*Store` methods |
|---|---|---|---|
| **STORE - stays in rig** | `store.go`, `records.go`, `control.go`, `links.go`, `refs.go`, `snapshot.go`, `progress.go` | **2,658** | 26 |
| **IMPORTERS - move untouched** | `backlog.go`, `plan.go`, `decisions.go`, `sections.go`, `section.go`, `fence.go`, `friendly.go` | **2,668** | 0 |
| **COUPLED - `brief.go` moves after B100, rewritten over the wire; `lateststeps.go` STAYS** (corrected above) | `brief.go` moves; `lateststeps.go` stays | **1,646** together | 8 move, 4 stay; at `f0682cf` none of them reaches an unexported store name |

**The planner-shaped code OUTSIDE the package**, which §43 sized at about
38,000 lines including tests and this table names file by file:

| Where | What | Lines | Goes or stays |
|---|---|---|---|
| `cmd/rigseed/` | the seeder: drives `rig record put` over the wire through the `rig` binary, and imports `internal/record` **only for the parsers and the kind constants** | 2,685 (4,944 with tests) | **goes, whole.** It is already a program talking to rig over the socket; only its import moves with the importers |
| `cmd/rig/brief.go` | the `brief` verb and its rendering | 2,116 | **goes**, becomes `docket brief`, reachable as `rig docket brief` through M1's CLI projection |
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
| **1** | **The planner imports nothing of rig BUT THE STUB.** `github.com/borismilner/rig/client` is *"the one piece of rig code that lives inside a program"* (§5d), placed outside `internal/` precisely so that a program in another module can reach rig; `go list -deps ./...` in docket's module names that package, its own three dependencies (`internal/paths`, `internal/wire`, `proto/rig/v1`, measured 2026-09-24) and no other rig package. ⛔ **CORRECTED 2026-09-24 by the lead: the skeleton said "nothing of rig" and had not read §5d.** `cmd/fakeapp` is the working example of a program on the stub | §5: *"No in-house program imports rig. They talk to it over a socket"* - and the stub IS the socket, budgeted and frozen by §3 and §5d so that importing it is not importing rig's semantics | a second client; copying `internal/wire`; importing `internal/record` or any store package. **A `replace github.com/borismilner/rig => ../rig` is tolerated ONLY until rig is pushed** (his command), named in `go.mod` with the reason, and replaced by a `require` at a pseudo-version the moment it is; acceptance E checks for it |
| **2** | **The 12 coupled methods become `record.query` and `record.refs` calls through the stub, assembled in docket - FIRST on the verbs rig has today, correctness proven by acceptance C; THEN §48's `fields` projection if the measured brief exceeds the bound below.** ⛔ **Sequenced this way 2026-09-24 for ASAP:** B108 priced the naive shape at 231 round trips, 5.6 ms at rigd's measured 24.25 us plus ~1.9 MiB of rows, against a 27.4 ms in-process derivation - so a brief on today's verbs lands well inside a CLI's budget and needs no rig change. **The bound: `docket brief` end to end at more than 2x `rig brief` measured the same way at the cut-sha (B108: 36.5 ms median), OR any single brief over 100 ms, opens the projection slice**, a `fields` list on `RecordQueryRequest`, which is rig's to build (the store stays) and S1's §48 already specifies | B108's verdict: serialising the answer is 3.555 ms, 69x the socket, so projection beats batching and shared memory is not justified; §44's *"the query API must push the work down"*. **The interface B100 gives `Store` proves the seam compiles; it is not the end state** | a `project.brief` served by rig on the planner's behalf; a brief that pays N+1 round trips *without a measurement saying so*; a transport picked before the measurement (§4) ⛔ **BOUND CROSSED 2026-09-24 (docket `2d83492`): 2.67x, 18 ms worst.** The projection slice is open and not built. The seat's measurement points at plan/48's deferred multi-id `record.refs`, whose stated trigger was "built when the brief moves". The brief has moved, so that trigger has fired. The `fields` projection is the second half |
| **3** | **`project.brief` leaves `self.go` and the brief's seven messages leave `wire.proto`** | §43: rig may name no program, project or plan, and a brief names all three | keeping a planner verb on rig's wire "for compatibility" |
| **4** | ⛔ **`project.brief` on wire v1 is §21-frozen and cannot simply vanish:** *"rig serves every wire version it has ever shipped."* **Recommendation: v1 keeps the verb name and answers it by invoking the planner's registered `brief` command (M1's invoke), and the message set is dropped at the next wire major** ⛔ **NARROWED 2026-09-24 07:40 by the lead, for ASAP: the v1 arm stays DECLARED and answers a structured refusal (`FAILED_PRECONDITION`) whose message names `rig docket brief`; nothing is forwarded.** Measured: `ProjectBriefResponse` is structured (`open` and `next_up` as `ItemState` lists) and `rig brief --json` emits its own `briefJSON` shape rather than protojson, so a forward would need a mapping layer built for a deprecated arm - §45's breadth. The seven messages stay in `wire.proto` until the next major; the `retire` seat re-records any conformance fixture that asserted a full brief to the refusal, and that re-recording is the evidence the arm still answers | §21 is a ruling and this section does not overturn it; a refusal is an answer on every shipped version, and it costs no mapping layer | deleting the arm from v1; a v2 opened for this alone; a forward built before a client asks for one ⛔ **AMENDED 2026-09-24 09:00 by the lead: the code is `CODE_DENIED`, not `FAILED_PRECONDITION`, because wire v1 has no such code.** `NOT_FOUND` was rejected: `daemon.go` answers it for an unknown method, so a moved verb would read as one that never existed. The refusal carries §9's precondition/actual/fix shape; its fix names `rig docket brief` in prose with `FixCommand` empty, because `refusal_verbs_test.go` cannot yet tell a program's command from a rig verb (landed `7647ad2`) |
| **5** | **The planner declares its kinds in its own code, and rig's store stays kind-agnostic - which it already is.** Measured 2026-09-24: `Put` refuses only an EMPTY kind or project (`store.go:434`), `records.go:157` says an unknown kind is invisible by construction, and `Declaration` on the wire carries no kinds. So the planner's kinds - project, case, work item, requirement, decision, note, section - live in a `kinds` package in docket (move 3 creates it), and rig's `records.go` loses its planner constants at move 6. **A rig-side "validate against the declaring program's kinds" mechanism is S1's (§48) and is NOT built for the split** - narrowed by the lead for ASAP; nothing rig does today depends on it | §43 Q1 STAYS: *"the planner declares ITS kinds"*; §39's nouns survive as the planner's declaration. The narrowing keeps move 6 to a deletion proven by `git grep` | rig's `records.go` carrying `KindWorkItem`; a store that knows what a backlog is; a new wire field built before a program needs it |
| **6** | **History stays in rig.** The planner's first commit names the rig sha it was cut from, and moved files are copied, not rewritten | rig's history is the evidence this project keeps citing; a filter-repo would make every `PLAN.md section N` citation in the logbook resolve against a rewritten tree | `git filter-repo`; a subtree split that leaves two histories of one file |
| **7** | **The planner's views are served through rig's pane tier** (M1a, built at the pre-kit tier) **and rig's own window keeps only platform data**: peers, presence, deployment, the estate, settings | §43's table: *"its own views, served through rig"*; §3: *"zero frontend code and no rig-specific logic"* | `Records.svelte` staying in `cmd/rigwindow` because it is already there |
| **8** | **`rigseed` moves whole and keeps its name until the planner retires the logbook** (§43: the logbook replacement is now the planner's job) | it already drives rig over the wire; §16's testable definition of "the logbook is replaced" is unchanged and is the planner's to meet | a seeder in rig for documents rig may not name |
| **9** | **The extraction is several seats' at once, each on files no other seat owns, one commit per slice** - his 2026-09-24 *"If possible, work in parallel!"*; the schedule is the table below | §45's loop, its parallel row answered; `COORDINATION.md`'s ownership rows are the mechanism that makes parallel safe on one tree, and a seat in the `docket` tree touches rig not at all | the lead doing it by hand; two seats in one file; a seat starting a move whose input has not landed |

---

### The parallel schedule, 2026-09-24

**Tracks run at once when their files are disjoint; a track starts when the
row it depends on has landed, never before.** The lead owns this table and
`COORDINATION.md`'s rows say the same thing per file.

| Track | Seat | Tree and files | Moves | Depends on |
|---|---|---|---|---|
| **A** | `seam` | rig, `internal/record/` | 1, 2 | **DONE 2026-09-24 02:10, rig `2fedfaa`, `b50bf42`, `f0682cf`**; `make ci` 0; four corrections to this section below came out of its scratch split |
| **B** | `rename` | rig, `cmd/docket/` to `cmd/abacus/`, `Makefile` size rows, the size baseline | frees the name | **DONE 2026-09-24 02:03, rig `83a1695` and `2ebe4a5`**; `abacus` chosen because every plainer noun was live rig vocabulary (its FINDINGS). The fictional program id `docket` in five `internal/` tests became `satchel` at `9f40679`, the lead's |
| **C** | `docket` | `~/me/projects/docket`, NEW tree | 3 | **DONE 2026-09-24 02:14, docket `e7ba4a8` (six commits), cut-sha rig `2fedfaa`**; proofs A, B, D taken, D on a reduced plan set because of the frame-cap defect below |
| **D** | lead | `plan/50` | the specification of 4 to 8 perfected: the client mechanism, the kinds declaration, the forward arm | reading, no code |
| **E** | `wire-brief` | `~/me/projects/docket`, whole, except `frontend/` and `cmd/docket/pane.go` (G's) | 4 | C. **SPAWNED 2026-09-24 07:45.** Decision 1, the stub; its query helper loops on `next` (B116) from its first commit ✅ **DONE 2026-09-24 at docket `2d83492`.** Acceptance C HOLDS: text and `--json` diffs EMPTY for a project, a case, an empty project, the B76 refusal, and a real TTY at five widths. Only stderr's program prefix differs (`rig:` becomes `docket:`). Fixture: SYNTHETIC, 38 records built by the old binary on a private estate, because the harness denied the seat reading production. Agreement over production's own data is NOT proven. Medians of 50: old 6 ms, new 16 ms. **2.67x crosses decision 2's 2x bound, so the projection slice is OPEN**; the worst run was 18 ms, well under the 100 ms bound. A brief costs 8 calls plus one `record.refs` per head, so 46 calls at 38 heads |
| **F** | `retire` | rig, `cmd/rig/brief.go` + goldens + entries; `internal/record`'s importers, `brief.go` and the tests that reach only them; `cmd/rigseed/` deleted; `mcp_records.go`'s arm; `self.go`; `serveProjectBrief` in `internal/daemon/record.go` LAST, after track I's commit to that file | 5, 6, 8 | A. **SPAWNED 07:45, and E is NOT a dependency** - acceptance C's old brief is built from the cut-sha whatever the tree holds, so deleting rig's brief cannot block C; only E can. `record.go` is sequenced behind track I by commit rather than split by file ✅ **DONE 2026-09-24 at `7647ad2`**: `f928ea6` move 5, `7316206` move 8a, `be7ea24` move 6 (10,776 deletions), `7647ad2` move 8b. Acceptance B prints nothing. `rig` -102,400 B, `rigd` -12,288 B. **Found:** `internal/record/brief.go` (1,281 lines) and `sections.go` have no caller left and no track retired them; the same seat was resumed at 09:00 to delete them. `mvp-demo` left the Makefile at `9680ca5` |
| **G** | `pane` | rig `frontend/`, `cmd/rigwindow/`; `docket/frontend/`, `docket/cmd/docket/pane.go`; ONE line of `docket/cmd/docket/main.go` after E is COMPLETE | 7 | C. **SPAWNED 07:45 for slice 1** (the nine files and the toolchain move; rig's window keeps platform data). Slice 2 (the embed, `/pane`, `pane_url`) waits on E's STATUS saying COMPLETE. `cmd/abacus` is the shape ✅ **DONE 2026-09-24**: slice 1 rig `062218f`, `2b01af2`, docket `4c468e7`; slice 2 docket `fc34b3b`, `33928e3`. `GET /pane` 200 against a private rigd, three JSON endpoints, traversal refused, contrast clean in both themes. The pane derives from `internal/brief.Derive`, because the terminal's `Brief` carries 5 of the card's 13 fields. **Not seen: rig's window drawing the pane in its frame**; that is Boris's screen |
| **H** | lead | `Makefile` under `rig-makefile`; the ratchet rows the shrink moves | 9 | everything above ✅ **DONE 2026-09-24**: `mvp-demo` left the Makefile (`9680ca5`), the dead `?gui=` contrast pages left (`5deca7c`), the `rig` and `rigd` rows re-recorded (`aaed1de`: -102,400 and -114,688 B) |
| **I** | `paging` | rig, `proto/rig/v1/wire.proto` + `wire.pb.go` for three fields (lent), `internal/record/store.go` for one paged read, `serveRecordQuery` in `internal/daemon/record.go`, `cmd/rig/record.go`'s query verb, its own test files | **B116** | nothing. **SPAWNED 07:45.** Specified below, "B116, the answer is paged" ✅ **DONE 2026-09-24 08:30 at rig `0eab674`** (six commits from `b15749c`): the requirement query was 1,116,733 B in one frame, 68,157 over the cap; now 5 pages, the largest 262,050 B. `decision` (801,308 B) was next to die, now 4 pages. The whole plan seeded a private estate (1214 records, 13.1 s) and `rigseed --check` exited 0. Notes `agent-work/b116-record-query-paging/` |

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
| **3** | **The repository:** `~/me/projects/docket`, `go.mod`, the seven importers and `cmd/rigseed` copied in, decision 6's cut-sha in the first commit | `go list -deps` names no rig package (acceptance A); `rigseed --check` exits 0 from the new repo against the same estate |
| **4** | **The brief over the wire:** `brief.go` rewritten as `record.query` and `record.refs` calls through the stub (decision 2, first shape); the eleven store symbols it reaches (seam FINDINGS) each become a wire call or a docket type; latest steps derived in docket from `record.query` on kind `progress`, since rig's `lateststeps.go` stays as the store's own read; `stronglyConnected` duplicated into docket (both sides need it); docket registers with rig the way `cmd/fakeapp` does and declares `brief` as a command, so `rig docket brief` answers through M1's projection | acceptance C, the diff of two briefs; **and the timing beside it**: `docket brief` and `rig brief` at the cut-sha, 50 runs each, medians in FINDINGS, judged against decision 2's bound |
| **5** | **`cmd/rig/brief.go` leaves**; `rig docket brief` answers through M1's projection; `project.brief` becomes decision 4's forward | `cmd/rig/refusal_verbs_test.go` green; `rig brief` prints the refusal that names the new command |
| **6** | **`records.go` loses its planner constants** (decision 5, narrowed), and `IsStepState`, `EncodeTags`, `DecodeTags` go with them if nothing store-side uses them (move 3 found the seeder reaching all three); `internal/daemon/record.go` keeps `record.KindCount`, which is a store type. Measured 2026-09-24: outside `internal/record`, only `cmd/rigseed` uses the planner kind and link constants, so this move waits for move 8's deletion of rig's `cmd/rigseed` | `git grep -n 'KindWorkItem\|KindRequirement\|KindDecision\|KindNote\|KindProject\|KindCase'` in rig returns nothing outside `plan/` |
| **7** | **The pane:** the nine frontend files move into `docket/frontend/` with a copy of rig's frontend toolchain (`package.json`, the vite config, the kit source it needs); docket builds them, embeds the build with `go:embed` and serves `/pane` from its own HTTP listener, then declares `pane_url` in its `Declaration` - **exactly `cmd/abacus`'s shape, measured 2026-09-24** (`cmd/abacus/main.go`: `//go:embed all:kit`, `mux.HandleFunc("/pane", ...)`, `declaration(id, origin+"/pane")`; `wire.proto` `Declaration.pane_url = 10`). `Dashboard`, `Sections` and `Waffle` in rig lose their imports | `make test-window` 0, `make contrast-window` clean, the window shows platform data only, and `rig peers` (or the window) shows docket's pane at its URL |
| **8** | **The daemon and the door:** `serveProjectBrief` and `project_brief` leave; decision 4's forward arm replaces them | `internal/daemon` tests green; the MCP door lists the planner's promoted tool and not rig's |
| **9** | **`bench-size` re-recorded with the shrink attributed** - gen 22 attributed `rigd`'s growth to the store arriving; the brief's leaving is the first row that goes down | `make bench-size` rows for `rig`, `rigd` and `rigwindow` re-recorded under `rig-makefile` in the same commit |

---

### Found by move 3, 2026-09-24, and where each went

| Found | Where it went |
|---|---|
| ⛔ **`rigseed --check` over the whole 50-section `plan/` cannot complete, and it is rig's wire:** `record.query --kind requirement` answers 1,123,924 bytes, over `MaxFrameSize`, so the query dies and the seeder's 30 s deadline kills the child. Rig's own seeder fails byte-identically, so it predates docket. **It bites his production re-seed too** the moment `plan/47-50` are in the store | **B116**, the lead's, before or with his re-seed. Decision 2's `fields` projection is one fix; paging the answer is the other; the cap alone is not a fix. ⛔ **PICKED 2026-09-24 07:40: PAGING.** The seeder's check compares BODIES (`cmd/rigseed/main.go`, `storeRecords`), so the projection cannot bring the requirement query under the cap; paging bounds every answer by construction. Specified below ✅ **CLOSED 2026-09-24 at `0eab674`**: row I above and the table below carry the numbers |
| decision 5 missed three symbols the seeder reaches: `IsStepState` (`progress.go`), `EncodeTags` and `DecodeTags` (`brief.go`) | docket carries them in `internal/kinds`; move 6 finds them on rig's side and the `git grep` in its proof row grows by three names |
| two plan pins passed over nothing: `os.DirFS` on a missing directory returns no error, so `TestTheRealPlanKeysAreUnique` and `TestNoPlanKeyRunsPastTheMeasuredBound` ran over zero entries | docket's copies skip loudly with the reason; rig's originals are correct today only because the directory exists - the B87 class, a check that passes by not checking |
| the seat's layout: `internal/importers`, `internal/kinds`, `cmd/rigseed`, and `internal/docs` (eight lines resolving `DOCKET_DOC_ROOT` so the 21 live-document pins run from the new home) | the "Files a future seat owns" table below is superseded by the repository itself for docket's side; `agent-work/split-move-3-docket-repo/FINDINGS.md` lists every adaptation |

### B116, the answer is paged - specified 2026-09-24 07:40 for the `paging` seat

**Wire, §21-additive.** `RecordQueryRequest` gains `uint32 limit = 6` and
`string after = 7` (**5 is reserved for §48's `fields`**, and the comment
says so); `RecordQueryResponse` gains `string next = 2`. Empty `after` is the
first page; empty `next` is the last. `limit` 0 means the daemon's budget
alone; non-zero caps the page's count as well.

**Daemon.** A page is filled in the store's existing order (`ORDER BY
r.project, r.kind, r.id`, unchanged since `5a158a6`) while the running
`proto.Size` of the answer stays under `RecordPageBudget = 256 KiB`; a page
always carries at least one record, because `record.put` already caps a body
below a frame. `next` is an OPAQUE cursor encoding the last row's (project,
kind, id); no client decodes it. A cursor that does not decode is refused
`INVALID_ARGUMENT`; one that decodes but names a row no longer present still
positions, because the store compares rather than looks up. Decision 6 binds
the string field: two mutations, empty and wrong non-empty.

**Store.** `Query`'s signature does not change (the MCP door and the brief
call it in-process). A paged read is a second method, cursor and count in.

**CLI.** `rig record query` loops until `next` is empty and prints what it
printed before, so `cmd/rigseed` (either copy) does not change. No new flag.

**Docket.** Its query helper loops on `next` from its first commit (track E);
against a daemon without B116, `next` is empty and the loop is one page.

| Acceptance | Command |
|---|---|
| **the whole plan seeds and checks** | a PRIVATE estate (state AND runtime roots private), seeded by docket's `cmd/rigseed --rig <scratchpad rig>` from the current 50-section `plan/`; `rigseed --check` exits **0**; `rig record query rig requirement --json \| jq length` equals the seeder's count; rigd's log has no `frame exceeds` ✅ 1214 records seeded, `--check` exit 0, `rig record query rig requirement --json \| jq length` = 489 = the seeder's count with 489 unique ids, 0 `frame exceeds` in the daemon log, production `record.db` size and mtime unchanged (FINDINGS) |
| **pages are exact** | a daemon test: records whose wire size exceeds the budget answer in N pages, every frame under `MaxFrameSize`, union complete, no duplicate, no gap ✅ `279fadd` |
| **additive** | with no `limit` and no `after`, a small fixture's answer is byte-identical to before and `next` is empty ✅ `279fadd` |
| **the guards bite** | `after` = garbage is refused `INVALID_ARGUMENT`; the budget mutated to 0 still yields one record per page; the CLI against a three-page daemon prints every record once ✅ four mutations in a detached copy: budget 0 gives 64 records in 64 pages with the union complete; `>` to `>=` on the cursor, the CLI dropping `after`, and a permissive decode each turn a test red |

**Recorded from the seat's report, 2026-09-24 08:35 (lead).** Three
properties nobody ruled on and nothing in rig needs today: a paged answer is
N queries rather than a snapshot, so a concurrent write is seen or missed by
where it sorts; the MCP door's `record.query` stays unbounded because it is
in-process and has no frame; a single record larger than `MaxFrameSize`
cannot be answered, and cannot enter either, so at-least-one holds by
construction rather than by a check. The cursor codec lives in
`internal/daemon/recordpaging.go`. **Cost, §37:** the wire commit landed
7 m 47 s before its handler and `TestNoServedRequestFieldIsSilentlyDropped`
held `internal/daemon` red for every gate in between; the rule is now in
`COORDINATION.md`: a wire field and its handler are ONE commit.

### Acceptance, restated so it can be checked rather than read

**§44's form: a thing is DONE when rig's own version of it is DELETED, proven
by `git grep`, and §45's: the test is stated before the seat starts.**

| | Test | Command |
|---|---|---|
| **A** | **the planner imports nothing of rig but the stub and the stub's closure** (decision 1) | `go list -deps ./... \| grep github.com/borismilner/rig \| grep -v -E '/client$\|/internal/paths$\|/internal/wire$\|/proto/rig/v1$'` prints **nothing** in docket's module. **At move 3, before the stub is needed, the plain `grep -c` prints 0** ✅ **PASSES 2026-09-24** at docket `4e88521`: the command prints nothing |
| **B** | **rig names no program, project or plan** (§43's test for a disputed row) | `git grep -l -E 'BacklogParse\|PlanParse\|DecisionParse\|serveProjectBrief\|ProjectBriefRequest\|KindWorkItem' -- ':!plan/' ':!*.md' ':!proto/'` prints nothing in rig. **`':!proto/'` added 2026-09-24 07:40:** the seven brief messages stay in `wire.proto` until the next major by decision 4, and the one arm that still decodes `ProjectBriefRequest` to refuse it is named `refuseProjectBrief` ✅ **PASSES 2026-09-24** at rig `aaed1de`: the command prints nothing |
| **C** | ⛔ **THE DEMONSTRATION: the same store, two briefs, no difference.** `rig backup` on production (S2); `rig restore --estate development` under a PRIVATE `XDG_STATE_HOME` and a private `XDG_RUNTIME_DIR` (§37's set has two names and `rigd` refuses every other - §46 row 8 was corrected for the same reason); the OLD `rig brief` from a binary built at the pre-extraction sha and the NEW `rig docket brief` both run against that estate | `diff <(old) <(new)` is **empty**. The restore is the fixture, so production is never touched, and the check is a byte comparison rather than a reading ◐ **HALF, 2026-09-24:** byte-identical at docket `2d83492` on a SYNTHETIC store built by the old binary. The production form waits on Boris: `rig backup` needs the redeployed rigd, and the harness denied a seat reading production |
| **D** | **the seeder still agrees with the documents from its new home** | `rigseed --check` exits **0** from the planner's repo against that estate ◐ **HALF, 2026-09-24:** docket's `rigseed --check` exited 0 against a private estate seeded from the current plan (paging seat, B116). Against C's restored production estate: waits on C |
| **E** | **both repositories gate green, and docket's `go.mod` carries no `replace` once rig is pushed** | `make ci` 0 and `make lint` 0 in rig; the planner's equivalents 0; `grep -c '^replace' go.mod` in docket prints 0 after his push ◐ **GATES GREEN 2026-09-24:** rig `make ci` 0 at `818db8a`, golangci 0 issues; docket `make ci` 0 and `make lint` 0 at `737a41a`. The `replace` stays until Boris pushes rig |
| **F** | **the tray test is untouched** | the production icon, during the session and after a reboot - §39, unchanged ⏳ **HIS EYES:** the agentbox session split the tray into its own process at `cf337e6`, so this test now also covers that change |

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
| `internal/record/brief.go` + tests | **move, rewritten** | decision 2. **`lateststeps.go` STAYS**, a store file (corrected 2026-09-24) |
| `internal/record/{store,records,control,links,refs,snapshot,progress,lateststeps,notes}.go` + tests | **stay** | decision 5 edits `records.go` only. `notes.go` is the seam seat's split of the note rows from their ordering (`f0682cf`). Open: whether `LinkPartOf` belongs in `links.go` beside the other link types rather than `records.go` |
| `cmd/rigseed/` | **move, whole** | decision 8 |
| `cmd/rig/brief.go` + tests, `testdata/exec/brief-*.golden` | **move** | move 5 |
| `internal/daemon/record.go`, `mcp_records.go` | **split** | one arm each leaves; the seat's grant is those arms and nothing else in the lead's files |
| `internal/daemon/self.go` | **one block** | `project.brief` becomes decision 4's forward |
| `proto/rig/v1/wire.proto`, `wire.pb.go` | **seven messages** | at the next wire major only, §21 |
| `frontend/src/lib/` nine files; `Dashboard`, `Sections`, `Waffle` | **move; edit** | move 7 |
| `~/me/projects/docket/` | **NEW** | the seat's, whole |
| `logbook/projects/docket/` | **NEW** | by the standing convention until the planner retires it |

---

### What this section does not change

- **§43 and §45 stand in full; §44's ORDER is reversed by him and its
  content stands.** This is §43's build shape under §45's gate, and the gate
  opened on 2026-09-24: Q1, Q4 and the order all answered by him in one turn.
  ✅ A seat may start any move whose inputs have landed, per the schedule.
- **§39's design survives** as the planner's declaration: the nouns, the
  provenance, the grain, the two co-equal consumers, the retention exemption.
  `progress` stays rig's, as §43 rules.
- **The tray acceptance test is untouched.** It is a deployment test and no
  move here reaches it.
- **§21 is not overturned.** Decision 4 keeps every shipped wire version
  served; the brief's messages leave at a major, not before.
- **Deferral is never deletion**, §43. The eight services §44 orders after
  the split are still rig's to build.
- ✅ **The capability after B104 was his to choose, and he chose this one**
  on 2026-09-24: *"We need it split ASAP; Do all needed to make it happen
  ASAP!"* Three seats were spawned the same turn.
