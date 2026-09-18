<!-- Every record of one kind, grouped for reading. The Spec and the Decisions.

     ⛔ BORIS, 2026-09-18: "In the GUI I don't see the utilization of the many
     goodies we have stored in `rig`." Measured the same day against his
     production store: 920 of its 1,057 current records - every requirement and
     every decision, 1.2 MB of prose - had no route to the screen at all. Not a
     bad screen. None. This is the screen.

     ⛔ IT DOES NOT GO THROUGH rigstore. That store holds one BRIEF per project
     and its contract is "read once, keep it, no timer". A record list is a
     different read with a different key (project AND kind), and threading it
     through would either widen that contract or quietly cache a second thing
     under the first thing's rules. The handoff said so; this file obeys it.

     ⛔ AND THE BODY IS RAW MARKDOWN RENDERED AS TEXT, DELIBERATELY. 1.2 MB of
     it, tables included. `white-space: pre-wrap` is the honest zero-dependency
     answer: it shows exactly what rig holds. A markdown library is a new
     frontend dependency and section 38 wants the search described before one is
     added; injecting HTML from a record body is not on the table at all. -->
<script lang="ts">
  import * as RigService from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/rigservice.js";
  import Markdown from "./Markdown.svelte";
  import type { RecordRow } from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";
  import {
    groupRecords,
    filterRecords,
    tagsOf,
    citation,
    indentOf,
    shownTally,
    groupCount,
    stripGroupDate,
  } from "./records";

  interface Props {
    project: string;
    /** "requirement" or "decision". Changing it re-reads. */
    kind: string;
    /** What the heading calls this view. */
    label: string;
    /* ⛔ THE MEASUREMENT FIXTURE, ON ItemRow's startOpen PRECEDENT. The contrast
       gate runs in a browser with no Wails runtime, so a component that only
       ever gets its rows from a binding renders an error panel for the gate and
       every colour on the real page ships unmeasured. Seeded rows change how
       the data is REACHED and never what is drawn. */
    seed?: RecordRow[] | null;
    /** Opens the first row of each group, so the gate can read an open body. */
    openRows?: boolean;
  }

  let { project, kind, label, seed = null, openRows = false }: Props = $props();

  let rows = $state<RecordRow[]>([]);
  let loading = $state(false);
  let error = $state("");
  let readAt = $state("");
  let query = $state("");

  /* One read per (project, kind), and no timer - rigstore.svelte.ts states the
     reason in full and it binds every reader in this window, not just that one:
     section 11 requirement 8 defers live updates until section 5h's bus, and a
     poll added here would be the same trap wearing a different file's name. */
  let asked = "";

  async function read(force = false) {
    if (seed) return;
    // ⛔ THE PAIR IS THE KEY AND IT IS JOINED UNAMBIGUOUSLY. Concatenated,
    // ("rig", "note") and ("rignote", "") are the same string and the second
    // read would be skipped. JSON is the shortest join that cannot collide,
    // and unlike a literal control character it is visible in the source.
    const want = JSON.stringify([project, kind]);
    if (!force && asked === want) return;
    asked = want;
    loading = true;
    error = "";
    try {
      rows = (await RigService.Records(project, kind)) ?? [];
      readAt = new Date().toLocaleTimeString([], { hour12: false });
    } catch (e) {
      rows = [];
      readAt = "";
      error = String(e);
    }
    loading = false;
  }

  $effect(() => {
    if (seed) {
      rows = seed;
      readAt = "seeded";
      return;
    }
    // Named so the dependency is the pair and nothing else: reading them here
    // is what makes a tab switch re-dial and a keystroke in the box not.
    void project;
    void kind;
    void read();
  });

  let shown = $derived(filterRecords(rows, query));
  let groups = $derived(groupRecords(shown));
  let filtering = $derived(query.trim() !== "");

  // Which rows are open, by id. A Set in $state would need re-assignment on
  // every toggle; a record of booleans is what the rest of this window uses.
  let open = $state<Record<string, boolean>>({});

  /* ⛔ IT INVERTS THE EFFECTIVE STATE, NOT THE STORED ONE, AND THE DIFFERENCE
     IS A BUG THIS ALREADY HAD. A row the fixture opened has NO entry in `open`,
     so `!open[id]` is `!undefined` - true - and the first click on a row that
     is visibly open re-opens it. It took two clicks to close. Caught by
     driving the real bundle rather than by reading it. */
  function toggle(r: RecordRow, groupKey: string) {
    open = { ...open, [r.id]: !isOpen(r, groupKey) };
  }

  function firstOf(key: string): string {
    const g = groups.find((x) => x.key === key);
    return g && g.rows.length > 0 ? g.rows[0].id : "";
  }

  function isOpen(r: RecordRow, groupKey: string): boolean {
    if (open[r.id] !== undefined) return open[r.id];
    return openRows && r.id === firstOf(groupKey);
  }
