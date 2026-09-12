## 39. The continuity record

**RULED BY BORIS, 2026-09-12**, after he raised the problem unprompted on two
consecutive days. Put to him as four options; he took rig carrying the
mechanism and then widened it in the same breath:

> *"I think rig should contain as much as possible since this is a single
> source of truth we can perfect, instead of perfecting general instructions
> across many different CLAUDE.md"*

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
| **linked** | ~370 citations of the form *"PLAN.md section 37"* | **they address sections by NUMBER, never by line, because a line address rots.** The convention exists precisely because there are no real links, and it is why splitting 5,218 lines into 38 files needed no citation edited |
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

**WHAT IS NOT SPECIFIED YET, AND IT IS MOST OF IT.** The record's shape, its
verbs, how a project extends the schema, what the window renders, **how the
standards register is versioned and how a project is checked against it**, **what
a session sees with rig down and how the record leaves the machine**, and how all
of it relates to the continuation slot above - which is the ninety-second version of
the same idea and must not become a second mechanism by accident. **This
subsection is the requirement and the ruling. The design is owed.**

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

**HOW AN ENTRY CLOSES, and there is exactly one way:** a written answer in this
section, **demonstrated against the document approach doing the same task**.
That is his acceptance bar applied per entry rather than once at the end. *"It
is nicer"* does not close anything; *"the documents cannot do this at all"*
does.

| # | Tension | Why it is not optional | State |
|---|---|---|---|
| 1 | **what a session sees with rig down** | §29: *"rig is not required. Every program works without it, at reduced service."* The record holding the only copy breaks a non-goal | OPEN |
| 2 | **how the record leaves the machine** | the logbook has a private remote; a local database file has none. **The four lost requirements get worse, not better, if the store cannot go offsite** | OPEN |
| 3 | **history of a requirement** | git answers *who changed this, when, and what it said before*. Parity is failure, so the record must answer it better and queryably | OPEN |
| 4 | **which documents materialise as files** | a `README` must read in a clone with no rig. The record is the source and files are projections - **which ones, and how they stay current** | OPEN |
| 5 | **the word "conformance" is taken** | §19 is a PROGRAM against rig's wire contract; the standards register is a PROJECT against a standard. One of them is renamed | OPEN |
| 6 | **continuation slots versus the record** | §16's slot is the ninety-second note; this is the durable one. **Two mechanisms for one idea is the outcome to avoid**, and the boundary is unwritten | OPEN |
| 7 | **does the record JOIN §37's minimum set** | it changes what the gate means and what "rig develops rig" is measured against. Nobody has priced either answer | OPEN |
| 8 | **how a project extends the schema** | without becoming N copies of a schema, which is the §38c defect wearing a different hat | OPEN |
| 9 | **what the window renders** | the human view is DERIVED at *"very small agent effort"*. That ceiling has to be met by a design, not asserted | OPEN |
| 10 | **how a standards check is scheduled** | *"checked from time to time"* - what triggers it, what "last checked" means once the standard has moved, and who is told | OPEN |
| 11 | **binary and bulky artefacts** | screenshots, briefs, audits and agent work are in the logbook today. **Held, referenced, or refused** - and refused is an answer only if it is stated | OPEN |
| 12 | **what replaces "commit both repos"** | during the staged per-project cutover, a project is half in each. The rule that replaces it has to work in the half-migrated state, not only after | OPEN |

**THIS TABLE IS ITSELF THE MAINTENANCE PROPERTY THE RECORD IS SUPPOSED TO HAVE,
applied to its own design.** A tension found later is added here rather than
mentioned in a session, and **an entry that closes says where its answer lives**
- which is the reverse-direction link §39 exists to provide.

**NOTHING HERE IS STARTED YET, DELIBERATELY.** He ruled on 2026-09-12 that row
1 cuts over first; this register is what the design begins from once it does.

