/* What a brief means, separated from how it is drawn.
 *
 * Section 11 requirement 14: the first tab's subject is PLANNING VERSUS
 * EXECUTION - "what did we say we would do, and what has actually happened".
 * Every derivation that answers that question lives here rather than in a
 * component, for one reason: it is the part that can be wrong silently. A
 * ratio computed in a template is a ratio nothing tests, and this project has
 * already recorded a caption that was a claim no gate checked.
 *
 * ⛔ NOTHING HERE MAY INFER. If the wire does not carry a fact, the answer is
 * UNKNOWN and the view has to render that word. The brief cannot see a closed
 * item at all (it leaves the list), so there is deliberately no "completed"
 * count anywhere in this file - see blindSpots() for what is said instead.
 */

import type {
  Brief,
  Item,
  SectionStatus,
} from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";

/* ── section states ─────────────────────────────────────────────────────── */

/* The eleven sections, in the proto's own order (proto/rig/v1, BriefSection
 * 1..11), with the names cmd/rigwindow/brief.go emits after trimming
 * BRIEF_SECTION_. Held as a list rather than derived from the response so the
 * view can say "rig answered about 9 of 11 sections" when the daemon is older
 * than the window - an absent section is itself a fact.
 */
export const SECTION_ORDER = [
  "OPEN",
  "NEXT_UP",
  "NOTES",
  "BLOCKED",
  "DRIFT",
  "MUST_READ",
  "PROJECTION_BEHIND",
  "PENDING",
  "LOCAL_ONLY",
  "FEATURES",
  "CASE_NOTES",
] as const;

export type SectionName = (typeof SECTION_ORDER)[number];

export const SECTION_LABEL: Record<string, string> = {
  OPEN: "Open items",
  NEXT_UP: "Next up",
  NOTES: "Notes",
  BLOCKED: "Blocked",
  DRIFT: "Drift from standards",
  MUST_READ: "Must read",
  PROJECTION_BEHIND: "Projection behind",
  PENDING: "Pending entries",
  LOCAL_ONLY: "Local only",
  FEATURES: "Features",
  CASE_NOTES: "Case notes",
};

/* What a section can be, from the view's side.
 *
 *   computed  rig worked it out, and an empty list means empty
 *   dark      rig cannot work it out yet, and carries a reason saying why
 *   absent    the daemon did not mention this section at all
 *
 * "dark" rather than "not computed" because the view needs a word that is not
 * a negation of the good case: an unbuilt section is a different KIND of thing
 * from an empty one, which is the whole point of the status field.
 */
export type SectionView =
  | { kind: "computed"; name: string; label: string }
  | { kind: "dark"; name: string; label: string; reason: string; state: string }
  | { kind: "absent"; name: string; label: string };

export function sectionView(brief: Brief | null, name: string): SectionView {
  const label = SECTION_LABEL[name] ?? name;
  const s = brief?.sections?.find((x: SectionStatus) => x.section === name);
  if (!s) return { kind: "absent", name, label };
  if (s.state === "computed") return { kind: "computed", name, label };
  return {
    kind: "dark",
    name,
    label,
    state: s.state,
    // ⛔ A dark section with no reason is still dark. Falling back to "" would
    // let a template render nothing and look like an empty panel, which is the
    // exact failure the status field exists to prevent.
    reason:
      s.reason ||
      `rig reported this section as "${s.state}" and gave no reason.`,
  };
}

export function sectionViews(brief: Brief | null): SectionView[] {
  return SECTION_ORDER.map((n) => sectionView(brief, n));
}

export function sectionTally(brief: Brief | null): {
  computed: number;
  dark: number;
  absent: number;
  total: number;
} {
  const v = sectionViews(brief);
  return {
    computed: v.filter((x) => x.kind === "computed").length,
    dark: v.filter((x) => x.kind === "dark").length,
    absent: v.filter((x) => x.kind === "absent").length,
    total: v.length,
  };
}

/* ── step states ────────────────────────────────────────────────────────── */

/* The four names cmd/rigwindow/brief.go's stepStateName can emit, plus the
 * open set it cannot: a newer daemon answers "unknown step state 7", and that
 * string is carried through rather than folded into one of these.
 */
export const NOT_STEPPED = "not stepped";
export const STEP_ORDER = ["done", "started", "blocked", NOT_STEPPED] as const;

/* Which token paints a state.
 *
 * NOT_STEPPED gets no hue on purpose and it is NOT the shell's "healthy is the
 * absence of colour" rule being reused - it is the opposite reading. An
 * unrecorded item is an EMPTY cell because nothing has been written about it,
 * and a hollow cell is what nothing looks like. Giving it a colour would make
 * absence look like a state somebody chose.
 */
export function stepTone(state: string): string {
  switch (state) {
    case "done":
      return "good";
    case "started":
      return "progress";
    case "blocked":
      return "bad";
    case NOT_STEPPED:
      return "none";
    default:
      // A state this build has never heard of. It is reported, never guessed
      // at, and it is drawn in the warn hue because an unreadable answer is
      // something the reader has to act on.
      return "warn";
  }
}

export type Tally = { state: string; count: number };

/* Counts by state, in a stable order, with every state the wire actually used.
 *
 * ⛔ THE ZERO ROWS ARE KEPT. "started 0" is the finding; dropping empty rows
 * would leave a legend showing one row and hide that three other states exist
 * and none of them has ever been reached.
 */
export function tally(items: Item[]): Tally[] {
  const seen = new Map<string, number>();
  for (const s of STEP_ORDER) seen.set(s, 0);
  for (const i of items) seen.set(i.state, (seen.get(i.state) ?? 0) + 1);
  const ordered: Tally[] = STEP_ORDER.map((s) => ({
    state: s,
    count: seen.get(s) ?? 0,
  }));
  for (const [state, count] of seen)
    if (!(STEP_ORDER as readonly string[]).includes(state))
      ordered.push({ state, count });
  return ordered;
}

