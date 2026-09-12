## 37. Two estates, and the gate before rig develops rig

**Boris, 2026-09-11:** *"Once all agent-facing mechanisms will be ready in their
full glory, we'll want to deploy rig and use it in the development process of
rig, which may cause a problem since I defined there can be a single rig
instance at all times."* And, ruling the shape: *"I don't think we should allow
more than one production and one development."*

**This section exists so that moment is prepared for rather than discovered.**

### THE GATE MOVED, 2026-09-12, AND IT IS HIS OWN CORRECTION OF HIMSELF

**Boris, 2026-09-12, verbatim, overturning the bar quoted at the head of this
section:**

> *"We also must redefine the definition of our readiness of starting
> 'developing rig by using rig'. I originally asked for all the capabilities to
> be iron-clad ready and perfectly tested before we do it and I think it was a
> mistake. We can bring only a few of these to perfection before starting using
> them - and we can keep perfecting things as we go. Find among the capabilities
> we are planning to get in the ones that are the absolute minimum required to
> being benefit and only plan to bring them to the absolute perfection as I asked
> you."*

**What is superseded, exactly.** The opening quotation's *"Once all agent-facing
mechanisms will be ready in their full glory"* is **no longer the condition**.
Nothing else in this section is withdrawn by it.

#### THE GATE HAD TWO HALVES WELDED TOGETHER AND ONLY ONE WAS EVER LOAD-BEARING

**This is the diagnosis, and it is what made the wait open-ended.** The
preconditions below and the capability bar were written as one gate. They answer
two different questions and only the first has a floor under it:

| | Question | Can it be partial? |
|---|---|---|
| **The preconditions** (the table below) | **can two estates coexist without corrupting each other?** | **NO.** A half-built isolation boundary is not a smaller boundary, it is an absent one. Five of six are DONE |
| **The capability set** | **how much of a seat's day runs through rig rather than AgentBox?** | **YES, and it always could.** Every capability is independently useful and independently cut over |

**Welding them made the second inherit the first's all-or-nothing.** Separated,
the first is nearly finished and the second starts paying the day its first
capability lands.

#### THE MINIMUM BENEFICIAL SET, and the test that produced it

**The test applied, stated so it can be disagreed with:** *what must exist for a
seat to run one complete workflow through rig without falling back to AgentBox
part-way?* **Not "what would be nice", and not "what is cheapest".**

**Why that test and not "most valuable first".** The failure mode of a partial
migration is **split-brain**: if one seat takes a lock in rig while another takes
the same lock in AgentBox, the lock protects nothing and neither seat finds out.
So the migration unit is a **whole capability moved by every seat at once**, and
a capability that cannot carry a seat end-to-end cannot be moved at all.

**THE EVIDENCE IS MEASURED, NOT REASONED.** `BACKLOG.md`'s table *"M7, ordered by
what this team ACTUALLY uses today"* records what three seats reached for across
one full day on AgentBox, plus the three absences a seat actually felt. **That is
the ranking below**, with the front door added because nothing is reachable
without it.

| # | Capability | Why it is in the MINIMUM | Where specified |
|---|---|---|---|
| **0** | **The front door** - the MCP surface, plus `rig.estate` so an agent can read WHICH estate it reached | **an agent cannot call rig at all without it, and cannot tell production from development.** Precondition to every row below rather than a peer of them | M2, §37.1 |
| **1** | **Presence and seats** - announce, roster, seat as the addressable identity, `partial` honestly reported | **nothing below is addressable without it.** Every seat announced; `partial: true` is what stopped a seat concluding it was alone | §16, seats |
| **2** | **Named leases with a REGISTERED scope** - witness, two-step expiry, overlap by intersection | **highest measured pain.** One Makefile under two lock names, both unlocked, neither seat found out. Registration and intersection are not polish here: they are the defect | §16, leases + ¶5 |
| **3** | **The versioned blackboard: CAS, claims, owner liveness** | **the seat registry itself runs on it.** Three seat handoffs resolved through `if_version` in one day | §16, blackboard + ¶1 |
| **4** | **Signals, with `await` and a cursor** | every handoff used one. **The alternative is a poll loop**, which is the thing this replaces | §16, signals |
| **5** | **Directed messaging to a SEAT, carrying the GENERATION** | every ruling crossed a seat boundary as a message. The generation closes a defect **Boris raised himself** - *"peers should be aware they may be contacted wrongly thinking they are the successor"* (2026-09-11) | §16, ¶2 + ¶3 |

**Rows 2 and 5 each absorb one of the three absences a seat reported feeling.**
That is deliberate: an absence a seat felt under load is stronger evidence than a
usage count, because the seat was trying to do the work when it found the hole.

#### WHAT IS DELIBERATELY OUT OF THE MINIMUM, with the reason

**A list where nothing is excluded is a list that was not applied.** These are
planned, specified and NOT in the first cutover:

| Excluded | Why it can wait |
|---|---|
| **Durability of an agent's own work** (§16 ¶6) - the third felt absence | **a working substitute already exists and is load-bearing today**: the logbook, the handoff documents, and `agent_persistence.py`. Painful, not blocking. **The strongest candidate for the second cutover** |
| Claimable queues, rendezvous, barriers, semaphores, leader election, deadlock detection | **no seat reached for one in the measured day.** Real capabilities with no evidence behind them yet |
| Continuation slots, retraction, the at-risk ladder | valuable, and each needs presence and leases underneath it. **They are cheaper after the minimum lands, not before** |
| `rig peers run --lease` | the only real fence for a resource rig does not own, **and the minimum set guards documents rather than deploys.** It arrives with the first resource that needs fencing |

