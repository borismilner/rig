## 42. What an agent is allowed to do

**RULED BY BORIS 2026-09-17, recorded the turn he said it.** ⛔ **HIS OWN SCOPE:
*"It is not critical for MVP but it should be implemented soon."* **

### His words, verbatim

> *"I want both the human user and the AI agents to be able to do this - as said
> previously, I want the AI agents to be able to do anything in the system.
> Which brings me to the following: I want to be able to configure restrictions
> on what AI agents are able to do; By default I want them to be able to call
> all MCP and do everything absolutely without restrictions, but perhaps I'll
> change my mind about some capabilities or maybe I'll want to grant some
> capabilities only to certain agents and not others so this must be supported."*

### ⛔ THE DEFAULT IS OPEN, AND THIS IS THE LINE A SEAT WILL GET WRONG

> **By default an agent can call ALL MCP and do EVERYTHING, absolutely without
> restrictions.**

⛔ **EVERY INSTINCT AND EVERY CONVENTION SAYS DEFAULT-DENY. HE SAID THE
OPPOSITE, TWICE, IN ONE SENTENCE - *"all"* and *"absolutely without
restrictions"*.** A seat that ships a capability behind an allowlist "to be
safe" has not been cautious, **it has built the wrong product and inverted a
requirement he stated in the plainest words available.**

**And it follows from §1's ordering:** agents come first in project and case
handling, and the bar there is *"the perfect environment to thrive in, with all
the facilities they need."* **A facility an agent must be granted is not one it
has.**

### What must be supported, and it is three things rather than one

| | |
|---|---|
| **1. the default** | ⛔ **everything, for everyone, with no configuration present** |
| **2. turn a capability off** | *"perhaps I'll change my mind about some capabilities"* - globally |
| **3. grant per AGENT** | ⛔ *"grant some capabilities only to certain agents and not others"* - **so the policy is keyed on WHICH agent, not only on which capability** |

⛔ **AND THE EXPORT/IMPORT OF §41 IS BOTH-HANDED BY NAME:** *"both the human user
and the AI agents to be able to do this."* **A capability reachable only from
the CLI has not met §41.**

### ⛔ THE IDENTITY THIS RESTS ON ALREADY EXISTS AND MUST NOT BE REINVENTED

**"Certain agents and not others" needs a name for an agent, and rig has
one.** §14 mints a `Principal` at accept from the socket's peer credentials;
presence carries a SEAT; §39's provenance is already written as
`terminal:<unix username>` or the announced seat, **derived by the daemon and
unforgeable by the caller.** ⛔ **THAT IS THE KEY THE POLICY IS WRITTEN
AGAINST.** §38's rule binds: **grep the plan before proposing a new identity.**

⛔ **AND THE UNSEATED CASE IS THE HOLE.** An unseated peer gets an empty seat and
generation 0, which `cutover-gap.md` already records as *"not an identity"*. **A
policy keyed on seat cannot express a rule about a caller that has no seat**, and
today the MCP door does not require one. **That is a precondition of this
section, not a detail of it.**

### ✅ WHAT THIS IS - ASKED AND ANSWERED BY BORIS, 2026-09-17

**The seat's reading was put to him and he confirmed it:**

> *"Is this me configuring your my agents. Avoiding being hacked by a hostile
> agent is something we'll defer to late in our development program. Our
> development must produce safe code, but we don't have a threat model yet so no
> point it doing premature optimizations."*

| | |
|---|---|
| ✅ **what §42 IS** | **Boris configuring HIS OWN agents.** A policy table he edits. **Build this** |
| ⛔ **what it is NOT, and it is DEFERRED rather than refused** | defence against a **hostile** agent. *"Late in our development program"* - **a schedule position, so it is NOT a non-goal and nothing here may be built in a way that forecloses it** |
| **why now** | *"we don't have a threat model yet so no point in doing premature optimizations"* |

#### ⛔ THE ONE DISTINCTION IN THAT ANSWER, AND A SEAT WILL COLLAPSE IT IN ONE OF TWO DIRECTIONS

> ***"Our development must produce safe code, but we don't have a threat model
> yet."***

**Two different things, and both collapses are wrong:**

| Collapse | What it produces |
|---|---|
| *"security is deferred"* | ⛔ **unsafe code, written on a licence he did not give.** He said the OPPOSITE in the same sentence |
| *"must produce safe code"* | ⛔ **a threat model and a sandbox, now** - the premature optimisation he named |

