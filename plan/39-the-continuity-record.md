## 39. The continuity record

**RULED BY BORIS, 2026-09-12**, after he raised the problem unprompted on two
consecutive days. Put to him as four options; he took rig carrying the
mechanism and then widened it in the same breath:

> *"I think rig should contain as much as possible since this is a single
> source of truth we can perfect, instead of perfecting general instructions
> across many different CLAUDE.md"*

### ⛔ THE MVP, DEFINED BY BORIS 2026-09-16 LATE, IN HIS OWN WORDS

> *"MVP is being able to use `rig` to work on `rig` with respect to the
> project/case management."*

**THIS SECTION IS THE MVP. Nothing else is.** Recorded the turn he said it,
because the word had two live meanings by then and the other one was about to
be built toward.

⛔ **IT IS NOT THE AgentBox GUI PORT.** `READINESS.txt` carries a standing
report answering *"is rig ready to replace the GUI of AgentBox"*, asked for by
him on the same day, and **that is a DIFFERENT question with a different
answer.** M8 and its embedded pane tier are the destination he confirmed for
the TRAY; they are not this. **A seat told to "converge to the MVP" and pointed
at §11 or §23 is building the wrong thing**, and the lead nearly reported it
that way before he said this sentence.

**WHAT IT MEANS FOR THE BUILD ORDER**, and it is the first thing §39 has ever
had that ranks its own slices:

| | |
|---|---|
| **slice 1** - the store and the three nouns | **on the MVP path.** Nothing works without it |
| **slice 2** - progress streams and `project.brief` | **THE MVP LANDS HERE.** "Work on rig with respect to project management" IS a work item driven start to finish and a brief read back |
| slice 4 - links and `record.refs` | on the path: a case that cannot say what it touches is not case management |
| slice 3 - the window renders the brief | **NOT the MVP.** He said *use* rig, and the CLI and the MCP door are both uses |
| slices 5 to 8 | **NOT the MVP.** The projection, the gate, standards and the migration all follow it |

**THE TEST OF THE MVP IS THE ONE HE NAMED: rig's own project and cases are
managed IN rig.** Not demonstrated on a fixture, not on a synthetic second
project - on rig itself, which is the only project there is.

**AND IT SETTLES THE PRIORITY QUESTION §39 COULD NOT ANSWER ABOUT ITSELF.**
This section was sized at 48-59 seat-days across eight slices on 2026-09-16 and
that number was read as the cost of the whole thing. **The MVP is slices 1, 2
and 4** - and a number for that subset is owed rather than assumed, because
the sizing was never cut that way.

### ⛔ THE GRAIN IS THE HEADING. RULED BY BORIS 2026-09-16 LATE.

**Put to him as three options with their costs; he took the middle one.** This
had been open since 2026-09-12, was dropped once as superseded, and was carried
by two seats as *"waiting on Boris"* **while nobody had asked him** - which is
the loop-nobody-sent failure, caught by the record seat chasing its lead.

| | |
|---|---|
| **ONE RECORD = ONE HEADING** | every `##`, `###` and `####`. **239 requirement records**, against 639 for bold-leads and 39 for whole sections |
| **what it buys** | a **~40% smaller graph than the bold-lead grain**, so every traversal figure the record seat has published improves |
| **what it costs, stated because he was shown it** | **a requirement stated mid-paragraph under a heading with five others is not separately addressable.** A supersede names the heading, not the sentence |
| **what it refuses** | the 39-section grain. §37 is **942 lines**; a link to it points at a DOCUMENT rather than a requirement, which is attack finding 5's failed row |

### ⛔ THE MIGRATION IMPORTS ALL 4,206 CITATIONS AND FLAGS THE COARSE ONES. RULED BY BORIS 2026-09-16 LATE.

**The 22% floor was put to him with its three answers.** He took the one that
imports everything.

| | |
|---|---|
| **22%** - 911 citations carrying a subsection letter | resolve to ONE record. Links |
| **78%** - 3,295 resolving only to a whole section | **imported as section-grained and COUNTED AND REPORTED AS FAILED ROWS** |
| ⛔ **what this refuses** | dropping the 78%, which would leave most of the estate's cross-references invisible to rig; and blocking the migration behind a manual citation rewrite, which is a large pass for a slice already off the MVP |
| ⛔ **the named risk, his to have accepted** | **a flagged count that nobody ever burns down.** `project.brief` surfaces it, so it is visible rather than silent - but visibility is not a plan for it, and this specification does not pretend otherwise |

### ⛔ THE MVP SHIPS NO EXPORT, SO THE LOGBOOK STAYS AUTHORITATIVE

**Raised by `backend-record` 2026-09-16 while sizing the MVP subset, and it is
the one condition attached to the MVP ranking above.**

**Slice 5 is the projection - the record written out as files, committed and
pushed - and slice 5 IS NOT IN THE MVP.** So for the whole MVP window, rig's
project and case management lives in **one SQLite file, on one laptop, with no
remote.**

⛔ **THE CONDITION, AND IT IS NOT OPTIONAL: THE LOGBOOK STAYS AUTHORITATIVE AND
NOTHING DELETES IT UNTIL SLICE 5 SHIPS.**

**This is not a new mechanism and not a gap.** §39's migration rule already says
the logbook stays authoritative until the projection is comparable and that both
run in parallel. **What changes is the timing: the MVP makes that promise
load-bearing months earlier than the slice that honours it.**

**The one way it goes wrong is a reader acting on "rig replaces the logbook"
while the export does not exist** - and §39 says that sentence in several
places, because it was written assuming all eight slices. **Against a
single-file store with no remote, acting on it is how the only copy is lost.**
§39's own words on exactly this failure: *"this is how a laptop ends up holding
the only copy."*

### ⛔ A `blocks` CYCLE IS DETECTED AND REPORTED, NEVER RESOLVED

**RULED BY THE LEAD 2026-09-16, on `backend-record`'s finding, because it
reaches slice 1's LINK SCHEMA and slice 1 is on the MVP path.**

**The defect it closes:** the brief's next-up list is specified as *"in
execution order"* and the only definition of that order is a back-reference
inside the CASE section - *"the same topological-sort-over-`blocks`-among-active-
items derivation"*. **A topological sort needs a DAG and §39 specified no cycle
detection anywhere, for any purpose.** So the ordering had no defined answer on
a cyclic backlog, and the failure is not a wrong order: it is a derivation that
does not terminate, or that silently drops the items in the cycle. **Every
arriving session calls this.**

| On a cycle | |
|---|---|
| **detected** | the derivation terminates, always. A cycle is found rather than run into |
| **reported** | it appears in the brief as a BLOCKED CONDITION, naming the items in it |
| **ordered around** | every item not in the cycle keeps its place in the list |
| ⛔ **never resolved** | rig does not pick an edge to break |

**WHY REPORTING RATHER THAN BREAKING, and it is §29 non-goal 1 rather than
caution:** choosing which `blocks` edge is the wrong one is a judgement about
the work, which is domain logic, which is *"the thing rig refuses to have"*. **A
cyclic backlog is a real state of a real project and it is one Boris would want
SURFACED.** Swallowing it silently is the failure; so is refusing to answer.

⛔ **AND IT IS NOW DEMONSTRATED RATHER THAN REASONED.** Measured 2026-09-16
by `backend-record` at the lead's instruction, after the ruling was taken on
its reasoning alone. A `0->1->2->0` cycle with the depth bound dropped **DOES
NOT TERMINATE IN 20 SECONDS** - exit 124.

**The mechanism is worth carrying, because it defeats the obvious defence:**
a recursive `UNION` dedups on `(node, depth)`, and **depth keeps incrementing,
so the dedup NEVER FIRES on a cycle.** A reader who assumes `UNION` is already
a visited set will not write a guard. **What the guard prevents is a HANG, not
a wrong answer**, which is the worse of the two failures and the one a test
with a timeout reports as an infrastructure problem.

**ONE RULING CLOSES TWO GAPS.** `STORE-REQUIREMENTS.md` R3.3 raises the same
need for bounded traversal from the other end. **A cycle guard costs almost
nothing in the link schema and is expensive bolted onto two derivations
afterwards** - if slice 1 defines links without it, slice 2 and slice 4 both pay.
**That is why this is ruled now rather than at slice 2, where it was found.**

### ⛔ A LINK MAY CROSS A PROJECT BOUNDARY. A TRAVERSAL IS PROJECT-SCOPED.

**RULED BY THE LEAD 2026-09-16 late, on `backend-record`'s finding, because it
reaches the LINK SCHEMA committed at `88bdc44` and slice 1 is on the MVP path.**
The store was opened with `links (src, type, dst)` and a reverse index on
`(dst, type)`, carrying **no project column**, and §39 had never stated whether
an edge may leave its project. The seat asked before building on either answer.

⛔ **CROSSING IS NOT PERMITTED - IT IS REQUIRED.** §39 names two verbs that
cannot exist without a crossing edge:

| Verb | Why it crosses |
|---|---|
| `standard.stamp` | *"record that a project was checked against a standard"* - a `checked-against` edge from a project's record to a register record, and the register is **the whole estate, ONE COPY** |
| `standard.drift` | *"every project behind the standard it claims to uphold"* - a reverse query across every project at once |

**And §39 twice names *"no cross-project query"* as a capability LOST when rig
is DOWN.** A thing listed as degraded is a thing that exists when the daemon is
up. The register is not an extension to this design; it is half of it.

**THE TWO NEARBY RULINGS DO NOT SAY OTHERWISE, and both were read too widely
before this was settled.** *"A local kind cannot be linked to from another
project"* is a containment rule about **local kinds**, not about links -
it is narrow and it stays. **Tension 14's separate repositories are about the
PROJECTION on disk**, not about the link schema.

| | |
|---|---|
| **a link MAY cross a project boundary** | it must, or the standards register has no mechanism at all |
| **a traversal is PROJECT-SCOPED BY DEFAULT** | `record.refs` and every `project.brief` derivation stay inside one project |
| **crossing is EXPLICIT** | a cross-project answer is ASKED FOR, never arrived at. `standard.drift` is the named caller |

⛔ **THE DEFAULT IS A PERFORMANCE REQUIREMENT, NOT A PREFERENCE. MEASURED.**
The scoping predicate prunes the frontier at **every hop**, so scoping does not
add a filter to the same work - **it removes the work**:

| At 100x, reverse CTE, median of 12 | unscoped | scoped |
|---|---|---|
| depth 4 | **307.3 ms** | 5.79 ms |
| depth 5 | **990.0 ms** | 16.33 ms |
| distinct nodes visited | 62,007 | 706 |

**Unscoped traversal at 100x is 15x over R2.3's budget. Scoped-by-default is
what makes that budget reachable at all**, which is a stronger reason than the
semantic one this ruling was taken for.

⛔ **THE REGISTER IS A HUB, AND THAT IS NOT A PATHOLOGICAL READING.** Every
project in the estate stamps the SAME standard record, so a heavy-tailed graph
is the register's REAL shape - measured at **1.752s**, 87x over budget, against
13.3ms for the same corpus read as separate projects. **What saves it is that
`standard.drift` is depth 1 by definition.** So: **drift is served by a bounded
reverse lookup and MUST NEVER be served by the general traversal.** If it ever
is, the register is 87x over budget on the one query it exists to answer.

**THE SCHEMA STAYS AS COMMITTED: no project column on the edge.** A crossing
edge has TWO projects - the source's and the destination's - so a single column
means something different on a crossing edge than on a local one, and every
query would have to know which kind of edge it was reading before it could
trust the column. A denormalised project is also a copy of a fact that lives on
the record, which is the drift class this project already counts instances of.
Scoped traversal joins `links.dst` to `records.id`, the leading column of the
primary key. **Measured, not assumed:** the join is the optimisation, not the
cost.

### ⛔ `project.brief` MUST RUN ALL FOUR SQL FORMULATIONS BEFORE IT PICKS ONE

**RULED 2026-09-16 on a measurement that nearly shipped backwards.** The
per-group-maximum question - which every `project.brief` derivation asks - has
four reasonable SQL formulations in SQLite, and **the spread between them is
117x at 44,000 rows and 8,200x at 4.4 million.** `GROUP BY MAX` is 327ms at
100x where the window-over-scan formulation is **5.02 SECONDS**, and the
correlated subquery is 613us.

⛔ **SO THE BRIEF'S SQL IS A CORRECTNESS SURFACE, NOT A TUNING EXERCISE.** A
seat that writes whichever formulation it thought of first has a 50ms budget
and may be four orders of magnitude outside it, with a query that returns the
right rows. **Whoever writes it runs all four and records which was taken and
what the other three measured**, in the commit body.

**This is the same instrument failure the store search paid for twice:** the
first benchmark measured the seat's own SQL and reported it as the database's,
a 414x win that did not exist. **The formulation is part of the measurement,
never a detail below it.**

**WHAT THE RECORD IS FOR.** A session arriving cold has to learn what the
project is, what was already decided, what is in flight, and what it is allowed
to write. Today that lives in prose: a `CLAUDE.md` per project telling a session
how to handle a `PLAN.md`, a `HANDOFF.md` and a `STATUS`. **The prose is the
defect he named** - *"we rely on information detailed in `CLAUDE.md` to know how
to handle `PLAN.md`"*. It is N copies, none of them enforced, and a session that
misread one is indistinguishable from a session that read it.

**TWO CONSUMERS, CO-EQUAL. This is a constraint on the schema, not a choice
about rendering.**

