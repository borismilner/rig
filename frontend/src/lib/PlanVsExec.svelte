<!-- One project or case, read as planning against execution.

     This is the body of a tab in the project/case GUI (section 11 requirement
     18: "a tab for each project/case"). It knows nothing about the tab strip
     above it; it is handed one brief and draws it.

     ⛔ BORIS NAMED THIS TAB'S SUBJECT, 2026-09-17 (section 11 requirement 14):
     "it will let me overview the planning vs execution of `rig` itself which
     is basically what our MVP does." So this is not a record browser and not
     a table of the store. Everything on it answers one question - what did we
     say we would do, and what has actually happened - and anything that does
     not answer it is chrome.

     ⛔ AND THE ANSWER IS CURRENTLY UNFLATTERING, WHICH IS THE POINT. Measured
     against live production on 2026-09-17: 59 items listed, every one of them
     unstepped. Requirement 15 makes rendering that honestly the job: "an ugly
     truth rendered honestly serves this requirement and a flattering summary
     defeats it." Nothing here infers completion, derives a kind percentage or
     hides the uniform column.

     NO LIVE UPDATES, AND THAT IS HIS RULING RATHER THAN A SHORTCUT: "We can
     have the data displayed without live updates until the mechanism is
     ready." So this reads once and offers an explicit refresh. There is no
     timer in this file. Section 5h's bus at M13 is what makes it live, and a
     poll wearing an event API's name is the trap section 11 names by name. -->
<script lang="ts">
  import type { Brief } from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";
  import {
    planVsExec,
    verdict,
    blindSpots,
    sectionView,
    sectionTally,
    stepTone,
    age,
    NOT_STEPPED,
  } from "./brief";
  import Waffle from "./Waffle.svelte";
  import Sections from "./Sections.svelte";

  interface Props {
    brief: Brief | null;
    project: string;
    /** Set while the read is in flight, so the refresh control can say so. */
    loading: boolean;
    /** Whatever went wrong on the last read, verbatim. */
    error: string;
    /** Local clock time of the last successful read, or "". */
    readAt: string;
    onrefresh: () => void;
  }

  let { brief, project, loading, error, readAt, onrefresh }: Props = $props();

  let p = $derived(planVsExec(brief));
  let t = $derived(sectionTally(brief));
  let blockedSection = $derived(sectionView(brief, "BLOCKED"));
  let notesSection = $derived(sectionView(brief, "NOTES"));
  let spots = $derived(blindSpots(brief));

  let summary = $derived(
    `${p.planned} work items, ` +
      p.tally
        .filter((x) => x.count > 0)
        .map((x) => `${x.count} ${x.state}`)
        .join(", "),
  );

  // Per-section counts, so a built-and-empty section can show a 0 while an
  // unbuilt one shows nothing at all. A section this window has no count for
  // gets null, never 0 - the same distinction the wire's status field exists
  // to keep.
  let counts = $derived({
    OPEN: brief?.open.length ?? null,
    NEXT_UP: brief?.nextUp.length ?? null,
    NOTES: brief?.notes.length ?? null,
    BLOCKED: brief?.blocked.length ?? null,
    DRIFT: null,
    MUST_READ: null,
    PROJECTION_BEHIND: null,
    PENDING: null,
    LOCAL_ONLY: null,
    FEATURES: null,
    CASE_NOTES: brief?.caseNotes.length ?? null,
  } as Record<string, number | null>);
</script>

