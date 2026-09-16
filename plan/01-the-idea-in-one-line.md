## 1. The idea, in one line

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
