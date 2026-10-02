## 53. Agent coordination taken from AgentBox

⛔ **RULED 2026-10-02, decisions 0258 and 0259.** Boris: *"Start as
recommended and take all non-visual capabilities, the ones that help agents
do a better job. Take the code as is if it saves tokens but make the logic
better if possible."* And: *"All functionality that is superior in `rig`
over AgentBox should be removed at least on the API level from AgentBox and
agents should be instructed to use it from `rig` instead."*

> **In one line: AgentBox's signals, blocking locks, shared table, rider,
> wrapped run and retraction, rebuilt on §16 and §52, each one done only
> when AgentBox's tool for it is gone and its manual names rig's.**

---

**Boris, 2026-10-02, on durable signals before #2:** *"Proceed as
recommended; Make sure `rig` functionality is superior."* So every row
below must beat AgentBox at what agents use it for, not merely match
it, before AgentBox's tool is removed.

### The six, in his order

| # | Capability | rig's form | Better than AgentBox by |
|---|---|---|---|
| 1+5 | signals; rig posts lock and peer changes | `signal.*`, `lease.changed`, `roster.changed` on §52's bus | the sender is the daemon's seat, never a request key; `to_seat` is that seat's alone |
| 2 | blocking acquire, FIFO | `lease.acquire` with `wait_ms`, a queue per lease | refuses a wait that would deadlock (A waits on B's lease while B waits on A's) |
| 3 | witnessed claims on a shared table | the store's `expected_version` CAS plus a witness | an entry dies with its owner, as a lease does |
| 4 | the `sync:` rider | what changed since your last call, on any answer | no extra call to learn the world moved |
| 6 | the wrapped run | `rig peers run --lease=NAME -- cmd` (§16) | expiry kills the writer, the only real fence rig has |
| 7 | retraction | `toast.retract` of a toast you sent | recorded, so a retraction is seen as one; a waiter on its reply is told |

**Not taken**: form, secret, the countdown card, the progress bar
(cards on his screen), walkthroughs, assignments, artifacts,
`request_review`, `show_document`.

### Done means, per capability

1. Built, tested with a red control, demonstrated live.
2. AgentBox's tool removed from its MCP surface, in AgentBox's repo
   (ADR-0014), in the same change that ships rig's.
3. AgentBox's `docs/agent-manual.md` names the rig tool in its place.

**Removal waits on parity.** A tool leaves AgentBox only when rig is at
least as good at what agents rely on it for. Where rig is not yet, the
row says what is missing.

---

### As built: slice 1, #1+#5, 2026-10-02 (`54f8b71`)

| Piece | As built |
|---|---|
| publish | a seat, a terminal included, posts `signal.<words>`; source `seat:<name>`; a program is refused |
| addressed | `to_seat` is seen by that seat's waits alone; tested with a third seat and an unseated caller |
| lease | `lease.changed {name, change, holder, token, by}` on acquired, released, broken |
| roster | `roster.changed {seat, change, purpose}` on announced and left, from every door |

**Demonstrated live on production, 2026-10-02:** a terminal followed
`signal.* roster.* lease.*` while seat `lead` announced, signalled,
signalled `to_seat: lead`, and took and released a lease over MCP. The
terminal saw every one but the addressed signal; `lead` saw both.

**What it does not do yet:**
- **An expiry posts nothing.** coord finds an expired lease when it is
  next touched. Slice 2's queue needs the wake, so it adds the post.

### As built: slice 1b, durable signals, 2026-10-02

`d275489`, `44b663b`, `4977746`, `e6e256e`; AgentBox `46ac47f`.

| Against AgentBox | AgentBox | rig |
|---|---|---|
| kept past a restart | 1000 per topic, 7 days | the same, in coord.db (schema v2) |
| a trim is told | per-topic recorded watermark | the same, per kind |
| sender | a key the request carries | the seat rigd names |
| addressed | `to:<key>`, any waiter on it sees it | `to_seat`, that seat alone |
| delivered | parked waits woken | the same |
| one answer | capped by count | count and 768 KiB, resumes where it stopped |
| at the prompt | `agentbox sync post` | `rig events publish` |

