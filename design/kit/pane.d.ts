/* Types for pane.js, the handshake that joins a program's page to a rig
 * pane (section 11). pane.js is the whole embedded tier and half of the kit
 * tier; these types are for a program whose frontend is TypeScript. Keep them
 * in step with pane.js: the options and the return value below are the ones
 * its header documents. */

/** The window's theme mode, as pushed in the `theme` message. */
export type RigMode = "light" | "dark";

/** The token set: CSS custom property names (`--fg`, `--panel`, ...) to values. */
export type RigTokens = Record<string, string>;

export interface RigPaneOptions {
  /** Called on the first token set and on every theme change. */
  onTheme?: (mode: RigMode, tokens: RigTokens) => void;
  /** Where the custom properties are set. Default: `document.documentElement`. */
  root?: HTMLElement;
  /** A subtree kept hidden until the first set arrives or the wait runs out. Needs kit.css. */
  hold?: HTMLElement | null;
}

export interface RigPane {
  /** Resolves with the first token set. Never rejects: if no window answers,
   * the page is revealed unthemed and `data-rig-unthemed` is set on the root. */
  connected: Promise<{ mode: RigMode; tokens: RigTokens }>;
}

/** Say hello to the window and receive its token set. Call once, early. */
export function rigPane(opts?: RigPaneOptions): RigPane;
