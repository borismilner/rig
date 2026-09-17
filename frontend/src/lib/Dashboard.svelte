<!-- The main GUI: a general-purpose dashboard, and a destination in its own
     right rather than the empty state before a program is picked.

     ⛔ BORIS, 2026-09-17 (section 11, requirement 6 as replaced): "the main GUI
     show show a general-purpose dashboard with all the most important
     information we'll define in the future." ITS CONTENT IS DELIBERATELY
     DEFERRED and filling it is explicitly not a seat's call, so what is here
     is the honest first payload: everything the window can already answer
     without a new wire verb, and nothing else.

     ⛔ NOTHING ON THIS PAGE IS INVENTED. Every number is read from the three
     calls the window already makes - Health, Programs, Build - plus rig's own
     brief. There is no uptime, no error rate, no activity feed and no
     sparkline, because nothing on this wire carries any of them, and a metric
     with no data behind it is worse on a dashboard than a gap. -->
<script lang="ts">
  import type {
    Health,
    Program,
  } from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";
  import type { RigStore } from "./rigstore.svelte";
  import { planVsExec, verdict } from "./brief";
  import Waffle from "./Waffle.svelte";
  import { INTERNAL_GUIS, PROJECT_CASE_GUI } from "./guis";

  interface Props {
    health: Health;
    programs: Program[];
    build: Record<string, string> | null;
    store: RigStore;
    lastRead: string;
    /** Opens an internal GUI, the same door the rail opens. */
    onopengui: (id: string) => void;
    /** Selects a registered program, also the rail's door. */
    onselect: (id: string) => void;
  }

  let { health, programs, build, store, lastRead, onopengui, onselect }: Props =
    $props();

  // Read once when the dashboard is first shown. The store keeps what it
  // read, so arriving here after visiting the project GUI costs no dial.
  $effect(() => {
    void store.open();
  });

  let held = $derived(store.held(store.current));
  let p = $derived(planVsExec(held.brief));

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
</script>

