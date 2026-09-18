/* Grouping and filtering for the record views - the arithmetic behind Spec and
 * Decisions, kept out of the component for the reason brief.ts states: the
 * rendering is demonstrated in the real window, and the half that can be wrong
 * while the page still looks right is demonstrated here.
 *
 * ⛔ WHAT THESE VIEWS ARE FOR. Boris, 2026-09-18: "In the GUI I don't see the
 * utilization of the many goodies we have stored in `rig`." Measured against
 * his production store the same day, AFTER the re-seed: the store holds 1,057
 * current records and the window's two calls could reach at most the work
 * items among them. The 387 requirements and 533 decisions - 920 records,
 * 1.2 MB of prose, 87% of everything rig currently stands behind - had no
 * screen at all.
 *
 * ⛔ AND 1,057 IS NOT 2,538. The design document counted rows in the `records`
 * table, which holds every superseded version: 2,568 rows against 1,057 heads,
 * so the headline was 2.4x the number of things rig actually says. The gap it
 * argued for is real and the arithmetic is corrected here and in the document.
 */

import type { RecordRow } from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";

/** One heading and the records under it. */
export type Group = {
  /** Stable across renders and unique within a view - the {#each} key. */
  key: string;
  /** What the heading reads, already in the reader's terms. */
  title: string;
  /** A second line under the heading, or "". Never repeats the title. */
  note: string;
  /* ⛔ THE SECTION'S OWN RECORD, LIFTED OUT OF THE LIST. Seen in the real
     window on 2026-09-18: the heading read "1. The idea, in one line" and the
     first row under it read "1. The idea, in one line" - the same words, one
     line apart, in all 42 sections. The heading IS that record, so it is drawn
     once and opens that record's body. 37 of the 42 heads carry a body worth
     opening; the other 5 are a title and nothing else, and their heading gets
     no affordance rather than an empty panel. */
  lead: RecordRow | null;
  rows: RecordRow[];
};

/** The view a reader picks, and the switch's whole vocabulary. */
export const VIEWS = ["now", "spec", "decisions"] as const;
export type View = (typeof VIEWS)[number];

/** Which record kind a view reads. "now" reads the brief and asks for none. */
export const VIEW_KIND: Record<View, string> = {
  now: "",
  spec: "requirement",
  decisions: "decision",
};

export const VIEW_LABEL: Record<View, string> = {
  now: "Now",
  spec: "Spec",
  decisions: "Decisions",
};

export function isView(v: string): v is View {
  return (VIEWS as readonly string[]).includes(v);
}

/** The keys the maps above use for a group nothing states. */
const NO_SECTION = "~none";
const NO_DATE = "~undated";

/* ⛔ THE SPEC GROUPS BY SECTION AND THE DECISIONS GROUP BY DAY, AND ONE RULE
 * CANNOT DO BOTH. A specification read newest-first scatters section 39 through
 * section 11; a decision log read in section order has no sections to read in -
 * every decision's `section` is empty. The kind decides, once, here.
 */
export function groupRecords(rows: RecordRow[]): Group[] {
  if (rows.length === 0) return [];
  return rows.some((r) => r.section) ? liftHeads(bySection(rows)) : byDay(rows);
}

/* liftHeads takes the record the heading is NAMED AFTER out of the list under
 * it, so the words appear once rather than twice.
 *
 * ⛔ IT MATCHES BY TITLE AND NOT BY POSITION, and that is measured. The rows
 * arrive sorted by id within a section, and the head is NOT first: section 11's
 * head is `11/the-window` while `11/after-the-gaps` sorts above it. Taking
 * rows[0] would lift an arbitrary requirement out of the middle of ten and
 * present it as the section.
 *
 * The title is safe to match on: SectionTitle was COPIED from that record on
 * the Go side, and over his 42 sections no other record in a section shares its
 * head's title. A section whose head is somehow absent keeps every row.
 */
function liftHeads(groups: Group[]): Group[] {
  for (const g of groups) {
    const i = g.rows.findIndex((r) => r.title === g.title);
    if (i < 0) continue;
    g.lead = g.rows[i];
    g.rows = g.rows.slice(0, i).concat(g.rows.slice(i + 1));
  }
  return groups;
}

function bySection(rows: RecordRow[]): Group[] {
  const out: Group[] = [];
  const at = new Map<string, Group>();
  for (const r of rows) {
    // Rows arrive already in section order from the Go side, so pushing a new
    // group the first time a section is seen preserves it without re-sorting.
    const key = r.section || NO_SECTION;
    let g = at.get(key);
    if (!g) {
      g = {
        key,
        // ⛔ THE NUMBER IS NOT A HEADING. "11" tells a reader nothing; "11. The
        // window" is what the document itself calls the section, and the Go
        // side now carries it. The bare number is the fallback for a record
        // whose section has no head in this answer, not the design.
        title:
          r.sectionTitle ||
          (r.section ? `Section ${r.section}` : "Ungrouped"),
        note: r.section ? "" : "these records state no section",
        lead: null,
        rows: [],
      };
      at.set(key, g);
      out.push(g);
    }
    g.rows.push(r);
  }
  return out;
}

