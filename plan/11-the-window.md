## 11. The window

The UI shell inside rig. Its visual system is a separate piece of work (§23 M1a) because for a
program whose whole job is presenting other programs, the visual design *is* the product. It is
built and measured: `design/visual-system.html`, engine at `design/theme.js`.

- **One window.** A left rail of registered programs, a context bar, a pane, a status strip.
  Chrome budget is a number that gets defended, not a feeling.
- **One tray icon** for the whole estate, replacing the six that exist today. Per-program status,
  badge, and commands runnable with no window open. A stopped program is still listed, with the
  reason and a Start entry.
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
