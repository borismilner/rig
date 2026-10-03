<!-- The program card: everything about one program that is not its GUI.

     ⛔ BORIS, 2026-10-03 (plan/55, requirements 42 to 44): "On the main
     panel, when clicking on a program row, a similar card in the absolute
     center of the screen should be opened with all the relevant information
     of the program. We should separate the program GUI and it's settings and
     all the other information [...] In each program GUI we should have a
     button that opens these settings and information for that specific
     program but the information should not part of the main gui to not
     interfere."

     So the card is a modal <dialog>, centred and fixed in size so switching
     its tabs never moves it (requirement 39), and it opens from two doors:
     a Programs row, and the context bar's "Info and settings" over a GUI.

     ⛔ TWO TABS SAY WHAT IS NOT BUILT YET, rather than being left out. Settings
     waits for section 47's per-program settings and Activity's cards wait for
     panel.put (plan/55 build order, slices 3 and 5). A card with the tab
     missing would read as "this program has no settings". -->
<script lang="ts">
  import { onMount } from "svelte";
  import * as RigService from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/rigservice.js";
  import type {
    Program,
    Running,
  } from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/models.js";
  import { forHow, stamp, stateOf } from "./estate";
  import { programGlyph, programIcon } from "./icons";
  import ProgramIcon from "./ProgramIcon.svelte";

  type Tab = "overview" | "settings" | "commands" | "activity";

  interface Props {
    program: Program;
    run?: Running;
    /* The program's own GUI is what is on screen behind the card. */
    inItsGui: boolean;
    tab?: Tab;
    onclose: () => void;
    onopengui: (id: string) => void;
    /* Rig's answer changed; the shell should read the estate again. */
    onchanged: () => void;
  }

  let {
    program,
    run,
    inItsGui,
    tab = "overview",
    onclose,
    onopengui,
    onchanged,
  }: Props = $props();

  // svelte-ignore state_referenced_locally
  let cur: Tab = $state(tab);
  let dialog: HTMLDialogElement;
  let busy = $state("");
  let said = $state("");
  let failed = $state(false);

  let st = $derived(stateOf(program, run));
  let art = $derived(programIcon(program.icon));
  let cmds = $derived(program.commandList ?? []);
  let hasGui = $derived(!!program.paneUrl);

  const TABS: [Tab, string][] = [
    ["overview", "Overview"],
    ["settings", "Settings"],
    ["commands", "Commands"],
    ["activity", "Activity"],
  ];
  let counts = $derived<Partial<Record<Tab, number>>>({
    commands: cmds.length,
  });

  onMount(() => {
    dialog.showModal();
    dialog
      .querySelector<HTMLElement>('.subtabs [aria-selected="true"]')
      ?.focus();
  });

  function go(t: Tab) {
    cur = t;
    queueMicrotask(() =>
      dialog
        .querySelector<HTMLElement>('.subtabs [aria-selected="true"]')
        ?.focus(),
    );
  }

  function ontabkey(e: KeyboardEvent) {
    const d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
    if (!d) return;
    e.preventDefault();
    const i = TABS.findIndex(([k]) => k === cur);
    go(TABS[(i + d + TABS.length) % TABS.length][0]);
  }

  const VERB = { start: "Start", stop: "Stop", restart: "Restart" } as const;

  async function supervise(action: "start" | "stop" | "restart") {
    busy = action;
    said = "";
    failed = false;
    try {
      const r = await RigService.Supervise(action, program.id);
      said = `${VERB[action]}: rig says ${program.id} is ${r.state ? r.state.replace("_", " ") : "no longer supervised"}.`;
    } catch (e) {
      failed = true;
      said = `${VERB[action]} refused: ${String(e)}`;
    } finally {
      busy = "";
      onchanged();
    }
  }
</script>

