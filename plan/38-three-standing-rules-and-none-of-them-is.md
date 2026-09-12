## 38. Three standing rules, and none of them is a preference

**All three are Boris's, stated 2026-09-12, and they bind every milestone rather
than any one of them.** They are here rather than folded into §22 or §29 because a
rule that applies to all future work needs an address a seat can be pointed at.

### 38a. rig is the world-best swiss knife of the capabilities it carries

**Boris, verbatim:** *"rig must always and forever remain world-best swiss-knife
of the capabilities we worked so hard to define."*

**READ IT AS A SCOPE RULE, WHICH IS THE HALF THAT IS EASY TO MISS.** A swiss
knife is defined as much by what it refuses to carry as by what it carries.
**"The capabilities we worked so hard to define" is a closed set** - §16's
primitives, §5's surfaces, the mechanism pass's additions - and §29's non-goals
are what keep it closed.

| The rule says | The rule does NOT say |
|---|---|
| **every capability in the set is the best version of that capability that exists** | that rig should acquire more capabilities |
| a mechanism rig carries is not "done" at merely working - §37's four clauses are the floor | that breadth is the goal |
| **a capability found weaker than its best-in-class equivalent is a defect**, filed like any other | that rig competes with anything on feature count |

**The operational form, so it is testable rather than inspiring:** **when a
capability is proposed for promotion, the seat names the best existing
implementation of that capability and says how rig's compares.** If the honest
answer is "worse", it does not get promoted. That is the same shape as the
AgentBox acceptance floor, widened from one comparator to the best one available.

### 38b. Never reinvent what a good Go library already does

**Boris, verbatim:**

> *"I don't want us to implement ANYTHING that can be achieved by using already
> best go libraries; we must never reinvent the wheel; we must have our feature
> best of the best but while making optimal reuse of existing code! So effort
> must be spent on this as well."*

**"SO EFFORT MUST BE SPENT ON THIS AS WELL" IS THE OPERATIVE CLAUSE.** The search
is work that is budgeted, not a courtesy check before writing the code anyway.

**The obligation, on whoever is about to write a mechanism:**

| Step | What it means |
|---|---|
| **1. Search before building** | name what exists. `context7` and the module proxy are in this session's tools; **"I did not find one" is only an answer after a search that is described** |
| **2. Report the candidates in the assignment or the commit** | at least the ones considered and why each was kept or dropped. **A build with no candidates listed did not do step 1** |
| **3. Reuse unless a stated reason blocks it** | and the reasons that count are listed below. Taste is not one |
| **4. If nothing fits, say what the nearest thing was and what it could not do** | so the next seat does not redo the search |

**The reasons that DO justify writing it here**, and they are the only ones:

- **§17's footprint.** A dependency that moves the ratchet materially is measured, not assumed - `make bench-size` answers it, and the answer goes in the commit.
- **§22's dependency bar**, which the stack already applies: maintenance, licence, transitive weight, whether it is still alive.
- **§13/§14's trust model.** A library that wants to own a process boundary, a socket, or an authorisation decision is doing rig's own job.
- **§29's non-goals.** A library that brings domain logic with it brings the thing rig refuses to have.

**WHERE THIS ALREADY APPLIES AND NOBODY HAS RUN IT.** The minimum beneficial set
in §37 is mostly coordination primitives - leases with witnesses, CAS on a
versioned key, a write-ahead log, cursored subscriptions. **Each of those has
mature Go implementations and none of them has been searched for.** That search
is the first application of this rule and it is a backlog item rather than a
sentence here.

**AND THIS RULE IS ITSELF AN ESTATE-WIDE STANDARD SITTING INSIDE ONE PROJECT'S
SPECIFICATION.** It binds every project Boris runs, not rig alone, and it lives
here because there has never been anywhere else to put it. **He named it as an
example when widening §16's record to a standards register on 2026-09-12**, and
under that design this rule lives once, versioned, with each project's
last-checked stamp against it. **Until that exists, 38b's home is this section
and the duplication is the known cost.**

**AND THE RULE HAS A LIMIT THAT MUST BE SAID, or it will be quoted against the
project's own reason for existing.** rig's product is the *composition* - one
declaration reaching a CLI, an MCP tool, a window, a tray and a schedule (§5).
**Reuse the parts; the composition is the thing being built and there is nothing
to reuse for it.** The rule is about not writing a WAL by hand, not about not
writing rig.

### 38c. rig is the single source of truth, so prefer rig carrying it

**Boris, verbatim, 2026-09-12**, stated while ruling on the continuity record
(§16):

> *"I think rig should contain as much as possible since this is a single source
> of truth we can perfect, instead of perfecting general instructions across
> many different CLAUDE.md"*

**THE ARGUMENT IS ABOUT WHERE A THING LIVES, not about how much rig does.**
Something that has to exist somewhere can live in one place that is built,
tested and improved, or in N copies of prose that every session has to read and
then comply with correctly. **One of those can be perfected. N cannot.** The
compliance half is the part that is easy to miss: an instruction is obeyed at
the reader's discretion, and a mechanism is not.

**AND IT PULLS AGAINST 38a, WHICH SAYS A SWISS KNIFE REFUSES THINGS. Both are
his and neither is withdrawn**, so the reconciliation is written here rather
than left for a seat to find the hard way:

| 38a asks | 38c asks |
|---|---|
| **should rig acquire this capability at all?** | **given it must exist somewhere, where does it live?** |
| defaults to NO. Breadth is never the goal | defaults to RIG, when the alternative is prose repeated per project |

**So they apply in order: 38a first, then 38c.** A capability that fails 38a is
not rescued by 38c. A capability that passes 38a, and whose only alternative is
an instruction every agent must read and obey, belongs in rig.

**The first application is the continuity record in §16**, which is what he was
ruling on when he stated it. **Its larger application is the standards register
in the same section**, stated the same day: the requirements every project must
uphold, held once and improved over time instead of copied into each project and
diverging. **38b above is the worked example of the divergence.**

