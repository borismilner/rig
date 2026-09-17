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
): Item => ({
  ...unstepped(id, title),
  descriptionShort: short,
  owner,
  tags,
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
    ),
    described(
      "B10",
      "`--json` converges by the DAEMON rendering the bytes, not by the client importing a renderer",
      "`--json` converges by the DAEMON rendering the bytes, not by the...",
      "backend-record",
      ["wire", "cli"],
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
