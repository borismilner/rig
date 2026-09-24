/* What the rail lists, and its unit is a GUI rather than a program.
 *
 * ⛔ BORIS, 2026-09-17 (section 11 requirement 16, which SUPERSEDES 12):
 * "Maybe the things that can be selected from the left are not only the
 * different programs but we can generalize it into different GUIs. So a GUI
 * can be a GUI of a program but it could also be a GUI of some internal `rig`
 * functionality like project management and things like that."
 *
 * ⛔ AND IT DISSOLVES THE PROMOTION PROBLEM RATHER THAN SOLVING IT. Section 5's
 * "PROMOTION IS DEAD" closed four doors against rig being a registered
 * program - no CapabilityMap entry, `meta` refuses "rig" as an invoke target,
 * `d.call` routes through `d.programs[program]`, and putting rig in the map
 * would place `rig.down` on the agent invoke surface. A GUI is not a program,
 * so an internal GUI in the rail asserts nothing about the registry and every
 * one of those closures stands. Nothing in this file registers anything.
 */

export type InternalGui = {
  id: string;
  /** What the rail's tooltip and the context bar call it. */
  title: string;
  /** One line, in the context bar, saying what the GUI is for. */
  note: string;
  /* ⛔ THE HUE, AND ONLY AN INTERNAL GUI MAY HAVE ONE (requirement 17).
   *
   * It is a member name from design/theme.js's own set, never a colour: the
   * shell resolves it to var(--h-<name>), so it moves with the theme and the
   * contrast gate already measures every member.
   *
   * WHY indigo AND NOT ONE OF THE OTHERS. The engine ships seven hues and five
   * carry a semantic role - rust is bad, amber is warn, sage is good, teal is
   * progress, steel is info. Only indigo and lilac carry none, so an identity
   * hue taken from either cannot be misread as a status. Lilac is left free
   * for the next internal GUI.
   */
  hue: string;
  /** A 16x16 path, drawn in the rail and nowhere else. */
  icon: string;
};

/* ⛔ EMPTY, AND EMPTY IS A STATE RATHER THAN A GAP.
 *
 * It held PROJECT_CASE_GUI, the project and case GUI, wearing `indigo`. That
 * GUI left this window at plan/50 move 7: the planner is a program now, it
 * serves its own pane, and rig draws it through the pane tier like any other
 * program's. Requirement 16's mechanism stays because requirement 16 is his
 * and the next internal capability goes straight in here.
 *
 * ⛔ WHOEVER ADDS THE NEXT ONE HAS TO PUT THE BRANCH BACK IN App.svelte.
 * While this list is empty `internalGui()` answers null for every id, so the
 * shell's `gui` is always null and the branch that drew an internal GUI's own
 * layout was removed with the GUI. A new entry here alone would put a rail
 * icon on screen that lands on the unserved pane.
 *
 * `indigo` and `lilac` are both free again; the other five carry a semantic
 * role and an identity hue must not be mistaken for a status.
 */
export const INTERNAL_GUIS: InternalGui[] = [];

export function internalGui(id: string | null): InternalGui | null {
  if (!id) return null;
  return INTERNAL_GUIS.find((g) => g.id === id) ?? null;
}
