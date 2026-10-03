<!-- The notifications panel on Main's right (plan/55 requirements 7 to 9,
     23, 31; design/dashboard/ is the reference).

     ⛔ BORIS, 2026-10-03: "a dedicated vertical panel on the right with the
     latest notifications sorted with the latest ones on top; in case the
     user missed them. Notifications that are older than a configurable
     amount of time [...] should be evicted." And: "Clicking notification
     should work on any place of the notification, not just the title [...]
     when hovering over a notification it should be highlighted."

     Every row is rig's record (RigService.Notifications); evicted ones stay
     in the record, only this panel lets them go. -->
<script lang="ts">
  import type {
    Note,
    NoteList,
  } from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/models.js";
  import { ago, answered, arrange, sevHue, sources, waiting } from "./notes";
  import { stamp } from "./estate";

  interface Props {
    list: NoteList | null;
    loadError: string;
    connected: boolean;
    /* Search and source live in the shell, so the card's "Only X's
       notifications" can set the source. */
    q: string;
    src: string;
    onopen: (n: Note) => void;
    /* Fixed for the measurement fixture, so its "ago" does not drift. */
    now?: number;
  }
  let {
    list,
    loadError,
    connected,
    q = $bindable(),
    src = $bindable(),
    onopen,
    now,
  }: Props = $props();

  let notes = $derived(list?.notes ?? []);
  let shown = $derived(arrange(notes, q, src));
  let srcs = $derived(sources(notes));
  let asking = $derived(notes.filter(waiting).length);
  let clock = $derived(now ?? Date.now());
</script>