- **Cursors order across restarts:** seq is `epoch<<32` onward, so an old
  cursor reads stored signals with `gap: false`; naming a non-durable kind
  beside them still answers `gap: true`, since the ring is gone.
- **A first wait (`after 0`) gets this run's signals only**, never a
  backlog from earlier runs.
- **Demonstrated live on production:** a signal published at epoch 95
  came back after `systemctl --user restart rigd.service` (epoch 96) with
  `gap: false`; `delivered: 1` with a terminal wait parked; the CLI
  published as `seat:terminal:boris-milner` and reached its waiter.
- **AgentBox side done:** `post_signal` is off its MCP surface (38 tools
  listed live); its manual names `events_publish`/`events_wait`.
  `await_signal` stays for AgentBox's own `lock:`, `agents:` and
  `shared:` topics until #2 and #3 land.

### Slice 2, #2: a blocking lease with a queue (designed 2026-10-02)

What agents rely on in AgentBox's `acquire_lock`, and what rig's must add
to retire it. Boris, 2026-10-02: *"start #2 only and proceed as much as
possible"*.

| AgentBox `acquire_lock` | rig `lease.acquire` |
|---|---|
| blocks in a FIFO queue | `wait_ms`, a FIFO queue per lease; a free lease goes to the queue's head, never to a newcomer |
| refuses a deadlock by name | the same, walked over seats and the leases they wait on |
| a timeout is a result with the holder's purpose, activity, held-for, queue | the same, plus the token, the witness and its liveness |
| a grant says why | `granted_because`: free, released, broken, expired |
| holds are memory-only, dropped on restart | holds stay durable and fenced; the queue is memory-only, a waiter's connection is its place |
| `lock:<name>` on a hand-over | `lease.changed` also on queued, orphaned and expired, which needs a clock: rig looks at each lease at its deadline, then polls an orphan's witness each second |

- **A waiter that goes away leaves the queue**, so the head is never a
  ghost. A grant that races a timeout is kept, never dropped.
- **`wait_ms` 0 keeps today's refusal**; the most a wait parks is 25
  minutes, AgentBox's bound, for the same MCP-client reason.

### As built: slice 2, 2026-10-02

`f8c135f`, `c5963bf` (size), `ec7e843`; AgentBox `a1fe921`.

- **Built as designed above.** A holder's purpose and activity come from
  its seat's roster row; `held_ms` counts from when the current holder
  took it, kept across a same-seat re-acquire and reset by a reboot.
- **A refusal without a wait carries the same picture** in its `Actual`,
  so `wait_ms` 0 callers decide without a second call.
- **`released` names the holder that released**, so a waiter on
  `lease.changed` learns whose turn ended.
- **Proved by tests that fail when the property is removed:** LIFO order,
  no deadlock walk, no departure on a closed connection, no watcher, and
  no incumbent picture each broke its test. Race detector clean.
- **Demonstrated live on production, three `rig mcp` sessions:** a wait
  timed out with holder, purpose, activity, held-for and queue; a release
  handed the lease to the queue's head as `released`; a deadlock was
  refused naming *"demo-a would wait on demo:deploy, held by demo-b;
  demo-b waits on demo:repo, held by demo-a"*. A holder's `rig mcp`
  SIGKILLed with a 3 s lease: the queued seat was granted it `expired`
  at 3.0 s, and `lease.changed` said acquired, queued, expired, acquired.
- **AgentBox side done:** `acquire_lock`, `try_lock` and `release_lock`
  are off its MCP surface (35 tools listed live); its manual's "Taking
  turns" names rig's `lease_*`. `agentbox sync lock` stays for shells.
- **The logbook protocol moved with it:** COORDINATION rule 5 and the
  lock names are rig leases now, under the same names.

Known gaps, none blocking:

- **AgentBox's Agents board no longer shows agents' holds**; rig's
  `lease_list` does. A shell's `agentbox sync lock` and an agent's rig
  lease of the same name do not exclude each other.
- **Terminal seats share `terminal:<user>`**, so two terminals do not
  queue against each other; one is "already yours" to the other.
