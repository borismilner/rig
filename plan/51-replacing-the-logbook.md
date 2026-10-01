## 51. Replacing the logbook

**Boris, 2026-09-27, verbatim:** *"Before we switch from logbook to rig, lets
plan so that we cover all logbook functionality and take it to the whole new
level."*

⛔ **THE BAR, Boris 2026-10-01, verbatim:** *"The next thing I want us to do
is to come to a phase where you as my agent (and all other agents) can use
`rig` instead of the logbook, but for that to happen, the `rig` must be
absolutely awesome and superior to the logbook in all aspects."* **So the
switch is gated on superiority, not on parity:** every row L1-L16 must be
MEASURED better in rig than in the logbook, for agents and for him, before
any agent is pointed at rig instead. Parity is a failure of this bar. It is
the next phase, ahead of S1's remaining clean-up and the bus.

⛔ **THE GOAL, RESTATED BY BORIS THE SAME DAY, verbatim:** *"What I want is
to is to make the management of the logbook more streamlined, more
efficient. Maybe in a way that is easier on the agents to do good job while
wasting less tokens. But I don't want `rig` to become the bottleneck"*. **So
the measure is agent tokens and agent effort, and the hard constraint is
that no agent's work may stop or slow because rig is down, busy or slow.**
A design where rig is the only write path fails the constraint.

⛔ **RULED BY BORIS THE SAME DAY: THE FILES STAY THE TRUTH, rig IS A FAST
INDEX OVER THEM.** Offered "files stay the truth, rig answers questions about
them, appends without reading, and the files are split smaller", he said,
verbatim: *"yes, go with that direction; That's what I meant; this way even
if rig is down the agents can still work with the files. Maybe along the way
we can think of a better structure for the files, maybe a better split for
more efficient access."* **This supersedes the projection-first path below
for the logbook:** agents write files; rig watches, indexes and answers; with
rig down an agent reads and writes the files exactly as before.

⛔ **DRAFT, written the same turn so the requirement is not held only in a
session.** The inventory below is measured; every "new level" row is a
proposal of the lead's (§37) and is his to accept, reshape or refuse. §39
already specifies most of the record (the spine, the kinds, the verbs); this
section is the checklist that says whether the switch loses anything, and it
points into §39 rather than restating it.

### What the logbook does today, measured 2026-09-27

29 directories (18 projects, 11 areas), one private GitLab
remote, 42 MB for `rig` alone, 111 agent-work folders under `rig`.

| # | Function | Carried by | rig today | Gap |
|---|---|---|---|---|
| L1 | notes keyed by directory, no registry | `projects/<name>/`, `areas/<area>/` | the record holds one project, `rig` | every other project and area |
| L2 | resume brief, overwritten per session | `HANDOFF.md`, `/handoff`, `/resume`, `recent-handoffs.sh` | `project.brief` (B46a built, B46g open) | a handoff kind, and the two commands pointed at rig |
| L3 | current state | `STATUS.md` (207 files) | `progress` stream, `rig health` | a status a person reads in one screen |
| L4 | dated narrative | `history.md` | record history per id | a per-project timeline view |
| L5 | decisions with reasoning, append-only | `DECISIONS.md` | kind `decision`, 586 seeded | seeding is one-shot, not a sync (B63) |
| L6 | work items and their order | `BACKLOG.md` | kind `work-item`, 135 seeded | order and ranking, B62 |
| L7 | readiness, numbers only | `READINESS.txt` | nothing | a derived view, not a hand-kept file |
| L8 | who owns which file, lock names | `COORDINATION.md` | peers, locks in AgentBox | lock verbs (cutover-gap.md) |
| L9 | tooling config reached by symlink | `CLAUDE.md`, `.claude/settings.json` | nothing | the tooling reads a FILE; the projection must write it |
| L10 | background agent persistence | `agent-work/*/STATUS.md`, `FINDINGS.md`, `agent_persistence.py` | nothing | an agent-run kind, and the hook pointed at rig |
| L11 | artefacts: audits, briefs, evidence dirs, scripts | loose files, 66 in `rig/` | `rig files`, kind `artefact` (1) | import, and links from records to files |
| L12 | works with rig down | plain files | nothing | §39's spine: the projection and `_pending/` |
| L13 | offsite and history | git plus GitLab | `rig backup` (local) | the projection commits and pushes (§39) |
| L14 | search across everything | `grep` | `record query`, FTS for files | one search over records and artefacts |
| L15 | private, never in a product repo | a separate repo | the estate is local | the projection target per document class (§39) |
| L16 | exclusions | `d2d` keeps its own notes | none | carried over unchanged |

### The whole new level, proposed

| # | Proposal | Why a file cannot do it |
|---|---|---|
| N1 | `/resume` is a query, never a stale file: the brief is derived from records at read time | a handoff is only as fresh as its last write |
| N2 | every quotation of Boris carries a link to its transcript line | "the carrier is not evidence" becomes a check instead of a rule |
| N3 | requirement, decision, work item and commit are typed links, and a requirement with no work item is a visible gap | today that walk is a grep |
| N4 | `READINESS` is computed from work-item state and seat-day estimates | today the lead must remember to edit it (§37) |
| N5 | an agent that dies leaves its state in the record, because progress is written as it works | today the hook reconstructs it after the fact |
| N6 | the window shows a project's brief, timeline and gaps; toasts and speech for deadlines and blocked items | files have no push |
| N7 | one search across every project, record and artefact, ranked | grep has no ranking and no kinds |

