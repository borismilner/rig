/* The arithmetic behind the Spec and Decisions views.
 *
 * brief.test.ts states the division and it holds here: the rendering is
 * demonstrated in the real window, and the half that can be wrong while the
 * page still looks right is demonstrated here. Every assertion below is a claim
 * one of the two views makes in a heading or a count.
 */

import { describe, it, expect } from "vitest";
import {
  groupRecords,
  filterRecords,
  matchesRecord,
  tagsOf,
  citation,
  indentOf,
  shownTally,
  isView,
  stripGroupDate,
  groupCount,
  VIEW_KIND,
  VIEW_LABEL,
  VIEWS,
} from "./records";

const rec = (r: Record<string, unknown>) =>
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
  }) as never;

describe("the view switch", () => {
  it("offers exactly the three the panel can draw", () => {
    expect([...VIEWS]).toEqual(["now", "spec", "decisions"]);
    expect(VIEWS.every((v) => isView(v))).toBe(true);
    expect(isView("records")).toBe(false);
  });

  // ⛔ "now" READS THE BRIEF AND MUST ASK FOR NO KIND. Records() refuses an
  // empty kind by design - an unfiltered read of this store is not a screen -
  // so a view that mapped "now" to a kind would dial and be refused.
  it("asks for no kind on the brief, and a real kind on the other two", () => {
    expect(VIEW_KIND.now).toBe("");
    expect(VIEW_KIND.spec).toBe("requirement");
    expect(VIEW_KIND.decisions).toBe("decision");
    expect(Object.values(VIEW_LABEL)).toEqual(["Now", "Spec", "Decisions"]);
  });
});

describe("grouping", () => {
  // ⛔ THE HEADING IS THE SECTION'S OWN TITLE, NOT ITS NUMBER. This shipped
  // reading `fields["section-title"]`, a key no seeder writes, so every one of
  // the 42 groups would have been headed "1", "2", "3".
  it("heads a section with what the document calls it", () => {
    const g = groupRecords([
      rec({ id: "11/a", section: "11", sectionTitle: "11. The window" }),
      rec({ id: "11/b", section: "11", sectionTitle: "11. The window" }),
      rec({ id: "12/a", section: "12", sectionTitle: "12. Toasts" }),
    ]);
    expect(g.map((x) => x.title)).toEqual(["11. The window", "12. Toasts"]);
    expect(g.map((x) => x.rows.length)).toEqual([2, 1]);
  });

  /* ⛔ THE HEADING'S OWN RECORD IS NOT ALSO A ROW UNDER IT. Seen in the real
     window on 2026-09-18: the heading read "1. The idea, in one line" and the
     row directly beneath it read "1. The idea, in one line". In all 42
     sections. */
  it("lifts the section's own record onto the heading instead of listing it twice", () => {
    const [g] = groupRecords([
      rec({
        id: "11/after-the-gaps",
        title: "After the gaps",
        section: "11",
        sectionTitle: "11. The window",
      }),
      rec({
        id: "11/the-window",
        title: "11. The window",
        section: "11",
        sectionTitle: "11. The window",
        body: "the section's own preamble",
      }),
      rec({
        id: "11/the-rail",
        title: "The rail",
        section: "11",
        sectionTitle: "11. The window",
      }),
    ]);
    expect(g.lead?.id).toBe("11/the-window");
    expect(g.lead?.body).toBe("the section's own preamble");
    expect(g.rows.map((r) => r.id)).toEqual([
      "11/after-the-gaps",
      "11/the-rail",
    ]);
  });

  /* ⛔ BY TITLE, NOT BY POSITION. The rows arrive sorted by id, and section
     11's head is `11/the-window` while `11/after-the-gaps` sorts above it -
     so lifting rows[0] would present an arbitrary requirement as the section.
     The fixture above is in exactly that order; this asserts the consequence. */
  it("does not lift the first row when the first row is not the head", () => {
    const [g] = groupRecords([
      rec({ id: "39/a-thing", title: "A thing", section: "39", sectionTitle: "39. The record" }),
      rec({ id: "39/the-record", title: "39. The record", section: "39", sectionTitle: "39. The record" }),
    ]);
    expect(g.lead?.id).toBe("39/the-record");
    expect(g.rows.map((r) => r.id)).toEqual(["39/a-thing"]);
  });

  it("keeps every row when no record carries the heading's title", () => {
    const [g] = groupRecords([
      rec({ id: "40/x", title: "X", section: "40", sectionTitle: "40. Knowledge" }),
      rec({ id: "40/y", title: "Y", section: "40", sectionTitle: "40. Knowledge" }),
    ]);
    expect(g.lead).toBe(null);
    expect(g.rows.length).toBe(2);
  });

  // A decision group has no head record, so nothing is lifted out of it.
  it("lifts nothing out of a day", () => {
    const [g] = groupRecords([
      rec({ kind: "decision", id: "2026-09-18-x", title: "2026-09-18", date: "2026-09-18" }),
    ]);
    expect(g.lead).toBe(null);
    expect(g.rows.length).toBe(1);
  });

  it("falls back to the number rather than to a blank heading", () => {
    const g = groupRecords([rec({ id: "39/a", section: "39" })]);
    expect(g[0].title).toBe("Section 39");
  });

  // ⛔ 91 OF 533 DECISIONS CARRY NO DATE ANYWHERE. A heap of rulings under a
  // blank heading reads as a bug, so the group is named and says why.
  it("gives undated decisions a group that explains itself", () => {
    const g = groupRecords([
      rec({ kind: "decision", id: "2026-09-18-x", date: "2026-09-18" }),
      rec({ kind: "decision", id: "2026-09-18-y", date: "2026-09-18" }),
      rec({ kind: "decision", id: "a-constant-dressed-as-a-live-field" }),
    ]);
    expect(g.map((x) => x.title)).toEqual(["2026-09-18", "No date stated"]);
    expect(g[0].note).toBe("");
    expect(g[1].note).toContain("neither the document nor the id");
  });

  // ⛔ THE GROUPING NEVER REORDERS. The Go side sorts once - sections
  // ascending, dates descending - and a second sort here would mean two places
  // decide the order and only one of them is tested.
  it("preserves the order it was handed", () => {
    const g = groupRecords([
      rec({ id: "9/a", section: "9", sectionTitle: "9. Built for agents" }),
      rec({ id: "11/a", section: "11", sectionTitle: "11. The window" }),
      rec({ id: "9/b", section: "9", sectionTitle: "9. Built for agents" }),
    ]);
    // Section 9 appears twice in the input and is ONE group, at its first
    // position - grouping is not sorting, and it does not become sorting.
    expect(g.map((x) => x.key)).toEqual(["9", "11"]);
    expect(g[0].rows.map((r) => r.id)).toEqual(["9/a", "9/b"]);
  });

  it("returns nothing for nothing", () => {
    expect(groupRecords([])).toEqual([]);
  });
});

