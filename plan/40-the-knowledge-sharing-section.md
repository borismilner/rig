## 40. The knowledge-sharing section

**RULED BY BORIS 2026-09-17, and recorded the turn he said it.** Not on the MVP's
critical path, and **he said it should be done soon.**

## His words, verbatim

> *"`rig` shall have a knowledge sharing section, mainly for the use of AI agents
> but human users may use it too."*

> *"All important lessons learned that are of use in the future, specially if is
> of great importance for many different users the AI agent can choose to put it
> in the knowledge sharing section to be available to all other AI agents and
> human users as well."*

> *"AI agents may choose to efficiently take a look at the knowledge sharing
> section to look whether it contains relevant information to problems in
> question when needed, maybe before doing a deep dive on things maybe other
> agents have already did the research and doing it again would be a waste of
> time and tokens."*

> *"They are not to read the whole content because it is huge; The content is to
> be very efficiently indexed and exposed to the AI agents so that they don't
> waste tokens unnecessarily and get to the relevant information only when seems
> as a great fit."*

## What it is, in one line

**A place a lesson is written ONCE by whoever learned it, and found by whoever
needs it - across projects, across agents, and across sessions that share no
context.**

## ⛔ The hard requirement is the index, not the store

**He stated the failure mode himself and it is a token argument, not a tidiness
one:** the content is huge, an agent must not read it whole, and an agent must
reach the relevant part **only when it looks like a great fit.**

| Requirement | Why it is his, not a seat's |
|---|---|
| **write once, read by everybody** | *"available to all other AI agents and human users as well"* |
| **the agent CHOOSES to contribute** | *"the AI agent can choose to put it"*. ⛔ **Not every lesson - the bar is *"important"* and *"of great importance for many different users"* ** |
| **the agent CHOOSES to consult** | *"may choose to efficiently take a look"*. A lookup, not a mandatory gate |
| ⛔ **consult BEFORE the deep dive** | *"maybe other agents have already did the research and doing it again would be a waste of time and tokens"*. **This is the whole return on the capability** |
| ⛔ **NEVER READ WHOLE** | *"They are not to read the whole content because it is huge"* |
| ⛔ **VERY EFFICIENTLY INDEXED AND EXPOSED** | so an agent *"gets to the relevant information only when seems as a great fit"* |
| **humans too** | *"mainly for the use of AI agents but human users may use it too"* - a second consumer, and §39's two-consumer rule therefore binds it |

## Scope, stated so a seat does not widen it

**ESTATE-SCOPED, NOT PROJECT-SCOPED.** *"available to all other AI agents"* is
across projects: a lesson learned in rig must be findable from another project,
or the capability answers a question nobody asked. That puts it beside the
standards register in §39's estate/project split rather than inside a project's
own record.

⛔ **THIS IS NOT THE CONTINUITY RECORD AND MUST NOT BE FOLDED INTO IT.** §39's
record is *what happened on this project* - decisions, requirements, work,
progress. This is *what was learned that outlives the project it was learned
in*. Same store, different question, and a lesson filed as a project decision
is invisible to the next project, which is the one thing he asked for.

## ⛔ What is not decided, and must not be decided by default

**The indexing mechanism is the whole difficulty and it is UNCHOSEN.** He
specified the PROPERTY - efficiently indexed, exposed cheaply, reached only on a
good match - and not the means.

- **§38's first standing rule binds this before any code:** *never reinvent what
  a good Go library already does.* Search first, name the candidates, and
  *"I did not find one"* is only an answer after a search you can describe.
- **§39 already names the two candidates this will reach for** and they are
  Boris's own: **SQLite FTS5** for full text, **`bleve`** beside it. ⛔ **AND
  SEMANTIC SEARCH IS A SEPARATE DECISION, NOT A LIBRARY** - §39 says so already:
  it brings an embedding model, a runtime and a model-versioning problem, and it
  is chosen deliberately or refused deliberately, never by default. **A
  relevance-ranked index over lessons is exactly the surface that invites it in
  by accident.**
- ⛔ **B28's search covers this axis and has still never been run.** Its job is
  to confirm or beat SQLite. **Sequencing an index here before B28 would answer
  a measurement with an implementation**, which is the same mistake B63's row
  records about the import.

## The one thing to get right, and it is measurable

⛔ **THE COST OF CONSULTING MUST BE SMALL ENOUGH THAT AN AGENT ACTUALLY DOES IT.**
A lookup that costs more tokens than a quick re-derivation will be skipped, and
a knowledge base nobody consults is worse than none: it accrues, it looks like
coverage, and it is a cost with no return.

**So the acceptance test is a NUMBER, not a demonstration that it works:** what
does one consult cost, in tokens, against what the deep dive it replaces would
have cost. **That number is the capability.** It is also the first thing in this
project that would be measured against rig's own ARMED reports, which are the
evidence for what agents actually re-derive by hand.

## Where it sits

**NOT ON THE MVP PATH.** Boris: *"it is not on the critical MVP path but should
be done soon."* The MVP is §39 slices 1, 2 and 4 - being able to use rig to work
on rig. **This is the slice after the ground under it is real**, and it depends
on the store search (B28) and on the record store already carrying kinds and a
field predicate, both of which exist.