/* ── the answer the tab exists to give ──────────────────────────────────── */

export type PlanVsExec = {
  /* Every item the brief listed, next-up first so the waffle's reading order
   * matches the reader's priority order. */
  items: Item[];
  planned: number;
  /* An item with ANY state other than "not stepped". Not "done": the question
   * is whether execution has been recorded at all, and a started item answers
   * it. */
  recorded: number;
  tally: Tally[];
  /* Whether rig could compute the two lists this is built from. Both dark
   * would make every number above meaningless, so the view asks before it
   * draws. */
  openComputed: boolean;
  nextUpComputed: boolean;
};

export function planVsExec(brief: Brief | null): PlanVsExec {
  const nextUp = brief?.nextUp ?? [];
  const open = brief?.open ?? [];
  const items = [...nextUp, ...open];
  return {
    items,
    planned: items.length,
    recorded: items.filter((i) => i.state !== NOT_STEPPED).length,
    tally: tally(items),
    openComputed: sectionView(brief, "OPEN").kind === "computed",
    nextUpComputed: sectionView(brief, "NEXT_UP").kind === "computed",
  };
}

/* The one-sentence reading of the numbers above, in the interface's voice.
 *
 * ⛔ IT IS ALLOWED TO BE UNFLATTERING AND THAT IS THE REQUIREMENT. Section 11
 * requirement 15: "an ugly truth rendered honestly serves this requirement and
 * a flattering summary defeats it." Today's live answer is the first branch.
 * The other branches exist because another seat is repairing the write path
 * and these numbers WILL start moving - a view that only makes sense while
 * everything is identical would have to be rewritten the day it improves.
 */
export function verdict(p: PlanVsExec): string {
  if (p.planned === 0) return "This project has no open work items recorded.";
  if (p.recorded === 0)
    return `rig knows what it plans to do and has recorded nothing about doing any of it. All ${p.planned} items are unstepped.`;
  if (p.recorded === p.planned)
    return `Every one of the ${p.planned} planned items has a step recorded against it.`;
  const pct = Math.round((p.recorded / p.planned) * 100);
  return `${p.recorded} of ${p.planned} planned items have a step recorded against them, ${pct}%.`;
}

/* ── what this view is structurally unable to see ───────────────────────── */

/* ⛔ NOT A DISCLAIMER AND NOT AN APOLOGY. Each of these is a fact about the
 * wire that changes how the numbers above should be read, and leaving any of
 * them off would let a reader take a ratio for a completion rate.
 */
export type BlindSpot = { title: string; body: string };

export function blindSpots(brief: Brief | null): BlindSpot[] {
  const out: BlindSpot[] = [];
  out.push({
    title: "Finished work is invisible here",
    body:
      "A closed item leaves the brief's lists entirely, so nothing above " +
      "counts it. There is no total to divide by and this view does not " +
      "invent one: the denominator is what is still open, never what was " +
      "ever planned.",
  });
  const dark = sectionViews(brief).filter((s) => s.kind === "dark");
  if (dark.length)
    out.push({
      title: `${dark.length} of ${SECTION_ORDER.length} sections are unbuilt`,
      body:
        "rig answered those sections with a reason instead of data. They are " +
        "drawn hatched wherever they appear, and never as an empty result.",
    });
  const absent = sectionViews(brief).filter((s) => s.kind === "absent");
  if (absent.length)
    out.push({
      title: `${absent.length} section(s) were not mentioned at all`,
      body:
        "The daemon answered without them. That usually means it is older " +
        "than this window, and what it would have said is unknown.",
    });
  return out;
}

/* ── formatting ─────────────────────────────────────────────────────────── */

/* An age, from the wire's nanoseconds, computed at render time.
 *
 * The Go side deliberately sends an instant rather than a formatted age,
 * because the window has no live updates yet (requirement 8 is deferred) and a
 * pre-formatted age would be wrong the moment the window sat open. This turns
 * it into words at the moment of drawing, and "never" is a real answer.
 */
export function age(sinceUnixNano: number, now: number = Date.now()): string {
  if (!sinceUnixNano) return "never stepped";
  const ms = now - sinceUnixNano / 1e6;
  if (ms < 0) return "in the future";
  const m = ms / 60000;
  if (m < 1) return "just now";
  if (m < 60) return `${Math.floor(m)}m ago`;
  const h = m / 60;
  if (h < 24) return `${Math.floor(h)}h ago`;
  return `${Math.floor(h / 24)}d ago`;
}

/* ── projects against cases ─────────────────────────────────────────────── */

/* The toggle's two buckets (section 11 requirement 18).
 *
 * `kind` is a real field on the wire - ProjectBriefResponse.kind, and a case
 * has no semver and a different status vocabulary - so this split is read
 * rather than invented. "Area of interest", the third level he asked about,
 * is NOT on the wire and deliberately has no function here.
 *
 * ⛔ ANYTHING THAT IS NOT A CASE IS A PROJECT, not "anything that says
 * project". A record whose kind the daemon left empty, or one from a newer
 * daemon with a kind this build has never heard of, has to land somewhere a
 * person can reach it, and silently dropping it from both buckets is how a
 * record becomes invisible. The default side is the one that is always shown.
 */
export function splitByKind<T extends { kind: string }>(
  entries: T[],
): { projects: T[]; cases: T[] } {
  return {
    projects: entries.filter((e) => e.kind !== "case"),
    cases: entries.filter((e) => e.kind === "case"),
  };
}