| Consumer | What it needs from the same record |
|---|---|
| **the agent** | resume with no re-learning: what is decided, what is claimed, what must be read before it may write |
| **the human** | oversee at any step, in the window, **without a seat composing a report for him** |

**Boris, verbatim on the second consumer:** *"...I won't really need the agents
to prepare me beautiful progress reports ad-hoc because as they work they will
report progress as they go and the human-report can be derived automatically or
with very small agent effort; so there are two consumers of this information,
both the human and the AI agents."*

**THE REQUIREMENTS THAT FOLLOW, and each one is falsifiable:**

| | |
|---|---|
| **Progress is written DURING the work** | a call at a step boundary, not a report composed at the end. The composed report is the cost he named, and every seat on this project has paid it |
| **The human view is DERIVED, not authored** | the record carries enough structure to render without prose written over it. *"Very small agent effort"* is the ceiling; "an agent writes the summary" is not it |
| **EVERY session writes it, not the ones that opted in** | rig sees every session; a program sees only its adopters. **A board with holes is worst exactly where a session is in trouble** - the failure this estate already has when a seat forgets to announce |
| **rig ships the schema; a project extends it** | his ruling. A schema supplied per project is still N copies of something to get right, which is the thing he is trying to stop |
| **It is NOT §15** | §15 is the wire history: which calls a client made, coalesced and redacted. It answers *"what did this client do to rigd"* and never *"how far along is this work, and is it going well"* |

**WHY THIS IS NOT A BREACH OF §29 NON-GOAL 1, and the test used is the
non-goal's own.** It reads *"rig does not run business logic. Ever. A program id
appearing in rig's code is a bug."* **A continuity record names no program.** It
is sessions, records, claims and steps - the same family as the seats, leases,
messages and slots above. **§16 already holds agent-owned data in rig-defined
shapes**, so the record extends a precedent rather than crossing a line. What
§29 still forbids, and this does not ask for, is rig having an opinion about
whether a plan is any *good*.

**THE ACCEPTANCE BAR, AND IT IS HIS. PARITY IS FAILURE.** Boris, verbatim,
2026-09-12:

> *"For it to be perfect, it must be significantly superior on our current
> approach to managing development projects as described with the documents.
> Super robust and super beneficial; Unlocking exceptional functionality and
> usability for the project."*

**THE COMPARATOR IS NAMED BY HIM AND IT IS THIS PROJECT'S OWN DOCUMENT SET** -
`PLAN.md` and `plan/`, `BACKLOG.md`, `DECISIONS.md`, `COORDINATION.md`, the
`HANDOFF*.md` files, and the per-project `CLAUDE.md` that tells a session how to
use them. **This is 38a's operational form with the comparator supplied: name
the best existing implementation and say how rig's compares.** Here the best
existing implementation is what the project runs on today, and it works.

**So "the same documents, typed and in a database" FAILS this bar.** A typed
record that a session reads and writes the same way, for the same reasons, at
the same moments, is parity with extra machinery. **The bar asks for things the
documents structurally cannot do**, and the honest list of those is short enough
to hold a design to:

| What documents cannot do, structurally | Why the record can |
|---|---|
| **refuse a write until what must be read has been read** | a document can only ASK. `COORDINATION.md` says *"read in full before your first write"* and nothing checks. **rig mediates the write**, so the precondition is a mechanism |
| **know that a claim has gone stale** | a document records a fact that WAS true and cannot announce it stopped being. **rig sees the sessions**, so a claim tied to a live seat expires with it. Four measured instances in one night, `BACKLOG.md` B23 |
| **route a requirement at the moment it is stated** | today a seat has to decide where it goes and then do it. **Four of his requirements were found living only in a volatile document**, which is the same failure four times |
| **derive the human view** | a report over documents is composed by a seat. **A record written during the work renders without one**, which is the second consumer above |
| **answer a query instead of being read** | 5,218 lines that *"nobody reads whole"* - `PLAN.md`'s own Map says so. A requirement can hide in a document. **It cannot hide in a queryable record** |
| **be the same in every project** | a `CLAUDE.md` per project is N copies to perfect. §38c |

**SO THE BAR IS TESTABLE, which is the only reason it is worth writing down:
before this capability is promoted, each row above is demonstrated against the
document approach doing the same task.** If the demonstration is *"it is nicer"*
rather than *"the document cannot do this at all"*, the bar is not met.

**AND THE PROJECT'S PER-SESSION INSTRUCTION FILE SHRINKS TO A POINTER. This is
the capability's cheapest falsification test.** Boris, verbatim, 2026-09-12:

> *"In such a setup, the CLAUDE.md would just make claude aware of this feature
> and all the rest should follow from it."*

**Today that file is routing prose**: where a requirement goes, where a decision
goes, which file is generated, what must be read before a first write, which
names the locks use. **If it is still that after the record ships, the record
did not replace it. It joined it.**

| The test | |
|---|---|
| **What the file may keep** | one line making the agent aware rig holds the project's record, plus anything genuinely project-specific that is not routing |
| **What it must lose** | the routing table, the read-before-write instruction, the never-hand-edit warnings, the lock names. **Each of those becomes a mechanism or it was not replaced** |
| **Why it is the right test** | it is measured in lines, before and after, and **it cannot be satisfied by an agent trying harder** - which is the failure mode of every instruction ever added to that file |

***"AND ALL THE REST SHOULD FOLLOW FROM IT"* IS A DESIGN CONSTRAINT, not a
hope.** An agent told only that rig holds the record must be able to find out
from rig what it may write, what it must read first, and what is already
claimed. **Discoverability is part of the capability**, not documentation
wrapped around it, and a design that needs a second document to explain itself
has failed this line rather than deferred it.

**IT IS NOT ONLY THIS PROJECT'S OWN STATE. WIDENED BY BORIS, 2026-09-12**,
verbatim:

> *"one of the goals of this is that rig will be the single source of truth for
> many of the common development requirements and expectations we can perfect
> with time instead of spreading and repeating it accross many different
> documents... it's also to contain the must requirements for projects that can
> be changed and improved with time and since they are managed in a single place
> we can always know when a project should be raised up to those standard and
> checked from time to time it still upholds all the requirements."*

**TWO REGISTERS, ONE MECHANISM. The scope is what separates them:**

| Register | Scope | What it holds |
|---|---|---|
| **the project's own record** | one project | what is decided, what is claimed, what must be read, progress as it happens |
| **the standards register** | **the whole estate, one copy** | the requirements EVERY project must uphold, versioned, with each project's last-checked stamp |

**HIS TWO EXAMPLES ARE BOTH REAL AND BOTH CURRENTLY MISFILED**, which is why
they are worth naming rather than paraphrasing:

| Standard | Where it lives today | The defect |
|---|---|---|
| **visual legibility** - measured contrast, the palette method, the defect classes a clean audit still misses | a personal skill, plus the library's admission bar | it binds every project that ships a page, and **no project knows whether it is being applied**, because nothing connects the standard to the work |
| **reuse before building** - search first, name the candidates, reuse unless a stated reason blocks it | **§38b of THIS specification** | **it is an estate-wide rule written inside one project's plan.** §38b is itself an instance of the bug he is describing, and that is the clearest evidence this register needs to exist |

**WHAT THE REGISTER ADDS, and this half is what documents cannot do at all:**

| | |
|---|---|
| **a standard carries a VERSION** | so *"perfected with time"* is a fact a project can be measured against, rather than a hope. A project records which version it was built to |
| **rig knows which projects are BEHIND** | *"we can always know when a project should be raised up to those standard"*. **A document cannot know who is reading it**, and a skill cannot know which project ignored it |
| **re-checking is scheduled, not remembered** | *"checked from time to time it still upholds all the requirements"*. **Drift is the normal case**: the standard moves and the project does not, and nothing today notices |
| **one copy improves; N copies diverge** | §38c. This is its second and larger application, and the first one it was not stated for |

**WHERE THE NON-GOAL LINE FALLS, AND IT IS STATED BEFORE THE DESIGN RATHER THAN
DURING IT.** **rig holds the standard, its version, and when each project was
last checked against it. rig does NOT decide whether the work meets it.** The
judging belongs to the project or to the agent doing the work. That leaves §29
non-goal 1 exactly where the continuity record left it: **rig records and
reminds; it has no opinion about whether the work is any good.**

**NOT §19, AND THE NAMES WILL COLLIDE IF NOBODY SAYS SO NOW.** §19 is the
PROTOCOL conformance suite - `rig verify ./program`, a program against rig's
wire contract. **This is a project against a standard.** The two share the word
and nothing else, so whichever keeps "conformance", it must not be both.

**AND THIS MAY OUTGROW §16.** The peers service is coordination between live
sessions, and a standards register is not that. **The subsection stays here
while the capability is one undesigned thing**; the design decides whether it
earns a section of its own.

**AND IT REPLACES THE LOGBOOK. Boris, 2026-09-12:**

> *"This part we were talking about should also replace logbook - perhaps I
> didn't mention it. It's a one stop shop for the project."*

**WHAT IS BEING REPLACED, NAMED RATHER THAN GESTURED AT.** The logbook is a
separate git repository, one tree per project, holding handoffs, the decision
log, the backlog, coordination, dated history, agent work, screenshots and
briefs. A project reaches it through gitignored symlinks. **Every session
commits twice - the project for code, the logbook for the notes - and a session
that commits one has silently lost the other half.**

| What it costs today | |
|---|---|
| **two repositories, every session** | plus the rule that both are committed, which is itself an instruction nothing enforces |
| **a convention document to explain it** | its `CONVENTION.md`, its setup script, and a section of the global instruction file. **All of it is routing prose**, which is the defect §38c names |
| **symlinks a fresh clone does not have** | remade by a script, and invisible until the moment they are missing |

**THREE THINGS GIT GIVES THE LOGBOOK FOR FREE, AND THE RECORD MUST BEAT THEM
RATHER THAN MATCH THEM.** His own bar: parity is failure.

| What git supplies today | What the record owes |
|---|---|
| **history - who changed a requirement, when, and what it said before** | the same answer from an append-only record, and **queryable rather than `git log`-able**, or this is a downgrade |
| **an offsite remote** | the logbook has a private remote and a local database file has none. **The four lost requirements get WORSE, not better, if the store cannot leave the machine** |
| **it works when rig does not** | **§29: *"rig is not required. Every program works without it, at reduced service."*** A session that cannot reach rig must still see the project's record. **This is the hardest of the three and it is not optional** |

**SO THE DESIGN OWES A DEGRADED PATH AND AN EXPORT, and both are requirements
rather than refinements.** What a session sees with rig down, and how the record
leaves the machine, are answered before this replaces anything. **Until they
are, nothing deletes the logbook.**

**THE CUTOVER IS PER PROJECT, NEVER ESTATE-WIDE AT ONCE.** §37's staged
migration rule already governs it: a capability lives in exactly one system at a
time, estate-wide, and **the logbook stays authoritative for a project until
that project's record is live.**

**WHAT MUST DISAPPEAR WHEN IT LANDS, because that is the only measure of "one
stop shop" that can be checked:** the logbook's convention document, its setup
script, the symlinks, the two-repo commit rule, and the section of the global
instruction file that explains all of it. **If those survive, it is not one.**

**AND IT HOLDS THE PROJECT'S DOCUMENTATION, NOT ONLY ITS STATE. Boris,
2026-09-12:**

> *"It holds in the best way possible all the documentation belonging to the
> project. Structured in a very efficient and beneficial way so that documents
> can be indexed, linked and correlated for future reference and maintenance."*

**THIS PROJECT HAS ALREADY HAND-BUILT ALL THREE, AND EACH ONE HAS A MEASURED
FAILURE BEHIND IT.** That is the evidence for specifying them rather than
leaving them to convention:

| | The hand-built version here | What it cost |
|---|---|---|
| **indexed** | `PLAN.md`'s Map | **the first one was hand-written and its line numbers were wrong inside the same session that wrote them.** It is generated now, by a tool, with a `--check` that exits 1 when stale |
| **linked** | **4,206 `§NN` citation edges**, 42 distinct targets, max in-degree 758 (§5). ⛔ **This row said "~370, of the form `PLAN.md section 37`" until 2026-09-16 - 11x low, and wrong about the dominant form: the prose spelling measures 15.** | **they address sections by NUMBER, never by line, because a line address rots.** The convention exists precisely because there are no real links, and it is why splitting 5,218 lines into 38 files needed no citation edited |
| **correlated** | **nothing** | and the failures are counted: **four requirements of his found living only in a volatile document**, and **a struck quotation still sitting verbatim in three unswept files** after the fact it rested on was withdrawn |

**THE MAINTENANCE HALF IS THE PART DOCUMENTS CANNOT DO AT ALL.** When a
requirement changes, nothing today finds the documents that cited it - a seat
greps, and the residue is whatever it did not think to grep for. **"Future
reference and maintenance" is asking for the reverse direction of every link**,
which is free in a record and impossible in a tree of files.

**WHAT "ALL THE DOCUMENTATION" INCLUDES, and it is more than the logbook:** the
specification itself, the decision log, the backlog, coordination, briefs,
audits, screenshots, and the generated index over them. **`tools/plansplit.py`
is another thing that must disappear** - it exists only because a specification
in flat files needs a generated index, and a record does not.

**THE TENSION THIS CREATES, AND THE PATTERN THAT RESOLVES IT.** Some documents
must be readable with no rig at all: a `README` in a clone, a doc a user of the
project needs. **So the record is the SOURCE and files in the repository are
PROJECTIONS of it** - generated, and regenerated when the record changes.

