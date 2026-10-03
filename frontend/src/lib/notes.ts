/* The notifications panel's arithmetic (plan/55 requirements 7 to 9, 23 to
 * 26), shared by the panel, Needs you, Main's figure and the card so they
 * can never disagree about what still asks.
 */

import type { Note } from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/models.js";

/* What still waits on him: it asked, nobody has answered, and the program
 * is still waiting (rig has not forgotten the question). */
export const waiting = (n: Note): boolean => n.waiting && !n.answer;

/* Requirement 23: what asks goes on top, then newest first. The Go side
 * already sorts by time, so a stable sort keeps that order inside each. */
export function arrange(notes: Note[], q: string, src: string): Note[] {
  const needle = q.trim().toLowerCase();
  return notes
    .filter((n) => !src || n.sender === src)
    .filter(
      (n) =>
        !needle ||
        [n.title, n.body, n.sender, n.severity, n.replies.join(" ")]
          .join(" ")
          .toLowerCase()
          .includes(needle),
    )
    .sort((a, b) => Number(waiting(b)) - Number(waiting(a)));
}

/* Requirement 9: every source, with how many each sent, by name. */
export function sources(notes: Note[]): [string, number][] {
  const m = new Map<string, number>();
  for (const n of notes) m.set(n.sender, (m.get(n.sender) ?? 0) + 1);
  return [...m.entries()].sort(([a], [b]) => a.localeCompare(b));
}

/* The colour a severity is drawn in. info is the neutral steel; the three
 * that want him take the semantic hues. */
export function sevHue(s: string): string {
  switch (s) {
    case "success":
      return "var(--sem-good)";
    case "warning":
      return "var(--sem-warn)";
    case "error":
    case "urgent":
      return "var(--sem-bad)";
    default:
      return "var(--h-steel)";
  }
}

/* How an answered notification was answered, in words. */
export function answered(n: Note): string {
  const a = n.answer;
  if (!a) return "";
  const who = a.by ? ` (${a.by})` : "";
  if (a.dismissed) return `dismissed${who}`;
  if (a.reply && a.text) return `"${a.reply}": ${a.text}${who}`;
  if (a.reply) return `"${a.reply}"${who}`;
  return `"${a.text}"${who}`;
}

/* "12 min", "3 h", "2 d" since an RFC 3339 stamp. */
export function ago(at: string, now = Date.now()): string {
  const t = Date.parse(at);
  if (Number.isNaN(t)) return "";
  const s = Math.max(0, Math.round((now - t) / 1000));
  if (s < 60) return "now";
  if (s < 3600) return `${Math.floor(s / 60)} min`;
  if (s < 86400) return `${Math.floor(s / 3600)} h`;
  return `${Math.floor(s / 86400)} d`;
}