**THIS SET IS PROVISIONAL AND ITS SUCCESSOR IS ALREADY COMMISSIONED.** Boris has
ordered an expert study - see the backlog's dated item - to derive the
minimal-but-optimal set from the typical needs of the seats working on rig.
**This table is the working answer until that study lands, not a ruling it must
respect.** It exists so work can proceed now; the study exists so the answer is
right.

#### "PERFECTED" IS NOT A FEELING, AND THIS IS THE FLOOR IT MEANS

**Only the rows above are taken to the bar Boris set. Everything else ships to
the ordinary standard of this repository.** The bar, applied per capability:

| Clause | Test |
|---|---|
| **At least as good as AgentBox's equivalent** | the equivalent named, and the comparison RUN, not argued. A lease without a witness is weaker than AgentBox's and therefore not done |
| **Its failure semantics are demonstrated, not specified** | the holder dies, the daemon restarts, the cursor falls off retention - each shown happening, with what a reader was told |
| **A conformance case that BITES** | a mutation proves the test fails when the mechanism does |
| **Two estates, live, exercised** | the capability used across production and development without either seeing the other |

**A capability that has not cleared all four is not cut over**, and the seats
keep using AgentBox for it. That is the whole discipline and it needs no other
enforcement.

### THE MIGRATION IS STAGED, PER CAPABILITY, AND BOTH SYSTEMS RUN AT ONCE

**Boris, 2026-09-12, verbatim:**

> *"In doing so, we'll gradually move from using AgentBox to using only rig in
> developing rig. We'll have the agents use AgentBox for features not yet
> implemented in rig and use the production instance of rig for all capabilities
> that we bring to the required perfection."*

**So there is no cutover day.** There is a per-capability move, and the estate is
mixed until the last one lands. Stated as rules because a mixed estate is exactly
where split-brain lives:

| Rule | Why |
|---|---|
| **A capability lives in exactly ONE system at a time, estate-wide** | the lock taken in two places is no lock. **This is the rule that makes a mixed estate safe** and it has no exceptions |
| **A capability moves when it clears the four clauses above, and not before** | otherwise "perfected" decays into "shipped" |
| **The move is announced to every live seat and recorded in `DECISIONS.md`** | a seat that did not hear is a seat still holding the old mechanism |
| **AgentBox stays authoritative for everything not yet moved** | it is not deprecated, it is the incumbent. **No seat improvises a replacement** |
| **A capability may move BACK** | if a moved capability is found weaker than the AgentBox one under real load, it returns and the finding goes to the backlog. **Recorded, never quiet** |

**The migration order is the table above**, and each move updates
`READINESS.txt` - see below.

### THE TWO FLAVOURS, AND WHO MAY DO WHAT TO EACH

**Boris, 2026-09-12, verbatim, and it settles the lifecycle question this section
left to convention:**

> *"rig comes with two flavours: production and development (each with its own
> system-tray icon) the production is to be loaded with the system start and to
> be upgraded in convenient times when it's not used and new capabilities are
> ready to be deployed. And the development flavours the peers can start, stop
> and experiment on as much as they need to in any way they like."*

| | **production** | **development** |
|---|---|---|
| **Starts** | **at login, by the `systemd --user` unit** (§5l, `packaging/rigd.service`). Nobody starts it by hand | **by a seat, whenever it wants** |
| **Stopped by** | **nobody, in the ordinary case.** Only inside a deliberate upgrade window | **any seat, freely, without asking and without announcing** |
| **Upgraded** | **only when idle AND a capability has cleared the four clauses.** A deliberate act with a window, never a side effect of a build | **continuously. That is what it is for** |
| **Broken** | never knowingly | **expected.** Breaking it is the work |
| **Tray icon** | **its own, distinguishable at 22px from development's** (§11) | **its own** |
| **Runtime dir** | **the DEFAULT `XDG_RUNTIME_DIR`**, so the unit cannot reach development by construction | **always placed explicitly** |
| **Carries** | only capabilities that cleared the bar | anything, including half-built work |

**THE UPGRADE WINDOW IS A MECHANISM, NOT A COURTESY.** *"Upgraded in convenient
times when it's not used"* is his wording and it has a testable form: **an
upgrade is refused while any seat holds a lease, a claim, or an unread directed
message in that estate.** The estate knows all three, so this is a check rather
than a convention, and it is the first real consumer of presence.

**WHY PRODUCTION MUST NOT BE STOPPED CASUALLY, said here because the reason is
not obvious:** by the time production carries the minimum set, **stopping it
breaks every seat's coordination at once** - their leases, their seat registry,
their parked `await`s. Development exists precisely so that nobody ever has a
reason to.

### The rule

**A user runs AT MOST TWO NAMED ESTATES: `production` and `development`.** Not
one, and not an open namespace of N.

| | |
|---|---|
| **production** | what the estate's agents coordinate through. Long-lived. Upgraded deliberately |
| **development** | where rig is built, restarted, broken and tested. Disposable by design |

**Why two rather than N.** §16's entire argument is that every coordination
operation passes through a single serialisation point *per estate*, which makes
them linearizable by construction. Two estates that share no state tree do not
weaken that; an open namespace invites a third that somebody forgets, and the
question "which estate is authoritative" stops having an answer.

