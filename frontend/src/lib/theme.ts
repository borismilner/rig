// The theme, from design/theme.js and nowhere else.
//
// Importing across the repo root rather than copying is the point: the contrast
// gate measures design/visual-system.html against that engine, so a copy here
// would be a second palette that passes no gate. Section 6 makes every one of
// these a declared, layered, live-pushed setting; until config lands (M4) the
// engine's own DEFAULTS are the layer, and applyTheme is what a live push will
// call.
// @ts-expect-error - plain ESM, and svelte-check is out while section 22 pins TypeScript 7
import {
  DEFAULTS,
  tokens,
  checkTheme,
  explainCheck,
} from "../../../design/theme.js";

export type Mode = "dark" | "light";

// What the engine does not emit, and it should.
//
// design/theme.js returns 47 tokens and none of these five, so the design page
// declares them in its own <style> and any second surface has to repeat them.
// That is a drift risk with a page the contrast gate trusts: --ring-w is the
// focus ring's width, and section 20's gate went red on a ring once already.
// Copied verbatim from design/visual-system.src.html:53-59, and owed back to
// the engine as one commit that moves them into tokens().
const SHELL_CONSTANTS: Record<string, string> = {
  "--ring-w": "3px",
  "--shadow-lg": "0 2px 4px rgba(0,0,0,.3),0 24px 70px -12px rgba(0,0,0,.62)",
  "--shadow": "0 1px 2px rgba(0,0,0,.4),0 10px 34px rgba(0,0,0,.38)",
  "--shadow-sm": "0 1px 2px rgba(0,0,0,.32)",
  "--spring":
    "linear(0,.006,.025 2.8%,.101 6.1%,.539 18.9%,.721 25.3%,.849 31.5%," +
    ".937 38.1%,.968 41.8%,.991 45.7%,1.006 50.1%,1.015 55%,1.017 63.9%,1.001)",
  "--ease": "cubic-bezier(.2,.8,.3,1)",
};

// The mode the page should be in: an explicit data-theme wins, and otherwise
// the desktop's own preference. data-theme is not an arbitrary choice of
// attribute - tools/contrast-audit.mjs flips exactly that one to measure a page
// in both themes, so honouring it is what makes the shell auditable by the gate
// that already exists rather than by a second one written for it.
export function preferredMode(
  root: HTMLElement = document.documentElement,
): Mode {
  const declared = root.getAttribute("data-theme");
  if (declared === "dark" || declared === "light") return declared;
  return window.matchMedia("(prefers-color-scheme: light)").matches
    ? "light"
    : "dark";
}

// Re-apply whenever data-theme changes, so the gate's flip lands and, later, so
// a live push from section 6 needs no new plumbing. Returns its own disconnect.
export function watchMode(onchange: (mode: Mode) => void): () => void {
  const root = document.documentElement;
  const obs = new MutationObserver(() => onchange(preferredMode(root)));
  obs.observe(root, { attributes: true, attributeFilter: ["data-theme"] });

  const mq = window.matchMedia("(prefers-color-scheme: light)");
  const onmq = () => {
    if (!root.hasAttribute("data-theme")) onchange(preferredMode(root));
  };
  mq.addEventListener("change", onmq);

  return () => {
    obs.disconnect();
    mq.removeEventListener("change", onmq);
  };
}

// The whole token set for a mode, as one object.
//
// It exists because the set now has a second consumer: a program serving its
// own pane is cross-origin, so it cannot read this document's custom
// properties and has to be handed them (section 11's embedded tier gets "the
// token set and nothing more"). Building that payload from a copy would be the
// second palette this file exists to prevent, so applyTheme and the push both
// come from here.
//
// The hue pair is included and is deliberately a reference rather than a
// colour: R6 says an element reads a token and never learns a program id, so
// what crosses is --hue, whatever it currently points at. It resolves in the
// program's page because --fg and --h-* travel in the same payload.
export function tokenSet(
  mode: Mode,
  hue: string | null = null,
): Record<string, string> {
  return {
    ...(tokens(DEFAULTS, mode) as Record<string, string>),
    ...SHELL_CONSTANTS,
    "--hue": hue ? `var(--h-${hue})` : "var(--fg)",
    "--ohue": hue ? `var(--o-${hue})` : "var(--fg)",
  };
}

// The hue argument is the whole of the shell's colour policy: the current
// program's hue is the only saturated colour on screen while you are in it, and
// no program ever owns an alarm hue. Nothing declares one yet, so it is null
// everywhere and the shell is achromatic, which section 11 makes the documented
// default rather than a gap.
export function applyTheme(
  mode: Mode,
  hue: string | null = null,
  root: HTMLElement = document.documentElement,
): void {
  for (const [k, v] of Object.entries(tokenSet(mode, hue)))
    root.style.setProperty(k, v);
  root.style.colorScheme = mode;
}

