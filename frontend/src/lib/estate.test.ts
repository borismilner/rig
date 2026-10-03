import { describe, expect, it } from "vitest";
import { forHow, stamp, stateOf } from "./estate";

const prog = (o: object = {}) =>
  ({ id: "p", down: false, atRest: false, ...o }) as any;
const run = (state: string) => ({ id: "p", state }) as any;

describe("stateOf", () => {
  it("lets supervision's word win", () => {
    expect(stateOf(prog({ down: true }), run("healthy"))).toMatchObject({
      word: "healthy",
      tone: "good",
      can: ["stop", "restart"],
    });
  });
  it("offers Restart first for a quarantined program", () => {
    expect(stateOf(prog(), run("quarantined")).can).toEqual([
      "restart",
      "stop",
    ]);
  });
  it("says at rest in words and offers Start", () => {
    expect(stateOf(prog(), run("at_rest"))).toMatchObject({
      word: "at rest",
      tone: "rest",
      can: ["start"],
    });
  });
  it("calls a supervised program with no state stopped, and offers Start", () => {
    expect(stateOf(prog(), run(""))).toEqual({
      word: "stopped",
      tone: "rest",
      can: ["start"],
    });
  });
  it("reads the registry's flags when rig does not supervise it", () => {
    expect(stateOf(prog({ down: true })).word).toBe("down");
    expect(stateOf(prog({ atRest: true })).word).toBe("at rest");
  });
  it("never invents a state or a button for a program started by hand", () => {
    expect(stateOf(prog())).toEqual({
      word: "started by hand",
      tone: "none",
      can: [],
    });
  });
});

describe("times (requirement 41)", () => {
  it("is 24-hour and ISO-ordered", () => {
    const s = stamp(new Date(2026, 9, 3, 16, 5, 9).getTime());
    expect(s).toBe("2026-10-03 16:05:09");
    expect(s).not.toMatch(/AM|PM/);
  });
  it("says how long in one unit", () => {
    const now = 10_000_000;
    expect(forHow(now - 30_000, now)).toBe("30 s");
    expect(forHow(now - 600_000, now)).toBe("10 min");
    expect(forHow(0, now)).toBe("");
  });
});
