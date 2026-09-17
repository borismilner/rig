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
