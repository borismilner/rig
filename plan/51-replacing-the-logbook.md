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

### Open, and his

| Q | Question |
|---|---|
| Q1 | ~~§43 Q1: does the record stay in rig, or move to the planner program?~~ **ANSWERED 2026-10-01: the record lives in rig.** |
| Q2 | ~~scope of the first switch: `rig` alone, or every project and area at once~~ **ANSWERED 2026-10-01: *"rig alone first, go ahead"*.** The rig project switches first; the others follow on its scorecard |
| Q3 | which "new level" rows are in the first switch, and which come after |
| Q4 | the dual run (§39): kept as ruled, ending on his word |
