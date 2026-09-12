## 15. Who is using rig, and what they did

§14 says clients cannot see each other. **A holder of `introspect` can see all of them, and
that includes every agent Boris runs** (§2). The asymmetry is deliberate: isolation is between
clients, not between rig and the people and agents working for its owner.

Both halves of that sentence are corrections, and they were made a day apart. `introspect` is
decided at connect time (§14), not a uid and not a token - because
without that correction every client on a one-user machine reads everything by accident, and
`rig history --client=X` hands over another program's secrets. And it is deliberately **not**
the same grant as acting estate-wide, because the version of the fix that made this section
safe also made it useless to the reader it exists for.

### The clients view

| Column | For every connected client |
|---|---|
| Who | Kind (agent, terminal, window, script, program), pid, command line, uid, session id |
| Since | Connected at, last active, wire version, stub build |
| Holding | Leases held, crew membership, subscriptions, capabilities granted |
| Waiting | What it is blocked on, and for how long |
| Doing | Calls in flight right now, with elapsed time |
| Rate | Calls per second, bytes, error rate, denied capability attempts |

Selecting a client opens its history: every call it made, when, with what arguments, how long it
took, what came back - **minus everything declared sensitive**, which was never recorded at all.
The same view exists in the window, the TUI, the CLI (`rig clients`, `rig history --client=X`)
and over MCP, because they are all surfaces over one registry, and all of them require
`introspect` (§14). **An agent Boris runs holds it**, so `rig history --client=X` is a question
an agent may ask about any client, including another agent - that is §2's decision, and the
redaction above is what makes it safe rather than the scoping that used to sit here.

**Looking is itself an event.** Opening another client's history is written to the audit log.
On a single-user machine that is close to pointless, but it costs nothing and it means the rule
is the same rule when it stops being a single-user machine.

**Coalesced, because the reader is usually an agent.** Written for a person opening one
history, per-read auditing turns the log into a transcript of whatever agent is sweeping the
estate (§9). The record is therefore **one entry per principal, per view, per minute, with a
count and the widest range touched** - which answers "who read what, when" exactly as well and
does not drown the thing it is auditing. A grant being *presented* is always its own
uncoalesced entry, so the interesting event - a principal becoming an introspector - is never
buried in a count.

### Redaction, or the history is an exfiltration channel

**`secrets.get` is a call, and this section records what every call returned, verbatim.** With
no redaction anywhere, a keyring token lands in an unencrypted segment kept for thirty days, and
`rig history` hands it over. Conformance item 15 does not catch it: that tests the *program's*
logs, not rig's own history.

Redaction is therefore part of the **registration declaration**, not of the logger:

- A command declares `sensitive`: a list of JSON pointers into its arguments and its results.
  Mandatory, may be empty (§5e).
- At registration rig compiles that list once per `(method, wire version)` into a **byte-span
  list over the wire encoding**. The hot path blanks known offsets: **82.5 ns, zero
  allocations**, inside budget. Decoding the payload to redact it at record time costs
  **3100 ns**, which is 15x the whole budget - so the declaration is the only affordable place
  for this.
- **Redaction is a write-side mechanism, and this plan has been stating it as though it were a
property of reading.** The compiled span list blanks known offsets on the way *into* the
history, so every estate-wide read that goes through the history is safe by construction. A read
path that does not go through the history is not, and §14's sentence - "the worst an
introspecting agent can read is everything Boris could read himself" - is true only of the paths
that cross that write. Every conformance run in §14 would pass while such a path leaked, because
run 3 reads the transcript and the leak is not in the transcript.

**The invariant, stated once so that a new read path has to satisfy it rather than notice it.**
Every estate-wide read path either:

1. renders its payload through **the same compiled span list** as the history write; or
2. declares its payload **wholly sensitive** - existence, size, age and owner readable
   estate-wide, contents never.

§16's continuation slots already take (2) and say so in their own text. Anything new picks one
explicitly, in writing, and **a third option does not exist**: a second access-control door is
precisely what §14 says it does not have.

**Why (2) has to be available at all.** `sensitive` is JSON pointers into arguments and results
(§5e), so a payload that is neither - a live outbound stream, an opaque blob a program owns -
has no pointer space in which the declaration could be made. For those, (2) is the only
mechanism there is, and a design that reaches for (1) and finds no pointer must take (2) rather
than conclude the question does not apply.

**Anything the `secrets` service returns is never recorded at all**, only the key name. A
  field that is not declared sensitive *is* recorded, so the default for a secret cannot be left
  to a declaration.
- The declaration is now security-relevant, so widening it needs the same explicit confirmation
  §13 requires for widening a capability.

### History has to be nearly free, and it can be

