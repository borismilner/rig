## 11. The window

- ⛔ **BORIS, 2026-09-17, verbatim - THE WINDOW IS TAB-BASED, AND THE FIRST TAB
  IS PROJECT/CASE MANAGEMENT. RECORDED THE TURN HE SAID IT:** *"alongside working
  on `rig` lets add tab-based content to the GUI for `rig` itself. How about we
  start with project/case management tab in which I'll be able to see
  project/cases managed in `rig` with all the most important and relevant and
  useful details. Things should update in real-time as the details change,
  probably by utilizing the in-process bus or some other notification/live-update
  mechanism."*

  **THREE REQUIREMENTS, NUMBERED ON FROM THE TRAY'S FIVE.**

  | # | Requirement |
  |---|---|
  | **6** | **THE WINDOW'S CONTENT IS TAB-BASED.** Not one pane with a rail switching it - **tabs**, his word, and the shape he expects to add content into |
  | **7** | **THE FIRST TAB IS PROJECT/CASE MANAGEMENT** - the projects and cases rig itself manages, *"with all the most important and relevant and useful details"* |
  | ⛔ **8** | ⛔ **IT UPDATES IN REAL TIME AS THE DETAILS CHANGE.** Not on open, not on a refresh button. **This is the requirement that decides the architecture**, and it is the reason he named a mechanism at all |

  ⛔ **HIS NAMED MECHANISM IS NOT BUILT, AND SAYING SO IS THE POINT OF RECORDING
  IT HERE RATHER THAN ACTING ON IT.** *"the in-process bus"* is §5h, it is
  specified, and §26 puts it at **M13** - *"it lands at M13 with the bus
  (§5h)"*. §13 already writes capability rules against it (*"The bus refuses a
  subscription without a grant"*) and §8 specifies a live tap on it. **None of
  that exists.** His own wording leaves the door open - *"or some other
  notification/live-update mechanism"* - **so this is a STEER, not a ruling, and
  building M13's bus to paint one tab would be the tail wagging the dog.**

  **WHAT ACTUALLY EXISTS TO CARRY REQUIREMENT 8 TODAY, so the choice is made on
  facts rather than on the word "bus":**

  | Option | What it costs | What it is |
  |---|---|---|
  | **the frontend polls through `RigService`** | nothing new | what the tray already does - `pollEstate` on a 5s tick. **Honest, ugly, and it is real-time only in the sense a clock is** |
  | **a Go-side watcher emitting a Wails event** | one goroutine | the window polls the daemon once and pushes to the webview. **Real-time to the viewer; still a poll at the seam** |
  | ⛔ **a subscription on the WIRE** | a new wire verb, §21 versioning | **the only one that is actually event-driven**, and the only one that makes the window a consumer of the same signal any other program would get. §5h's bus is this, generalised |

  ⛔ **THE TRAP TO NAME BEFORE ANYBODY BUILDS THIS: A POLL BEHIND AN EVENT API
  IS THE HARDEST KIND OF CLAIM TO FALSIFY LATER.** If the window emits
  `records-changed` from a 1s poll, every consumer written against it believes it
  is event-driven, and the day the bus lands nothing tells anyone which callers
  were only ever getting a tick. **If the seam is a poll, the API's own name and
  doc comment must say so**, exactly as §11 already requires of the tray's
  last-known icon versus its text.

  **AND A SCOPE NOTE HE HAS ALREADY RULED ONCE, so it is not re-opened here:**
  porting AgentBox's GUI into rig is *"one of the original reasons rig exists"*
  and is explicitly NOT priority until the groundwork is done (§01). **Tabs are
  that groundwork arriving**, which is why this is additive to the MVP rather
  than a new front - but the AgentBox panes are still not this tab.

  ⛔ **AMENDED BY BORIS THE SAME DAY, MINUTES LATER, AND IT CHANGES WHAT GETS
  BUILT. VERBATIM:** *"The GUI parts I've mentioned are not part of the MVP. We
  can have the data displayed without live updates until the mechanism is ready.
  I want to remind you that the GUI and the visuals must be top of the art, the
  best of the best the technology offers, best of CSS, best of JS or TS, best of
  HTML and best of all components, making use of the best visualization
  mechanisms."*

  | What it changes | |
  |---|---|
  | ⛔ **requirement 6 and 7 are OFF THE MVP PATH** | The MVP's three conditions are unchanged and tabs are not a fourth. **A seat told to converge on the MVP does not build this**, and a seat told to build this does not report it as MVP progress |
  | ⛔ **requirement 8 is DEFERRED, NOT DROPPED** | *"without live updates UNTIL the mechanism is ready"* - **the mechanism, not the tab, is what is waited on.** So the tab ships reading its data once, and §5h's bus at M13 is what turns it live |
  | ✅ **the three-option table above is SETTLED by this** | **Do not build the Go-side watcher and do not build a wire subscription for this tab.** Both were only ever there to fake requirement 8 early, and the poll-behind-an-event-API trap named above is now a trap nobody needs to walk into |
  | ⛔ **NEW REQUIREMENT 9: THE VISUAL BAR, AND IT IS HIS OWN WORDS** | *"top of the art, the best of the best the technology offers"* - CSS, JS/TS, HTML, components, **and specifically *"the best visualization mechanisms"***, which names charts and data display rather than chrome |

  ⛔ **THE STANDARD HAS A NAME AND AN ADDRESS, GIVEN 2026-09-17 AFTER HE SAW
  THE FIRST BUILD. BORIS, verbatim:** *"The GUI is quite dull, try to see our
  golden standard for visualization partly having to do with our library
  project and in general"*, and then, for the record: *"I want the GUIs to be
  beautiful with respect to our golden level of quality and beauty you can find
  in the library project and probably in other places too."*

  | | |
  |---|---|
  | **the reference** | **`~/me/library/index.html`** - the library project's page. **GO AND LOOK AT IT RENDERED** |
  | **the second reference** | **`design/visual-system.html`** (177 KB) and `design/theme.js` - **rig's OWN**, which this section already calls built and measured |
  | **"and probably in other places too"** | his words. The two above are the ones located; **he has not claimed they are the only ones** |

  ⛔ **AND THE FINDING THAT SAVES THE NEXT SEAT A WRONG TURN, MEASURED RATHER
  THAN ADMIRED.** `[ran it]` 2026-09-17 against `~/me/library/index.html`, 190,762
  bytes: **8 `box-shadow`, 1 `@keyframes`, 2 `transform`, and ZERO gradients of
  any kind** - no linear, no radial, no conic, no `backdrop-filter`, no
  `mix-blend-mode`, no `clip-path`. Two font families, both stack defaults: one
  serif, one sans.

  ⛔ **SO WHATEVER MAKES IT GOLDEN IS NOT EFFECTS, AND A SEAT THAT ANSWERS
  "DULL" WITH GRADIENTS AND GLOW HAS MISREAD THE STANDARD IT WAS POINTED AT.**
  What is left to carry it is typography, hierarchy, density, restraint and how
  a large set is made browsable - which is `browsable-page`'s subject and
  `readable-output`'s, not a decoration budget. **The likeliest diagnosis for a
  dull rig window is therefore not that it needs more; it is that it is not
  using the visual system this repository already built.**

  ⛔ **REQUIREMENT 9 IS NOT A STYLE PREFERENCE AND MUST NOT BE FILED AS ONE.**
  §23 M1a already says that *"for a program whose whole job is presenting other
  programs, the visual design IS the product"*, and `readable-output`'s standing
  rule binds it: **"hard to read" is a measurement, not taste.** So this
  requirement is DISCHARGED BY NUMBERS - measured contrast at real sizes on both
  themes, not by admiring a screenshot. **The reason it needs saying is that
  "make it beautiful" is the one requirement a seat will always believe it has
  met.**

  ⛔ **AND THE ORDER THAT FALLS OUT, because two of his instructions could look
  like they compete:** the MVP keeps its priority, *"strive toward the perfect
  MVP"* is still standing, and **this is the work that runs ALONGSIDE it** -
  *"alongside working on `rig`"* is his own framing in the sentence that opened
  this requirement. It is not a front that pauses the MVP and it is not filler.

