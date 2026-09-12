## 18. Supervision and failure

- **Register:** handshake, wire version check, declaration validated against its schema,
  capabilities granted, then the program appears on every surface. A failure at any step is a
  quarantine with the reason, not a retry loop.
- **Start:** rig builds the child's environment rather than inheriting its own. **No grant
  travels in it, because no grant exists as a variable** - `introspect` is decided from the
  connection (§14), so there is nothing to scrub and nothing a future reader can reintroduce
  by forgetting to. What the constructed environment *does* carry is an allowlist, written
  down because a constructed environment silently governs the estate and an empty one breaks
  four subsystems:

  | Variable | Without it |
  |---|---|
  | `DBUS_SESSION_BUS_ADDRESS` | `go-keyring` cannot reach the secret store (§22), and §12's D-Bus toast fallback is dead |
  | `DISPLAY`, `WAYLAND_DISPLAY`, `XAUTHORITY` | every `needs_display` command fails, including §5e's own worked example `snapper.capture-region` |
  | `SSH_AUTH_SOCK` | a program that shells out to git over ssh hangs on a passphrase nobody can answer |
  | `TZ`, `LANG`, `LC_ALL` | timestamps and collation drift from every surface that renders them, and `TZ` feeds the DST hole in §33 |
  | `HOME`, `PATH`, `XDG_RUNTIME_DIR`, `XDG_*_HOME` | nothing resolves - a program cannot find its own socket, config or cache |
  | `RIG_*` | the program's own registration handles: socket path, program id, wire version. Never a grant |

  Everything else is dropped, and the allowlist is config, resolved through §11 like anything
  else, so a program needing one more variable declares it rather than getting the parent's
  whole environment back. Conformance item 22 already reads the child's environment; it
  asserts the positive half in the same test.
- **Health:** on the interval, with a timeout. Three failures is degraded, five is a restart.
- **Restart:** exponential backoff inside a budget. Exceeding it is quarantine - a visible state
  with the full history and a manual restart, never a silent disappearance.
- **Crash:** the pane becomes a panel with the exit status, the last 200 log lines, the restart
  decision and the countdown. Nothing else changes.
- **Hang:** every call has a deadline. A program that misses them is degraded, then restarted. A
  hung program can never block a rig goroutine, and a lint rule keeps it that way.
- **Flood:** per-program rate limits on events, logs and notifications, with the drop count shown.
- **rig dies:** every program keeps running, tolerating the absence (§5g). On restart all
  reconnect, present their session token, and are told explicitly what was lost. In-flight
  commands are marked interrupted with partial output kept, and are **never silently replayed** -
  a retry is the client's decision, made against the `idempotent` property and deduplicated by
  the request id on the wire (§5f).
- **Shutdown:** the lifecycle notice first (§5g), then SIGTERM, grace period, then SIGKILL.
  Programs rig did not start are never killed. A `DRAINING` notice moves every held lease to
  ORPHANED rather than FREE, so a restart cannot hand one resource to two holders (§16).
- **Upgrade:** stop, swap the binary, start. **No handover.** Measured against a built handover
  path with fifteen connections and real fd inheritance: worst client gap 4.4-5.0 ms plain
  against 2.5-3.4 ms handed over, and the handover's 30-45 refused dials are removed by the
  notice alone. The entire marginal benefit is **1.9 ms per upgrade**, or 5.2 seconds a year at
  weekly upgrades, against four defect classes - a replay path less safe than `kill -9`, a
  fencing counter that breaks in the 0.86 ms serialise window, an unversioned second wire
  format, and value that is highest exactly when the state struct changes daily.
- **A hosted program panics** (§5j): the invoker recovers it, quarantines that program with the
  stack and the reason, and keeps serving. Twice inside the restart budget and it stays
  quarantined.
- **A missed schedule fire.** The laptop suspends nightly, so this is the normal case rather
  than the exception, and cron's answer (drop it) and anacron's (fire them all) are both wrong
  here. The policy is decided by the command's own `idempotent` property: **idempotent** - all
  missed fires coalesce into exactly one run at resume; **not idempotent** - nothing fires,
  and the misses are reported in the schedule view with their times and a one-click run. Either
  way the coverage log records the gap (§15), and expiry is frozen for one TTL after resume
  (§16).


### The supervisor as a machine, not a description

**Everything above this line is prose, and that was the finding.** §34 named
§18 as the calibration point at the wrong end: five state names that appear only
in §5's candidate section, no state set, no transition table, no owner, no
statement of what is illegal. **The consequence is not cosmetic** - §5's
"declared state machines, if the supervisor can be its first client" cannot be
tested while the supervisor is sentences, because there is nothing to express.

**The states, and this is the set. There are no others.**

| State | Means |
|---|---|
| `STARTING` | rig has launched the child and it has not completed the handshake |
| `HEALTHY` | registered, and showing evidence of progress (below) |
| `DEGRADED` | reachable, and failing its health definition |
| `RESTARTING` | inside the restart budget, backing off |
| `QUARANTINED` | out of budget, or failed registration. **Visible, with history, and it stays until a human acts** |

**The transitions, with the trigger and who owns causing it.**