<dialog bind:this={dialog} class="card" aria-labelledby="pcard-title" {onclose}>
  <header>
    <span class="ic"
      >{#if art}<ProgramIcon node={art} />{:else}{programGlyph(
          program.icon,
          program.id,
        )}{/if}</span
    >
    <h2 id="pcard-title" class="t-sec">
      {program.name || program.id}
      <span class="ver">{program.version}</span>
    </h2>
    {#if hasGui}<span class="tag">GUI</span>{/if}
    {#if program.stale}<span class="tag warn">STALE</span>{/if}
    <button class="act x" aria-label="Close" onclick={() => dialog.close()}
      >&times;</button
    >
  </header>

  <div class="subtabs" role="tablist" aria-label="{program.id}'s card">
    {#each TABS as [k, label] (k)}
      <button
        role="tab"
        aria-selected={cur === k}
        tabindex={cur === k ? 0 : -1}
        onclick={() => go(k)}
        onkeydown={ontabkey}
        >{label}{#if counts[k] !== undefined}<span class="cnt">{counts[k]}</span
          >{/if}</button
      >
    {/each}
  </div>

  <div class="body" role="tabpanel">
    {#if cur === "overview"}
      <div class="pstate">
        <span class="pst big" data-tone={st.tone}><i></i>{st.word}</span>
        {#if run?.since}<span class="dim t-num"
            >for {forHow(run.since)}, since {stamp(run.since)}</span
          >{/if}
        <span class="sp"></span>
        {#each st.can as a (a)}
          <button class="act" disabled={!!busy} onclick={() => supervise(a)}
            >{busy === a ? `${VERB[a]}ing…` : VERB[a]}</button
          >
        {/each}
      </div>
      <!-- Polite, and in a fixed slot so a result appearing does not push
           the facts down (requirement 39). -->
      <p class="said" class:bad={failed} aria-live="polite">{said}</p>
      {#if run?.parked}
        <p class="ask"><b>Waiting on you:</b> {run.parked}</p>
      {/if}
      {#if program.description}<p class="desc">{program.description}</p>{/if}
      <dl class="facts">
        <dt>id</dt>
        <dd class="mono">{program.id}</dd>
        <dt>version</dt>
        <dd class="mono">{program.version || "not declared"}</dd>
        <dt>load</dt>
        <dd>{program.load || "not declared"}</dd>
        <dt>coverage</dt>
        <dd>
          {program.coverage}{#if program.coverageNote}: {program.coverageNote}{/if}
        </dd>
        <dt>supervision</dt>
        <dd>
          {#if run}supervised by rig, {run.restarts} restart{run.restarts === 1
              ? ""
              : "s"}{:else}not supervised by rig{/if}
        </dd>
        {#if run?.lastExit}
          <dt>last exit</dt>
          <dd class="mono">{run.lastExit}</dd>
        {/if}
        <dt>own GUI</dt>
        <dd class="mono">{program.paneUrl || "none declared"}</dd>
        <dt>hosted</dt>
        <dd>{program.hosted ? "by rig" : "no"}</dd>
        <dt>services</dt>
        <dd>
          {program.services?.length ? program.services.join(", ") : "none"}
        </dd>
        <dt>events</dt>
        <dd class="mono">
          {program.events?.length ? program.events.join(", ") : "none declared"}
        </dd>
        {#if program.stale}
          <dt>listing</dt>
          <dd>
            its binary changed since rig read it; rig has not read it again
          </dd>
        {/if}
      </dl>
    {:else if cur === "settings"}
      <p class="empty">
        {program.id} declares no settings rig can draw yet. Program settings arrive
        with section 47; when they do they appear here and under Settings, drawn the
        same way.
      </p>
    {:else if cur === "commands"}
      {#if cmds.length === 0}
        <p class="empty">{program.id} declares no commands.</p>
      {:else}
        <ul class="cmds">
          {#each cmds as c (c.id)}
            <li>
              <span class="mono">{program.id} {c.id}</span>
              <span class="what"
                >{c.summary ||
                  c.title ||
                  "no summary declared"}{#if c.description}<span class="dim">
                    {c.description}</span
                  >{/if}</span
              >
              {#if c.effects}<span class="tag" data-effect={c.effects}
                  >{c.effects}</span
                >{/if}
            </li>
          {/each}
        </ul>
      {/if}
    {:else}
      {#if run?.parked || run?.waiting || run?.lastExit}
        <ul class="acts-list">
          {#if run.parked}<li><b>Asked you:</b> {run.parked}</li>{/if}
          {#if run.waiting}<li><b>Waiting for:</b> {run.waiting}</li>{/if}
          {#if run.lastExit}<li><b>Last exit:</b> {run.lastExit}</li>{/if}
        </ul>
      {/if}
      <p class="empty">
        What {program.id} pushes to the board appears here once the board exists (plan/55,
        panel.put).
      </p>
    {/if}
  </div>

  <footer>
    {#if hasGui && !inItsGui}
      <button
        class="act primary"
        onclick={() => {
          dialog.close();
          onopengui(program.id);
        }}>Open its GUI</button
      >
    {/if}
    <span class="sp"></span>
    <button class="act" onclick={() => dialog.close()}>Close</button>
  </footer>
</dialog>

<style>
  /* Fixed width AND height, so changing tab never resizes it and the
     absolute centre stays the centre (requirements 39 and 42). */
  .card {
    /* Tailwind's preflight zeroes every margin, and a modal dialog is
       centred by margin: auto, so it is put back here. */
    margin: auto;
    inset: 0;
    width: min(940px, 94vw);
    height: min(640px, 86vh);
    padding: 0;
    border: 1px solid var(--border-2);
    border-radius: 14px;
    background: var(--panel);
    color: var(--fg);
    box-shadow: var(--shadow-lg);
    flex-direction: column;
  }
  .card[open] {
    display: flex;
  }
  .card::backdrop {
    background: rgb(0 0 0 / 0.45);
  }

  header {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    padding: 0.9rem 1.1rem;
    border-bottom: 1px solid var(--border);
  }
  h2 {
    margin: 0;
    flex: 1;
    font-size: var(--fs-1);
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    min-width: 0;
  }
  .ver {
    font: 400 var(--fs--1) var(--mono);
    color: var(--fg-dim);
  }
  .ic {
    width: 34px;
    height: 34px;
    border-radius: 9px;
    display: grid;
    place-items: center;
    background: var(--tint);
    font: 600 var(--fs--1) var(--mono);
    flex: none;
  }
  .ic :global(svg) {
    width: 18px;
    height: 18px;
  }

  .subtabs {
    padding: 0 1.1rem;
    flex: none;
  }

  .body {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 1rem 1.1rem;
  }

  footer {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    padding: 0.75rem 1.1rem;
    border-top: 1px solid var(--border);
  }
  .sp {
    flex: 1;
  }

  .pstate {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
  }
  .pst.big {
    font-weight: 600;
    font-size: var(--fs-0);
  }
  .dim {
    color: var(--fg-dim);
    font-size: var(--fs--1);
  }
  .said {
    min-height: 1.5em;
    margin: 0.4rem 0 0.6rem;
    font-size: var(--fs--1);
    color: var(--fg-dim);
    padding-bottom: 0.6rem;
    border-bottom: 1px solid var(--border);
  }
  .said.bad {
    color: var(--sem-bad);
  }
  .ask {
    margin: 0 0 0.8rem;
    border-left: 3px solid var(--sem-warn);
    padding-left: 0.6rem;
  }
  .ask b {
    color: var(--sem-warn);
  }
  .desc {
    margin: 0 0 0.8rem;
    max-width: 70ch;
  }

  .facts {
    display: grid;
    grid-template-columns: 140px minmax(0, 1fr);
    gap: 0.5rem 1rem;
    margin: 0;
    font-size: var(--fs--1);
  }
  .facts dt {
    color: var(--fg-dim);
  }
  .facts dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .mono {
    font-family: var(--mono);
  }

  .empty {
    color: var(--fg-dim);
    max-width: 62ch;
    margin: 1.5rem auto;
    text-align: center;
  }

  .cmds {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .cmds li {
    display: grid;
    grid-template-columns: 220px minmax(0, 1fr) auto;
    gap: 0.8rem;
    align-items: baseline;
    padding: 0.6rem 0.25rem;
    border-bottom: 1px solid var(--border);
    font-size: var(--fs--1);
  }
  .cmds .what {
    min-width: 0;
  }
  .tag[data-effect="destructive"],
  .tag[data-effect="drives-input"] {
    color: var(--sem-bad);
    border-color: var(--sem-bad);
  }
  .tag[data-effect="writes-files"],
  .tag[data-effect="network"] {
    color: var(--sem-warn);
    border-color: var(--sem-warn);
  }

  .acts-list {
    margin: 0 0 1rem;
    padding: 0;
    list-style: none;
    display: grid;
    gap: 0.4rem;
  }
</style>
