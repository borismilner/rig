/* Measurement fixtures, and none of them is a mock of the product path.
 *
 * The contrast gate is the whole reason they exist. A browser cannot reach the
 * Wails runtime, so an audited page has an empty rail, no brief and no tab
 * content, and every pass measures zero - which this project has already
 * shipped once as a clean run (Makefile's `contrast` comment, and the focus
 * ring that went out at 2.17:1 while the gate was down). These seed the real
 * components with real-shaped data so the real CSS is on screen to be read.
 *
 * ⛔ THE BRIEF FIXTURE IS THE LIVE PRODUCTION ANSWER, NOT A PRETTY ONE. 59
 * items, every one unstepped, five sections computed and six dark. Seeding a
 * healthier project would measure colours the product does not currently
 * produce, and the uniform column is exactly the thing section 11 requirement
 * 15 says must be rendered rather than smoothed over.
 */

import type {
  Brief,
  Item,
  Program,
  RecordRow,
} from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";

export const PROGRAMS: Program[] = [
  {
    id: "shelf",
    name: "shelf",
    version: "v1.4.0",
    icon: "sh",
    description: "the library",
    coverage: "full",
    coverageNote: "3 of 20 commands declared",
    services: ["storage", "logs"],
    hosted: false,
    commands: 3,
    paneUrl: "",
  },
  {
    id: "graft",
    name: "graft",
    version: "v0.9.1",
    icon: "gr",
    description: "the grafter",
    coverage: "partial",
    coverageNote: "",
    services: [],
    hosted: false,
    commands: 7,
    paneUrl: "",
  },
  {
    id: "snapper",
    name: "snapper",
    version: "v2.0.0",
    icon: "sn",
    description: "screen capture",
    coverage: "partial",
    coverageNote: "",
    services: [],
    hosted: true,
    commands: 1,
    paneUrl: "",
  },
  // The pane fixture's program, and the port is DEAD on purpose.
  //
  // Nothing is listening on 7399 and nothing should be: onload never fires, so
  // the unserved state renders, and the gate measures its real colours with no
  // background server for the gate to depend on. A live port would make this
  // measurement flaky by construction.
  {
    id: "quarry",
    name: "quarry",
    version: "v0.2.0",
    icon: "qu",
    description: "declares a pane and serves nothing",
    coverage: "partial",
    coverageNote: "",
    services: [],
    hosted: false,
    commands: 2,
    paneUrl: "http://127.0.0.1:7399/",
  },
];

/* ⛔ THE DEPLOYMENT FIXTURE IS THE SKEW STATE, AND THAT IS DELIBERATE.
 *
 * It carries B89's two REAL version strings - the daemon Boris installed at
 * 21:17 and the window that stayed thirteen hours behind it. The skew state is
 * the only loud one on the panel (a rust border and a bolder line), so it is
 * the state the contrast gate most needs on screen. A fixture showing the calm
 * "they agree" case would leave the loud one unmeasured, which is this
 * project's own named defect: a check that cannot fail reads as a pass.
 */
export const DEPLOYMENT = {
  windowVersion: "v0.0.0-m0-409-g57c99ca-dirty",
  windowCommit: "57c99ca-dirty",
  windowBuilt: "2026-09-17T08:07:24Z",
  daemonVersion: "v0.0.0-m0-471-g8c4d195",
  daemonWire: "v1",
  windowWire: "v1",
  epoch: 21,
  reached: true,
  agree: false,
  verdict:
    "THE WINDOW AND THE DAEMON ARE DIFFERENT BUILDS. `make install` does not " +
    "install the window: run `make install-window` and restart the tray.",
};

export const BUILD: Record<string, string> = {
  rig: "v0.0.0-m0-388-gfixture",
  wire: "v1",
  schema: "v1",
  built: "fixture",
};

const unstepped = (id: string, title: string): Item => ({
  id,
  title,
  state: "not stepped",
  sinceUnixNano: 0,
  note: "",
  itemType: "",
  descriptionShort: "",
  priority: "",
  status: "",
  owner: "",
  tags: [],
  targetDate: "",
  semver: "",
});

/* ⛔ ONE ROW WITH ITS RECORD FILLED, AND ONE WITHOUT, BECAUSE BOTH STATES
 * SHIP AND BOTH MUST BE MEASURED.
 *
 * A row that opens and a row that cannot are different renders - different
 * cursor, different affordance, a whole detail panel with its own colours -
 * and a fixture carrying only bare rows would leave the opened panel
 * unmeasured by the contrast gate. That is this project's named defect: a
 * check that cannot fail reads as a pass.
 *
 * The values are the shape the real store holds after rig dd6d102, not invented
 * prose: description_short is a deterministic cut of the row's own words, owner
 * is read from the Seat/Adopter column, and tags are the section a row sits
 * under. Nothing here is a summary a seat composed. */
