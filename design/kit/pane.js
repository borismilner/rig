/* Joining a rig pane: the protocol, not an element.
 *
 * Separate from kit.js on purpose. Section 11 has three tiers, and the middle
 * two both need this: a program on the EMBEDDED tier brings its own components
 * and takes no elements at all, but it still gets "the token set and nothing
 * more" and still has to receive it. Locking the handshake inside the element
 * kit would make the tokens reachable only by adopting elements.
 *
 * So: pane.js alone is the embedded tier. pane.js plus kit.js is the kit tier.
 *
 * THE PROTOCOL, four messages:
 *   page -> window  {rig:1, type:"hello"}   I am up, send the tokens
 *   window -> page  {rig:1, type:"theme", mode, tokens}
 *   page -> window  {rig:1, type:"focus"}   the keyboard is mine
 *   page -> window  {rig:1, type:"blur"}    and now it is not
 *
 * The page speaks first, because the window cannot know when this script is
 * ready and a push timed from that side would race the frame's own load.
 */

/* How long to wait for the window before giving up and rendering anyway.
 *
 * This exists because of the failure it prevents, which was found by building
 * it wrong first: a page that holds its paint until the tokens arrive stays
 * invisible FOREVER if the push never comes. Holding the paint is right - it is
 * what stops a flash of the program's own defaults in the wrong theme - but it
 * has to be a race the page can lose safely. An unstyled page is a bad page; a
 * blank one is a broken program.
 */
const WAIT_MS = 1500;

/* rigPane(opts) -> { connected: Promise<{mode, tokens}> }
 *
 * opts.onTheme(mode, tokens)  called on the first set and on every change
 * opts.root                   where the custom properties are set, default <html>
 * opts.hold                   an element to keep hidden until the first set
 *                             arrives or the wait runs out
 */
export function rigPane(opts = {}) {
  const root = opts.root || document.documentElement;
  const hold = opts.hold || null;
  let painted = false;

  function reveal() {
    if (painted) return;
    painted = true;
    if (hold) hold.removeAttribute("data-rig-holding");
  }

  if (hold) hold.setAttribute("data-rig-holding", "");

  // Lost races are a result, not an error: render unstyled rather than never.
  const timer = setTimeout(() => {
    if (!painted) {
      root.setAttribute("data-rig-unthemed", "");
      reveal();
    }
  }, WAIT_MS);

  let settle;
  const connected = new Promise((res) => (settle = res));

  window.addEventListener("message", (e) => {
    // A page trusts its embedder and nothing else. It cannot know the window's
    // asset origin in advance - that is the window's own scheme, not something
    // on the wire - so the source is the check that can actually be made.
    if (e.source !== window.parent) return;
    const d = e.data;
    if (!d || d.rig !== 1 || d.type !== "theme") return;

    for (const k in d.tokens) root.style.setProperty(k, d.tokens[k]);
    root.style.colorScheme = d.mode;
    root.removeAttribute("data-rig-unthemed");

    clearTimeout(timer);
    const first = !painted;
    reveal();
    if (opts.onTheme) opts.onTheme(d.mode, d.tokens);
    if (first) settle({ mode: d.mode, tokens: d.tokens });
  });

  // Focus and blur on the window, so what is reported is "the keyboard is
  // somewhere in this document" rather than which control has it. The window
  // draws a ring around the whole pane from this, because :focus-within does
  // not cross a cross-origin frame boundary in webkit2gtk and it cannot see
  // inside here at all.
  window.addEventListener("focus", () =>
    window.parent.postMessage({ rig: 1, type: "focus" }, "*"),
  );
  window.addEventListener("blur", () =>
    window.parent.postMessage({ rig: 1, type: "blur" }, "*"),
  );

  window.parent.postMessage({ rig: 1, type: "hello" }, "*");

  return { connected };
}