⛔ **SAFE CODE IS A STANDING QUALITY BAR AND IT IS NOT PART OF THIS SECTION'S
DEFERRAL.** It is craft, and it applies to every line: no injection-shaped string
building, no unchecked bounds, no secret in a log, no path taken from a caller
and joined without validation. **None of that needs a threat model - it is what
competent code looks like.** ⛔ **Recorded as a standing rule at §38, because it
binds all development and not only this section.**

⛔ **AND IT IS ALREADY LIVE IN THIS TREE RATHER THAN ASPIRATIONAL:** B65's field
predicate uses `json_each` and not a concatenated `json_extract` path precisely
so a caller's field name is never interpreted, and `TestAFieldNameIsNeverInterpreted`
pins it. **That is safe code written with no threat model, which is the proof
the two are separable.**

#### What stays untouched under either answer

§14's `scoped` boolean and §15's redaction invariant. **What `secrets.get`
returned is never recorded, only the key name** - a policy granting an agent
secret-reading would still not make secrets introspectable, because that is a
different ruling and this section does not reach it.

### ⛔ THE MECHANISM MUST COST NOTHING WHEN IT IS EMPTY

**The default is no configuration at all**, and that is the state rig will be in
for most of its life. So:

- **absent configuration is not an empty allowlist.** ⛔ **THE EMPTY-VERSUS-ABSENT
  DEFECT, ARRIVING WHERE IT CAN DENY WORK** - this project has recorded it five
  times in other shapes, and here it fails toward a seat that cannot work and
  cannot see why.
- **a refusal names the rule that produced it**, the capability, and the agent.
  An agent told only *"denied"* cannot report the gap, which §37's ARMED path
  requires it to do.
- ⛔ **AND HE MUST BE ABLE TO SEE THE POLICY IN FORCE**, per §1's second
  objective: *"introspect on everything that is of importance and being able to
  interact."* **A restriction he cannot list is one he cannot remember setting.**

### Acceptance

⛔ **THREE DEMONSTRATIONS, AND THE FIRST IS THE ONE THAT WILL BE SKIPPED:**

1. **with NO configuration present, an agent calls every capability and is
   refused none.** Watched green against a real caller, not reasoned.
2. one capability turned off globally, and the refusal names the rule.
3. one capability granted to one agent and refused to another, **distinguished by
   an identity the caller cannot set.**

### Where it sits

⛔ **NOT CRITICAL FOR THE MVP - HIS WORDS - AND *"SHOULD BE IMPLEMENTED SOON"*,
which is the same weight he gave §40.** The MVP is §39 slices 1, 2 and 4.

### ⛔ A CLI-WRITTEN RECORD CARRIES WHAT THE DAEMON CAN CHECK, AND NOTHING MORE. RULED BY BORIS 2026-09-17.

**The measurement that forced the ruling:** 451 decisions in the production
store, **one distinct seat, 783 sessions**. Every record written through the CLI
is anonymous, because `terminalSeat` returns the username from the uid and there
is one daemon per uid - **so the seat is a machine-wide constant, not an
identity.** A terminal has no name the daemon can check.

**Boris was shown three answers and took the first:**

| | |
|---|---|
| ✅ **RULED** | **derive what is derivable.** The daemon already holds the caller's pid from `SO_PEERCRED`, so it can derive a real per-terminal SESSION id from the unix session or tty. **The seat stays the username** |
| what it buys | two terminals become distinguishable, and a run of writes from one terminal groups together as one session instead of minting a new one per write |
| ⛔ **what it does NOT buy, and he was told** | the seat remains a machine-wide constant. **It answers "which terminal", never "which agent."** Do not report it as attribution |

⛔ **TWO ANSWERS STAY REFUSED, AND THEY WERE REFUSED BEFORE THIS RULING RATHER
THAN BY IT.** A seat name carried in the REQUEST is refused because provenance
must be the daemon's and unforgeable; terminals handshaking for a name is refused
on §37's own terms. **This ruling does not reopen either.**

⛔ **AND THE AGENT SURFACE IS ALREADY WHOLE - the gap is the CLI's alone.** The
MCP door refuses a record write from a connection holding no seat, with §9's four
fields, and the seat it stamps comes from the roster row rather than from the
request. **An agent that needs an attributable write already has one.**
