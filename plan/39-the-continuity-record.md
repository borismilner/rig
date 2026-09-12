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
| **1. rig is down** | the materialised files are complete and readable by a human or an agent with no rig at all. **Reduced service, which is precisely what §29 asks for**: read-only, no live progress, no cross-project query |
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

### The model: records, kinds, links

**Three nouns and no more.** The whole of the indexing, linking and correlating
he asked for falls out of them.

| Noun | What it is |
|---|---|
| **a RECORD** | the atomic unit. An id, a kind, the project it belongs to, a body, typed fields, and provenance - **which session wrote it, when, under which seat**. Append-only: a change writes a new version and the previous one is retained |
| **a KIND** | what the record is, and it carries the schema for the fields. `requirement`, `decision`, `work-item`, `standard`, `note`, `artefact`, `progress` |
| **a LINK** | a typed, directed edge between two records. `rules-on`, `cites`, `supersedes`, `implements`, `checked-against`, `produced-by`, `blocks` |

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


### The verbs, and there are eleven

**Each is a call on `rigd`, and §5's single declaration projects every one of
them to the CLI, to MCP and to the window without being written three times.**

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
| **a must-read record that CHANGES re-arms the gate** | for every session that read the old version. This is drift detection pointed at the project's own rules, and it is free once drift exists for standards |

**THIS IS THE ENTRY THAT PROVES THE BAR.** A document can only ask. **rig
mediates the write, so the precondition is a mechanism** - and that is the
demonstration §39's acceptance bar requires, not an opinion that it feels
better.

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

| The brief carries | |
|---|---|
| open work items, each with its last progress step and its age | a stale step beside a live session is the signal that a seat is stuck |
| what is blocked, and on whom | including what is waiting on Boris |
| standards drift, and any check now due | |
| the must-read set and whether this session has cleared it | |

**ONE DERIVATION, TWO CONSUMERS.** That is the two-consumer requirement met by
construction rather than by discipline, and it is what makes *"the human-report
can be derived automatically or with very small agent effort"* true: the effort
is one call, and no agent writes prose over it.

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

#### Tension 7 - it sits BESIDE §37's minimum set. Recommended, not ruled

**The recommendation is that the record does NOT join the minimum beneficial
set**, and the reason is that the set has a different bar. §37's rows are
coordination primitives measured as *"the best inter-agent coordination
substrate we can build"*; **§39 has its own bar - significantly superior to the
documents - and its own twelve-entry register.** Folding it in would move a gate
Boris narrowed on 2026-09-12 and blur two acceptance tests into one.

**What does NOT change either way:** the record depends on durable storage, so
**row 3 keeps its position in the set and the record follows it.** That
dependency is a fact about the build order and is true under both answers.

**This one is his, and it is the last thing in this section that is.**

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

| # | Tension | State, and where the answer lives |
|---|---|---|
| 1 | **what a session sees with rig down** | **CLOSED - the spine.** The projection is a complete, committed, pushed git repository read exactly as the logbook is read today. §29's *"reduced service"* is read-only, no live progress, no cross-project query |
| 2 | **how the record leaves the machine** | **CLOSED - rig commits and pushes the record repository itself.** A push that fails is retried and surfaced in `project.brief`, never swallowed |
| 3 | **history of a requirement** | **CLOSED - records are append-only and `record.history` answers it with provenance.** Git versions the projection underneath, so the weaker answer survives if the stronger one is ever wrong |
| 4 | **which documents materialise as files** | **CLOSED - two targets with opposite commit rules.** The record repository is rig's and rig commits it; documentation in the project repository is written by rig and committed by the human with their code, as `PLAN.md` is today |
| 5 | **the word "conformance" is taken** | **CLOSED - §19 keeps it.** The register says `upholds`, `standard.stamp`, `standard.drift`. "Conformance" is reserved and the register may not borrow it |
| 6 | **continuation slots versus the record** | **CLOSED by removing a mechanism.** A slot is `kind: continuation` with a TTL and a `sensitive` payload. Anything that survives its TTL should have been a record |
| 7 | **does the record JOIN §37's minimum set** | **OPEN, AND IT IS BORIS'S.** Recommended: it sits beside the set, which has a different bar. Row 3 keeps its position either way and the record follows it |
| 8 | **how a project extends the schema** | **CLOSED - add fields, never redefine; local kinds are namespaced and unlinkable across projects.** A local kind two projects want is a proposal to the estate register |
| 9 | **what the window renders** | **CLOSED - `project.brief`, the same call an arriving agent makes.** One derivation, two consumers, which is the two-consumer requirement met by construction |
| 10 | **how a standards check is scheduled** | **CLOSED - a standard carries `recheck_after`; due checks surface in `project.brief` and the window. rig surfaces, never runs, never judges** |
| 11 | **binary and bulky artefacts** | **CLOSED - `kind: artefact` holds metadata and provenance; the bytes are files in the projection, where git already versions them.** rig does not become a blob store |
| 12 | **what replaces "commit both repos"** | **CLOSED - nothing does, because the second commit stops being a session's job.** rig owns the record repository and commits it |

**THIS TABLE IS ITSELF THE MAINTENANCE PROPERTY THE RECORD IS SUPPOSED TO HAVE,
applied to its own design.** A tension found later is added here rather than
mentioned in a session, and **an entry that closes says where its answer lives**
- which is the reverse-direction link §39 exists to provide.

**ELEVEN OF THE TWELVE CLOSED IN THE SESSION THAT OPENED THEM, 2026-09-12**,
and the twelfth is a ruling rather than a problem. **Each closure names where its
answer lives, which is the reverse-direction link this section exists to
provide.** A tension found later is added here rather than raised in a session.

**NO CODE IS STARTED.** He ruled the same day that row 1 cuts over first; this
design is what the build begins from once it does.

