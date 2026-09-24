package main

// The page lantern serves: its own markup and its own CSS, no rig element.
//
// The components are the program's - a status card, a row of cards and a
// toggle button - and they are styled with rig's token names, which is the
// whole of what the embedded tier gets. The status line is the demo: it says
// whether the window's token set arrived and which theme it carried, and it
// changes when the window's theme does, so a person looking at the pane can
// see the handshake worked without opening anything. The window sends the set
// more than once on load, so the count starts above 1; a switch adds one.
//
// The paint is held on the absence of pane.js's data-rig-painted marker, the
// one release hook pane.js documents for a page with no kit.css. Every token
// has a fallback, for the give-up state where no window answers.
//
//nolint:misspell // `color` is a CSS property name, which is American by spec.
const page = `<!doctype html>
<meta charset="utf-8">
<title>lantern</title>
<style>
  html:not([data-rig-painted]) { visibility: hidden }
  body { margin: 0; padding: 1.25rem 1.5rem;
         background: var(--panel, #fff); color: var(--fg, #111);
         font-family: var(--ui, system-ui), sans-serif; font-size: var(--fs-0, 14px) }
  h1 { font-size: 1.4rem; margin: 0 0 .25rem }
  .sub { color: var(--fg-dim, #444); margin: 0 0 1rem; max-width: 44rem }
  .status { display: inline-block; padding: .4rem .75rem; border-radius: 6px;
            border: 1px solid var(--border, #999); font-family: var(--mono, monospace) }
  .status.ok { border-color: var(--h-sage, green) }
  .status.none { border-color: var(--h-amber, orange) }
  .cards { display: flex; gap: .75rem; margin: 1rem 0 }
  .card { flex: 1; padding: .75rem; border-radius: 8px;
          border: 1px solid var(--border, #999); background: var(--bg, #f6f6f6) }
  .card b { display: block; font-size: 1.2rem }
  .card span { color: var(--fg-dim, #444) }
  button { font: inherit; padding: .35rem .8rem; border-radius: 6px; cursor: pointer;
           border: 1px solid var(--border, #999); background: var(--bg, #f6f6f6);
           color: var(--fg, #111) }
  button[aria-pressed="true"] { border-color: var(--hue, var(--h-sage, green)) }
</style>

<h1>Lantern</h1>
<p class="sub">An embedded-tier page: this markup and this CSS are the program's
own, and no rig element is on it. Only the colours and type come from rig,
through the token set the window hands over.</p>

<p><span id="status" class="status none">waiting for the window's tokens</span></p>

<div class="cards">
  <div class="card"><b id="n-tokens">0</b><span>tokens received</span></div>
  <div class="card"><b id="mode">none</b><span>theme from the window</span></div>
  <div class="card"><b id="changes">0</b><span>theme sets received</span></div>
</div>

<button id="toggle" aria-pressed="false">A button of lantern's own</button>

<script type="module">
import { rigPane } from "/rig/pane.js";

let changes = 0;
const status = document.getElementById("status");

rigPane({
  onTheme(mode, tokens) {
    changes++;
    document.getElementById("n-tokens").textContent = Object.keys(tokens).length;
    document.getElementById("mode").textContent = mode;
    document.getElementById("changes").textContent = changes;
    status.textContent = "themed by the window: " + mode;
    status.className = "status ok";
  },
});

// The give-up state is reachable and has to say so rather than look themed.
setTimeout(() => {
  if (document.documentElement.hasAttribute("data-rig-unthemed")) {
    status.textContent = "no window answered: unthemed";
  }
}, 1600);

const t = document.getElementById("toggle");
t.addEventListener("click", () =>
  t.setAttribute("aria-pressed", t.getAttribute("aria-pressed") === "true" ? "false" : "true"));
</script>
`