const described = (
  id: string,
  title: string,
  short: string,
  owner: string,
  tags: string[],
  itemType = "",
): Item => ({
  ...unstepped(id, title),
  descriptionShort: short,
  owner,
  tags,
  itemType,
  status: "active",
  priority: "high",
});

// Five real next-up titles and fifty-four real open ones would cost more than
// they measure; the gate reads colour and size, and it reads them off the same
// components either way. The COUNTS are the live ones, because the waffle's
// wrap behaviour at 59 cells is a layout the gate should see.
const filler = (n: number, prefix: string, from: number) =>
  Array.from({ length: n }, (_, i) =>
    unstepped(
      `${prefix}${from + i}`,
      i % 3 === 0
        ? "A work item whose title runs long enough to need the list's own truncation, because a short title measures a case the product does not have"
        : `Work item ${prefix}${from + i}`,
    ),
  );

export const BRIEF: Brief = {
  project: "rig",
  kind: "project",
  title: "rig",
  status: "active",
  semver: "",
  nextUp: [
    described(
      "B1",
      "`confirms` is ruled and unbuilt, so rig tells a user something false today",
      "`confirms` is ruled and unbuilt, so rig tells a user something...",
      "team-lead",
      ["wire", "honesty"],
      "bug",
    ),
    described(
      "B10",
      "`--json` converges by the DAEMON rendering the bytes, not by the client importing a renderer",
      "`--json` converges by the DAEMON rendering the bytes, not by the...",
      "backend-record",
      ["wire", "cli"],
      // ⛔ A TYPE THE WINDOW DOES NOT KNOW, ON PURPOSE. His set ends in "/..."
      // and the open-set behaviour ships: this must render as its own word
      // rather than be refused or folded into a default. It is also what the
      // contrast gate then measures.
      "spike",
    ),
    unstepped(
      "B12",
      "A decided item's decision does not reach `DECISIONS.md` on its own, and B10 is the proof",
    ),
    unstepped(
      "B13",
      "A defect reached a live estate that eighteen green packages could not see",
    ),
    unstepped(
      "B14",
      "A claimed key's VALUE has no freshness of its own, and CAS makes it look current",
    ),
  ],
  open: filler(54, "B", 15),
  blocked: [],
  cycles: [],
  notes: [],
  caseNotes: [],
  coarseCitations: 0,
  health: {
    projectionBehindCommits: 0,
    pendingEntries: 0,
    localOnly: false,
    localOnlyReason: "",
  },
  sections: [
    { section: "OPEN", state: "computed", reason: "" },
    { section: "NEXT_UP", state: "computed", reason: "" },
    { section: "NOTES", state: "computed", reason: "" },
    { section: "BLOCKED", state: "computed", reason: "" },
    {
      section: "DRIFT",
      state: "not computed",
      reason:
        "the standards register does not exist. standard.stamp and standard.drift are slice 7 and are deliberately off this wire, so nothing can be behind a standard rig cannot yet hold",
    },
    {
      section: "MUST_READ",
      state: "not computed",
      reason:
        "neither half of the must-read gate is built: the SET is records marked in the store, and the MARK is per-session state keyed on the session Token. Until they exist an empty must_read set means UNKNOWN, never “this project requires nothing”",
    },
    {
      section: "PROJECTION_BEHIND",
      state: "not computed",
      reason:
        "the git projection does not exist yet, so there is nothing to be behind",
    },
    {
      section: "PENDING",
      state: "not computed",
      reason:
        "the git projection does not exist yet, so there is nothing to be behind",
    },
    {
      section: "LOCAL_ONLY",
      state: "not computed",
      reason:
        "the git projection does not exist yet, so there is nothing to be behind",
    },
    { section: "FEATURES", state: "computed", reason: "" },
    {
      section: "CASE_NOTES",
      state: "not computed",
      reason:
        "the record store's brief derivation does not collect this kind yet. BACKLOG B46g",
    },
  ],
} as Brief;

/* ── the record views ─────────────────────────────────────────────────────
 *
 * ⛔ SHAPED FROM THE LIVE STORE, NOT INVENTED. Every structural case below was
 * measured on his production store on 2026-09-18 and each one changes what the
 * gate can see: a section head at level 2 with nested requirements under it
 * (387 requirements over 42 sections), a body holding a markdown TABLE (the
 * wrap that stops a 120-character row pushing a horizontal scrollbar onto the
 * whole panel), a retracted row (shown and struck, never dropped), a dated
 * decision and an undated one (91 of 533 carry no date anywhere).
 *
 * The prose is real prose at real length, because the gate reads colour off
 * rendered text and a three-word body measures a page the product never draws.
 */