- **rig leases are per estate**; AgentBox's locks were machine-wide.
- **A session started before the deploy keeps the old tool schema**:
  no `wait_ms` until its `rig mcp` restarts, and the retired AgentBox
  tools still listed until its AgentBox MCP restarts.
- **No `rig lease` CLI yet**; it is #6's wrapped run.
- **Untested:** a newcomer arriving in the instant a lease expires.
  `serveLeaseAcquire` looks first so the queue's head wins; no test
  pins it.

### Slice 3, #3: witnessed claims on a shared table (designed 2026-10-02)

Boris, 2026-10-02: *"start #3 and proceed as much as possible"*. What
agents rely on in AgentBox's `shared`, and where rig's must beat it:

| What agents use it for | AgentBox `shared` | rig `shared.*` |
|---|---|---|
| claim an item, first writer wins | `if_version` 0 | `expected_version` 0, the store's own rule |
| update what you read | `if_version` N, losing is `stale` with the current value | the same |
| a version never comes back | **no**: a deleted key restarts at 1, so a stale writer can hit a new claim (ABA) | **yes**: versions come from one estate counter and are never reused |
| an owner that died | roster, then a bare pid, which a recycled pid fakes | roster, then §16's witness, pid plus start time plus boot |
| told it was abandoned | only when somebody reads | also posted: `shared.<key>` with `owner_gone` when the owner leaves and its process is dead |
| who wrote it | owned writes only | every write names its seat in `by` |
| delete | unconditional allowed, so one can erase a peer's takeover | at the version you read, always |
| wait for a family | `shared:claims/*` | `shared.claims.*`, the same `.*` as every rig kind |

- **A key is dotted lowercase words** (`claims.chunk-3`), at most 120
  bytes, because the key IS its event kind's tail. A get on `claims.*`
  reads the family, capped at 200 per answer with `more`.
- **Values are JSON, at most 16 KiB; at most 1000 keys**, refused loudly
  at the cap rather than evicting a claim. AgentBox's numbers.
- **A lost race is a result, not an error**: `applied` false, `stale`
  true, the current value and a note saying what to do next.
- **Writes need a seat; reads do not.** The owner is the seat, and the
  witness is the caller's process from the socket, never the request.
- **A seat taken again is the same owner**, as with a lease: a session
  that reconnects into its seat keeps its claims.
- **For fan-out work rig already has `queue.*`**, whose claims are leases;
  `shared` is for state a queue does not model.
- **Kept in coord.db** beside the leases, schema v3, so a claim survives
  a restart.

### As built: slice 3, 2026-10-02

`1e5c1c2`, `e00d857` (size), `45b36b7`, `7faacb2` (size); AgentBox
`f764449`.

- **Built as designed above.** Versions come from one counter in
  coord.db (`shared_seq`, schema v3), so a key made again never repeats
  a version. The witness is the caller's pid from `SO_PEERCRED`; a
  terminal seat writes unwitnessed and never reads as gone.
- **`owner_gone` needs both halves:** the seat off the roster AND its
  witness dead. It is posted as `shared.<key>` when the roster says
  `left`, polling 5 s for the process to end.
- **On the agent door a value is JSON, not a string:** the MCP bridge
  presents `value_json` as `value` both ways. `google.protobuf.Value`
  was the alternative and cost the rig CLI +176 KiB; this cost +4 KiB.
- **Proved by tests that fail when the property is removed:** per-key
  versions, no roster check first, no owner watch, no witness, and no
  JSON presentation each broke its test. Race detector clean.
- **Demonstrated live on production, three `rig mcp` sessions:** the
  race loser was told the key is still held; a SIGKILLed owner read
  `ownerGone` and `owner_gone` was posted within 1 s; the takeover
  landed at its version; a stale delete was refused; a key made again
  got v15, not the reused v12; values round-tripped as JSON objects.
- **AgentBox side deployed:** `shared` is off its MCP surface (34
  tools listed live), its manual and wiki point agents at rig's `shared_*`,
  and `tools/sync-probe.py` lost the shared scenario. Its full test
  suite passes. `agentbox sync set` stays for shells.

