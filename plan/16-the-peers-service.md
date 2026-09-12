## 16. The peers service

Isolation is the default (§14); coordination is a capability you opt into. This service is
**the best inter-agent coordination substrate we can build**, and once it is finished and
proved, every agent on this machine moves off AgentBox onto it. Decided 2026-09-10.

That is a high bar, so this section says what clears it.

### The structural advantage: one daemon means no consensus

There is exactly one rig per user. Every coordination operation therefore passes through a
single serialization point, which makes them **linearizable by construction**. No Raft, no
Paxos, no split brain, no clock skew, no quorum. Distributed coordination is hard because of
consensus; rig does not need consensus, so it can spend the entire budget on semantics and
observability instead. That is why this can be better than the general-purpose tools.

**Except that one serialization point orders *applied* operations, and linearizability is about
the client's view.** A `barrier.arrive()` that times out and is retried once releases a barrier
of nine with eight agents present. The fix is not in this service: it is the request id and the
session token on the wire (§5f), so every mutating call is exactly-once from the client's side
and every future primitive gets it free. Without those two fields the claim above is false on
any retry, and §5d forbids the stub from fixing it.

Durability comes from a write-ahead log: coordination state survives a daemon restart, and
clients reconcile on reconnect rather than losing their place.

### Time, named, because a laptop suspends

Every deadline in this service is **absolute, on `CLOCK_BOOTTIME`**, stored in the WAL as
`boot_id + boottime_deadline`. Never a remaining TTL, which a restart silently extends; never
the client's clock; never wall time, which a timezone change moves.

Go's monotonic clock does not advance across suspend, and this laptop suspends nightly, so the
two readings of "the same lease" differ by hours depending on whether rig restarted. A changed
`boot_id` means the machine rebooted, which is a different and simpler case.