<aside class="notepanel" aria-label="Notifications">
  <header>
    <h2>Notifications</h2>
    {#if asking}<span class="badge">{asking} waiting</span>{/if}
  </header>

  <div class="nfilter">
    <input
      type="search"
      placeholder="Search notifications"
      aria-label="Search notifications"
      bind:value={q}
    />
    <label class="from"
      ><span class="dim">From</span>
      <select bind:value={src} aria-label="From">
        <option value="">every source ({notes.length})</option>
        {#each srcs as [s, n] (s)}<option value={s}>{s} ({n})</option>{/each}
        {#if src && !srcs.some(([s]) => s === src)}<option value={src}
            >{src} (0)</option
          >{/if}
      </select></label
    >
  </div>

  {#if !connected}
    <p class="none">Rig is not answering, so there are none to show.</p>
  {:else if loadError}
    <p class="none">Rig did not answer the notifications: {loadError}</p>
  {:else if !list}
    <p class="none">Reading the notifications.</p>
  {:else}
    <ol class="notes">
      {#each shown as n (n.id)}
        <!-- The row is the click target (31); the button inside is the
             keyboard's way in, so a click on it must not open twice. -->
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
        <li
          class:asks={waiting(n)}
          style="--sev: {sevHue(n.severity)}"
          onclick={(e) => {
            if (!(e.target as HTMLElement).closest("button")) onopen(n);
          }}
        >
          <span class="bullet" class:fill={waiting(n)}></span>
          <div class="txt">
            <button
              class="nopen"
              aria-label="{n.title}: open its details"
              onclick={() => onopen(n)}
            >
              <span class="nt"
                ><b>{n.title}</b><time
                  class="dim"
                  datetime={n.at}
                  title={stamp(Date.parse(n.at))}>{ago(n.at, clock)}</time
                ></span
              >
              {#if n.body}<span class="dim body">{n.body}</span>{/if}
            </button>
            <div class="nmeta">
              <button
                class="src"
                title="Only {n.sender}'s notifications"
                onclick={() => (src = n.sender)}>{n.sender}</button
              >
              {#if waiting(n)}<span class="decide">needs your answer</span
                >{:else if n.answer}<span class="dim"
                  >answered {answered(n)}</span
                >{:else if n.forgotten}<span class="dim"
                  >asked; nobody waits now</span
                >{/if}
              {#if n.suppressed}<span class="dim">held by Do Not Disturb</span
                >{/if}
            </div>
          </div>
        </li>
      {:else}
        <li class="none">
          {notes.length
            ? "Nothing matches."
            : `No notifications in the last ${list.keepDays} day${list.keepDays === 1 ? "" : "s"}.`}
        </li>
      {/each}
    </ol>
  {/if}

  <footer>
    {#if list}
      <span class="dim"
        >Older than {list.keepDays} day{list.keepDays === 1 ? "" : "s"} leave
        this panel, never the record.</span
      >
      <span class="faint mono"
        >dashboard.notifications.keep.days, {list.keepFrom}{list.evicted
          ? ` · ${list.evicted} older`
          : ""}</span
      >
    {/if}
  </footer>
</aside>

<style>
  .notepanel {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
    background: var(--panel);
    border-left: 1px solid var(--border);
    box-sizing: border-box;
  }
  header {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.7rem 0.9rem;
    border-bottom: 1px solid var(--border);
  }
  h2 {
    margin: 0;
    font-size: var(--fs-1);
  }
  .badge {
    padding: 0 0.5rem;
    border-radius: 999px;
    font-weight: 600;
    font-size: var(--fs--1);
    color: var(--on-hue);
    background: var(--sem-warn);
  }
  .nfilter {
    display: grid;
    gap: 0.5rem;
    padding: 0.6rem 0.9rem;
    border-bottom: 1px solid var(--border);
  }
  .nfilter input,
  .nfilter select {
    font: inherit;
    font-size: var(--fs--1);
    color: var(--fg);
    background: var(--bg);
    border: 1px solid var(--border-2);
    border-radius: 7px;
    padding: 0.3rem 0.5rem;
    min-width: 0;
  }
  .from {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    align-items: center;
    gap: 0.5rem;
  }
  .notes {
    list-style: none;
    margin: 0;
    padding: 0;
    overflow: auto;
    flex: 1;
    min-height: 0;
  }
  .notes li {
    display: grid;
    grid-template-columns: 10px minmax(0, 1fr);
    gap: 0.6rem;
    padding: 0.55rem 0.9rem;
    border-bottom: 1px solid var(--border);
    font-size: var(--fs--1);
    cursor: pointer;
    transition: background 0.12s;
  }
  .notes li:hover,
  .notes li:focus-within {
    background: var(--tint);
  }
  .notes li.asks {
    background: color-mix(in srgb, var(--sem-warn) 9%, transparent);
  }
  .notes li.asks:hover,
  .notes li.asks:focus-within {
    background: color-mix(in srgb, var(--sem-warn) 17%, transparent);
  }
  .none {
    display: block !important;
    color: var(--fg-dim);
    padding: 1rem 0.9rem;
    margin: 0;
    font-size: var(--fs--1);
    cursor: default !important;
  }
  /* A ring, filled while it waits. Full colour either way: a faded mark
     fails 3:1. */
  .bullet {
    width: 10px;
    height: 10px;
    margin-top: 0.3rem;
    border-radius: 50%;
    border: 2px solid var(--sev);
    box-sizing: border-box;
  }
  .bullet.fill {
    background: var(--sev);
  }
  .txt {
    min-width: 0;
  }
  .nopen {
    all: unset;
    display: grid;
    gap: 0.1rem;
    width: 100%;
    cursor: pointer;
  }
  .nopen:focus-visible {
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
    border-radius: 4px;
  }
  .nt {
    display: flex;
    gap: 0.5rem;
    justify-content: space-between;
    align-items: baseline;
  }
  .nt b {
    overflow-wrap: anywhere;
  }
  .nt time {
    flex: none;
  }
  .body {
    overflow: hidden;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .nmeta {
    display: flex;
    flex-wrap: wrap;
    gap: 0.2rem 0.6rem;
    margin-top: 0.2rem;
  }
  .src {
    all: unset;
    cursor: pointer;
    color: var(--fg-dim);
    text-decoration: underline;
  }
  .src:focus-visible {
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }
  .decide {
    font-weight: 600;
    color: var(--fg);
  }
  .dim {
    color: var(--fg-dim);
  }
  .faint {
    color: var(--fg-dim);
    font-size: var(--fs--2);
  }
  .mono {
    font-family: var(--mono);
  }
  footer {
    display: grid;
    gap: 0.2rem;
    padding: 0.6rem 0.9rem;
    border-top: 1px solid var(--border);
    font-size: var(--fs--1);
  }
</style>