describe("find", () => {
  const rows = [
    rec({
      id: "11/never-stale",
      title: "THE WINDOW MAY NEVER BE STALE",
      body: "He caught a six-hour-old window on a deleted inode.",
      source: "plan/11-the-window.md",
    }),
    rec({
      id: "12/toasts",
      title: "12. Toasts",
      body: "A toast is how rig says a thing happened.",
      source: "plan/12-toasts.md",
    }),
  ];

  // ⛔ THIS IS THE ONE THING THESE VIEWS CAN DO THAT THE BRIEF'S FILTER CANNOT.
  // PlanVsExec matches an id, a title and a short description, because the
  // brief carries a cut. These rows carry whole bodies, so the prose matches.
  it("matches the body, which is what the brief's filter cannot reach", () => {
    expect(filterRecords(rows, "deleted inode").map((r) => r.id)).toEqual([
      "11/never-stale",
    ]);
  });

  it("matches the id, the title and the source file", () => {
    expect(filterRecords(rows, "12/").map((r) => r.id)).toEqual(["12/toasts"]);
    expect(filterRecords(rows, "NEVER BE STALE").map((r) => r.id)).toEqual([
      "11/never-stale",
    ]);
    // The source is its own field and matches on nothing else here: "plan/11"
    // appears in no id, title or body.
    expect(filterRecords(rows, "plan/11").map((r) => r.id)).toEqual([
      "11/never-stale",
    ]);
  });

  it("ignores case and surrounding space", () => {
    expect(matchesRecord(rows[0], "  NEVER be STALE  ")).toBe(true);
  });

  // Filters, never rearranges - the design's rule 2. A result set that
  // reorders the page destroys the spatial memory that makes a second visit
  // fast.
  it("keeps the order and returns everything for an empty query", () => {
    expect(filterRecords(rows, "   ")).toBe(rows);
    expect(filterRecords(rows, "e").map((r) => r.id)).toEqual([
      "11/never-stale",
      "12/toasts",
    ]);
  });
});

