## 35. What the agent-parallelism pass changed, 2026-09-11

**Boris's steer, which is a ranking instruction and not new scope:** the
mechanisms that matter are the ones that help **AI agents work in parallel** on
one machine, so that rig can run like AgentBox and the agents can then be moved
off AgentBox onto rig - *"the rig takes all this functionality to the highest
possible level of quality, robustness and utilization benefit."*

**And the bound on it, stated the same day:** *"we decided to not make perfect
parity to AgentBox, some of its features have no place in the rig."* **Nothing
is imported because AgentBox has it.** Each mechanism below is here because an
agent working beside another agent fails without it, and the IN/OUT judgement
for every candidate is recorded in the cross.

### THE ACCEPTANCE FLOOR - **ITS ATTRIBUTION IS CORRECTED 2026-09-12 AND THE REQUIREMENT SURVIVES**

**THE QUOTATION BELOW IS NOT HIS, AND THE TRANSCRIPT SWEEP OF 2026-09-12 IS WHAT
ESTABLISHED THAT.** It is kept, struck, because the section's whole lesson is how
a misquotation reads as evidence - and this section was itself carrying one.

> ~~*"One sanity check is that the functionality in rig must be at the absolute
> minimum as good as in AgentBox; It should be taken to the highest level of
> quality, robustness, usefullness and feature-completeness."*~~

**What the sweep ran and what it found.** The whole session transcript store was
searched for the distinctive fragments of that sentence - *"as good as in
AgentBox"*, *"sanity check"*, *"feature-completeness"*, *"highest level of
quality"* - restricted to messages Boris actually typed. **Zero hits, in every
project, on every date.** Its first appearance anywhere is in a SEAT's own
message on 2026-09-11 at 09:28:37, **two minutes before** he typed the sentence
below. **The exact method, the instrument and the yield are recorded in
`logbook/projects/rig/salvaged-2026-09-12.md`**, so the finding is re-runnable
rather than taken on this document's word.

**WHAT HE ACTUALLY TYPED, 2026-09-11 at 09:30:48, and it is the floor:**

> *"Not all AgentBox are must-have in rig, but the ones that are must be taken
> to the absolute highest level as I already said"*

**THE REQUIREMENT IS UNCHANGED AND IS IF ANYTHING SHARPER.** *"The ones that are
must be taken to the absolute highest level"* says the same thing the invented
sentence said, in his own words, and adds the half the inventions kept dropping:
**the bar attaches to the subset rig adopts, not to parity with AgentBox.** That
is exactly §37's minimum beneficial set, written eighteen hours before anyone
proposed it.

**WHAT THE SWEEP CANNOT SAY.** Transcripts are not a complete record - a
compacted session can lose its original user text, and a message sent from
another client would not be here. **"Not found in the store" is the finding;
"he never said it" is not.** But the ordering is hard evidence: the seat wrote
the sentence first.

**This is an ACCEPTANCE CRITERION, not a steer.** It says when the M16 cutover
is allowed to happen: a mechanism rig takes over from AgentBox is not done when
it exists, it is done when it is **at least as good as the thing it replaces**.
It bounds §29's non-goals from the other side - a feature may be ruled OUT, but
one ruled IN may not ship weaker than AgentBox's.

**Where it was found, and why that matters:** in a departed seat's handoff
document, quoted verbatim, and in **no specification, backlog or decision log**.
It is the second requirement of his in two days to be found living only in a
volatile file, after the tray icon. **A handoff is notes; it is not where a
requirement is kept.**

**THE THREE-WAY QUOTATION DISPUTE IS RESOLVED, 2026-09-12. THE FIRST ROW IS
VERBATIM AND THE OTHER TWO WERE NEVER SAID.**

| Source | Quoted as |
|---|---|
| the coordinating seat's handoff | *"Not all AgentBox are must-have in rig, **but the ones that are must be taken to the absolute highest level as I already said.**"* |
| §16's agent-parallelism subsection | *"not all AgentBox are must-have in rig, **some of its features have no place in the rig**"* |
| this section, above | *"we decided to not make perfect parity to AgentBox, some of its features have no place in the rig."* |

**The middle one is the head of the first welded to the tail of the third.**

**RESOLVED BY THE TRANSCRIPTS, 2026-09-12.** The previous text read *"the dispute
is recorded rather than resolved, deliberately. Nothing in this repository can
say which is verbatim; only a transcript can"* - and it was right about the
method. **The transcript was read and the answer is row 1**, at 2026-09-11
09:30:48, quoted above in full.

| Row | Verdict |
|---|---|
| **the coordinating seat's handoff** | **VERBATIM.** The only form he ever typed |
| §16's agent-parallelism subsection | **INVENTED.** *"some of its features have no place in the rig"* appears in no message of his, in any project |
| this section, above | **INVENTED.** *"we decided to not make perfect parity to AgentBox"* likewise |

**THE OPEN LOOP THIS CLOSES** was recorded as *"needs a transcript"* in
`HANDOFF.md` and in the outgoing lead's open-loops table. **It is closed and it
cost one search**, which is the argument for the standing rule below.

**AND IT GENERALISES INTO A RULE, because the same defect produced the floor
above and both inventions here:** **a quotation attributed to Boris that cannot
be traced to a transcript is a SEAT'S PARAPHRASE until proved otherwise.** The
two failures were not carelessness - each seat was quoting what it had been
handed. **The carrier of a quotation is never its evidence**, which is the rule
`DECISIONS.md` already states for claims and which is hereby extended to his
words specifically.

