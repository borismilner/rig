<!-- Guidelines: what a program or an agent must be to work with rig, each
     rule dated, and which programs were built before which rule (plan/55
     requirement 29, decision 0266).

     ⛔ BORIS, 2026-10-03: "rig should expose full guidelines for programs and
     agents ... these instructions should be time-stamped so that we can know
     when a program may need to be adjusted."

     The rules and the build days are the running daemon's answer
     (rig.guidelines); this page only compares them, through lib/guidelines.ts,
     the same comparison Main's Programs table makes. -->
<script lang="ts">
  import type {
    Guidelines,
    Program,
  } from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/models.js";
  import { builtBefore, standing } from "./guidelines";

  interface Props {
    guide: Guidelines | null;
    loadError: string;
    loading: boolean;
    connected: boolean;
    programs: Program[];
    /* The program Main's table sent you here for; its rows are marked. */
    focus: string | null;
    /* Absent on the measurement fixture, which has nothing to read again. */
    onreload?: () => void;
  }
  let {
    guide,
    loadError,
    loading,
    connected,
    programs,
    focus,
    onreload,
  }: Props = $props();

  const nameOf = (id: string) => {
    const p = programs.find((x) => x.id === id);
    return p?.name || id;
  };
  let focusRules = $derived(
    guide && focus
      ? new Set(standing(guide, focus).newer.map((r) => r.id))
      : null,
  );
</script>

<div class="guide">
  <header class="head">
    <h1 class="t-sec">Guidelines</h1>
    {#if guide?.revision}<span class="mono dim">revision {guide.revision}</span
      >{/if}
    <span class="dim"
      >What a program or an agent must be to work with rig. Every rule carries
      the day it took effect.</span
    >
    <span class="sp"></span>
    {#if onreload}<button class="act" onclick={onreload} disabled={loading}
        >{loading ? "Reading…" : "Read again"}</button
      >{/if}
  </header>

  {#if loadError}
    <p class="empty">Rig did not answer the guidelines: {loadError}</p>
  {:else if !connected}
    <p class="empty">
      Rig is not answering, so there are no guidelines to show.
    </p>
  {:else if !guide}
    <p class="empty">Reading rig's guidelines.</p>
  {:else}
    <div class="scroll">
      <section>
        <h2>Programs against the guidelines</h2>
        <p class="dim note">
          Built is the day of the commit each binary was built from, read from
          its Go build info. A rule dated the same day is not counted, and a
          rule only ruled cannot be met yet, so neither is.
        </p>
        {#if guide.programs.length === 0}
          <p class="dim">No programs are registered.</p>
        {:else}
          <table class="progs">
            <thead>
              <tr>
                <th scope="col">Program</th>
                <th scope="col">Built</th>
                <th scope="col">Standing</th>
                <th scope="col">Rules since then</th>
              </tr>
            </thead>
            <tbody>
              {#each guide.programs as b (b.program)}
                {@const s = standing(guide, b.program)}
                <tr class:open={focus === b.program}>
                  <td class="pn">{nameOf(b.program)}</td>
                  <td class="mono"
                    >{b.day || "unknown"}{#if b.modified}<span
                        class="tag warn"
                        title="built from a tree with uncommitted changes"
                        >uncommitted</span
                      >{/if}</td
                  >
                  <td
                    ><span class="pst" data-tone={s.tone}><i></i>{s.word}</span
                    ></td
                  >
                  <td class="why">
                    {#if !b.day}<span class="dim">{b.unknownBecause}</span
                      >{:else if s.newer.length}{#each s.newer as r (r.id)}<span
                          class="tag warn"
                          title={r.title}>{r.id}</span
                        >{/each}{:else}<span class="dim">none</span>{/if}
                  </td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}
      </section>

      <section>
        <h2>The rules, newest first</h2>
        <ol class="rules">
          {#each guide.rules as r (r.id)}
            {@const behind = builtBefore(guide, r)}
            <li class:hit={focusRules?.has(r.id)}>
              <div class="rhead">
                <span class="mono">{r.id}</span>
                <time class="mono" datetime={r.date}>{r.date}</time>
                <b>{r.title}</b>
                <span class="tag">{r.who}</span>
                <span class="tag" class:warn={!r.built}
                  >{r.built ? "in force" : "ruled, not built"}</span
                >
              </div>
              <p>{r.body}</p>
              <div class="dim">
                {r.cite}{#if behind.length}{` · built before it: ${behind
                    .map(nameOf)
                    .join(", ")}`}{/if}
              </div>
            </li>
          {/each}
        </ol>
      </section>
    </div>
  {/if}
</div>

<style>
  /* The GUI owns its scroll (requirement 37): the header stays. */
  .guide {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    padding: calc(1rem * var(--den)) 1.15rem 0;
    box-sizing: border-box;
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: 0.9rem;
    flex-wrap: wrap;
    margin-bottom: 0.9rem;
    flex: none;
  }
  h1 {
    margin: 0;
    font-size: var(--fs-2);
  }
  h2 {
    font-size: var(--fs-0);
    margin: 0 0 0.4rem;
  }
  .sp {
    flex: 1;
  }
  .dim {
    color: var(--fg-dim);
    font-size: var(--fs--1);
  }
  .mono {
    font-family: var(--mono);
  }
  .empty {
    color: var(--fg-dim);
    text-align: center;
    margin: 2.5rem auto;
    max-width: 62ch;
  }
  .scroll {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding-bottom: 1.25rem;
  }
  section + section {
    margin-top: 1.4rem;
  }
  .note {
    margin: 0 0 0.6rem;
    max-width: 80ch;
  }

  .progs {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--fs--1);
  }
  .progs th {
    text-align: start;
    font-weight: 600;
    color: var(--fg-dim);
    padding: 0.5rem 0.75rem;
    border-bottom: 1px solid var(--border);
  }
  .progs td {
    padding: 0.4rem 0.75rem;
    border-bottom: 1px solid var(--border);
    vertical-align: middle;
  }
  .progs .pn {
    font-weight: 600;
  }
  .progs tr.open td {
    background: var(--glow);
  }
  .progs .tag {
    margin-inline-start: 0.4rem;
  }
  .why .tag:first-child {
    margin-inline-start: 0;
  }

  .rules {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }
  .rules li {
    border: 1px solid var(--border);
    border-radius: 9px;
    padding: 0.6rem 0.8rem;
    max-width: 90ch;
  }
  .rules li.hit {
    border-color: var(--sem-warn);
  }
  .rhead {
    display: flex;
    align-items: baseline;
    gap: 0.6rem;
    flex-wrap: wrap;
  }
  .rules p {
    margin: 0.35rem 0;
  }
</style>
