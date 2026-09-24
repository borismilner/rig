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

import type { Program } from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";
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
