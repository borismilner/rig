<!-- The main GUI: a general-purpose dashboard, and a destination in its own
     right rather than the empty state before a program is picked.

     ⛔ BORIS, 2026-09-17 (section 11, requirement 6 as replaced): "the main GUI
     show show a general-purpose dashboard with all the most important
     information we'll define in the future." ITS CONTENT IS DELIBERATELY
     DEFERRED and filling it is explicitly not a seat's call, so what is here
     is the honest first payload: everything the window can already answer
     without a new wire verb, and nothing else.

     ⛔ NOTHING ON THIS PAGE IS INVENTED. Every number is read from the three
     calls the window already makes - Health, Programs, Build. There is no
     uptime, no error rate, no activity feed and no
     sparkline, because nothing on this wire carries any of them, and a metric
     with no data behind it is worse on a dashboard than a gap. -->
<script lang="ts">
  import type {
    Deployment as DeploymentState,
    Health,
    Program,
  } from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/models.js";
  import { INTERNAL_GUIS } from "./guis";
  import Deployment from "./Deployment.svelte";

  interface Props {
    health: Health;
    programs: Program[];
    build: Record<string, string> | null;
    lastRead: string;
    /* ⛔ WHAT IS RUNNING, AND IT REPLACED A CARD THAT RESTATED THE PROJECT
       VIEW. The rail already lands on that view directly, so this page linking
       to it was duplicating its own destination - the "strange partial
       dashboard" Boris named on 2026-09-17. */
    deployment: DeploymentState | null;
    /** Selects a registered program, also the rail's door. */
    onselect: (id: string) => void;
  }

  let { health, programs, build, lastRead, deployment, onselect }: Props =
    $props();

  // Declared facts, summed. Coverage is a field every program declares at
  // registration, so "3 declared full coverage" is a reading rather than a
  // guess about how complete anything is.
  let full = $derived(programs.filter((x) => x.coverage === "full").length);
  let commands = $derived(programs.reduce((n, x) => n + (x.commands || 0), 0));
  let ownPane = $derived(programs.filter((x) => !!x.paneUrl).length);
  let hosted = $derived(programs.filter((x) => x.hosted).length);
  let services = $derived(
    new Set(programs.flatMap((x) => x.services ?? [])).size,
  );

  let buildRows = $derived(Object.entries(build ?? {}));

  /* ⛔ THE CENSUS, AND IT IS THE LIBRARY PAGE'S DEVICE RATHER THAN A NEW ONE.
   *
   * ~/me/library/index.html leads with "68 gold, 9 below the bar across 27
   * topics" and repeats a count beside every rail entry, every filter and
   * every section title. That is what makes a set of 68 feel walkable, and it
   * is the thing the dashboard was missing: it had six numbers, all of them
   * buried inside a panel at the same weight as their own labels.
   *
   * ⛔ EVERY FIGURE BELOW IS READ, NEVER DERIVED INTO A SCORE. There is no
   * percentage, no ratio and no total-planned-ever, because the wire carries
   * none of them - requirement 14's consequence, and the reason `recorded`
   * sits next to `planned` as two separate figures rather than as "0%".
   */
  type Figure = {
    n: string;
    label: string;
    /* True when the number IS the finding and must not be softened. Exactly
       one figure may claim this, or the emphasis means nothing. */
    loud?: boolean;
  };

  /* ⛔ TWO FIGURES, AND THE TWO THAT WENT WERE THE PLANNER'S. This strip
     also carried "open work items in rig's own plan" and "with any step
     recorded", read from a brief. plan/50 move 7 moved the planner's views
     out of this window, and a dashboard that kept counting a plan rig no
     longer holds would be the invented-metric failure the note at the top of
     this file refuses. The planner declares its own pane and draws them
     there. */
  let figures: Figure[] = $derived([
    { n: String(programs.length), label: "programs registered" },
    { n: String(commands), label: "commands declared" },
  ]);
</script>

