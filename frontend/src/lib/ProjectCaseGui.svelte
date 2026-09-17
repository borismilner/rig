<!-- The project and case GUI: an internal GUI in the rail, with its own
     internal layout.

     ⛔ BORIS, 2026-09-17 (section 11 requirement 18): "The GUI for project/case
     management internally can have its own layout, for example a tab for each
     project/case and maybe even toggling between cases and projects and inside
     cases maybe even sub-division by are of interest, for example health,
     finances, and so on."

     ⛔ THE TABS HAVE MOVED TWICE TODAY, SO READ THE CURRENT POSITION RATHER
     THAN A REMEMBERED ONE. They began as the shell's top level, moved into
     rig's own program GUI, and now live HERE, one per project or case. A tab
     bar across the shell and a rig-as-a-program entry are both superseded.

     ⛔ THE THIRD LEVEL IS DELIBERATELY NOT BUILT. "Inside cases maybe even
     sub-division by area of interest" is a maybe he said twice, and the plan
     is explicit that nothing on the wire holds it: `kind` is a real field
     (`ProjectBriefResponse.kind`, and a case has no semver and a different
     status vocabulary), and area of interest is not. So the projects/cases
     toggle is rendered from data and the sub-division is stated as absent
     rather than mocked with an invented axis. The layout leaves room for it:
     the panel below the tab strip is one component, and an area strip would
     sit between them without moving anything else. -->
<script lang="ts">
  import { untrack } from "svelte";
  import PlanVsExec from "./PlanVsExec.svelte";
  import type { RigStore } from "./rigstore.svelte";
  import { SELF } from "./rigstore.svelte";
  import { splitByKind } from "./brief";

  interface Props {
    store: RigStore;
    /** Which side of the toggle to open on, so the contrast gate can reach
        the cases side, which has no data to click its way into. */
    initialSide?: "projects" | "cases";
    /** Same fixture idea one level down: open the first work-item row so the
        gate can read the opened panel's colours. */
    openRows?: boolean;
  }

  let { store, initialSide = "projects", openRows = false }: Props = $props();

  type Entry = {
    id: string;
    title: string;
    kind: string;
    /** False when this window asked for the name rather than being told it.
        Today that is every entry - see the note on Projects() in
        cmd/rigwindow/brief.go. */
    discovered: boolean;
  };

  // Read once and deliberately not tracked: this is a starting position, not
  // a binding. Svelte warns about reading a prop non-reactively here and it is
  // right to - the same shape caught a real bug in Pane.svelte - so the intent
  // is stated rather than the warning silenced.
  let side = $state(untrack(() => initialSide));
  let tabEls: HTMLButtonElement[] = $state([]);

  $effect(() => {
    void store.open();
  });

  // The roster, honestly. An empty answer from Projects() is not padded with
  // a discovered-looking entry: the one name this window can ask about is
  // marked as asked-for, and the view says so above the tabs.
  let entries: Entry[] = $derived(
    store.projects.length > 0
      ? store.projects.map((p) => ({
          id: p.id,
          title: p.title || p.id,
          kind: p.kind || "project",
          discovered: true,
        }))
      : [
          {
            id: SELF,
            title: SELF,
            // Unknown until the brief answers, and `project` is the safer
            // default only because it is what the toggle opens on. Once read,
            // the brief's own kind wins - so a case asked for by name lands
            // on the right side of the toggle by itself.
            kind: store.held(SELF).brief?.kind || "project",
            discovered: false,
          },
        ],
  );

  let split = $derived(splitByKind(entries));
  let projects = $derived(split.projects);
  let cases = $derived(split.cases);
  let shown = $derived(side === "cases" ? cases : projects);

  // Keep the selection inside the visible bucket. Switching the toggle to a
  // side the current record is not on would otherwise leave a panel open with
  // no tab marked, which reads as a rendering fault.
  $effect(() => {
    if (shown.length === 0) return;
    if (!shown.some((e) => e.id === store.current))
      void store.open(shown[0].id);
  });

  function ontabkey(e: KeyboardEvent) {
    const i = shown.findIndex((t) => t.id === store.current);
    let next = -1;
    if (e.key === "ArrowRight") next = (i + 1) % shown.length;
    else if (e.key === "ArrowLeft")
      next = (i - 1 + shown.length) % shown.length;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = shown.length - 1;
    if (next < 0 || shown.length === 0) return;
    e.preventDefault();
    void store.open(shown[next].id);
    tabEls[next]?.focus();
  }

  let held = $derived(store.held(store.current));
</script>

