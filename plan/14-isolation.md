## 14. Isolation

**Default deny, at three boundaries.** A user of rig is not aware that other users exist.

| Boundary | Between | How |
|---|---|---|
| **Instance** | Unix users, **and the two named estates of one user (§37)** | One daemon per estate, keyed by runtime directory. One socket at `$XDG_RUNTIME_DIR/rig/rigd.sock`, mode 0600, separate config, state and storage trees. Nothing is shared, including the tray, the window and the notification centre |
| **Session** | Clients of one daemon: agents, terminals, the window, scripts | Every connection carries a principal - uid, client kind, client id, session id - and belongs to one or more **scopes**. Every kernel view is filtered by scope set, **unless the principal holds `introspect`** (below - and it is not presented, it is decided
when the connection is made). Enforced against accident and against a merely buggy client; see the floor stated below for what it is not enforced against |
| **Program** | Programs | Capabilities (§13): secrets namespaced, storage pathed, events by grant, no cross-program read of anything |

**A client sees itself and the programs it may reach. Nothing else.** It cannot enumerate other
clients, cannot see their commands, cannot receive their events and does not appear in their
views. Two agents working in the same repository, through the same daemon, are invisible to each
other unless both opt in.

**That is the ordinary API, and it has exactly one door out of it: `introspect` (below).** The
default is deny because a program should not read another program's events by accident; it is
not deny because Boris should be kept out of his own machine. An agent he runs holds
`introspect` and sees all of it. The isolation this section builds is between *programs and
unprivileged clients*, and the proof below is stated over that population, not over a
principal that was handed the read grant on purpose.

### The kernel exposes no unscoped accessor at all

The previous version of this section listed six views that get filtered, and claimed in the same
paragraph that a new surface cannot leak. An allowlist of six is exactly how a seventh leaks.

**Every registry read is a method on a principal, the principal is in the type, and an unscoped
read is unrepresentable rather than discouraged.** A house analyzer - `no registry handle
outside the kernel` - sits beside the existing `no program id in rig code`.

**The corollary the plan was missing: any surface whose product is an estate-wide aggregate is
by definition a granted surface**, and needs one of the two grants below. That is the
capability map, `config export`, `doctor`, the palette, the tray, the notification centre and
the schedule - seven things that sat outside the old six-view list, one of which §9 tells an
agent to read first. **Which grant is the whole question**, and the earlier version of this
section answered it with the only credential it had, making every one of those seven closed to
an agent. Reading them takes `introspect`; changing anything through them takes `operate`.

### The operator is not a uid - and not a credential either

§15 used to grant the clients view to "the uid that runs the daemon", and the Instance
boundary above puts one daemon per uid. A unix socket carries no credential but `SO_PEERCRED`, so **every client that could
connect already satisfied that predicate** - and `rig history --client=X` would then hand any
agent another program's secrets (§15). It is the highest-ranked finding of the 2026-09-10
attack and it was invisible to every seat individually.

The fix went through two wrong drafts before the right one, and both were wrong the same way,
so the reasoning is worth keeping:

| Draft | Mechanism | How it broke |
|---|---|---|
| 1 | A token minted at startup into a 0600 file | Every client runs as this uid, so every client could read it. Any program could grant itself the estate |
| 2 | A token in `RIG_INTROSPECT`, exported by Boris's shell | An environment is inherited by a shell's **whole descendant tree** - `make`, an npm postinstall, every in-house program he starts by hand. And it is minted per daemon start, so after the first `rigd` restart every long-lived shell holds a stale one and is silently downgraded, with no way to rewrite it from outside |

**Both drafts tried to push a secret to the right set of processes.** The set was never the
problem. rig already knows which connections are programs, because it registered them.

### `introspect` is decided at connect time, not distributed

**What it grants**, stated before how it is decided: **every read**. The estate-wide views, the
full capability map, `config export`, `doctor`, the schedule, the notification centre, any
client's history, any trace. It grants **no action at all** - that is `operate`, below.

- `introspect` is **not a token, a file or an environment variable**. There is nothing to
  deliver, nothing to store and nothing to steal. It is decided when a connection is made:
    * a connection that completes the **program registration** handshake (§5) is a program,
      and is scoped for the life of that connection;
    * every other connection on the local socket from this uid is a client of Boris's, and
      reads everything.
- **HTTP is excluded.** A connection with no unix peer mints its principal from a bearer
  token per client kind - `MintPrincipal(evidence)`, below - and gets no scopes at all,
  `introspect` included.

Nothing is inherited, so a build script descended from Boris's shell gets no more and no less
than the shell did - and a program gets nothing, however it was started. Nothing is minted per
daemon start, so a restart cannot silently downgrade a window that has been open for a week.
Nothing is persisted, so there is no file to find.

