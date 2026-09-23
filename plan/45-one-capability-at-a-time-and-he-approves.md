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

### ⛔ TWO OF THESE HE ANSWERED ON 2026-09-20. TWO ARE STILL OPEN.

**He ruled the process on 2026-09-19 and left four things unstated.** He was
asked the first two on 2026-09-20 and answered both; they are ANSWERED rows
below and bind from that day. The other two remain his to rule.

| Open | Why it cannot be answered from what he said |
|---|---|
| ✅ **ANSWERED 2026-09-20. What order are the capabilities taken in?** | **Backup and restore is FIRST.** His choice, put to him as a question with Storage, Configuration and a scope-cutting pass as the alternatives. The reason offered and accepted: it is small enough that the whole loop - explore, approve, specify, hand to a subagent - completes in one session, so the LOOP gets proven on a cheap case before an expensive one. ⛔ **This is one pick, not a standing order. The second capability is his to choose too** |
| ✅ **ANSWERED 2026-09-20. What does "perfected" mean for an ALREADY-IMPLEMENTED capability?** | **Confirm the design only.** We review it together, he approves or reshapes, and the output is a specification recorded in `plan/`. ⛔ **No code changes unless the review finds the DESIGN itself wrong** - a defect in the design is in scope, polishing the implementation is not. He declined the two costlier readings (design plus a live demo; design, demo and fix what the demo shows) |
| **Does a killed capability leave the plan?** | §43's correction says deferral is never deletion. **A capability he rejects in step 3 is a different case and has no precedent** |
| **How many subagents run at once?** | *"a subagent"* is singular in his sentence. Whether step 5 may have two in flight is not stated |

**Seat recommendations on the two open rows, 2026-09-23 evening, parked per
this section's own rule below.** Neither is an answer.

| Open row | Recommendation | Consequence |
|---|---|---|
| **a killed capability** | **Stays in `plan/`, marked ⛔ KILLED with his words and the date, and leaves every order and queue.** Deletion loses the reasoning, and the section is the evidence against the next seat re-proposing it | the plan carries closed sections. B24's route to the logbook exists for the day that costs something |
| **how many subagents at once** | **One BUILD at a time on the shared tree; any number of read-only agents beside it.** A measurement mutates only a detached copy and the tree sees one writer | **Evidence, 2026-09-23 evening:** one build (B104) and one measurement (B108) ran together; the measurement took a `VACUUM INTO` copy and touched no shared file. The next approved build waits for the running one's last commit |

⛔ **THE FIRST TWO ARE ANSWERED AND A SEAT MUST NOT RE-ASK THEM.** What still
has to be asked is the NEXT capability, every time one finishes - the
2026-09-20 answer picked one row, and a seat that reads a queue out of it has
taken back the decision this section exists to give him.

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

### ⛔ HOW THE LOOP RUNS WHEN HE IS AWAY. BORIS, 2026-09-23.

> *"Find the plan to separate the planner from `rig` and follow it. I want you
> to do as much as possible on your own without bothering me!"*

**The plan is §43, §44 and this section, and following it means working §44's
services in §44's order; the separation is where the plan ends.** The loop
above is unchanged in WHO decides and changed in WHEN he is asked:

| Step | Who | Running without him |
|---|---|---|
| 1, 4, 5, 6 | the seat | **done without asking.** Presenting, specifying, spawning an APPROVED capability, moving on |
| **3** | **Boris** | ⛔ **still his, and BATCHED.** One consolidated ask per session: every capability that reached step 4, each with a recommendation and the consequence of each answer. Never one interruption per capability |
| the next capability | Boris | **the seat PREPARES in §44's order** (S1, S2, S4, S3) so his approvals are one word each. **Preparing is not approving** |

⛔ **Nothing is BUILT that he has not approved.** A subagent is spawned for a
capability he approved and the seat specified, and for nothing else. B104 is
the first: chosen by him on 2026-09-20, specified in §46, shown to him on the
morning of 2026-09-23 with the lever to reshape any row, and spawned that
evening on this instruction.

**A question that is his** (§43's open rows) **is parked with a recommendation
and with what proceeds meanwhile**, never as *"ask Boris"* alone.
`DECISIONS.md` 2026-09-23 carries the reasoning.

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