<div class="dash">
  <header class="top">
    <h1>rig</h1>
    <p class="state">
      <span class="dot" class:bad={!health.connected}></span>
      {#if health.connected}
        answering on <code>{health.socket}</code>
      {:else}
        not answering. {health.detail || "The daemon is not reachable."}
      {/if}
    </p>
    {#if lastRead}
      <span class="stamp">registry read at {lastRead}</span>
    {/if}
  </header>

  <div class="grid">
    <!-- ── the estate ───────────────────────────────────────────────────── -->
    <section class="block estate">
      <h2>The estate</h2>
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
                <span class="pv">{pr.version}</span>
                <span class="pd">{pr.description || ""}</span>
                <span class="pc">{pr.commands} cmd</span>
              </button>
            </li>
          {/each}
        </ul>
        <dl class="facts">
          <div>
            <dt>registered</dt>
            <dd>{programs.length}</dd>
          </div>
          <div>
            <dt>declared full coverage</dt>
            <dd>{full}</dd>
          </div>
          <div>
            <dt>commands declared</dt>
            <dd>{commands}</dd>
          </div>
          <div>
            <dt>serve their own pane</dt>
            <dd>{ownPane}</dd>
          </div>
          <div>
            <dt>hosted by rig</dt>
            <dd>{hosted}</dd>
          </div>
          <div>
            <dt>distinct services</dt>
            <dd>{services}</dd>
          </div>
        </dl>
      {/if}

      <!-- The rail lists GUIs, not programs (requirement 16), so the count
           above is not the count of rail entries. Saying only one of the two
           numbers would make the rail look wrong. -->
      <p class="also">
        The rail also carries {INTERNAL_GUIS.length} internal GUI{INTERNAL_GUIS.length ===
        1
          ? ""
          : "s"} that rig provides itself. An internal GUI is not a registered program
        and needs no registration.
      </p>
    </section>

    <!-- ── the project and case record ──────────────────────────────────── -->
    <section class="block record">
      <h2>{store.current}: plan against execution</h2>
      {#if held.error}
        <p class="muted">{held.error}</p>
      {:else if !held.brief}
        <p class="muted">
          {store.loading ? "Reading the project record." : "Not read yet."}
        </p>
      {:else}
        <div class="rfig">
          <Waffle
            items={p.items}
            dense
            summary={`${p.planned} open work items, ${p.recorded} with a step recorded`}
          />
        </div>
        <p class="rnum">
          <strong>{p.recorded}</strong> of <strong>{p.planned}</strong> open items
          have any step recorded.
        </p>
        <p class="rverdict">{verdict(p)}</p>
        <button class="link" onclick={() => onopengui(PROJECT_CASE_GUI.id)}>
          Open {PROJECT_CASE_GUI.title}
        </button>
      {/if}
    </section>

    <!-- ── the build ────────────────────────────────────────────────────── -->
    <section class="block build">
      <h2>This build</h2>
      {#if buildRows.length === 0}
        <p class="muted">The window could not read its own version stamps.</p>
      {:else}
        <table>
          <tbody>
            {#each buildRows as [k, v] (k)}
              <tr><th scope="row">{k}</th><td>{v}</td></tr>
            {/each}
          </tbody>
        </table>
      {/if}
    </section>
  </div>

  <!-- ⛔ SAID OUT LOUD BECAUSE THE DEFERRAL IS HIS. Requirement 6 leaves this
       page's content to be defined later; a dashboard that looked finished
       would hide that, and the next person to work on it would not know what
       was decided and what was merely available. -->
  <p class="foot">
    What belongs on this page is still being decided. These three are what the
    window can answer today without asking rig for anything new.
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

  h1 {
    margin: 0;
    font-family: var(--disp);
    font-size: var(--fs-3);
    font-weight: 600;
    letter-spacing: var(--tight-disp);
    color: var(--fg);
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

  /* ── three blocks, and they are deliberately not three identical cards ── */

  .grid {
    display: grid;
    grid-template-columns: minmax(0, 1.5fr) minmax(0, 1fr);
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

  .estate {
    grid-row: span 2;
  }

  h2 {
    margin: 0 0 0.7rem;
    font-size: var(--fs-0);
    font-weight: 650;
    color: var(--fg);
  }

  .block {
    min-width: 0;
  }

  /* The estate is a list with a rule, the record is a figure, the build is a
     table. Three structures because they hold three kinds of thing; one
     rounded box repeated three times would say they were the same kind. */
  .estate,
  .record {
    padding: calc(1rem * var(--den)) 1.1rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-2);
  }

  .muted {
    margin: 0;
    color: var(--fg-dim);
    font-size: var(--fs--1);
    max-width: 62ch;
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
    grid-template-columns: 2.2rem 7ch auto minmax(0, 1fr) auto;
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

  .pid {
    font-family: var(--mono);
    color: var(--fg);
  }

  .pv,
  .pc {
    font-family: var(--mono);
    color: var(--fg-dim);
  }

  .pd {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* ── the declared facts ───────────────────────────────────────────────── */

  .facts {
    margin: 0;
    padding-top: 0.8rem;
    border-top: 1px solid var(--border);
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(9.5rem, 1fr));
    gap: 0.7rem 1.1rem;
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
    max-width: 62ch;
  }

  .rfig {
    margin-bottom: 0.8rem;
  }

  .rnum {
    margin: 0 0 0.5rem;
    font-size: var(--fs--1);
    color: var(--fg-dim);
  }

  .rnum strong {
    font-family: var(--mono);
    font-size: var(--fs-1);
    color: var(--fg);
  }

  .rverdict {
    margin: 0 0 0.8rem;
    font-family: var(--disp);
    font-size: var(--fs-0);
    line-height: 1.45;
    color: var(--fg);
    max-width: 44ch;
  }

  .link {
    font: inherit;
    font-size: var(--fs--1);
    background: none;
    border: 0;
    border-bottom: 1px solid var(--border-2);
    padding: 0.1rem 0;
    color: var(--fg-dim);
    cursor: pointer;
  }

  .link:hover {
    color: var(--fg);
    border-color: var(--hue);
  }

  .link:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
    border-radius: 4px;
  }

  /* ── the build table ──────────────────────────────────────────────────── */

  .build {
    padding: 0 0 0 1.1rem;
    border-inline-start: 2px solid var(--border);
  }

  table {
    border-collapse: collapse;
    font-size: var(--fs--1);
  }

  th {
    text-align: start;
    font-weight: 400;
    color: var(--fg-dim);
    padding: 0.15rem 1.2rem 0.15rem 0;
    white-space: nowrap;
  }

  td {
    font-family: var(--mono);
    color: var(--fg);
    padding: 0.15rem 0;
  }

  .foot {
    margin: 0;
    color: var(--fg-dim);
    font-size: var(--fs--1);
    max-width: 72ch;
  }
</style>
