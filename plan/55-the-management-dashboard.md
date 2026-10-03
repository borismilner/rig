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

**Status: the mockup is built to requirement 44, and he has asked for
implementation to start (2026-10-03).** The build order is below.

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
| **10** | **Every external program can register a GUI in rig, to interact with it both ways** |
| **11** | **Each registered GUI is a separate tab on the dashboard** |
| **12** | **A GUI's content is wholly the program's, but it looks and feels like one rig application: its CSS is chosen from rig's when the program is written** |
| **13** | **A program's GUI uses rig's capabilities as much as possible to interact with its program, instead of rig being just a container** |
| **14** | **Picking a program opens its tab** (his "I think": he sees it in the mockup next session before it is final) |
| **15** | **Registering a GUI adds its tab, but Main stays the tab shown by default** |
| **16** | **Main needs as little scrolling as possible**; his suggestion is tabs inside Main that group what it offers (his "perhaps": he sees it in the mockup before it is final) |
| **17** | **The pointer over a tab is a hand, never a text cursor** |
| **18** | **The wire is searchable** |
| **19** | **A settings panel holds every setting rig exposes** (§6, §47: every key is schema-declared, so the settings UI is generated) |
| **20** | **Every program exposes its settings to rig, and its GUI shows them to review and to change** (§47 requirement 1: the two program layers, wired but empty until now) |
| **21** | **In a program's GUI, the wire shows only what concerns that program** |
| **22** | **Panels opening from the bottom and the side open and close with a smooth animation** |
| **23** | **Anything that needs his action, reply or decision comes on top**, close to the eye |
| **24** | **A dedicated panel gathers everything that needs his action, decision or reply**; it can be a tab inside Main, beside Programs and Board |
| **25** | **While such items exist, rig's tray icon shows it, and clicking the icon then opens that panel** |
| **26** | **Clicking a notification opens a panel with its full details and every action it offers.** Undecided, the options are clickable; decided, it shows what was chosen |
| **27** | **The board has a tab with every source, as now, plus a tab per source**, to follow one (on his reading that the board carries programs' and agents' progress, which is right: §55 req 3) |
| **28** | **A panel holds every capability of rig and of each registered program**, each with its description taken from the binary and controls to try it: *"kind of Swagger UI but for our needs"* |
| **29** | **rig publishes full guidelines for programs and agents: what a program must be built to and support to be rig compatible. They are time-stamped**, so a program built before a change can be found and adjusted |
| **30** | **"Needs you" is right as built** (his ruling on the mockup: *"spot on"*) |
| **31** | **A notification opens from a click anywhere on it, and is highlighted under the pointer**, so he knows which one will open |
| **32** | **A notification's full details open as a pop-up card in the exact centre of the screen**, fully interactive, then closed; not a side panel |
| **33** | **A setting's input fits what it holds: a file or a folder is chosen through the operating system's own picker**, not typed as free text |
| **34** | **Program settings are in tabs, one per program**, not all on one page |
| **35** | **A list he scrolls never jumps back to the top** (it did, in Capabilities, on every redraw) |
| **36** | **The settings icon looks like settings**, not a sun |
| **37** | **On Main, the content scrolls and the page does not**: the dashboard's details at the top and the notifications on the right stay in view while a long board scrolls |
| **38** | **Layout is very important: it must be slick and good looking.** A broken layout (the Board's header after the 31 to 37 build) is a defect, not polish |
| **39** | **Changing a value never moves the layout** (the settings row jumped when Reset appeared) |
| **40** | **Controls are good looking, useful and convenient**, beyond a bare browser input |
| **41** | **Every timestamp is 24-hour, never AM/PM**, wherever the dashboard or a program's GUI shows a time |
| **42** | **Clicking a program's row on Main opens a centred card** (as a notification's does, 32) holding everything about that program |
| **43** | **A program's GUI and its information are separate.** Picked from the side rail, a program shows its GUI; picked from Main, its information and settings. This replaces 14's "picking a program opens its tab" for Main |
| **44** | **Every program GUI carries a button that opens its information and settings**, as a layer over the GUI, never as part of it, so they do not interfere |

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

