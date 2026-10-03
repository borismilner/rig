## 55. The management dashboard

**Requirement, Boris, 2026-10-03, verbatim** (said in the beacon seat's
session and traced to its transcript, 07:29 to 07:45 UTC):

> So `rig` as we said, needs to have a management panel that programs can
> push items and information into it in real time. Lets plan this panel. I
> think that the proper way of publishing things to `rig` in this manner
> would be through the bus but I'm open to suggestions.

> Do as recommended and yes, the board is in `rig` - it is part of the
> management dashboard that will also include a place for this board.

> Lets plan this dashboard visually. Give it your best shot and let me test
> it in my browser.

> Wait, no - let the other worker that works on `rig` to do it

> In general I'd like it to have one main tab and tabs per specific
> programs. The specific tabs are opened on program/agent request so if
> there are no such requests there should be no such tabs open but the user
> can choose to open previously opened tabs for specific programs/agents to
> see their last state.

This is the content §11 left deferred for the dashboard (requirement 6,
*"all the most important information we'll define in the future"*). The
rig lead plans it with Boris directly.

Boris, 2026-10-03, to the rig lead, verbatim: *"We should plan the
management dashboard visually before we do much work on it - maybe with
stubs and mocks."* **So no dashboard code is built before he has tried a
mockup**, driven by simulated programs and agents.

**Status: requirements recorded; the design is not yet put to him.** The
next step is the visual mockup he asked for, which he tests in his own
browser.

### ⛔ What he ruled

| # | Requirement |
|---|---|
| **1** | **Programs and agents push items into the dashboard in real time.** It opens from the tray, through rig's window |
| **2** | **A verb writes, the store keeps, the bus wakes** ("do as recommended"; the recommendation is below) |
| **3** | **The board of agents' cards moves into rig**, with its own place on the dashboard. beacon keeps its board until rig's is live, then removes it |
| **4** | **One main tab, plus a tab per program or agent.** A specific tab opens only when a program or agent asks for it, so with no requests there are none. The user can reopen a previously opened one to see its last state |
| **5** | **He plans it visually**: a mockup he tests in his own browser |
| **6** | **The Main tab, the default, shows the most important information about rig and about every program it represents**, the scanned ones included: each one's **version**, and what matters about it, there for the user to inspect |
| **7** | **A dedicated vertical panel on the right holds the latest notifications, newest on top**, so one he missed is still there |
| **8** | **A notification older than a configurable age is evicted from that panel. The default is one week** |
| **9** | **The notifications panel is searchable, holds notifications from every program, and filters by source** to show only one program's |

Requirements 6 to 8, Boris to the rig lead, 2026-10-03, verbatim: *"The
main tab which is the default should show all the most important
information about `rig` and about all the programs that it represents (the
ones it scanned for). It should know their versions too and it should have
all the most important information about them to be inspected by the user.
It should have a dedicated vertical panel on the right side with the latest
notifications sorted with the latest ones on top; in case the user missed
them. Notifications that are older than a configurable amount of time, for
now lets make it a week, are evicted from this panel."*

Requirement 9, the same day, verbatim: *"The panel should be searchable to
filter for relevant information. It should contain notifications from all
the programs and we can filter by program to see only notifications
relevant by source."*

### The write path he approved (requirement 2), as the beacon seat put it to him

- `rig.panel.put {card?, fields}` adds a card, or changes the one named.
  `rig.panel.close` closes a section.
- **The writer's identity comes from its connection**, a program id or a
  seat, never from the request. Seats write too, so §52 B2 (only programs
  publish) does not apply.
- rigd checks the fields and applies the caps; a refused edit changes
  nothing. **Every version, whole, goes to the store.** Only open cards are
  held in memory, capped.
- rigd publishes `panel.changed` (rig's own kind) and the window waits on
  it. A click on a card goes back to its owner as `panel.acted`.
- Later slice: a program's declared events shown on the dashboard with no
  code.
- **Bus-only was turned down**: §52 E4 (an event is a fact, not a
  command), E7/E8 (in memory and at most once, so no durable history), B2,
  and E3 (no single checked card shape).

### The board (requirement 3), carried over from rigged/SPEC.md "A board"

- A section per agent, newest card on top. The agent closes a section; the
  user dismisses one.
- Card: title, status, severity (info, success, warning, error), body,
  progress, busy, facts. Created, changed and closed times, each version
  whole.
- The board minimises and restores.
- Uses: search past work, act on a card, filter and group, summaries.
- The model to lift from: rigged f7bbee9, `beacon/board.go` (caps, atomic
  apply, ids unique across runs), `history.go`, `board_test.go`,
  `ui/board.js`.

### ⛔ Two tensions with §11, for Boris, not for a seat

- **§11 requirement 20** says the agents' GUI *"must not push, badge,
  notify or occupy the dashboard"*. That ruling is about the stream of
  **all** agent interactions (requirement 21). The board holds cards that
  agents write to him on purpose, and he has now put it on the dashboard.
  Read as two different things, both hold. That reading is the lead's,
  and it is to be confirmed.
- **§11's window is "dashboard or program"**, with programs in the rail.
  Requirement 4's tabs are opened by a program's request, not by picking it
  in the rail. How the rail and the tabs relate is open.

### For the mockup (beacon seat's notes, a seat's, not his)

- Tokens from `design/theme.js` (`tokens(DEFAULTS, 'dark')`, which passes
  `checkTheme`). **No lilac or pink**: he rejected pink (rigged SPEC "No
  pink"). Severity: info steel, success sage, warning amber, error rust.
- Keep what `Dashboard.svelte` shows today: the header and state, the
  figures strip, the estate list, the supervised, healthy and asking facts.
- His standing rules for artefacts: measure contrast, keep memory capped,
  exercise it in a headless browser on a port other than 9222, and never
  capture his screen.
