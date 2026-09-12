## 34. What the mechanism pass changed, 2026-09-11

An expert enumerated, **blind to this document and to the source**, the mechanisms
infrastructure of this class must have. 507 mechanisms in 33 categories; the record is
`logbook/projects/rig/mechanism-taxonomy-2026-09-11.md` and the brief that produced it is
`mechanism-expert-brief-2026-09-11.md`. The blindness is the method: a list written after
reading this document reproduces this document, and the gaps are precisely what anchoring
hides.

**The subject is narrower and harder than "programs".** Boris, 2026-09-11: *"user-programs
is meant for example AI agents - in fact it's mainly AI agents coordinating, locking,
communicating, syncing and all that."* §1's "user" is the unix user; the **adopting
programs** this section is about are autonomous agent sessions. They differ from services
in ways that change the mechanism rather than the tuning:

| The actor | What it changes |
|---|---|
| **Death is the normal termination** - a bounded context or a usage budget, not a crash. Frequent, and not an error | A lease design whose correctness depends on holders releasing is wrong at the centre, not at the edge |
| **It usually dies holding something** | ORPHANED vs FREE is the most consequential single decision in the coordination design |
| **It hands off to a successor, and both exist for a period** | "Alive" does not mean "will still be here". Two occupants of one role is legitimate, not a duplicate-registration fault |
| **Names recycle in shape** | A message can reach a session that is not the one the sender believes it is addressing |
| **It spawns children whose work dies with it** | Parent closure is a supervised event with obligations |
| **It is non-deterministic, and can repeat a claim it was corrected on** | A command served by an agent is not a function. Provenance must travel with a claim |
| **Its context is its memory, and that is not durable** | Durability of an agent's own work is a mechanism, not a discipline asked of the author |
| **A human supervises several at once and acts only on what he can see** | Visibility and reachability are mechanisms, not conveniences |
| **Idleness is a failure** | "Healthy" cannot mean "responding". Progress is the health signal |

### The bar this section is measured against

**A requirement that names a mechanism without stating its failure semantics has mentioned
it, not specified it.** §16's lease is the calibration point at one end - it says what
happens when the holder dies (ORPHANED, not FREE), what a resume does to expiry, and what a
`DRAINING` notice does to every held lease. §18's supervisor is the calibration point at the
other: five state names, no state set, no transition table, no owner, described entirely in
prose.

### Confirmed, and cheap to say so

| | Where | |
|---|---|---|
| Holder-death disposition of a lease | §16 | **The strongest thing in this document.** Two-step expiry, ORPHANED before FREE, a liveness witness, and an `unwitnessed` lease needing a recorded human break |
| Suspend-aware time | §16 | Absolute deadlines on `CLOCK_BOOTTIME`, `boot_id` stamping, a resume grace epoch, and the admission that `synctest` cannot model suspend |
| Advisory exclusion described honestly | §16 | "A token can only fence writes rig mediates", stated plainly *"because the unqualified sentence will otherwise be quoted back later"* |
| Snapshot and deltas atomically | §16 | "One global revision, transaction-granular delivery", so a multi-key claim is never observed half-applied |

### The foundational gaps, each traceable to a mechanism id

**Absent means the concept is not present under any wording** - each row was checked by
reading, not only by term count, and the term counts are given where they are the evidence.

