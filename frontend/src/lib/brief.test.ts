/* The derivations behind the planning-versus-execution tab.
 *
 * WHY THESE AND NOT THE COMPONENTS. Every assertion below is a claim the tab
 * makes in words or in a count, and this project has already shipped a caption
 * that was a claim no gate checked. The rendering is demonstrated in the real
 * window; the arithmetic is demonstrated here, because it is the half that can
 * be wrong while the page still looks right.
 */

import { describe, it, expect } from "vitest";
import {
  SECTION_ORDER,
  splitByKind,
  NOT_STEPPED,
  sectionView,
  sectionViews,
  sectionTally,
  stepTone,
  tally,
  planVsExec,
  verdict,
  blindSpots,
  age,
} from "./brief";

const item = (id: string, state: string) => ({
  id,
  title: id,
  state,
  sinceUnixNano: 0,
  note: "",
});

// The live production answer on 2026-09-17, trimmed to three items a side.
// Five sections computed, six dark, which is what the daemon really returns.
const live = {
  project: "rig",
  kind: "project",
  title: "rig",
  status: "active",
  semver: "",
  nextUp: [item("B1", NOT_STEPPED), item("B10", NOT_STEPPED)],
  open: [item("B15", NOT_STEPPED), item("B16", NOT_STEPPED)],
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
      reason: "the standards register does not exist",
    },
    {
      section: "MUST_READ",
      state: "not computed",
      reason: "neither half of the must-read gate is built",
    },
    {
      section: "PROJECTION_BEHIND",
      state: "not computed",
      reason: "the git projection does not exist yet",
    },
    {
      section: "PENDING",
      state: "not computed",
      reason: "the git projection does not exist yet",
    },
    {
      section: "LOCAL_ONLY",
      state: "not computed",
      reason: "the git projection does not exist yet",
    },
    { section: "FEATURES", state: "computed", reason: "" },
    {
      section: "CASE_NOTES",
      state: "not computed",
      reason: "the record store's brief derivation does not collect this kind",
    },
  ],
} as any;

describe("section states", () => {
  it("knows all eleven sections the proto defines", () => {
    expect(SECTION_ORDER).toHaveLength(11);
  });

  it("calls a computed section computed", () => {
    expect(sectionView(live, "OPEN").kind).toBe("computed");
  });

  // ⛔ THE ONE THAT MATTERS. An unbuilt section must never reach the view as
  // anything a template could draw as empty.
  it("calls an unbuilt section dark and carries its reason", () => {
    const v = sectionView(live, "DRIFT");
    expect(v.kind).toBe("dark");
    if (v.kind !== "dark") throw new Error("unreachable");
    expect(v.reason).toContain("standards register");
    expect(v.state).toBe("not computed");
  });

  it("invents a reason rather than letting a dark section render blank", () => {
    const b = {
      ...live,
      sections: [{ section: "DRIFT", state: "withheld", reason: "" }],
    };
    const v = sectionView(b, "DRIFT");
    if (v.kind !== "dark") throw new Error("expected dark");
    expect(v.reason).not.toBe("");
    expect(v.reason).toContain("withheld");
  });

  it("calls a section the daemon never mentioned absent, not computed", () => {
    expect(sectionView({ ...live, sections: [] }, "OPEN").kind).toBe("absent");
  });

  it("treats a null brief as eleven absent sections", () => {
    const t = sectionTally(null);
    expect(t).toEqual({ computed: 0, dark: 0, absent: 11, total: 11 });
  });

  it("tallies the live answer at five computed and six dark", () => {
    expect(sectionTally(live)).toEqual({
      computed: 5,
      dark: 6,
      absent: 0,
      total: 11,
    });
  });

  it("labels every section it knows about", () => {
    for (const v of sectionViews(live)) expect(v.label).not.toBe(v.name);
  });
});

describe("step states", () => {
  it("gives an unrecorded item no colour", () => {
    expect(stepTone(NOT_STEPPED)).toBe("none");
  });

  it("maps the three recorded states onto semantic roles", () => {
    expect(stepTone("done")).toBe("good");
    expect(stepTone("started")).toBe("progress");
    expect(stepTone("blocked")).toBe("bad");
  });

  it("flags a state this build has never heard of", () => {
    expect(stepTone("unknown step state 7")).toBe("warn");
  });

  // ⛔ "started 0" IS THE FINDING. A tally that drops empty rows hides that
  // three states exist and none has been reached.
  it("keeps the zero rows", () => {
    const t = tally([item("a", NOT_STEPPED), item("b", NOT_STEPPED)]);
    expect(t).toEqual([
      { state: "done", count: 0 },
      { state: "started", count: 0 },
      { state: "blocked", count: 0 },
      { state: NOT_STEPPED, count: 2 },
    ]);
  });

  it("appends a state the wire used that this build does not know", () => {
    const t = tally([item("a", "unknown step state 7")]);
    expect(t.at(-1)).toEqual({ state: "unknown step state 7", count: 1 });
    expect(t).toHaveLength(5);
  });
});