describe("the fields a row shows", () => {
  it("opens the tags the store writes as a JSON string", () => {
    expect(tagsOf(rec({ tags: '["section:window","rule"]' }))).toEqual([
      "section:window",
      "rule",
    ]);
    expect(tagsOf(rec({ tags: "" }))).toEqual([]);
  });

  // A field that will not parse is shown as itself. Dropping it silently is
  // how a rendering bug is reported as a data bug.
  it("shows an unparseable tag field rather than swallowing it", () => {
    expect(tagsOf(rec({ tags: "not json" }))).toEqual(["not json"]);
    expect(tagsOf(rec({ tags: '{"a":1}' }))).toEqual(['{"a":1}']);
  });

  it("cites source and line together, and neither alone is invented", () => {
    expect(citation(rec({ source: "plan/11.md", line: "88" }))).toBe(
      "plan/11.md:88",
    );
    expect(citation(rec({ source: "plan/11.md" }))).toBe("plan/11.md");
    expect(citation(rec({ line: "88" }))).toBe("");
  });

  // ⛔ CLAMPED AT THREE STEPS. Level 5 exists in his plan; a fourth step of
  // indent starts eating the title's width on the 1080px installed window.
  it("indents by heading depth and stops before the title suffers", () => {
    expect([2, 3, 4, 5, 6].map((level) => indentOf(rec({ level })))).toEqual([
      0, 1, 2, 3, 3,
    ]);
    expect(indentOf(rec({ level: 0 }))).toBe(0);
  });
});

// The count only says "n of m" while something is hidden. A page that always
// reads "387 of 387" trains a reader to stop reading the number.
it("says how many are hidden only when some are", () => {
  expect(shownTally(387, 387)).toBe("387");
  expect(shownTally(4, 387)).toBe("4 of 387");
});

/* ⛔ 141 OF HIS 540 DECISION TITLES OPEN WITH THEIR OWN DATE, under a heading
   that is that date. About thirty characters of every top-level row carried no
   information and pushed the ruling out of the visible width. B98. */
describe("a title does not repeat its own heading", () => {
  it("takes the date off and keeps which session ruled it", () => {
    expect(
      stripGroupDate(
        "2026-09-18, team-lead generation 21 - the fence rule is one type",
        "2026-09-18",
      ),
    ).toEqual({
      lead: "team-lead generation 21",
      text: "the fence rule is one type",
    });
  });

  it("unwraps a parenthesised qualifier rather than leaving a stray bracket", () => {
    expect(
      stripGroupDate("2026-09-10 (fifth session) - the project carries no dates", "2026-09-10"),
    ).toEqual({ lead: "fifth session", text: "the project carries no dates" });
  });

  // The qualifier is the part that is NOT redundant: three sessions can rule on
  // one day, and which one did is the thing the heading cannot say.
  it("keeps a title that has no qualifier whole", () => {
    expect(stripGroupDate("2026-09-12 - the window is cmd/rigwindow", "2026-09-12")).toEqual({
      lead: "",
      text: "the window is cmd/rigwindow",
    });
  });

  it("leaves a title that does not open with the heading's date alone", () => {
    const t = "A constant dressed as a live field is a lie by shape";
    expect(stripGroupDate(t, "2026-09-12")).toEqual({ lead: "", text: t });
    // A DIFFERENT date must not be stripped either - it is not redundant then.
    expect(stripGroupDate("2026-09-10 - older", "2026-09-12")).toEqual({
      lead: "",
      text: "2026-09-10 - older",
    });
  });

  // ⛔ THE UNDATED GROUP'S KEY IS NOT A DATE, so nothing may be taken off a
  // title under it. 91 of his decisions live there.
  it("takes nothing off under a group with no date", () => {
    const t = "§5e gains the element list";
    expect(stripGroupDate(t, "")).toEqual({ lead: "", text: t });
    expect(stripGroupDate(t, "~undated")).toEqual({ lead: "", text: t });
  });

  // A title that is ONLY its date would otherwise render as an empty row.
  it("never renders a row with no text at all", () => {
    expect(stripGroupDate("2026-09-12", "2026-09-12")).toEqual({
      lead: "",
      text: "2026-09-12",
    });
  });

  it("does not hyphen-split a title that merely contains a dash", () => {
    expect(
      stripGroupDate("2026-09-12 - a door nobody can dial - and how it grew", "2026-09-12"),
    ).toEqual({ lead: "", text: "a door nobody can dial - and how it grew" });
  });
});

/* ⛔ THE GROUP COUNTS MUST SUM TO THE NUMBER IN THE TITLE BAR. Lifting the head
   out of `rows` dropped 42 records out of the per-section counts while the
   header still said 387, so the page contradicted itself on its first screen. */
it("counts the record on the heading as one of the section's", () => {
  const g = groupRecords([
    rec({ id: "11/the-window", title: "11. The window", section: "11", sectionTitle: "11. The window" }),
    rec({ id: "11/rail", title: "The rail", section: "11", sectionTitle: "11. The window" }),
  ]);
  expect(g[0].rows.length).toBe(1);
  expect(groupCount(g[0])).toBe(2);
});
