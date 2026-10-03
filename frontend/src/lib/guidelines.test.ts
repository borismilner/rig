import { describe, expect, it } from "vitest";
import { builtBefore, standing } from "./guidelines";

const rule = (id: string, date: string, o: object = {}) =>
  ({ id, date, who: "programs", built: true, ...o }) as any;
const G = {
  revision: "2026-10-03",
  rules: [
    rule("G6", "2026-10-03", { built: false }),
    rule("G4", "2026-10-03"),
    rule("A2", "2026-09-24", { who: "agents" }),
    rule("G3", "2026-10-01"),
  ],
  programs: [
    { program: "old", day: "2026-09-30" },
    { program: "today", day: "2026-10-03" },
    { program: "script", day: "", unknownBecause: "no build info" },
  ],
} as any;

describe("standing", () => {
  it("counts built program rules after the build day", () => {
    const s = standing(G, "old");
    expect(s.word).toBe("2 newer");
    expect(s.newer.map((r) => r.id)).toEqual(["G4", "G3"]);
  });
  it("does not count a rule dated the build day, an unbuilt one, or an agents' one", () => {
    expect(standing(G, "today")).toMatchObject({
      word: "current",
      tone: "good",
    });
  });
  it("says unknown without a build day, and without an answer at all", () => {
    expect(standing(G, "script").word).toBe("unknown");
    expect(standing(G, "absent").word).toBe("unknown");
    expect(standing(null, "old").word).toBe("unknown");
  });
});

describe("builtBefore", () => {
  it("names the programs older than a counting rule", () => {
    expect(builtBefore(G, G.rules[1])).toEqual(["old"]);
    expect(builtBefore(G, G.rules[0])).toEqual([]);
    expect(builtBefore(G, G.rules[2])).toEqual([]);
  });
});
