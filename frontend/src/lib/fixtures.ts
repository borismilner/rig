/* Measurement fixtures, and none of them is a mock of the product path.
 *
 * The contrast gate is the whole reason they exist. A browser cannot reach the
 * Wails runtime, so an audited page has an empty rail and no panel content,
 * and every pass measures zero - which this project has already shipped once
 * as a clean run (Makefile's `contrast` comment, and the focus ring that went
 * out at 2.17:1 while the gate was down). These seed the real components with
 * real-shaped data so the real CSS is on screen to be read.
 *
 * ⛔ BRIEF, SPEC AND DECISIONS LEFT WITH THE VIEWS THEY SEEDED, at plan/50
 * move 7. They are in docket's own frontend now, beside the components that
 * read them. What is left here is the platform's: the rail, what is deployed,
 * and this build's stamps.
 */

import type {
  Capability,
  Card,
  CardList,
  Guidelines,
  Note,
  NoteList,
  Program,
  Running,
} from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/models.js";
export const PROGRAMS: Program[] = [
  {
    id: "shelf",
    name: "shelf",
    version: "v1.4.0",
    icon: "library",
    description: "the library",
    coverage: "full",
    coverageNote: "3 of 20 commands declared",
    services: ["storage", "logs"],
    hosted: false,
    commands: 3,
    paneUrl: "",
    load: "resident",
    atRest: false,
    down: false,
    stale: false,
    events: ["shelf.admitted"],
    commandList: [
      {
        id: "search",
        title: "Search",
        summary: "Find items in the library by text",
        description: "",
        effects: "read-only",
        returns: "matching items",
        args: "",
      },
      {
        id: "admit",
        title: "Admit",
        summary: "Add a file to the library",
        description: "Copies the file in and indexes it.",
        effects: "writes-files",
        returns: "",
        args: "",
      },
      {
        id: "reindex",
        title: "Reindex",
        summary: "Rebuild the search index",
        description: "",
        effects: "writes-files",
        returns: "",
        args: "",
      },
    ],
  },
  {
    id: "graft",
    name: "graft",
    version: "v0.9.1",
    icon: "workflow",
    description: "the grafter",
    coverage: "partial",
    coverageNote: "",
    services: [],
    hosted: false,
    commands: 7,
    paneUrl: "",
    load: "on call",
    atRest: true,
    down: false,
    stale: false,
    events: [],
    commandList: [
      {
        id: "apply",
        title: "Apply",
        summary: "Graft a change onto a branch",
        description: "",
        effects: "destructive",
        returns: "",
        args: "",
      },
    ],
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
    load: "resident",
    atRest: false,
    down: true,
    stale: false,
    events: [],
    commandList: [
      {
        id: "shot",
        title: "Shot",
        summary: "Capture the screen",
        description: "",
        effects: "drives-input",
        returns: "a PNG path",
        args: "",
      },
    ],
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
    load: "",
    atRest: false,
    down: false,
    stale: true,
    events: [],
    commandList: [
      {
        id: "dig",
        title: "Dig",
        summary: "",
        description: "",
        effects: "",
        returns: "",
        args: "",
      },
      {
        id: "sift",
        title: "Sift",
        summary: "",
        description: "",
        effects: "read-only",
        returns: "",
        args: "",
      },
    ],
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

/* Supervision for the fixture estate: one of each state the Programs table
 * colours, and one parked question so Needs you has a row to measure. */
export const RUNNING: Running[] = [
  {
    id: "shelf",
    state: "healthy",
    since: Date.now() - 7_380_000,
    restarts: 0,
    waiting: "",
    parked: "",
    lastExit: "",
  },
  {
    id: "snapper",
    state: "quarantined",
    since: Date.now() - 600_000,
    restarts: 5,
    waiting: "",
    parked: "Its display is gone. Restart it on the new one?",
    lastExit: "exit status 2",
  },
];

/* Capabilities for the gate: one of each effect the panel colours, a schema
 * with each control kind, and one with no arguments. The words are rig's
 * own, copied from a live tools/list on 2026-10-03. */
export const CAPABILITIES: Capability[] = [
  {
    owner: "rig",
    id: "health",
    title: "Health",
    summary: "Each supervised program's state.",
    description:
      "Each supervised program's state. Answers the state, since when, restarts and what it last reported waiting on.",
    returns: "",
    effects: "read-only",
    args: '{"type":"object","properties":{"programs":{"type":"array","items":{"type":"string"},"description":"ids; empty is all"}}}',
  },
  {
    owner: "rig",
    id: "notify",
    title: "Notify",
    summary: "Shows a toast on the desktop.",
    description: "Shows a toast on the desktop, and files it in the record.",
    returns: "",
    effects: "changes state",
    args: '{"type":"object","required":["title"],"properties":{"title":{"type":"string"},"severity":{"type":"string","enum":["info","success","warning","error"]},"sticky":{"type":"boolean"},"timeoutMs":{"type":"integer","description":"0 keeps the default"}}}',
  },
  {
    owner: "rig",
    id: "down",
    title: "Down",
    summary: "Stops rigd.",
    description: "Stops rigd and every program it supervises.",
    returns: "",
    effects: "destructive",
    args: '{"type":"object","properties":{}}',
  },
  {
    owner: "rig",
    id: "list_agents",
    title: "",
    summary: "Who is on the roster.",
    description: "Who is on the roster, and what each says it is for.",
    returns: "",
    effects: "",
    args: '{"type":"object","properties":{}}',
  },
  {
    owner: "shelf",
    id: "search",
    title: "Search",
    summary: "Find items in the library by text",
    description: "",
    returns: "matching items",
    effects: "read-only",
    args: '{"type":"object","required":["q"],"properties":{"q":{"type":"string"},"limit":{"type":"integer"}}}',
  },
];

/* rig.guidelines, seeded so the gate measures every standing: current, newer
 * with an uncommitted build, and unknown (plan/55 requirement 29). */
const rule = (
  id: string,
  date: string,
  who: string,
  built: boolean,
  title: string,
) => ({
  id,
  date,
  who,
  built,
  title,
  body: `${title}, in full.`,
  cite: "plan/55",
});
export const GUIDELINES: Guidelines = {
  revision: "2026-10-03",
  rules: [
    rule(
      "G6",
      "2026-10-03",
      "programs",
      false,
      "Declare your settings as a schema",
    ),
    rule("G4", "2026-10-03", "programs", true, "Declare how you load"),
    rule(
      "G3",
      "2026-10-01",
      "programs",
      true,
      "Declare the events you publish",
    ),
    rule("A2", "2026-09-24", "agents", true, "Keep your notes in rig"),
  ],
  programs: [
    {
      program: "shelf",
      day: "2026-10-03",
      commitTime: "2026-10-03T09:12:00Z",
      modified: false,
      unknownBecause: "",
    },
    {
      program: "graft",
      day: "2026-09-28",
      commitTime: "2026-09-28T17:40:00Z",
      modified: true,
      unknownBecause: "",
    },
    {
      program: "snapper",
      day: "",
      commitTime: "",
      modified: false,
      unknownBecause: "its binary carries no Go build info",
    },
    {
      program: "quarry",
      day: "2026-10-02",
      commitTime: "2026-10-02T08:00:00Z",
      modified: false,
      unknownBecause: "",
    },
  ],
};

/* The notifications panel, Needs you and the card (plan/55 requirements 7
 * to 9, 23 to 26): one ask with options, one free-text ask, one answered,
 * one rig forgot, and plain infos from three senders. NOTES_NOW is the
 * clock the panel reads them against, so "ago" holds still. */
export const NOTES_NOW = Date.parse("2026-10-03T18:00:00Z");
const at = (min: number) => new Date(NOTES_NOW - min * 60_000).toISOString();
const note = (
  n: Partial<Note> & Pick<Note, "id" | "title" | "sender">,
): Note => ({
  severity: "info",
  body: "",
  at: at(5),
  suppressed: false,
  replies: [],
  replyText: false,
  asks: false,
  waiting: false,
  forgotten: false,
  answer: null,
  ...n,
});
export const NOTES: NoteList = {
  keepDays: 7,
  keepFrom: "default",
  evicted: 3,
  notes: [
    note({
      id: "n-deploy",
      severity: "warning",
      title: "Deploy rigd 20.16 now?",
      body: "The build passed and production is two commits behind. Deploying restarts rigd; supervised programs come back on their own.",
      sender: "lead",
      at: at(2),
      replies: ["Deploy", "Later"],
      asks: true,
      waiting: true,
    }),
    note({
      id: "n-shelf",
      title: "shelf finished indexing",
      body: "1,204 files, 3 skipped as unreadable.",
      sender: "shelf",
      at: at(9),
      severity: "success",
    }),
    note({
      id: "n-name",
      title: "What should the new estate be called?",
      body: "docket needs a name before it can file the first case.",
      sender: "docket",
      at: at(14),
      replyText: true,
      asks: true,
      waiting: true,
    }),
    note({
      id: "n-graft",
      severity: "error",
      title: "graft quarantined after 5 restarts",
      body: "It exited with status 2 five times in a minute. Its log has the reason.",
      sender: "rig",
      at: at(40),
    }),
    note({
      id: "n-answered",
      title: "Run the nightly backup now?",
      sender: "lead",
      at: at(95),
      replies: ["Run it", "Skip tonight"],
      asks: true,
      answer: {
        reply: "Run it",
        text: "",
        dismissed: false,
        by: "bubble",
        at: at(93),
      },
    }),
    note({
      id: "n-forgot",
      title: "Keep the old socket path?",
      sender: "docket",
      at: at(60 * 26),
      replies: ["Keep", "Move"],
      asks: true,
      forgotten: true,
    }),
    note({
      id: "n-dnd",
      title: "shelf is low on space",
      body: "2.1 GB left on the index volume.",
      sender: "shelf",
      at: at(60 * 50),
      severity: "warning",
      suppressed: true,
    }),
  ],
};

/* ?board=1: the board, seeded, on the notes' fixed clock. Three sources,
 * every severity, progress, busy, facts, buttons and two closed cards, so
 * every part of a card is on screen to be measured. */
const minsAgo = (min: number) => new Date(NOTES_NOW - min * 60_000).toISOString();
const card = (
  c: Partial<Card> & Pick<Card, "id" | "from" | "title">,
): Card => ({
  version: 1,
  project: "",
  status: "",
  severity: "info",
  body: "",
  progress: 0,
  hasProgress: false,
  busy: false,
  facts: [],
  actions: [],
  closed: false,
  created: minsAgo(30),
  updated: minsAgo(2),
  ...c,
});
export const BOARD: CardList = {
  omitted: 0,
  missing: false,
  cards: [
    card({
      id: "b-graft",
      from: "graft",
      project: "rig",
      title: "Grafting the board into rig",
      status: "running",
      progress: 0.62,
      hasProgress: true,
      version: 4,
      updated: minsAgo(1),
      facts: [
        { label: "jobs", value: "5 of 8" },
        { label: "host", value: "lab-2" },
      ],
      actions: ["Stop"],
    }),
    card({
      id: "b-deploy",
      from: "graft",
      project: "rig",
      title: "Deploy to production",
      status: "waiting for you",
      severity: "warning",
      updated: minsAgo(4),
      body: "Two commits behind. The tray, the board and the CLI change.",
      actions: ["Deploy", "Later"],
    }),
    card({
      id: "b-tests",
      from: "lead",
      project: "rig",
      title: "Full test run",
      status: "failed",
      severity: "error",
      version: 2,
      updated: minsAgo(6),
      facts: [{ label: "failed", value: "TestTheBinary/help-long" }],
      actions: ["Retry"],
    }),
    card({
      id: "b-index",
      from: "shelf",
      project: "library",
      title: "Re-indexing the library",
      status: "busy",
      busy: true,
      updated: minsAgo(8),
    }),
    card({
      id: "b-done",
      from: "lead",
      project: "rig",
      title: "Slice 2 shipped",
      status: "done",
      severity: "success",
      closed: true,
      version: 3,
      updated: minsAgo(40),
    }),
    card({
      id: "b-old",
      from: "shelf",
      project: "library",
      title: "Nightly backup",
      status: "done",
      severity: "success",
      closed: true,
      updated: minsAgo(300),
    }),
  ],
};