Known gaps, none blocking:

- **AgentBox's board no longer shows agents' claims**; rig's
  `shared_get` on a family does.
- **A shell's `agentbox sync set` and an agent's `shared_set` are
  separate tables.**
- **Keys are dotted lower-case words**, so AgentBox's `claims/x` keys
  do not carry over as written.
- **A seat taken again reads as the same live owner**, as with leases.
- **A session started before the install lacks the `shared_*` tools**
  until its `rig mcp` restarts.

### Slice 4, #4: the `sync:` rider (designed 2026-10-02)

AgentBox appends one `sync:` line to any MCP answer, saying which
agents joined or left the caller's area since its last call, and which
of its locks the human broke. rig's rider rides every MCP answer the
same way, and says more:

| Kind | Told to seat S when |
|---|---|
| `roster.changed` | another seat announced or left, with its purpose |
| `lease.changed` | a lease S holds was broken or expired, or a seat queued on it |
| `shared.<key>` | a key S owns was set by another seat, deleted, or read as gone |
| `signal.*` | a signal was addressed to S alone (`to_seat`) |

- **Better than AgentBox by:** a lease lost, a claim taken over and a
  peer waiting on you reach the agent mid-task, not only company.
- **It names its cursor.** The line ends with the bus seq it read
  through, so `events_wait` from there gives every payload in full.
  A ring gap is said as one: re-read the state.
- **Bounded:** at most 8 items, then "and N more".
- **Never S's own doing**, and never on `announce`, `list_agents` or
  `events_wait`, whose answers already carry it.
- **The cursor starts when S takes its seat** and lives with the
  connection. A failed send drops the connection and the seat with it,
  so nothing is owed afterwards, which is why AgentBox's put-back has
  no counterpart here.
- **Scope is the estate**, rig's only scope (`list_agents`), where
  AgentBox narrows to an area.
- **AgentBox's side:** its rider stops riding MCP answers; its manual
  points to rig's.

### As built: slice 4, 2026-10-02

`5dda778`, `7403703` (size, rigd +28 KiB); AgentBox `2930426`.

- **Built as designed, with two changes.** `list_agents` carries the
  line, since its answer shows the roster but not leases or claims.
  `events_wait` neither carries it nor moves the cursor, because it
  answers only the kinds it was asked for.
- **One middleware on `tools/call`** in `internal/mcpserver`. The
  daemon's `rider.go` decides what the line says. A `shared.<key>`
  set now names `was`, the owner it replaced, when rig knows it
  exactly.
- **Proved by tests that fail when the property is removed:** no
  cursor move, `events_wait` spending news, broadcast signals riding,
  no previous owner, and other seats' leases riding each broke a test.
  Race detector clean.
- **Demonstrated live on production, two `rig mcp` sessions:** A was
  told *"demo-b arrived"* mid-task. One later call carried *"demo-b is
  waiting on your lease demo:build; your claim demo.chunk-1 was taken
  by demo-b; signal.demo.review for you from demo-b"*. The broadcast
  signal stayed out, and the next call carried nothing.
- **AgentBox side deployed:** its daemon no longer installs the rider,
  so a live probe of two sessions got no `sync:` line. Its manuals
  name rig's line, and the probe's rider scenario is gone.

Known gaps, none blocking:

- **"Your lease was broken" has no test**; expiry does. It uses the
  same holder and by fields.
- **AgentBox's rider code stays** (`SyncRider`, `internal/mcp/rider.go`
  and their tests), now inert. Deleting it is a follow-up.
- **Mail is not on the line**: `message_inbox` and `message_await`
  carry it.
- **A session started before the install has no line** until its
  `rig mcp` restarts.

### Slice 6, #6: the wrapped run (designed 2026-10-02)

`rig peers run --lease=NAME [--ttl=30s] [--wait=0s] -- CMD ...` holds a
lease for exactly as long as one command runs. AgentBox's
`agentbox sync lock NAME -- CMD` releases when the command ends, but
nothing stops the command when the hold is lost. §16's fifth gate asks
for that: *"a stalled holder's work stops, not merely that its token
is rejected"*.

