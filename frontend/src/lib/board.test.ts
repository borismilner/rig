import { describe, expect, it } from "vitest";
import type { Card } from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/models.js";
import { hhmm, sections, shown, sourceTabs, worst } from "./board";

const card = (c: Partial<Card> & Pick<Card, "id" | "from">): Card => ({
  version: 1,
  project: "",
  title: c.id,
  status: "",
  severity: "info",
  body: "",
  progress: 0,
  hasProgress: false,
  busy: false,
  facts: [],
  actions: [],
  closed: false,
  created: "2026-10-03T18:00:00Z",
  updated: "2026-10-03T18:00:00Z",
  ...c,
});

const cards = [
  card({ id: "a1", from: "graft", updated: "2026-10-03T18:05:00Z" }),
  card({ id: "a2", from: "graft", severity: "warning", closed: true }),
  card({
    id: "b1",
    from: "lead",
    severity: "error",
    facts: [{ label: "failed", value: "help-long" }],
    updated: "2026-10-03T18:09:00Z",
  }),
  card({ id: "c1", from: "shelf", closed: true }),
];

describe("the board", () => {
  it("has Every source first, then a tab per source with its open count (27)", () => {
    expect(sourceTabs(cards)).toEqual([
      ["", 2],
      ["graft", 1],
      ["lead", 1],
      ["shelf", 0],
    ]);
  });

  it("filters by source, by severity and by a search that reads facts", () => {
    expect(shown(cards, "graft", "", new Set()).map((c) => c.id)).toEqual([
      "a1",
      "a2",
    ]);
    expect(shown(cards, "", "", new Set(["error"])).map((c) => c.id)).toEqual([
      "b1",
    ]);
    expect(shown(cards, "", "HELP-LONG", new Set()).map((c) => c.id)).toEqual([
      "b1",
    ]);
    expect(shown(cards, "lead", "", new Set(["info"]))).toEqual([]);
  });

  it("puts the source changed last first and splits open from closed", () => {
    const s = sections(cards);
    expect(s.map((x) => x.from)).toEqual(["lead", "graft", "shelf"]);
    expect(s[1].open.map((c) => c.id)).toEqual(["a1"]);
    expect(s[1].closed.map((c) => c.id)).toEqual(["a2"]);
  });

  it("stripes a section by its worst OPEN card, and by nothing when all are closed", () => {
    const s = sections(cards);
    expect(s[0].worst).toBe("error");
    expect(s[1].worst).toBe("info");
    expect(s[2].worst).toBe("");
    expect(worst([card({ id: "x", from: "x", severity: "" })])).toBe("info");
  });

  it("says a time on the 24-hour clock (41)", () => {
    expect(hhmm("2026-10-03T21:04:00")).toBe("21:04");
    expect(hhmm("not a time")).toBe("");
  });
});
