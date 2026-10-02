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

### The six, in his order

| # | Capability | rig's form | Better than AgentBox by |
|---|---|---|---|
| 1+5 | signals; rig posts lock and peer changes | `signal.*`, `lease.changed`, `roster.changed` on §52's bus | the sender is the daemon's seat, never a request key; `to_seat` is that seat's alone |
| 2 | blocking acquire, FIFO | `lease.acquire` with `wait_ms`, a queue per lease | refuses a wait that would deadlock (A waits on B's lease while B waits on A's) |
| 3 | witnessed claims on a shared table | the store's `expected_version` CAS plus a witness | an entry dies with its owner, as a lease does |
| 4 | the `sync:` rider | what changed since your last call, on any answer | no extra call to learn the world moved |
| 6 | the wrapped run | `rig peers run --lease=NAME -- cmd` (§16) | expiry kills the writer, the only real fence rig has |
| 7 | retraction | `retract` of a signal or claim | recorded, so a retraction is seen as one |

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

**What it does not do yet:**
- **A signal does not outlive rigd.** §52's ring is in memory; a
  restart answers `gap: true`. AgentBox keeps signals 7 days in SQLite,
  so `post_signal`/`await_signal` stay in AgentBox until rig's signals
  are durable.
- **An expiry posts nothing.** coord finds an expired lease when it is
  next touched. Slice 2's queue needs the wake, so it adds the post.
- **No `rig signal` at the prompt.** Agents use `events_publish`.