// ── the live theme, and the gate that guards it ─────────────────────────────
//
// Section 6 makes `ui.theme` a declared, layered, live-pushed setting, and says
// the engine's own DEFAULTS are the layer until config lands in M4. So this
// holds ONE mutable overlay in memory: not persistence, which is M4's, but a
// real theme object that the settings UI edits and the engine renders. When
// config arrives, this object is what a live push replaces and nothing above
// it has to change.

export type Theme = Record<string, any>;
export type Finding = {
  token: string;
  ground: string;
  ratio: number;
  need: number;
  kind: string;
  colour: string;
};
export type CheckResult = {
  ok: boolean;
  findings: Finding[];
  unsolvable: { token: string; need: number }[];
  mode: Mode;
};

const clone = (o: Theme): Theme => JSON.parse(JSON.stringify(o));

let theme: Theme = clone(DEFAULTS);

export const getTheme = (): Theme => theme;
export const defaultTheme = (): Theme => clone(DEFAULTS);

// The judgement, straight from the engine. Re-exported rather than
// reimplemented: the window, the design page and the build gate have to agree
// about what "unreadable" means, and three copies would not.
export function check(candidate: Theme, mode: Mode): CheckResult {
  return checkTheme(candidate, mode) as CheckResult;
}
export function explain(result: CheckResult): string[] {
  return explainCheck(result) as string[];
}

/* Try to make `candidate` the live theme.
 *
 * REFUSES rather than applying when the token set is unreadable, which is the
 * load-bearing half of section 23's M1a row: a settings UI that changes the
 * theme but accepts an unreadable one has not closed the step. Nothing is
 * written to the document on refusal, so the page a person is reading stays
 * readable - the failure mode to avoid is a theme that makes the settings UI
 * itself unreadable, which would leave no way back.
 *
 * BOTH modes are judged, not just the one on screen. Applying a set that is
 * fine in dark and unreadable in light would make the other mode a trap the
 * person discovers later, and one theme is half a measurement.
 */
export function tryApply(
  candidate: Theme,
  mode: Mode,
  root: HTMLElement = document.documentElement,
): { ok: boolean; refused: CheckResult[] } {
  const refused = (["dark", "light"] as Mode[])
    .map((m) => check(candidate, m))
    .filter((r) => !r.ok);
  if (refused.length) return { ok: false, refused };

  theme = clone(candidate);
  applyWith(theme, mode, root);
  return { ok: true, refused: [] };
}

// Apply a specific theme object. Kept separate from applyTheme so the token
// payload the pane receives and the tokens on the shell's own root come from
// the same call with the same object - two paths here is how a program's pane
// ends up a theme behind the window it sits in.
export function applyWith(
  candidate: Theme,
  mode: Mode,
  root: HTMLElement = document.documentElement,
): void {
  const set = {
    ...(tokens(candidate, mode) as Record<string, string>),
    ...SHELL_CONSTANTS,
    "--hue": "var(--fg)",
    "--ohue": "var(--fg)",
  };
  for (const [k, v] of Object.entries(set)) root.style.setProperty(k, v);
  root.style.colorScheme = mode;
}

// The token payload for a program's pane, built from the LIVE theme rather
// than from DEFAULTS, or a program keeps the colours the window has left.
export function liveTokenSet(mode: Mode): Record<string, string> {
  return {
    ...(tokens(theme, mode) as Record<string, string>),
    ...SHELL_CONSTANTS,
    "--hue": "var(--fg)",
    "--ohue": "var(--fg)",
  };
}

/* An explicit mode choice is `data-theme` on the root, and "system" is its
 * ABSENCE - which is exactly what preferredMode() already reads and what
 * watchMode() observes, so a choice made here needs no second mechanism.
 *
 * It is also the attribute tools/contrast-audit.mjs flips to measure a page in
 * both themes. That is not a coincidence worth breaking: making the settings UI
 * write anything else would leave the gate measuring a theme the product cannot
 * actually be put into.
 */
export type ModeChoice = Mode | "system";

export function setModeChoice(
  choice: ModeChoice,
  root: HTMLElement = document.documentElement,
): Mode {
  if (choice === "system") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", choice);
  const resolved = preferredMode(root);
  applyWith(theme, resolved, root);
  return resolved;
}

export function modeChoice(
  root: HTMLElement = document.documentElement,
): ModeChoice {
  const d = root.getAttribute("data-theme");
  return d === "dark" || d === "light" ? d : "system";
}