| Step | What rig does |
|---|---|
| take | acquires NAME, queueing up to `--wait`, witnessed by the `rig` process |
| run | starts CMD in its own process group, killed if `rig` dies |
| fence | `lease.fence` records the group's leader with the lease |
| hold | renews every TTL/3; exits with CMD's code and releases |
| lose | a failed renew stops the group: SIGTERM, then SIGKILL after 2 s; exit 75 |
| stall | at the deadline rigd kills the fenced group and the stalled `rig`, sees both dead, then frees it as `expired` |

- **Better than AgentBox by:** a stalled or suspended holder cannot
  keep writing while the next holder starts. rigd stops it first and
  posts `lease.changed` `fenced`, and the rider tells the holder.
- **rigd never kills a pid it was handed unchecked.** `lease.fence`
  refuses unless the pid leads its own process group, its parent is
  the caller's witnessed process, and it runs as rigd's user. The
  leader is recorded as a witness (pid, start ticks, boot id), so a
  recycled pid is never killed.
- **The fence is in the lease record**, so it outlives a rigd restart.
- **The holder is the `rig` CLI's seat**, so `terminal:<user>` from a
  shell. Holding under an agent's own seat is a later step.
- **AgentBox's side:** its manual and its own `make deploy` wrapper
  point to `rig peers run`; `agentbox sync lock` stays for the shells
  that use it, as `sync set` did.

### As built: slice 6, 2026-10-02

`694aeb2`, `a2b4a24` (size, rig +36 KiB, rigd +16 KiB), `54c701e`.

- **Built as designed.** `lease.fence` is a new verb; the fence lives
  in the lease record and a fresh acquire clears it.
- **The live demonstration found a defect the tests had missed.** Every
  shell of one user is the holder `terminal:<user>`, so a second
  `rig peers run` took a running one's lease by the reconnect path, and
  both wrote. The tests had used two different seats.
- **The fix, `54c701e`: the reconnect path is only for the process that
  holds the lease**, or for anyone once that process is dead. A holder's
  other live process is refused like any other holder, and may queue
  behind it; a wait on your own seat's lease is not a deadlock.
- **Proved by tests that fail when the property is removed:** rigd never
  killing, the CLI never fencing, any caller fencing, a non-leader or a
  recycled pid accepted, any parent accepted, and a holder's twin
  allowed to take over each broke a test. Race detector clean.
- **Demonstrated live on production:** a run writing a file had its
  `rig` frozen with SIGSTOP. A second run from the same shell user
  queued, was granted the lease 3.5 s later at the TTL, and the frozen
  run's writes stopped at 39 lines; its `rig` and its writer were gone.

Known gaps, none blocking:

- ~~The renew-failure and restart re-acquire paths had no test.~~
  Tested at `bfe2c3d`, each with a red control. **The renew test found a
  defect:** a renew to a gone daemon waited out a 5 s call timeout, so
  the run stopped 5.7 s after rigd went, past its 3 s hold. Each attempt
  is now bounded by the stop point (deadline less two graces), and the
  grace is a sixth of the TTL, at most 2 s. A restart is ridden out:
  the run re-acquires, re-fences and finishes with exit 0.
- **A group whose leader exited while its children run on is not
  killed**, because rigd cannot tell that the group id was not reused.
- **AgentBox's `make deploy` keeps its `flock`**, against the design:
  it was never `agentbox sync lock`, and a kernel lock does not need
  rigd up. AgentBox `180979a` points its manuals and ADR-0014 at
  `rig peers run`, deployed and read back from `agentbox docs agent`.

### Slice 7, #7: retraction (designed 2026-10-02)

**The row above was corrected on 2026-10-02.** It said "retract of a
signal or claim". AgentBox's `retract` does something else: it takes
back a `notify_user` card its own session posted, before Boris deals
with it. *"A 'build failed' whose build you have since fixed is worse
than no notification at all."* rig had no way to withdraw a toast.