### The design: files stay the truth, rig is the fast index (DRAFT, 2026-10-01)

**Every row is the lead's proposal until he approves it.** It follows his
ruling above. Measured on the rig project the same day: DECISIONS.md has
247 entries (median 2.2 KB, largest 18 KB) in one 629 KB file; BACKLOG.md
has 248 rows (median 0.4 KB, largest 5.8 KB) in one 270 KB file.

**One entry, one file; the big file becomes a generated index.** This is
the split `plan/` already proved (`tools/plansplit.py`): citations keep
resolving, and a byte-for-byte reassembly check proves nothing was lost.

```
logbook/projects/rig/
  HANDOFF.md                 small, overwritten per session (unchanged)
  DECISIONS.md               GENERATED index: one line per decision
  decisions/2026-10-01-the-logbook-files-stay-the-truth.md
  BACKLOG.md                 GENERATED index: open items in order, then closed
  backlog/B107-the-event-bus.md      front matter: status, order, owner
  ...
```

| | Rule | Why |
|---|---|---|
| F1 | **An entry is one markdown file with a few front-matter fields** (date, title, status, order, owner, links). The body is free prose, as today | an agent reads the 2 KB it needs, not the 629 KB around it |
| F2 | **Adding an entry is creating a file.** No read before append, and two peers never collide on one file | the "read it before appending" rule cost ~157k tokens per decision |
| F3 | **The index files are generated** by a tool that needs no daemon, and checked like `plansplit.py --check` | an index nobody regenerates goes stale silently |
| F4 | **Old paths keep resolving:** DECISIONS.md and BACKLOG.md stay, as indexes | the same reason PLAN.md stayed: hundreds of citations |
| R1 | **rig watches the logbook and keeps a search index over it**; it never writes an entry on an agent's behalf | his constraint: rig is never on the write path |
| R2 | **`rig logbook` answers small questions**: `brief`, `grep <pattern>`, `show <id>`, `open` (open items in order), `add <doc>`. Named `logbook`, not `log`, so it never collides with §49's logging | an answer in 1-5 KB instead of a whole-file read |
| R3 | **With rig down, `rig logbook` still works on the files directly** (slower search, same answers), and plain `grep` and an editor always work | no agent's work stops because rig is down |
| R4 | **Agents reach it through the CLI and MCP alike**; the skills (`/resume`, `/handoff`, `where-it-belongs`) are pointed at it | the token saving only happens if the tooling uses it |

⛔ **NO REGRESSION, Boris 2026-10-01, verbatim:** *"go ahead with all the
splits and then what's needed in `rig` to support it; note it must not
regress from the current usage of the files."* **Every way an agent or he
uses these files today - read, grep, append, edit, cite, resume - keeps
working after a split, or the split is wrong.** The breaks found so far are
listed in the scorecard and each is owed a fix, not an apology.

**As built, 2026-10-01 (rig `8015d70`): how each use survives the split.**
`rig logbook` runs client-side, so R3 holds by construction.

| Use today | After the split |
|---|---|
| read the whole file | `rig logbook cat <doc>`: byte-identical to the pre-split file, proved on all five |
| `grep DECISIONS.md` | `rig logbook grep <pat> DECISIONS.md`: the same lines, plus what waits below the marker |
| cite "DECISIONS.md line 5889" | `rig logbook line DECISIONS.md 5889`: the part and its line |
| append the old way | still works below the marker; `index` moves it into a part |
| add without reading | `rig logbook add <doc> --title T` from stdin, or create the next file |
| edit an entry | edit its part; an edit above the marker in the index is lost (named in the index's first line) |

`brief` and `open` landed the same day (rig `274244f`): the brief is
11 KB against 1.33 MB of whole files. ⛔ **`open` lists 108 of 126
items as open**, because a state is free prose and only CLOSED, DONE or
REJECTED at the start of it closes an item; many rows say "RULED ...
DONE" or "BUILT" further in. F1's state field is what fixes that. Not
built: R1's search index in rigd.

**The acceptance is measured in tokens, per task, before and after:**

| Task | Today | Target |
|---|---|---|
| append a decision | read 629 KB | search 1-3 KB, write one file |
| find one backlog item | read 270 KB | one file, 0.4-6 KB |
| cold resume | 1.33 MB over 7 files | HANDOFF + `rig logbook brief`, under 20 KB |

**Not in this design:** the record does not become the source of the
logbook, and §39 slice 5's projection is not built for it.

### Open, and his

| Q | Question |
|---|---|
| Q1 | ~~§43 Q1: does the record stay in rig, or move to the planner program?~~ **ANSWERED 2026-10-01: the record lives in rig.** |
| Q2 | ~~scope of the first switch: `rig` alone, or every project and area at once~~ **ANSWERED 2026-10-01: *"rig alone first, go ahead"*.** The rig project switches first; the others follow on its scorecard |
| Q3 | which "new level" rows are in the first switch, and which come after |
| Q4 | the dual run (§39): kept as ruled, ending on his word |