Requirements 10 to 12, the same day, verbatim: *"Every external program
can register a GUI in `rig` to interact with it, both ways. Each GUI is a
separate tab on the management dashboard. The content of each GUI is
completely up to the external program but we must make the visual parts
feel as one `rig` application so the CSS should be chosen from `rig` when
the external program is written."*

**Open, for him:** requirement 4 opens a program's tab only on its
request; requirement 10 has a program register a GUI. Whether
registering opens the tab, or only lets it be opened, is not yet ruled.
How the GUI and rig talk both ways (the board's `rig.panel.put` and
`panel.acted`, or a channel of its own) is also open.

Requirement 13, the same day, verbatim: *"The GUI of each program should
use `rig` capabilities as much as possible to interact with the program
instead of just being a container."* **This overrides the private
channel below** (`gui.send` and `gui.push`): a GUI acts on its program
through what rig already carries, such as the program's declared
commands (invoke), the store, the bus, notifications, toasts, progress and
the queue. Which capability serves which interaction is the next design
step, and it goes to him in the mockup.

**The lead's proposed mapping for requirement 13** (put to him 2026-10-03;
he answered "make the mockup as up to date as possible", so the mockup is
being rebuilt on it, not yet ruled):

| The GUI's interaction | rig capability |
|---|---|
| Act on an entry (match, reject) | `invoke ledger <command>`: a command the program declares, checked by rig against its declaration |
| Read what to show | the store, the program's own collection |
| Hear a change, live | a bus event the program publishes (`ledger.changed`) |
| Start long work (reconcile again) | the queue, with rig's progress |
| Tell the user it finished or failed | `notify`, which lands in the notifications panel |
| Confirm before acting | a rig toast with buttons |

The GUI's bridge exposes these, and only these: `rig.invoke`,
`rig.store.get`, `rig.events.on`, `rig.queue.push`, `rig.toast`. Nothing
is carried that rig does not already carry.

Requirements 14 and 15, the same day, verbatim: *"Picking a program should
open its tab I think - let me see it the next session. Registering a GUI
adds its tab but the main tab is the default to be shown."* This answers
the rail-and-tabs question and the register-opens-the-tab question; the
mockup's "available under Tabs, not open" is superseded by "the tab is
added, Main stays in front".

Requirement 16, the same session, verbatim: *"It would be great if the
main panel will need as little scrolling as possible. Perhaps it should
have tabs inside of it to group the different functionalities offered in
the main panel."* The goal (little scrolling) is ruled; the means (tabs
inside Main) is his suggestion, to be shown in the mockup.

Requirements 17 to 29, the same session (2026-10-03), verbatim, his
spelling kept: *"The mouse cursor over the tabs seems like a text edit -
it shouldn't. The wire details is good, I'd like it to be searchable. I'd
like there to be a settins panel containing all the settings `rig`
exposes. Also, every program should expose its settings to `rig` and the
GUI of each program should also have these settings visually available to
review and to change. For every program, when in its GUI, we should be
able to see the wire only that is relevant to that program. I like the
usage of panels than open from the bottom and from the side; Maybe they
can be opened and closed with a smooth animation. Items that require my
action or reply or decision and so on must come on top so that they are
close to the eye. There should be a dedicated panel than concentrates
everything that needs my action/decision/reply/... When such items exist
rig the system-tray icon can have an indication and when I click on it in
such a mode it should take me to this panel; This panel can be in a tab on
the main tab same as programs and board. The notirifcations panel is good,
clicking a notification should open a panel with the full details and all
the cations and options applicable to the notification if not yet decided
then the options should be clickable and if already clicked I should see
what has been decided/clicked. If I understand correctly, the board shows
the progress notifications of the different programs/agents; if so, it
should have a tab displaying all as now and then a tab per source this way
I can follow the progress of a specific source of interest. I want a
dedicated panel containing all the capabilities of `rig` including all of
its programs that are registered. Each capability should have a
description taken from the binary and convenient visual controls to play
with these capabilities, it kind of reminds the Swagger UI but for our
needs. `rig` should expose full guidelines for programs and agents so that
it is absolutely clear how the programs must be built and what they must
support in order to be `rig` comapatible; these instructions should be
time-stamped so that we can know when a program may need to be
adjusted."*