**That pattern is not new here; it is already running.** `PLAN.md` is generated
from `plan/` and hand-editing it is forbidden. **The record generalises what
this project already proved works**, which is a better position to design from
than a blank page, and it is the same answer the degraded path above needs.

## THE VERDICT OF THE SEAT THAT SPECIFIED IT

**Recorded because a specification full of open problems reads as doubt, and
this is not doubt.** Asked directly on 2026-09-12 whether this feature was weak
or worse than the documents, the answer was no, and the reasoning belongs here
rather than in a terminal:

| Why it is rated the strongest item in the backlog | |
|---|---|
| **it is the mechanism fix for the failure that has cost this project four times** | requirements living only in a volatile document. Every other capability makes rig better; this one stops rig's own development from losing what Boris says |
| **two unprompted raises by the adopter, on consecutive days** | the strongest evidence class `BACKLOG.md` admits, and **no other row has it** |
| **its failure mode is a gap, not corruption** | cheap to get wrong and cheap to iterate, which is exactly what the coordination primitives are not |
| **§38b is the case against us, written by us** | an estate-wide rule sitting inside one project's specification because there has never been anywhere else to put it |

**AND THE REGISTER BELOW IS NOT A RISK LIST.** Its twelve entries are the things
that would make this WORSE than the logbook if left unanswered. **None of them
is a research problem**; the three that could genuinely lose to git today - rig
down, offsite, history - are answered by one mechanism, and it is the first
thing the design specifies.

**THE ONE REAL CONCERN IS DELIVERY, NOT THE IDEA.** The scope widened four times
in a single sitting. The ordering ruled the same day handles it: **cut row 1
over first, then design this while it is no longer moving.**

## THE DESIGN

### The spine: rig is the writer, git is the reader of last resort

**One sentence.** The record lives in rig; **rig continuously materialises it
into files inside a git repository**; those files are what a session reads when
rig is down and what leaves the machine.

**THIS ONE MECHANISM CLOSES THE THREE TENSIONS THAT COULD OTHERWISE LOSE TO THE
LOGBOOK**, and it closes them by keeping what git is already good at rather than
by rebuilding it:

| Tension | How the spine answers it |
|---|---|
| **1. rig is down** | the materialised files are complete and readable by a human or an agent with no rig at all, **and a write still lands** - `rig record put` falls back to `records/_pending/` in the same repository and the daemon ingests it on start. **Reduced service as §29 asks**: no live progress, no cross-project query, no links until ingest. **Not read-only - the attack's finding 1 killed that** |
| **2. offsite** | the files are in a git repository that already has a remote. **No backup mechanism is invented**, and the one that exists is the one that has been holding this project's notes all along |
| **3. history** | rig's own store is append-only and answers *"what did this say before"* as a query. **The projection is versioned by git on top of it**, so the weaker answer is still there if the stronger one is ever wrong |

**WHY THIS IS NOT A RETREAT TO FILES.** The files are an OUTPUT. Nothing writes
them by hand, exactly as `PLAN.md` is generated from `plan/` today and
hand-editing it is forbidden. **rig is the only writer**, so the read-before-write
gate, the typed links, the drift detection and the derived human view all work
against the record - and the files are what survives rig being unavailable.

**THE PROJECTION TARGET IS CONFIGURED PER DOCUMENT CLASS, and that is what makes
the cutover reversible.** A project says where each class of document
materialises: product documentation to its normal path in the repo, working
notes to `.rig/` or to a separate repository. **Today's logbook layout - notes
in a second repo, reached by symlinks - is expressible as one configuration**,
which means the migration can be run and un-run without a rewrite, and a project
that wants its notes out of the product repo still gets that.

#### What the projected files actually look like, because that IS the degraded path

**A projection nobody specified is a degraded path nobody can judge.** Two
layers, and they exist for different readers:

| Layer | Shape | Who it is for |
|---|---|---|
| **the lossless layer** | **one file per record**, `records/<kind>/<id>.md`, with the fields, the links and the provenance in front matter and the body below | **rig, and git.** One record changing touches one file, so a diff is readable and a merge is never needed. **This layer is what the store rebuilds from**, so it must lose nothing |
| **the roll-up layer** | **generated documents per kind** - the backlog as one file, the decisions as one file, the specification as its sections, `FEATURES.md` as one file per project | **a human, and an agent with no rig.** They are the documents that exist today, in the shapes they exist in today |

**THE ROLL-UPS ARE WHY THE DEGRADED PATH IS NOT A DOWNGRADE.** With rig down,
what a session opens is `BACKLOG.md`, `DECISIONS.md` and the specification -
**the same files, in the same shapes, with the same git history behind them.**
Nothing new has to be learned at the worst possible moment, which is the moment
rig is unavailable.

**AND THIS IS THE PATTERN THE PROJECT ALREADY RUNS**, for the third time in this
section: `PLAN.md` is generated from `plan/`, hand-editing it is forbidden, and
`--check` exits 1 when it is stale. **The roll-ups are that, generalised - and
`tools/plansplit.py` becomes redundant rather than being ported**, which is one
of the things §39 promised must disappear.

**LOSSLESS IS A REQUIREMENT, AND IT HAS EXACTLY ONE EXCEPTION.** The store
rebuilds from the per-record layer, so a field the projection cannot represent is
a field that does not survive a corrupt store. **The exception is a `sensitive`
payload, which is NEVER projected and is NOT recoverable** - it would otherwise be
committed and pushed to a remote, which destroys the guarantee it exists for.
The kinds that carry one expire in minutes, so a rebuild losing them costs
nothing. **Attack finding 2: lossless and sensitive cannot both be absolute.** **The demonstration is the same round trip
`plansplit.py` already proves: export, rebuild, and compare - and nothing is
written until it matches.**

#### The id scheme and the front matter format, a first pass

**Cheap to decide now and cheap to change before any code exists** - slice 1
cannot start without SOME answer here, and neither of these needs B28's
store search to settle.

| Decision | What, and why |
|---|---|
| **id** | **UUIDv7** (RFC 9562), via `google/uuid` - the most-used UUID library in Go, top-shelf per §38. Time-ordered, so `ls records/<kind>/` sorts chronologically for free, which is exactly the diff-readability goal this section already states. ULID does the same job; UUIDv7 wins for being the standard rather than a de-facto one |
| **filename** | `records/<kind>/<id>.md`, id in canonical lowercase hyphenated form |
| **front matter** | **YAML**, delimited by `---`, block style throughout - `links:` is a block list of `{type, to}` pairs, never `[a, b]` inline. **A one-line array is a diff that hides which entry changed**, the same instinct already behind reformatting `buildskew_test.go`'s table literals |
| **provenance fields** | `session`, `seat`, `created_at`, `epoch` - the epoch is not new, precondition 4 above already requires one on every stamp; provenance carries it rather than inventing a second place for it |
| **roll-up display id** | `BACKLOG.md`'s `B28`-style short codes are DERIVED at render time - the Nth work-item created in the project - never a stored field. **Same reason auto-completion is derived above**: a counter kept in sync by hand is a field that can go stale |

#### `record.changed` is the one internal event, and it is what triggers the other two

⛔ **AND IT IS SLICE 5's, NOT SLICE 1's. RULED BY THE LEAD 2026-09-16 LATE**,
on the record seat's finding, after verifying it rather than relaying it.

**§5's `events` bus DOES NOT EXIST IN CODE** - no `Publish`, no `Subscribe`
anywhere in `internal/` `[ran it]` - and `plan/05` puts the `events` service at
**M4**, which is not built. So slice 1 either silently contained *"build an M4
estate service"* or this event moved.

**THE DECIDING FACT IS NOT THE MISSING BUS. IT IS THAT NOTHING ON THE MVP PATH
SUBSCRIBES.** This subsection names both subscribers and there are only two -
**the projection writer (slice 5) and the gate's re-arm (slice 6)**. The MVP is
slices 1, 2 and 4. **Slice 1 would have built a publisher with no subscriber,
on an unbuilt service, for a consumer three slices away.**

**So `record.changed` and the bus both land at SLICE 5, beside the first thing
that listens.** Slice 1 is `put`/`get`/`query`/`history` and nothing else.
**`SLICE-SIZING.md`'s line that slice 7 rides on "`record.changed` which slice
1 already builds" is thereby FALSE, and slice 7 costs more** - the number moves
rather than the scope.

**The events bus is nobody's in `COORDINATION.md` and is NOT the record seat's**
- it is an estate service, not a record concern.

**Two mechanisms this section already names have never had a stated
trigger.** The projection ("rig continuously materialises it into files",
above) and the gate's re-arm ("a must-read record that CHANGES re-arms the
gate", below) both fire on the same thing - a record was written - and
neither says how. **`record.changed`, on the internal `events` bus §5
already specifies** (id, kind, project, what changed): `record.put`,
`record.link` and `record.unlink` are its only publisher. **The
projection writer and the gate's re-arm check are both subscribers, not
callers `record.put` reaches into directly** - which is the decoupling
§5's new note is about, applied to the first two internal consumers that
actually need it.

#### rig commits and pushes the projection itself

**Boris, 2026-09-12:** *"things can be managed in git too, we can architect so
that certain things can be exported into git managed repository and committed
and pushed"*. **A materialised file that nobody commits is not offsite**, so the
projection is not finished at `write()`.

**TWO PROJECTION TARGETS, AND THEY HAVE OPPOSITE COMMIT RULES. The split is who
owns the repository:**

| Target | Who commits | Why |
|---|---|---|
| **the RECORD repository** - the project's notes, decisions, backlog, progress, briefs | **rig, every time**, with a message derived from what changed, and pushed on a policy | **nobody hand-edits it**, so there is no human commit to interleave with and no conflict to resolve. This is today's logbook with its writer replaced |
| **the PROJECT repository** - documentation a reader of the project needs | **the human, with their code** | exactly how `PLAN.md` works today: rig writes the file, the commit belongs to whoever changed the thing it describes. **A daemon committing into a source repository beside a half-finished feature is a defect, not a service** |

**THIS IS WHAT KILLS "COMMIT BOTH REPOSITORIES", tension 12.** The rule exists
because a session has to remember the second commit and a session that forgets
loses the notes half. **rig does not forget, and the notes half stops being a
session's job at all.**

| The policy, and each line is a knob rather than a decision taken here | |
|---|---|
| **commit granularity** | per change, or coalesced on a timer. Coalescing is the default: one commit per record version turns a busy hour into three hundred commits |
| **push trigger** | on commit, on a timer, or on demand. **A push is outward-facing and can fail**, so it is retried and its failure is surfaced - never silently swallowed, which is how a laptop ends up holding the only copy |
| **credentials** | pushing needs one, and §13's capability model already governs what `rigd` may reach. **A record repository rig cannot push to degrades to local-only and SAYS so** in `project.brief`, rather than looking healthy |

**AND THE DEGRADED PATH IS NOW SYMMETRIC, which is the part worth noticing.**
With rig up, the record is the source and git is the durable projection. **With
rig down, the projection is a complete, committed, pushed git repository that a
human or an agent reads exactly as they read the logbook today** - the same
files, in the same shapes, with the same history commands. **That is §29's
"reduced service" satisfied by something that already exists rather than by a
fallback nobody exercises.**

#### The internal representation is not text, and that is the point

**Boris, 2026-09-12:**

> *"since rig is a binary software, it can make use of more optimized structures
> and systems for indexing and linking and other things that can be done with
> the contenst currently managed inside the logbook; it is not restricted to
> text files; results can be exported into text files and pushed into git."*

**THE PROJECTION IS AN EXPORT, NEVER THE STORE.** Everything above already
depends on this - `record.refs`, drift detection and the brief are all cheap
against a real index and all quadratic against a directory of markdown. **What
the binary buys, stated concretely rather than as "it is faster":**

| | What text files force today | What a store gives |
|---|---|---|
| **finding a requirement** | grep across 5,218 lines, and the answer depends on guessing the phrase | a secondary index on kind and field. **Exact, and it cannot miss a file nobody thought to grep** |
| **the reverse link** | there is none. A seat greps for citations and the residue is what it did not think of | **adjacency held in both directions.** `record.refs` is a lookup, not a scan |
| **searching the prose** | grep again, with no ranking and no stemming | a real inverted index, so *"what did he say about the tray icon"* is a query |
| **history** | `git log -S` over a file that has been split, renamed and regenerated | versions of one record, keyed, with provenance |
| **the same fact in three places** | three copies that drift, which is the measured 2026-09-12 residue | one record, and everything else links to it |

**AND THE TEXT SIDE LOSES NOTHING**, which is why this is not a trade: the
export still lands in git, still has a remote, still reads the same when rig is
down. **The optimised structures are how the record beats the documents; the
export is how it never falls below them.**

#### Tension 13, NEW: bbolt was chosen for coordination and does not answer this

**`BACKLOG.md` B25's library search resolved the COORDINATION primitives - leases,
CAS, a WAL, cursored subscriptions - and `go.etcd.io/bbolt` won it at +355 KB.
That search did not cover the record store**, and the record store wants three
things bbolt has none of natively: **query by field, full-text search, and graph
traversal.** Hand-building all three on top of a key-value store is precisely
what §38b forbids.

| Candidate to weigh, and this list is what to SEARCH rather than a finding | |
|---|---|
| **bbolt plus hand-built indexes** | one engine for the whole daemon, and **§38b's objection applies to the hand-built half** |
| **bbolt plus `bleve`** for the text index | keeps the KV store, buys a real search engine, adds a second dependency and its footprint |
| **SQLite, pure-Go** | query, full-text (FTS5) and relational joins in ONE dependency, which is the shape §38b rewards. Costs a second storage engine inside one daemon |

