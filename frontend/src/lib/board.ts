/* The board's arithmetic (plan/55 requirements 2, 3, 27), kept apart from
 * the view so the tabs, the counts and the sections can be tested.
 */

import type { Card } from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/models.js";

export const SEVERITIES = ["error", "warning", "success", "info"] as const;

/* Requirement 27: "Every source" and then one tab per source, each with
 * how many of its cards are open. Every source is the empty name. */
export function sourceTabs(cards: Card[]): [string, number][] {
  const m = new Map<string, number>();
  for (const c of cards)
    m.set(c.from, (m.get(c.from) ?? 0) + (c.closed ? 0 : 1));
  const tabs = [...m.entries()].sort(([a], [b]) => a.localeCompare(b));
  return [["", cards.filter((c) => !c.closed).length], ...tabs];
}

/* The cards one tab, one search and the severity chips leave. An empty
 * severity set means every severity; a card with none counts as info. */
export function shown(
  cards: Card[],
  src: string,
  q: string,
  sev: ReadonlySet<string>,
): Card[] {
  const needle = q.trim().toLowerCase();
  return cards.filter(
    (c) =>
      (!src || c.from === src) &&
      (sev.size === 0 || sev.has(c.severity || "info")) &&
      (!needle ||
        [
          c.title,
          c.status,
          c.body,
          c.project,
          c.from,
          ...c.facts.flatMap((f) => [f.label, f.value]),
        ]
          .join(" ")
          .toLowerCase()
          .includes(needle)),
  );
}

export interface Section {
  from: string;
  open: Card[];
  closed: Card[];
  /* The most severe open card's severity, for the section's stripe. */
  worst: string;
}

/* One section per source, the source changed most recently first; inside,
 * open cards newest change first, then the closed ones. rig already sorts
 * newest first, so the order inside is kept as it came. */
export function sections(cards: Card[]): Section[] {
  const by = new Map<string, Section>();
  for (const c of cards) {
    let s = by.get(c.from);
    if (!s) {
      s = { from: c.from, open: [], closed: [], worst: "" };
      by.set(c.from, s);
    }
    (c.closed ? s.closed : s.open).push(c);
  }
  for (const s of by.values()) s.worst = worst(s.open);
  const latest = (s: Section) =>
    Math.max(
      ...[...s.open, ...s.closed].map((c) => Date.parse(c.updated) || 0),
    );
  return [...by.values()].sort((a, b) => latest(b) - latest(a));
}

/* The most severe of these cards' severities, or "" for none open. */
export function worst(cards: Card[]): string {
  for (const s of SEVERITIES)
    if (cards.some((c) => (c.severity || "info") === s)) return s;
  return "";
}

/* "21:04", a card's time on the 24-hour clock (requirement 41). */
export function hhmm(at: string): string {
  const t = new Date(at);
  if (Number.isNaN(t.getTime())) return "";
  return t.toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}
