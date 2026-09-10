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
//nolint:misspell // `color` is a CSS property name, which is American by
const page = `<!doctype html>
<meta charset="utf-8">
<title>docket</title>
<link rel="stylesheet" href="/kit/kit.css">
<style>
  /* Held until the tokens land: without this the program's own defaults show
     for a frame in whatever theme the page was authored in. */
  html { visibility: hidden }
  html.themed { visibility: visible }
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
  .pr .ext { font-size: .8em; color: var(--fg-faint) }
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
  { key: "number", label: "PR", type: "num", el: r => {
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