- ⛔ **BORIS, 2026-09-17, verbatim - THE WHOLE INFORMATION ARCHITECTURE, AND IT
  RELOCATES THE TABS. RECORDED THE TURN HE SAID IT:** *"I think the main GUI
  show show a general-purpose dashboard with all the most important information
  we'll define in the future. Choosing a registered program we'll see the GUI of
  that program but we can always go back to the main dashboard from all places
  conveniently. For the `rig` program we already have there, its GUI contains
  different tabs for each interesting functionality that I can observe and
  interact with using the GUI. `rig` settings is a separate GUI and can be
  accessed both from a dedicated GUI button and from the system-tray."*

  ⛔ **THIS SUPERSEDES REQUIREMENT 6 AS IT WAS WRITTEN ABOVE.** Requirement 6
  said *"the window's content is tab-based"*. **It is not - the tabs are one
  level DOWN.** The window is dashboard-or-program; **rig is one of the
  programs**, and the tabs are inside rig's own program GUI. A seat that built
  a tab bar across the top of the shell would have built the wrong thing, and
  that is what the previous wording asked for.

  | # | Requirement |
  |---|---|
  | ⛔ **6 (REPLACED)** | **THE MAIN GUI IS A GENERAL-PURPOSE DASHBOARD.** It is a destination in its own right, not the empty state before a program is picked. **Its CONTENT is deliberately unspecified** - *"all the most important information we'll define in the future"* - so it is a named surface with a deferred payload, and filling it is not a seat's call |
  | **10** | **CHOOSING A REGISTERED PROGRAM SHOWS THAT PROGRAM'S GUI.** Already the shape the rail and the pane implement (§11's three tiers), now stated as the top-level navigation rather than as pane behaviour |
  | ⛔ **11** | ⛔ **BACK TO THE DASHBOARD FROM EVERYWHERE, CONVENIENTLY.** *"from all places"* is the operative phrase: it is a global affordance, not a control that lives on one screen. **A program's own pane draws its own chrome, so the way home must not depend on the program cooperating** - which is the constraint that decides where it can live |
  | ⛔ **12** | ⛔ **rig IS A PROGRAM IN ITS OWN SHELL, AND ITS GUI IS TABBED** - *"different tabs for each interesting functionality"*. The project/case tab (requirement 7) is the FIRST of those tabs, not the whole of rig's GUI |
  | ⛔ **13** | ⛔ **SETTINGS IS A SEPARATE GUI WITH TWO DOORS** - a dedicated button in the GUI, **and** the system tray. Not a tab, not a pane |

  ⛔ **REQUIREMENT 12 HAS A CONSEQUENCE NOBODY HAS PRICED, AND IT IS THE
  INTERESTING ONE.** §5's promotion ruling established that **rig is not in
  `CapabilityMap`, has no connection to itself, and `meta` refuses `"rig"` as an
  invoke target** - four independent closures, recorded under *"PROMOTION IS
  DEAD"*. So *"the `rig` program we already have there"* describes something
  that, on the registry's own terms, **is not a registered program at all.**
  Either the rail carries a rig entry the registry does not serve, or rig
  registers with itself and reopens a namespace reservation refused twice.
  **This is a real design question and it is not a seat's to settle** - but it
  must not be discovered halfway through the build.

  ✅ **REQUIREMENT 13 ANSWERS AN OPEN ITEM THIS SECTION ALREADY CARRIED.** M1a
  step 5 shipped settings as keyboard-only - opened with `,`, closed with Esc -
  and the recorded reason was *"the panel is reachable without a pointer, which
  is the only way it is reachable at all while the context bar has no room for
  another control"*. **He has now ruled that it gets a control.** So the room
  has to be found; the keyboard path stays, because it was never the problem.

  **WHAT IS UNCHANGED BY ALL OF THIS, so nobody re-reads it as a reversal:**
  requirement 8 is still deferred (no live updates until §5h's bus), requirement
  9's visual bar still binds, and **the whole of this is still OFF THE MVP
  PATH** by his ruling minutes earlier.

  ⛔ **AND THEN HE GAVE IT ITS PURPOSE, WHICH IS THE THING A SPECIFICATION
  USUALLY LACKS. BORIS, 2026-09-17, verbatim:** *"The GUI is to be built
  alongside working on `rig`. It will help me get a grip on the usability of
  [rig] and get a feel of the product while it's being built. At the very least
  it will let me overview the planning vs execution of `rig` itself which is
  basically what our MVP does."*

  | # | Requirement |
  |---|---|
  | ⛔ **14** | ⛔ **THE FIRST TAB'S SUBJECT IS PLANNING VERSUS EXECUTION.** This retires the *"we'll define in the future"* deferral for THIS tab - he has now named what the useful details are. It is not a record browser and it is not a table of the store; **it is the answer to "what did we say we would do, and what has actually happened"** |
  | **15** | **THE GUI IS ALSO AN INSTRUMENT ON rig ITSELF** - *"get a grip on the usability"*, *"get a feel of the product while it's being built"*. **So an ugly truth rendered honestly serves this requirement and a flattering summary defeats it** |

  ⛔ **REQUIREMENT 14 HAS A CONSEQUENCE THAT MUST NOT BE SOFTENED IN THE BUILD,
  AND IT IS THE WHOLE VALUE OF THE TAB.** Measured against live `production`
  2026-09-17: **all 68 records read `status: active`**, **every one of the 59
  rendered rows reads `not stepped`**, and **the 9 items that ARE closed are
  expressed only by their ABSENCE from the list.** So a truthful
  planning-versus-execution view today shows **a full plan and almost no
  recorded execution** - and three of those facts are live defects the
  2026-09-17 attack filed (S2-1, S6-1, S1+S4-1).

  ⛔ **A TAB THAT SMOOTHS THAT OVER HAS FAILED REQUIREMENT 15 WHILE APPEARING TO
  SATISFY 14.** He asked for a feel of the product; the product's current feel
  is that execution is not being recorded. **Render it.**

  **AND IT SETTLES THE "IS THIS A DISTRACTION" QUESTION WITH HIS OWN
  REASONING:** *"which is basically what our MVP does"*. The MVP is rig managing
  rig's own project; this tab is that capability with a face on it. **It is off
  the MVP's critical path and it is pointed at the same target** - which is why
  it runs alongside rather than after.

- ⛔ **BORIS, 2026-09-17, verbatim - THE RAIL LISTS *GUIs*, NOT PROGRAMS, AND
  THIS RESOLVES THE PROMOTION TENSION RATHER THAN INHERITING IT:** *"About rig's
  own capabilities in the GUI, we can probably think of a better way showing it.
  Maybe the things that can be selected from the left are not only the different
  programs but we can generalize it into different GUIs. So a GUI can be a GUI
  of a program but it could also be a GUI of some internal `rig` functionality
  like project management and things like that. I'd also like these selectables
  on the left to have icons and maybe colors so that they are easily
  distinguishable. This way we can have for example a GUI for AgentBox, a GUI
  for lets say library, and so on, but also a GUI for project/case management
  and other GUIs. The GUI for project/case management internally can have its
  own layout, for example a tab for each project/case and maybe even toggling
  between cases and projects and inside cases maybe even sub-division by are of
  interest, for example health, finances, and so on."*

  | # | Requirement |
  |---|---|
  | ⛔ **16 (SUPERSEDES 12)** | ⛔ **THE RAIL'S UNIT IS A *GUI*.** A GUI is **either** a registered program's **or** an internal rig capability's. Requirement 12 said *"rig is a program in its own shell"*; **it is not, and it does not need to be** |
  | **17** | **A RAIL ENTRY CARRIES AN ICON AND A COLOUR**, so entries are distinguishable at a glance rather than by reading |
  | ⛔ **18** | ⛔ **THE PROJECT/CASE GUI HAS ITS OWN INTERNAL LAYOUT, AND THE TABS LIVE HERE.** A tab per project or case; a toggle between **cases** and **projects**; and within a case, an optional sub-division **by area of interest** - his examples, *health*, *finances* |

  ✅ ⛔ **REQUIREMENT 16 ANSWERS THE OPEN DESIGN QUESTION THIS SECTION RAISED
  ONE HOUR EARLIER, AND IT ANSWERS IT BETTER THAN EITHER OPTION ON THE TABLE.**
  The question was: requirement 12 asked for *"the `rig` program we already have
  there"*, while §5's **PROMOTION IS DEAD** closed four independent doors - rig
  is in no `CapabilityMap`, `meta` refuses `"rig"` as an invoke target,
  `d.call` routes through `d.programs[program]` and rig has no connection to
  itself, and putting rig in the map would place `rig.down` on the agent invoke
  surface. **The two options were: fake a registry entry, or register rig with
  itself and reopen a namespace reservation refused twice.**

  ⛔ **HE TOOK NEITHER. A GUI IS NOT A PROGRAM**, so a project/case GUI in the
  rail asserts nothing about the registry, needs no `CapabilityMap` entry, and
  leaves every one of the four closures standing. **The promotion ruling is
  untouched and requirement 12's problem simply stops existing.** Recorded at
  this length because the next seat will find the promotion ruling and think it
  blocks the rail.

  ⛔ **REQUIREMENT 17 LANDS ON A GAP THIS PROJECT ALREADY FOUND AND COULD NOT
  CLOSE**, and the two halves now have different answers:

  | | |
  |---|---|
  | **an INTERNAL GUI's colour** | **free.** It is rig's own surface, so rig declares it. §6's theme already computes a hue set (`--h-*`) and the contrast gate already measures every member |
  | ⛔ **a PROGRAM's colour** | ⛔ **NOT AVAILABLE ON THE WIRE.** `cmd/rigwindow/service.go` records it: §11 gives a program one ownable hue and says a program without one leaves the shell achromatic, but **`rigv1.Identity` carries id, name, version, icon and description and no hue at all** - so every program is achromatic today, and not by choice. **Deriving one from the id would fake a declaration the program never made** |

  **So requirement 17 is satisfiable NOW for the internal GUIs and needs a wire
  field for the program ones.** A build that quietly invents program hues to
  make the rail look finished has broken §5e's declaration model to win a
  screenshot.

  ⛔ **AND REQUIREMENT 18 MOVES THE TABS FOR THE SECOND TIME TODAY, SO READ THE
  CURRENT POSITION RATHER THAN A REMEMBERED ONE.** They began as the shell's top
  level, moved down into rig's program GUI, and now sit inside the
  **project/case GUI**, one per project or case. **The area-of-interest
  sub-division is a THIRD level** and he marked it *"maybe"* twice - treat it as
  a shape to leave room for, not a thing to build now.

  ⛔ **A MODEL QUESTION FALLS OUT OF 18 AND IT IS NOT THE WINDOW'S TO ANSWER:**
  `project` and `case` are already a distinction the wire carries
  (`ProjectBriefResponse.kind`, and a case has no semver and a different status
  vocabulary). **"Area of interest" is not** - nothing in §39 holds it, and the
  2026-09-17 attack found `kind` unconstrained while every other axis is
  unqueryable. **So the toggle is renderable today and the sub-division is not.
  Say so rather than inventing a field.**

- ⛔ **BORIS, 2026-09-17, verbatim - A GUI FOR THE AGENTS' AREA, AND ITS DEFAULT
  IS THAT HE IS NOT LOOKING:** *"The user will be able to introspect into agents
  area through the GUI but it will usually be of no interest to him so only if
  he want so; There should be a GUI that lets him see all interactions of the
  agents with `rig`."*

  | # | Requirement |
  |---|---|
  | **19** | **AN AGENTS GUI EXISTS IN THE RAIL** - another internal GUI under requirement 16, not a program |
  | ⛔ **20** | ⛔ **ITS DEFAULT IS OUT OF THE WAY.** *"Usually of no interest to him so only if he want so."* **It must not push, badge, notify or occupy the dashboard.** This is a requirement ABOUT ATTENTION and it is as binding as the content one |
  | ⛔ **21** | ⛔ **IT SHOWS *ALL* INTERACTIONS OF THE AGENTS WITH rig** - every call, not a curated summary. *"All"* is his word |

  ⛔ **REQUIREMENT 21 IS THE ONE WITH A COST, AND IT IS A STORAGE COST BEFORE IT
  IS A UI ONE.** *"All interactions"* is the highest-volume stream this product
  will have - §15 already sizes history at **500 MB holding 10.56M records** and
  §9 already warns about a program emitting **tens of thousands of frames**.
  **So requirement 21 is the first real customer of §7's retention ladder** (his
  one-month compress, three-month delete), and the two were stated within
  minutes of each other. **Build them as one thing or the GUI ships a surface
  that grows without bound.**

  ✅ **AND THE GOOD NEWS, WHICH IS THAT MOST OF THIS IS SPECIFIED ALREADY:** §8
  specifies *"Events: a live tap on the bus - publisher, topic, payload, who
  received, who was denied"*, and §15 is *"who is using rig and what they did"*
  with retention by size, age AND rate. **Requirement 21 is those two pointed at
  agents and given a pane** - it is largely a RENDERING job over a stream that
  is already designed, which is the cheapest shape a new requirement can have.
  ⛔ **Do not design a new capture path. Grep §8 and §15 first.**

  ⛔ **ONE THING REQUIREMENT 20 FORBIDS THAT A BUILDER WILL WANT TO DO ANYWAY:**
  an agent-activity feed is the single most tempting thing to put on the
  dashboard, because it moves and looks alive. **He has ruled it usually of no
  interest.** A dashboard that fills with agent chatter has broken requirement
  20 while satisfying 21.