<div class="dash">
  <header class="top">
    <h1 class="t-sec">rig</h1>
    <p class="state">
      <span class="dot" class:bad={!health.connected}></span>
      {#if health.connected}
        answering on <code>{health.socket}</code>
      {:else}
        not answering. {health.detail || "The daemon is not reachable."}
      {/if}
    </p>
    {#if lastRead}
      <span class="stamp t-num">registry read at {lastRead}</span>
    {/if}
  </header>

  <!-- ── the census ─────────────────────────────────────────────────────
       Four read figures at the top of the page, large, in the numeral face.
       design/visual-system.html puts exactly this strip under its own hero
       and calls the pattern built; the shell had never instantiated it. -->
  {#if health.connected}
    <dl class="figures">
      {#each figures as f (f.label)}
        <!-- dt is the label and dd the value, as everywhere else in this
             file; the grid puts the value on the first row. Reversing the
             elements to get the visual order would make a screen reader
             announce a bare number with no term. -->
        <div class="figure">
          <dt class="fl">{f.label}</dt>
          <dd class="fn t-num" class:loud={f.loud}>{f.n}</dd>
        </div>
      {/each}
    </dl>
  {/if}

  <div class="grid">
    <!-- ── the estate ───────────────────────────────────────────────────── -->
    <section class="block estate">
      <h2 class="t-sec">
        The estate
        {#if health.connected}<span class="cnt t-num">{programs.length}</span
          >{/if}
      </h2>
      {#if !health.connected}
        <p class="muted">
          The registry cannot be read while rig is not answering. Nothing below
          is a stale copy: there is no copy.
        </p>
      {:else if programs.length === 0}
        <p class="muted">
          No programs are registered. rig is answering and its registry is empty
          - one appears here the moment it registers.
        </p>
      {:else}
        <ul class="progs">
          {#each programs as pr (pr.id)}
            <li>
              <button onclick={() => onselect(pr.id)}>
                <span class="glyph"
                  >{(pr.icon || pr.id.slice(0, 2)).slice(0, 2)}</span
                >
                <span class="pid">{pr.id}</span>
                {#if pr.version}<span class="pv">{pr.version}</span>{:else}<span
                  ></span>{/if}
                <span class="pd">{pr.description || ""}</span>
                <span class="pc">{pr.commands} cmd</span>
              </button>
            </li>
          {/each}
        </ul>
        <!-- ⛔ FOUR FIGURES HERE, NOT SIX, AND THE CUT IS A REDUNDANCY FIX.
             `registered` and `commands declared` now lead the page in the
             census strip above; printing them again ten centimetres lower is
             the repetition `browsable-page` calls a bug, and the instruction
             there is to delete it rather than shorten it. What is left is the
             four nobody else says. -->
        <dl class="facts">
          <div>
            <dt>declared full coverage</dt>
            <dd class="t-num">{full}</dd>
          </div>
          <div>
            <dt>serve their own pane</dt>
            <dd class="t-num">{ownPane}</dd>
          </div>
          <div>
            <dt>hosted by rig</dt>
            <dd class="t-num">{hosted}</dd>
          </div>
          <div>
            <dt>distinct services</dt>
            <dd class="t-num">{services}</dd>
          </div>
        </dl>
      {/if}

      <!-- The rail lists GUIs, not programs (requirement 16), so the count
           above is not the count of rail entries. Saying only one of the two
           numbers would make the rail look wrong. -->
      <!-- Said only when there is one to say it about. The list is empty
           since the planner's GUI became a program's own pane, and "the rail
           also carries 0 internal GUIs" is a sentence about nothing. -->
      {#if INTERNAL_GUIS.length > 0}
        <p class="also">
          The rail also carries {INTERNAL_GUIS.length} internal GUI{INTERNAL_GUIS.length ===
          1
            ? ""
            : "s"} that rig provides itself. An internal GUI is not a registered
          program and needs no registration.
        </p>
      {/if}
    </section>

    <!-- what is deployed, and it replaced a card that restated the project
         view. The rail already lands on that view directly, so this page
         linking to it was duplicating its own destination. -->
    <Deployment {deployment} />
  </div>

  <!-- ── the build ──────────────────────────────────────────────────────
       ONE ROW, NOT A PANEL. Four version stamps in a bordered box of their
       own took a third of the grid and left the page ending two thirds of
       the way down the viewport - the "grid stretched short cards" defect
       this project's readability notes list by name, in its other shape.
       They are provenance, which the library page sets as one small line at
       the foot of every card. -->
  <footer class="prov">
    <span class="plabel">This build</span>
    {#if buildRows.length === 0}
      <span class="muted">version stamps could not be read</span>
    {:else}
      <dl class="stamps">
        {#each buildRows as [k, v] (k)}
          <div>
            <dt>{k}</dt>
            <dd class="t-num">{v}</dd>
          </div>
        {/each}
      </dl>
    {/if}
  </footer>

  <!-- ⛔ SAID OUT LOUD BECAUSE THE DEFERRAL IS HIS. Requirement 6 leaves this
       page's content to be defined later; a dashboard that looked finished
       would hide that, and the next person to work on it would not know what
       was decided and what was merely available. -->
  <p class="foot">
    What belongs on this page is still being decided. Everything above is what
    the window can answer today without asking rig for anything new.
  </p>
</div>

<style>
  .dash {
    display: grid;
    gap: calc(1.4rem * var(--den));
    align-content: start;
    max-width: 1180px;
  }

  /* ── the header, and it is type rather than a card ────────────────────── */

  .top {
    display: grid;
    grid-template-columns: auto 1fr auto;
    align-items: baseline;
    gap: 0.9rem;
    padding-bottom: 0.7rem;
    border-bottom: 1px solid var(--border);
  }

  /* The face and the tracking come from .t-sec in app.css; only the step is
     local, because only this page knows which step it is. */
  h1 {
    margin: 0;
    font-size: var(--fs-3);
    line-height: 1;
  }

  .state {
    margin: 0;
    color: var(--fg-dim);
    font-size: var(--fs--1);
    display: flex;
    align-items: center;
    gap: 0.5rem;
    min-width: 0;
  }

  .state code {
    font-family: var(--mono);
    color: var(--fg);
  }

  .stamp {
    font-family: var(--mono);
    font-size: var(--fs--1);
    color: var(--fg-dim);
    white-space: nowrap;
  }

  /* ── the census strip ─────────────────────────────────────────────────

     Four figures at --fs-3 (32px at the default base) with their labels at
     --fs--1. A measured census of the shell's own CSS is why: 48 of its 66
     type declarations named --fs--1 and ONE named --fs-3, so nearly the whole
     product was set at a single size one step BELOW body and hierarchy had
     nothing to work with. Every ratio passed; nothing had a rank. */

  .figures {
    margin: 0;
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(10rem, 1fr));
    gap: 0.9rem 1.6rem;
    padding-bottom: calc(0.9rem * var(--den));
    border-bottom: 1px solid var(--border);
  }

  .figure {
    display: grid;
    /* The value first and the label under it, while the DOM keeps dt before
       dd so the pair is still announced as a term and its definition. */
    grid-template-rows: auto auto;
    gap: 0.1rem;
    min-width: 0;
  }

  .fn {
    grid-row: 1;
    margin: 0;
    font-size: var(--fs-3);
    line-height: 1;
    color: var(--fg);
  }

  /* ⛔ THE LOUD FIGURE, AND NOTHING CLAIMS IT ON THIS PAGE TODAY. Requirement
     15 makes an honest ugly reading the job and a flattering one a failure,
     so the number that carries the finding is the one that gets the hue.
     The figure that used to claim it was "with any step recorded", which
     left at plan/50 move 7 with the rest of the planner's views; the rule
     and the token stay because the next figure that carries a finding takes
     them, and because the planner's own pane draws the same reading with the
     same token, so the two surfaces cannot disagree about which number
     matters. Amber is the warn member: section 11's rule is that a hue
     appears only when something wants you. */
  .fn.loud {
    color: var(--sem-warn);
  }

  .fl {
    grid-row: 2;
    font-size: var(--fs--1);
    color: var(--fg-dim);
    line-height: 1.3;
  }

  /* ── two blocks, and they are deliberately not two identical cards ────── */

  .grid {
    display: grid;
    grid-template-columns: minmax(0, 1.35fr) minmax(0, 1fr);
    gap: calc(1.2rem * var(--den));
    /* Independent blocks, so a short one is short. A stretched card with 100px
       of content in a 700px box is a defect this project's own readability
       notes list by name. */
    align-items: start;
  }

  @media (max-width: 900px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }

  /* A section title is the display face, one step above body, with its count
     beside it - the library page's device, where every rail entry, filter and
     section header carries the number of things behind it. */
  h2 {
    margin: 0 0 0.75rem;
    font-size: var(--fs-1);
    display: flex;
    align-items: baseline;
    gap: 0.55rem;
  }

  .cnt {
    font-size: var(--fs--1);
    font-weight: 400;
    letter-spacing: 0;
    color: var(--fg-dim);
  }

  .block {
    min-width: 0;
  }

  /* The estate is a list, the record is a figure, the build stamps are a
     footer rule. Three structures because they hold three kinds of thing;
     one rounded box repeated three times would say they were the same. */
  .estate {
    padding: calc(1rem * var(--den)) 1.1rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-2);
  }

  /* Prose is body size and metadata is a step below it - the library page's
     split, and the reason its cards read at a glance while carrying four
     tiers of fact. The shell had one size for both. */
  .muted {
    margin: 0;
    color: var(--fg-dim);
    font-size: var(--fs-0);
    line-height: 1.5;
    max-width: 58ch;
  }

  /* ── the program list ─────────────────────────────────────────────────── */

  .progs {
    list-style: none;
    margin: 0 0 1rem;
    padding: 0;
    display: grid;
    gap: 1px;
  }

  .progs button {
    font: inherit;
    font-size: var(--fs--1);
    width: 100%;
    display: grid;
    grid-template-columns: 2.2rem minmax(7ch, auto) auto minmax(0, 1fr) auto;
    align-items: baseline;
    gap: 0.7rem;
    text-align: start;
    background: none;
    border: 1px solid transparent;
    border-radius: 8px;
    padding: 0.4rem 0.5rem;
    color: var(--fg-dim);
    cursor: pointer;
  }

  .progs button:hover {
    border-color: var(--border-2);
    color: var(--fg);
  }

  .progs button:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }

  .glyph {
    font-family: var(--mono);
    font-size: var(--fs--1);
    color: var(--fg);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 0.1rem 0;
    text-align: center;
    background: var(--panel);
  }

  /* ── three tiers of chrome, and the library page is where they come from.
     Its cards give the title a face of its own, the format and duration an
     OUTLINED chip, and the tags no chrome at all - bare words in a dimmer
     ink. Three weights let one row carry four facts without any of them
     shouting. The row used to give all four the same size and colour. */

  .pid {
    font-family: var(--mono);
    font-size: var(--fs-0);
    color: var(--fg);
  }

  .pv {
    font-family: var(--mono);
    color: var(--fg-dim);
    border: 1px solid var(--border);
    border-radius: 999px;
    padding: 0 0.45rem;
    justify-self: start;
  }

  .pc {
    font-family: var(--mono);
    font-variant-numeric: tabular-nums;
    color: var(--fg-dim);
  }

  .pd {
    color: var(--fg-dim);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* ── the declared facts ───────────────────────────────────────────────── */

  /* Four across, declared rather than auto-fit: auto-fit wrapped the fourth
     onto a row of its own at this width, and one orphan under three is the
     ragged shape that makes a block look unfinished. */
  .facts {
    margin: 0;
    padding-top: 0.8rem;
    border-top: 1px solid var(--border);
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 0.7rem 1rem;
  }

  @media (max-width: 1100px) {
    .facts {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  .facts div {
    display: grid;
    gap: 0.1rem;
  }

  .facts dt {
    font-size: var(--fs--1);
    color: var(--fg-dim);
  }

  .facts dd {
    margin: 0;
    font-family: var(--mono);
    font-size: var(--fs-1);
    font-variant-numeric: tabular-nums;
    color: var(--fg);
    line-height: 1.1;
  }

  /* ── rig's own record ─────────────────────────────────────────────────── */

  .also {
    margin: 0.9rem 0 0;
    padding-top: 0.8rem;
    border-top: 1px solid var(--border);
    color: var(--fg-dim);
    font-size: var(--fs--1);
    line-height: 1.5;
    max-width: 62ch;
  }

  /* ── the build stamps, as provenance ──────────────────────────────────── */

  .prov {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.4rem 1.4rem;
    padding-top: calc(0.8rem * var(--den));
    border-top: 1px solid var(--border);
    font-size: var(--fs--1);
  }

  .plabel {
    color: var(--fg-dim);
  }

  .stamps {
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem 1.4rem;
  }

  .stamps div {
    display: flex;
    align-items: baseline;
    gap: 0.45rem;
  }

  .stamps dt {
    color: var(--fg-dim);
  }

  .stamps dd {
    margin: 0;
    color: var(--fg);
  }

  .foot {
    margin: 0;
    color: var(--fg-dim);
    font-size: var(--fs--1);
    max-width: 72ch;
  }
</style>