<div class="pve">
  <div class="tbar">
    <h2>{brief?.title || project}</h2>

    <span class="chips">
      {#if brief}
        <span class="chip">{brief.kind}</span>
        <span class="chip">{brief.status}</span>
        {#if brief.semver}<span class="chip mono">{brief.semver}</span>{/if}
      {/if}
    </span>

    <span class="tbar-right">
      <span class="readat">
        {#if loading}reading{:else if readAt}read at {readAt}{:else}not read yet{/if}
      </span>
      <button class="btn" onclick={onrefresh} disabled={loading}>
        Read again
      </button>
    </span>
  </div>

  {#if error}
    <p class="err" role="alert">
      <strong>rig did not answer.</strong>
      {error}
    </p>
  {/if}

  {#if brief}
    <!-- The grid is the headline and it gets the full width. It sat in a
         column beside the numbers first, and the real window showed why that
         was wrong: the numbers made the row 240px tall and the grid, being
         two rows of cells, left most of that empty. Found by looking at a
         screenshot after every ratio had already passed. -->
    <section class="hero">
      {#if p.planned > 0}
        <div class="fig">
          <Waffle items={p.items} {summary} />
        </div>
      {:else}
        <!-- An empty grid reads as a rendering fault rather than as an empty
             project, so there is no grid to draw. -->
        <p class="nofig">
          Nothing is open here, so there is no plan to draw against.
        </p>
      {/if}

      <div class="reading">
        <div class="nums">
          <div class="pair">
            <span class="n">{p.planned}</span>
            <span class="l">planned and still open</span>
          </div>
          <div class="pair">
            <span class="n" class:zero={p.recorded === 0}>{p.recorded}</span>
            <span class="l">have any step recorded</span>
          </div>
        </div>

        <ul class="legend">
          {#each p.tally as row (row.state)}
            <li>
              <span class="sw" data-tone={stepTone(row.state)}></span>
              <span class="ls">{row.state}</span>
              <span class="lc">{row.count}</span>
            </li>
          {/each}
        </ul>
      </div>
    </section>

    <p class="verdict">{verdict(p)}</p>

    <div class="row2">
      <section class="spots">
        <h3>What this view cannot see</h3>
        <dl>
          {#each spots as s (s.title)}
            <dt>{s.title}</dt>
            <dd>{s.body}</dd>
          {/each}
        </dl>
      </section>

      <section class="cov">
        <h3>What rig can work out here</h3>
        <p class="covn">
          <strong class="t-num">{t.computed}</strong>
          of <span class="t-num">{t.total}</span> sections are built.
        </p>
        <div
          class="pips"
          role="img"
          aria-label={`${t.computed} of ${t.total} brief sections are built, ${t.dark + t.absent} are not`}
        >
          {#each Array(t.computed) as _, i (`c${i}`)}
            <span class="pip on"></span>
          {/each}
          {#each Array(t.dark + t.absent) as _, i (`d${i}`)}
            <span class="pip off"></span>
          {/each}
        </div>
        <p class="covn">
          Each unbuilt one is listed at the foot of this tab with the reason rig
          gave.
        </p>
      </section>
    </div>

    <div class="lists">
      <section>
        <h3>
          Next up
          <span class="cnt">{brief.nextUp.length}</span>
        </h3>
        {#if !p.nextUpComputed}
          {@const v = sectionView(brief, "NEXT_UP")}
          <p class="dark">{v.kind === "dark" ? v.reason : "not answered"}</p>
        {:else if brief.nextUp.length === 0}
          <p class="empty">
            Nothing is queued. rig computed this, so the list is genuinely
            empty.
          </p>
        {:else}
          <ul class="items">
            {#each brief.nextUp as it (it.id)}
              <li>
                <span class="dotm" data-tone={stepTone(it.state)}></span>
                <span class="iid">{it.id}</span>
                <span class="it">{it.title}</span>
                <span class="ist" class:none={it.state === NOT_STEPPED}>
                  {it.state === NOT_STEPPED ? age(it.sinceUnixNano) : it.state}
                </span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section>
        <h3>
          Open
          <span class="cnt">{brief.open.length}</span>
        </h3>
        {#if !p.openComputed}
          {@const v = sectionView(brief, "OPEN")}
          <p class="dark">{v.kind === "dark" ? v.reason : "not answered"}</p>
        {:else if brief.open.length === 0}
          <p class="empty">
            Nothing is open. rig computed this, so the list is genuinely empty.
          </p>
        {:else}
          <ul class="items">
            {#each brief.open as it (it.id)}
              <li>
                <span class="dotm" data-tone={stepTone(it.state)}></span>
                <span class="iid">{it.id}</span>
                <span class="it">{it.title}</span>
                <span class="ist" class:none={it.state === NOT_STEPPED}>
                  {it.state === NOT_STEPPED ? age(it.sinceUnixNano) : it.state}
                </span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    </div>

    <div class="lists pair2">
      <section>
        <h3>
          Blocked <span class="cnt"
            >{blockedSection.kind === "computed"
              ? brief.blocked.length
              : "-"}</span
          >
        </h3>
        {#if blockedSection.kind === "dark"}
          <p class="dark">{blockedSection.reason}</p>
        {:else if blockedSection.kind === "absent"}
          <p class="dark">The daemon did not answer about this section.</p>
        {:else if brief.blocked.length === 0}
          <p class="empty">
            Nothing is blocked. rig computed this, so the list is genuinely
            empty.
          </p>
        {:else}
          <ul class="items">
            {#each brief.blocked as b (b.item)}
              <li>
                <span class="dotm" data-tone="bad"></span>
                <span class="iid">{b.item}</span>
                <span class="it">
                  {b.title}
                  <span class="by"
                    >held by {b.blockers
                      .map((k) => `${k.id} (${k.state})`)
                      .join(", ")}</span
                  >
                </span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section>
        <h3>
          Notes <span class="cnt"
            >{notesSection.kind === "computed" ? brief.notes.length : "-"}</span
          >
        </h3>
        {#if notesSection.kind === "dark"}
          <p class="dark">{notesSection.reason}</p>
        {:else if notesSection.kind === "absent"}
          <p class="dark">The daemon did not answer about this section.</p>
        {:else if brief.notes.length === 0}
          <p class="empty">
            No notes. rig computed this, so the list is genuinely empty.
          </p>
        {:else}
          <ul class="items">
            {#each brief.notes as n (n.id)}
              <li>
                <span class="dotm" data-tone="none"></span>
                <span class="iid">{n.priority || n.id}</span>
                <span class="it">{n.body}</span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>
    </div>

    <!-- ⛔ THE ELEVEN SECTION STATES, IN FULL, AND THE REASON IS THE CONTENT.
         The wire says it outright: "A caller that renders a section without
         reading its status is the failure this field exists to prevent."
         Everything above this line is drawn from the five sections rig can
         compute; this is what the other six would have told you and why they
         cannot. It sits at the foot rather than in a tab of its own because
         it is about THIS project's brief, and requirement 18 gave the tabs to
         the projects. -->
    <section class="secs">
      <h3>What rig can and cannot work out about {brief.title || project}</h3>
      <Sections {brief} {counts} />
    </section>
  {:else if !loading && !error}
    <p class="empty">Nothing has been read yet. Press Read again.</p>
  {/if}
</div>

<style>
  .pve {
    display: grid;
    gap: calc(1.4rem * var(--den));
    align-content: start;
    max-width: 1180px;
  }

  /* ── the toolbar ──────────────────────────────────────────────────────── */

  .tbar {
    display: flex;
    align-items: center;
    gap: 0.9rem;
    flex-wrap: wrap;
  }

  .tbar h2 {
    margin: 0;
    font-family: var(--disp);
    font-size: var(--fs-2);
    font-weight: 600;
    letter-spacing: var(--tight-disp);
    color: var(--fg);
    line-height: 1.1;
  }

  .chips {
    display: inline-flex;
    gap: 0.35rem;
  }

  .chip {
    border: 1px solid var(--border);
    border-radius: 999px;
    padding: 0.08rem 0.55rem;
    font-size: var(--fs--1);
    color: var(--fg-dim);
  }

  .chip.mono {
    font-family: var(--mono);
  }

  .tbar-right {
    margin-inline-start: auto;
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
  }

  .readat {
    font-size: var(--fs--1);
    color: var(--fg-dim);
    font-family: var(--mono);
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

  .err {
    margin: 0;
    padding: 0.7rem 0.9rem;
    border-radius: var(--radius);
    border: 1px solid var(--sem-bad);
    background: color-mix(in srgb, var(--sem-bad) 6%, var(--panel));
    color: var(--fg);
    font-size: var(--fs--1);
  }

  /* ── the hero: the grid IS the headline ───────────────────────────────── */

  /* ⛔ THE FIGURE AND ITS READING SIDE BY SIDE, AND THIS REVERSES AN EARLIER
     DECISION FOR A REASON THAT IS MEASURABLE RATHER THAN A PREFERENCE.

     It was stacked, and the note here said two columns "put 240px of legend
     beside 40px of grid". That was true of the OLD figure: a flex-wrapped
     ragged band 40px tall. The figure is now a fixed 20-column grid, three
     rows and ~77px tall for 59 items, so the two halves are within 10px of
     each other's height and the stack instead left 780px of the box empty to
     the right of the grid - the same defect, turned ninety degrees.

     Re-measure before stacking it again; the right answer depends on the
     figure's aspect ratio and that changed. */
  .hero {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    align-items: center;
    gap: calc(1.1rem * var(--den)) 2.2rem;
    padding: calc(1.2rem * var(--den)) 1.25rem;
    background: var(--bg-2);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }

  /* ⛔ 1200, NOT 900, AND BOTH NUMBERS IN THIS COMMENT WERE MEASURED RATHER
     THAN CHOSEN.

     At a 950px viewport the `auto` track was squeezed to nothing and the
     figure collapsed into a 16px DOTTED LINE while every contrast ratio
     still passed. Caught by screenshotting at 950; the gate cannot see it,
     because a figure that is the wrong size is not a colour.

     900 -> 1040 fixed that and introduced a worse one: the installed window
     is 1080 CSS pixels wide, so the DEFAULT size sat just inside the
     two-column branch and the figure was pushed to its 240px floor - 9px
     cells where the stacked layout at 950 had 15px ones. A breakpoint that
     makes the default window the worst case is the wrong breakpoint.

     1200 is where both halves fit at full size. Below it they stack, which
     gives the figure the whole width - the half that carries the data.
     `min-width` on .fig is the second guard, so a future column change
     cannot reproduce the collapse silently. */
  @media (max-width: 1200px) {
    .hero {
      grid-template-columns: 1fr;
    }
  }

  .fig {
    min-width: 240px;
  }

  .nofig {
    margin: 0;
    color: var(--fg-dim);
    font-size: var(--fs--1);
  }

  /* The two figures over the legend, against the grid rather than under it.
     The rule is vertical now, because it separates two columns and no longer
     two stacked rows. */
  .reading {
    display: grid;
    gap: 0.9rem;
    align-content: center;
    padding-inline-start: 2.2rem;
    border-inline-start: 1px solid var(--border);
  }

  @media (max-width: 1200px) {
    .reading {
      padding: 0.9rem 0 0;
      border-inline-start: 0;
      border-top: 1px solid var(--border);
    }
  }

  .nums {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.6rem 2rem;
  }

  .pair {
    display: flex;
    align-items: baseline;
    gap: 0.55rem;
  }

  .pair .n {
    font-family: var(--mono);
    font-size: var(--fs-3);
    font-variant-numeric: tabular-nums;
    color: var(--fg);
    min-width: 2.6ch;
    text-align: end;
  }

  /* A zero that matters is still a zero. It is NOT dimmed and NOT hidden:
     dimming the number that carries the finding is the flattering summary
     requirement 15 rules out. */
  .pair .n.zero {
    color: var(--sem-warn);
  }

  .pair .l {
    color: var(--fg-dim);
    font-size: var(--fs--1);
  }

  .legend {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem 1.15rem;
  }

  .legend li {
    display: flex;
    align-items: center;
    gap: 0.45rem;
    font-size: var(--fs--1);
    color: var(--fg-dim);
  }

  /* ⛔ A BAR, NOT A SQUARE, AND THE REASON IS MEASURED RATHER THAN AESTHETIC.
     It was a 14x14 hollow rounded square first - the same mark as a waffle
     cell - and in the real window the "not stepped" one read as an unchecked
     CHECKBOX sitting next to a label, inviting a click that does nothing.
     That is the first defect in this project's own "a clean audit is not a
     clean page" list, reproduced exactly. A 18x9 bar cannot be mistaken for a
     control and still carries hollow-versus-filled. */
  .sw {
    width: 18px;
    height: 9px;
    border-radius: 2px;
    border: 1.5px solid var(--border-2);
  }

  .sw[data-tone="good"] {
    background: var(--sem-good);
    border-color: var(--sem-good);
  }
  .sw[data-tone="progress"] {
    background: var(--sem-progress);
    border-color: var(--sem-progress);
  }
  .sw[data-tone="bad"] {
    background: var(--sem-bad);
    border-color: var(--sem-bad);
  }
  .sw[data-tone="warn"] {
    background: var(--sem-warn);
    border-color: var(--sem-warn);
  }

  .lc {
    font-family: var(--mono);
    font-variant-numeric: tabular-nums;
    color: var(--fg);
  }

  /* ── the sentence ─────────────────────────────────────────────────────── */

  /* The one place the display face is used in the shell, and it is the
     sentence a person should leave with. --disp is already in the token set;
     nothing new is introduced. */
  .verdict {
    margin: 0;
    padding: 0.1rem 0 0.1rem 0.9rem;
    border-inline-start: 2px solid var(--border-2);
    font-family: var(--disp);
    font-size: var(--fs-2);
    line-height: 1.32;
    letter-spacing: var(--tight-disp);
    color: var(--fg);
    max-width: 46ch;
  }

  /* ── the two supporting blocks ────────────────────────────────────────── */

  .row2 {
    display: grid;
    grid-template-columns: minmax(0, 1.6fr) minmax(0, 1fr);
    gap: calc(1.2rem * var(--den));
    align-items: start;
  }

  @media (max-width: 860px) {
    .row2 {
      grid-template-columns: 1fr;
    }
  }

  /* ⛔ THE DISPLAY FACE ON EVERY SECTION TITLE, AND IT IS THE WHOLE OF WHY
     THIS SURFACE READ FLAT. A census of the shell's CSS found 48 of 66 type
     declarations naming --fs--1 and one naming --fs-3: nearly the entire
     product set at a single size one step BELOW body, with hierarchy left to
     font-weight, which has two usable steps. ~/me/library/index.html carries
     68 items on system fonts with no effect of any kind and stays walkable
     because a serif titles everything and a sans says everything; the token
     engine here already emits both and the shell spent --disp on one word.
     .t-sec in app.css is the face and the tracking; the step is local. */
  h3 {
    margin: 0 0 0.7rem;
    font-family: var(--disp);
    font-size: var(--fs-1);
    font-weight: 600;
    letter-spacing: var(--tight-disp);
    line-height: 1.15;
    color: var(--fg);
    display: flex;
    align-items: baseline;
    gap: 0.55rem;
  }

  .cnt {
    font-family: var(--mono);
    font-size: var(--fs--1);
    font-weight: 400;
    letter-spacing: 0;
    color: var(--fg-dim);
    font-variant-numeric: tabular-nums;
  }

  .spots dl {
    margin: 0;
    display: grid;
    gap: 0.6rem;
  }

  .spots dt {
    color: var(--fg);
    font-size: var(--fs-0);
    font-weight: 650;
  }

  .spots dd {
    margin: 0.2rem 0 0;
    color: var(--fg-dim);
    font-size: var(--fs-0);
    line-height: 1.5;
    max-width: 62ch;
  }

  .covn {
    margin: 0 0 0.6rem;
    color: var(--fg-dim);
    font-size: var(--fs-0);
    line-height: 1.5;
    max-width: 48ch;
  }

  .covn strong {
    color: var(--fg);
    font-family: var(--mono);
  }

  .pips {
    display: flex;
    gap: 4px;
    margin-bottom: 0.7rem;
  }

  .pip {
    width: 16px;
    height: 8px;
    border-radius: 2px;
  }

  .pip.on {
    background: var(--fg-dim);
  }

  /* An unbuilt section is hatched wherever it appears, here too, so the strip
     and the coverage tab say the same thing in the same language. */
  .pip.off {
    background-color: var(--bg-2);
    border: 1px solid var(--border-2);
    background-image: repeating-linear-gradient(
      -45deg,
      transparent 0 2px,
      var(--border-2) 2px 3px
    );
  }

  /* ── the lists ────────────────────────────────────────────────────────── */

  /* ONE COLUMN FOR THE ITEM LISTS, and it was two. Next up holds 5 rows and
     Open holds 54, so side by side left half the width empty for the length
     of the long list while the long list's own titles were being truncated.
     Full width gives the titles the room and costs only scroll. */
  .lists {
    display: grid;
    gap: calc(1.2rem * var(--den));
    align-items: start;
  }

  /* The short pair - blocked and notes - keeps two columns, because neither
     runs long and a full-width list of nothing is worse than a narrow one. */
  .lists.pair2 {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }

  @media (max-width: 980px) {
    .lists.pair2 {
      grid-template-columns: 1fr;
    }
  }

  .items {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 1px;
  }

  .items li {
    display: grid;
    grid-template-columns: 10px 4.5ch minmax(0, 1fr) auto;
    align-items: baseline;
    gap: 0.6rem;
    padding: 0.32rem 0.5rem;
    border-radius: 6px;
    font-size: var(--fs--1);
  }

  .items li:nth-child(odd) {
    background: var(--bg-2);
  }

  /* A bar, for the same measured reason as the legend's swatch: a small
     hollow square with a label beside it reads as an unchecked checkbox, and
     a list of them reads as a form. */
  .dotm {
    width: 12px;
    height: 6px;
    border-radius: 2px;
    border: 1.5px solid var(--border-2);
    align-self: center;
  }

  .dotm[data-tone="good"] {
    background: var(--sem-good);
    border-color: var(--sem-good);
  }
  .dotm[data-tone="progress"] {
    background: var(--sem-progress);
    border-color: var(--sem-progress);
  }
  .dotm[data-tone="bad"] {
    background: var(--sem-bad);
    border-color: var(--sem-bad);
  }
  .dotm[data-tone="warn"] {
    background: var(--sem-warn);
    border-color: var(--sem-warn);
  }

  .iid {
    font-family: var(--mono);
    color: var(--fg-dim);
  }

  .it {
    color: var(--fg);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .by {
    color: var(--fg-dim);
    margin-inline-start: 0.5rem;
  }

  .ist {
    font-family: var(--mono);
    color: var(--fg-dim);
    white-space: nowrap;
  }

  /* ⛔ NOT AMBER, AND THE REASON IS A COUNT. It was --sem-warn, and the real
     window showed 54 rows of identical amber text down one column - the hue
     stopped meaning "this wants you" and became the list's background noise.
     Section 11's rule is that a hue appears only when something wants you, so
     the warn hue is spent ONCE, on the headline zero, and the per-row state
     stays neutral. The finding is not weakened: every row still says it. */
  .ist.none {
    color: var(--fg-dim);
  }

  .secs {
    padding-top: calc(0.6rem * var(--den));
    border-top: 1px solid var(--border);
  }

  .empty,
  .dark {
    margin: 0;
    font-size: var(--fs-0);
    line-height: 1.5;
    color: var(--fg-dim);
    max-width: 62ch;
  }

  /* An unbuilt section's reason gets the same hatched ground it gets on the
     coverage tab, so it can never be mistaken for an empty result wherever a
     reader meets it. */
  /* ⛔ 30%, AND THE NUMBER IS MEASURED RATHER THAN CHOSEN. It was 55%, which
     puts the reason text at 3.73:1 dark and 3.71:1 light against a 4.5 floor -
     A FAILURE THE CONTRAST GATE STRUCTURALLY CANNOT SEE, because every pass
     reads backgroundColor and a stripe is a background-IMAGE. Measured by
     compositing --border over --bg-2 by hand in the same headless Chrome:

        stripe   --fg-dim dark / light
          55%        3.73 / 3.71     FAILS
          40%        4.54 / 4.46     light fails
          30%        5.16 / 5.01     passes both
          20%        5.86 / 5.63

     Do not push it back up without redoing that arithmetic; a green gate will
     not stop you. */
  .dark {
    padding: 0.55rem 0.7rem;
    border: 1px dashed var(--border);
    border-radius: var(--radius);
    background-color: var(--bg-2);
    background-image: repeating-linear-gradient(
      -45deg,
      transparent 0 5px,
      color-mix(in srgb, var(--border) 30%, transparent) 5px 6px
    );
  }
</style>