**HIS LEAN, 2026-09-12: *"Should probably use SQLite for something."*** That is
a lean rather than a ruling, and it lands on the candidate that already looked
strongest: **one dependency covering query, full-text and joins, with the
best-understood migration story of the three** - which the release-durability
requirement below makes weigh more than it did an hour ago. **So B28's job is to
confirm or beat SQLite, not to start from nothing.**

**WIDENED, 2026-09-15: SQLite + `bleve` named as a good-looking combination,
best-of-breed additions welcome.** Boris, reading the axis table below:
*"bleve seems cool... SQLite + bleve seems like a good fit; if you have
additional best-of-breed additions you can add them in."* This is a stronger
steer toward the table's own second candidate row, not a ruling that skips the
search - **B28 still runs per axis and prices combinations exactly as
specified below**, but the search now has two concrete named leads (SQLite
alone, and SQLite + `bleve`) rather than one, and an open invitation to name a
third best-of-breed piece per axis (links/traversal in particular has none
named yet beyond SQL recursive CTEs).

#### "State of the art" is the bar for this choice, and it has axes rather than one answer

**Boris, 2026-09-12:** *"Either SQLite or other/additional great technologies
allowing to be state of the art."* **"Additional" is the operative word: a
combination is permitted, so the search prices combinations and not only single
picks.** This is 38a applied to a dependency - **name the best existing thing
and say how what we would build compares** - and it is why the brief below is
per axis.

| Axis | What it has to do | Candidates to MEASURE, not findings |
|---|---|---|
| **the store** | atomic commit, a schema version, forward migration, and survive a release | pure-Go SQLite; `bbolt`; `badger`. **SQLite's migration story is the best understood of the three**, which the release-durability requirement above makes weigh more |
| **query by field** | find a requirement without reading 5,218 lines | SQL directly; or hand-built secondary indexes over a KV store, **which is the half §38b objects to** |
| **links and traversal** | the reverse edge, and multi-hop *"what does this touch"* | SQL with recursive CTEs at this scale; a graph engine is almost certainly oversized and the search should say so with a number rather than an opinion |
| **full text** | ranked, stemmed search over the prose, not grep | SQLite FTS5; `bleve` |
| **semantic search** | ⛔ **THE WORKED EXAMPLE HERE DID NOT SURVIVE BEING CHECKED.** It read *"what did he say about the tray icon" answered without the word "tray"*. `[ran it]`: **"tray" appears 45 times in `plan/` and FTS5 answers that query at rank 1.** The one piece of evidence offered for the axis argues for FULL TEXT instead. **This is not an argument against semantic search - it is the absence of one.** | **A SEPARATE DECISION, NOT A LIBRARY.** It brings an embedding model, a runtime and a model-versioning problem. ⛔ **REFUSED 2026-09-16 (B28), deliberately rather than by default**, on §29's non-goals and §22's bar. **The ARMED trigger to reopen it: a REAL query, written down, that full text demonstrably failed to answer.** Not a hypothetical one |

**WHAT THE SEARCH MUST PRICE, because a combination is not free:** each added
dependency costs footprint, a §22 bar to clear, and a §13 trust surface. **Two
libraries that each win their axis can still lose to one that wins three of
them**, and that comparison is the actual output of B28 rather than a ranked
list per axis.

**AND THE ONE THING THE SEARCH MAY NOT CONCLUDE: "we will write our own."** §38b
allows that only for footprint, the dependency bar, the trust model or a
non-goal - **and taste is not one of them.** If every candidate is refused, the
refusal names which of the four reasons applied, per axis.

**NO SEARCH HAS BEEN RUN AND THIS SECTION DOES NOT PRETEND OTHERWISE.** §38b's
rule is that *"I did not find one" is only an answer after a search that is
described*, and the same honesty applies to a recommendation: **these are the
candidates to measure, in the manner B25 measured its seven - resolved live,
built, and sized against an empty-main baseline.** The footprint gate is gone,
so the number goes in the commit rather than into a gate.

### The model: records, kinds, links

**Three nouns and no more.** The whole of the indexing, linking and correlating
he asked for falls out of them.

| Noun | What it is |
|---|---|
| **a RECORD** | the atomic unit. An id, a kind, the project it belongs to, a body, typed fields, and provenance - **which session wrote it, when, under which seat**. Append-only: a change writes a new version and the previous one is retained |
| **a KIND** | what the record is, and it carries the schema for the fields. `requirement`, `decision`, `work-item`, `standard`, `note`, `artefact`, `progress`, `feature`, `project`, `case` |
| **a LINK** | a typed, directed edge between two records. `rules-on`, `cites`, `supersedes`, `implements`, `checked-against`, `produced-by`, `blocks`, `part-of` |

**THE THREE PROPERTIES HE ASKED FOR ARE EACH ONE OF THOSE, and none needs a
fourth mechanism:**

- **indexed** is querying records by kind and field. A requirement cannot hide in 5,218 lines because it is not in 5,218 lines; it is a record with a kind
- **linked** is the edge. A decision `rules-on` a requirement; a work item `implements` one; a commit is `produced-by` a session that was working a record
- **correlated** is traversing the edge BACKWARDS, which is the direction files cannot go. *"What cites this requirement"* is the query that would have caught a struck quotation still sitting in three unswept files

**PROVENANCE IS NOT OPTIONAL AND IT IS WHY THE QUOTATION RULE EXISTS.** This
project found two invented quotations and one misattributed requirement in a
single day, and the rule that came out of it - **a quotation attributed to Boris
that cannot be traced to a transcript is a paraphrase until proved otherwise** -
is a provenance check performed by hand. **A record carries its source, so the
check is a field rather than an investigation.**

### Work-item and project metadata, exhaustive

**Boris, 2026-09-15: *"Lets also decide what metadata we keep up to
date for every work-item; be exhaustive, it will be maanged by AI
agents."*** Named exhaustively so an agent never has to guess which
field to fill. **Every one of them is a typed field an agent queries with
`record.query`, never prose buried in a body an agent has to parse** - the
same reason the short/long description split exists and `tags` is its own
field rather than a line in `description_long`. Optimal access for an agent
IS the design constraint, not a nice property of it.

**CORRECTION: `project` is its own KIND, one record per project.** The
fields below were marked "applies to: project" from the day they were
written, which already implied a record to hold them; the KIND list simply
had not caught up. **One exception to the id-scheme table above: a
project's id is its SLUG** (tension 14's `<name>` in
`~/.rig/<scope>/<name>/`), never a UUIDv7 - it is already a path
component and a human types it, so it stays stable and readable. Every
other record's "project it belongs to" field is this same slug.

| Field | Kind | What it is |
|---|---|---|
| `title` | project, work-item | one line |
| `description_short` | project, work-item | one line, for lists and briefs |
| `description_long` | project, work-item | the prose `body` was going to carry - typed and separate rather than one blob |
| `tags` | project, work-item | free-form grouping, queryable like any typed field |
| `status` | project, work-item | `idea` or `active`. `idea` means no progress stream is expected yet - it has not been picked up. Once active, the live state (started/blocked/done) is the latest `progress.step`, not a second field to keep in sync |
| `priority` | work-item, note | as before. Extended to `note` 2026-09-15 - the ordering signal a case's `attention_n` derivation sorts on, below |
| `principles` | project, work-item | optional list; **a parent's principles bind its children** rather than being restated per item |
| `owner` | project, work-item | which seat is currently driving it |
| `target_date` | work-item | optional |
| `source` | project, work-item | which document or session it originated from - distinct from `cites`, which is for rules-on |
| `next_up_n` | project | how many work-items `project.brief` surfaces as "next up." **Default 5**, override per project |

**Sub-task nesting uses the LINK, not a new kind.** A sub-task IS a
work-item, `part-of` its parent.

**Auto-completion is DERIVED, never stamped.** *"When all sub-tasks
are complete, the parent is complete"* is computed at read time from
the children's latest progress state - it is never written as a
`progress.step` on the parent. **Attack finding 4 is the reason:** a
stamp with no witness is asserted, not evidenced, and a system that
writes "done" on itself the moment its own precondition is met is
exactly that stamp. A derivation has no such gap - it is recomputed
every time, never stored, and cannot go stale.

**`blocks` covers ordering; no separate `precedes` link.** A hard
dependency is `blocks`; relative importance within a backlog is
`priority`. A second link for pure sequencing would duplicate one of
the two without covering a case neither already does - it gets added
if that case ever shows up.

**"Next up to `next_up_n`, in expected execution order"** is a
topological sort over the `blocks` graph among `status: active`
work-items, unresolved dependencies excluded, ties broken by
`priority`. **This is the concrete case the graph-traversal axis of
B28 exists for** - a real ordering query, not a hypothetical one -
and it is why the axis is priced rather than skipped even though the
store itself could be decided without it.

### Cases, for what is ongoing and never ships

**Boris, 2026-09-15: *"we have projects but we have also things
that are ongoing things... the way we track my health case or my
mother case or my financial issues, so not everything is a
project."*** A new KIND, `case` - the name he settled on after
`watch`, `situation`, `thread` and `aspect` were weighed and set
aside. A `case` is the same container shape as `project` minus the
two fields that assume shipping.

| Field | Same as `project`? |
|---|---|
| `title`, `description_short`, `description_long`, `tags` | yes |
| `owner`, `source` | yes |
| `next_up_n`, the "next up" derivation | yes - reuses the same topological-sort-over-`blocks`-among-active-items derivation |
| `status` | **NO** - `open` / `resolved`, not `idea` / `active`. A case is not picked up, it already exists; it does not ship, it gets resolved |
| `semver` | **NO** - a case does not ship, so it has no version to advance |
| `feature` / `implements` | **NO** - `FEATURES.md` is a project artefact; a case has no features |

**A `case`'s id is its SLUG, the same exception `project` already
has** - it is a path component and a human types it. This is the
concrete first tenant of tension 14's `me` scope: `~/.rig/me/<name>/`
holds exactly the things his examples name, health and family and
finances, kept out of `~/.rig/work` by the same scope split already
ruled there.

**Boris, 2026-09-15, extending the ruling: *"Each case should keep
the latest N important messages to the user (in this case me).
Those are the last N things I was notified about or the N most
important things I must attend to, default of 10... up to N in all
cases, no need to force creation of N items."*** Reuses the existing
`note` KIND rather than adding one - a note `part-of` a case IS the
message.

| Field | What changes |
|---|---|
| `attention_n` | new field, `case` only. How many notes surface. **Default 10**, override per case |
| `priority` | now applies to `note` too, not only `work-item` - the "most important" half of his requirement, same field, nothing invented |

**The derivation:** up to `attention_n` notes `part-of` this case,
ordered by `priority` descending, tie-broken by `created_at`
descending - reads both halves of his requirement in one sort:
importance leads, recency is the tie-break. **Never padded to N**,
same phrasing as `next_up_n` above. **No new verb** - a note is
written with the same `record.put` every kind uses, and a
case-shaped brief surfaces it exactly as `project.brief` already
surfaces next-up work-items - the same derivation aimed at a
different container.

⛔ **AND THE VERB THAT SERVES IT IS `project.brief` ITSELF, TAKING A
CONTAINER OF KIND `project` OR `case`.** The verb list calls itself
exhaustive at eleven and there is no `case.brief` in it, so this
paragraph assumed a verb that did not exist. **Twelve verbs is not
better than eleven**, and §39 already reuses `note` and `part-of`
rather than inventing kinds per container. **The name stays and is
mildly wrong; the alternative is a second verb whose body is the
first one's.** A `case` differs from a `project` in three fields -
`status` is `open`/`resolved`, and there is no `semver` and no
`feature` - so the brief omits rows 10 and 2's `semver` for a case
and carries row 11 instead.

### Comments, and anything else Boris attaches to a record

**Boris, 2026-09-15: *"The user can assign a comment/question or
basically anything he wants to any work-item or project. It is for
the agents to read and act accordingly; remember everything in `rig`
is to be optimally accessible to AI agents."*** Reuses **`note`
`part-of` the record** - the same two names doing this job for the
third time in this section. No new KIND, no new LINK.

- **Not limited to work-item and project.** `part-of` already links a
  note to any record kind, so a comment on a `case` or a `feature`
  costs nothing extra to support - his two examples are the common
  case, not a restriction written into the schema.
- **Surfaced wherever the target record is, not on request.** A work
  item's or project's notes render alongside it, in full, in
  `project.brief` - the same derivation `case`'s `attention_n` above
  already draws on. **A case caps and orders what it surfaces; every
  other kind just shows all of them**, since only the case's stream
  is unbounded enough to need a cap.
- **No status field, no resolution workflow.** "Act accordingly" is
  left to the agent reading it - inventing an open/answered state
  here would be a mechanism nobody asked for. Provenance already
  says who wrote it (Boris's session, distinct from an agent's),
  which is enough to tell a directive from routine progress narration.

### Features, and the FEATURES.md roll-up

**Boris, 2026-09-15: *"a dedicated list of features that lists
everything the project contains in highlights with some
description; each feature can also be in different stages of
development."*** A new KIND, `feature`, rather than a repurposed
`work-item` - a feature is a capability the project HAS, at a
coarser grain than any single work-item, and it outlives the items
that built it.

| Field | What it is |
|---|---|
| `title`, `description_short`, `description_long`, `tags` | same shape as project/work-item, for the same reason - typed, queryable, no prose to parse |
| `stage` | `planned`, `building`, `shipped`, `deprecated` |