| Step | What rig does |
|---|---|
| ask | `toast.retract {record_id?, reason?}`; with no id, every toast of the caller's still on screen |
| check | only the toast's own sender, named from the connection as `notify` names it |
| record | the notification record is retracted, the reason kept |
| screen | the ring carries the withdrawal; the bubble says "withdrawn" and leaves |
| reply | an unanswered ask is answered `withdrawn`, so `toast.answer` wakes and a late reply is refused |
| bus | `toast.retracted {record_id, sender}` |

- **Better than AgentBox by:** the record keeps the toast and its
  retraction, so what was said and taken back is never lost. A seat
  waiting on the reply learns it was withdrawn, not merely unanswered.
- **"Still on screen", with no id:** an unanswered ask or an urgent
  toast, which never close by themselves, or one inside its dwell.
- **A tray that has no renderer running starts none** for a withdrawal.
- **AgentBox's `retract` stays** while its `notify_user` cards do:
  it withdraws those, and the cards were not taken (decided for Boris,
  2026-10-02, to be put to him).

### As built: slice 7, 2026-10-02

`5ec5e00`, `2cf4d5d` (size, rig +12 KiB, rigd +20 KiB); AgentBox `08c64ff`.

- **Built as designed.** `toast.retract` is the verb, `toast_retract`
  the MCP tool, `rig notify retract [ID] [--reason R]` the shell's door.
  A `Toast` carries `retracted`, and a `ToastAnswer` carries `withdrawn`.
- **Proved by tests that fail when the property is removed:** any
  sender allowed, a sweep taking another's toast, a closed toast swept,
  the waiter not told, the tray not handed it, a second retraction
  counted, a withdrawal starting a renderer, and the feed drawing a
  withdrawal as a toast each broke a test. Race detector clean.
- **Demonstrated live on production:** an info toast, taken back after
  4 s, left the screen and its renderer exited 5 s later, against its
  own 20 s dwell. The record reads *"RETRACTED ... withdrawn by its
  sender: demo over"*. An urgent ask taken back while `rig notify
  --wait` waited answered *"taken back by its sender ... before anybody
  answered"*.
- **AgentBox's `retract` stayed, as designed, for one hour.** Then
  Boris ruled (decision 0260) that `notify_user` and `retract` both
  leave AgentBox: see "Ruled: notifying is rig's" below.

Known gaps, none blocking:

- **`make install` does not install `rigwindow`**; `make install-window`
  does, and the running tray needs a restart. The first live run used
  the morning's tray and looked like a failure: the bubble stayed for
  its whole dwell. Evidence for §37: it cost one false demo result.
- ~~A toast that went to the desktop's notification service was not
  taken down there.~~ Built at `d7781aa`: the tray keeps the ids `Notify`
  answered (the newest 64) and a withdrawal calls `CloseNotification`;
  a toast taken back before a late fallback is never put up. Tested
  with two red controls. **Live, on GNOME Shell 50.1:** `Notify` gave
  id 42 and `CloseNotification(42)` went through without error, but the
  shell had already signalled `NotificationClosed(42, reason 2)` 30 ms
  after showing it, and does the same for a plain `gdbus` notification.
  So on this desktop a fallback toast may never stay up at all. That is
  unexplained, and the screen was not looked at.
- **The sweep's "still on screen" is an estimate**: rig cannot see the
  pointer, so it doubles the page's dwell.

### Ruled: notifying is rig's (2026-10-02, decision 0260)

Asked *"should I remove AgentBox's `notify_user` and `retract`, now that
rig's `notify` and `toast_retract` do more?"*, Boris answered *"Dp all of
that"*. So:

- **AgentBox `b4002da`, deployed and pushed:** `notify_user` and
  `retract` are off its MCP surface, which serves 32 tools, counted
  live. Its manuals, ADR-0014 and the assignment prompt name rig's
  `notify` and `toast_retract`.
- **The cards that block on Boris's answer stay** (`ask_user`, forms,
  secrets, the countdown). They were never in this plan.
- **Not carried: `actions`**, notify_user's buttons that run a shell
  command on click. rig's replies go back to the sender instead. The
  `agentbox notify --action` CLI keeps them, and the warm-handoff skill's
  one-click trust prompt now uses that CLI.
