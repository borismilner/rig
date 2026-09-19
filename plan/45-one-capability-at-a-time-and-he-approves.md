## 45. One capability at a time, and he approves each

**RULED BY BORIS, 2026-09-19**, at the close of the session that produced §43
and §44. ⛔ **IT GOVERNS HOW rig IS DEVELOPED FROM THE NEXT `/resume` ONWARD**,
and it outranks the build queue every other document carries.

> *"After `/resume` I want the different capabilites whether already implemented
> on just planned/proposed to be perfected before doing any further work with
> the planner. For each I want to explore its usability and usefulness before
> going any further with tit. Any feature/capability we'll approve together will
> go to a subagent for implementation while we are working on the next
> feature/capability."*

**The spelling is his and is kept.**

### ⛔ THE REASON IS HIS OWN DIAGNOSIS, AND IT IS THE MOST IMPORTANT SENTENCE HERE

> *"The mistake I've done is trying to get as much as possible implemented and
> by doing this I lost the big picture and got drifter to many different
> directions that hurt my efficiency and as a result I don't have the product I
> aimed for by now."*

⛔ **THIS IS A STANDING RULE AND NOT A MOOD.** Every seat that reads this
section is reading the reason the queue is not simply worked through: **breadth
was tried, it cost him the product, and depth replaces it.** A seat that
optimises for how many rows it closes is repeating the exact mistake he has
just named.

**It is the fifth standing rule in effect, beside §38's four**, and it is the
one that binds hardest on the seats that feel most productive.

---

### The process, stated so a fresh session can run it

| Step | Who | What |
|---|---|---|
| **1** | the seat | **Present ONE capability.** What it is, what a program does with it, what it costs, the honest verdict, what is unresolved |
| **2** | **Boris and the seat, together** | ⛔ **EXPLORE ITS USABILITY AND USEFULNESS.** His words. This is a conversation, not a document handed over |
| **3** | **Boris** | approve, reshape, or kill it |
| **4** | the seat | **Perfect the specification** of what was approved, and record it |
| **5** | **a subagent** | **Implement it, unattended**, from that specification |
| **6** | Boris and the seat | **Move to the next capability while step 5 runs** |

⛔ **STEPS 5 AND 6 OVERLAP BY DESIGN. THAT IS THE WHOLE POINT OF THE SUBAGENT**
- his attention is the scarce resource, and it is spent on deciding rather than
on watching an implementation.

**Nothing about the planner happens until the capability pass is finished.**
§43's extraction and §44's four services are both downstream of this.

---

### ⛔ WHAT IS IN SCOPE: ALL THREE STATES, AND THAT IS UNUSUAL

> *"whether already implemented on just planned/proposed"*

| State | Examples | Why it is still reviewed |
|---|---|---|
| **already implemented** | declaration, CLI projection, MCP projection, the window and tray | ⛔ **Being built is not evidence of being useful.** Every one was built against a fake adopter |
| **planned** | storage, backup, config, logging, tracing, secrets, the bus, peers, the hand, generated UI, scheduling, the palette | The specification says what they are and nobody has asked whether he wants them |
| **proposed** | B109 to B113, the five from `service-review-2026-09-19.md` | Proposed by a seat, never attacked, never put to him |

**The starting agenda already exists:** `logbook/projects/rig/service-review-2026-09-19.md`
reviews all 21 with a verdict on each, and the five proposals are in the same
document. ⛔ **IT IS AN AGENDA, NOT AN ANSWER.** Its verdicts are one seat's and
step 2 is where they get tested against what he actually wants.

---

### ⛔ OPEN, AND A SEAT MUST ASK HIM RATHER THAN DECIDE

**He ruled the process. He did not rule any of the following**, and each changes
how the first session after `/resume` behaves.

| Open | Why it cannot be answered from what he said |
|---|---|
| ⛔ **What order are the capabilities taken in?** | The review's order is a reviewer's ranking; §44's order is a build dependency. **Neither is a statement about what he wants to look at first** |
| ⛔ **What does "perfected" mean, concretely?** | For a planned capability it is plausibly a specification he has approved. **For an already-implemented one it is unclear whether it means changing code or only confirming the design** |
| **Does a killed capability leave the plan?** | §43's correction says deferral is never deletion. **A capability he rejects in step 3 is a different case and has no precedent** |
| **How many subagents run at once?** | *"a subagent"* is singular in his sentence. Whether step 5 may have two in flight is not stated |

⛔ **THE FIRST ACT AFTER `/resume` IS TO ASK HIM THE FIRST TWO.** A seat that
picks an order and a definition of "perfected" on its own has taken the two
decisions this section exists to give him.

---

### What the subagent is owed, because an unattended implementation is where this fails

**A subagent implementing from step 4 cannot ask a question.** The specification
handed to it is the whole of its context, so:

- **It gets a written specification, not a conversation summary.** If step 2
  produced a ruling, that ruling is in `plan/` or `DECISIONS.md` before the
  subagent starts, never only in the session.
- **It persists to disk as it goes.** `~/.claude/agent-persistence.md` is the
  standing mechanism and this project has already lost work to a subagent that
  returned without writing anything down.
- **Its acceptance test is stated before it starts.** §44's form is the
  precedent: *a service is DONE when rig's own hand-rolled version is DELETED*,
  which is checkable by `git grep` rather than by reading.
- ⛔ **It owns named files.** `COORDINATION.md` is protocol, not a courtesy, and
  a subagent writing into the shared tree while the lead works the next
  capability is the collision this project has already recorded.

---

### What this does not change

- **§43 and §44 stand.** The planner still leaves rig, and the services rig
  itself uses are still what it owes first. **What changed is that neither
  starts until he has approved the capability in front of him.**
- **§38's four standing rules are untouched**, and the swiss-knife rule now has
  a partner: best at what it carries, and **it does not build what he has not
  looked at.**
- **The tray acceptance test is untouched.**
- **Deferral is still never deletion** - §43's 2026-09-19 correction. A
  capability waiting for its turn in this pass has not been dropped.