describe("planning versus execution", () => {
  it("counts next-up and open together, next-up first", () => {
    const p = planVsExec(live);
    expect(p.planned).toBe(4);
    expect(p.items.map((i) => i.id)).toEqual(["B1", "B10", "B15", "B16"]);
  });

  it("records nothing when every item is unstepped", () => {
    expect(planVsExec(live).recorded).toBe(0);
  });

  it("counts a started item as recorded, not only a done one", () => {
    const b = { ...live, open: [item("B15", "started"), item("B16", "done")] };
    expect(planVsExec(b).recorded).toBe(2);
  });

  it("reports whether the two lists could be computed at all", () => {
    const p = planVsExec(live);
    expect(p.openComputed).toBe(true);
    expect(p.nextUpComputed).toBe(true);
    const dark = {
      ...live,
      sections: live.sections.map((s: any) =>
        s.section === "OPEN" ? { ...s, state: "not computed" } : s,
      ),
    };
    expect(planVsExec(dark).openComputed).toBe(false);
  });
});

describe("the verdict sentence", () => {
  it("says so plainly when nothing has been recorded", () => {
    const v = verdict(planVsExec(live));
    expect(v).toContain("recorded nothing");
    expect(v).toContain("4");
  });

  // Another seat is repairing the write path. The sentence has to survive the
  // day these numbers start moving, or the tab is a snapshot of one bad hour.
  it("reads correctly once execution starts being recorded", () => {
    const some = {
      ...live,
      nextUp: [item("B1", "started"), item("B10", "done")],
      open: [item("B15", "started"), item("B16", NOT_STEPPED)],
    };
    expect(verdict(planVsExec(some))).toBe(
      "3 of 4 planned items have a step recorded against them, 75%.",
    );
  });

  it("reads correctly when everything has been stepped", () => {
    const all = {
      ...live,
      nextUp: [item("B1", "started")],
      open: [item("B15", "done")],
    };
    expect(verdict(planVsExec(all))).toContain("Every one of the 2");
  });

  it("does not divide by zero on an empty project", () => {
    expect(verdict(planVsExec({ ...live, open: [], nextUp: [] }))).toContain(
      "no open work items",
    );
  });
});

describe("blind spots", () => {
  it("always names the closed items it cannot see", () => {
    expect(blindSpots(live)[0].title).toContain("Finished work");
  });

  it("names the unbuilt sections by count", () => {
    expect(blindSpots(live).some((b) => b.title.startsWith("6 of 11"))).toBe(
      true,
    );
  });

  it("names sections the daemon never mentioned", () => {
    const b = blindSpots({ ...live, sections: [] });
    expect(b.some((x) => x.title.includes("11 section"))).toBe(true);
  });

  it("drops the unbuilt-section spot once every section is computed", () => {
    const b = {
      ...live,
      sections: SECTION_ORDER.map((s) => ({
        section: s,
        state: "computed",
        reason: "",
      })),
    };
    expect(blindSpots(b)).toHaveLength(1);
  });
});

describe("age", () => {
  const now = Date.UTC(2026, 8, 17, 12, 0, 0);
  const ago = (ms: number) => (now - ms) * 1e6;

  it("says never rather than showing the epoch", () => {
    expect(age(0, now)).toBe("never stepped");
  });

  it("rounds down through the units", () => {
    expect(age(ago(30_000), now)).toBe("just now");
    expect(age(ago(5 * 60_000), now)).toBe("5m ago");
    expect(age(ago(3 * 3_600_000), now)).toBe("3h ago");
    expect(age(ago(50 * 3_600_000), now)).toBe("2d ago");
  });

  it("does not render a negative age as a huge one", () => {
    expect(age(ago(-60_000), now)).toBe("in the future");
  });
});

describe("projects against cases", () => {
  const e = (id: string, kind: string) => ({ id, kind });

  it("splits on the kind the wire carries", () => {
    const { projects, cases } = splitByKind([
      e("rig", "project"),
      e("tax-2026", "case"),
      e("shelf", "project"),
    ]);
    expect(projects.map((x) => x.id)).toEqual(["rig", "shelf"]);
    expect(cases.map((x) => x.id)).toEqual(["tax-2026"]);
  });

  // ⛔ A record must not fall out of both buckets and become unreachable.
  it("puts an empty kind on the projects side rather than nowhere", () => {
    const { projects, cases } = splitByKind([e("mystery", "")]);
    expect(projects).toHaveLength(1);
    expect(cases).toHaveLength(0);
  });

  it("puts a kind this build has never heard of on the projects side", () => {
    const { projects } = splitByKind([e("future", "dossier")]);
    expect(projects.map((x) => x.id)).toEqual(["future"]);
  });

  it("answers both sides empty for an empty roster", () => {
    expect(splitByKind([])).toEqual({ projects: [], cases: [] });
  });
});