**A resume grace epoch.** rig detects a gap larger than one TTL (BOOTTIME against MONOTONIC, or
logind's `PrepareForSleep`) and freezes expiry for one full TTL after resume, during which every
holder must re-present its token before its first guarded operation. Without it, a suspend
expires the whole estate's leases at once, on the machine this is built for.

§20 mandates `testing/synctest` for all timing, and synctest **cannot model suspend**. So this
class needs a real-clock test with an injected clock jump, or the Simulated gate below is
structurally blind to it.

### The primitives

**Presence.** What AgentBox does today, kept because it works.

| Call | What it does |
|---|---|
| `announce` | State a purpose and current activity; receive the crew as it stands |
| `activity` | Update what you are doing now. Cheap, non-blocking |
| `list` | Who is here, their purpose, activity, state, and what they hold or await |

**Mutual exclusion, done correctly.** This is where AgentBox is improved on rather than copied.

| Primitive | Why it is here |
|---|---|
| **Leases, not locks** | Every hold has a TTL and must be renewed. A dead holder expires automatically. There is no orphan state to detect and no human to break a lock |
| **Fencing tokens, scoped to what they can actually fence** | The classic bug: holder A stalls, its lease expires, B acquires, A wakes and writes anyway. Each acquisition returns a token, monotonic **per lease**, and every rig-mediated write must present it. **But a token can only fence writes rig mediates**, and every lease in this estate guards something rig does not own - git, a deploy, the VM, the desktop. The token is rejected while `make deploy` runs on regardless. Said plainly here, because the unqualified sentence will otherwise be quoted back later. **One exception now exists and it is narrow**: the display, once rig owns the only driver that can reach it (§5m). It is an exception because the resource is reached only through rig, which stays false of git, the deploy and the VM |
| **A liveness witness, and two-step expiry** | This is what makes the line above safe. `acquire` takes a witness: a pid or pidfd rig can poll, a cgroup, or the literal `unwitnessed`, recorded in the WAL beside the token. TTL expiry moves the lease to **ORPHANED, not FREE**; ORPHANED becomes FREE only when the witness is observed dead. An `unwitnessed` lease needs an explicit break, which is a recorded human action - AgentBox already works this way and discarding it would be a regression |
| **`rig peers run --lease=NAME -- make deploy`** | The primitive that makes the honest path the easy path. rig owns the child process, so the witness is exact and, critically, **expiry can kill the writer**. It is the only real fence for a resource rig does not own, which is all of them |
| **Read/write leases** | Many readers or one writer, because "everyone waits for everyone" is how a crew stops working |
| **Deadlock detection** | rig holds the wait-for graph and can see a cycle. It refuses the acquisition that would close one, naming the cycle, rather than letting two agents wait forever |
| **Semaphores** | At most N agents doing the expensive thing at once |
| **Barriers** | N agents wait until all arrive, then all proceed |
| **Leader election** | Exactly one agent runs the migration, and the others know who it is |

**Shared state.**

| Primitive | Why it is here |
|---|---|
| **Versioned blackboard** | Compare-and-swap on a key, with a revision per change. **A key may also be CLAIMED, with an owner and a liveness witness** - see "A claim has an owner and a liveness witness" below, which is the half of this primitive the estate actually uses |
| **Multi-key transactions** | Claim three things or none. Single-key CAS cannot express "divide this work" safely |
| **Watches with a cursor** | Subscribe from a revision. A client that reconnects gets what it missed instead of a gap it cannot detect. **One global revision, transaction-granular delivery**, so a multi-key claim is never observed half-applied |

**Work distribution**, which is what "divide the work so nobody doubles" actually needs.

| Primitive | Why it is here |
|---|---|
| **Claimable queues, at-least-once and it says so** | Claim a task under a lease, heartbeat it, and it is requeued if you die. **Duplicates are possible**: a task carries a mandatory `idempotency_key` and the consumer contract is replay-safety, or the task runs through the witnessed path above so requeue can kill the stalled worker before re-offering it. Requeue is the same two-step machine: EXPIRED → witness dead → REQUEUEABLE; witness alive → ORPHANED. One mechanism, three findings |
| **`duplicate_execution` is an event, not an error** | A stale `complete` is recorded on the timeline, so the post-mortem view can answer the question this primitive creates. Detecting it in the simulator is not the same as seeing it in production |
| **Rendezvous** | Hand a result to a named successor and park until it is collected |
| **Signals** | `post` and `await`. Park with nothing burned until a peer wakes you. Replaces every poll loop and every "check back in five minutes". **This is the primitive; the contract is "The message contract, from queued to acted-on" below** - five delivery states, a cursor with `gap: true`, topic families, and what happens to a queued message when the recipient dies |

**The human is a peer.** `ask` puts a question to whoever is actually present, routed to the
window, a toast, the terminal or a phone. That is AgentBox's most valuable single idea and it
is kept whole.

### What the agent-parallelism pass added, 2026-09-11

**Why this subsection exists.** Boris, 2026-09-11: the mechanisms that matter
are the ones that let **AI agents work in parallel** on one machine, so that rig
can run like AgentBox and the agents can then migrate off AgentBox onto rig.
**Not at parity - above it.** And explicitly *"not all AgentBox are must-have in
rig, some of its features have no place in the rig"*, so nothing below is here
because AgentBox has it; each is here because an agent working beside another
agent fails without it.

**The bar is this section's own**, set by the lease above: a primitive that does
not say what happens when the holder dies has been mentioned, not specified. The
five below are ordered by
`logbook/projects/rig/taxonomy-parity-cross-2026-09-11.md`, which crosses the
blind mechanism taxonomy against the AgentBox parity pass. **Three of the five
appear in both lists independently**, and the cross records which.

#### 1. A claim has an owner and a liveness witness

**The blackboard above has compare-and-swap, a revision per change, multi-key
transactions and watches with a cursor. It has no owner**, so a session that
dies holding `claims/chunk-3` leaves a chunk nobody will ever finish and nothing
says so. **The estate divides work with exactly this key shape**, so the half of
claiming that rig specified with a witness (the lease-backed queue) is not the
half anybody uses.

| | |
|---|---|
| **A key may be claimed, not merely written** | `own` records the claiming session **and its liveness witness** - the same witness the lease takes: a pid, a pidfd, a cgroup, or the literal `unwitnessed` |
| **A read reports the owner's liveness, not just the value** | **The same two-step as the lease, and deliberately the same words: OWNED, then ABANDONED when the witness is observed dead. Never silently FREE.** A claim whose owner is gone is visible as abandoned rather than indistinguishable from a healthy one |
| **Taking over an abandoned claim is one CAS write** at the version just read. No break, no human, no second mechanism |
| **An `unwitnessed` claim needs a recorded human break**, exactly as an unwitnessed lease does |
| **Values are NEVER trimmed** | The deliberate difference from signals. **Retention on a claim table hands one chunk to two agents**, which is the failure the whole primitive exists to prevent |
| **The cap refuses a NEW key rather than evicting an existing claim** | Under pressure, an eviction is indistinguishable from a completion to every reader. Refusing is loud; evicting is silent and wrong |

**Two opposite policies over the same event already exist in one daemon, and
nothing states which is intended where.** Measured 2026-09-11 by reading every
piece of state `internal/` holds:

- **The single-instance claim is engineered to VANISH on holder death.** It is
  an `flock` living on an open descriptor, so `kill -9` releases it and leaves
  no stale claim behind. Death frees it, deliberately.
- **The registry's name claims are engineered to be REFUSED to a successor.**
  A second registration of one identity is denied while the holder lives, and
  the holder's liveness is the connection.

**Both are defensible and they are opposites.** One says a dead holder's claim
should evaporate; the other says a claim should outlive the moment and be
defended. **This is gap 3 meeting gap 6**: the disposition of a claim on holder
death is the single most consequential decision in this design, and rig has made
it twice, differently, without writing either down.

**The rule this section adopts:** a claim that exists to prevent two actors
doing one thing at once (the single instance) may evaporate on death, because a
dead actor is not doing the thing. **A claim that carries identity a successor
needs (a registered name, a seat, a chunk of divided work) goes ORPHANED and is
inheritable, never silently free and never permanently refused.** The
distinction is whether anything downstream has to find the claim again.

**Why this is one mechanism and not two.** rig currently splits claiming: the
work queue claims **under a lease** with a heartbeat and a witness, and the
blackboard claims **a key** with CAS and no liveness at all. That split is why
the gap was invisible - the witness exists in the document, just not on the
surface anyone uses. **The witness belongs to the act of claiming, wherever it
happens.**

#### 2. The message contract, from queued to acted-on

**Signals were one sentence.** `post` and `await` is the primitive; the contract
is below. With a recipient that can be confidently wrong and can repeat what it
was corrected on, **the gap between "delivered" and "acted on" is where
multi-agent coordination actually fails**, and no existing wording distinguishes
them.

**Five states, and who owns each transition:**

| State | Owned by | Means |
|---|---|---|
| `queued` | the daemon | accepted and durable. **`delivered: 0` means nobody was parked, NOT that anything was lost** |
| `delivered` | the daemon | handed to a recipient's open subscription |
| `read` | the recipient | returned from an `await` into the recipient's own context |
| `acknowledged` | the recipient, **explicitly** | the recipient states it understood. Never inferred from delivery |
| `acted-on` | the recipient, **explicitly, and it carries the outcome** | the only state a sender may plan against |

**Nothing promotes a message on the recipient's behalf.** A sender that needs
`acted-on` asks for it and waits for it; a sender that does not, does not. The
failure being designed out is a sender treating `delivered` as agreement.

| | |
|---|---|
| **A cursor per subscriber, and `gap: true`** | When the cursor is older than retention the batch **cannot** be complete. **Treat what you were tracking as UNKNOWN, never as not having happened** - a silently incomplete batch is how two agents come to believe they each own one chunk. This is the lease's own best sentence applied to messages, and this section set that bar itself |
| **Everything since the cursor arrives in ONE batch** | Three events that fired while an agent was editing are one wake-up, not three missed ones |
| **Topic families** | a trailing `*` is a prefix, so one waiter covers a fan-out without naming its members in advance |
| **Directed messaging** | every seat names a private topic, so "message that agent" is an ordinary post and needs no second mechanism |
| **A bounded payload, and a named anti-pattern** | a payload where a pointer belongs. The bound is stated, and a message over it is refused rather than truncated |
| **The rider** | when an agent's area gains or loses a peer, or a lease it holds is broken, or a claim it owns is reclaimed, **the news is appended to the result of whatever call it makes next.** It needs no subscription and no decision to listen, which is the point: **it reaches an agent that did not know to ask.** A watch and a signal both require having already decided the thing was interesting |

**What happens to a queued message when the recipient dies, which is the
question nothing answered:** it is queued against the **seat**, not the session.
A session's death does not destroy it and a successor claiming that seat
receives it. **A message queued to a session that has no seat dies with the
session, and the sender is told so** rather than the message expiring silently.

#### 3. Seats, generations, and `HANDING_OFF` as a published state

**This is the one place rig overtakes AgentBox rather than catching up, because
AgentBox has no seats either.** The estate runs multi-seat teams today on a
naming convention over a blackboard key, a `state` field every seat agrees to
honour, and a signal topic every seat must spell identically. **None of that is
a mechanism. All of it is discipline, enforced by nothing.**

| Concept | Definition |
|---|---|
| **Seat** | a named role, independent of who occupies it. **The addressable identity.** Senders address seats; the daemon resolves to the current occupant |
| **Generation** | one occupancy of a seat by one session. Monotonic per seat. **A message carries the generation it was addressed to**, so a receiver can tell it is not the session the sender believes it is talking to - which is a check no amount of care can do today |
| **Lineage** | the chain of generations. A successor can prove it is the successor; a stranger cannot claim to be |

**The seat states, and the transitions:**

| State | Means | Sending to it |
|---|---|---|
| `VACANT` | nobody holds it | refused, and the sender is told the seat exists and is empty - not that the name is unknown |
| `LIVE` | one session holds it | delivered |
| `HANDING_OFF(successor)` | **two sessions legitimately occupy one seat**, the predecessor briefing the successor | **queued for the successor, not delivered to the closing session.** This is the whole reason the state is published |
| `ORPHANED` | the occupant's witness is observed dead | queued against the seat, delivered when it is next claimed. **Not discarded** |

**`HANDING_OFF` is legitimate and rig must stop reading it as a fault.** §5
refuses a second registration of one identity with `CODE_DENIED`, which is
correct for two unrelated programs racing for a name and **wrong for a
succession**.

**Measured 2026-09-11, by building both cases and comparing the strings: the two
refusals are BYTE-IDENTICAL.** Not similar - identical. The message carries the
**holder's** rendered identity and nothing whatever about the newcomer: not its
session, not its pid, not whether it declared the same commands as the holder or
a completely unrelated set. So a client has one code and one sentence, **and
neither varies with the case it is in.** It cannot choose between waiting for a
predecessor to finish closing, retrying, and failing outright - and neither can
a person reading the log.

**The identity that IS in the message is not a discriminator either.** It is the
holder's session id, minted per connection by the daemon and never sent to
anybody, so no caller has ever seen it. It is a label, not something a newcomer
can compare itself against.

**Two further facts a specification here has to know**, both pinned by test:
there are **two** duplicate refusals in the tree - the kernel's registration
guard and the daemon's routing map - and only the kernel's is reachable on this
path, so a change to one must account for the other. And **the prose half is
separable and cheaper**: §9 says a failed call never returns prose, and this
refusal returns exactly that, with all four structured fields empty. *"Wait for
the holder to go"* and *"take another name"* are different fixes and the wire
expresses neither. **That can be closed without ruling on succession at all**,
and it should be, because it is useful under either ruling.

**Death is the normal termination here, not the exception**, so `ORPHANED` is
the state a seat spends real time in and its behaviour is load-bearing rather
than a tidy-up path.

#### 4. Retraction

**An agent takes back an item it posted, before the human or the peer acts on
it.** Absent today under every wording.

**The reason it is a correctness property and not a convenience:** a warning
waits on a supervisor's screen until it is dismissed. **A "build failed" whose
build has since been fixed is a false statement rig is making on the agent's
behalf**, and it is worse than no notification at all because the supervisor
acts on it. §12's auto-dismiss is the *timer* half; the poster withdrawing a
claim that has become false is the half that was missing.

**A retraction returns what actually happened to the item** - never posted, seen
but not acted on, or already acted on - because those are three different
situations for the agent and only the daemon knows which.

#### 5. A lease name is registered with the resource it protects

**Two actors guarding one resource under two names are both unlocked and
neither finds out.** Non-deterministic actors invent plausible names as a matter
of course, so this is the normal case rather than the careless one.

**This estate has already paid the bill**: one session took `rig-makefile` and
another `repo:rig-shared-build` for the same file, and the file was found dirty
mid-edit. **Nothing else in the mechanism backlog has a recorded local
failure.**

| | |
|---|---|
| **A name is registered with what it protects**, and is discoverable before use. Asking "what guards this path" is a call, not a convention in a document |
| **An unregistered name is not silently honoured.** What the daemon does with one is stated: it is granted and **flagged as unregistered to both the holder and to `rig doctor`**, because refusing it outright breaks every ad-hoc use and honouring it silently is the bug |
| **Overlap is tested by SCOPE INTERSECTION, not name equality** | two names over one path collide and rig says so. Name equality is precisely the test that failed |

#### 6. Durability of an agent's own work

**The default outcome of every agent session today is that its findings die with
it**, and that is treated as a discipline asked of the author rather than as a
mechanism. It is a mechanism. An agent's context IS its memory, and the estate's
supervised sessions end by budget exhaustion far more often than by finishing.

| | |
|---|---|
| **A durable location is assigned and SEEDED BEFORE the work begins** | not chosen by the worker when it decides it has something worth keeping. A worker that has to decide where to write has already lost the case where it dies before deciding |
| **Checkpoints at PHASE BOUNDARIES** | not at the end, and **never in a termination handler.** A handler does not run on `kill -9`, on an out-of-budget stop, or on the container going away - which are the three ways this actually ends |
| **Checkpoint writes are atomic** | a half-written checkpoint is worse than none, because it reads as a record |
| **Work not recorded is UNKNOWN, never done** | the reader of a checkpoint store is told which phases have records and which do not, and an absent record is never rendered as an absent phase |
| **Child work is persisted independently of its parent** | a spawned worker's findings must not die because the session that spawned it did. Parent closure is a supervised event with obligations, not a cascade |

#### 7. At risk, and the escalation ladder

**A session that knows it is nearly out of context or budget is the only actor
that can say so.** Nothing else on the machine can see it: not the supervisor,
not the human, not a peer. So there is no monitoring answer to this, only a
declaration one.

| | |
|---|---|
| **A session declares AT RISK with a reason and an estimate** | budget, context, a deadline it will not meet. The reason matters because the remedies differ |
| **The ladder, and each rung names who decides** | **continue** (the session) → **checkpoint now** (the session) → **hand off to a successor** (the session, and it is the rung that must not need a human) → **stop** (a human, or the hard floor below) |
| **A hard floor, below which rig acts without asking** | because the rung that needs a human is the rung a dying session cannot reach. At the floor, rig checkpoints and marks the seat `ORPHANED` itself |
| **Announce-with-a-countdown, and it proceeds unless stopped** | the one interaction shape that costs the supervising human nothing when they agree, which is the common case. It is the ladder's escalation primitive and not a separate feature |

**Why this is ranked below durability and not above it.** At-risk detection
without durable work is a session announcing its own death and taking its
findings with it anyway. **Durability makes the announcement worth making.**

### Continuation slots, so an agent can hand off to itself

An agent that is about to reset its context - because the context is nearly full, not because
the work is done - needs to leave a note for the process that replaces it. Today that note goes
to the logbook: a path decision, a schema decision, a commit. That is right for work worth
keeping and wrong for a note whose whole lifetime is the minute between one context and the
next.

**A continuation slot is the versioned blackboard with a scope of one. It is not a new
service.** Same compare-and-swap, same revision per change, same watches with a cursor. What it
adds is three rules, and each exists because the obvious design fails without it:

| Rule | Why it is a rule and not a convention |
|---|---|
| **A slot carries a label, and the label is mandatory** | The replacement finds its slot by listing candidates and asking (below). `7f3a2b · 14:22 · 41 KB` cannot be chosen from, and an optional label is always missing exactly when there are three rows to pick between |
| **Existence is readable estate-wide; the payload is not** | A slot's label, age and owner are ordinary introspectable state (§14). Its contents are declared `sensitive`, so they are absent from every other reader's answer by the mechanism §15 already uses - not filtered by a second access-control door, which §14 does not have |
| **A slot expires, and the expiry is visible before it fires** | Two-step expiry, the same as a lease above. An abandoned reset otherwise leaves a row forever, and by the third week the question has eleven candidates and stops being read |

**How the replacement finds its slot, which is the only genuinely hard part.** A respawned agent
is a **new connection with a new client id**: §5's session token survives a *reconnect*, and a
context reset is not one - the token died with the context that held it. So nothing the old
agent knew can name the slot, and the replacement does not try to reconstruct it. **It lists
what is waiting and asks.** That is deliberately the mechanism the owner's own resume flow
already uses over the logbook - scan every candidate, newest first, with its title, and ask one
question when they conflict. Nothing is derived from the cwd, so two agents in one checkout do
not collide by convention, which is the failure that killed the two earlier designs.

**This inverts the isolation requirement, and the inversion is the point.** "Each agent's
clipboard is private" and "a respawned agent can list what is waiting" are only jointly
satisfiable one way: **isolate the values, not the existence.** Per-connection isolation would
hand the replacement an empty list, because it is a different client than the one that wrote -
so the listing is scoped to the uid, and privacy comes from the payload being `sensitive`
rather than from the row being hidden.

**What this does not claim.** It does not avoid a disk write - `rigd` writes slots to the WAL,
so they survive a daemon restart, which is the property that makes them worth trusting at all.
What it avoids is the *agent* having to choose a path, invent a schema and make a commit for a
note it expects to consume in ninety seconds. That is the whole benefit and it is smaller than
"no persistence", which is why it is written here rather than sold as a headline.

**On the CLI it is `rig continue`**, listing slots with label, age and owner, and `--take` to
claim one. Not `rig loose-ends`, which is taken and means declarations that stopped pointing at
anything (§8).

### The continuity record lives in §39

**It was written here and outgrew the section.** The peers service is
coordination between live sessions; the continuity record is a project's whole
documentation, state and standards, and at 250 lines inside another section it
was hiding in exactly the way §16's own bulk once hid four of his requirements.

**§39 is its home.** It depends on this section - seats, leases, the blackboard
and the WAL are underneath it - and it is not one of these primitives.

### Safety defaults, because agents get killed mid-operation

- Every lease has a TTL, and an absolute one on `CLOCK_BOOTTIME`. There is no infinite hold.
- Every `await` has a deadline. There is no infinite block.
- A lock is never held across a question to a human. rig refuses the combination. **One
  exception:** a `confirm` or an elevation that gates the lease-holder's *own* call is not a
  question across a lock - nobody else is blocked on the answer. Expiry is frozen on every
  lease that caller holds for the duration of the question, the same freeze applied after a
  suspend below (§13a, §14).
- Every primitive's crash semantics are written down and tested, not inferred.
- **What a contended acquisition reveals about other clients is written down in §14** and
  accepted there, rather than discovered later: BUSY proves a peer exists, and a refused
  acquisition names leases, never peers.

### Observability of coordination, which is where it wins

Nothing else in this class shows you what is happening. rig does, on every surface.

| View | Shows |
|---|---|
| Crew | Everyone, their purpose, activity, what they hold, what they await |
| Wait-for graph | Live, drawn. A cycle is visible before it is a mystery |
| Contention | Which lease is hot, who waited longest, queue depths over time |
| Timeline | Every coordination event, replayable, filterable by peer |
| Post-mortem | "Why was this agent blocked for four minutes" answered from the record, without anyone having been watching |

### What "fully proved" has to mean

The cutover from AgentBox does not happen on a feeling. Four gates, all of them tests.

| Gate | The proof |
|---|---|
| **Specified** | Lease, fencing and transaction semantics written as a model, with property tests generated against it |
| **Simulated** | Deterministic simulation testing: thousands of randomised interleavings with injected crashes, restarts and slow clients, asserting linearizability. Seed printed on failure so any violation reproduces exactly |
| **Adversarial** | The specific attacks: two holders of one exclusive lease, a stale fence token accepted, a watch that misses a revision, a claimed task run twice, a deadlock not detected. Each is a named test that must fail before the fix and pass after |
| **Shadowed** | Replaces "baked", which had two readings and the plan stated neither. During the shadow period **AgentBox remains the sole authority**: every coordination call still goes to it, and rig receives the same request in the same order and records what it *would* have decided. Divergence then means something, and there is never a second lock namespace over one resource. Cutover flips the authority, not the traffic - and **the dual-write path is in M7's ships column**, because it is the gate's only apparatus and it was previously in nobody's milestone |
| **Fenced** | The fifth gate, and the only one that tests the boundary rather than rig's own model. For every class of guarded external resource, name the enforcement mechanism - rig-owned pid, pidfd witness, cgroup kill - and prove by test that a stalled holder's **work stops**, not merely that its token is rejected. Without this the other four are self-referential |

**Simulated is only adequate if it injects the right failures**: lost replies (apply the
operation, drop the response, watch the client retry), a clock jump across suspend, and a
restart mid-transaction. Add to Adversarial: *an operation whose reply was lost is applied
exactly once when the client retries.*

Only after all five does the agent tooling point at rig. AgentBox stays running and untouched
until then, and stays available afterwards until a month has passed with no regression.

---
