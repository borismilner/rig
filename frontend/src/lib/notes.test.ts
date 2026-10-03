import { describe, expect, it } from "vitest";
import { ago, answered, arrange, sources, waiting } from "./notes";

const note = (id: string, o: object = {}) =>
  ({
    id,
    severity: "info",
    title: id,
    body: "",
    sender: "rig",
    at: "2026-10-03T10:00:00Z",
    suppressed: false,
    replies: [],
    replyText: false,
    asks: false,
    waiting: false,
    forgotten: false,
    answer: null,
    ...o,
  }) as any;

const N = [
  note("new", { sender: "shelf", body: "disk full" }),
  note("ask", { asks: true, waiting: true, replies: ["yes", "later"] }),
  note("done", {
    asks: true,
    answer: { reply: "yes", text: "", dismissed: false, by: "boris" },
  }),
  note("lost", { asks: true, forgotten: true, sender: "shelf" }),
];

describe("arrange", () => {
  it("puts what waits on top and keeps the time order otherwise", () => {
    expect(arrange(N, "", "").map((n) => n.id)).toEqual([
      "ask",
      "new",
      "done",
      "lost",
    ]);
  });
  it("searches title, body, sender and options, and filters by source", () => {
    expect(arrange(N, "DISK", "").map((n) => n.id)).toEqual(["new"]);
    expect(arrange(N, "later", "").map((n) => n.id)).toEqual(["ask"]);
    expect(arrange(N, "", "shelf").map((n) => n.id)).toEqual(["new", "lost"]);
  });
});

describe("waiting", () => {
  it("is only an unanswered ask rig still holds", () => {
    expect(N.map(waiting)).toEqual([false, true, false, false]);
  });
});

describe("sources", () => {
  it("counts each sender, sorted by name", () => {
    expect(sources(N)).toEqual([
      ["rig", 2],
      ["shelf", 2],
    ]);
  });
});

describe("answered", () => {
  it("names the choice, the text and who", () => {
    expect(answered(N[2])).toBe('"yes" (boris)');
    expect(
      answered(note("x", { answer: { dismissed: true, by: "" } })),
    ).toBe("dismissed");
    expect(
      answered(note("x", { answer: { reply: "", text: "because", by: "b" } })),
    ).toBe('"because" (b)');
  });
});

describe("ago", () => {
  it("rounds down to the unit", () => {
    const t = Date.parse("2026-10-03T10:00:00Z");
    expect(ago("2026-10-03T10:00:00Z", t + 30e3)).toBe("now");
    expect(ago("2026-10-03T10:00:00Z", t + 90 * 60e3)).toBe("1 h");
    expect(ago("2026-10-03T10:00:00Z", t + 50 * 3600e3)).toBe("2 d");
    expect(ago("bad", t)).toBe("");
  });
});
