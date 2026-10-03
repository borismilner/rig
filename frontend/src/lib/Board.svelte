<!-- Main's Board tab: the cards programs and agents put on rig's board
     (plan/55 requirements 2, 3, 27), read with rig.panel.list and read
     again on every panel.changed.

     Every source and then a tab per source (27); search and the severity
     chips under them, and both rows stay above the cards while they scroll
     (37). A section per source, its stripe the worst open card's colour. A
     button goes to the card's owner as rig.panel.act; what the owner does
     with it shows as its next version. -->
<script lang="ts">
  import type {
    Card,
    CardList,
  } from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/models.js";
  import { SEVERITIES, hhmm, sections, shown, sourceTabs } from "./board";
  import { ago, sevHue } from "./notes";

  interface Props {
    list: CardList | null;
    loadError: string;
    src: string;
    q: string;
    /** Presses a card's button; rejects with rig's refusal. */
    onpress: (card: string, action: string) => Promise<void>;
    /* The measurement fixture's fixed clock. */
    now?: number;
  }
  let {
    list,
    loadError,
    src = $bindable(),
    q = $bindable(),
    onpress,
    now,
  }: Props = $props();

  let sev = $state(new Set<string>());
  let cards = $derived(list?.cards ?? []);
  let tabs = $derived(sourceTabs(cards));
  let closedN = $derived(
    cards.filter((c) => c.closed && (!src || c.from === src)).length,
  );
  let sects = $derived(sections(shown(cards, src, q, sev)));
  let clock = $derived(now ?? Date.now());

  // Closed cards stay folded until asked for, by section.
  let unfolded: Record<string, boolean> = $state({});
  let refused: Record<string, string> = $state({});
  let pressing: string | null = $state(null);

  function chip(s: string) {
    const next = new Set(sev);
    if (next.has(s)) next.delete(s);
    else next.add(s);
    sev = next;
  }

  async function press(c: Card, action: string) {
    pressing = c.id;
    refused[c.id] = "";
    try {
      await onpress(c.id, action);
    } catch (e) {
      refused[c.id] = String(e);
    } finally {
      pressing = null;
    }
  }

  function ontabkey(e: KeyboardEvent) {
    const d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
    if (!d) return;
    e.preventDefault();
    const i = tabs.findIndex(([s]) => s === src);
    src = tabs[(i + d + tabs.length) % tabs.length][0];
    queueMicrotask(() =>
      document
        .querySelector<HTMLElement>('.srctabs [aria-selected="true"]')
        ?.focus(),
    );
  }
</script>

