// The theme, from design/theme.js and nowhere else.
//
// Importing across the repo root rather than copying is the point: the contrast
// gate measures design/visual-system.html against that engine, so a copy here
// would be a second palette that passes no gate. Section 6 makes every one of
// these a declared, layered, live-pushed setting; until config lands (M4) the
// engine's own DEFAULTS are the layer, and applyTheme is what a live push will
// call.
// @ts-expect-error - plain ESM, and svelte-check is out while section 22 pins TypeScript 7
import { DEFAULTS, tokens } from "../../../design/theme.js";

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

export function applyTheme(
  mode: Mode,
  root: HTMLElement = document.documentElement,
): void {
  const t = tokens(DEFAULTS, mode) as Record<string, string>;
  for (const [k, v] of Object.entries(t)) root.style.setProperty(k, v);
  for (const [k, v] of Object.entries(SHELL_CONSTANTS))
    root.style.setProperty(k, v);
  root.style.colorScheme = mode;
  // The shell is achromatic until a program is selected, and a program without
  // a declared hue leaves it achromatic (section 11). Nothing declares one yet.
  setHue(null, root);
}

// setHue is the whole of the shell's colour policy: the current program's hue
// is the only saturated colour on screen while you are in it, and no program
// ever owns an alarm hue.
export function setHue(
  hue: string | null,
  root: HTMLElement = document.documentElement,
): void {
  root.style.setProperty("--hue", hue ? `var(--h-${hue})` : "var(--fg)");
  root.style.setProperty("--ohue", hue ? `var(--o-${hue})` : "var(--fg)");
}
