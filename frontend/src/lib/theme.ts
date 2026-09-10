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
