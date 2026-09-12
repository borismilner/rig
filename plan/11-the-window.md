## 11. The window

The UI shell inside rig. Its visual system is a separate piece of work (§23 M1a) because for a
program whose whole job is presenting other programs, the visual design *is* the product. It is
built and measured: `design/visual-system.html`, engine at `design/theme.js`.

- **One window.** A left rail of registered programs, a context bar, a pane, a status strip.
  Chrome budget is a number that gets defended, not a feeling.
- **One tray icon** for the whole estate, replacing the six that exist today. Per-program status,
  badge, and commands runnable with no window open. A stopped program is still listed, with the
  reason and a Start entry.
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
  - **An UNNAMED estate gets no tray at all.** Every test and every reproduction
    recipe starts one, they are not deployments (§37's ephemeral clause), and a
    third icon appearing during `make ci` would be the failure this requirement
    is trying to prevent.
- **Three pane tiers**, and the middle one exists because the outer two leave a gap.
  *Generated*: the program declared a schema and rig renders forms, tables, actions, progress,
  detail and status - and the program never names a widget (§5h). *Kit*: the program serves its
  own HTML and composes it from rig's own elements. *Embedded*: the program serves its own HTML
  and brings its own components, getting the token set and nothing more.
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