**The one limit, stated because a boundary nobody writes down is a boundary nobody tests:** a
program that opens a *second* connection and declines to register on it is a client of Boris's
and reads everything. That is the same floor the file and the environment had, but reaching it
now takes deliberate action rather than inheritance - which is the standard §13 sets for
capability enforcement, labelled the same way. **No program acquires the estate by accident, or
by being merely buggy.** Genuinely enforced against a hostile local process: only the uid
boundary and redaction.

This is also the only version of the mechanism a **hosted** program (§5j) can be held to. A
hosted plugin is compiled into `rigd` and shares its environment, so no environment scrub could
ever have covered it; its *connection* is registered like any other program's, so the predicate
covers it with no special case. Conformance item 23 asserts it, and item 24 holds the hosted
class to it (§19).

### Every new principal gets walked against this table

The 2026-09-10 sweep found the same defect four times - the grant reaching a caller kind nobody
was thinking about - and diagnosed the cause correctly: **reasoning about a grant from the
caller it was written for, and not from the others.** The artefact that prevents it is a list of
the callers. Anything that adds a principal, a grant or a scope is walked against all of it, and
the walk is part of the change, not a review comment:

| Caller | Has a connection | What it gets |
|---|---|---|
| An agent Boris runs | yes, unregistered | everything. This is the motivating caller, and the one that misleads |
| A terminal, the window, the tray | yes, unregistered | everything |
| A script, a `make` target, anything descended from his shell | yes, unregistered | everything, and it inherited nothing to get there |
| A registered program, spawned or hosted | yes, registered | its own scope only |
| An HTTP client | no unix peer | its bearer principal's scopes, never `introspect` |
| A scheduled fire, a house rule, an internal timer | **none** | rig's own principal. It has no connection to decide from, so it is named explicitly or it is refused (§13a) |

### `operate` is an authorisation decision per call, not a grant at all