<div class="pcg">
  <div class="head">
    <div class="toggle" role="group" aria-label="Projects or cases">
      <button
        aria-pressed={side === "projects"}
        onclick={() => (side = "projects")}
      >
        Projects <span class="n">{projects.length}</span>
      </button>
      <button aria-pressed={side === "cases"} onclick={() => (side = "cases")}>
        Cases <span class="n">{cases.length}</span>
      </button>
    </div>

    {#if store.projects.length === 0}
      <!-- ⛔ SAID, NOT HIDDEN. There is no verb on this wire that enumerates
           projects - record.query needs a project name, which is circular -
           so the roster is empty and this window asks for one name. Showing
           it as though it had been discovered would be the window lying about
           where a name came from. -->
      <p class="asked">
        rig has no verb that lists what it holds, so this window cannot
        enumerate your projects. It asks for <code>{SELF}</code> by name.
      </p>
    {/if}
  </div>

  {#if shown.length === 0}
    <div class="panel">
      <p class="none">
        {#if side === "cases"}
          Nothing here is a case. rig carries the project-or-case distinction on
          every brief, so this is a real answer and not a missing feature - none
          of the records this window can reach is a case.
        {:else}
          Nothing here is a project.
        {/if}
      </p>
    </div>
  {:else}
    <div class="tabs" role="tablist" aria-label={side}>
      {#each shown as t, i (t.id)}
        <button
          bind:this={tabEls[i]}
          role="tab"
          id={`tab-${t.id}`}
          aria-selected={store.current === t.id}
          aria-controls="pcg-panel"
          tabindex={store.current === t.id ? 0 : -1}
          onclick={() => void store.open(t.id)}
          onkeydown={ontabkey}
          title={t.discovered
            ? t.title
            : `${t.title} - asked for by name, not discovered`}
        >
          {t.title}
        </button>
      {/each}
    </div>

    <!-- ⛔ NO tabindex ON THE PANEL, AND IT IS THE APG RULE RATHER THAN AN
         omission: a tabpanel is made focusable only when it holds nothing
         focusable, and this one holds the refresh control. It HAD tabindex="0"
         and the real window showed the cost - webkit2gtk put focus on the
         scroll container after a keyboard activation, and a 3px ring drew
         itself around the entire panel for a person who had only pressed
         Space on the rail. Headless Chrome never reproduced it, which is the
         argument for running the thing. -->
    <div
      class="panel"
      role="tabpanel"
      id="pcg-panel"
      aria-labelledby={`tab-${store.current}`}
    >
      <PlanVsExec
        brief={held.brief}
        project={store.current}
        loading={store.loading}
        error={held.error || store.rosterError}
        readAt={held.readAt}
        onrefresh={() => void store.refresh()}
        {openRows}
      />

      <!-- The third level, named rather than mocked. -->
      <p class="third">
        A case can be divided by area of interest - health, finances and so on.
        rig carries no field for that yet, so there is nothing to divide by and
        this window does not invent one.
      </p>
    </div>
  {/if}
</div>

<style>
  .pcg {
    display: grid;
    grid-template-rows: auto auto minmax(0, 1fr);
    min-height: 0;
    height: 100%;
  }

  .head {
    display: flex;
    align-items: center;
    gap: 1rem;
    flex-wrap: wrap;
    padding: 0.7rem 1.15rem 0.6rem;
  }

  /* A segmented control rather than two links: the two sides are exclusive
     and exhaustive, which is what a segment says and a link does not. */
  .toggle {
    display: inline-flex;
    border: 1px solid var(--border-2);
    border-radius: var(--radius);
    overflow: hidden;
  }

  .toggle button {
    font: inherit;
    font-size: var(--fs--1);
    color: var(--fg-dim);
    background: var(--bg-2);
    border: 0;
    padding: 0.32rem 0.85rem;
    cursor: pointer;
    display: inline-flex;
    align-items: baseline;
    gap: 0.45rem;
  }

  .toggle button + button {
    border-inline-start: 1px solid var(--border-2);
  }

  .toggle button:hover {
    color: var(--fg);
  }

  .toggle button[aria-pressed="true"] {
    color: var(--fg);
    background: color-mix(in srgb, var(--hue) 13%, var(--panel));
  }

  .toggle button:focus-visible {
    outline: none;
    box-shadow: inset 0 0 0 var(--ring-w) var(--hue);
  }

  .toggle .n {
    font-family: var(--mono);
    font-variant-numeric: tabular-nums;
    color: var(--fg-dim);
  }

  .toggle button[aria-pressed="true"] .n {
    color: var(--fg);
  }

  .asked {
    margin: 0;
    font-size: var(--fs-0);
    line-height: 1.5;
    color: var(--fg-dim);
    max-width: 58ch;
  }

  .asked code {
    font-family: var(--mono);
    color: var(--fg);
  }

  /* ── one tab per project or case ──────────────────────────────────────── */

  .tabs {
    display: flex;
    gap: 0.15rem;
    border-bottom: 1px solid var(--border);
    padding: 0 0.8rem;
    overflow-x: auto;
  }

  [role="tab"] {
    font: inherit;
    font-size: var(--fs--1);
    color: var(--fg-dim);
    background: none;
    border: 0;
    border-bottom: 2px solid transparent;
    padding: 0.5rem 0.85rem;
    cursor: pointer;
    margin-bottom: -1px;
    white-space: nowrap;
  }

  [role="tab"]:hover {
    color: var(--fg);
  }

  [role="tab"][aria-selected="true"] {
    color: var(--fg);
    border-bottom-color: var(--hue);
  }

  /* Same ring, same width and same token as every other focus target in this
     window. Section 20's gate went red on a ring once; there is one ring in
     this shell and this is it. */
  [role="tab"]:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
    border-radius: 6px;
  }

  .panel {
    overflow: auto;
    min-height: 0;
    padding: calc(1.1rem * var(--den)) 1.15rem calc(2rem * var(--den));
  }

  .none {
    margin: 0;
    color: var(--fg-dim);
    font-size: var(--fs-0);
    line-height: 1.5;
    max-width: 62ch;
  }

  .third {
    margin: calc(1.6rem * var(--den)) 0 0;
    padding-top: 0.9rem;
    border-top: 1px solid var(--border);
    color: var(--fg-dim);
    font-size: var(--fs--1);
    line-height: 1.5;
    max-width: 72ch;
  }
</style>