| # | Gap | ids | State |
|---|---|---|---|
| 1 | **Naming is not part of the synchronisation mechanism.** Two actors guarding one resource under two names are both unlocked and neither finds out. Non-deterministic actors invent plausible names as a matter of course. **This estate has already paid for it**: one session took `rig-makefile` and another `repo:rig-shared-build` for the same file, and the file was found dirty mid-edit | J23, M4, K9 | **absent.** `well-known name`, `lease name`, `unregistered name`, `scope intersection`: 0 occurrences each. Needs: names registered with the resource they protect and discoverable before use; what the daemon does with an unregistered name; and overlap tested by **scope intersection**, not name equality |
| 2 | **Seats, generations and the misaddressed successor.** `generation` appears 13 times in this document and every one is a kit or semantics generation; `seat` appears 9 times and every one is a reviewer in §31's attack. **The concept of a role occupied by a succession of sessions does not exist here** | K1, K4, K6, C15 | **absent, and it has already shaped a design.** §16's continuation slots solve "find my note" by listing-and-asking *because there is no seat identity to address* - "nothing the old agent knew can name the slot". With seats and generations the successor could be handed its slot |
| 3 | **HANDING_OFF as a published state.** `handoff`, `handing-off`, `succession`, `overlap`: 0 occurrences. Two sessions legitimately occupy one role for a period; today that reads as a duplicate registration, which §5 refuses with `CODE_DENIED` | K2, K3, K6, K11 | **absent** |
| 4 | **Acknowledgement, and who owns confirming a message landed.** `acknowledg`, `acted-on`, `delivered-unread`: 0 occurrences. With a recipient that can be confidently wrong and can repeat what it was corrected on, the gap between "delivered" and "acted on" is where multi-agent coordination actually fails | L2, L3, L4, L5 | **absent.** §16 has Signals (`post`/`await`) and Rendezvous; neither distinguishes queued, delivered, read, acknowledged and acted-on, and neither says what happens to a queued message when the recipient dies |
| 5 | **Authority attenuation along delegation and spawn chains.** `attenuat`, `confused deputy`, `on behalf of` as an authority notion, `delegat`: 0 occurrences. §13 and §14 are entirely about authority and this is not in them | AC4, AC5, K14 | **absent**, and it is the one gap in a security-adjacent area. An agent spawning a child that calls rig has no stated authority relationship to it |
| 6 | **The state ownership matrix.** §18 says programs "are told explicitly what was lost", which is the promise; the table is the mechanism. Scattered per-subsystem prose means no reader sees the whole thing and programs guess | H3 | **partial.** Needs one table: a row per class of state a session can put into the daemon (declaration, lease, claim, seat, blackboard key by lifetime class, subscription, queued message, pending confirmation, detached operation, schedule, config) against every event it can survive (program restart, daemon restart, upgrade, reboot, holder death, seat succession), **with an answer in every cell including "nothing survives"**, and which side rebuilds what is not preserved |
| 7 | **Idleness as a failure, and progress as the health signal.** §18 measures health by responsiveness and treats idle as a rest state. **Every agent-specific failure is responsive**: idle, stuck, looping, parked on a question nobody sees | O1, O3, O4 | **absent.** Needs: health defined by evidence of progress; idle-and-not-blocked named as unhealthy with a threshold; a waiting session declaring **what** it waits for, so waiting is distinguishable from stuck; and renewal carrying a progress marker rather than being emitted by a background thread |
| 8 | **Durability of an agent's own work.** `checkpoint`: 0 occurrences. Treated nowhere as a mechanism, and the default outcome of every session is that its findings die with it | N1, N2, N3, N4, N5 | **absent.** Needs: a durable location assigned and seeded **before** the work begins; checkpointing at phase boundaries rather than at the end or in a termination handler; atomic checkpoint writes; **work not recorded is UNKNOWN, never done**; and child work persisted independently of its parent |
| 9 | **Intent records and successor re-issue.** `intent record`, `re-issue`, `ambiguous outcome`: 0 occurrences. A call whose reply was lost has an unknown outcome, and a successor cannot safely redo its predecessor's work without the record | U2, F3, Q1 | **absent.** §16 names the request id and session token that make a call exactly-once *from the client's side*; nothing survives the client |
| 10 | **At-risk detection and the escalation ladder.** `at risk`, `remaining budget`, `escalation`: 0 occurrences. A session that knows it is nearly out of context is the only actor that can say so | O2, O5 | **absent** |
| 11 | **"What a program must never assume", stated explicitly.** 0 occurrences | D6 | **absent.** A contract that never says what is *not* guaranteed is read optimistically |
| 12 | **The confirmation channel being out of band from the caller.** §14 and §13a route `confirm` to a window, toast or terminal and bind the answer to *"that one call, not the connection"*, which is the hard half and is right. **The rest is unstated**: `out of band` 0, `argument digest` 0, what happens when **no human is reachable** 0, replay-binding 0 | AB1, AB2, AB3, AB4 | **partial.** Needs: no argument, flag, header or second call can satisfy a confirmation; **deny when no human is reachable, not queue and not wait**; approval bound to one invocation by id and argument digest; and the prompt composed by the daemon from structured data with caller text shown only as marked, escaped, secondary content |

