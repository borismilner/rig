/* Which of rig's guidelines are newer than what a program was built from
 * (plan/55 requirement 29, decision 0266), shared by Main's Programs table and
 * the Guidelines GUI so the two can never disagree.
 *
 * A rule counts against a program when it is for programs, is built (a rule
 * only ruled cannot be met yet), and is dated after the day the program's
 * commit was made. A rule dated the same day is not counted: a day cannot say
 * which came first.
 */

import type {
  Guideline,
  Guidelines,
  ProgramBuild,
} from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/models.js";

export interface Standing {
  /* "current", "N newer", or "unknown". */
  word: string;
  tone: "good" | "warn" | "none";
  newer: Guideline[];
  build?: ProgramBuild;
}

export function standing(g: Guidelines | null, program: string): Standing {
  const build = g?.programs.find((p) => p.program === program);
  if (!g || !build || !build.day) {
    return { word: "unknown", tone: "none", newer: [], build };
  }
  const newer = g.rules.filter(
    (r) => r.who === "programs" && r.built && r.date > build.day,
  );
  return newer.length
    ? { word: `${newer.length} newer`, tone: "warn", newer, build }
    : { word: "current", tone: "good", newer, build };
}

/* The programs built before a rule, by id; empty for a rule that does not
 * count against programs. */
export function builtBefore(g: Guidelines, rule: Guideline): string[] {
  if (rule.who !== "programs" || !rule.built) return [];
  return g.programs
    .filter((p) => p.day && rule.date > p.day)
    .map((p) => p.program);
}
