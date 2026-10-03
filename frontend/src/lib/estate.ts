/* What one program is doing, in one word and one tone, shared by the Programs
 * table and the program card so the two can never disagree (plan/55,
 * requirement 42).
 *
 * Supervision's word wins when rig supervises the program. Otherwise the
 * registry's own flags say why the listing is a kept declaration (section 54),
 * and a program with neither was started by hand, which is said rather than
 * dressed up as a state.
 */

import type {
  Program,
  Running,
} from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/models.js";

export type Tone = "good" | "warn" | "bad" | "rest" | "none";

export interface ProgramState {
  word: string;
  tone: Tone;
  /* The buttons the card offers, in rig's own verbs' terms. */
  can: ("start" | "stop" | "restart")[];
}

const TONE: Record<string, Tone> = {
  healthy: "good",
  starting: "warn",
  degraded: "warn",
  restarting: "warn",
  quarantined: "bad",
  at_rest: "rest",
};

export function stateOf(p: Program, run?: Running): ProgramState {
  if (run?.state) {
    const live = run.state !== "quarantined" && run.state !== "at_rest";
    return {
      word: run.state.replace("_", " "),
      tone: TONE[run.state] ?? "none",
      can: live
        ? ["stop", "restart"]
        : run.state === "at_rest"
          ? ["start"]
          : ["restart", "stop"],
    };
  }
  if (p.down) return { word: "down", tone: "bad", can: ["start"] };
  if (p.atRest) return { word: "at rest", tone: "rest", can: ["start"] };
  return { word: "started by hand", tone: "none", can: [] };
}

// "2 h": how long a program has been in its state, from the Since the daemon
// stamped, so the page needs no clock of its own beyond now.
export function forHow(since: number, now = Date.now()): string {
  if (!since) return "";
  const s = Math.max(0, Math.round((now - since) / 1000));
  if (s < 60) return `${s} s`;
  if (s < 3600) return `${Math.round(s / 60)} min`;
  if (s < 86400) return `${Math.round(s / 3600)} h`;
  return `${Math.round(s / 86400)} d`;
}

// Requirement 41: 24-hour, never AM/PM, and the full stamp is ISO-ordered.
export function stamp(ms: number): string {
  const d = new Date(ms);
  const p = (n: number) => String(n).padStart(2, "0");
  return (
    `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ` +
    `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`
  );
}