**A work-item `implements` a feature.** The existing link, reused -
today it points a work-item at a requirement; a feature is the same
relationship, "what this work is FOR," one level coarser.

**`FEATURES.md` is a roll-up, generated, never hand-edited** - the
same pattern as `BACKLOG.md` and `DECISIONS.md`: one row per
feature, its stage, its description, what implements it. **This IS
the "everything the project contains in highlights" list** - a
human or an agent with no rig reads it exactly as they read the
backlog today.

### The version every project and work-item carries

**Boris, 2026-09-15: *"All projects and work-items have a version
like x.y.z and it should be advanced a major, minor, patch according
to the development progress."*** A `semver` field, string
`MAJOR.MINOR.PATCH`, on `project` and `work-item` - named `semver`
rather than reusing "version," which already means the record's own
revision number in `record.history`. A new record starts at `0.1.0`.

| Bump | Fires on | Judgment? |
|---|---|---|
| **PATCH** | any `progress.step` recorded against the item | none - mechanical, "development happened" |
| **MINOR** | the item's own completion: a work-item's terminal `progress.step: done`, or - for a project - one of its `feature`s reaching `stage: shipped` | none - a derived fact about state, not a quality call. **Reuses the auto-completion derivation already defined above rather than a second mechanism** |
| **MAJOR** | never mechanical | **requires a witness** - a link to the `decision` record that names it as the reason, the same rule `standard.stamp` already has. Attack finding 4 applies here exactly as it does there: a bump with no witness is asserted, not evidenced, and self-certifying "this was major" is the decorative stamp the project has already had to take back once |

### The verbs, and there are eleven

**Each is a call on `rigd`.**

⛔ **THIS PARAGRAPH USED TO SAY §5's SINGLE DECLARATION PROJECTS EVERY ONE OF
THEM TO THE CLI, MCP AND THE WINDOW "WITHOUT BEING WRITTEN THREE TIMES". THAT
IS FALSE FOR rig's OWN VERBS AND IT SIZED THIS SECTION WRONG.** Measured
2026-09-16 by the lead, `[ran it]`:

| Surface | What it actually does |
|---|---|
| the CLI | `cmd/rig/main.go` is a **hand-written switch** - ping, apps, down, estate, peers, describe, mcp |
| the MCP door | **eight hand-written `AddTool` calls**, including announce, set_activity and list_agents |
| the projected path | `toolFor` over **PROMOTIONS**, and a promotion is a **PROGRAM's** command |

**The projection is real and it serves PROGRAMS. rig's own verbs are outside
it by design** - `registry.self` is deliberately not an entry in `programs`,
and holding it beside the registry is the mechanism that keeps `rig.down`,
declared destructive, off every invoke surface. **So the eleven verbs are
written more than once, and any estimate that assumed otherwise is low.**

| Verb | What it does |
|---|---|
| `record.put` | create, or supersede an existing record. Returns the new version |
| `record.get` | one record, at head or at a named version |
| `record.query` | by kind, field and project. **This is "indexed"** |
| `record.link` / `unlink` | a typed edge between two records. **This is "linked"** |
| `record.refs` | **what points AT this record.** The reverse direction, and **this is "correlated"** |
| `record.history` | every version of one record with its provenance |
| `progress.step` | append one step to a work item's stream |
| `standard.stamp` | record that a project was checked against a standard, by whom, when |
| `standard.drift` | every project behind the standard it claims to uphold |
| `project.brief` | **the derived answer to "what is going on here"** |
| `project.gate` | what this session must read before it may write, and whether it has |

**ON THE CLI: `rig record`, `rig progress`, `rig standard`, `rig brief`.** Four
commands, because a surface an agent has to learn is a surface it gets wrong.

### ⛔ THE WIRE SURFACE FOR THE RECORD VERBS. RULED 2026-09-16 LATE, ALL MEASURED.

**Written before a line of it is built, because the last estimate of this
section assumed a projection mechanism that does not reach rig's own verbs.**

#### The method name is `rig.` + the verb name, verbatim. PROVED, not assumed.

`splitMethod` takes the **first** dot (`internal/daemon/daemon.go:1021`), so the
command half may contain more. **`badID` refuses a dot only on the PROGRAM id**,
and its stated reason is exactly that split. **Run, not read** - a throwaway
probe declared four dotted commands through the real `DeclareSelf` path and
they were accepted:

    rig.record.put      -> program="rig"  command="record.put"
    rig.progress.step   -> program="rig"  command="progress.step"
    rig.project.brief   -> program="rig"  command="project.brief"

⛔ **THIS IS WORTH MORE THAN CONVENIENCE: IT AVOIDS REPEATING A RECORDED
INJURY.** The door says `announce`/`set_activity`/`list_agents` while the wire
says `announce`/`activity`/`peers` - **two vocabularies, three calls,
disagreeing on two**, and renaming was offered and not chosen. **The record
verbs carry ONE spelling across the wire, the CLI and the door.** Nothing to map,
nothing to drift.

#### ⛔ THE REQUEST MESSAGES CARRY NO PROVENANCE FIELDS AT ALL

**`Session`, `Seat` and `Epoch` are the DAEMON's and are absent from every
request message** - not optional, not ignored, absent.

**Found by the `cli` seat against the lead's own interface**, which had mirrored
the package's `PutRequest` into a CLIENT interface. **The package struct is the
daemon-to-store boundary, where provenance is already known; mirroring it
outward hands the caller three fields it must never set.** The seat's word for
it is the right one: **a forgery surface.** A client able to set its own session
and seat can write a record attributed to another seat.

**It is enforced by the WIRE rather than by CLI convention**, because a
convention the wire does not carry is one the first non-CLI caller breaks. The
connection already knows all three and `newPrincipal` mints them.

#### `progress.step`'s state is an enum with an explicit UNSPECIFIED zero

`started | blocked | done`, and **§21's rule plus the `noenumzero` analyzer make
the zero mandatory**: an enum whose zero means something makes an unset field
decode as a decision. A bad value is **refused by name with the value quoted
back**, which the package already does - the wire surfaces that message rather
than pre-validating and inventing a second one. **Two validators drift; one does
not.**

#### The MCP surface is ONE sibling tool, and it describes itself

Ruled on §9's context budget - nine record tools on every seat's list forever is
the failure §9 exists to prevent. ⛔ **It addresses the RECORD verbs only, never
the whole of `registry.self`**, or it becomes the invoke surface `rig.down` was
deliberately kept off. **And it owes its own discovery**: `[ran it]`, neither
`list` nor `describe` can see rig's own commands - `list` reads `CapabilityMap`
which excludes rig, and `describe` on `rig` hits the `SelfID` refusal. So the
sibling must describe itself or it is a door with nine rooms and no map.

#### ⛔ `record.link` AND `record.unlink` MOVE FORWARD TO SLICE 2

**The build order inverted its own dependency and nobody had noticed.**
Execution order is defined in the PROJECT half as *"a topological sort over
`blocks`, among items whose `status` is `active` AND whose latest
`progress.step` is not `done`"*. **`project.brief` is slice 2. `blocks` edges
are slice 4.** So slice 2 was specified to derive over a graph slice 4 creates,
and the order ran 2 then 4 - **slice 2's own demonstration could not produce a
next-up list, a blocked list or a cycle report**, because nothing could make the
edges all three read.

**`link` and `unlink` land WITH slice 2. `record.refs` STAYS at slice 4** -
writing an edge and traversing the graph are different capabilities, and refs
carries the depth bound, the cycle report and the truncation flag, none of which
the brief needs. **The MVP scope does not change: slice 4 was already on the
path. Only the order moves.**

**And the alternative was refused for a reason worth keeping: WHO CHOOSES THE
EDGE.** A `part-of` edge from a step to its item is chosen by nobody - it is
what `progress.step` MEANS - so the package writes it internally and that is
not slice 4 leaking. **A `blocks` edge is a caller's assertion about the work**,
which is precisely slice 4's capability, and writing it from inside the package
would have put the acceptance demonstration on a path no caller has.

#### ⛔ THE SET THAT GOES ON THE WIRE IS EIGHT ROWS AND NINE VERBS, NOT ELEVEN

**Ruled 2026-09-16 late by the lead, and recorded here rather than in
`DECISIONS.md` because it is a scope rule about what the table above MEANS.**
It had lived in one session's briefing until the handover that folded it in.

| Excluded | Why it is not on the wire yet |
|---|---|
| `standard.stamp`, `standard.drift` | slice 7 |
| `project.gate` | slice 6 |

**The reason, and it is the whole of it: a declared-and-unimplemented verb is a
WIRE THAT LIES.** A caller that can see a method assumes it works, and §21's
extensibility makes the later additive change cheap - so the cost of waiting is
lower than the cost of a reader believing a verb exists. This binds the proto,
the `self.go` declarations and the sibling MCP tool alike.

⛔ **THE NUMBER IS RECORDED BECAUSE IT WAS GOT WRONG TWICE, BOTH TIMES BY
ARITHMETIC RATHER THAN BY JUDGEMENT.** The lead said "four off-path" when it is
three, and a predecessor certified "seven verbs" off that figure before it was
caught. **Eleven rows, minus three excluded, is EIGHT rows; the `record.link` /
`unlink` row carries TWO verbs, so it is NINE verbs.** Nobody needs to re-derive
this.

#### ⛔ EVERY RECORD VERB OWES A `kernel.Command` IN `self.go`, AND THE MISS IS CHEAP TO MAKE

**`internal/daemon/self.go` declares EIGHT commands today** - `ping`, `estate`,
`programs`, `announce`, `activity`, `peers`, `session` and `down`. `[ran it]`.

⛔ **SEVEN OF THE EIGHT ARE INVISIBLE TO THE OBVIOUS GREP.** They are built by a
`readOnly(id, title, summary, description, returns)` helper at `self.go:17`, so
only `down` carries a literal `ID:` field. **A seat searching for `ID: "` finds
one declaration and concludes rig declares almost nothing** - measured, it
happened during the handover that wrote this paragraph, and it was heading for a
wire shipped with nine undeclared methods.

**WHY THAT WOULD NOT HAVE BEEN A GAP BUT A SELF-INFLICTED OUTAGE**, in
`DeclareSelf`'s own terms: rig's methods are otherwise *"the one surface that
never reaches the rules table, and an unresolvable ref would otherwise resolve
to `EffectsCeiling`"* - **so ANY RULE AT ALL would stop rig answering.** An
undeclared verb is not a missing row, it is rig refusing itself the moment a
house rule exists.

| Verb | Effects | Note |
|---|---|---|
| `record.get`, `record.query`, `record.history`, `project.brief` | `EffectsReadOnly` | the `readOnly` helper covers them whole |
| `record.put`, `record.link`, `record.unlink`, `progress.step` | ⛔ **NOT read-only** | they need real values, and **`Idempotent` is mandatory with no safe default** - `missing()` refuses the declaration without it |

#### ⛔ `project.brief` CARRIES ITS DELIVERY MARK IN SLICE 2, AND IT IS STILL `EffectsReadOnly`

**This resolves the one sentence in this section that specified a build and a
rewrite at the same time.** The brief's row 6 says calling it MARKS the must-read
set delivered to this session, and warns that *"a builder implementing the brief
as a pure derivation in slice 2 rewrites it in slice 6"*. **Two leads have now
read that sentence in opposite directions**, so it is ruled here.

**RULED: the slice-2 brief RETURNS the must-read set and MARKS it delivered.**
Not pure. The mark is per-session state and costs one map write; **omitting it
buys nothing and puts a known rewrite on the MVP's own verb.**

⛔ **AND THE OBJECTION THAT ALMOST CARRIED IT IS ANSWERED BY rig's OWN
PRECEDENT, WHICH IS WHY THE RULING WENT THIS WAY.** The argument against was
that a marking brief cannot declare `EffectsReadOnly`, so slice 6 would change a
DECLARED PROPERTY of a shipped verb rather than only its body. **Measured, that
is false:** `announce` mutates the estate's roster on every call and is declared
through the `readOnly` helper (`self.go:81`), and `activity` replaces a
connection's activity line and is declared the same way (`self.go:85`).

**`Effects` is what a command does to THE WORLD** (`declaration.go:135`), and
the ladder's next rung is `EffectsWritesFiles`. **Per-session delivery state is
connection state, exactly like presence** - §39 already requires it be tracked
*"per session, never per uid"*, so it never reaches the record store and never
touches a file. **`EffectsReadOnly` is honest for it, today and at slice 6.**

**What the publisher-with-no-subscriber argument really bounds.** `record.changed`
was held to slice 5 because nothing on the MVP path SUBSCRIBES to it, and that
reasoning was offered here too. **It does not transfer: `record.changed` costs a
BUS, and the delivery mark costs a map write on state the daemon already holds
per connection.** The shapes are not the same size and the rule that fits one
does not fit the other.

### The read-before-write gate, which is where the prose layer actually dies

**Today the instruction is *"read `COORDINATION.md` in full before your first
write"*, and nothing checks.** An instruction is obeyed at the reader's
discretion; that is the whole defect §38c names.