function byDay(rows: RecordRow[]): Group[] {
  const out: Group[] = [];
  const at = new Map<string, Group>();
  for (const r of rows) {
    const key = r.date || NO_DATE;
    let g = at.get(key);
    if (!g) {
      g = {
        key,
        title: r.date || "No date stated",
        /* ⛔ THE UNDATED GROUP SAYS WHY IT EXISTS. 91 of his 533 decisions
           carry no date in the document or in their id, and a reader who finds
           a heap of rulings under a blank heading will read it as a bug. It is
           a property of the source text, and saying so is cheaper than the
           question. */
        note: r.date ? "" : "neither the document nor the id states a day",
        lead: null,
        rows: [],
      };
      at.set(key, g);
      out.push(g);
    }
    g.rows.push(r);
  }
  return out;
}

/* ⛔ THIS SEARCHES THE FULL PROSE, AND THAT IS NEW RATHER THAN A CLAIM.
 * PlanVsExec's filter can only match a row's id, title and short description,
 * because the brief carries a cut and not the body. These views hold the whole
 * body of every record of their kind - 756 KB for the spec, 490 KB for the
 * decisions - so matching the text is free and honest.
 *
 * What it still CANNOT do is search ACROSS kinds, or search what is not on this
 * page. That is B28, the store has no full-text index, and the view says so in
 * as many words rather than letting a reader conclude a word is absent from rig.
 */
export function matchesRecord(r: RecordRow, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  return (
    r.id.toLowerCase().includes(q) ||
    r.title.toLowerCase().includes(q) ||
    r.body.toLowerCase().includes(q) ||
    r.source.toLowerCase().includes(q)
  );
}

/** Rows matching the query, in the order they came. Filters, never rearranges. */
export function filterRecords(rows: RecordRow[], query: string): RecordRow[] {
  const q = query.trim();
  if (!q) return rows;
  return rows.filter((r) => matchesRecord(r, q));
}

/* The store writes tags as a JSON array in a string field, so the view has to
 * open it. A field that will not parse is shown as itself rather than dropped:
 * silently swallowing it is how a rendering bug becomes a data bug. */
export function tagsOf(r: RecordRow): string[] {
  const raw = (r.tags ?? "").trim();
  if (!raw) return [];
  try {
    const v = JSON.parse(raw);
    if (Array.isArray(v)) return v.map(String).filter(Boolean);
  } catch {
    /* falls through to the raw string */
  }
  return [raw];
}

/** Where the document says it, as one citable string, or "". */
export function citation(r: RecordRow): string {
  if (!r.source) return "";
  return r.line ? `${r.source}:${r.line}` : r.source;
}

/** The indent a nested requirement gets, clamped so a deep one stays readable. */
export function indentOf(r: RecordRow): number {
  if (!r.level || r.level <= 2) return 0;
  return Math.min(r.level - 2, 3);
}

/** How many rows a filtered view is showing out of how many it holds. */
export function shownTally(shown: number, held: number): string {
  return shown === held ? `${held}` : `${shown} of ${held}`;
}

/** A title split into the qualifier its heading makes redundant, and the rest. */
export type TitleParts = { lead: string; text: string };

/* stripGroupDate takes off the front of a title what the heading above it
 * already says.
 *
 * ⛔ 141 OF HIS 540 DECISION TITLES OPEN WITH THEIR OWN DATE, directly under a
 * heading that is that date: *"2026-09-18, team-lead generation 21 - the fence
 * rule is one type and four callers"* under **2026-09-18**. About thirty
 * characters of every top-level row carried no information and pushed the
 * actual ruling out of the visible width. B98.
 *
 * ⛔ IT IS DONE IN THE VIEW AND NOT IN THE DOCUMENT, and that is a decision
 * rather than the cheaper option. `DECISIONS.md` is read linearly by people and
 * by agents, where the date on an entry is the only thing placing it; and a
 * title edit re-slugs the record, which would supersede 141 ids that other
 * documents cite. The redundancy exists only under the heading, so it is
 * removed only under the heading.
 *
 * ⛔ THE QUALIFIER IS KEPT, BECAUSE IT IS NOT THE REDUNDANT PART. "(fifth
 * session)" and "team-lead generation 21" say WHICH pass of that day ruled it,
 * and three sessions can rule on one day. It moves to its own dim slot; only
 * the date itself goes.
 */
export function stripGroupDate(title: string, date: string): TitleParts {
  if (!date || !title.startsWith(date)) return { lead: "", text: title };
  let rest = title.slice(date.length);
  // A title that IS just the date keeps itself rather than becoming blank.
  if (rest.trim() === "") return { lead: "", text: title };

  const cut = rest.indexOf(" - ");
  if (cut < 0) return { lead: "", text: trimLead(rest) };

  return {
    lead: trimLead(rest.slice(0, cut)),
    text: rest.slice(cut + 3).trim(),
  };
}

/** Drops the punctuation that only joined a title to the date in front of it. */
function trimLead(s: string): string {
  return s
    .replace(/^[\s,;:-]+/, "")
    .replace(/^\((.*)\)$/, "$1")
    .trim();
}

/* How many records a group holds, INCLUDING the one lifted onto its heading.
 *
 * ⛔ THE GROUP COUNTS HAVE TO SUM TO THE NUMBER IN THE TITLE BAR. Lifting the
 * head out of `rows` dropped 42 records out of the per-section counts while
 * the header still said 387, so the page contradicted itself on its own first
 * screen. The heading's record is still a record of that section.
 */
export function groupCount(g: Group): number {
  return g.rows.length + (g.lead ? 1 : 0);
}