Requirements 30 to 37, the same session, verbatim, his spelling kept:
*"The "Needs you" tab is spot on. Clicking notification should work on
any place of the notification, not just the title, so that it is
comfortable. And when hovering over a notification it should be
highlighted so I know which one is going to open. I think the opened panel
with the full details should be a popup card on the absolute center of the
screen that I can interact with in all the relevant ways and then close it
(and not from the side as it is today). In the settings, the relevant
inputs should be more helpful, for example when needs to select a file or
a folder then I should be able to click and then an OS allows me to do it
visually instead of having free-text. Under the settings, the
program-specific settings should be in tabs one for each program and not
all in one. In capabilities, when I scroll the different capabilities it
keeps jumping back up which is annoying. The icon for settings should be
more appropriate, not a sun like it's now, maybe for the settings too. In
the main panel/tab what scrolled should be the content and not the whole
page, so for example now that the board is long, scrolling it should not
get the notifications on the right out of view and same about the details
on the top of the dashboard, they should not be scrolled out of the
view."*

Requirements 38 to 40, the same session, verbatim, on two screenshots
(the Board's header, a settings row): *"In general everything looks like a
good start. I showed you in the screenshot a very bad layout; Layout is
very important; it must be slick and good looking. In settings like in
[the screenshot] when changing a value the whole layout jumps because
"reset" is added to it - and it's a bad user experience. The controls
could be much more better looking and useful and more convenient for the
user. Other than that it seems like a good start."*

Requirement 41, the same session, verbatim: *"Timestamps must be 24h and
not AM/PM."*

Requirements 42 to 44, the same session, verbatim: *"On the main panel,
when clicking on a program row, a similar card in the absolute center of
the screen should be opened with all the relevant information of the
program. We should separate the program GUI and it's settings and all the
other information; when choosing a program from the side-panel we should
get its GUI and when we choose it from the main panel we should aceess all
the information and the settings. In each program GUI we should have a
button that opens these settings and information for that specific
program but the information should not part of the main gui to not
interfere."*

**Requirement 33 binds the config schema too:** a key must say what it
holds (a folder, a list of folders, a file) for the window to choose the
control, so §47's schema gains that annotation. The window is a native
one, so the picker can return a real path, which a browser cannot.

**Reach beyond the dashboard.** Requirements 20, 25 and 29 bind rig itself,
not only its window: 20 fills §47's program layers, 25 is the tray (§12's
icon), 29 is a published contract beside §19's conformance suite. Each
points back here.

He also said: *"I'm not sure I understand all these questions - you'll
have to be more specific during our next session."* **So an open question
goes to him concrete**: shown in the mockup, one at a time, with the
choice drawn, never as a list of abstract readings.

**What the mockup shows for 14 to 16 (2026-10-03, the lead's build, for
him to see):** registering adds the program's tab and Main stays in
front. A program is picked from the rail, which lists registered GUIs
(§11 requirement 16), or from Main's program list; either opens its tab.
A program with no GUI gets rig's own page about it; a GUI tab keeps rig's
facts behind a "What rig knows" button. Main has two inner tabs, Programs
and Board, and its figures jump to the one that explains them. Measured
at 1440x900: Main scrolled 466px before, now 0px on Programs and 131px on
Board. The superseded private channel (`gui.send`, `gui.push`) is gone
from the mockup; requirement 13's mapping above is what it uses.