| The gate | |
|---|---|
| **a project declares a must-read set** | records of any kind, marked. It is data, not a document listing documents |
| **rig tracks which of them THIS SESSION has fetched** | per session, never per uid. **A fresh context is a fresh session and reads again** - which is correct, because the context that read it is gone |
| **`record.put` REFUSES until it has, and names what is missing** | never *"you must read the docs"*. The refusal is a list |
| **ONE CALL SATISFIES IT** | `project.brief` returns the must-read set. **Complying is cheaper than arguing with it**, which is the only reason a gate like this survives contact with a working agent |
| **a must-read record that CHANGES re-arms the gate** | for every session that read the old version. This is drift detection pointed at the project's own rules, and it is free once drift exists for standards. **Triggered by `record.changed` on the internal bus, above** - the gate does not poll |

**WHAT THE GATE ACTUALLY GUARANTEES, stated narrowly after the attack.** **rig
guarantees the material was DELIVERED to this session and that the session
acknowledged it. Comprehension is not enforceable and this section must not
imply it.** That is still strictly more than a document, which guarantees
neither - and it is the demonstration the acceptance bar requires.

**AND THE GATE NEVER BLOCKS `progress.step`.** Its own failure mode is silence,
which is the thing the record exists to prevent: a refused write can simply
become no write. **Progress is always accepted; only substantive records are
gated.** Attack finding 7.

### Progress is a stream, and nothing composes a report

| | |
|---|---|
| **a step is** | the work item, one line of what, a state (`started`, `blocked`, `done`), and optionally a link to evidence |
| **written at a boundary DURING the work** | never at the end, and **never in a termination handler** - §16.6 already establishes why: the handler does not run on `kill -9`, on an out-of-budget stop, or on the container going away |
| **the human view is DERIVED from the stream** | this is the second consumer, and it is satisfied structurally rather than by asking a seat to also write a summary |
| **an absent stream reads as UNKNOWN** | never as *"nothing happened"*. §16.6's rule, reused rather than restated |

### The standards register, and what rig refuses to do with it

| | |
|---|---|
| **a standard is a record**, estate-scoped rather than project-scoped, carrying a version | so *"perfected over time"* is a number |
| **a project `upholds` a standard**, and the link carries `checked_version` and `checked_at` | the stamp is on the relationship, which is why it can go stale without either end changing |
| **drift is `standard.version > link.checked_version`** | `standard.drift` lists every project behind. **A document cannot know who is reading it; this is a query** |
| **a standard may carry a `recheck_after` interval** | *"checked from time to time"*. Due checks surface in `project.brief` and on the window |
| **rig SURFACES a due check. It never runs one, and it never judges the work** | §29 non-goal 1 exactly where §39 left it. `standard.stamp` records **who** checked and **when**; the judging belongs to them |
| **A STAMP CARRIES A WITNESS OR IT IS NOT A STAMP** | a link to the artefact, the command output, or the record showing the check ran. One without is recorded as **asserted** and renders differently from **evidenced**. **Attack finding 4:** `BACKLOG.md` B26 measured what happens otherwise - *a control whose every firing is answered by overriding it is measuring nothing* - and that gate was dropped for it |
| **and the redaction invariant is §15's, inherited rather than re-argued** | what `secrets.get` returned is never recorded, only the key name. **Attack finding 6: auto-push makes an unredacted record a PUBLISHED mistake, not a local one** |
| **a shared default guideline set is a standard, not a new mechanism** | *"a shared default guideline/rule/resource collection every project points to"* is exactly what `upholds` already models. A project's default set is simply the standards it upholds; nothing new is built for this |

### Extending the schema without becoming N copies again

**Kinds and their field schemas are themselves records in the estate register**,
so the schema is versioned and improved in one place like everything else.

| | |
|---|---|
| **a project may ADD fields to a kind** | never redefine or remove one. An addition cannot break another project's query |
| **a project may define a LOCAL kind, namespaced to it** | because refusing outright is how a project ends up keeping a private document instead - the exact failure being fixed |
| **a local kind cannot be linked to from another project** | which contains the divergence and makes it visible. **A local kind that two projects want is a proposal to the estate register**, and that promotion is the mechanism §38c asks for |

### What the window renders, and why it needs no separate design

**It renders `project.brief`.** The same call an arriving agent makes.

⛔ **THIS TABLE CARRIED SIX ROWS AND THE BRIEF HAS ELEVEN SECTIONS.** Five were
specified elsewhere in this section and appeared in no list, so **a builder
working from the table alone shipped something wrong in six ways.** Found by
`backend-record` 2026-09-16, assembling the first consolidated field list from
**fourteen separate specifying sites** `[ran it]`. Corrected here.

| The brief carries | |
|---|---|
| **1. open work items NOT in the next-up set**, each with its last progress step and its age | a stale step beside a live session is the signal that a seat is stuck. ⛔ **THE TWO LISTS ARE DISJOINT BY CONSTRUCTION**, and this sentence is the fix for a contradiction: rows 1 and 3 gave the SAME item two incompatible renderings, because notes are *"rendered in full"* for an item this list carries and *"only whether one is attached"* on a compact card. Disjoint lists give every item exactly one rendering and drop neither rule |
| **2. the next up to `next_up_n` work-items, in execution order** | title, `description_short`, `priority`, `status`, `owner`, `tags`, `target_date`, **`semver`**, and a flag for whether a note is attached - **the compact card, never `description_long`.** A list meant to be scanned in one pass fails its own readability requirement the moment it carries a paragraph per row; the long form is one `record.get` away. **`semver` is on it because Boris's word for the metadata was "exhaustive" and the brief is where metadata is seen**; it is one short string answering *"how far along is this"*, which is the brief's whole job |
| ⛔ **"EXECUTION ORDER" IS DEFINED HERE**, not in the `case` section | **a topological sort over `blocks`, among items whose `status` is `active` AND whose latest `progress.step` is not `done`.** Both halves are corrections. The definition previously existed ONLY as a back-reference inside the `case` row, so **a builder reading the project half end to end never learned what the phrase meant.** And *"among active items"* alone is ambiguous in this schema: §39 is explicit that completion lives in the progress stream rather than in `status`, so `status == active` on its own **keeps a finished item in next-up forever and ships a list that never empties** |
| **3. any notes `part-of` the project itself, or `part-of` a work-item in list 1** | rendered in full, never summarised - **this is Boris's comment/question mechanism above**, and an agent that skips it has not read the item. The next-up compact card carries only whether one is attached; the note's own text is one `record.refs` away, same rule as `description_long` |
| **4. what is blocked, and on whom** | including what is waiting on Boris, **and any `blocks` CYCLE, naming its items** - see the cycle ruling above |
| **5. standards drift, and any check now due** | |
| **6. the must-read set and whether this session has cleared it** | ⛔ **THIS SECTION MUTATES STATE AND THE TABLE PRESENTED ALL ELEVEN IDENTICALLY.** The gate's rule is *"ONE CALL SATISFIES IT - `project.brief` returns the must-read set"*, so **calling the brief MARKS THE SET DELIVERED to this session.** It is the only section with a side effect. **A builder implementing the brief as a pure derivation in slice 2 rewrites it in slice 6**; saying so costs a sentence now |
| **7. the projection is BEHIND, and by how far** | **THE HEALTH BLOCK, rows 7-9, and none of them was in this table.** Each exists so a degraded state is VISIBLE rather than silent |
| **8. pending entries, visible AS pending** | attack finding 1 |
| **9. the record repository is LOCAL-ONLY because a push failed** | §39's own words: *"this is how a laptop ends up holding the only copy."* ⛔ **A brief built from the old six-row table reported a healthy-looking project that was not backed up** |
| **10. features** | features at `stage: building`, plus counts per stage. **This row resolves a CONTRADICTION 20 lines wide:** the GUI overview paragraph says the brief carries *"next-up work-items, **features**, drift, gate status"* and this table had no features row. **The paragraph is the side written against Boris's actual request** - `feature` is a kind he asked for by name with a `stage` field - **and a brief without features cannot drive the overview that paragraph specifies** |
| **11. a case's `attention_n` notes** | priority desc, then `created_at` desc. See the cases section |

#### WHICH OF THE ELEVEN SECTIONS EACH VIEW CARRIES

⛔ **THE TWO VIEWS WERE NAMED AND NEVER SPECIFIED.** Attack finding 9 rules one
derivation, two views, *"and the caller says which"* - and nothing said which
sections differ. **Unanswerable from §39 until now: is the must-read set in the
human view? Is the health block in both?**

| Section | Human | Agent |
|---|---|---|
| 1, 2, 4 - open, next-up, blocked | yes | yes |
| 3 - notes | yes | **yes** - finding 9: an agent that skips them has not read the item |
| 5 - standards drift | yes | yes |
| **6 - must-read set** | **no** - it is not a decision he makes | **yes, it IS the gate** |
| **7, 8, 9 - the health block** | **yes** - these are his decisions to make | as a FLAG only |
| 10 - features | yes | yes |
| 11 - case notes | yes | yes |

⛔ **THIS TABLE IS DERIVED FROM FINDING 9's REASONING, NOT READ OUT OF §39**, and
it is the one part of the brief's specification that was proposed rather than
recovered. **It is written down so it can be argued with**, which an absence
cannot be.

**ONE DERIVATION, TWO VIEWS, AND THE CALLER SAYS WHICH.** The derivation is
shared - that is what makes the human view free - but **he wants *is this going
well and what must I decide* and an arriving agent wants *what must I read, what
is claimed, what is decided*.** Attack finding 9: one rendering for both is
mediocre for each. That is the two-consumer requirement met by
construction rather than by discipline, and it is what makes *"the human-report
can be derived automatically or with very small agent effort"* true: the effort
is one call, and no agent writes prose over it.

**Boris, 2026-09-15: *"rig is to expose in the GUI a section that
overviews all managed projects that exposes all of the different
relevant sections in a beautiful and handy way."*** **No new verb.**
The roster is `record.query(kind: project)`; the content per project
is the same `project.brief` this section already specifies -
next-up work-items, features, drift, gate status. The GUI composes
one call per project into an overview pane; §39 supplies the data,
§11 renders it through the Generated pane tier (a schema, not a
hand-drawn widget) since rig is a program in its own window like any
other.

### The four that were not answered by the spine

**Each is one of the register's entries below, answered here so the entry can
close rather than be tolerated.**

#### Tension 5 - the word "conformance" stays with §19, and the register never uses it

**§19 is shipped vocabulary**: `rig verify ./program`, `pkg/rigtest`, a program
against rig's wire contract. **It keeps the word.** A project measured against a
standard is *upholding* it: `upholds` is the link, `standard.stamp` and
`standard.drift` are the verbs, `rig standard` is the command. **"Conformance"
is a reserved word in this specification and the standards register may not
borrow it**, which costs nothing now and prevents two subsystems answering to
one noun later.

#### Tension 6 - a continuation slot is a KIND OF RECORD, not a second mechanism

**The boundary was the open question and the answer removes a mechanism instead
of drawing a line through one.** §16's slot is *"the versioned blackboard with a
scope of one"*; the record is a versioned store with links and provenance.
**They are the same thing at different lifetimes.**

| A slot becomes | |
|---|---|
| **`kind: continuation`** | one more kind in the same register, so `rig continue` lists records and nothing new is stored |
| **with a TTL** | which is the only property that made it a separate idea. Two-step expiry, visible before it fires, exactly as §16 specifies |
| **with a `sensitive` payload** | §16's isolation answer is unchanged: existence is estate-readable, contents are not, and the listing is scoped to the uid so a respawned agent can still find its own |
| **and the rule that follows for free** | **anything in a slot that survives its TTL should have been a record.** A slot carries what a successor needs to CONTINUE; a record carries what the project needs to KNOW. Claiming a slot returns `project.brief` beside it, so the successor gets both without being told to ask |

#### Tension 11 - artefacts are records with their bytes in the projection

**Screenshots, briefs, audits and agent work are in the logbook today and they
are files.** A record of `kind: artefact` carries the metadata, the provenance
and the links; **the bytes live in the record repository as ordinary files**,
where git already versions them and the remote already carries them offsite.

**rig does not become a blob store**, and that is a refusal with a reason rather
than a gap: the projection is already a git repository, git is already good at
this, and §38b says do not rebuild what exists. **Anything too large for git was
never going in the logbook either**, so the honest boundary is unchanged by this
design.

#### Tension 14, NEW 2026-09-15: where the record lives on disk, and how it is organized

**Boris, 2026-09-15:** *"The textual representation should probably be under
the home folder of rig, probably under `~/.rig` right? ... I want the
filesystem structure to be top of the art, best organized per scope, per
subject, remember that like with logbook it will manage a lot of different
project/works."* And, on the same artefact question tension 11 already
answers the WHAT of: *"The rig will also hold artifacts from the different
stages of the different projects/works so the filesystem structure must also
support these and the rig system should also know how to index them for easy
access."*

**RULED: the root is `~/.rig`.** One home, many projects - centrally rather
than one record repository per project, the same way `logbook/projects/<name>/`
already gives every project one home under one tree.

