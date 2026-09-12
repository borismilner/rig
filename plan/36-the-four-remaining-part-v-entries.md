## 36. The four remaining Part V entries, adjudicated 2026-09-11

**§34's second count was open and said so.** Seventeen of Part V's 22 entries
were adjudicated there; **V13, V15, V18 and V20 remained.** They are closed
here, which closes count 2. Record of the method and of the first seventeen:
`logbook/projects/rig/count2-part-v-audit-2026-09-11.md`.

**Each verdict is per CLAUSE, not per entry**, because an entry answering four
of seven clauses is `partial` and the missing clauses are the backlog - and
recording only entry-level verdicts is how "mentions it" passes for "specifies
it".

### V13 - bounded queues and the gap signal: now `partial`, was absent

**Answered by §16's message contract**: a cursor per subscriber, `gap: true`
when the cursor has fallen behind retention, everything since the cursor in one
batch, and a bounded payload that is **refused rather than truncated**.

**Two clauses remain and they are ruled here, because leaving them open is the
robustness hole:**

- **A full queue REFUSES the post; it does not drop the oldest.** Dropping the
  oldest is silent, and it discards precisely the message that has been waiting
  longest - which correlates with the recipient being in trouble. A refusal is
  loud and the poster can act on it.
- **A slow consumer is a health signal, not a queue policy.** A subscriber
  falling behind is reported as such, on the same surface that reports a session
  idle-and-not-blocked (§18). **The queue does not quietly compensate for a
  consumer that has stopped consuming**, because compensating is what makes the
  failure invisible until the buffer is gone.

### V15 - single daemon and epoch handles: now `partial`

**The single-instance claim exists** and behaves correctly - an `flock` on an
open descriptor, so a dead daemon's claim is released rather than left stale.

**The epoch half is ABSENT and it is a robustness gap, not a nicety.** Measured
2026-09-11: nothing rig holds survives a restart, and a client's declaration is
rebuilt by re-registering. **But a client cannot currently tell a daemon restart
from a network blip** - it reconnects, re-registers, and is not told that the
thing it reconnected to is a different process with none of its state.

**Ruled: the daemon publishes an epoch, and every handle carries it.** A handle
presented across an epoch boundary is refused as stale, naming the epoch rather
than failing generically. §18's matrix says what is lost; **the epoch is what
makes a client able to ASK rather than infer.** Inference here is the same class
of defect as a check that cannot tell "nothing is wrong" from "it did not run".

### V18 - session identity is not connection identity: now specified

**Measured: today they are the same thing.** Session ids are minted per
connection by the daemon and never travel to any caller.

**§16's seats make that wrong**, and the contradiction has to be resolved
rather than left for whoever notices it: a **seat outlives every connection**,
a **generation** is one occupancy, and a **connection** is shorter than both.
Three lifetimes, and collapsing any two of them loses something:

| Identity | Lives as long as | Addressable by a peer |
|---|---|---|
| Seat | the role exists | **yes - this is the address** |
| Generation | one session's occupancy | yes, to prove who you are talking to |
| Connection | one socket | **no.** It is transport, and no peer should ever hold one |

**A peer never holds a connection identity**, which is the clause that was
missing and the reason the byte-identical refusal was not obviously wrong: the
one identity in that message is a connection-scoped session id that no caller
has ever seen.

### V20 - projection totality: now specified, and it lands on work in flight

**A projection must be TOTAL**: every piece of state it covers is either
represented or explicitly named as not representable. A projection that silently
omits is a projection that lies, and it lies specifically to the introspecting
caller who cannot check.

| Clause | Ruled |
|---|---|
| **State not representable in the projection is NAMED as such** | never omitted. The caller is told the projection has a hole, and where |
| **Sanitisation is declared, not incidental** | a field removed for a scoped caller is reported as withheld, not as absent. **Absent and withheld are different facts** and only one of them means "ask for more access" |
| **The projection states its own coverage** | which is §5k's coverage principle applied to rig itself, and the same rule that refused `rig doctor` at M1 |

**This is live work, not a future clause.** M2 slice 2 is the registry
projection and it is being built now; slice 4's capability map is the same rule
at a larger scale, where *"a scoped caller and an introspecting one read
different maps at the same instant"* is exactly the withheld-versus-absent
distinction above.

**HOW V20 IS APPLIED TO THE CAPABILITY MAP: THE BASIS MARKER, AND A WITHHELD
LIST IS REFUSED.** Annotated 2026-09-11, owed since the map shipped and never
written down. **The map says HOW IT WAS FILTERED, not WHAT WAS REMOVED.** Read
without this, the withheld-versus-absent clause above appears to demand an
enumeration of what a scoped caller did not get - and a seat acting on that
would build the very oracle the registry refuses, with §36 apparently on its
side. **The reasoning, and the entry that owns it, is `DECISIONS.md`'s
basis-marker entry** - it is deliberately not restated here, because two copies
of one argument drift and the log is the durable half.


---
