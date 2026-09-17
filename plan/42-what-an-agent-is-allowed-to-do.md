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

### ⛔ WHAT THIS IS, AND WHAT IT IS NOT - FLAGGED AS THE SEAT'S READING

⛔ **THIS IS BORIS CONFIGURING HIS OWN AGENTS. IT IS NOT A SECURITY BOUNDARY
AGAINST A HOSTILE ONE, AND THE TWO BUILD COMPLETELY DIFFERENT THINGS.** A policy
surface is a table he edits; a security boundary needs a threat model,
sandboxing, and an assumption that the caller lies. **He said *"configure"* and
*"grant"*, which are the words of the first.**

⛔ **THIS READING IS THE SEAT'S AND HAS NOT BEEN PUT TO HIM. ASK BEFORE
BUILDING** - it is the difference between a configuration file and a quarter of
work, and getting it wrong in the expensive direction looks responsible.

**What is untouched either way:** §14's `scoped` boolean and §15's redaction
invariant. **What `secrets.get` returned is never recorded, only the key name** -
a policy that granted an agent secret-reading would still not make secrets
introspectable, because that is a different ruling.

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