### Ephemeral estates are NOT a third estate, and this is the clause that keeps the tests alive

**The rule binds NAMED estates.** Every test in this repository and every
reproduction recipe in the handoff starts an anonymous estate in a temporary
`XDG_RUNTIME_DIR` and tears it down. **Those are not deployments and the rule
does not reach them.** Stated explicitly because "at most two" read literally
would forbid `make ci`.

**So the enforceable form is: a NAMED estate refuses to start when its name is
already held**, which is the existing `flock` keyed by name rather than a new
mechanism. An unnamed estate claims no name and collides with nothing.

### What is already true, demonstrated rather than argued

**Measured 2026-09-11 on this machine, two `rigd` from one binary as one user:**

| Checked | Result |
|---|---|
| Two estates in two runtime directories | **both serve.** Sockets `srw-------` |
| A second `rigd` in an estate's own directory | **refused, naming the incumbent pid** |
| A program registered in one | the other answers `no programs are registered` |
| **`rig down` on one** | **the other is untouched and still answering** |

**`rig down` already targets one estate**, which is why it retired `pkill -x
rigd` - that killed every estate on the machine, and it is the reason the verb
was pulled forward from M6.

### The ISOLATION preconditions, and NONE of them may be skipped

**RENAMED 2026-09-12, and the word added is the whole correction.** These six
answer one question - *can two estates coexist without corrupting each other?* -
and that is the half of the gate that genuinely cannot be partial. **They are
NOT the capability bar**, which is the minimum beneficial set above and which
moves one capability at a time. Welding the two is what made the wait
open-ended; see "THE GATE MOVED" at the head of this section.

**NARROWED 2026-09-12, BY BORIS, AND THE CONTRADICTION IT FIXES WAS REAL.**
This block used to say all six must land before the first cutover. **Items 2
and 4 build at M7, alongside the WAL and leases** - which are rows 2 and 3 of
the minimum beneficial set. So "the gate needs all six preconditions plus rows
0 and 1" could not be satisfied without also building rows 2 and 3, and the two
halves of the gate contradicted each other. `READINESS.txt` reported the first
cutover as 1-2 weeks on the basis of the reading that was wrong.

**THE RULING: the gate fires on preconditions 1, 3, 5 and 6.** Items 2 and 4
are deferred to the cutover of the first capability that HOLDS PERSISTENT
STATE, which is where they become load-bearing and not before.

**The mechanism argument, so this is not merely convenient.** Precondition 2
keys persistent state per estate and precondition 4 publishes an epoch so a
restart is distinguishable from a blip. **Presence - row 1, and the first
candidate for cutover - holds no persistent state and survives no restart by
design**: an occupant lives exactly as long as its connection. There is no
state for item 2 to key and no handle for item 4's epoch to stamp. **A
precondition that cannot be violated by the capability being cut over is not
protecting that cutover.**

**AND THEY HAVE FIRED. THE CAPABILITY IS §39, NAMED 2026-09-12.** The continuity
record holds a project's documentation, state and standards across daemon
restarts and across releases, so **item 2 keys it per estate and item 4 stamps
its provenance with an epoch.** They are §39's preconditions now rather than the
cutover's, and the 1-2 seat-days they were always owed sit there.
`READINESS.txt` carries it.

**WHAT THIS DOES NOT SAY.** Items 2 and 4 are NOT cancelled and NOT weakened.
They gate the WAL, leases and the blackboard, and any capability holding state
across a restart needs both before it moves. **The first capability that
persists anything re-arms them**, and a seat proposing such a cutover checks
this list again rather than citing this paragraph.

The gate in §24 fires when preconditions 1, 3, 5 and 6 have landed **and the
first capability of the minimum set has cleared its four clauses** - both
halves, because an isolation boundary with nothing running through it proves
nothing.