<section class="board" aria-label="Board">
  <div class="btop">
    <div class="row1">
      <div class="srctabs" role="tablist" aria-label="Board sources">
        {#each tabs as [s, n] (s)}
          <button
            role="tab"
            aria-selected={src === s}
            tabindex={src === s ? 0 : -1}
            onclick={() => (src = s)}
            onkeydown={ontabkey}
            >{s || "Every source"}<span class="cnt">{n}</span></button
          >
        {/each}
        {#if src && !tabs.some(([s]) => s === src)}
          <button role="tab" aria-selected="true" tabindex="0"
            >{src}<span class="cnt">0</span></button
          >
        {/if}
      </div>
      {#if closedN}<span class="dim nowrap">{closedN} closed</span>{/if}
    </div>
    <div class="row2">
      <input
        type="search"
        placeholder="Search the board"
        aria-label="Search the board"
        bind:value={q}
      />
      <div class="chips" role="group" aria-label="Severity">
        {#each SEVERITIES as s (s)}
          <button
            class="chip"
            aria-pressed={sev.has(s)}
            style="--chipc: {sevHue(s)}"
            onclick={() => chip(s)}><i></i>{s}</button
          >
        {/each}
      </div>
    </div>
  </div>

  {#if loadError}
    <p class="empty">Rig did not answer the board: {loadError}</p>
  {:else if !list}
    <p class="empty">Reading the board.</p>
  {:else if list.missing}
    <p class="empty">
      This rig has no board yet: it was built before rig.panel.put. Deploy a
      newer rig and the board appears here.
    </p>
  {:else if sects.length === 0}
    <p class="empty">
      {cards.length
        ? "Nothing matches. Clear a filter to see more."
        : "No cards yet. Programs and agents put them here with rig panel put, and each one appears the moment it is put."}
    </p>
  {:else}
    {#each sects as s (s.from)}
      <section
        class="sect"
        aria-label={s.from}
        style="--sev: {s.worst ? sevHue(s.worst) : 'var(--border)'}"
      >
        <header>
          <span class="stripe"></span>
          <span class="name">{s.from}</span>
          <span class="dim">{s.open.length} open</span>
        </header>
        <div class="cards">
          {#each [...s.open, ...(unfolded[s.from] ? s.closed : [])] as c (c.id)}
            <article
              class="card"
              class:closed={c.closed}
              style="--sev: {sevHue(c.severity)}"
              aria-label="{c.title}{c.status ? ', ' + c.status : ''}"
            >
              <div class="top">
                <b class="t">{c.title}</b>
                {#if c.status}<span class="status">{c.status}</span>{/if}
              </div>
              <div class="sevw">
                {c.severity || "info"}{#if c.project}<span class="dim"
                    >{" · "}{c.project}</span
                  >{/if}
              </div>
              {#if c.body}<p class="body">{c.body}</p>{/if}
              {#if c.hasProgress || c.busy}
                <div class="pct">
                  <div
                    class="meter"
                    class:indet={!c.hasProgress}
                    role="progressbar"
                    aria-label={c.title}
                    aria-valuemin={0}
                    aria-valuemax={100}
                    aria-valuenow={c.hasProgress
                      ? Math.round(c.progress * 100)
                      : undefined}
                  >
                    <i
                      style={c.hasProgress
                        ? `width: ${(c.progress * 100).toFixed(0)}%`
                        : ""}
                    ></i>
                  </div>
                  {#if c.hasProgress}<b>{Math.round(c.progress * 100)}%</b>{/if}
                </div>
              {/if}
              {#if c.facts.length}
                <dl class="facts">
                  {#each c.facts as f, i (i)}<dt>{f.label}</dt>
                    <dd>{f.value}</dd>{/each}
                </dl>
              {/if}
              {#if c.actions.length && !c.closed}
                <div class="acts">
                  {#each c.actions as a, i (a)}
                    <button
                      class="act"
                      class:primary={i === 0}
                      disabled={pressing === c.id}
                      onclick={() => press(c, a)}>{a}</button
                    >
                  {/each}
                </div>
              {/if}
              {#if refused[c.id]}<p class="refused" role="alert">
                  {refused[c.id]}
                </p>{/if}
              <div class="foot">
                <span>created {hhmm(c.created)}</span>
                <span
                  >{c.closed
                    ? `closed ${hhmm(c.updated)}`
                    : c.version > 1
                      ? `${c.version - 1} change${c.version === 2 ? "" : "s"}`
                      : "new"}</span
                >
                <span class="sp"></span>
                <time datetime={c.updated}>{ago(c.updated, clock)}</time>
              </div>
            </article>
          {/each}
        </div>
        {#if s.closed.length}
          <button
            class="more"
            onclick={() => (unfolded[s.from] = !unfolded[s.from])}
            >{unfolded[s.from]
              ? `Hide ${s.closed.length} closed`
              : `Show ${s.closed.length} closed`}</button
          >
        {/if}
      </section>
    {/each}
    {#if list.omitted}
      <p class="dim more-n">
        And {list.omitted} older cards rig did not send; a search or a source tab
        narrows the board.
      </p>
    {/if}
  {/if}
</section>

<style>
  /* The tabs and filters stay above the cards as they scroll (37): sticky
     inside Main's own scroll, on the page's ground so cards pass under. */
  .btop {
    position: sticky;
    top: calc(-0.9rem);
    z-index: 1;
    background: var(--panel);
    padding-top: 0.2rem;
    margin-top: -0.2rem;
    padding-bottom: 0.7rem;
    border-bottom: 1px solid var(--border);
    margin-bottom: 0.9rem;
    display: grid;
    gap: 0.6rem;
  }
  .row1,
  .row2 {
    display: flex;
    align-items: center;
    gap: 0.8rem;
    min-width: 0;
  }
  .srctabs {
    display: flex;
    gap: 4px;
    overflow-x: auto;
    min-width: 0;
    flex: 1;
  }
  .srctabs button {
    font: inherit;
    font-size: var(--fs--1);
    font-weight: 600;
    display: flex;
    align-items: center;
    gap: 0.45rem;
    white-space: nowrap;
    padding: 0.3rem 0.7rem;
    color: var(--fg-dim);
    background: none;
    border: 1px solid transparent;
    border-radius: 999px;
    cursor: pointer;
  }
  .srctabs button:hover {
    color: var(--fg);
    background: var(--tint);
  }
  .srctabs button[aria-selected="true"] {
    color: var(--fg);
    border-color: var(--h-steel);
    background: var(--tint);
  }
  .srctabs button:focus-visible,
  .chip:focus-visible,
  .more:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }
  .nowrap {
    white-space: nowrap;
  }
  .dim {
    color: var(--fg-dim);
    font-size: var(--fs--1);
  }
  .row2 input {
    font: inherit;
    font-size: var(--fs--1);
    color: var(--fg);
    background: var(--bg);
    border: 1px solid var(--border-2);
    border-radius: 7px;
    padding: 0.3rem 0.55rem;
    width: min(320px, 40%);
    min-width: 0;
  }
  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }
  .chip {
    font: inherit;
    font-size: var(--fs--1);
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.2rem 0.6rem;
    color: var(--fg-dim);
    background: none;
    border: 1px solid var(--border-2);
    border-radius: 999px;
    cursor: pointer;
  }
  .chip i {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--chipc);
  }
  .chip[aria-pressed="true"] {
    color: var(--fg);
    border-color: var(--chipc);
    background: color-mix(in srgb, var(--chipc) 14%, transparent);
  }

  .empty {
    color: var(--fg-dim);
    max-width: 62ch;
    margin: 2.5rem auto;
    text-align: center;
  }

  .sect {
    margin-bottom: 1.3rem;
  }
  .sect header {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-bottom: 0.55rem;
  }
  .stripe {
    width: 4px;
    height: 1.1em;
    border-radius: 2px;
    background: var(--sev);
  }
  .name {
    font-weight: 600;
  }
  .cards {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
    gap: 10px;
  }
  .card {
    display: grid;
    align-content: start;
    gap: 0.4rem;
    padding: 0.65rem 0.75rem;
    background: var(--bg-2);
    border: 1px solid var(--border);
    border-left: 3px solid var(--sev);
    border-radius: var(--radius);
    font-size: var(--fs--1);
    min-width: 0;
  }
  .card.closed {
    background: var(--bg);
  }
  .card.closed .t {
    color: var(--fg-dim);
  }
  .top {
    display: flex;
    gap: 0.6rem;
    justify-content: space-between;
    align-items: baseline;
  }
  .t {
    font-size: var(--fs-0);
    overflow-wrap: anywhere;
  }
  .status {
    flex: none;
    max-width: 50%;
    color: var(--fg-dim);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .sevw {
    color: var(--fg-dim);
    text-transform: capitalize;
  }
  .sevw .dim {
    text-transform: none;
  }
  .body {
    margin: 0;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
  }
  .pct {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }
  .meter {
    flex: 1;
    height: 6px;
    border-radius: 3px;
    background: var(--tint);
    overflow: hidden;
  }
  .meter i {
    display: block;
    height: 100%;
    background: var(--h-steel);
  }
  .meter.indet i {
    width: 35%;
    animation: slide 1.4s ease-in-out infinite;
  }
  @keyframes slide {
    from {
      transform: translateX(-100%);
    }
    to {
      transform: translateX(290%);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .meter.indet i {
      animation: none;
      width: 100%;
      opacity: 0.5;
    }
  }
  .facts {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: 0.15rem 0.7rem;
    margin: 0;
  }
  .facts dt {
    color: var(--fg-dim);
  }
  .facts dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .acts {
    display: flex;
    flex-wrap: wrap;
    gap: 0.4rem;
  }
  .refused {
    margin: 0;
    color: var(--sem-bad);
  }
  .foot {
    display: flex;
    gap: 0.7rem;
    color: var(--fg-dim);
    border-top: 1px solid var(--border);
    padding-top: 0.35rem;
  }
  .sp {
    flex: 1;
  }
  .more {
    font: inherit;
    font-size: var(--fs--1);
    color: var(--fg-dim);
    background: none;
    border: 0;
    padding: 0.3rem 0;
    margin-top: 0.3rem;
    cursor: pointer;
    text-decoration: underline;
  }
  .more:hover {
    color: var(--fg);
  }
  .more-n {
    text-align: center;
  }
</style>
