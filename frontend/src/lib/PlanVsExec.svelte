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
    <section class="hero">
      <div class="fig">
        <Waffle items={p.items} {summary} />
      </div>

      <div class="reading">
        <div class="pair">
          <span class="n">{p.planned}</span>
          <span class="l">items planned and still open</span>
        </div>
        <div class="pair">
          <span class="n" class:zero={p.recorded === 0}>{p.recorded}</span>
          <span class="l">of them have any step recorded</span>
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
        <h3>How much of this rig can compute</h3>
        <p class="covn">
          <strong>{t.computed}</strong> of {t.total} sections are built.
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

    <div class="lists">
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

  .hero {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: calc(1.6rem * var(--den));
    align-items: start;
    padding: calc(1.2rem * var(--den)) 1.25rem;
    background: var(--bg-2);
    border: 1px solid var(--border);
    border-radius: var(--radius);
  }

  @media (max-width: 860px) {
    .hero {
      grid-template-columns: 1fr;
    }
  }

  .fig {
    min-width: 0;
  }

  .reading {
    display: grid;
    gap: 0.55rem;
    align-content: start;
    min-width: 14rem;
  }

  .pair {
    display: flex;
    align-items: baseline;
    gap: 0.55rem;
  }

  .pair .n {
    font-family: var(--mono);
    font-size: var(--fs-2);
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
    max-width: 22ch;
  }

  .legend {
    list-style: none;
    margin: 0.35rem 0 0;
    padding: 0.7rem 0 0;
    border-top: 1px solid var(--border);
    display: grid;
    gap: 0.3rem;
  }

  .legend li {
    display: grid;
    grid-template-columns: 14px 1fr auto;
    align-items: center;
    gap: 0.55rem;
    font-size: var(--fs--1);
    color: var(--fg-dim);
  }

  /* The swatch repeats the waffle's own mark, hollow and filled alike, so the
     legend reads as a key to the grid rather than as a row of buttons. A 2px
     outlined square with nothing in it reads as an empty checkbox and invites
     a click; this one is 14x14 with the same radius and border width as a
     cell, which ties it to the figure instead. */
  .sw {
    width: 14px;
    height: 14px;
    border-radius: 3px;
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
    font-family: var(--disp);
    font-size: var(--fs-1);
    line-height: 1.42;
    letter-spacing: var(--tight-disp);
    color: var(--fg);
    max-width: 62ch;
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

  h3 {
    margin: 0 0 0.6rem;
    font-size: var(--fs-0);
    font-weight: 650;
    color: var(--fg);
    display: flex;
    align-items: baseline;
    gap: 0.5rem;
  }

  .cnt {
    font-family: var(--mono);
    font-size: var(--fs--1);
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
    font-size: var(--fs--1);
    font-weight: 650;
  }

  .spots dd {
    margin: 0.15rem 0 0;
    color: var(--fg-dim);
    font-size: var(--fs--1);
    max-width: 70ch;
  }

  .covn {
    margin: 0 0 0.6rem;
    color: var(--fg-dim);
    font-size: var(--fs--1);
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

  .lists {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: calc(1.2rem * var(--den));
    align-items: start;
  }

  @media (max-width: 980px) {
    .lists {
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

  .dotm {
    width: 9px;
    height: 9px;
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

  .ist.none {
    color: var(--sem-warn);
  }

  .secs {
    padding-top: calc(0.6rem * var(--den));
    border-top: 1px solid var(--border);
  }

  .empty,
  .dark {
    margin: 0;
    font-size: var(--fs--1);
    color: var(--fg-dim);
    max-width: 70ch;
  }

  /* An unbuilt section's reason gets the same hatched ground it gets on the
     coverage tab, so it can never be mistaken for an empty result wherever a
     reader meets it. */
  .dark {
    padding: 0.55rem 0.7rem;
    border: 1px dashed var(--border);
    border-radius: var(--radius);
    background-color: var(--bg-2);
    background-image: repeating-linear-gradient(
      -45deg,
      transparent 0 5px,
      color-mix(in srgb, var(--border) 55%, transparent) 5px 6px
    );
  }
</style>