| From | To | Trigger | Caused by |
|---|---|---|---|
| - | `STARTING` | launch | rig, or a program connecting unbidden |
| `STARTING` | `HEALTHY` | handshake and declaration validated | the program |
| `STARTING` | `QUARANTINED` | any registration step fails | rig. **Not a retry loop** |
| `HEALTHY` | `DEGRADED` | three health failures, or a missed deadline | rig's health check |
| `DEGRADED` | `HEALTHY` | health recovers | rig's health check |
| `DEGRADED` | `RESTARTING` | five failures | rig's restart budget |
| `RESTARTING` | `STARTING` | backoff elapses, budget remains | rig's restart budget |
| `RESTARTING` | `QUARANTINED` | budget exhausted | rig's restart budget |
| any | `QUARANTINED` | a hosted program panics twice in budget (§5j) | the invoker |
| `QUARANTINED` | `STARTING` | **a human, and only a human** | manual restart |

**The illegal transitions, and what happens on one.** Everything not in the
table above is illegal. **An attempted illegal transition is not ignored and not
best-guessed: it is recorded as a supervisor fault and the program is
quarantined**, because a supervisor that cannot account for its own state is the
one component whose confusion is not survivable. This is the row the prose could
never have: *inferable* transitions are exactly the ones nobody checks are total.

**Four actors can move a program and they are named here** because "three
failures is degraded" hid that: the health check, the restart budget, the
invoker's panic recovery, and a human. **A `DRAINING` notice moves leases, not
program states** (§16) - it is not a fifth actor, and reading it as one is the
mistake this row prevents.

### Health is evidence of progress, not evidence of responsiveness

**"Responding" is the wrong health definition for the actor this estate
actually supervises.** A service that answers is working. **An agent session
that answers may be idle, stuck, looping, or parked on a question nobody can
see** - every agent-specific failure is responsive, so a liveness check passes
through all of them.

| | |
|---|---|
| **Health is evidence of PROGRESS** | a renewal carries a progress marker. **A renewal emitted by a background thread proves the process runs and nothing else**, which is the check that cannot fail and therefore cannot help |
| **Idle-and-not-blocked is UNHEALTHY, with a threshold** | not a rest state. A session doing nothing and waiting for nothing is the characteristic failure, and it looks identical to diligence from outside |
| **A waiting session declares WHAT it waits for** | so waiting is distinguishable from stuck. Without it the two are the same observation, and one of them is fine |
| **Parked on a question nobody has seen is its own state** | it is neither progress nor a fault, and rendering it as either loses the only action that helps, which is showing the question to a human |

**This is the same defect as the roster's `partial`, and as four measurement
bugs this repository has already shipped:** a check that cannot distinguish
"nothing is wrong" from "the check did not run". **Being told a program is
responsive answers a question nobody asked.**

### What survives what: the state ownership matrix

**§18 promises programs "are told explicitly what was lost". That is the
promise; this is the mechanism.** Scattered per-subsystem prose means no reader
sees the whole thing and programs guess.

**Six events reduce to three, and the reduction is a finding rather than a
convenience.** An *upgrade* is stop-swap-start with no handover, and neither
version number is ever compared (§21), so **an upgrade is exactly a daemon
restart**. A *reboot* is a daemon restart in which every program restarts too. A
*program restart* is holder death for everything that program put in. So the
columns are: **daemon restart**, **holder death**, **seat succession**.

| State class | Daemon restart | Holder death | Seat succession |
|---|---|---|---|
| Declaration | **lost; the PROGRAM rebuilds it** by re-registering on reconnect | lost with the connection | n/a - programs do not occupy seats |
| Lease | **survives**, with expiry frozen one TTL past resume (§16) | **ORPHANED, not FREE**, until the witness is observed dead | held leases transfer to the successor generation |
| Coordination claim | **survives** | **ABANDONED, inheritable by one CAS write** | transfers |
| Seat | **survives** | **ORPHANED**, and the seat outlives every occupant | the point of the mechanism |
| Blackboard key, durable class | **survives** | survives; ownership goes abandoned if claimed | n/a |
| Blackboard key, session class | lost | lost | lost |
| Subscription | **lost; the CLIENT rebuilds it** from its cursor, and `gap: true` if retention has moved past it | lost | the successor resubscribes from the seat's cursor |
| Queued message | **survives**, queued against the seat | **survives** - this is what queueing against a seat rather than a session buys | **delivered to the successor** |
| Pending confirmation | **DENIED, never carried across** | denied | denied. **An approval outliving the invocation that asked for it is the hole §13a closes** |
| Detached operation | survives; output buffered | survives - it is rig's child, not the caller's | reattachable by the successor |
| Schedule entry | **survives**; missed fires resolve by `idempotent` (above) | survives | n/a |
| Config | **survives**; re-resolved through §6's layers | n/a | n/a |
| Single-instance claim | released and retaken | **released by design** - the flock lives on the descriptor | n/a |

**Every cell has an answer, "nothing survives" included, and the side that
rebuilds what is not preserved is named where anything does.** That naming is
the half that makes the table usable: a program that knows a declaration is lost
and that rebuilding it is *its* job needs no negotiation on reconnect.

**What is true TODAY is a different document and deliberately so:**
`logbook/projects/rig/state-ownership-today-2026-09-11.md`, measured by reading
every piece of state `internal/` holds. **The headline: nothing rig holds
survives anything, and nine of these thirteen classes do not exist yet.** The
two tables are meant to diverge - one is the target, the other is the distance
to it - and **merging "does not exist" with "exists and is lost" is the thing
that would make both useless.**

---