You gave the permission that makes this cheap: **losing recent entries to a crash or a power cut
is acceptable.** Durability is what makes event logging expensive, so not needing it changes the
design completely.

```
  call happens
      │
      ▼
  per-client ring buffer      sharded, so the cursor and the interner are
      │                       uncontended by construction, not by luck
      │  flush armed BY THE FIRST RECORD, then 250 ms or 256 KB
      ▼
  write(2) to a segment file  NO fsync, ever
      │                       the kernel now owns it
      │  on rotation
      ▼
  zstd the closed segment     + a self-describing header
                              + a per-segment column summary
                              + an offset index (rebuildable)
```

**Six things that keep it cheap, and each one was wrong in at least one way before.**

- **No fsync in the hot path.** The whole reason it is affordable, and a locked decision.
- **Sharded per client, merged at flush.** A single shared ring measured 2.1x over budget on the
  mean at ten clients and 5.1x at p99; with the obvious RWMutex-map interner, 33x over. The
  budget below is stated at p99 with sixteen concurrent clients, because a single-writer mean is
  not a number anyone experiences.
- **Interned, fixed-width records**, with the **dictionary written into the segment**, not the
  index. A segment holds the integer 42, not `peers.lease.acquire`; if that mapping lives only
  in the index then "delete the index, it rebuilds by scanning" - which this plan used to
  advise - destroys data and breaks §7's locked rule. Each segment carries a header of the
  id→name pairs first referenced in it, appended at rotation from data already in memory. A
  segment then decodes standalone, ids are scoped to the segment that defines them, so a rename
  coexists with its old spelling instead of colliding. Client ids are interned **per principal**,
  not per connection, so a Makefile loop does not mint 100k entries.
- **The flush timer is armed by the first record**, not run on an interval. An idle daemon has no
  history timer at all, which is where §17's wakeup budget went: a 250 ms ticker alone measured
  **45.5 wakeups/second** against a stated budget of one.
- **Sampling under load, with the rate recorded**, so counts stay correct.
- **Retention by size, age AND rate.** 500 MB holds 10.56M records; ten programs health-checking
  once a second collapses "thirty days" to 12.2 days, and one client at the wire's own 161k
  calls/s unlinks everything in 66 seconds. A per-client rate ceiling is enforced before the
  size ceiling, so one noisy client cannot evict the estate's record.

### The coverage log, so a gap is visible rather than silent

Four separate paths lose records - sampling engaging, the ring overwriting, a segment being
unlinked, a segment failing to close cleanly - and without this they are indistinguishable from
"nothing happened", which makes the whole log useless for the one question it exists to answer.

One small append-only object: `(from_ts, to_ts, client_id or *, cause, retained/total)`, written
on the cold path whenever any of those four occurs. **Every view in §8 and §15 renders a gap
band and refuses to answer "every call it made" without stating its coverage.**

**The audit log and the peers timeline are never sampled and have their own budget.** They are
not the same kind of object as a call log and must not share its eviction.

### Querying thirty days without nine cores

500 MB of zstd is 2.0 GB raw; at a measured 1197 MB/s/core, a full scan is 1.7 s of core time,
so the old "< 200 ms over 30 days" needed 8.5 cores. A segment is therefore not the unit of
reading. On rotation, alongside the blob and the index, rig writes a **column summary**: the set
of client ids present, the set of method ids, min/max timestamp, error count, and a bitmap of
which client appears in which one-second bucket. A few KB per 64 MB segment, computed from data
already in hand.

### The budget

| What | Target | Basis |
|---|---|---|
| History append, **p99 at 16 concurrent clients** | **< 200 ns** | measured 110.8 ns single, 334.9 ns unsharded at 8 |
| **Whole recorded call**: history + span + slog | **< 2 µs** | measured 1757 ns (§8). The old budget bounded one third of the cost |
| Redaction, per sensitive field | **< 100 ns** | measured 82.5 ns as compiled spans |
| Worst-case history lost to a power cut | **< 1 s** | unchanged |
| Memory: all history buffers | **2 MB**, configurable | was 8 MB, which was 41% of the whole RSS budget (§17) |
| Memory: interning dictionary, resident | **512 KB** | new line; it was previously unbudgeted |
| Disk, default | **500 MB** call log, **plus** a separate audit + timeline budget | oldest dropped first, with a coverage entry |
| **Filtered** 30-day query | **< 200 ms** | what the column summary delivers |
| **Unfiltered** 30-day scan | **seconds, streaming** | the truth, and it is fine |

All of them are asserted by `make bench-idle` and a history-specific benchmark, so a regression
fails the build rather than being noticed a year later. Index rebuild is an explicit background
task with history served degraded-but-labelled meanwhile, so it cannot blow §17's cold start.
