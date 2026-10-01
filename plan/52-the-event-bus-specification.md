## 52. The event bus specification

⛔ **DRAFT. STEP 1 AND A STEP-4 DRAFT OF §45's LOOP FOR B107, WRITTEN
2026-10-01.** Boris put the bus first (§44, *"Put the bus next, before
storage"*) and said *"start working on it"*. Under §45 the start is this
draft: **he has not yet approved, reshaped or killed it, and no subagent is
spawned from it until he does.** Every row is the seat's unless it cites a
ruling.

> **In one line: something happens, rig numbers it on one global counter,
> and every connection that armed a matching kind gets everything since its
> cursor in one batch, or `gap: true` when it fell too far behind.** A kind
> can be caused (the hand started driving) or timed (a cron entry fired).

---

### Step 1. What it is, what a program does with it, what it costs

**What it is.** One in-memory log in `rigd`: an event is `{seq, kind, at,
source, payload}`. Publishing appends and wakes the waiters. A waiter asks
for *"everything after seq N whose kind matches these patterns"* and parks
until there is some. The shape is §16's watch, reused: one global revision,
a cursor per subscriber, one batch per wake, and `gap: true` past retention.

**What a program does with it.**

| It wants to | It does |
|---|---|
| hear that something happened | `rig.events.wait {after, kinds: ["hand.*"], timeout_ms}` in a loop, carrying `latest` forward |
| say that something happened | `rig.events.publish {kind: "<its id>.<name>", payload}`, for a kind it declared under `events` (§5e) |
| run at a time | (step 2: see Q1) arms a timer; rig publishes `timer.fired` to it |
| catch up after a restart | waits from the cursor it kept; a `gap` means re-read the state, never assume nothing happened |

**What it costs.**

| Cost | Size |
|---|---|
| New verbs | 2 (`events.publish`, `events.wait`); the timer verbs are Q1's |
| New dependency | none for the bus; `robfig/cron/v3` only if Q1 says cron syntax |
| Memory | a ring of the last 4096 events, payloads at most 16 KB each: a bounded 64 MB worst case, a few hundred KB in practice |
| Code it removes | the hand-built wake-and-cursor loops in `toast.wait` and `hand.wait` |

---

### The rows

| | Rule | Why |
|---|---|---|
| E1 | **One global `seq`**, never reused while `rigd` runs; a restart starts a new epoch and every cursor from the old one answers `gap: true` | §16: a cursor that cannot tell "nothing happened" from "I missed it" is the failure |
| E2 | **A kind is dotted lower case; a pattern is a kind or a prefix ending in `.*`**. No other wildcards | §16's topic families; a regex filter is a performance and a footgun question nobody asked |
| E3 | **A program publishes only kinds it declared, under its own id** (`graft.job.done`). Bare kinds (`hand.changed`) are rig's alone | §5e's `events` field; a program forging `program.health` would be an invoker by the back door (§5: a subscription is a filter, never a trigger) |
| E4 | **An event carries what happened, never a command.** The payload is JSON, at most 16 KB; over that is refused, not truncated | §5 and §16's "a payload where a pointer belongs" |
| E5 | **Waiting needs the `subscribe` grant for that kind** (§13). rig's own surfaces (tray, window, CLI) hold it for rig's kinds | §13 row `subscribe` |
| E6 | **A wait parks at most 60 s**, then answers empty with `latest` | the same bound as `toast.wait` and `message.await`: a call that can park forever is a connection rig cannot account for |
| E7 | **Retention is in memory until S1 lands.** Past the ring, the answer is `gap: true` and the oldest seq still held | Boris put the bus before storage (2026-10-01); durability is S1's, and the gap makes its absence visible |
| E8 | **Delivery is at most once per wait and in seq order**; a subscriber that must not miss one keeps its cursor | a push that retries is a queue, and rig has queues (§16) |

---

### The first kinds, each with a user on the day it lands

| Kind | Published by | First user |
|---|---|---|
| `hand.changed` | rigd, on every hand state change, payload the `HandState` | the tray (starts the strip), the strip, righand's park latch: today's `hand.wait` |
| `toast.posted` | rigd, on `rig.notify` | the tray's renderer: today's `toast.wait` |
| `system.resumed` | rigd, when `CLOCK_BOOTTIME` jumps past `CLOCK_MONOTONIC` | every timer; §5 names it as a live defect in every program |
| `timer.fired` | rigd, when an armed time comes (Q1) | the task scheduler (§25); Boris: *"future events, like chrontab"* |

`hand.wait` and `toast.wait` stay as verbs for one release, answered from
the bus, so nothing deployed breaks. **`message.await` is not folded in**:
mail is durable, per seat and carries five states (§16), which an
in-memory event log cannot honour. Only its wake-up may ride the bus.

---

### Searched, per §38

| Candidate | Verdict |
|---|---|
| `asaskevich/EventBus` | set aside in §5 already: in-process callbacks, no cursor, no gap |
| `cskr/pubsub` | channels per topic, no replay from a cursor: the property that matters is missing |
| `ThreeDotsLabs/watermill` | a framework over brokers; its in-memory `gochannel` has no cursor either, and it brings a large tree |
| embedded `nats-server` + JetStream | has cursors and durability, and is the real alternative; ⛔ a second server inside `rigd`, against §2's footprint (its size not measured here) |
| `robfig/cron/v3` | **the cron parser if Q1 wants cron syntax**: standard, small, no dependencies |

The ring and the wake are already written twice in this tree
(`toastRing`, `handDesk`); the bus is those, once.

---

### Unresolved: step 2, for Boris

| | Question | The seat's lean |
|---|---|---|
| Q1 | **Who owns the clock for "like crontab"?** rig fires armed timers (`timer.fired`), or the scheduler program keeps its own clock and only publishes | rig: it is the only one that knows about suspend (`system.resumed`), and §18 already rules on missed fires |
| Q2 | **Timer syntax:** cron lines, plain `every 15m` / `at 09:00`, or both | both, cron via `robfig/cron/v3` |
| Q3 | **A missed fire after suspend:** fire once on wake, fire every missed one, or skip | once, with `missed: N` in the payload (§18: cron drops, anacron floods, both wrong) |
| Q4 | **May an agent's MCP door wait on events?** | yes for rig's kinds, behind the same grant; it is how an agent hears the hand stopped |
