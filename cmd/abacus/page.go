package main

// The page docket serves. Modelled on dispatch's frontend/src/pages/CiBoard.tsx,
// read rather than remembered: seven columns, a pill-toggle filter, and an
// empty message whose text depends on which filter is on.
//
// What this file is FOR is the cells. On pull-report almost every cell is text,
// which is why ledger never touched the question. On this board the PR is a
// link with a mark, the assignment is a pill, the state is a chip and the
// builds are a matrix of squares - four of seven columns are not text, and one
// more is a right-aligned count. So `a cell is a thing rather than a string` is
// the normal case here, and the kit's answer to it is what step 4 was for.
func pane() string { return page }

// spec. The repo's UK locale is deliberate and right for prose, so this is
// exempted where it meets CSS rather than weakened everywhere. Same directive
// as cmd/ledger/page.go's, and written with no space after the slashes because
// nolintlint rejects the spaced form.
//
// How the paint is held, and the bug that was found by looking at the pane.
//
// The first version of this page hid <html> outright and released it with an
// `html.themed` rule. Nothing in rig has ever set a `themed` class: pane.js
// reveals either by removing `data-rig-holding` from an element the program
// registers as opts.hold, or, on the give-up timer, by setting
// `data-rig-unthemed` on the root. So this pane painted NOTHING, in either
// theme, on every path, from the day it was written - and it still served
// /pane, still registered over the wire, still appeared in `rig apps list`
// and still passed both contrast gates, because neither gate looks at a fake
// application's pane. It was found the first time a person opened it.
//
// It now gates on the ABSENCE of `data-rig-painted`, which pane.js sets
// unconditionally in reveal(). That cannot deadlock - the timer reveals even
// when no window is listening - and it needs no registered element, so it is
// also the form an EMBEDDED-tier program can use. opts.hold is for holding a
// subtree, which is what cmd/ledger wants and this page does not.
//
// The explanation lives here rather than in the CSS deliberately: the string
// below is shipped to the browser on every pane load, and an essay about a
// fixed bug cost docket 4096 bytes against its size ratchet when it was
// written there.
//
//nolint:misspell // `color` is a CSS property name, which is American by
const page = `<!doctype html>
<meta charset="utf-8">
<title>docket</title>
<link rel="stylesheet" href="/kit/kit.css">
<style>
  /* Held until the tokens land, on the absence of pane.js's marker. See the
     Go comment on this const for why it is not a class. */
  html:not([data-rig-painted]) { visibility: hidden }

  /* The give-up state, which pane.js guarantees reaching and this page has to
     survive. Measured cold: with no token set, NOTHING is defined, so
     a background of var(--fg-faint) computes to transparent and the whole
     Builds column disappears while its boxes keep their 8x13 space - a column of
     silently missing information, which is worse than a wrong colour. The
     page's own text computed to black at the same time.

     One block rather than 21 per-property fallbacks, keyed on the attribute
     pane.js sets when it gives up. Values are design/theme.js's dark output,
     copied rather than invented so this is not a second palette. It is still a
     copy and will drift; the engine emitting a fallback set that pane.js can
     apply on the timeout path is the real fix and is a kit decision, not this
     program's to take. */
  html[data-rig-unthemed] {
    --fg: #dae5f3; --fg-dim: #a1b1c5; --fg-faint: #8e9fb1;
    --panel: #212a34; --border: #64778c; --hue: #dae5f3;
    --h-sage: #92cf9a; --h-amber: #e1b673;
  }
  body { margin: 0; min-height: 100vh; box-sizing: border-box;
         padding: 1rem 1.15rem 1.6rem;
         background: var(--panel); color: var(--fg);
         font-family: var(--ui, system-ui), sans-serif;
         font-size: var(--fs-0, 14px) }

  /* The filter, and it is the program's own markup on purpose. rigToolbar
     always makes a search input and has no segmented control, so a board that
     filters by three named states cannot be one. Written here rather than
     bent into the kit, because which of those two happens is the decision
     step 4 exists to inform. */
  .filters { display: flex; gap: .4rem; flex-wrap: wrap; margin: 0 0 .9rem }
  .filters button {
    font: inherit; font-size: var(--fs--1, .85rem);
    padding: .2rem .7rem; border-radius: 999px; cursor: pointer;
    border: 1px solid var(--border);
    background: transparent; color: var(--fg-dim);
    text-transform: uppercase; letter-spacing: .06em }
  .filters button[aria-pressed="true"] {
    border-color: var(--hue); color: var(--fg);
    background: color-mix(in srgb, var(--hue) 13%, transparent) }
  .filters button:focus-visible {
    outline: none; box-shadow: 0 0 0 var(--ring-w, 3px) var(--hue) }

  /* Cells that are not text. Every colour here is a rig token: a program
     brings its own components in this tier and still gets rig's palette. */
  .pr { display: inline-flex; align-items: baseline; gap: .35rem;
        font-family: var(--mono, monospace); color: var(--fg);
        text-decoration: none }
  .pr:hover { text-decoration: underline }
  /* var(--fs--2), not .8em: the scale carries a max(12px, ...) floor and a
     raw em walks around it. .8em of the cell size is 10.16px. */
  .pr .ext { font-size: var(--fs--2, max(12px, .8em)); color: var(--fg-faint) }
  .asg { display: inline-block; font-family: var(--mono, monospace);
         font-size: var(--fs--2, .78rem);
         padding: .05rem .5rem; border-radius: 999px;
         border: 1px solid var(--border); color: var(--fg-dim) }
  .chip { display: inline-block; font-size: var(--fs--2, .78rem);
          padding: .05rem .5rem; border-radius: 999px;
          border: 1px solid currentColor }
  .chip.ok { color: var(--h-sage) }
  .chip.bad { color: var(--h-rose, var(--hue)) }
  .chip.run { color: var(--h-amber) }
  .chip.idle { color: var(--fg-faint) }
  .mx { display: inline-flex; gap: 2px }
  .mx i { width: .5rem; height: .8rem; border-radius: 1px;
          background: var(--fg-faint) }
  .mx i.ok { background: var(--h-sage) }
  .mx i.bad { background: var(--h-rose, var(--hue)) }
  .mx i.run { background: var(--h-amber) }
</style>

<h1 class="rig-heading">CI board</h1>
<p class="rig-lead">
  Every pull request with an assignment behind it, and what its checks are
  doing. One table, which is what <code class="rig-mono">dispatch</code>
  actually has.
</p>

<div class="filters" id="filters" role="group" aria-label="Filter runs"></div>

<div class="rig-tablewrap"><table id="t-ci"></table></div>

<div id="panel-caveat" style="margin-block-start:1.1rem"></div>

<script type="module">
import { rigTable, rigPanel } from "/kit/kit.js";
import { rigPane } from "/kit/pane.js";

rigPane();

const ROWS = [
  { repo: "rig",       number: 118, title: "the element kit",        asg: "m1a-3",  state: "open",   builds: ["ok","ok","ok"],   threads: 2,  merge: "clean",    age: "4h" },
  { repo: "dispatch",  number: 92,  title: "retry the poller",       asg: "ci-7",   state: "open",   builds: ["ok","bad","ok"],  threads: 11, merge: "blocked",  age: "2d" },
  { repo: "archi",     number: 43,  title: "edge routing",           asg: null,     state: "draft",  builds: ["run","idle","idle"], threads: 0, merge: "unknown", age: "6d" },
  { repo: "shelf",     number: 7,   title: "admission checklist",    asg: "lib-2",  state: "open",   builds: ["ok","ok","ok"],   threads: 1,  merge: "clean",    age: "9h" },
  { repo: "snapper",   number: 21,  title: "gio 0.9",                asg: "snap-1", state: "merged", builds: ["ok","ok","ok"],   threads: 4,  merge: "merged",   age: "3w" },
  { repo: "dispatch",  number: 95,  title: "drop the alpha runtime", asg: "ci-9",   state: "open",   builds: ["bad","bad","run"],threads: 6,  merge: "blocked",  age: "1d" },
];

const el = (tag, cls, text) => {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text != null) n.textContent = text;
  return n;
};

const TONE = { ok: "ok", clean: "ok", merged: "ok", bad: "bad", blocked: "bad",
               run: "run", open: "run", draft: "idle", idle: "idle",
               unknown: "idle" };

// Seven columns, four of which are not text. c.el returns a NODE, so the cell
// is built with the DOM rather than handed to the kit as a string of HTML -
// which is what the kit used to offer and what a repo name or a PR title would
// have been interpolated into.
const COLS = [
  { key: "number", label: "PR", type: "num", align: "l", el: r => {
      const a = el("a", "pr");
      a.href = "https://example.invalid/" + r.repo + "/" + r.number;
      a.target = "_blank"; a.rel = "noreferrer"; a.title = r.title;
      a.append(el("span", null, r.repo + "#" + r.number), el("span", "ext", "↗"));
      return a;
    } },
  { key: "asg", label: "Assignment", type: "str", el: r =>
      r.asg ? el("span", "asg", r.asg) : el("span", "rig-mono", "—") },
  { key: "state", label: "State", type: "str", el: r =>
      el("span", "chip " + (TONE[r.state] || "idle"), r.state) },
  { key: "builds", label: "Builds", type: "str", el: r => {
      const m = el("div", "mx");
      r.builds.forEach(b => m.appendChild(el("i", TONE[b] || "idle")));
      return m;
    } },
  { key: "threads", label: "Threads", type: "num" },
  { key: "merge", label: "Merge", type: "str", el: r =>
      el("span", "chip " + (TONE[r.merge] || "idle"), r.merge) },
  { key: "age", label: "Age", type: "str", cls: "rig-r rig-mono" },
];

const FILTERS = [
  { id: "all",     label: "all",     keep: () => true,
    empty: "No linked PRs yet." },
  { id: "failing", label: "failing", keep: r => r.builds.includes("bad"),
    empty: "No failing runs." },
  { id: "running", label: "running", keep: r => r.builds.includes("run"),
    empty: "Nothing running right now." },
];

let active = FILTERS[0];

// emptyText is a FUNCTION because this board says something different for each
// filter, exactly as dispatch does. A single string could not.
const table = rigTable(document.getElementById("t-ci"), COLS, ROWS, {
  sortKey: "age",
  emptyText: () => active.empty,
});

const bar = document.getElementById("filters");
FILTERS.forEach(f => {
  const b = el("button", null, f.label);
  b.type = "button";
  b.setAttribute("aria-pressed", String(f === active));
  b.addEventListener("click", () => {
    active = f;
    [...bar.children].forEach(c =>
      c.setAttribute("aria-pressed", String(c === b)));
    table.setRows(ROWS.filter(f.keep));
  });
  bar.appendChild(b);
});

rigPanel(
  document.getElementById("panel-caveat"),
  "A green board is not a merged board",
  "checks only, no review state",
).appendChild(
  el("p", "rig-lead", "Builds are the three required checks. A row can be all "
    + "green and still be blocked on a review this board never asked about."));
</script>
`