| # | Precondition | Where it lands | State |
|---|---|---|---|
| 1 | **Estate identity in the protocol.** A name and a role (`production` / `development`) an agent can READ | §14, and the wire | **DONE 2026-09-11, both halves.** On the wire as `rig.estate` (`1177f80`); reachable from a terminal as `rig estate` (`9e8b9fc`, ratchet `a3c97e1`). **Demonstrated on three live estates** - unnamed, `production` and `development` - each isolated in its own `XDG_RUNTIME_DIR`, plus the no-daemon refusal through the shared renderer and completion offering the verb. **Zero and `UNNAMED` render differently on both surfaces, asserted by a test that fails if they ever match**, and role and name are asserted to travel together or not at all |
| 2 | **State scoped per estate.** Config, storage and the call log keyed by estate, not by uid | **M7, where the WAL lands - CORRECTED 2026-09-11.** It said M5, where storage lands; §16's coordination service carries its own write-ahead log and that is the first persistent state rig holds | **SPECIFIED 2026-09-11, builds at M7 - CORRECTED 2026-09-11.** This cell said M5 while the cell to its left said M7, one row naming two milestones; M7 is right, because the WAL is the first persistent state rig holds. Estate-scoped state lives under `$XDG_STATE_HOME/rig/estates/<name>/`, extending the subtree `paths.EstateLock` already keys by name. The shared root keeps exactly one tenant, the cross-estate name claim. **An unnamed estate gets no persistent state at all**, and that is the answer rather than an omission - see below |
| 3 | **Build and semantic skew is detected, not discovered.** A client built from the development tree talking to the production daemon is refused or warned | §21, and **the wire** | **RE-MEASURED 2026-09-11, and it is TWO different gaps wearing one row - see below.** For a PROGRAM the fields already exist and nothing reads them. For a TERMINAL or an AGENT there is no handshake to carry them at all. **Batched with row 1 as one wire change** |
| 4 | **A restart is survivable and distinguishable from a blip.** Epoch handles, two-step lease expiry with witnesses, and `owner_gone` | M7, §16 | **ruled, unbuilt, and the gap in the ruling is CLOSED 2026-09-11.** V15 rules the daemon publishes an epoch and every handle carries it. The row used to end *"ruled for crashes; a deliberate self-upgrade is the SAME event and nothing says so"*. It says so now: **the epoch is bumped on every start, unconditionally, and nothing distinguishes a planned restart from a crash** - see below. The BUILD is still M7 |
| 5 | **The `systemd --user` unit manages production ONLY.** The development estate is never under it | M6, §5l | **SPECIFIED 2026-09-11, builds at M6.** **`production` owns the DEFAULT `XDG_RUNTIME_DIR` and `development` is always placed explicitly**, so a unit that sets nothing cannot reach development by construction rather than by a flag it might omit. **The unit carries NO `ExecStop` AT ALL - CORRECTED 2026-09-11.** This row used to say `ExecStop` is `rig down`. It is struck, 3-to-0: §5l's own heading (*"its unit must not carry an `ExecStop`"*), §5l's body (*"Do not give it an `ExecStop`, and this is not a style note"*) and §23's two M6 rows all say no. **This row was never an independent fourth vote** - its author confirmed it wrote from a quotation of §5l reproduced inside this very cell, and never opened §5l, so it had the scar and not the rule. `rig down` is the same shape as the `agentbox quit` that caused the scar: single-instance-by-flock plus auto-spawn makes the start command exit 0, systemd thinks the service finished, and `ExecStop` kills the healthy daemon already serving |
| 6 | **A named estate refuses a name already held**, and says which name and which pid | §5f | **BUILT 2026-09-11 (`2a110c2`), and the name set closed at two (`76e2d86`).** The claim lives under `XDG_STATE_HOME`, not `XDG_RUNTIME_DIR` - two estates differ exactly in their runtime directory, so a claim beside the socket would refuse nothing. **An unnamed estate claims nothing**, which is the ephemeral-estates clause holding by construction; verified live rather than assumed, its state directory empty |

### Three preconditions specified ahead of the milestone they build at, 2026-09-11

**Boris, asked whether to pull this work forward:** *"If it is architecturally
smart: do it."* **Three of the four open preconditions are decidable now and one
is not.** The test applied to each was not whether it could be written down, but
whether the decision is cheaper before the code exists or after. For three it is
strictly cheaper before, because the code that would otherwise have to be
revisited has not been written yet. **The fourth is refused below, with its
reason**, because a list where everything qualifies is a list that was not
applied.

#### Precondition 2: `$XDG_STATE_HOME/rig/estates/<name>/`, and an unnamed estate gets none

**The mechanism already exists and this extends it rather than inventing one.**
`paths.EstateLock` is `$XDG_STATE_HOME/rig/estates/<name>.pid` today, so the
`estates` subtree is already the idiom and the estate name is already the key.
Config, storage and the call log go under `estates/<name>/` when M5 lands them.

**The shared root keeps exactly one tenant, and that is a rule rather than an
observation.** `StateDir`'s own comment says what the root is for: a name claim
only refuses a duplicate if both estates can see it, and the one place the
runtime directory does not reach is outside the runtime directory. **Anything
placed at that root is by construction visible to both estates**, which is the
property the claim needs and precisely the property everything else must not
have.

**An unnamed estate therefore has NO persistent state, and that is the answer
rather than an omission.** Its name is the empty string, `ValidEstateName`
refuses the empty string, and so there is no subtree for it to be keyed to.
**This falls out of the ephemeral-estates clause above rather than contradicting
it:** an ephemeral estate that persisted across runs would be a third estate
arriving by the back door. A test that needs state gives itself a name, or does
without.

**Why it could not wait for M5.** The row said *"not specified, and it is the
one that bites silently"*. Specified at M5 this is a rule applied to storage
call sites that already exist; specified now it is the rule the first one is
written against. **The failure it prevents is silent in both directions** - the
development estate writing into production's store raises nothing at all - so
the moment to decide it is the moment before there is anything to decide it
about.

#### Precondition 4: a deliberate restart publishes a new epoch exactly as a crash does

**The epoch is bumped on every daemon start, unconditionally, and nothing may
distinguish a planned restart from a crash.**

**Why the asymmetry decides it.** A handle that trusts a restart because it was
told about that restart in advance is a handle trusting a claim, and it breaks
the first time the claim is wrong. **The cost of treating a planned restart as a
crash is one re-acquisition. The cost of the reverse is a fencing token that
outlives the thing it fences**, which is the failure leases exist to prevent.

**This is the case rig's own development creates constantly and no other user
does.** Rebuilding and restarting the development daemon is the inner loop of
self-hosting, so the estate that walks this path most is the one running the
work. A rule written only for crashes would have been tested by the crash case
and used by the restart case.

**The BUILD is unmoved and still M7.** Epoch handles need leases, leases need
presence, and presence is M7's content. What closes here is the specification.