const rec = (r: Partial<RecordRow>): RecordRow =>
  ({
    id: "",
    version: 1,
    kind: "requirement",
    title: "",
    section: "",
    sectionTitle: "",
    level: 3,
    date: "",
    body: "",
    source: "",
    line: "",
    tags: "",
    retracted: false,
    ...r,
  }) as RecordRow;

export const SPEC: RecordRow[] = [
  rec({
    id: "11/the-window",
    title: "11. The window",
    section: "11",
    sectionTitle: "11. The window",
    level: 2,
    source: "plan/11-the-window.md",
    line: "1",
    tags: '["section:window"]',
    body:
      "The window is rig's own surface, and it is one window rather than one " +
      "per program. What it draws is what rig holds; what it cannot reach it " +
      "says it cannot reach.",
  }),
  rec({
    id: "11/the-window/never-stale",
    title: "THE WINDOW MAY NEVER BE STALE",
    section: "11",
    sectionTitle: "11. The window",
    level: 3,
    source: "plan/11-the-window.md",
    line: "88",
    tags: '["section:window","rule"]',
    body:
      "Boris, 2026-09-18: \"You must make sure the GUI is never stale.\" Three " +
      "parts, and a window that is merely rebuilt satisfies none of them:\n\n" +
      "| part | what it means | built |\n" +
      "|---|---|---|\n" +
      "| 1 | a redeployment replaces the running window | yes |\n" +
      "| 2 | the window detects that its own binary has moved | no |\n" +
      "| 3 | the data on screen carries the time it was read | partly |\n",
  }),
  rec({
    id: "11/the-window/never-stale/what-it-costs",
    title: "What a stale window costs, measured twice",
    section: "11",
    sectionTitle: "11. The window",
    level: 4,
    source: "plan/11-the-window.md",
    line: "104",
    tags: '["section:window"]',
    body:
      "He caught a six-hour-old window on a deleted inode on 2026-09-18, and " +
      "the same class of fault the day before. Both times the pixels were " +
      "right and the binary behind them was gone.",
  }),
  rec({
    id: "12/toasts",
    title: "12. Toasts",
    section: "12",
    sectionTitle: "12. Toasts",
    level: 2,
    source: "plan/12-toasts.md",
    line: "1",
    tags: '["section:toasts"]',
    body:
      "A toast is how rig says a thing happened that the human did not ask " +
      "about. It is never how rig asks a question.",
  }),
  rec({
    id: "12/toasts/one-channel",
    title: "One channel, and the desktop owns it",
    section: "12",
    sectionTitle: "12. Toasts",
    level: 3,
    retracted: true,
    source: "plan/12-toasts.md",
    line: "31",
    tags: '["section:toasts"]',
    body:
      "Superseded: the tray carries its own notifications now, so this is no " +
      "longer the only channel and the paragraph claiming it was is withdrawn.",
  }),
];

export const DECISIONS: RecordRow[] = [
  rec({
    id: "2026-09-18-the-window-reads-the-store",
    kind: "decision",
    title: "2026-09-18 - the window reads the store, not just its brief",
    level: 2,
    date: "2026-09-18",
    source: "logbook/projects/rig/DECISIONS.md",
    line: "8121",
    body:
      "920 of the store's 1,057 current records had no screen. One call - " +
      "record.query, which already answered - closes it, and it needs no new " +
      "verb, no schema change and no daemon work.",
  }),
  rec({
    id: "2026-09-18-the-window-reads-the-store/why-not-the-brief-store",
    kind: "decision",
    title: "Why the record list does not go through rigstore",
    level: 3,
    date: "2026-09-18",
    source: "logbook/projects/rig/DECISIONS.md",
    line: "8140",
    body:
      "rigstore holds one brief per project under a read-once contract. A " +
      "record list is keyed by project AND kind, so putting it there either " +
      "widens that contract or caches a second thing under the first one's " +
      "rules.",
  }),
  rec({
    id: "2026-09-17-the-fence-rule-is-one-type-and-four-callers",
    kind: "decision",
    title: "2026-09-17 - the fence rule is one type and four callers",
    level: 2,
    date: "2026-09-17",
    source: "logbook/projects/rig/DECISIONS.md",
    line: "7902",
    body:
      "Four call sites had each grown their own copy of the fence check. The " +
      "backport found zero behavioural hits over 47 documents and the sets " +
      "before and after are byte-identical.",
  }),
  rec({
    id: "a-constant-dressed-as-a-live-field-is-a-lie-by-shape",
    kind: "decision",
    title: "A constant dressed as a live field is a lie by shape",
    level: 2,
    source: "logbook/projects/rig/DECISIONS.md",
    line: "3310",
    body:
      "A field that never varies but is rendered as though it might teaches a " +
      "reader to trust the next one that does vary. There are three partial " +
      "instances of this in the window and each is named where it lives.",
  }),
];