The UI shell inside rig. Its visual system is a separate piece of work (§23 M1a) because for a
program whose whole job is presenting other programs, the visual design *is* the product. It is
built and measured: `design/visual-system.html`, engine at `design/theme.js`.

- **One window.** A left rail of registered programs, a context bar, a pane, a status strip.
  Chrome budget is a number that gets defended, not a feeling.
- **One tray icon** for the whole estate, replacing the six that exist today. Per-program status,
  badge, and commands runnable with no window open. A stopped program is still listed, with the
  reason and a Start entry.
  - ⛔ **BORIS, 2026-09-16 (evening), verbatim - THE TRAY'S LIFETIME IS RIGD'S,
    NOT THE WINDOW'S. RECORDED THE TURN HE SAID IT:** *"As long as rig is
    present in the background, the appropriate icon must appear on the
    system-tray, like AgentBox. Clicking on the icon reveals the different
    options and it also shows the version and whether it's prod or dev."*

    **FIVE REQUIREMENTS NOW, and the first is the one that may move
    architecture.** It said THREE while the table held four rows, from
    2026-09-16 until 2026-09-17 - a caption that is a claim and that no gate
    checks, which is the defect class this project recorded seven times in one
    day. **Count the rows before you trust the number above them.**

    | # | Requirement |
    |---|---|
    | 1 | **PRESENT WHENEVER rig IS IN THE BACKGROUND.** The condition is rig running, not a window being open. *"like AgentBox"* is the benchmark and AgentBox's tray is up whenever its daemon is |
    | 2 | **CLICKING REVEALS THE OPTIONS.** The menu is the access point, not a window toggle. This restates and sharpens the access-point requirement below |
    | 3 | **THE MENU SHOWS THE VERSION AND THE ESTATE** - prod or dev - **as TEXT a human reads**, not only as the icon's colour |
    | ⛔ **4** | ⛔ **CLOSING THE WINDOW MUST NOT REMOVE OR TERMINATE THE TRAY ICON.** Boris, 2026-09-16, verbatim: *"Closing the window should not remove or terminate the system-tray icon"*. **It follows from requirement 1 and is stated separately because the implementation can satisfy 1 at startup and break it on the first close** - the tray belongs to the WINDOW PROCESS, so anything that ends that process ends the icon |
    | ⛔ **5** | ⛔ **THE WINDOW STARTS HIDDEN. THE PROCESS AUTOSTARTS; THE WINDOW DOES NOT.** Boris, 2026-09-17, verbatim: *"I see that rig is started automatically after a reboot, but the window should be hidden by default, it is usually a background worker and shown ad-hoc."* **This is the half of the third answer that was named and never built.** The scope call below chose *"the window process is always started and the window itself is what is optional"* and **only the first clause shipped** |

    ⛔ **REQUIREMENT 1 WAS NOT SATISFIED BY WHAT WAS BUILT, and the gap was
    structural rather than a missing feature.** The tray lives in
    `cmd/rigwindow/tray.go` - it is the WINDOW process that owns it. So a
    headless `rigd`, which is the ordinary case and the one a systemd unit
    creates, showed NO ICON AT ALL. **"As long as rig is present in the
    background" was exactly the state that had no tray.**

    **The scope call was Boris's**, because the honest answers were not small:
    the tray moves into `rigd`, or `rigd` supervises a tray process, **or the
    window process is always started and the window itself is what is
    optional.**

    ✅ ⛔ **HE TOOK THE THIRD ONE, 2026-09-16, AND IT IS NOW INSTALLED AND
    RUNNING.** `packaging/rigwindow.service` starts the window process at
    login (`WantedBy=graphical-session.target`), so the tray is up whenever the
    graphical session is - which is *"like AgentBox"*, the benchmark
    requirement 1 names. `make install-window` puts the binary and the unit in
    place; rig `6afd76e`.

    **MEASURED, not inferred** `[ran it]` 2026-09-16: `systemctl --user
    is-enabled rigd.service rigwindow.service` -> `enabled`, `enabled`;
    `is-active` -> `active`, `active`; `rig estate` over the real socket ->
    `name production`. **A reboot has NOT been observed** - Boris waived it:
    *"We won't reboot to check if it survives a reboot - I trust you install it
    properly"*. **So the install is EVIDENCED and the survival is TRUSTED, and
    those are different words on purpose.**

    ✅ ⛔ **THE REBOOT HAPPENED, 2026-09-17, AND THE WAIVER IS SPENT.** The
    machine booted at `09:11:31` and `rigwindow.service` entered active at
    `09:12:00` with `NRestarts=0` and no hand on it. **Boris saw it himself and
    said so:** *"I see that rig is started automatically after a reboot"*. **So
    requirement 1, and the MVP's condition 2, are now EVIDENCED across a real
    boot rather than trusted.** `[ran it]` 2026-09-17: `uptime -s`,
    `systemctl --user show rigwindow.service -p ActiveEnterTimestamp -p
    NRestarts -p MainPID`, `systemctl --user is-enabled|is-active`.

    ⛔ **AND THE SAME BOOT PRODUCED REQUIREMENT 5, WHICH IS WHAT HE ACTUALLY
    SAW.** The unit starting the window process is correct and is what makes the
    tray survive a login. **What is not correct is that the WINDOW came up with
    it.** `cmd/rigwindow/main.go` builds the window with no `Hidden` option, and
    Wails shows a window on creation unless told otherwise
    (`webview_window_options.go:177`, *"Hidden will hide the window when it is
    first created"*), so every login and every `Restart=on-failure` puts a
    1280x800 window in front of whatever he was doing.

    ⛔ **AND THE FIELD IS READ ON THIS PLATFORM, WHICH HAD TO BE CHECKED
    RATHER THAN ASSUMED.** A first grep said `options.Hidden` was touched only
    by the darwin backend, which would have made `Hidden: true` the exact shape
    §9 records under promotion - *compiles, lints, passes, does nothing*. **The
    grep was truncated by `head`.** `webview_window_linux.go:453` guards
    `w.show()` on `!options.Hidden` and the Linux path honours it.
    **Positive-control every empty grep, and count the rows `head` ate.**

    **ONE THING THAT GUARD ALSO SKIPS, recorded before it surprises somebody:**
    `applyScreenPlacement()` sits inside the same `if`, so a window first shown
    from the tray takes GTK's placement rather than any `StartState` the options
    ask for. **Nothing is lost today** - this window asks for no `StartState` -
    **but one added later would be silently dropped until the first show.**

    **THE SHOW PATH ALREADY EXISTS, WHICH IS WHY THIS IS AN OPTION AND NOT A
    FEATURE.** `cmd/rigwindow/tray.go:61` adds a **Show rig** menu entry whose
    title already starts at *"Show rig"* and is retitled by `refreshWindowItem`
    from `win.IsVisible()`, so a hidden start is the state that entry was written
    for. **Requirement 5 is one field; what it is NOT is one field of evidence** -
    a hidden start is only demonstrated by a real login or a real unit restart,
    and the unit is on his screen, so the restart is his to authorise.

    ⛔ **AND IT SHARPENS REQUIREMENT 4 RATHER THAN RESTATING IT.** Requirement 4
    says a close must not kill the tray; requirement 5 says the window was never
    supposed to be open. **Together they make the window a pure toggle on the
    tray with no path that opens it uninvited**, which is what *"it is usually a
    background worker and shown ad-hoc"* means in one sentence.

    ✅ ⛔ **BUILT, DEPLOYED AND DEMONSTRATED THE SAME MORNING. THE TABLE ROW
    ABOVE THAT SAYS THE CLOSE HOOK IS `[read it]`, NEVER EXERCISED, IS NOW
    SUPERSEDED.** Boris: *"You can redeploy whenever you want."* `make
    install-window` + `systemctl --user restart rigwindow.service` at
    `v0.0.0-m0-381-g862ae30`, **against the real installed unit, not a
    worktree binary and with no `-dirty` in the stamp.**

    **The instrument, because it is the part worth reusing:** the tray menu was
    driven over its own **DBusMenu** interface - `GetLayout` to read the entry's
    live label and `Event`/`clicked` to activate it - so the *"Show rig"* path
    was exercised through the same code a panel click reaches, not simulated.
    The close was a real `_NET_CLOSE_WINDOW` via `wmctrl -ic`, which is the
    titlebar X's own event.

    | # | State | Tray entry reads | Window |
    |---|---|---|---|
    | 1 | after a unit restart | **`Show rig`** | **not mapped** |
    | 2 | after activating that entry | `Hide rig` | mapped |
    | 3 | 1s after the X closed it | **`Show rig`** | **not mapped** |

    **Throughout: same pid, `NRestarts=0`, `ActiveState=active`, and the tray's
    `StatusNotifierItem` still in the watcher's registered list.** So requirement
    5 passes and requirement 4 is now `[ran it]` rather than `[read it]`. **No
    `SIGABRT`** - consistent with the 2026-09-16 review that downgraded it, and
    it stays UNVERIFIED rather than closed because this close was also
    synthetic; a pointer on the titlebar is still the one path never tried.

    ⛔ **AND THE DEMONSTRATION FOUND A DEFECT THAT READING HAD NOT, WHICH IS THE
    WHOLE ARGUMENT FOR RUNNING IT.** Step 3 first printed **`Hide rig` for an
    already-hidden window** - and `toggleWindow`'s branch is on
    `win.IsVisible()`, so clicking an entry labelled *Hide* would have **shown**
    the window. **A label doing the opposite of what it says.**

    **WHY NOBODY CAUGHT IT BY READING, and it is the interesting half:**
    `retitleWindowItem` IS called from `pollEstate`, every `trayRefresh` = 5s,
    and its own doc comment says so - *"from the poll loop so it self-corrects
    when the window is hidden or shown by anything other than this menu"*.
    **The comment is TRUE. The defect was bounded at five seconds, never
    permanent, and therefore invisible to any reader who checked whether a
    caller exists.** `toggleWindow` retitles on its own click path *"for
    immediacy"*; the `WindowClosing` hook is the OTHER way visibility changes and
    it was the one that did not. **Fixed at rig `862ae30`** - one call in the
    hook - and step 3 above is the re-run.

    ⛔ **A METHOD NOTE, BECAUSE IT COST TIME TWICE IN ONE HOUR AND BOTH WERE THE
    SAME CLASS.** Diagnosing this, a grep for `refreshWindowItem` returned empty
    and read as *"nothing calls it"*; the function is named `retitleWindowItem`.
    Earlier the same hour, a grep for `options.Hidden` **truncated by `head`**
    read as *"the Linux backend never reads it"*. **Both empty results were the
    instrument, not the code, and both pointed toward a bigger defect than was
    there.** `POSITIVE-CONTROL EVERY EMPTY GREP` is already written down in four
    places in this project and it still bit twice in sixty minutes, so the
    operational form is the one to carry: **an absence is a claim about your
    instrument before it is a claim about the code.**

    ⛔ **THE THIRD ANSWER CARRIES A COST THAT THE OTHER TWO DID NOT, AND IT IS
    REQUIREMENT 4.** If the tray's owner is the window process, then **every
    path that ends that process ends the icon** - closing the window, an
    unhandled panic, a `Quit` the user did not mean. Requirement 4 is that
    cost made explicit. The other two answers (tray in `rigd`, or `rigd`
    supervising it) would not have had it, and **choosing this one makes the
    close path load-bearing rather than cosmetic.**

    **WHAT IS BUILT AGAINST REQUIREMENT 4, and what is not:**

    | | |
    |---|---|
    | the `WindowClosing` hook | `main.go:106` **cancels the event and hides**, skipping Wails' default listener, which destroys the window and quits the process once none remain |
    | ⛔ **not demonstrated** | the hook is `[read it]`, never exercised on this machine. **A comment describing a cancel is not evidence that the cancel fires** |
    | ⛔ **and `cmd/rigwindow` IS IN NO GATE** | `Makefile:71` and `:78` exclude it, so `make ci` says nothing about any of this. `BACKLOG.md` B50 |
    | the systemd unit | `Restart=on-failure`, so a **clean** exit is NOT restarted. That is right for the tray's own `Quit` entry and **wrong for every other way the process could end** |

    **REQUIREMENT 3 IS SEPARATELY NOT BUILT.** The tray polls `rig.estate`
    every 5 s and swaps its ICON to carry the estate; nothing renders the
    estate or the version as readable text, and `fyne.io/systray`'s menu items
    are where it would go. **The icon says which estate to somebody who knows
    the colour convention; he asked to be TOLD.**

  - **BORIS, 2026-09-16, verbatim - THE TRAY IS AN ACCESS POINT, NOT A WINDOW
    TOGGLE:** *"The icon-tray is not only for when a window is needed, it is
    my access point to many of the future rig functionality."* Said right
    after the mark shipped (the "DONE 2026-09-16" bullet below), so it is not
    a restatement of that work - it is scope on everything built against the
    tray from here on. **The click handler wired in this session
    (`cmd/rigwindow/tray.go`) only toggles the window**, which this
    requirement says is not the ceiling: `SystemTray.SetMenu` is the
    mechanism Wails already exposes for tray-native commands, and it is
    unused so far. Not built this session - recorded so the next one does not
    treat "the tray shows an icon" as this requirement closed.
  - **BORIS, 2026-09-11, verbatim and still unsatisfied:** *"I also want the rig
    system tray icon to be eye-catching and engaging."* **This is a requirement,
    not a preference, and it is HIS OWN WORDS rather than a seat's reading.**
    Recorded here 2026-09-11 after an audit found it existed **only** in
    `HANDOFF.md`'s session narrative - not in this document, not in
    `BACKLOG.md`, not in `DECISIONS.md` - having been raised to two separate
    leads and answered by neither.
  - **It is not the same request as the visual system's.** *"Engaging and
    eye-catching, not dull"* was said of `design/visual-system.html` and was
    satisfied there (Fraunces display type, a full-bleed hero, the window on a
    lit stage). **The tray is a 22px icon in a shell panel and none of that
    transfers.** Treating the two as one request is how this one gets marked
    done without being done.
  - **The constraint that makes it hard, and it is why it needs deciding rather
    than drawing:** the icon must read at 22px, carry per-program status and a
    badge, hold a fourth **detached** state (§5g), and stay legible against an
    arbitrary user shell theme in both light and dark. `readable-output`'s rule
    binds it - "hard to read" is a measurement, not taste - so whatever is drawn
    is measured at its real size on real panel backgrounds, not admired at 512px.
  - **BORIS, 2026-09-11, verbatim and also his own:** *"Production and
    development icons should be different if possible."* **This is a SECOND
    requirement, not a restatement of the first**, and it is the one that
    interacts with §37.
  - **IT QUALIFIES THIS SECTION'S OWN HEADLINE, which nobody had noticed.**
    "One tray icon for the whole estate" was written when there was one estate.
    **§37 allows TWO named estates running at once - that is the entire point of
    self-hosting - so there are two trays on one panel, and the sentence above
    means one icon PER ESTATE rather than one icon.** Until this requirement
    landed, the two would have been byte-identical and Boris would have had two
    indistinguishable icons in his own panel, which is the exact condition
    §37 exists to make legible on every other surface.
  - **What it costs, stated because it is the hard part:** the icon already owes
    per-program status, a badge and the **detached** state (§5g). Estate identity
    is a **fourth dimension on a 22px glyph**, and the three that were already
    there are not negotiable. So this is a design constraint on the whole mark
    rather than a decoration added to it, and it is why §37's `rig.estate`
    matters here: **the window learns `production` / `development` from the wire
    (`EstateResponse.role`), never by guessing from a name**, so the icon's
    variant is derived from the same source every other surface reads.
  - **BORIS, 2026-09-16, verbatim and his own - THE ASSETS EXIST AND THIS IS
    THE WORK ON THEM:** *"Optimize development.png and production.png from my
    desktop to act as our system-tray icons, remove the text and crop them
    properly so that they are perfectly circle and that we don't take
    unnecessary parts of the images and when finished persist them into the
    rig plan."* He picked the pair from a generated candidate sheet
    (`logbook/projects/rig/icon-candidate-prompt.md` holds the prompt), so
    the MARK is now chosen and is no longer an open design question.
    - **Four operations, all of them his words:** remove the text, crop to a
      PERFECT CIRCLE, drop every unnecessary part, and optimize for use at
      tray size.
    - **"Persist them into the rig plan" means the repository owns the
      assets**, not a Desktop folder. They live under `design/` beside the
      visual system that is already built and measured there.
    - **THE THREE UNDRAWN DIMENSIONS ARE STILL OWED AND THIS DOES NOT CLOSE
      THEM.** A chosen mark is the first of four: per-program status, the
      count badge and the **detached** state of §5g are not decoration added
      afterwards. A mark that reads at 22px alone and dies with a badge on it
      has failed the requirement, not passed it.
    - **DONE 2026-09-16: `design/tray/development.png` and
      `design/tray/production.png`, transparent PNG, perfect circle, no
      text.** Boris reviewed both in-session and corrected the direction
      twice before accepting them, both now recorded because neither is
      derivable from the files alone:
      - **the crosshair ring and its tick marks are DROPPED, not carried
        over from the candidate sheet.** Boris: *"The production can be just
        the robot inside with the circle around it - no need for the
        background at all and it needs to fill it well."* The glyph (bug /
        robot) is cropped tight to fill the circle instead, on his
        instruction that content should "fill it almost completely, just
        leave little room to see it's round."
      - **the source art's 3D bevel (gloss highlight, drop shadow) is
        flattened to one flat fill colour per icon.** Boris: *"I think the
        3D effect is redundant and will probably just hurt the good-looks of
        the icons."* This also brings the shipped asset in line with
        `icon-candidate-prompt.md`'s own flat-vector constraint, which the
        generated sheet had not actually followed.
      - Both checked legible at 22px against a dark panel before being
        accepted - the untouched candidate art was not (see
        `icon-candidate-prompt.md`'s own "starting point for a hand-drawn
        SVG, never the shipped asset" caveat, which the flattening step
        answers for now without yet being that SVG).
    - **SUPERSEDED 2026-09-16, same session: the mark above is no longer the
      shipped one.** Boris: *"Let's use the agentbox tray-icon for rig. The
      production version should be as the one AgentBox is currently using,
      the development should have a yellow background."* `design/tray/*.png`
      is now built from `agentbox/internal/tray/icons/idle.png` (that
      project's own steady-state tray glyph), not the candidate sheet -
      **production** is that glyph recropped to this project's existing
      256px/perfect-circle convention, **development** is the same glyph at
      84% scale centred on a solid disc in rig's own amber hue (OKLCH
      hue 78°, `design/theme.js`'s `--h-amber`, pushed to L .80/C .19 -
      `#f5af26` - for a bold badge rather than the muted UI-text token).
      Checked legible and clearly distinct at 22px before being kept: *"I
      like the icons - keep them."* Boris also cleared picking a better
      development background on the spot if contrast asked for it - *"I
      allow you to choose the best background if you find one that is
      better for the development flavour so that the contrast is
      optimal"* - answered by the amber choice above rather than acted on
      further, since it already reads clearly at 22px and holds a
      conventional prod-vs-dev colour meaning (stable green, cautionary
      yellow). The bug/robot candidate-sheet pair and
      `icon-candidate-prompt.md`'s own generation prompt are now history,
      not the source of truth - kept in the repo and this document's earlier
      bullets as the record of how the first pass was made and un-made, not
      because either is still live.
    - **WIRED 2026-09-16, same session, twice: `cmd/rigwindow/tray.go` is a
      real `org.kde.StatusNotifierItem` on the session bus**, not just files
      on disk. First wiring used Wails' own `application.SystemTray` and
      LOOKED right - the object exported fine, properties answered over
      D-Bus, `register()` logged no error - but its item never reached
      `org.kde.StatusNotifierWatcher`'s own `RegisteredStatusNotifierItems`
      list, confirmed by resolving the tray's own D-Bus unique name against
      that list and finding it absent, so nothing ever drew it. **Root cause
      not fully chased down** (Wails v3 is beta.19); the fix taken instead
      was switching to `fyne.io/systray` - the library `agentbox` itself
      uses in this same session's environment and visibly works there -
      confirmed this time by the same check succeeding: the tray's unique
      name resolves to `rigwindow`'s own pid inside the Watcher's list.
      `make build-rigwindow` copies `design/tray/*.png` into
      `cmd/rigwindow/icons` (go:embed cannot reach above its own package,
      same arrangement as the frontend's `dist` and the fake applications'
      `kit`). It polls `rig.estate` every 5s and creates or quits the tray
      itself rather than repainting a static one, so an unnamed estate gets
      none - live-verified before the library swap: stopping the named
      estate on the default runtime dir and starting the other one flips the
      `IconPixmap` bytes within one poll, with no restart of the window
      process. Detached (daemon unreachable) keeps the last icon up rather
      than removing it.
      - **This wiring only toggles the window on click.** See the
        2026-09-16 "ACCESS POINT, NOT A WINDOW TOGGLE" bullet above -
        `fyne.io/systray`'s own menu items are the mechanism for tray-native
        commands now (Wails' `SetMenu` no longer applies, the tray no longer
        being Wails'), and it is unused so far.
  - **BORIS, 2026-09-16, verbatim, found closing the window: "I see that closing
    the window of the development flavour removes the icon from the
    system-tray maybe even closes the instance as far as I can tell; This is
    not the intent."** Confirmed: Wails' own default `WindowClosing` listener
    destroys the window and `application_linux_gtk3.go`'s `unregisterWindow`
    quits the whole process once no window remains (Linux and Windows both
    default `DisableQuitOnLastWindowClosed` to false; only macOS's polarity
    happens to already do the right thing). **FIXED, the same session:**
    `cmd/rigwindow/main.go` now calls
    `win.RegisterHook(events.Common.WindowClosing, ...)` and cancels the
    event, calling `win.Hide()` instead - the hook runs before Wails' own
    listener and, being cancelled, that listener never runs, so neither the
    window nor the process is destroyed. This is the standard Wails v3
    "minimise to tray" pattern, not a rig-specific workaround.
    - ✅ **CLOSED 2026-09-17 - IT WAS NEVER rig's DEFECT, AND THE CRASH WAS THE
      INSTRUMENT.** `man xdotool`: *"windowclose - Close a window. This action
      will destroy the window"*. **That is `XDestroyWindow`, not
      `WM_DELETE_WINDOW` and not the X button** - and against an X server with no
      window manager it kills ANY GTK application. ⛔ **`zenity --info`, a plain
      GTK dialog with no Wails, no webview and not one line of rig, DIED
      BYTE-IDENTICALLY on the same display** - the positive control that settled
      it. Requirement 4 is about the X button, which is a thing only a window
      manager has. **Boris then confirmed it by hand:** *"The icon survives
      closing the window."* / *"I can see the icon."* Ruled at logbook
      `a9854f8`, `BACKLOG.md` B52. ⛔ **THE PARENTHESIS BELOW IS THE FALSE
      PREMISE THAT PRODUCED ALL OF THIS, AND THIS SECTION HAD ALREADY FLAGGED IT
      AS ITS OWN ASSERTION RATHER THAN THE RUN'S** - kept, struck, because the
      lesson is that the document argued with itself for five days and won.
      **The original statement follows.**
    - ⛔ **A SEPARATE, DEEPER BUG WAS FOUND WHILE VERIFYING THIS FIX AND IS
      STILL OPEN.** Closing the window on this machine (GTK3 + WebKitGTK,
      X11, Mesa/Intel) SIGABRTs the whole `rigwindow` process regardless of
      the fix above: `Gdk-WARNING: GdkSurface ... unexpectedly destroyed`
      followed by `Gdk:ERROR:gdksurface.c:978:_gdk_surface_destroy_hierarchy:
      assertion failed: (priv->egl_native_window == NULL)`, `SIGABRT`.
      **Reproduced identically on the UNPATCHED baseline** (`git stash` the
      hook, rebuild, close - same crash), so the hook above did not cause it
      and does not fix it: the crash happens before Go's close handling ever
      runs, deep in GTK/WebKitGTK's native reaction to the window-close
      request itself. **Four env-var mitigations were tried and NONE stopped
      it** - `WEBKIT_DISABLE_DMABUF_RENDERER=1`,
      `WEBKIT_DISABLE_COMPOSITING_MODE=1`, `GDK_GL=disable`,
      `LIBGL_ALWAYS_SOFTWARE=1` - so this is not a GPU-backend selection
      problem. **Repro:** build `rigwindow`, run it against a named estate,
      find its `rig`-titled window with `xdotool search --name rig`, close it
      with `xdotool windowclose <id>` (⛔ ~~a synthetic `_NET_CLOSE_WINDOW`, which
      GDK's X11 backend turns into the same `delete-event` a real titlebar
      click sends~~ **FALSE, AND IT IS THE ORIGIN OF B52.** `xdotool windowclose`
      is `XDestroyWindow`; nothing turns it into a `delete-event`.) - the process aborts within ~1-2s, taking rigd's own comfort
      (development-estate `rigd` stays up fine) but the tray and window both
      vanish with it. **Not chased further into Wails' or WebKitGTK's own
      source** - this needs either an upstream fix, a WebKitGTK/GTK version
      change, or an architecture change (e.g. `Frameless: true` with an
      in-webview close button that calls `Hide()` over the RPC bridge instead
      of ever letting the native delete-event fire) - a real scope decision,
      not a one-line patch. **The next session should treat this as the
      actual open item**, not the hook fix above, which is done.
    - ⛔ **RE-RUN 2026-09-16 EVENING AND IT DID NOT REPRODUCE.** Section 9's
      review instruction was carried out at `695ba56`, in a detached worktree
      with `git status --porcelain` printed empty from inside it and the binary
      stamped `695ba56` with no `-dirty`. **Three closes across three runs** -
      `wmctrl -ic` twice, and once `xdotool windowclose <id>`, the repro above
      **verbatim**, against a live `rigd` on the real runtime directory. **No
      `SIGABRT`, no `Gdk-WARNING`, no `egl_native_window` assertion; the process
      was alive eight seconds after every close** and exited only on the
      `SIGTERM` that ended the probe (`rc=143`, never `rc=134`). The three
      claims this section calls done all HELD in the same run: the tray item
      registers (one new `@/StatusNotifierItem` appears in
      `RegisteredStatusNotifierItems` exactly at launch), **it survives the
      window close** - which was Boris's original complaint - and
      hide-instead-of-quit holds with zero windows mapped.
      **DOWNGRADED FROM BLOCKER TO UNVERIFIED, NOT CLOSED**, and the limits are
      the point: the window was **never closed by a pointer click on the
      titlebar X**, only by a synthetic `_NET_CLOSE_WINDOW` (⛔ **this section
      asserted those are the same `delete-event`, flagged its own assertion as
      the document's rather than the run's, AND WAS RIGHT TO DOUBT IT: the
      assertion is FALSE and it is what produced B52**); **the tray menu's Quit item was never
      exercised**; and the window was **never re-shown from the tray and closed
      a second time.** *"Did not reproduce three times"* is not *"fixed"* and
      must not be cited as if it were. Full record, including an instrument bug
      that cost one run, in `DECISIONS.md` under "THE SECTION 9 REVIEW
      INSTRUCTION WAS RUN, AND IT CUT THE OTHER WAY".
  - **An UNNAMED estate gets no tray at all.** Every test and every reproduction
    recipe starts one, they are not deployments (§37's ephemeral clause), and a
    third icon appearing during `make ci` would be the failure this requirement
    is trying to prevent.
- **Three pane tiers**, and the middle one exists because the outer two leave a gap.
  *Generated*: the program declared a schema and rig renders forms, tables, actions, progress,
  detail and status - and the program never names a widget (§5h). *Kit*: the program serves its
  own HTML and composes it from rig's own elements. *Embedded*: the program serves its own HTML
  and brings its own components, getting the token set and nothing more.
- **The all-managed-projects overview is a Generated pane**, rig registered as a program in its
  own window. §39 supplies the schema and the data (`record.query(kind: project)` for the roster,
  `project.brief` per project for the content) - this section owes only the rendering.
- **The gap the kit closes, and the version of this claim that was measured and found false.**
  The tier exists because a program serving its own HTML gets tokens and stops there, so one
  visual system cannot reach it. What this section used to say - that `archi`, `dispatch` and
  `snapper` each rebuild a table, a toolbar, an empty state and a spinner and get all four
  subtly wrong - is not true of any of them (§5h). **The kit's adopters are the fake
  applications of §23 until a real program asks for it.**
- **The shell is achromatic.** `design/theme.js` ships **seven** hues of which **five** are
  ownable, and a program with no identity hue stays achromatic, which is what the shell is
  anyway. The owned hue is the only saturated colour on screen while you are in it. A host that
  wears the colour of whatever it is holding.
- **Healthy is the absence of colour.** Six programs, most idle, most of the time, so `ok` is
  drawn as a hollow tick and a hue appears only when something wants you. A stopped program
  keeps its place and goes dashed, so the rail never re-orders under your hand.
- **The window survives a daemon restart.** It is a separate process (§17), which is what makes
  the lifecycle notice in §5g drawable at all: the tray gains a fourth state, **detached**,
  meaning the window is up and `rigd` is not.
- **The visual system is config** (§6). Faces, sizes, scale, radius, density, the hue family's
  parameters and whether anything animates are declared, layered and live-pushed, and rig
  refuses a token set that fails its contrast targets. Built and measured:
  `design/visual-system.html`, with `design/theme.js` as the engine.

## ⛔ The tray is how he knows rig is running, and it owes a down state. Ruled by Boris 2026-09-17.

> *"I want to know about the presence of the background process of `rig` by the
> system-tray icon and I'm not sure this is the case. If I don't have it I don't
> have any visual way of knowing whether it's up or not, if the daemon is down
> it can also indicate it with a red dot and details when clicked. ... I want as
> I said to see the `rig` system-tray icon whenever it is relevant."*

⛔ **"AS I SAID" IS THE LOAD-BEARING PHRASE. THIS IS THE SECOND TIME, AND THE
FIRST TIME IT WAS FOUND LIVING ONLY IN A HANDOFF.** `CLAUDE.md` names the tray
icon as one of four requirements of his that survived several sessions in a
volatile document. **It is in the specification now.**

### What he measured, and it was not there

`[ran it]` 2026-09-17, on his machine, while he was asking:

| | |
|---|---|
| `systemctl --user status rigwindow.service` | ⛔ **`inactive (dead)` for 2h42m**, `status=0/SUCCESS` |
| the StatusNotifier watcher | **ten registered items, none of them rig's.** AgentBox's was there |

⛔ **HE WAS RIGHT AND THE ICON WAS SIMPLY GONE.** Nothing reported it, which is
the whole of his complaint: **the icon is the only readout, so when the readout
is absent there is no second way to look.**

⛔ **THE CAUSE IS A DESIGN DECISION, NOT A CRASH.** `rigwindow.service` carries
`Restart=on-failure`, and the unit's own comment explains why: *"A window the
user CLOSED is not a failure and must stay closed."* The process exited **0**,
so systemd correctly did nothing. **But `Quit rig window` quits the PROCESS, and
the process owns the tray** - so an action about the WINDOW silently takes the
TRAY with it. Those are two different things to him and the menu conflates them.

### The three states the icon owes

| State | Icon |
|---|---|
| **rigd answering, named estate** | today's `production.png` / `development.png` |
| ⛔ **rigd down or unreachable** | ⛔ **A RED DOT. HIS WORD.** Today the icon does not change at all - `pollEstate`'s detached branch keeps the last-known glyph and changes only the menu TEXT |
| **unnamed estate** | no tray, unchanged (§11's existing rule) |

⛔ **THE DETACHED GLYPH WAS ALREADY OWED AND WAS DEFERRED FOR WANT OF A
DECISION.** `cmd/rigwindow/tray.go` says so in as many words: *"A dedicated
detached glyph is one of the three dimensions section 11 still owes and is not
decided here."* **He has now decided it.**

⛔ **AND "DETAILS WHEN CLICKED" IS A REQUIREMENT ON THE MENU, NOT ONLY ON THE
GLYPH.** A red dot that opens a menu saying `no daemon answering` and nothing
else tells him it is down and not why, when, or what to do. **The menu owes
what the window cannot say while it is detached.**

### ⛔ "Whenever it is relevant" is a presence requirement and it is the hard half

The glyph is the easy part. **His sentence is about the icon BEING THERE**, and
today three separate things remove it: quitting the window, an unnamed estate,
and the process dying with `Restart=on-failure` declining to bring it back.

⛔ **THE ANSWER IS NOT `Restart=always`.** That would make the close button do
nothing, which is the scar `rigd.service` spends thirty lines on. **The answer
is to stop the WINDOW's lifecycle from owning the TRAY's** - closing a window
must not end a process whose job is to be visible.

⛔ **AND IT COMPOSES WITH HIS MVP ACCEPTANCE TEST, WHICH IS THE SAME SENTENCE
ONE DAY EARLIER:** *"I'll know we reached MVP when I'll see the production icon
on my system-tray both during this session and after I reboot the machine."*
**A tray that can vanish mid-session fails the first half of that test**, and
the first half is the one that looked settled.

## ⛔ AgentBox's tray icon becomes "AB". Ruled by Boris 2026-09-17.

> *"Replace the icon for AgentBox with \"AB\" so that it doesn't look like the
> rig icon and we'll redeploy agentbox."*

**It is a rig requirement even though the change is in another repository**,
because the thing being protected is rig's: **two icons a glance apart make the
rig tray icon unreadable as rig's**, and the icon is his only readout.

⛔ **THE TEST IS A GLANCE, NOT A DIFF.** They are adjacent in one strip at one
size. **Distinguishable when compared side by side is not the bar**; telling
them apart without comparing is.


### ⛔ "Always available" is the requirement, and it is stronger than "whenever relevant". Boris, 2026-09-17.

> *"Rig system-tray icon should always be available."*

⛔ **THIS SUPERSEDES HIS OWN "whenever it is relevant" FROM THE SAME DAY**, and
the narrowing is his: **there is no state in which the icon is absent.** A seat
weighing whether a given moment counts as *relevant* is answering a question he
has now closed.

**Three things remove it today and each needs its own answer:**

| # | What removes it | Why the obvious fix is wrong |
|---|---|---|
| 1 | `Quit rig window` | it quits the PROCESS, which owns the tray. **The menu row conflates a window with a tray** |
| 2 | an unnamed estate | §11's own rule quits the tray. **Written to stop a third icon appearing during `make ci`** - a real problem, wrong remedy |
| 3 | `Restart=on-failure` after a clean exit | ⛔ **`Restart=always` IS NOT THE ANSWER.** It makes the close button do nothing, which is the scar `rigd.service` spends thirty lines on |

⛔ **THE SHAPE OF THE FIX IS THAT THE WINDOW'S LIFECYCLE MUST STOP OWNING THE
TRAY'S.** Closing a window is a normal act; ending the one thing whose job is to
be visible is not. **They are one process today and that is the whole defect.**

⛔ **AND THE `make ci` CASE IS REAL AND MUST NOT BE REGRESSED.** Every test and
reproduction recipe starts a daemon, and an icon per test run is worse than no
icon. **The answer is to distinguish the ESTATE a tray may show from the tray
EXISTING** - an unnamed estate is a reason to say "unnamed" on the icon, not a
reason to have no icon. That is the same argument §11 already makes about the
detached state, applied one level up.

## ⛔ The work-item rows are not legible to a human reviewer. Boris, 2026-09-17, with a screenshot.

**He sent the Projects-and-cases tab and said, of the work-item rows:**

> *"I'm not sure if what I'm seeing is the short descriptions in this GUI but
> they are not user friendly and clicking them doesn't show the full
> description. The information must be stored in a way that helps the human
> reviewers. I'm missing search/filter functionality. I'm sure they can be
> grouped or at least tagged so that the user can see what relates to what."*

⛔ **THIS IS FOUR REQUIREMENTS AND ONLY ONE OF THEM IS A GUI CHANGE.** Reading
it as "make the window nicer" loses three of them, and the one most likely to be
lost is the one that decides the others.

| # | What he asked for | Where it is answered |
|---|---|---|
| 1 | **a row that is readable, and a way to see the FULL text** | the window. A row is a truncated line today and clicking it does nothing |
| 2 | ⛔ **"The information must be STORED in a way that helps the human reviewers"** | **the STORE and the SEEDER, not the window.** §39's grain and what `rigseed` writes |
| 3 | **search and filter** | the window, over a store that can already answer - `record.query` has project, kind and a field predicate |
| 4 | **grouping or at least tagging, so a reader sees what relates to what** | the STORE first (`tags` is a §39 field nothing writes), the window second |

### ⛔ Requirement 2 is the one that binds the seeding, and it arrived before the re-seed

**He said it as an instruction about what to do when the work items are next
populated** - *"When you later populate the rig work-items just note that"* - so
it lands on the re-seed rather than after it. ⛔ **A re-seed that writes what
the current seeder writes has spent the one-way write and NOT met this.**

**What the store holds per work item today, measured 2026-09-17:** `title` and
`status`, and for the ranked rows a `rank`. **`description_short` is a §39 field
and nothing writes it. `tags` is a §39 field and nothing writes it. `priority`
and `owner` are §39 fields and nothing writes them** - which is B62's and B65's
measured six-of-eleven.

⛔ **SO THE ROW IN THE WINDOW IS A TRUNCATED `title` AND THERE IS NOTHING ELSE
TO SHOW.** The window cannot be fixed on its own: clicking a row can only reveal
a fuller description if one was stored, and rows can only be grouped if
something tagged them. **The GUI complaint is a STORAGE finding with a GUI
symptom**, and fixing the symptom first would produce a detail pane that renders
the same truncated line twice.

### What the legible rows do NOT license

⛔ **NOT a new grain.** §39's grain is Boris's own and was widened by him to
`#####` on 2026-09-17. This is about which FIELDS a record carries, not about
what counts as a record.

⛔ **THIS ROW USED TO READ "NOT INVENTING A DESCRIPTION" AND BORIS OVERTURNED IT
ON 2026-09-18.** Kept in full, because a reader who meets only the new rule
cannot tell which failure it was written against.

> **What it said:** *"A backlog row's prose is what it is; a `description_short`
> derived by summarising it would be a seat composing content, which is the
> thing the record exists to stop. The full text is what the document says and
> the short form is a deterministic cut of it, or it is absent and says so."*

**His words, verbatim:**

> *"As I said, AI agents are allowed to do anything - so `plan/11 forbids a seat
> composing a description` seems strange; AI agents can do anything they want as
> long as it is a real benefit and bring improvement to what I'm doing."*

⛔ **SO A SEAT MAY COMPOSE, AND THE TEST IS BENEFIT RATHER THAN PROVENANCE.**
*"As I said"* points at §42, which he had already ruled in the plainest words
available - *"I want the AI agents to be able to do anything in the system"*,
default open, *"absolutely without restrictions"*. **This row was a seat's
caution written as his requirement, and it contradicted a ruling already in the
specification.** Full reasoning in `DECISIONS.md`, 2026-09-18.

| | |
|---|---|
| **allowed** | writing a `description_short` that is genuinely clearer than a cut of the title. Rewriting a title for his reading. Filling any field rig offers |
| **the test he stated** | ⛔ **"a real benefit and bring improvement to what I'm doing"** - and he is the judge of it, so a seat that cannot say what the improvement IS has not met it |
| ⛔ **still refused, and it is a DIFFERENT rule** | **misrepresentation.** Inventing a quotation, changing what a requirement or a ruling SAYS, dropping a clause in a merge. That is the honesty rule this project already carries, and it was never about composing prose |
| **what a deterministic cut is still good for** | the fallback. **A row nobody has read yet gets the cut**, and a cut is honest; it is no longer the ceiling |

⛔ **NOT a tag vocabulary a seat picks.** Tagging *"so the user can see what
relates to what"* is answered first by what the documents ALREADY assert - the
section a row sits under, the `part-of` parent, the owner column - before any
new classification is invented.

⛔ **NARROWED BY BORIS 2026-09-18, AND THE NARROWING MATTERS BECAUSE A SEAT READ
THIS ROW AND SHIPPED AN UNREADABLE TAG UNDER IT.** The first tags this project
ever wrote were raw heading slugs - `section:b46-the-mvp-acceptance-test-and-it-
existed-in-no-document-at-all`, sixty-one characters in a filter chip - and the
seat that wrote them was obeying this row to the letter.

> *"The tags as well as the content and the titles must be useful and friendly,
> for example `B46` is none of them."*

**So the two halves come apart:** the tag's **SUBJECT** still comes from what the
document asserts, which is what this row was protecting. **Its WORDING is the
seat's to make readable**, and *"as long as it is a real benefit"* is the test he
gave the same day. **`mvp-acceptance` instead of the slug invents nothing.** Full
statement in §39.

## ⛔ The rail must land on the thing it names. Boris, 2026-09-17.

> *"The 'Open Projects and cases' way of navigation seems strange and not
> natural. I'm guessing the left button says 'Projects and cases' so I expect to
> see a tabbed display of projects and cases and not this strange partial
> dashboard or whatever it is... Basically I expect to see what I see after I
> click 'Open Projects and cases' with relevant information for each
> project/case."*

⛔ **A RAIL ENTRY IS A DESTINATION AND NOT A TEASER.** His principle stands
and is not in question. ⛔ **BUT THE MECHANISM THIS SECTION RECORDED WAS
WRONG, AND IT WAS FALSIFIED BY DEMONSTRATION ON 2026-09-17 BY GENERATION 17.**

**WHAT THIS ENTRY USED TO SAY, kept because deleting it hides the lesson:**
*"the rail entry called Projects and cases opens a summary card whose own
content is a count and a paragraph, carrying a LINK called Open Projects and
cases."* ⛔ **THAT IS NOT WHAT THE RAIL DOES.** `App.svelte`'s `pick()` sets
`atHome = false` and `selected = id`, so `internalGui()` resolves and
`ProjectCaseGui` renders. **Clicked in a real browser against the real built
bundle: the rail entry lands directly on the tabbed Projects/Cases display.**
The rail was never the defect.

⛔ **THE TEASER IS THE DASHBOARD, WHICH IS WHERE THE WINDOW LANDS ON OPEN.**
`Dashboard.svelte` carries a card titled *"rig: plan against execution"* holding
a waffle, the same verdict sentence the real view prints, and a button reading
*Open Projects and cases*. **That card is the "strange partial dashboard"** - a
partial duplicate of a view one click away, on the page he sees first and did
not choose.

| | |
|---|---|
| **what he clicks** | a rail entry named `Projects and cases` |
| **what he must get** | the projects and cases, tabbed, with each one's information |
| ✅ **what the rail actually does** | exactly that, and it always did |
| ⛔ **what he was looking at** | the DASHBOARD, the landing page, whose record card restates the real view and links to it |

⛔ **SO THE FIX MOVES: IT IS THE DASHBOARD'S CARD, NOT THE RAIL'S ROUTING.**
A seat that "fixed" the rail would have changed working code, shipped it, and
left his complaint standing - **and this section would have been its evidence.**
**The lesson is the one this project keeps re-learning: a seat described a
mechanism it had read and not run.** The page's own footer already conceded it -
*"What belongs on this page is still being decided."*

**THE TEST IS HIS SENTENCE:** *"I expect to see what I see after I click 'Open
Projects and cases'."* The second page's content is right; its POSITION is the
defect. **A page reachable only by clicking through a page that says nothing new
is a page the rail lied about.**

### What each project must open with, and it is three things

> *"Each project should start with the name of the project, the overall state
> something similar to what there is now, some description of the project to
> remind what it is about."*

| # | On the project | Where it comes from |
|---|---|---|
| 1 | **the NAME** | the container record's `title` |
| 2 | **the overall STATE** | ⛔ *"something similar to what there is now"* - the counts and the unstepped/unbuilt lines already rendered. **He is keeping this, not replacing it** |
| 3 | ⛔ **a DESCRIPTION reminding him what the project is about** | **nothing holds this today.** The project record carries `title` and `status` |

⛔ **ITEM 3 IS A STORAGE GAP AND NOT A LAYOUT ONE**, which is the same finding
the work-item rows produced an hour earlier in this section. **The window cannot
render a reminder nobody stored.**

### ⛔ Human-friendly fields are obligatory, and he ruled the fork

> *"The content should be user-friendly, the AI agent populating it should have
> no problem setting proper fields for that, in fact human-friendly fields are
> obligatory. I thought the existing ones can be more friendly and
> understandable but if it's not efficient then we can add dedicated fields."*

**This closes the question the previous entry left open.** The order is his:

1. ⛔ **TRY THE EXISTING FIELDS FIRST.** `description_short`, `tags`, `priority`
   and `owner` are already §39's and nothing writes them. *"I thought the
   existing ones can be more friendly and understandable."*
2. ✅ **ADDING DEDICATED FIELDS IS AUTHORISED** when the existing ones will not
   carry it. *"if it's not efficient then we can add dedicated fields."* **A
   seat does not need to come back and ask for this.**

⛔ **AND "OBLIGATORY" IS THE WORD THAT BINDS.** Human-friendly content is not a
quality bar a seat may trade against effort or against schema tidiness. **A
record a person cannot read is not a record that met this requirement**, however
well it serves a query.

⛔ **THE AGENT IS NAMED AS THE POPULATOR AND THAT IS PART OF THE REQUIREMENT:**
*"the AI agent populating it should have no problem setting proper fields for
that."* So the fields must be ones a writer can fill from what the source
DOCUMENT already says. **A field that can only be filled by composing new prose
is a field that will arrive empty or invented**, and both outcomes fail this.

## ⛔ Work items are classified, filterable, and the hatch is hard to read. Boris, 2026-09-17.

> *"The different work-items should probably be classified to task/bug/idea/...
> with appropriate icons. Should probably be able to filter by tags or aspects or
> by searching. [screenshot] these are not that great for the eyes to read and in
> general the layout of this display could be way much better."*

**Three things, and the third has a filed row with a measured answer already.**

### 1. A work item has a TYPE, and the type has an icon

⛔ **`kind` IS ALREADY TAKEN AND THIS IS NOT IT.** §39's `kind` separates a
work-item from a decision from a requirement. **What he is asking for is a
classification WITHIN work-item** - task, bug, idea - so it is a FIELD on the
record, not a new kind.

⛔ **AND IT IS NOT DERIVABLE FROM THE DOCUMENT TODAY.** `BACKLOG.md` says
nothing about whether a row is a bug or a task; the rows read as defects because
that is what this project has been filing, not because anything typed them.
**So this is his authorised "add dedicated fields" case**, and the writer is the
agent: *"the AI agent populating it should have no problem setting proper fields
for that."*

⛔ **A SEAT MUST NOT BACKFILL THE EXISTING ROWS BY GUESSING.** A type inferred
from a row's wording is a seat composing content, which this section already
refuses twice. **An untyped row says untyped**; new rows carry a type because
whoever writes them knows it.

### 2. Filter by tag or aspect, and search

**The store can already answer most of this and nothing asks it.** `record.query`
takes project, kind and a field predicate (B65), so filtering on `owner`,
`status` or a type field is a query the window does not make.

⛔ **SEARCH OVER THE BODY IS THE PART THAT IS NOT BUILT.** The field predicate is
an exact match on a field's value; nothing does substring or full-text over a
record's prose - which is now where a row's real content lives (rig `dd6d102`).
**B28 is the store-search row and it is open.**

### 3. ⛔ The hatched "not computed" ground is B71, and it is measured

**He is reading body text over a diagonal hatch.** This is **not a taste
judgement and must not be answered with one**: `BACKLOG.md` B71 already carries
the measurement, taken 2026-09-17 before he complained.

| | |
|---|---|
| **measured** | the hatched *not computed* ground puts its reason text at **3.73:1 dark / 3.71:1 light** |
| **what the gate said** | *"clean: every pass green in every theme"* |
| **why the gate missed it** | every pass reads `backgroundColor`; a hatch is a `background-image`, so the gate measures the colour UNDERNEATH it |
| **the answer, already computed** | **30% is the first alpha that clears both themes** - 5.16 dark / 5.01 light |

⛔ **SO THE FIX IS KNOWN AND THE ROW IS OPEN.** He has now hit it from the
outside, which raises it from a filed finding to a live complaint.

⛔ **AND "the layout of this display could be way much better" IS A SEPARATE
CLAIM FROM THE CONTRAST**, larger and not yet measured. The screenshot shows
seven consecutive full-width cards, five of which say the same two sentences
about the git projection. **A panel that repeats one reason five times is
spending a screen to say one thing** - that is a grouping defect, not a colour
one, and the two must not be closed together.

## ⛔ The redundant scrollbars go. Boris, 2026-09-17, with an annotated screenshot.

> *"These scrolls are unnecessary."*

**He arrowed the stacked scrollbar arrows at the top right of the projects
tab** - two scroll affordances one above the other, in a pane whose content
did not need either of them.

⛔ **A SCROLLBAR IS A CLAIM THAT THERE IS MORE TO SEE, AND A FALSE ONE IS
WORSE THAN NO AFFORDANCE AT ALL.** A nested scroll container also steals the
wheel: a reader scrolling the page stops dead when the pointer crosses it, which
is the specific thing that makes a window feel wrong without a person being able
to name why.

⛔ **ONE SCROLL OWNER PER PANE.** The `browsable-page` rule this project
already follows says who owns the scroll must be decided rather than inherited.
A tab strip holding ONE tab must not be a scroll container; a list inside a page
that already scrolls must not open a second one.

**What this does NOT license:** removing a scrollbar that is doing real work by
clipping content. **The fix is to stop creating the container, never to hide its
bar with `overflow: hidden` or a styled-away scrollbar** - a hidden scrollbar on
a real overflow is content a reader cannot reach and no longer knows is there.

## ⛔ A management panel, and redeployment must be trivial and automatic. Boris, 2026-09-17.

> *"Should have management panel to do things like `make install` in the proper
> place with ease. And `make install-window`. You can click these buttons
> yourself too and perhaps it would bypass different strange limitations you
> have. Redeployment should be trivial and automatic."*

⛔ **THIS IS THE ANSWER TO A PROBLEM THAT HAS RIDDEN FIVE GENERATIONS OF HANDOVER
DOCUMENTS**, and every one of them recorded it as a wall rather than as a
requirement: *"`make install` is denied to a seat; three denials is an answer."*
**He has now said what to build instead of asking again.**

### The requirement, in four parts

| # | What he asked for |
|---|---|
| 1 | **a MANAGEMENT PANEL in the window** - deployment actions in the place he already looks, rather than in a terminal he has to find |
| 2 | **`make install` and `make install-window` are the first two**, named |
| 3 | **a seat can press them too** |
| 4 | ⛔ **"Redeployment should be trivial and automatic"** - which is a bar on the WHOLE act, not a request for two buttons |

### ⛔ Part 4 outranks parts 1-3 and must not be answered with them

**Two buttons is the SMALL reading.** *"Trivial and automatic"* says the thing
that should not require a decision is the redeploy itself. B89 is the measured
case: `make install` reported success while the window it did not touch stayed
thirteen hours stale, **and nobody noticed for a day.** A panel with two buttons
on it would have had the same outcome - somebody has to know to press the second.

⛔ **SO THE PANEL OWES A STATE BEFORE IT OWES A BUTTON:** what is deployed, what
is built, and whether they are the same thing. **A redeploy control that cannot
say what is currently running is a button that reports success over B89 again.**

### ⛔ Whether a seat may press it is rig's capability model, not a workaround

**He is right about the effect and the mechanism must be built as the feature it
is.** §13 and §42 already govern what an agent may do, and a deploy control is
exactly the kind of declared, effectful capability they exist for: it is
`EffectsDestructive` or close to it, it replaces a live deployment, and §28
already spends thirty lines on why a stale daemon left running is a failed
termination.

⛔ **SO IT IS DECLARED, ATTRIBUTED AND LOGGED LIKE EVERY OTHER EFFECTFUL VERB** -
who pressed it, from which seat, at which epoch - and **the policy of whether a
seat may is HIS to set**, which is what §42's *"absolutely without restrictions"*
default already says. **Building it as a way around a harness gate rather than as
a capability is the version that goes wrong**: an unattributed shell-out from a
GUI is the one shape §13 exists to refuse, and it would also be a worse product.

### What the management panel does NOT license

⛔ **NOT arbitrary shell from the window.** The panel runs rig's OWN declared
deployment actions. A text box that runs what is typed into it is a different
product and a much worse one.

⛔ **NOT automatic deployment of unreviewed work.** *"Automatic"* is his word
about the ACT being trivial once decided, in a message whose whole subject is
making a deploy easy to perform. **Nothing here says a commit deploys itself**,
and a seat reading it that way has widened a convenience into a policy he has not
stated. If he wants that, he will say so.

## ⛔ After the gaps, the window is where rig starts paying. Boris, 2026-09-18.

**HIS WORDS, AND THEY SET THE NEXT PHASE RATHER THAN A FEATURE:**

> *"Once all gaps are closed we shall make use of the tokens left to plan and
> implement the best possible user-experience for GUI interaction with the
> existing data we have for the `rig` project and in general for all projects
> we'll import later into `rig` so that it starts paying for its long
> development."*

### What this rules, and it is three things

| # | What he ruled | What it forbids |
|---|---|---|
| 1 | **IT STARTS WHEN THE GAPS CLOSE, NOT BEFORE.** The budget named is *"the tokens left"* | a seat opening GUI work while a gap is open. His 2026-09-17 ruling - *"Don't spend too much time on GUI, lets get the functionality first to its finish line"* - is NARROWED here, not reversed: it expires at the last gap |
| 2 | **IT IS OVER THE DATA THAT ALREADY EXISTS**, rig's own project first | a design demonstrated on a fixture. The store holds his real backlog, his real decisions and 380 requirement records, and the window's job is to make THOSE usable |
| 3 | **AND IT GENERALISES TO EVERY PROJECT IMPORTED LATER** | a surface built around rig's own shape alone. §41 is the import; this is what the imported thing must be worth looking at through |

⛔ **"SO THAT IT STARTS PAYING FOR ITS LONG DEVELOPMENT" IS THE ACCEPTANCE
SENTENCE, AND IT IS A HUMAN JUDGEMENT.** Not a feature list and not a metric a
seat may invent: **the test is whether HE reaches for the window instead of the
documents.** The MVP bar (§39) is the same shape - *"being able to use rig to
work on rig"* - and this is that bar moved from the CLI to the window.

**PLANNING IS PART OF THE INSTRUCTION.** *"plan and implement"*, in that order,
so the first act is a design put to him rather than a screen built.

---