</script>

<div class="recs">
  <div class="tbar">
    <h3>{label}</h3>
    <span class="count">
      {shownTally(shown.length, rows.length)}
      {rows.length === 1 ? "record" : "records"}
    </span>

    <span class="tbar-right">
      <span class="readat">
        {#if loading}reading{:else if readAt}read at {readAt}{:else}not read yet{/if}
      </span>
      <button class="btn" onclick={() => void read(true)} disabled={loading || !!seed}>
        Read again
      </button>
    </span>
  </div>

  <div class="tools">
    <label class="find">
      <span class="lbl">Find</span>
      <input
        type="search"
        placeholder="id, title or anything in the text"
        bind:value={query}
      />
    </label>
    {#if filtering}
      <button class="btn" onclick={() => (query = "")}>Clear</button>
    {/if}
    <!-- ⛔ WHAT THE BOX CAN AND CANNOT REACH, SAID BEFORE IT IS ASKED. This one
         DOES match the full prose - unlike the brief's filter, these rows carry
         whole bodies. What it cannot do is look outside this list. B28. -->
    <p class="scope">
      Matches the full text of every {label.toLowerCase()} record on this page.
      rig has no index over its prose yet (B28), so this cannot reach the other
      kinds or anything the page has not loaded.
    </p>
  </div>

  {#if error}
    <p class="err">{error}</p>
  {:else if loading && rows.length === 0}
    <p class="none">Reading {label.toLowerCase()} records from rig.</p>
  {:else if rows.length === 0}
    <!-- Every empty state says WHY, which is this window's standing rule. -->
    <p class="none">
      rig holds no <code>{kind}</code> record for <code>{project}</code>. The
      store answered, so this is an empty list and not a failed read.
    </p>
  {:else if shown.length === 0}
    <p class="none">
      Nothing on this page matches <strong>{query}</strong>. rig cannot yet
      search its own prose (B28), so a word absent here may still be somewhere
      in the store this page did not load.
    </p>
  {:else}
    {#each groups as g (g.key)}
      {@const leadOpen = g.lead ? isOpen(g.lead, g.key) : false}
      <section class="grp">
        <!-- ⛔ THE HEADING IS THE SECTION'S OWN RECORD, DRAWN ONCE. It used to
             be drawn twice - as the heading and again as the first row beneath
             it, the same words one line apart, in all 42 sections. Seen in the
             real window before anyone complained. So the heading itself opens
             that record, and the count beside it is the SECTION's - lifting the
             head out of the rows must not drop it out of the arithmetic, or 42
             records vanish from the per-section counts while the title bar
             still says 387. -->
        <h4>
          {#if g.lead}
            <!-- ⛔ THE AFFORDANCE SITS AT THE RIGHT EDGE, IN LINE WITH EVERY
                 ROW'S. It was inline after the title, which put two different
                 +/- columns on one page a few pixels apart - the eye then
                 scans for the affordance instead of reading down it. -->
            <button
              type="button"
              class="lead"
              aria-expanded={leadOpen}
              disabled={!g.lead.body}
              onclick={() => g.lead && toggle(g.lead, g.key)}
            >
              <span class="gt">{g.title}</span>
              <span class="gn">{groupCount(g)}</span>
              <span class="chev" aria-hidden="true">
                {g.lead.body ? (leadOpen ? "−" : "+") : ""}
              </span>
            </button>
          {:else}
            <span class="gt">{g.title}</span>
            <span class="gn">{groupCount(g)}</span>
            <span class="chev" aria-hidden="true"></span>
          {/if}
        </h4>
        {#if g.note}<p class="gnote">{g.note}</p>{/if}

        {#if g.lead && leadOpen}
          <div class="detail lead-detail">
            <p class="rid">{g.lead.id}</p>
            {#if citation(g.lead)}
              <p class="cite">{citation(g.lead)}</p>
            {/if}
            <div class="body"><Markdown src={g.lead.body} /></div>
          </div>
        {/if}

        <ul class="rows">
          {#each g.rows as r (r.id)}
            {@const opened = isOpen(r, g.key)}
            <!-- ⛔ THE HEADING ALREADY SAID THE DATE. 141 of his 540 decision
                 titles opened with their own day, directly under a heading that
                 IS that day. The qualifier - which session, which generation -
                 stays, because three sessions can rule on one date. B98. -->
            {@const t = stripGroupDate(r.title || r.id, g.key)}
            <li class="row" class:open={opened} style:--ind={indentOf(r)}>
              <button
                type="button"
                class="head"
                aria-expanded={opened}
                disabled={!r.body}
                onclick={() => toggle(r, g.key)}
              >
                <span class="rt">
                  {#if t.lead}<span class="lq">{t.lead}</span>{/if}{t.text}
                </span>
                <!-- ⛔ THE SLOT IS ALWAYS PRESENT, EVEN WHEN EMPTY, AND THIS IS
                     A MEASURED DEFECT RATHER THAN ItemRow's RULE REPEATED. An
                     {#if} REMOVES the element, so on the 385 rows that are not
                     retracted the chevron became the grid's SECOND child and
                     landed in the `auto` column - 27px left of where the group
                     heading puts its own. Seen in the real window: two +/-
                     columns a thumb apart down one page. -->
                <span class="ret" class:on={r.retracted}>
                  {r.retracted ? "retracted" : ""}
                </span>
                <span class="chev" aria-hidden="true">
                  {r.body ? (opened ? "−" : "+") : ""}
                </span>
              </button>

              {#if opened}
                <div class="detail">
                  <!-- ⛔ THE ID IS A SEPARATE, SELECTABLE LINE AND IT IS NOT
                       ELLIPSISED. These ids share prefixes long enough that a
                       cut one cannot be pasted into `rig record get` - the CLI
                       says so in its own header and prints them whole for the
                       same reason. -->
                  <p class="rid">{r.id}</p>
                  {#if citation(r)}
                    <p class="cite">{citation(r)}</p>
                  {/if}
                  {#if tagsOf(r).length > 0}
                    <ul class="tags">
                      {#each tagsOf(r) as t (t)}<li>{t}</li>{/each}
                    </ul>
                  {/if}
                  <div class="body"><Markdown src={r.body} /></div>
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      </section>
    {/each}
  {/if}
</div>

<style>
  .recs {
    display: grid;
    gap: calc(1.1rem * var(--den));
    align-content: start;
    max-width: 1180px;
  }

  /* ── the toolbar, deliberately the same shape as PlanVsExec's ─────────── */

  .tbar {
    display: flex;
    align-items: baseline;
    gap: 0.9rem;
    flex-wrap: wrap;
  }

  .tbar h3 {
    margin: 0;
    font-family: var(--disp);
    font-size: var(--fs-2);
    font-weight: 600;
    letter-spacing: var(--tight-disp);
    color: var(--fg);
    line-height: 1.1;
  }

  .count,
  .readat {
    font-family: var(--mono);
    font-size: var(--fs--1);
    color: var(--fg-dim);
    font-variant-numeric: tabular-nums;
  }

  .tbar-right {
    margin-inline-start: auto;
    display: inline-flex;
    align-items: baseline;
    gap: 0.6rem;
  }

  .btn {
    font: inherit;
    font-size: var(--fs--1);
    color: var(--fg);
    background: var(--bg-2);
    border: 1px solid var(--border-2);
    border-radius: var(--radius);
    padding: 0.28rem 0.7rem;
    cursor: pointer;
  }

  .btn:hover:not(:disabled) {
    border-color: var(--hue);
  }

  .btn:disabled {
    color: var(--fg-dim);
    cursor: default;
  }

  .btn:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }

  /* ── find ─────────────────────────────────────────────────────────────── */

  .tools {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    flex-wrap: wrap;
  }

  .find {
    display: inline-flex;
    align-items: center;
    gap: 0.5rem;
  }

  .lbl {
    font-size: var(--fs--1);
    color: var(--fg-dim);
  }

  .find input {
    font: inherit;
    font-size: var(--fs--1);
    color: var(--fg);
    background: var(--bg-2);
    border: 1px solid var(--border-2);
    border-radius: var(--radius);
    padding: 0.28rem 0.6rem;
    min-width: 26ch;
  }

  .find input:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }

  .scope {
    margin: 0;
    flex-basis: 100%;
    font-size: var(--fs--1);
    line-height: 1.5;
    color: var(--fg-dim);
    max-width: 78ch;
  }

  .err {
    margin: 0;
    padding: 0.7rem 0.9rem;
    border-radius: var(--radius);
    border: 1px solid var(--sem-bad);
    background: color-mix(in srgb, var(--sem-bad) 6%, var(--panel));
    color: var(--fg);
    font-size: var(--fs--1);
  }

  .none {
    margin: 0;
    color: var(--fg-dim);
    font-size: var(--fs-0);
    line-height: 1.55;
    max-width: 72ch;
  }

  .none code,
  .none strong {
    color: var(--fg);
  }

  .none code {
    font-family: var(--mono);
  }

  /* ── one group ────────────────────────────────────────────────────────── */

  .grp {
    display: grid;
    gap: 0.35rem;
  }

  /* The same three-column grid as a row's head, so the count and the
     affordance land in the same two columns all the way down the page. */
  .grp h4 {
    margin: 0;
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto 1.2rem;
    gap: 0.5rem;
    align-items: baseline;
    font-family: var(--disp);
    font-size: var(--fs-1);
    font-weight: 600;
    letter-spacing: var(--tight-disp);
    color: var(--fg);
    padding: 0 0.3rem 0.3rem;
    border-bottom: 1px solid var(--border);
  }

  .gt {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* The heading is a control when it has a body to open, and it must not stop
     LOOKING like a heading to become one: same font, same size, same colour,
     no button chrome. The affordance is the single +/- at its end. */
  .lead {
    font: inherit;
    color: inherit;
    background: none;
    border: 0;
    padding: 0;
    margin: 0;
    text-align: start;
    cursor: pointer;
    /* It spans all three of the heading's columns and repeats them, so the
       whole heading is the target and the cells still line up with the rows. */
    grid-column: 1 / -1;
    display: grid;
    grid-template-columns: subgrid;
    align-items: baseline;
    border-radius: 4px;
  }

  .lead:disabled {
    cursor: default;
  }

  .lead:not(:disabled):hover {
    color: var(--hue);
  }

  .lead:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }

  /* The section's own text sits under its rule and above its children, with
     the same inset rule every opened body gets - so it reads as the section
     speaking, not as a first child. */
  .lead-detail {
    margin: 0.2rem 0 0.5rem 0;
  }

  .gn {
    font-family: var(--mono);
    font-size: var(--fs--1);
    font-weight: 400;
    font-variant-numeric: tabular-nums;
    color: var(--fg-dim);
  }

  .gnote {
    margin: 0;
    font-size: var(--fs--1);
    color: var(--fg-dim);
    line-height: 1.5;
    max-width: 72ch;
  }

  .rows {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  /* ── one record, and it opens ─────────────────────────────────────────── */

  /* The plan is a tree and the indent is the only thing that says so. It is
     clamped at three steps in records.ts, because level 5 exists and a fourth
     step of indent starts eating the title's width on an 1080px window. */
  .row {
    display: block;
    padding-inline-start: calc(var(--ind, 0) * 1.1rem);
  }

  .head {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto 1.2rem;
    gap: 0.5rem;
    align-items: baseline;
    width: 100%;
    font: inherit;
    text-align: start;
    background: none;
    border: 0;
    border-radius: 4px;
    padding: 0.3rem 0.3rem;
    color: inherit;
    cursor: pointer;
  }

  .head:disabled {
    cursor: default;
  }

  .head:not(:disabled):hover {
    background: var(--bg-2);
  }

  .head:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }

  .rt {
    color: var(--fg);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Which pass of that day ruled it. Dim and ahead of the title, because it
     qualifies the row rather than being what the row says. */
  .lq {
    color: var(--fg-dim);
    font-size: var(--fs--1);
    margin-inline-end: 0.5rem;
  }

  /* A retracted row is dimmed and LABELLED, never hidden - the Go side carries
     the flag for exactly this. */
  .row:has(.ret.on) .rt {
    color: var(--fg-dim);
    text-decoration: line-through;
  }

  /* Empty by default and drawing nothing - it reserves its column so the
     chevron cannot move, which is the whole reason it is not an {#if}. */
  .ret {
    font-size: 0.75rem;
    white-space: nowrap;
  }

  .ret.on {
    color: var(--sem-warn);
    border: 1px solid var(--sem-warn);
    border-radius: 999px;
    padding: 0.02rem 0.45rem;
  }

  .chev {
    color: var(--fg-dim);
    font-size: 0.85rem;
    text-align: center;
  }

  /* Inset with a rule and no tinted ground - ItemRow's note gives the reason:
     a background behind body text is the B71 defect, where the gate measures
     the token underneath and the eye reads something else. */
  .detail {
    margin: 0.1rem 0 0.7rem 0.3rem;
    padding-inline-start: 0.85rem;
    border-inline-start: 2px solid var(--border-2);
    display: grid;
    gap: 0.4rem;
  }

  .rid,
  .cite {
    margin: 0;
    font-family: var(--mono);
    font-size: 0.78rem;
    color: var(--fg-dim);
    overflow-wrap: anywhere;
    user-select: text;
  }

  .tags {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .tags li {
    font-size: 0.75rem;
    color: var(--fg-dim);
    border: 1px solid var(--border);
    border-radius: 999px;
    padding: 0.05rem 0.5rem;
  }

  /* ⛔ THIS WAS A <pre> UNDER white-space: pre-wrap AND BORIS CALLED IT UGLY.
     It showed markdown as its own punctuation: 348 of his bodies hold a table
     and every one of them read as a wall of pipes with each cell wrapped over
     four lines. Markdown.svelte now renders the tokens, so this element only
     owns selection and the text colour - the measure, the faces and the table
     grid are its. */
  .body {
    color: var(--fg);
    user-select: text;
  }
</style>
