## 1. The idea, in one line

### ⛔ WHO rig IS FOR, AND IN WHICH ORDER. RULED BY BORIS 2026-09-17.

**Two statements, hours apart, and the second SCOPES the first. Read them
together or the ordering comes out wrong.**

> *"What is important about `rig` is that it perfectly serve the AI agents, and
> the secondary objective is me being able to introspect everything."*

> *"`rig` will serve different needs, as I said previously, not only AI-agents.
> The actual goal is to serve me as its creator and user, but in handling
> projects and cases it's definitely AI-agents first, they must have the perfect
> environment to thrive in, with all the facilities they need and the secondary
> goal is for me to introspect on everything that is of importance and being
> able to interact."*

⛔ **THE FIRST WAS RECORDED HERE AS A FLAT "AGENTS ARE PRIMARY" AND THAT WAS
WRONG. HE CORRECTED IT THE SAME DAY.** The ordering is real and it is **SCOPED**:

| Scope | Who comes first |
|---|---|
| **rig, overall** | ⛔ **BORIS. *"The actual goal is to serve me as its creator and user."* ** rig serves different needs and the agents are one of them |
| **handling PROJECTS AND CASES** - §39's whole subject | ⛔ **AI-AGENTS FIRST, and the bar is *"the perfect environment to thrive in, with all the facilities they need"* ** |
| **the second objective, inside that scope** | he introspects **everything that is of importance** - ⛔ **AND INTERACTS.** Not a read-only window |

⛔ **THE SEAT THAT WROTE THE FIRST VERSION GENERALISED A SCOPED STATEMENT INTO A
GLOBAL ONE, WHICH IS THE SECOND TIME IN ONE DAY THIS SEAT DID THAT** - the other
was a one-session handover rule written into §39 as standing. **RECORD WHAT HE
SAID THE TURN HE SAYS IT, AND RECORD ITS SCOPE WITH IT.** A requirement whose
scope is guessed is worse than one that is late, because it reads as settled.

#### What the ordering decides, concretely

**Inside project and case handling, where agents come first:**

- **a surface that is cheap for an agent and plain for a human BEATS one that is
  handsome for a human and expensive for an agent.** §40's knowledge-sharing
  index is this shape in his own words: *"very efficiently indexed and exposed
  to the AI agents so that they don't waste tokens"*.
- **token cost is a FIRST-ORDER design constraint, not an optimisation.** An
  answer an agent cannot afford to read has not served it.
- ⛔ **"ALL THE FACILITIES THEY NEED" IS `plan/09` A0, AND IT IS STILL
  UNDISCHARGED.** *"agents get every facility they need or may need, and the
  LEAD owns discovering what those are."* **Zero written.** This statement is
  the second time he has made that point and it raises the bar from *needed* to
  *thrive*.

**And the second objective has teeth of its own:**

- ⛔ **"INTROSPECT ON EVERYTHING THAT IS OF IMPORTANCE AND BEING ABLE TO
  INTERACT."** Interaction is named. **A read-only surface does not satisfy
  this**, and "he can see it in a log" is not introspection.
- **It is a COMPLETENESS bar, not a beauty one.** Not satisfied by a nicer GUI,
  not violated by a plain one. ⛔ **The GUI beauty requirement at §11 is his and
  stands on its own - it is NOT this row, and reading this as a licence to let
  the window rot inverts both.**
- **A capability an agent can use and he cannot see or reach is a FAILURE of the
  second objective**, not an acceptable trade. The ordering decides ties; it
  does not delete the loser.

**An app declares what it can do, once. rig projects that onto every way anyone might reach it.**

```
       shelf    graft    archi   snapper   nudge   grabbit
         └────────┴────────┴────────┴────────┴────────┘
                            │  declares once:
                            │  commands · config schema · state · data · events
                            ▼
                 ┌───────────────────────┐
                 │          rig          │
                 │  one registry, one    │
                 │  truth, one contract  │
                 └───────────────────────┘
                            │  projected onto every surface
   ┌──────┬──────┬──────┬───┴──┬──────┬──────┬──────┬──────┐
  GUI    tray   CLI    MCP    HTTP  palette cron   URL   toast
  pane   item  subcmd  tool   route  entry   job  scheme action
   │      │      │      │      │      │      │      │      │
 window hotkey terminal agents scripts keys  time  links  phone
```

The right-hand row is free. An app writes nothing to get a CLI subcommand, an MCP tool, a tray
entry, a schedulable job or a button inside a toast. It declared its commands; rig did the rest.

**And it compounds.** Add a new surface to rig in 2028 - a voice interface, a phone app, a new
UI - and every program already has it, without being touched.

**BORIS, 2026-09-16, verbatim - ONE STOP SHOP FOR THE IN-HOUSE TOOLS,
NAMED AS AN ORIGINAL REASON RIG EXISTS:** *"In our plan for rig consolidate
my requirement to port AgentBox GUI into rig GUI as one of the original
reasons rig exists, to be one stop shop for all of our in-house tools, like
AgentBox, so we don't need a dedicated system-tray for it, it will be
accessible through rig."* AgentBox today runs its own tray (`fyne.io/systray`,
`internal/tray/` in that repo) and its own window, entirely outside rig - the
opposite of this section's own diagram, which has every program reaching the
tray and the GUI THROUGH rig rather than shipping its own. Porting AgentBox's
GUI into rig's is that diagram closing its largest open gap: the tool Boris
actually runs every day is not yet one of the programs it draws.
**NOT PRIORITY, HIS OWN WORDS:** *"It doesn't take precedence, we'll do it
once all the ground work is finished that allows this to happen."* Recorded
here rather than left to a session narrative because it names a reason rig
exists, not a task to schedule - `BACKLOG.md` is where it becomes ordered
work once §37's ground work (the self-hosting cutover this document's own
"Status" line is mid-way through) is far enough along to take it.
**THE REASON IT IS WORTH DOING RATHER THAN JUST CONSOLIDATION, ALSO HIS OWN
WORDS:** *"Porting AgentBox visual elements into the rig GUI as planned will
be a good usability test."* §11's three pane tiers (Generated, Kit, Embedded)
have only ever been proven against programs built to fit them - the fake
applications of §23. AgentBox is real, daily-driven and already has its own
opinionated UI it was never designed to give up, so fitting it into rig's
split is the first test of that split against a program that resists it.

**BORIS, 2026-09-16 LATE, THE REPORT HE IS OWED AND WHO OWES IT:** *"They
should let me know when we get to a perfectly good MVP that is perfectly ready
to replace the GUI of AgentBox; No rush, everything must be ready so that we
don't harm the `rig` development process."* Said in the prompt that launched
the cutover team, so it binds THIS team and not some later GUI seat. **It does
not schedule the port and it does not move it up** - the two rulings above
still hold, and "no rush" is his third statement of the same thing. What it
adds is an OWED REPORT: somebody must be watching this readiness and must say
so unprompted, rather than Boris having to ask. **The team-lead owns it, in
`READINESS.txt`, beside the "how much longer" line**, because that file is
already the only place allowed to answer a readiness question.
**MVP-ready is not a feeling, and at a minimum it means:** the tray carries all
four dimensions §11 requires at 22px (only the mark and the estate are wired
today), the window survives being closed by the user (the SIGABRT found
2026-09-16 is open), and AgentBox's own live surfaces - the agents board, the
cards, the HANDS OFF strip - each have a rig pane tier that actually fits them.
**Short of that the line reads NOT READY with the gap named**, which is the
report he asked for just as much as the yes is.

---