**But the damage is already legible without resolving it.** Both of the
specification's copies keep the clause that **licenses omission** and drop the
clause that **demands quality**. So until today this document recorded Boris
permitting features to be left out, and recorded nowhere his requirement that
what remains be taken to the highest level. **A misquotation is worse than a
gap, because it reads as evidence.**

### The ranking, and what it is ranked by

§34 lists twelve gaps in the order they were found and does not claim that is a
ranking. **This is the ranking.** It comes from crossing the blind mechanism
taxonomy against the AgentBox parity pass - two lists produced independently and
deliberately so, where **a mechanism appearing in both is not arguable**.
Record: `logbook/projects/rig/taxonomy-parity-cross-2026-09-11.md`.

| Rank | Mechanism | In both lists? | Landed |
|---|---|---|---|
| 1 | A claim has an owner and a liveness witness | **yes** | §16 |
| 2 | The message contract, queued to acted-on | **yes** | §16 |
| 3 | Seats, generations, `HANDING_OFF` as a published state | **no - see below** | §16 |
| 4 | Retraction of a posted item | **yes** | §16 |
| 5 | A lease name registered with the resource it protects | no | §16 |
| 6 | Health is evidence of progress; idle-and-not-blocked is unhealthy | **yes** | §18 |
| 7 | Durability of an agent's own work | no | §16 |
| 8 | At-risk detection and the escalation ladder | partial | §16 |
| 9 | The state ownership matrix | no | §18 |

**§18 also gained the thing that BLOCKED something else.** The supervisor was
described entirely in prose, which is why §5's "declared state machines, if the
supervisor can be its first client" could never be tested: there was nothing to
express. It now has a state set, a transition table with the trigger and the
owner of each, a statement of what is illegal and what happens on one, and the
four actors that can move a program named. **That test is now runnable**, and it
was costed at a design session rather than a milestone.

**Rank 3 is absent from the parity pass because AgentBox has no seats either**,
and that is the finding rather than a weakness in the pass. The estate runs
multi-seat teams today on a naming convention over a blackboard key and a signal
topic every seat must spell identically - **discipline, enforced by nothing.**
It is therefore the one place rig overtakes rather than catches up.

**The symmetric caution, kept because the ranking is worthless without it:**
ranks 1, 2 and 4 are things AgentBox **already does** and rig had not specified.
Overtaking on rank 3 while losing those at the M16 cutover would be a net loss
to every agent on the machine. **The floor comes first; rank 3 is what the floor
is for.**

### What was PROVED in code rather than argued, and by whom

**Every claim in this section that describes rig's behaviour today was run.**
The three findings that changed what was written:

| Finding | What it changed |
|---|---|
| **A succession and a name collision are BYTE-IDENTICAL refusals.** Both cases built and the strings compared directly; the message names the holder and says nothing about the newcomer | §16's rank 3. The first draft said the two "produce the same error"; the measurement made it exact, and turned up that the holder's session id in the message is a label no caller has ever seen rather than a discriminator |
| **The weaker-effects path is a RECONNECT, not a live re-declaration.** The daemon refuses a second handshake on a live connection, so the reachable path is close, deregister, reconnect one level weaker - an ordinary deploy | §13a. **The first statement of this hole was wrong about the path**, which mattered: the old declaration is gone by the time the new one arrives, so closing it means remembering a declaration past the connection that made it. Harder than the version that was written first |
| **`confirms` is declared, rendered to a human, and consumed by nothing.** The reference program's `purge` declares `destructive` with `confirms` yes, its help says so, and it runs to completion with no terminal: no prompt, no refusal, no channel | §13a gained its own subsection. This is conflation #16 in its sharpest form - **rig stating something false to a user**, rather than a document misleading a reader |

### A declared field that no surface can return is a DEFECT, and there are two

**Found twice on one day, by two different seats, in two unrelated fields.**
That is a class, not a coincidence.

| Field | What happens today |
|---|---|
| `confirms` | declared, validated, **rendered to a human as a guarantee**, and consumed by nothing. Ruled above: it becomes an input rig acts on |
| `preamble` | declared, validated, stored since M1 - and **there is no wire message on any path that can carry it back to a caller.** The daemon-to-caller message is the declaration minus the preamble, so it was accepted and dead |

**The rule, because two instances is enough to state one:** a field the
declaration accepts must be reachable by some caller on some surface, or it is
not a field - it is a validated place to put something that goes nowhere.
**Acceptance is not a contract; retrievability is.**

**And it is a conformance item rather than a review habit** (§19): the suite
already drives a reference program, so a declaration that sets every optional
field and a projection that has to return them is a test a machine can run.
**Neither of these two was going to be caught by reading**, and both were found
only because somebody went to USE the field.

### What this pass has NOT done, and it is deliberate

- **Ranks 6 to 9 are specified nowhere yet.** Rank 9's data is in hand and the
  other three are not started. Listing them ranked and unbuilt is the honest
  state; a pass that reported only what it finished would be the same defect
  §34 exists to audit.
- **The `important` (290) and `situational` (34) taxonomy tiers are still not
  consolidated**, unchanged from §34.
- **Nothing here rules on `confirms`.** Two options are live - a program's claim
  about itself, or an input rig acts on - and **only the current state is ruled
  out**, because a declared-and-unconsumed field shown to a user is a false
  statement whichever way the question goes.