**What the mockup shows for 17 to 29 (2026-10-03, the lead's build, for
him to see; the choices below are the lead's, not his):**

| Req | In the mockup |
|---|---|
| 17 | every clickable thing shows a hand |
| 18, 21 | the wire has a search box; on a program's tab it shows only that program's lines, with a chip back to every program |
| 19, 20 | a Settings tab from the rail: rig's 12 real keys read from `internal/config/schema.json`, plus each program's declared ones. A GUI tab has a Settings button with the same rows. A bad value is refused in place |
| 22 | the side drawer and the wire slide in and out |
| 23, 24 | Main's inner tabs are **Needs you**, Programs, Board, and Main opens on Needs you. What asks sits on top of the notifications and of the board |
| 25 | a simulated tray button carries the count; clicking it opens Needs you |
| 26 | a notification opens a drawer: details, facts, options. Decided, the options are disabled and the choice is named. A notification linked to a card is one decision |
| 27 | the board has Every source plus a tab per source |
| 28 | a Capabilities tab: rig's 93 commands with their own words and argument schemas from `selfDeclaration()` (dumped by `design/dashboard/dump-self.sh`), every program's commands, a generated form, the call as it goes on the wire, and a confirm before anything that writes |
| 29 | a Guidelines tab: eight dated rules (dates from git where built, the day he ruled it where not), each program's built-to revision, and which rules are newer. Main's program list shows "current" or "N newer" |

**What the mockup shows for 31 to 37 (2026-10-03, the lead's build):**
a notification is one click target with a hover tint; its details are a
modal `<dialog>` centred on screen (Esc, Close, or a click outside); a
folder or file setting has a picker button (a browser returns only the
name, the native window will return the path) with "or type it" kept as
a fallback; byte sizes show in MiB; Settings has a tab for rig and one per
program; every redraw restores each list's scroll; Main's head, figures,
inner tabs and the notifications panel stay fixed while the content
scrolls, and the board's filters stay above its cards.

**What the mockup shows for 38 to 44 (2026-10-03, the lead's build):**

- **Layout (38).**
  - The tab strip scrolls sideways, fades at a hidden edge, and keeps
    Main pinned.
  - The mock's own tools shrink to icons on a narrow screen and never
    wrap.
  - The Board drops its heading for two rows: the sources, then search,
    severity, status, grouping and time.
- **No shift (39).**
  - A setting is a row in a fixed grid: key, control, origin badge, Reset.
  - Reset keeps its slot and is only hidden while the value is the
    default.
  - A refusal shows as a pop-over that takes no space.
  - Measured: no box in the row or the row below moves when a value
    changes.
- **Controls (40).**
  - A whole number gets a stepper with its unit inside; arrow keys step.
  - A byte size is read in the largest whole unit (2 MiB) and changed in
    that unit.
  - Booleans get a switch, an enum of four or fewer a segmented control,
    and larger enums a styled select.
- **Times (41):** every time is 24-hour; a full one reads
  `2026-10-03 16:32:28`.
- **The program card (42 to 44).**
  - A row on Main's Programs opens a centred card with Overview,
    Settings, Commands (each with Try it) and Activity.
  - The card keeps one size whichever tab is showing.
  - The rail opens a program's GUI. Its strip has one "Info and settings"
    button, which opens the same card over the GUI.
  - From Main, a program with a GUI offers "Open its GUI".
  - The settings drawer and the program page reached from Main are gone.
    A program's own tab now appears only when it asks for one.

### Build order (the lead's proposal, 2026-10-03; his ruling is only "start implementation")

Boris, 2026-10-03, after trying the mockup through requirement 44:
*"Write the handoff and start implementation."* The mockup in
`design/dashboard/` is the reference for every slice. Each slice is
exercised in the real window before the next starts.

| Slice | What | Needs a new verb? |
|---|---|---|
| **1** | **The window's shell to the mockup.** Main with its head, figures and inner tabs; Programs from `Programs` and `Health`; the program card (42); the rail gives a GUI and the card opens over it (43, 44); 24-hour times (41); the control kit (40) and the fixed settings rows (39) in `Settings.svelte` | no |
| **2** | **Notifications and Needs you** (7 to 9, 23 to 26): the panel reads the notifications rig already files in the record; deciding answers through `toast.answer`; eviction by age; the tray's count | maybe a record query by kind and age |
| **3** | **The board in rig** (2, 3, 27): `panel.put` writes, the store keeps, the bus wakes; beacon moves onto it | yes |
| **4** | **Capabilities** (28): every verb from `describe`, with Try it | no |
| **5** | **Program settings** (20, 34): §47's program layers, and the schema's control annotation (33) | yes |
| **6** | **Registered GUIs** (10 to 13, 15, 21): `gui.register`, rig.css, the per-program wire | yes |
| **7** | **Guidelines** (29): published, dated, and checked against each program. Ruled 2026-10-03 (decision 0266): one dated file in the repo, embedded in rigd and served by a read-only `rig.guidelines`; a program's age is the commit date in its binary's Go build info, "unknown" without one | yes, `rig.guidelines` |

### Slice 1 as built (2026-10-03)

| Part | In the real window |
|---|---|
| Main | head, a skew line when window and daemon differ, five figures that open their tab, inner tabs Needs you / Programs / Board; only the content under the tabs scrolls (37) |
| Programs | a table from `Programs` and `Supervision`; the row or the name opens the card (42) |
| Card | a centred, fixed-size `<dialog>` with Overview, Settings, Commands, Activity; Start, Stop and Restart call `rig.up`, `rig.stop`, `rig.restart` through `RigService.Supervise` |
| GUI | the rail gives the GUI; the context bar's "Info and settings" opens the card over it (43, 44) |
| Times | `lib/estate.ts` `stamp()`, 24-hour and ISO-ordered (41) |

**Moved, not dropped: the control kit (40) and the fixed settings rows (39)
go to slice 5.** `Settings.svelte` is the theme panel, with one mode choice
and two sliders; rig's and the programs' settings do not reach the window
until §47's verbs do, and a kit with nothing to edit cannot be exercised.

**Honest gaps, said on screen:** the card's Settings tab waits for slice 5,
and its Activity tab and Main's Board wait for slice 3. (Needs you listed
only parked programs until slice 2.)

**Start, Stop and Restart were exercised against a scratch `rigd`**
(greeter and lantern, 2026-10-03): stop answers with no state, start and
restart answer `starting`, an unknown program is refused with rig's own
NOT_FOUND. The empty state after a stop is drawn as "stopped" with a Start
button (af18fc4). **Not exercised:** the same calls through the Wails
bridge in the running window, which needs a window on a live desktop.

### Slice 2 as built (2026-10-03)

| Part | As built |
|---|---|
| Source | `RigService.Notifications`: rig's notification and reply records joined, plus one zero-timeout `rig.toast.answer` per unanswered ask to learn whether anyone still waits. No new verb |
| Panel | Main's right column: what waits on top, then newest first; search; a From filter with a count per source; the whole row opens the card and is tinted on hover (7, 9, 23, 31) |
| Eviction | `dashboard.notifications.keep.days`, default 7, live; the footer names the key, its layer and how many are older. The record keeps them (8) |
| Card | a centred `<dialog>`: facts, body, the options while undecided; decided, the options stay disabled with the choice marked; "Only X's notifications" sets the filter (26, 32) |
| Answer | `RigService.Answer` calls `rig.toast.reply`, the bubble's own call, so the asker is told whichever surface answered |
| Live | the window watches `rig.toast.wait` and every waiting ask, and re-reads on `rig:notifications`; nothing polls |
| Needs you | every waiting question with its options inline, then parked programs; the figure and the tab count are the same total (24) |
| Tray | while anything waits: an amber badge top-right on the estate's glyph (the down badge is red, bottom-right), a "Needs you (N)" row, and the click opens Needs you instead of toggling (25) |

**Decided by the lead, to confirm with him:**
- Main opens on Needs you when the window's FIRST read finds a question
  waiting, else on Programs. Later arrivals never move the page.
- While something waits, the tray click never closes the window. An open
  window is told "needs" on its stdin and comes forward. "Close the
  window" in the menu still closes it.

**Exercised:** against a scratch `rigd`, the real window on a private X
display: it opened on Needs you; a `needs` line on its stdin brought it
back from Programs; a `rig notify` filed meanwhile appeared without a
reload; Deploy clicked in the window reached the waiting `rig notify`
(`reply: Deploy`). Over the Go service: option and free-text answers, a
second answer refused CONFLICT, an unknown id refused. Headless on
`?notes=1`: 20 checks, 8 runs clean. Contrast: `?notes=1` clean, now in
`make contrast-window` with `?caps=1` and `?guide=1`.

**The tray, on his panel after the deploy (v0.0.0-m0-1014-g650dde9),**
read over D-Bus: a filed question badged the icon and showed "Needs you
(1)" within 3 s; a left-click (StatusNotifierItem.Activate) opened the
window, a second left it open; the answer cleared the badge and hid the
row, and the asker was told. **Not exercised:** which tab the opened
window showed, read by eye. The card pages (`&note=`) cannot pass the focus pass:
behind a modal the page is inert, the same as `?card=`.

### Slice 3 as built (2026-10-03)

| Part | As built |
|---|---|
| Verbs | `rig.panel.put`, `rig.panel.list`, `rig.panel.act`; MCP tools from the same table; `rig panel put / list / act` at the prompt |
| Store | one record per card, project `board`, kind `card`; every put is a version, a close is the last one, never a delete. A press is filed as kind `card-act` |
| Owner | the connection's program id, else its seat, never the request's. Only the owner changes a card; a closed card refuses changes (CONFLICT) |
| Caps | beacon's: title 1 to 200 bytes, status 80, body 4096, 6 facts, 3 distinct actions, progress 0 to 1, four severities. Past 40 open cards a sender's oldest is closed |
| Bus | `panel.changed` to everyone on every version; `panel.acted` addressed to the owner only. `panel` is now one of rig's event roots |
| List | open cards and those closed in the last 24 h, newest change first, at most 200 with a count of the rest |
| Window | Main's Board tab: Every source, then a tab per source with its open count (27); search over titles, bodies and facts; severity chips; both rows stay above the cards (37); a section per source striped by its worst open card; closed cards folded per section; buttons call `rig.panel.act` |
| Live | the window waits on `panel.changed` and re-reads; nothing polls. A rig without the board says so on the tab |

**Decided by the lead, to confirm with him:**
- A card with buttons is NOT counted in Needs you or on the tray badge
  yet: a "Stop" on a running job does not need him. The mock counted
  only waiting or failed cards; the board's status is free text, so the
  rule needs his word.
- Grouping by project or severity, the time filter, minimise, dismiss
  and a card's history from the mock are not built in this slice.

**Exercised:** against a scratch `rigd`, from the CLI: put, a change,
a press, a refused action, a close, a refused change after it; a
waiter saw three `panel.changed` and the `panel.acted`. The real window
on a private X display: the tab counted the card from the live rigd;
a change and a new card appeared without a reload; Deploy clicked in
the window reached a `rig events wait panel.acted` and was filed.
Headless on `?board=1`: 10 checks, both themes. Contrast: clean, lowest
text 5.63; now in `make contrast-window`. **Not exercised:** a program
(not a terminal) owning a card.
**beacon's board removed (rigged a17dbe2), so requirement 3 is met.**
Its `board`, `board-close` and `board-history` commands, its column and
its store collection are gone; `rig describe beacon` lists eight card
commands and a call to `board` is refused by name. Exercised on a
scratch rigd: an ask card drew, a click answered it, beacon exited.
**Known limit:** each list reads every card head ever put; the closed
ones are filtered in memory.

### Slice 4 as built (2026-10-03)

| Part | In the real window |
|---|---|
| Source | rig's verbs from the running daemon's MCP `tools/list`, not `describe` as the table above says: `describe` reaches programs only, and no wire verb lists rig's own. Program commands from `Programs` |
| List | search, grouped by owner, an effects dot per entry; arrow keys move between entries |
| Detail | the verb's own words, its returns, a form from its argument schema, and "What goes on the wire" showing exactly what Try sends |
| Try | `RigService.Try`; a program's command goes through `invoke`. Anything not read-only is confirmed first; a refusal shows in rig's own words |

**The window grew 1.82 MB for the MCP client** (15.22 to 17.04 MB).
Boris accepted it on 2026-10-03 over a hand-written JSON-RPC client
(ac37379).

**Exercised against a scratch `rigd`:** a read (`health`), a program
command (`greeter greet`), a refused write, bad arguments refused by rig's
schema check, and an unknown tool. Against production, read-only: 91 verbs
and 8 program commands listed, 29 verbs with no declared effects (drawn as "effects
not declared" and confirmed like a write). **Not exercised:** the Wails
bridge in the running window, and a successful write (an unnamed estate
refuses them).

### Slice 7 as built (2026-10-03)

| Part | As built |
|---|---|
| Rules | `internal/guidelines/guidelines.json`, eight rules, each date checked against git (the mockup had A2 a day late). Served by `rig.guidelines` and the MCP `guidelines` tool, read-only |
| Build day | rigd reads `vcs.time` and `vcs.modified` from each program binary's Go build info; never runs it. No build info, or no binary rig knows, reads unknown with the reason |
| Main | the Programs table's Guidelines column says current, N newer or unknown; the word opens the Guidelines GUI with that program's rules marked |
| GUI | programs against the guidelines, then the rules newest first, each naming the programs built before it |

**What counts:** a rule for programs, built, and dated after the build day.
A rule dated the build day is not counted (a day cannot say which came
first), and a rule only ruled cannot be met yet.

**rigd grew 610 KB** (3.1%) for `debug/buildinfo`'s object-file readers,
recorded in 8272979.

**Exercised:** against a scratch `rigd`, over MCP and over the control
socket the window uses: eight rules, and greeter's and lantern's commit
times match `go version -m`. Against production (read-only), the old daemon
answers NOT_FOUND, which the window words as "older than rig.guidelines".
**Not exercised:** the Wails bridge in the running window, and a program
with no build info on a live daemon (unit-tested only). **Not built:** the
card's Overview does not show the standing, which the mockup does.

### ⛔ Two tensions with §11, for Boris, not for a seat

- **§11 requirement 20** says the agents' GUI *"must not push, badge,
  notify or occupy the dashboard"*. That ruling is about the stream of
  **all** agent interactions (requirement 21). The board holds cards that
  agents write to him on purpose, and he has now put it on the dashboard.
  Read as two different things, both hold. That reading is the lead's,
  and it is to be confirmed.
- ~~**§11's window is "dashboard or program"**~~: answered by requirement
  14, picking a program in the rail opens its tab, and by 43: the rail
  gives its GUI, Main gives its card.

### For the mockup (beacon seat's notes, a seat's, not his)

- Tokens from `design/theme.js` (`tokens(DEFAULTS, 'dark')`, which passes
  `checkTheme`). **No lilac or pink**: he rejected pink (rigged SPEC "No
  pink"). Severity: info steel, success sage, warning amber, error rust.
- Keep what `Dashboard.svelte` shows today: the header and state, the
  figures strip, the estate list, the supervised, healthy and asking facts.
- His standing rules for artefacts: measure contrast, keep memory capped,
  exercise it in a headless browser on a port other than 9222, and never
  capture his screen.