The first draft handed `operate` to "the process that launched `rigd`" - which under M15's
autostart is systemd, so `rig stop shelf` from a terminal, the tray's Start entry and the crash
panel's manual restart were all unreachable. The second draft kept a file for "non-interactive
callers - systemd units, `make deploy`", and recreated the same defect one row lower: `make
deploy` typed in a terminal descends from Boris's shell, not from the daemon's launcher, so the
row promised a file to a caller that provably cannot read it.

**So `operate` stops being a credential.** Estate-wide action - stopping a program another
client owns, revoking a grant, breaking a lease, editing config outside the caller's scope - is
an authorisation decision taken per call, which is exactly what §13a already is:

| Caller | How the call is authorised |
|---|---|
| Any interactive surface - terminal, tray, window, an agent | `confirm`, which routes to window, toast or terminal (§12, §13a). The answer authorises **that one call**, not the connection |
| Anything unattended - a systemd unit rig installs, `make deploy`, a scheduled fire | A **house rules** entry naming the caller and the command. Declared, auditable, reviewable in one file, and no secret exists to leak |

That deletes the second file, the second boolean and the distribution problem in one move.
**One boolean remains on the principal struct** - `scoped`, set by registration - and there is
no other way to become either kind of caller.

There is no new elevation vocabulary: `confirm` already routes to window, toast or terminal.
Authorisation is per call, so nothing holds standing estate-wide power for hours. And the audit
log already records every `confirm` decision, so an estate-wide action is audited for free -
with `origin: elevation` distinguishing it from `origin: rule` (§13a).

**The question must name what it is authorising, and reach the caller that asked.** A toast
that says only "stop shelf?" is answered by whoever is looking at the screen, in the belief it
came from the terminal in front of them - and §12's toast deliberately does not steal focus.
So the prompt carries, always: the **principal** and client kind, the **pid**, the
**command**, and the **arguments** as they will be invoked. And it routes to the requesting
caller's own surface first - an interactive caller is asked where it is, and only an
unattended caller's question goes to whoever is present. The audit log records the decision
afterwards (§13a); this is what makes the decision itself attributable at the moment it is
taken.

**An unanswered elevation is denied, and the denial is recorded.** Estate-wide action fails
closed: an agent at 3am that asks to stop a program another client owns, with nobody present
and no house rule naming it, is refused rather than queued. Do Not Disturb never suppresses an
`ask` that gates a command (§12) - it suppresses notices, and this is not one.

**A lease-holder can still be asked.** §16 says a lock is never held across a question to a
human, and rig refuses the combination - which would refuse the motivating case, `rig peers run
--lease=deploy -- make deploy` needing to stop a program. The exception, and it reuses existing
machinery: **an elevation or `confirm` that gates the lease-holder's own call freezes expiry on
every lease that caller holds for the duration of the question**, the same freeze §16 already
applies after a suspend. The rule §16 states is about a lock blocking *someone else* on an
answer, and here nobody else is blocked.

**Why complete reading is safe.** The 2026-09-10 attack's highest-ranked finding was that
`SO_PEERCRED` made every client an operator, and that `rig history --client=X` would then hand
any agent **another program's secrets**. That was a finding about *what the history contains*,
and §15 fixed it at the source: anything `secrets` returns is never recorded, and every field
declared `sensitive` is blanked before the write. **Redaction is the precondition for complete
introspection.** With it, the worst an introspecting agent can read is everything Boris could
read himself - the requirement, not the leak. **That holds for the paths that cross the
history's write, which is where redaction happens**, so §15 states the invariant every other
estate-wide read path has to satisfy; a path that satisfies neither half of it leaks while
passing run 3. Without it, no mechanism would have been safe
enough - so M1 ships the connect-time predicate, and complete history reading is gated on M5's
redaction spans (§23).
### Scopes, so peers is not a hole in the filter

`announce` must return the crew, so under the old design the kernel needed a peers special case
at the very enforcement point that was supposed to make leaks impossible - and §3's two-client
test would have had to fail the moment peers compiled in.

Instead the kernel offers one general mechanism: `MintPrincipal(evidence)` and
`JoinScope(principal, scope, grant)`. Peers becomes an ordinary service that creates a crew
scope and joins consenting principals to it. The visibility it needs is **granted data, not a
code branch**, which is what §14 always intended by "invisible to each other unless both opt in".

`MintPrincipal` also answers a question no section previously did: **a surface with no unix peer
has to mint a principal from something.** HTTP hits this at M2. Evidence is a bearer token
issued per client kind, and an unauthenticated HTTP request gets a principal with no scopes
rather than the daemon's own.

### Written down, and therefore testable: what a client may infer

A boundary nobody wrote down is a boundary nobody tests.

- Acquiring a contended lease reveals that *a* peer exists. Accepted, and stated.
- `try_lock` returning BUSY reveals the same. Accepted.
- Deadlock refusal names **the leases the caller holds or requested**, never the peers. The
  named cycle goes to the clients view only, which needs `introspect` (§14) - so an agent
  debugging a stuck `make deploy` can see the cycle, and the blocked peer still cannot.
- Counters are scoped: fencing tokens monotonic **per lease**, blackboard revisions **per
  namespace**. Never one global sequence, which leaks the rate of other clients' activity.
- A crew is created by a principal and joined only through a handle it hands out. Joining an
  unknown crew name returns the same error as joining one that does not exist.
- A colliding program id from a principal that cannot see the incumbent returns exactly the
  error it would get for an id it is not permitted to use - indistinguishable from "not
  allowed", never "already exists". An error that distinguishes those two is an oracle.

### The proof: observational equivalence over two worlds

§3's old test was a *content* predicate - run every surface against a two-client fixture and
fail if either sees the other - and every channel above except one is a **differential**
channel, invisible to it. Worse: with the operator bug, the fixture's expected result for
`rig clients` was "B is visible", so the test asserted the breach.

The property is stated as observational equivalence instead. Run the whole battery against
**W1 = {A}** and **W2 = {A, B}**, with B exercising every primitive, and require A's complete
transcript - responses, error codes, ids and tokens issued, ordering - to be identical. A
difference is a failure unless it appears on the enumerated, reviewed list above. The battery
covers non-surface observation too: the runtime directory, the config tree, the state tree, the
process table.

This is the same machinery §16 already buys for deterministic simulation at M7. It just has to
be pointed at isolation as well as at linearizability.

**A is a client with no grants. That is the whole population the property is stated over**,
and saying so is not a weakening - a property whose population is unstated is a property that
gets quietly falsified the first time a legitimate reader is added. So the battery runs three
ways, and the third is the one that catches the bug this design could actually have:

| Run | A holds | Required outcome |
|---|---|---|
| 1 | nothing | A's transcript is identical in W1 and W2. Isolation holds |
| 2 | `introspect` | A sees B completely, in every view, **with no gap that §15's coverage log does not declare**, and no silent filtering. A missing row whose absence the coverage log explains is a pass; a missing row it does not explain is a failure here, the same way an extra row is a failure in run 1. The assertion is on the coverage entry, not on the row - §15 loses records through four declared paths, so a completeness test with no such clause is false on the first busy day |
| 3 | `introspect` | Every value B declared `sensitive`, and everything B's `secrets` calls returned, is absent from A's transcript - at any sampling rate, in any encoding |

Run 2 is the requirement in §2 made testable, and it fails loudly rather than
degrading: **a view that quietly returns the scoped answer to a principal holding
`introspect` is a defect of the same rank as a leak.** Run 3 is why run 2 is safe, and it is
§3's existing no-secret-reaches-the-history test pointed at the reader instead of the log.