**RULED 2026-09-15: `~/.rig` is NOT one repository - it is split by SCOPE, and
each scope is its OWN git repository.** Boris: *"`~/.rig/projects/<scope>/<name>/`
... sounds like a good direction. The two must be kept under different git
repositories so that my personal life is not mixed with my work life; in fact,
we may want to add additional categories so it's not a closed group of only
{work, me} - there could be others in the future."* This CORRECTS the
"mirrors the logbook's own shape" framing above: the logbook itself is one
repository for every project, but `~/.rig` cannot be, because a single history
would put personal and professional material in one place Boris has
explicitly refused elsewhere - `~/me/d2d` is already carved out of the logbook
migration for exactly this reason ("its own private remote... do not migrate
it"). `~/.rig` generalizes that precedent rather than breaking new ground.

**The scope set is OPEN, not `{work, me}` closed.** Two scopes are named so
far - `work` and `me` - but the set is extensible; the design must not bake in
an enum of two. **RULED, same message: rig is to support both logbook
functionality and d2d functionality**, with what that means left for further
discussion - *"we shall discuss further what that means"* - so this is a
direction, not yet a specification of what a `me`-scope repository must do
beyond hosting personal records the way the logbook hosts project records.

**OPEN, for the design slice: the exact per-scope/per-subject tree inside each
scope's repository, and how a per-scope repository set is discovered,
addressed and indexed as one whole from the outside** (a link or a query that
crosses scopes has to resolve two - eventually more - repositories rather than
one). Tension 11 already answers WHAT an artefact is (`kind: artefact`, bytes
as ordinary files in the projection, git-versioned); this tension is about
WHERE those files sit relative to a project's other records so artefacts from
every stage of every project stay indexed and reachable, not about reopening
what an artefact is. **"Top of the art, best organized" is a design bar, not a
design** - it lands in the slice that designs the lossless-layer file tree
(`records/<kind>/<id>.md`, laid out under
`~/.rig/projects/<scope>/<name>/`), and it must show its structure against the
logbook's own layout, demonstrated rather than argued, exactly as his
acceptance bar already asks everywhere else in this document.

#### Tension 7 - it sits BESIDE §37's minimum set. RULED 2026-09-15

**Boris, 2026-09-15, choosing the recommendation put to him: "do as
recommended."** The record does NOT join the minimum beneficial set. §37's
rows are coordination primitives measured as *"the best inter-agent
coordination substrate we can build"*; **§39 has its own bar - significantly
superior to the documents - and its own twelve-entry register.** Folding it in
would have moved a gate Boris narrowed on 2026-09-12 and blurred two
acceptance tests into one.

**What does NOT change either way:** the record brings its OWN durable storage.
**Tension 13 establishes that the coordination store** (`bbolt`, chosen by B25
for leases and CAS) **does not answer the record's axes**, so the record was
never going to inherit storage from §37's set. **The record's only storage
dependency is B28** - a dependency on a DECISION, not on a row. Row 3's position
in §37 is decided on §37's own grounds and §39 does not constrain it.

⛔ **THIS PARAGRAPH USED TO PIN THE RECORD BEHIND ROW 3 AND IT WAS WRONG IN
THREE WAYS AT ONCE.** It read *"the record depends on durable storage, so row 3
keeps its position in the set and the record follows it."* Written 2026-09-15,
when row 3 was the generic versioned blackboard. **Boris narrowed row 3 to the
seat claim on 2026-09-16 morning, and rows 2-5 were cancelled outright that
evening. The sentence outlived both** - and tension 13 had contradicted it from
the day the two were written.

**IT IS RECORDED RATHER THAN QUIETLY DELETED BECAUSE IT IS §39's OWN CASE MADE
AGAINST §39's OWN SPECIFICATION.** A `record.refs` on the row-3 dependency would
have listed this paragraph the moment row 3 changed, and the gate's re-arm would
have fired. **The capability this section specifies is the one whose absence
produced the defect**, found in the document that specifies it. It is also
`COORDINATION.md` rule 8's class - a fact that was true with no way to announce
it had stopped being - and the first instance found inside `plan/`.

**This closes the "row C" ordering question `00-rig-kickoff.md`'s Step 0 put
to §37 too**: with the record beside the set rather than in it, there is no
row-C cutover-order slot to argue about. §37's own minimum-set ordering is
unaffected by this ruling.

### The store survives the release, and it is a requirement rather than a hope

**Boris, 2026-09-12:** *"Storage should not reset with each new release
(obviously)."* **It is written down BECAUSE it is obvious** - an obvious
requirement that nobody states is the kind this project has lost four times.

| | |
|---|---|
| **the store carries a SCHEMA VERSION**, stamped in it | separate from §21's wire version. A daemon and a store version independently, because they move for different reasons |
| **migrations are forward-only and run at START** | which §18 already made the natural moment: **rig does not hot-upgrade itself - notice, restart, resume.** A migration is one of the things the restart is for |
| **a migration runs once and is idempotent** | a half-applied migration that reruns must converge, because the way this actually fails is a crash partway |
| **an UNKNOWN (newer) schema version REFUSES TO START** | it does not guess and it does not repair. **A downgrade that silently opens a newer store is how data is destroyed by a rollback**, which is the one failure a rollback exists to avoid |
| **the pre-migration backup is free and already specified** | the projection is a committed, pushed git repository. **Migrate, and if it fails, rebuild the store from the export.** The recovery path pays for itself a second time |

**THE DEMONSTRATION, and it is one of §37's four clauses rather than a test:**
a store is filled with records, the daemon is upgraded across a release that
changes the schema, and **every record, link, version and provenance field is
read back.** Then the same store is opened by the OLD binary and refuses.

#### AND THIS RE-ARMS TWO OF §37'S DEFERRED PRECONDITIONS

**Part A items 2 and 4 were deferred on 2026-09-12, not cancelled, with a stated
trigger: *"they re-arm at the first capability that persists anything."*** **§39
is that capability.**

| Precondition | Why it fires now |
|---|---|
| **2 - state keyed per estate, not per uid** | the record is per project and per estate. **Two estates sharing one uid must not see one another's records**, and presence never needed this because it holds nothing |
| **4 - epoch bumped on every daemon start** | a record carries provenance stamped by the daemon. **An epoch is what makes a stamp from before a restart distinguishable from one after it** |

**SO THE GATE'S NARROWING WAS CORRECT AND IS NOW SPENT.** It was narrowed
because presence holds no state; **the record holds all of it, and the two
deferred items are preconditions of §39 rather than of the cutover.**
`READINESS.txt` carries the 1-2 seat-days they were always owed.

### Failure semantics, because §37's four-clause pass demands them demonstrated

**Every one of these is a demonstration owed before promotion, not a paragraph.**
§16.6's rule governs the whole set: **a half-written record is worse than no
record, because it reads as a record.**

| What fails | What happens, and what the caller is told |
|---|---|
| **two sessions write the same record** | **compare-and-swap on the version**, exactly as the blackboard does. A `put` naming a stale version fails and **returns the current one** so the caller can merge rather than guess. **One concurrency model in the daemon, not two** - a second model is how §16's lease and claim policies ended up opposite |
| **`rigd` dies mid-write** | the store's transaction is the guarantee: the record is committed whole or not at all. **Tension 13 is partly this** - whichever store is chosen must give atomic commit, and both candidates do |
| **the projection write fails** | **the record is already committed; the store is authoritative.** The projection retries, and `project.brief` reports it as BEHIND with how far. **A stale projection is visible, never silent** |
| **the push fails** | same shape. The repository is local-only, and that is a reported state rather than a healthy-looking one. **This is how a laptop ends up holding the only copy**, and it is the failure the whole offsite answer exists to prevent |
| **the store is corrupt or lost** | **rebuild it from the projection.** The export is a complete git repository with history, so recovery is the degraded path run backwards. **The backup costs nothing extra because it is the same mechanism** |
| **the clock moved** | provenance timestamps are the daemon's, never the client's. §16's resume grace epoch already handles a suspended laptop and the record inherits it rather than inventing a second answer |
| **a projection and the store disagree** | **the store wins and the projection is regenerated.** Nothing hand-edits the record repository, so a disagreement is a bug in rig rather than a merge to resolve - and it is detectable, since the projection is deterministic from the records |

**THE ONE THAT IS NOT A MECHANISM AND MUST BE SAID: a record nobody wrote is
still nothing.** The gate refuses a write without a read; it cannot force a
session to record what it learned. **`project.brief` reporting a work item whose
progress stream has been silent for an hour is the closest rig gets** - ⛔ **and
the brief carries THE AGE AND NO THRESHOLD, deliberately.** The data is in row 1
already. **A threshold is a judgement about whether work is going well, which is
domain logic, which §29 non-goal 1 keeps out of rig.** Stated so nobody adds one
later believing its absence was an oversight. **The age is a signal to Boris
rather than a guarantee**, and reading it is his job precisely because deciding
what it means is not rig's.

### The build order, in eight slices, each one demonstrable on its own

**Ordered so that the entries which could lose to the logbook are answered
first, not last.** §20's rule holds throughout: a slice is not done because it
passes, it is done when it has been exercised for real.

| # | Slice | What proves it, and it is a demonstration rather than a test |
|---|---|---|
| 1 | **the store and the three nouns.** `record.put/get/query/history` | a requirement is written, superseded twice, and **its first wording is read back with the session that wrote it** |
| 2 | **progress streams and `project.brief`** | **value lands HERE, and that is the attack's finding 8.** A work item is driven start to finish and **the report is read off the brief with no seat having written a sentence of prose** |
| 3 | **the window renders the brief** | Boris watches a live session without asking for a report. **Two slices to the thing he asked for first** |
| 4 | **links and `record.refs`** | *"what cites this requirement"* answers with a copy that a grep for the obvious phrase misses. **The struck-quotation residue of 2026-09-12 is the fixture** |
| 5 | **the projection: files, commits, push, and the pending path** | `rigd` is killed; the project is read from the repository alone, **a record is written while it is still down**, and the daemon ingests it on start. Then the remote is checked |
| 6 | **the read-before-write gate** | a `record.put` is refused **by name**, one `project.brief` clears it, a change to a must-read record re-arms it - **and `progress.step` is never refused** |
| 7 | **the standards register and drift** | a standard's version is raised and every project behind it is listed, **with asserted stamps rendering differently from evidenced ones** |
| 8 | **migrate one project, and it is rig** | below. **It cannot start while any register entry is merely ANSWERED** |

### The migration, which is also the design's hardest test

**rig is the first project migrated, under §37's staged rule: a capability lives
in exactly one system at a time, so the logbook stays authoritative until rig's
record is live.**

| Step | |
|---|---|
| **import structure mechanically** | `plan/` sections become `requirement` records, `DECISIONS.md` entries become `decision` records, `BACKLOG.md` rows become `work-item` records, `COORDINATION.md` rows become ownership records. **The structure is already there** - this project has been writing tables with stable shapes for three days |
| **derive the links from the citations** | ⛔ **MEASURED 2026-09-16 AND BOTH HALVES OF THE OLD CLAIM WERE WRONG.** This read *"~370 of them, all of the form `PLAN.md section 37`"*. `[ran it]`: **4,206 `§NN` citation edges** across `plan/` and the logbook, 42 distinct targets, **maximum in-degree 758** (§5). **The count is 11x higher and the dominant form is `§NN`; the prose form measures 15.** Excludes two archived `PLAN.md` snapshots under `agent-work/` that inflated a first pass by 313 - **an archived copy of a generated document is not an independent citation, and a grep cannot tell the two apart.** ⛔ **AND 78% ARE SECTION-GRAINED, WHICH IS ATTACK FINDING 5's FAILED ROW:** 911 carry a subsection letter, the other 3,295 resolve only to a whole section, and §37 is 942 lines. **So the migration's success number STARTS AT 22%, and the remaining 78% is not a parsing problem** - a `§37` does not contain the information needed to resolve it. **This is the first real test of "correlated"**: if the import cannot turn an existing citation into a link, the model is wrong and it is better to find that out on an import than on a year of use |
| **and the import's unit is the REQUIREMENT, never the section** | **Attack finding 5, which is the one that would have let this pass while delivering nothing.** §37 is **942** lines - this row said 738 until 2026-09-16, one cell away from the measurement that corrected it; a link to it is the same coarse pointer in a new format. **A citation resolvable only to a section is a FAILED row, counted and reported**, and the migration's success number is the share resolving to ONE record |
| **run both systems in parallel until the projection is comparable to the logbook** | and **this is a pattern this project has already proved**: `tools/plansplit.py` refuses to write anything until it has shown the parts reassemble into the original byte for byte. **The migration owes the same proof before the logbook stops being authoritative** |
| **cut over per project, never estate-wide** | §37 again. A second project follows only after rig's own has run long enough to have been wrong once |

**AND THE MIGRATION IS THE FIRST REPORT THE ARMED SEATS OWE.** §37's obligation
is that a seat says what a mechanism cost it or saved it. **Importing three days
of this project's own documents is the cheapest measurement of whether the
record is significantly superior, and it happens before anything depends on it.**

### The acceptance demonstrations, one per claim

**The bar is that parity is failure and each claim is demonstrated against the
documents doing the same task.** These are the demonstrations, named now so the
build cannot finish by declaring itself superior.

| The claim | The demonstration, and the document's answer beside it |
|---|---|
| **refuse a write until what must be read has been read** | a session writes without reading and is refused by name. **`COORDINATION.md` says *"read in full before your first write"* and cannot refuse anything** |
| **know a claim has gone stale** | a seat's claim is tied to its lease and expires with the seat. **The document keeps four claims that were true at the time**, which is the measured 2026-09-11 failure |
| **route a requirement at the moment it is stated** | a requirement is stated, recorded, and found by query in the same minute. **Four of his requirements were found living only in a volatile document**, days later |
| **derive the human view** | the brief is rendered with no seat writing prose. **Today a seat composes a report, and he named that cost himself** |
| **answer a query instead of being read** | a requirement is found without reading 5,218 lines. **`PLAN.md`'s own Map says nobody reads it whole** |
| **be the same in every project** | a second project adopts the record and adds no routing prose of its own. **Today each project carries a per-project instruction file that must be perfected separately** |

**AND THE CHEAPEST ONE OF ALL, which is the whole capability in a number:** this
project's per-session instruction file is measured before the cutover and after.
**If it has not shrunk to a pointer, none of the above mattered.**

## WHAT THE ATTACK CHANGED, 2026-09-12

**Boris typed `/attack` on this section the hour it was written.** Run
single-seat rather than fanned out, because the weekly budget was at 93% against
his own 95% cap and a fan-out is what that cap names. **Nine findings survived;
six of them change the design rather than its wording, and two contradict claims
this section made about itself.**

**READ THIS BEFORE BUILDING ANY SLICE.** The fixes are applied in the text above
where they belong; this part records what was wrong so the argument is not had
again.

#### FINDING 1, and it is the worst one: read-only when rig is down means capture STOPS

**The spine above says rig is the only writer.** So during an outage a session
**cannot record anything at all** - not a decision, not a progress step, not a
requirement Boris states while it is down. **The logbook can always be written
to, because it is a file.**

**THAT IS STRICTLY WORSE THAN THE DOCUMENTS, and §29 asks for *reduced* service,
not none.** It also fails on the exact axis this capability exists for: **the
requirement stated during an outage is the requirement that gets lost**, which
is the failure being fixed.

**THE FIX: a pending path that never needs the daemon.**

| | |
|---|---|
| **`rig record put` with no daemon writes to `records/_pending/`** in the projection repository | a file, in a git repository, exactly as today. **The one thing that must keep working uses the one mechanism that cannot be down** |
| **rigd ingests pending entries on start**, assigns versions, and resolves order by timestamp | an ingest that conflicts with a record written meanwhile **fails that entry and leaves it in `_pending/` with the reason**. It is never silently dropped and never silently overwritten |
| **a pending entry is VISIBLE as pending**, in `project.brief` and in the window | so a run of them is a signal that rig has been down, rather than the appearance of a quiet project |
| **the store stays authoritative** | a pending entry is not a record until it is ingested, which keeps one writer and one version line. **The fallback adds a queue, not a second source of truth** |

#### FINDING 2: `sensitive` payloads and a lossless projection cannot both hold

**A continuation slot is `kind: continuation` with a `sensitive` payload, and the
projection is claimed lossless so the store can be rebuilt from it.** If
sensitive payloads are projected they land in git and are **pushed to a
remote**, which destroys the guarantee §16 gives them. If they are not, the
projection is not lossless and the rebuild is not complete.

**THE FIX, and it is the honest half rather than the clever one:** **sensitive
payloads are NEVER projected and are NOT recoverable.** A rebuild restores every
record, link, version and provenance field **except the bodies of sensitive,
expiring kinds** - which is correct, because a continuation slot's whole lifetime
is minutes and a slot that outlives a daemon restart had already failed its own
purpose. **"Lossless" is now stated with its exception rather than as an
absolute.**

#### FINDING 3: eleven tensions were marked CLOSED against the register's own rule

**The register says an entry closes with *"a written answer, demonstrated
against the document approach doing the same task"*.** Nothing is built, so
nothing has been demonstrated. **Eleven entries were marked CLOSED by the seat
that wrote the answers, in the session that wrote them, with no adversarial
review** - which is the shape of every self-graded gate this project has already
had to take back.

**THE FIX: a state between open and closed.** An entry is **ANSWERED** when the
design settles it and **ANSWERED** only when its demonstration has run. **The
eleven are ANSWERED.** Slice 8 cannot start while any entry is merely answered.

#### FINDING 4: a stamp with no evidence makes drift decorative, and this project has the precedent

**`standard.stamp` records that a project was checked, by whom and when. Nothing
requires that a check happened.** Within weeks every project is stamped and the
drift number measures nothing.

**THE PRECEDENT IS IN THIS PROJECT'S OWN BACKLOG, B26:** the size ratchet fired
on every build and the only available response was to override it, so **a
control whose every firing is answered by overriding it is measuring nothing.**
The gate was dropped for exactly that.

**THE FIX, and it is the same shape §16 already uses for claims:** **a stamp
carries a witness or it is not a stamp** - a link to the artefact, the command
output, or the record that shows the check ran. A stamp without one is recorded
as **asserted** and renders differently from **evidenced** in drift. **rig still
does not judge the work; it distinguishes a check from a claim about a check.**

#### FINDING 5: the migration test would PASS while delivering nothing

**The 4,206 citations are of the form *"PLAN.md section 37"* - a pointer to a
SECTION, and §37 is 942 lines.** Turning that into a link produces a link to 942
lines. **That is the same coarse pointer wearing a new format**, and the test as
written would report success.

⛔ **THE NUMBERS IN THIS PARAGRAPH WERE ~370 AND 738 UNTIL 2026-09-16**, and both had been corrected in the register's own row above. **A correction landed where it was argued and three summaries kept the old number** - inside the section whose entire purpose is catching that class. Found by the record seat, which had itself reintroduced the same drift one table away the same morning.

**THE FIX: the import's unit is the REQUIREMENT, not the section.** A section
becomes many records. **A citation that can only be resolved to a section is a
FAILED import row, counted and reported**, not a passed one - and the migration's
success number is the share of citations that resolve to a single record.

#### FINDING 6: auto-push publishes whatever was recorded, including a secret

**rig committing and pushing means anything written to a record reaches a remote
without a human seeing it.** Today a person commits and can look. **§15 already
carries the invariant for history** - what `secrets.get` returned is never
recorded, only the key name - and **the record has no such rule written.**

**THE FIX: the record inherits §15's redaction invariant, stated here rather
than assumed**, and a field declared `sensitive` never projects (finding 2 makes
that mechanism exist anyway). **The push makes this urgent rather than tidy: an
unredacted record is not a local mistake, it is a published one.**

#### FINDING 7: the gate guarantees delivery, not attention, and the text overclaims

**`project.brief` returning the must-read set is not the same as the set having
been read.** The design claims *"a document can only ask; rig mediates the write,
so the precondition is a mechanism"*. **The honest claim is narrower: rig
guarantees the material was delivered to this session and that the session
acknowledged it. Comprehension is not enforceable and must not be implied.**

**AND THE GATE'S OWN FAILURE MODE IS SILENCE**, which is what the record exists
to prevent: a refused write can simply become no write. **So the gate never
blocks `progress.step`** - progress is always accepted - and it blocks only
substantive records. **Losing a progress step to a gate would be the mechanism
defeating its own purpose.**

#### FINDING 8: nothing delivers standalone value until slice 5

**Slices 1-4 build a store, links, a projection and a gate. Boris sees nothing
until slice 5.** A capability that needs five slices before anyone benefits is
the shape that gets half-built and abandoned - **which is what §37's minimum-set
narrowing exists to prevent, and this design walked back into it.**

**THE FIX: reorder so value lands at slice 2.** **1 - the store and the three
nouns. 2 - progress streams and `project.brief`**, which gives him live
oversight of a running session with no links, no gate, no standards and no
migration. Links, the projection, the gate and the rest follow. **The first
thing built is the thing he asked for first.**

#### FINDING 9: one derivation is right, one rendering is not

**The human and the agent want different answers from the same data.** He wants
*is this going well and what must I decide*; an arriving agent wants *what must I
read, what is claimed, what is decided*. **`project.brief` stays one derivation
- that part is correct and is what makes the human view free - but it carries
both views and the caller says which.** Claiming one rendering serves both
produces something mediocre for each.

## THE OPEN TENSIONS, AND NONE OF THEM MAY BE CLOSED AS AN ACCEPTED COST

**Boris, 2026-09-12, and it is a rule about the design rather than a wish about
it:**

> *"We obvously must resolve all failures within my requirements to make it as I
> asked it to be - maximally superior and the absolute best for the purpose."*

**SO EVERY TENSION THIS SECTION RECORDS IS A REGISTER ENTRY, NOT A CAVEAT.**
The failure mode this closes is the one the rest of this specification is full
of: a hard problem written down honestly, called a known cost, and then shipped
around. **A design with an unresolved tension in it is not "the absolute best
for the purpose", so none of these is optional and none closes by being
tolerated.**

**THREE STATES, AND THE MIDDLE ONE EXISTS BECAUSE OF THE ATTACK.** An entry is
**OPEN** until the design settles it, **ANSWERED** once it does, and **CLOSED
only when its demonstration has RUN**. **Eleven entries were marked closed by the
seat that wrote the answers, in the session that wrote them** - the shape of
every self-graded gate this project has had to take back. They are ANSWERED.
**Slice 8 cannot start while any entry is merely ANSWERED.**

**HOW AN ENTRY CLOSES, and there is exactly one way:** a written answer in this
section, **demonstrated against the document approach doing the same task**.
That is his acceptance bar applied per entry rather than once at the end. *"It
is nicer"* does not close anything; *"the documents cannot do this at all"*
does.

| # | Tension | State, and where the answer lives |
|---|---|---|
| 1 | **what a session sees with rig down** | **ANSWERED - the spine.** The projection is a complete, committed, pushed git repository read exactly as the logbook is read today. §29's *"reduced service"* is read-only, no live progress, no cross-project query |
| 2 | **how the record leaves the machine** | **ANSWERED - rig commits and pushes the record repository itself.** A push that fails is retried and surfaced in `project.brief`, never swallowed |
| 3 | **history of a requirement** | **ANSWERED - records are append-only and `record.history` answers it with provenance.** Git versions the projection underneath, so the weaker answer survives if the stronger one is ever wrong |
| 4 | **which documents materialise as files** | **ANSWERED - two targets with opposite commit rules.** The record repository is rig's and rig commits it; documentation in the project repository is written by rig and committed by the human with their code, as `PLAN.md` is today |
| 5 | **the word "conformance" is taken** | **ANSWERED - §19 keeps it.** The register says `upholds`, `standard.stamp`, `standard.drift`. "Conformance" is reserved and the register may not borrow it |
| 6 | **continuation slots versus the record** | **ANSWERED by removing a mechanism.** A slot is `kind: continuation` with a TTL and a `sensitive` payload. Anything that survives its TTL should have been a record |
| 7 | **does the record JOIN §37's minimum set** | **ANSWERED 2026-09-15 - it sits BESIDE the set**, which has a different bar. ⛔ **This cell said "Row 3 keeps its position and the record follows it" until 2026-09-16** - the superseded sentence, still stated flat here while the body quoted it INSIDE its own correction block. The record's only storage dependency is B28, a DECISION, not row 3; Boris narrowed row 3 and then cancelled rows 2-5 outright. Also closes `00-rig-kickoff.md` Step 0's "row C" ordering question - there is no row C once the record is not in the set |
| 8 | **how a project extends the schema** | **ANSWERED - add fields, never redefine; local kinds are namespaced and unlinkable across projects.** A local kind two projects want is a proposal to the estate register |
| 9 | **what the window renders** | **ANSWERED - `project.brief`, the same call an arriving agent makes.** One derivation, two consumers, which is the two-consumer requirement met by construction |
| 10 | **how a standards check is scheduled** | **ANSWERED - a standard carries `recheck_after`; due checks surface in `project.brief` and the window. rig surfaces, never runs, never judges** |
| 11 | **binary and bulky artefacts** | **ANSWERED - `kind: artefact` holds metadata and provenance; the bytes are files in the projection, where git already versions them.** rig does not become a blob store |
| 13 | **which store backs the record** | **OPEN, direction sharpened 2026-09-15.** B25's search covered the coordination primitives and chose bbolt; the record store additionally wants query-by-field, full-text and graph traversal. **His lean is pure-Go SQLite, widened to *"other/additional great technologies allowing to be state of the art"*, and on 2026-09-15 named SQLite + `bleve` as a good-looking combination** with best-of-breed additions welcome per axis. **The graph-traversal axis now has a named lead too: `github.com/dominikbraun/graph`** - generics, zero dependencies, ~90% coverage, mature and widely used, found by search rather than asserted (`sixafter/graph` and `graphium` are newer alternates, the latter needing Go 1.26+). Search still runs per AXIS (store, query, links, full text, semantic, graph) and prices combinations, **including against SQLite's own recursive CTEs per §38b - a library is not owed the win** |
| 12 | **what replaces "commit both repos"** | **ANSWERED - nothing does, because the second commit stops being a session's job.** rig owns the record repository and commits it |
| 14 | **where the record lives on disk, and how it is organized** | **RULED (root): `~/.rig`, `<scope>/<name>/` under it, one home for many projects.** **RULED (repos): split by SCOPE, each scope its own git repository** so personal (`me`) and professional (`work`) never share a history - scope set is OPEN, not closed to two. **RULED: rig is to support both logbook and d2d functionality**, what that means is still to be discussed. OPEN (structure): the per-scope/per-subject tree and cross-scope indexing is a design-slice job measured against the logbook's own layout |

**THIS TABLE IS ITSELF THE MAINTENANCE PROPERTY THE RECORD IS SUPPOSED TO HAVE,
applied to its own design.** A tension found later is added here rather than
mentioned in a session, and **an entry that closes says where its answer lives**
- which is the reverse-direction link §39 exists to provide.

**ELEVEN OF THE FIRST TWELVE ARE ANSWERED, NONE IS CLOSED, AND NOTHING HAS BEEN
DEMONSTRATED.** The twelfth is a ruling rather than a problem, **and a thirteenth
was added the same day by the mechanism this register describes** - a tension
found later is added here rather than raised in a session. **Each closure names where its
answer lives, which is the reverse-direction link this section exists to
provide.** A tension found later is added here rather than raised in a session.

**NO CODE IS STARTED.** He ruled the same day that row 1 cuts over first; this
design is what the build begins from once it does.