### The three counts the taxonomy asks for

| Count | |
|---|---|
| `foundational` mechanisms absent | **10 absent and 2 partial, of 12 examined.** Anything above zero is load-bearing. **This is a floor, not a count of 183** - the twelve were reached by triage from Part V's 22 entries and from the categories a density measurement pointed at, not by walking all 183. The earlier wording, "11 of 183", was both wrong arithmetic and a triage presented as an audit |
| Named but not defined to the bar | **CLOSED 2026-09-11.** All 22 of Part V's entries are adjudicated: four confirmed present (above), twelve became the gap rows, §13a's conflict resolution is `partial`, and **V13, V15, V18 and V20 are adjudicated in §36**. Record of the first seventeen: `logbook/projects/rig/count2-part-v-audit-2026-09-11.md`. **This count was open when §34 was written and the section said so; saying so is what let it be finished rather than forgotten** |
| Part IV confusion pairs this document conflates | **2 of 25.** #16 `confirmation` / `authorization` conflated outright, #15 `policy denial` / `precondition` / `validation` / `unconfirmed` partially. Record: `logbook/projects/rig/conflation-audit-2026-09-11.md` |

**The second count is open, and saying so is the point.** A pass that reported
only what it finished would be the same defect it is auditing.

### Added by the count-2 pass: house rules are not fully specified

**§13a states the conflict-resolution rule and states it well** - *"the most restrictive
wins: `deny` over `confirm` over `allow`"*, with first-match and most-specific both named
and refused because *"both fail open"*. A term sweep scored this section zero on every word
the taxonomy uses for it, and **the sweep was wrong**: the plan is ahead of the probe,
stating `deny-overrides` in its own vocabulary. That false positive is why every row in
this section was read before it was written.

**Four clauses of the same mechanism are unanswered**, and the fourth is the one that
matters:

| Clause | State |
|---|---|
| Evaluated on the exact effective **arguments** that will execute, with no re-parsing between decision and dispatch | **absent.** §13a matches on the pair `(caller, effects)`; arguments are never matched |
| Evaluation cannot error or block, and any failure is **deny** | **absent** |
| A decision names its matching rule, its **config layer** and the **remedy** | **partial.** `origin` carries `rule`/`elevation` and the rule id; layer and remedy are not recorded |
| **A program re-declaring a command with weaker `effects` is detected rather than trusted** | **absent** |

**The last row is an authority hole, and it is the same shape as the two already queued for
Boris.** `effects` is declared by the program, and house rules match on that declaration.
Nothing says what happens when a program re-registers a command at a *weaker* level - so a
rule written as `(agent, destructive) -> confirm` stops matching the moment the program
re-declares that command as `writes-files`. **No rule is violated and no denial is logged**,
because the pair simply stopped matching. `effects` being *"a floor, not an equality"* widens
it: one step down the enum drops every rule written at or above the old level.

**Traceable to** V17 and category AA, and it belongs beside gap 5 (authority attenuation):
both are authority leaking because nobody stated who may change the input to the decision.

### What this pass has not done

The `important` (290) and `situational` (34) mechanisms are recorded and not consolidated.
The AgentBox parity yardstick - a second, deliberately independent enumeration of what this
estate already has and must not lose - is
`logbook/projects/rig/agentbox-parity-2026-09-11.md`. **It has now been crossed with the
taxonomy: `logbook/projects/rig/taxonomy-parity-cross-2026-09-11.md`, and §35 carries the
result.** The twelve gaps above are listed in the order they were found, which is not a
ranking; **§35 is the ranking.** Four of its surfaces are already ruled out (below).

### Ruled out, 2026-09-11, by Boris

**Walkthroughs, assignments, artifacts and `request_review` are OUT** - 18 of AgentBox's
~40 tools. Dropped deliberately and on the record rather than discovered at the M16
cutover. See §29.