#### Precondition 5: production owns the default runtime directory, so the unit cannot reach development

**`production` takes the DEFAULT `XDG_RUNTIME_DIR`; `development` is always
placed explicitly.** A `systemd --user` unit inherits the ordinary environment,
so a unit that sets nothing reaches production and **cannot reach development by
construction rather than by a flag somebody might omit.** Reversing the two
would have the unit manage development by accident, which is why which estate
takes the default is a decision and not a convention.

**STRUCK 2026-09-11. The unit carries no `ExecStop` at all**, per §5l's heading and body. The superseded text read *"`ExecStop` is `rig down`, never a signal to a pid"* and is kept struck rather than deleted because the reasoning below shows how it was reached: it treated "never a signal to a pid" as §5l's lesson, when §5l's lesson is that the unit must carry no stop command whatever its form. `agentbox quit` was not a signal either, and it is what did the damage. Original text follows.

**~~`ExecStop` is `rig down`, never a signal to a pid.~~** `rig down` needs no
estate argument at all - `main.go` states the reason, that two estates are two
runtime directories, *"so a stop needs no estate name and there is nothing to
get wrong"* - and it is already per-estate, which is why it retired `pkill -x
rigd`: that killed every estate on the machine.

**§5l's scar is what this is shaped against.** An `ExecStop` killed the healthy
daemon it managed, because single-instance-by-flock plus auto-spawn makes the
start command exit 0. **rig has the identical shape and a second estate is
exactly the condition that fires it**, so the unit never learns a pid and never
signals one.

#### Precondition 3 is NOT specified here, and the reason is a measurement that has not landed

**It has an open measurement and new evidence from the same day, and a rule
written now would be written about the wrong half.**

§37 already leaves open **whether a daemon-level semantic generation floor is
coherent at all**, put to backend-1 with the shape and deliberately not decided.
And on 2026-09-11 backend-2 produced evidence that moves what *detected* has to
mean: **`effectsLabel` and `durationLabel` in `cmd/rig` render an enum value
outside the descriptor as a bare decimal**, so a newer daemon's fifth effect
prints as `4` beside four words. **That is skew DISCOVERED, by a human squinting
at output, at the CLIENT** - and this row has only ever been written about the
wire.

**So the row is two gaps in two layers and only one of them was known when it
was written.** Specifying it now would settle the wire half and leave the client
half to be found again later, which is the shape this section exists to prevent.

### `rig.estate`, the fourth self-method, and why items 1 and 3 are ONE change

**Specified 2026-09-11 by the team-lead seat, after walking §14 rather than
reasoning from the rows.** Preconditions 1 and 3 were written as separate items
and they are the same gap reached from two directions.

#### The walk, which is what produced this

| Caller | Says hello? | What it can learn about the DAEMON |
|---|---|---|
| a **program** | yes | `HelloResponse` carries `wire` and `daemon_version`. **Both are populated and NEITHER IS EVER READ** - `grep GetWire()/GetDaemonVersion()` over `client/`, `cmd/rig/` and `internal/` finds only the daemon's own config plumbing |
| a **terminal** | **no, and it must not start** | **nothing.** It never receives a `HelloResponse`. All it sees is `PingResponse{nonce, program, version}`, and that `version` is *the answering program's* - `rig ping fakeapp` reports fakeapp |
| an **agent** on the MCP socket | no | **identical to a terminal**, because it connects as an ordinary unregistered client. That is what the socket is for |

**So precondition 3 is two gaps, not one.** For programs it needs a
**consumer**, not a field. For terminals and agents there is **nowhere to put
one**, because the carrier the section assumed is a handshake they never
perform. The row previously said "this needs a NEW FIELD"; that is true of one
caller kind and false of the other, and the distinction is the whole design.

**And precondition 1 has the identical shape.** Boris's *"peers will know which
rig instance is which"* requires an AGENT to read the estate, and an agent is
terminal-shaped. Same callers, same missing carrier. **One change.**

#### The shape

**A fourth method on rig's own surface**, beside the three that exist:

| Method | Answers |
|---|---|
| `rig.hello` | the program handshake |
| `rig.ping` | liveness, with the program as an argument |
| `rig.programs` | the caller's scoped view |
| **`rig.estate`** | **who you just reached** - the estate NAME, its ROLE (`production` / `development` / unnamed), the daemon BUILD, the wire major, and the semantic generation floor |

#### The two alternatives, and why each is refused

- **A field on `PingResponse`. No.** It is a liveness probe with a nonce and it
  answers **from the program addressed**. Estate identity and daemon build are
  facts about the DAEMON. Hanging them off a per-program probe is exactly the
  conflation §21 exists to catch, and it would make `rig ping fakeapp` report
  two things about two different subjects in one message.
- **Make terminals handshake. No, and this one is refused on §14's own terms.**
  §14 says `scoped` is *"the whole authorisation state a connection carries: one
  boolean, set here, with no other way to become either kind of caller"*. Giving
  a terminal a hello makes that boolean ambiguous, and it is load-bearing for
  every authorisation decision in the estate. **A precondition must not weaken
  the model it is a precondition for.**

**A method is readable by every caller kind without changing what any of them
IS**, which is the property both alternatives destroy.

#### Precondition 3 is THREE halves, not one, and only one has nothing behind it

**Measured by backend-1 2026-09-11, verified by the lead.** The row read as one
missing capability. It is three facts in three different states:

| | State |
|---|---|
| **per-program semantic skew** | **fields EXIST and are READABLE.** `semantics_gen` is on the wire and `rig apps list --json` already returns it for a registered program. Missing only a CONSUMER |
| **the daemon's own semantic generation** | **exists and is UNREACHABLE.** `self.go` declares rig with `SemanticsGen: 1`, and `registry.go` keeps `self` deliberately out of `programs` - *"it has no owner, no session and no scope, because it is not a registration"* - so the estate projection never reaches it. **rig is absent from its own estate** |
| **the daemon's build** | **already reachable, and UNDISCOVERABLE.** `rig ping rig` replies `Program: SelfID, Version: d.version`, which is rigd's own build. Reachable only by a caller that already knows to address the probe to `rig` |
| **the estate's name and role** | **does not exist anywhere.** The only one of the four with nothing behind it |

**So `rig.estate` is a CONSOLIDATION for three of these and a new capability for
one.** That is a weaker and more honest claim than the row made.

**And the two generations must not be conflated, which is what the row did.**
`EstateResponse.semantics_gen` is **rig's OWN** generation - "what semantics does
this daemon implement", which a client built from the development tree needs.
**It is NOT a per-program skew detector.** "Has `fakeapp`'s meaning changed under
me" is per-program, already on the wire, and belongs on the **capability map**
(M2 slice 4). Two halves, two places, and the proto comment says so.

#### The wire shape

    message EstateRequest {}

    message EstateResponse {
      string name           = 1;  // "" when the estate is unnamed
      EstateRole role       = 2;  // DERIVED from name by the daemon, never stored
      string daemon_version = 3;  // the same value `rig ping rig` returns
      string wire           = 4;  // the major, from the daemon's config
      int32  semantics_gen  = 5;  // rig's OWN generation, from self.go. NOT per-program
    }

    enum EstateRole {
      ESTATE_ROLE_UNSPECIFIED = 0;  // nothing was said (section 21). Never a fact
      ESTATE_ROLE_UNNAMED     = 1;  // deliberately unnamed: a test, a build, an
                                    // ephemeral run. THIS is the fact
      ESTATE_ROLE_PRODUCTION  = 2;
      ESTATE_ROLE_DEVELOPMENT = 3;
    }

**`EstateRequest` is empty and still exists**, for `ProgramsRequest`'s reason: a
method with no request message cannot later gain an argument without a wire
break.

**Why `UNNAMED` gets a number of its own rather than sharing zero.** Proto3
cannot tell an unset scalar from a zero one, so a daemon that has the field and
fails to set it renders as *"this is an unnamed estate"* - and **an unnamed
estate reads as ephemeral and disposable where production does not.** The safe
guess and the useful guess point opposite ways, which is `answerJSON.Partial`'s
argument in a different file. **Zero must mean unset, always, because the wire
cannot say otherwise.**

**Why name AND role, when the closed set makes them equivalent.** Because they
are equivalent *today* and the set is reopenable. **With name alone, the
derivation `name == "production"` lives in every consumer** - every agent, every
script, uncountable and outside this project's control - and reopening the set
breaks all of them silently. **With both, the derivation lives in the daemon,
once.** The "two places to disagree" objection is answered structurally rather
than by discipline: **role is never stored and never accepted as input**, but
computed from the name at reply time, so there is no second place to write.

#### The section 14 walk, which is required rather than a review comment

**`rig.estate` is UNSCOPED. Every caller kind gets the same full answer.**

| Caller | Gets |
|---|---|
| an agent Boris runs, unregistered | **full** - and it is the motivating caller |
| a terminal, the window, the tray | **full** |
| a script, a `make` target, anything from his shell | **full** |
| **a registered program** | **full** |
| an HTTP client | full, and deferred with the HTTP surface |
| a scheduled fire, a house rule, an internal timer | no connection, so rig's own principal - named explicitly or refused (§13a) |

**Why it is not a granted surface**, which is the question this section's own
rule forces. §14 says *"any surface whose product is an estate-wide AGGREGATE is
by definition a granted surface"* and names seven. **`rig.estate` aggregates
nothing**: every field is a fact about the DAEMON ITSELF and none is data
belonging to another principal. Scoping restricts which PROGRAMS a client sees,
and the estate is not a program.

**So it is the first rig method whose answer does not depend on who is asking.**
That is the property that makes it safe without a grant, and it is the property
a later field would quietly destroy - **anything added to `EstateResponse` that
varies by caller turns this into a granted surface and re-opens this walk.**

#### What is still open, and it is a worker's measurement rather than a ruling

**Whether a DAEMON-level semantic generation floor is coherent at all.**
`semantics_gen` is validated per declaration and hashed into the capability
digest, which is per PROGRAM. If generation is only ever per-program then
precondition 3's semantic half belongs on the **capability map** and not on
`rig.estate` - which moves it into slice 4 rather than into this change. Put to
backend-1 with the shape; not decided here.

### Why item 1 is the one Boris asked for by name

**Boris, 2026-09-11:** *"The peers (all agents) will know which rig instance is
which and will know when to be talking to which and for what purposes."*

**An agent cannot know that today.** `XDG_RUNTIME_DIR` is inherited from whatever
launched the agent, so the estate is something an agent *is placed in*, never
something it *reads*. **Routing deliberately requires the estate to answer for
itself**, which makes this a wire and §14 change rather than a convention. A
convention over two directory paths is what we have now, and it is exactly what
fails the first time an agent is launched from the wrong shell.

### Using rig to develop rig is an INSTRUMENT, and every seat is the instrument

**Boris, 2026-09-11:** *"all peers are to be instructed to be on the lookout for
the way using rig benefits them or making their life harder and propose
features, bugs and improvements to be added to the rig backlog."*

**This is the reason self-hosting is worth its cost, and it is not a side
effect.** rig exists so that sessions like the ones building it can do a good
job. **They are therefore its best requirements source, and the only one that
meets it under load** - a seat that has just lost an hour to a missing mechanism
knows something no design review produces.

**The obligation, on every seat, from the moment the gate is crossed:**

| | |
|---|---|
| **Report what rig DID for you, and what it did TO you** | both directions. A mechanism that saved an hour is evidence as much as one that cost an hour |
| **Evidence, not wishes** | *"I derived this by hand three times today"* is a proposal. *"It would be nice if"* is not. This bar is §36's and it does not relax because the reporter is inside the project |
| **To the team-lead, never straight to the backlog** | the lead ATTACKS a proposal rather than collecting it: is it already specified, who adopts it, is it domain logic, what is the evidence, what does it cost |
| **The lead does not review its own** | it puts its proposals to a worker, or to Boris |

**Only what survives the argument enters the backlog, with its evidence and its
adopter recorded beside it**, so the argument is not had twice.

**THE BACKLOG IS `logbook/projects/rig/BACKLOG.md`, and it did not exist until
2026-09-11.** §36 and the team charter both said *"only what survives goes in the
backlog"* and **neither ever said where that was** - proposals went into dated
one-off files and were then folded into this document or dropped, with no
standing list and no state per item. **A continuous feedback stream from every
seat cannot land in a dated file**, which is why the address is named here rather
than left to a convention.

**The failure mode this is written against:** a menu of features nobody adopts.
**Nothing enters the backlog without a named adopter**, and under self-hosting
the adopter is usually the seat that proposed it, which is the strongest form
that has ever been available here.

### Every seat is ARMED, not merely permitted, and this is his stated MAIN REASON for the self-hosting push

**Boris, 2026-09-12, extending the instruction above rather than replacing it:**

> *"I want that the general briefing of the project for Claude to make it arm all
> peers see if the difficulties or errors they experience due to lack of
> coordination or loss of sync or anything having relation to abilities rig can
> implement to greatly ease future work that they can make suggestions to the
> lead and the lead shall weigh them against our plan and backlog and anything
> it finds necessary to review and consider the suggestion so that rig is
> improved from its own development experience; That's basically the main reason
> I want to come as fast as possible to the point we are able to have the peers
> to use production instance of rig for rig development."*

**THE RATIONALE IS THE PART THAT WAS MISSING, and it reorders nothing by
itself.** The subsection above already carried the obligation. What it did not
carry is **why, in his words**. A future seat weighing *"is §37 worth what it
costs"* now has his answer rather than a lead's inference: **getting the peers
onto a production rig for rig's own development is his stated main reason for
the urgency, because that is what lets rig be improved out of its own
development experience.**

**ARMED, not permitted.** Noticing is part of a seat's job rather than a
courtesy it extends when it has spare time. The bar does not drop: an armed seat
reports more, not worse.

**THE THREE TRIGGERS HE NAMED**, broken out because they are the ones a seat is
most likely to absorb silently as "how it is":

| Trigger | What it looks like from inside a seat |
|---|---|
| **Lack of coordination** | two seats colliding, work redone, a file edited under someone, an ownership question nobody could answer, a decision taken twice |
| **Loss of sync** | acting on something that had already changed: a stale count, a rotted citation, a peer's status that was true when written, a gate result that had expired |
| **Anything rig could implement** | something done by hand, or derived twice, that a supervisor on this machine could have told you |

**The third is the widest and deliberately so.** It is not "a rig feature a seat
wants". It is **anything that cost a seat time and that a program coordinating
agents on one machine could have prevented.**

**THE LOOP, stated once so it is a mechanism rather than a sentiment:**

> a seat hits coordination pain -> it proposes to the **lead** -> the lead
> **weighs it against this plan AND the backlog**, and says what else it
> reviewed -> what survives lands, with its evidence and its adopter.

**"Against our plan and backlog and anything it finds necessary to review" is
his wording and it widens the lead's half.** The five attack questions in the
subsection above are how; this says what the proposal is weighed against, and
that the lead states what else it consulted.

#### WHAT A SEAT'S REPORT DOES, 2026-09-12: it CHOOSES the next capability

**Boris, 2026-09-12, extending the loop again and giving it teeth it did not
have:**

> *"Throughout the whole development of rig, the peers and the agents must be
> encouraged to raise their difficulties or things that took time/tokens/effort
> and so on that could be greatly improved had we had the right mechanism
> implemented in rig for that usecase; these are to affect the next features
> we'll choose to promote to be worked on and perfected for deployment; they can
> choose among the planned capabilities and if none is perfect for the need then
> they can suggest what mechanism would be perfect for it and we'll consider
> adding it to the plan."*

**THE NEW PART IS THAT A REPORT IS AN ORDERING INPUT, NOT AN INBOX ITEM.** The
subsections above obliged a seat to report and obliged the lead to attack. **This
says what the surviving report then DOES: it moves the promotion order** - which
capability is taken to the four-clause bar next and cut over to production.

**The two shapes a report may take, and a seat picks between them rather than
being handed one:**

| Shape | When | What the lead does with it |
|---|---|---|
| **"This planned capability would have solved it"** | the need maps onto something already in §16 or the milestone table | **weighs it into the promotion order.** No plan change, no new specification. The cheapest and the expected case |
| **"None of the planned ones fits, and here is the mechanism that would"** | the need has no home in the plan | **attacked as a specification proposal**, and if it survives it enters the plan. Rarer, and deliberately harder |

**A seat that reaches for the second shape without having checked the first is
answered with the grep.** Proposing what already exists is this project's named
failure mode and the admission bar already says so.

**AND THE LOOP RUNS DOWN THE LINEAGE TOO, not only up to the lead.**

**Boris, 2026-09-11, immediately after the instruction above, and RECORDED
NOWHERE UNTIL THE TRANSCRIPT SWEEP OF 2026-09-12:** *"You as well can and should
be making suggestions to your successors."*

**Said to a LEAD**, which is what makes it more than a restatement: the
obligation does not stop at the seat that collects proposals, and it does not
only travel sideways. **A seat hands its successor what it learned about rig's
own gaps**, in the handoff, the same way it hands over live state.

**Why it needs saying at all:** a seat's sharpest observations about a missing
mechanism arrive in its last hour, which is exactly when it is handing over and
has the least room to argue a proposal through the lead. **Those observations
die with the session unless the handoff carries them.** The route to the lead is
unchanged; this is the second copy, and a duplicate here is cheap where a loss is
not.

**THE PROMOTION ORDER IS THE LIVE ARTEFACT, and it has an address:**
`logbook/projects/rig/READINESS.txt` carries what is being waited for, what each
costs, and how much is left - see the next subsection. **A report that changes
the order changes that file in the same commit**, or the ordering input did not
land.

**THE BRIEFING HALF IS NOT HERE AND MUST NOT BE DUPLICATED.**
`~/.claude/skills/team-up/references/ROLES.md` carries it, with this same
verbatim and the three triggers, so every seat is armed at spawn. **Cited rather
than reproduced, so the two cannot drift** - the same treatment §36's V20 gets
against the basis-marker entry.

**WHAT THIS DOES NOT DO, and a seat must not over-read it:**

- **It does not renumber or reorder any milestone.** The rationale explains the
  urgency of the existing route; it does not change it.
- **It does not relax the backlog's admission bar.** Evidence rather than a
  wish, and a named adopter, both unchanged.
- **It does not let a seat file straight to the backlog.** To the lead, always.

### `READINESS.txt`, the one artefact that answers "how much longer"

**Boris, 2026-09-12, verbatim:**

> *"I want a dedicated simple txt file that the team-lead must always maintain -
> it must clearly depict concisely what are the features we are waiting until we
> start developing using rig and for each a rough estimate of the effort it takes
> to finish it and a summary of how much longer to the launch of my mentioned
> vision."*

**`logbook/projects/rig/READINESS.txt`. Plain text, deliberately.** It is the one
file he opens to answer *"how much longer"*, and it is the file a lead must not
let rot.

| | |
|---|---|
| **Owner** | **the team-lead seat, always.** Not a worker, not a successor's good intention |
| **Format** | plain text, no markup, short enough to read in one screen. **Not a fifth markdown document** |
| **Contains** | every capability still being waited on, its rough effort, its state, and one summary line of what is left |
| **Updated** | **whenever a capability changes state, in the same commit as the change.** A report that reorders the promotion queue updates it too |
| **Does NOT contain** | specification (that is here), ordering rationale (`BACKLOG.md`) or decisions (`DECISIONS.md`). **It carries numbers and state, nothing else** |

**Effort is in SEAT-DAYS, never in calendar dates.** This project carries no
dates and this file introduces none: a seat-day is one seat working one day, and
the summary line converts it at the observed rate with the rate stated. **A
calendar date would be the only unfalsifiable number in the file.**

**Every estimate carries its basis**, because an estimate with no basis cannot be
corrected when it is wrong - only replaced by another guess.

### The notification obligation, written as a mechanism because a promise cannot survive a session

**Boris asked to be notified specifically when that time comes.** No session
alive today will be alive then, so the obligation is placed on whichever seat
crosses the line rather than on a memory:

> **The seat that moves the FIRST capability onto production rig MUST notify
> Boris before anything is repointed, and MUST NOT proceed on its own
> judgement.** The best-judgement delegation does not reach this gate. The
> answer goes in `DECISIONS.md` in writing, per §24.

**AMENDED 2026-09-12 and the trigger changed.** It used to read *"the seat that
lands the LAST precondition"*. With the gate split, landing the last isolation
precondition is no longer the moment anything is repointed - **the first
capability cutover is.** The superseded trigger would have fired the notice while
production still carried nothing.

**EVERY SUBSEQUENT MOVE IS RECORDED, NOT GATED.** Only the first needs his
answer. Each later capability move goes to `DECISIONS.md` with its four-clause
evidence and is announced to live seats, and no seat waits for him. **A gate on
every move is a gate nobody takes**, and the staged migration would stall on the
second one.

**This is the same shape as the M3 and M8 gates and for the same reason:** what
stops a gate being passed rather than taken is the block on the next thing, not
a calendar. **This project carries no dates and this gate introduces none.**

---
