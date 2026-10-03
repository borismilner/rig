<!-- Main: rig and the estate it carries, in the shape plan/55 settled with
     Boris on 2026-10-03 (design/dashboard/ is the reference).

     ⛔ THE LAYOUT IS HIS, REQUIREMENT BY REQUIREMENT. Main's parts are tabs
     inside it rather than one long page (16); the head, the figures and those
     tabs stay put and only the content under them scrolls (37); a Programs
     row opens the program's card in the centre of the screen, and its GUI is
     the rail's door, not this page's (42, 43).

     ⛔ NOTHING ON THIS PAGE IS INVENTED. Every figure is read from the calls
     the window already makes - Health, Programs, Supervision, Deployment,
     Build, Notifications. The board needs a verb rig does not have yet
     (plan/55 build order, slice 3), so the Board tab says so instead of
     showing a sample.

     The notifications panel is the right-hand column (7 to 9); Needs you
     is every question still waiting on him plus every parked program (24),
     and the "need you" figure and the tab count are that same total. -->
<script lang="ts">
  import type {
    Deployment as DeploymentState,
    Guidelines,
    Health,
    Note,
    NoteList,
    Program,
    Running,
  } from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/models.js";
  import { forHow, stateOf } from "./estate";
  import { standing } from "./guidelines";
  import { programGlyph, programIcon } from "./icons";
  import ProgramIcon from "./ProgramIcon.svelte";
  import Deployment from "./Deployment.svelte";
  import Notifications from "./Notifications.svelte";
  import { waiting } from "./notes";

  export type MainTab = "needs" | "programs" | "board";

  interface Props {
    health: Health;
    programs: Program[];
    running: Running[];
    build: Record<string, string> | null;
    lastRead: string;
    deployment: DeploymentState | null;
    /* Which inner tab, held by the shell so leaving for a GUI and coming
       back lands where you were. */
    tab: MainTab;
    /** Opens a program's card (requirement 42). */
    onopen: (id: string) => void;
    /* rig.guidelines, or null before the first read (requirement 29). */
    guide: Guidelines | null;
    /** Opens the Guidelines GUI with this program's rules marked. */
    onguide: (id: string) => void;
    /* RigService.Notifications, or null before the first read. */
    notes: NoteList | null;
    notesError: string;
    noteQ: string;
    noteSrc: string;
    /** Opens a notification's card (requirement 26). */
    onnote: (n: Note) => void;
    /** Answers a waiting notification; rejects with rig's refusal. */
    onanswer: (
      id: string,
      reply: string,
      text: string,
      dismissed: boolean,
    ) => Promise<void>;
    /* The measurement fixture's fixed clock. */
    now?: number;
  }

  let {
    health,
    programs,
    running,
    build,
    lastRead,
    deployment,
    tab = $bindable(),
    onopen,
    guide,
    onguide,
    notes,
    notesError,
    noteQ = $bindable(),
    noteSrc = $bindable(),
    onnote,
    onanswer,
    now,
  }: Props = $props();

  let asks = $derived((notes?.notes ?? []).filter(waiting));
  // A refusal from an inline answer, by notification, shown on its row.
  let refused: Record<string, string> = $state({});
  let busy: string | null = $state(null);

  async function answer(n: Note, reply: string) {
    busy = n.id;
    refused[n.id] = "";
    try {
      await onanswer(n.id, reply, "", false);
    } catch (e) {
      refused[n.id] = String(e);
    } finally {
      busy = null;
    }
  }

  let byId = $derived(new Map(running.map((r) => [r.id, r])));
  let rows = $derived(
    programs.map((p) => ({
      p,
      run: byId.get(p.id),
      st: stateOf(p, byId.get(p.id)),
    })),
  );
  let asking = $derived(rows.filter((r) => r.run?.parked));
  let needN = $derived(asks.length + asking.length);
  let up = $derived(rows.filter((r) => r.st.tone === "good").length);
  let resting = $derived(rows.filter((r) => r.st.tone === "rest").length);
  let bad = $derived(rows.filter((r) => r.st.tone === "bad").length);

  type Fig = { n: number; label: string; tone?: string; to: MainTab };
  let figs: Fig[] = $derived([
    { n: programs.length, label: "programs", to: "programs" },
    { n: up, label: "up", tone: up ? "good" : "", to: "programs" },
    { n: resting, label: "at rest, on call", to: "programs" },
    {
      n: bad,
      label: "down or quarantined",
      tone: bad ? "bad" : "",
      to: "programs",
    },
    {
      n: needN,
      label: "need you",
      tone: needN ? "warn" : "",
      to: "needs",
    },
  ]);

  const TABS: [MainTab, string][] = [
    ["needs", "Needs you"],
    ["programs", "Programs"],
    ["board", "Board"],
  ];
  let counts = $derived<Record<MainTab, string>>({
    needs: String(needN),
    programs: String(programs.length),
    board: "",
  });

  function ontabkey(e: KeyboardEvent) {
    const d = e.key === "ArrowRight" ? 1 : e.key === "ArrowLeft" ? -1 : 0;
    if (!d) return;
    e.preventDefault();
    const i = TABS.findIndex(([k]) => k === tab);
    tab = TABS[(i + d + TABS.length) % TABS.length][0];
    queueMicrotask(() =>
      document
        .querySelector<HTMLElement>('.dash .subtabs [aria-selected="true"]')
        ?.focus(),
    );
  }

  // Requirement 42: the whole row opens the card, and the name is the
  // keyboard's way in, so a click on the name must not open it twice.
  function onrow(e: MouseEvent, id: string) {
    if ((e.target as HTMLElement).closest("button")) return;
    onopen(id);
  }

  let skew = $derived(!!deployment?.reached && !deployment.agree);
</script>

<div class="home">
<div class="dash">
  <header class="head">
    <h1 class="t-sec">rig</h1>
    {#if deployment?.daemonVersion}<span class="mono dim"
        >{deployment.daemonVersion}</span
      >{/if}
    <span class="state">
      <span class="dot" class:bad={!health.connected}></span>
      {#if health.connected}
        answering on <code>{health.socket}</code>
      {:else}
        not answering. {health.detail || "The daemon is not reachable."}
      {/if}
    </span>
    {#if lastRead}<span class="sp"></span><span class="mono dim"
        >read at {lastRead}</span
      >{/if}
  </header>

  {#if skew}
    <!-- The one loud line Main can carry without a verb: B89's skew, which
         a person cannot otherwise see without four commands. -->
    <p class="skew">{deployment?.verdict}</p>
  {/if}

  <div class="figs">
    {#each figs as f (f.label)}
      <button
        class="fig"
        data-tone={f.tone || null}
        title="Show {f.to === 'needs' ? 'what needs you' : 'the programs'}"
        onclick={() => (tab = f.to)}
      >
        <span class="n t-num">{health.connected ? f.n : "–"}</span>
        <span class="l">{f.label}</span>
      </button>
    {/each}
  </div>

  <div class="subtabs" role="tablist" aria-label="Main">
    {#each TABS as [k, label] (k)}
      <button
        role="tab"
        aria-selected={tab === k}
        tabindex={tab === k ? 0 : -1}
        class:hot={k === "needs" && needN > 0}
        onclick={() => (tab = k)}
        onkeydown={ontabkey}
        >{label}{#if counts[k]}<span class="cnt">{counts[k]}</span>{/if}</button
      >
    {/each}
  </div>

  <div class="scroll" role="tabpanel">
    {#if !health.connected}
      <p class="empty">
        Rig is not answering, so there is nothing to list. Nothing here is a
        stale copy: there is no copy.
      </p>
    {:else if tab === "needs"}
      {#if needN === 0}
        <p class="empty">
          Nothing needs you. A question a program asks, and a program rig
          supervises that is parked, land here.
        </p>
      {:else}
        <ul class="needs">
          {#each asks as n (n.id)}
            <li>
              <div class="who">
                <b>{n.title}</b>
                <span class="dim">from {n.sender}</span>
              </div>
              {#if n.body}<p>{n.body}</p>{/if}
              <div class="opts">
                {#each n.replies as r, i (r)}
                  <button
                    class="act"
                    class:primary={i === 0}
                    disabled={busy === n.id}
                    onclick={() => answer(n, r)}>{r}</button
                  >
                {/each}
                <button class="act" onclick={() => onnote(n)}
                  >{n.replyText && n.replies.length === 0
                    ? "Answer"
                    : "Details"}</button
                >
              </div>
              {#if refused[n.id]}<p class="refused" role="alert">
                  {refused[n.id]}
                </p>{/if}
            </li>
          {/each}
          {#each asking as r (r.p.id)}
            <li>
              <div class="who">
                <b>{r.p.name || r.p.id}</b>
                <span class="pst" data-tone={r.st.tone}><i></i>{r.st.word}</span
                >
              </div>
              <p>{r.run?.parked}</p>
              <button class="act" onclick={() => onopen(r.p.id)}
                >Open its card</button
              >
            </li>
          {/each}
        </ul>
      {/if}
    {:else if tab === "programs"}
      {#if programs.length === 0}
        <p class="empty">
          No programs are registered. Rig is answering and its registry is
          empty; one appears here the moment it registers.
        </p>
      {:else}
        <table class="progs">
          <thead>
            <tr>
              <th scope="col">Program</th>
              <th scope="col">Version</th>
              <th scope="col">Load</th>
              <th scope="col">State</th>
              <th scope="col" class="num">Commands</th>
              <th scope="col" class="num">Restarts</th>
              <th scope="col">Guidelines</th>
            </tr>
          </thead>
          <tbody>
            {#each rows as { p, run, st } (p.id)}
              {@const art = programIcon(p.icon)}
              <tr class="row" onclick={(e) => onrow(e, p.id)}>
                <td>
                  <button
                    class="pname"
                    aria-label="{p.name || p.id}: open its card"
                    onclick={() => onopen(p.id)}
                  >
                    <span class="ic"
                      >{#if art}<ProgramIcon node={art} />{:else}{programGlyph(
                          p.icon,
                          p.id,
                        )}{/if}</span
                    >
                    {p.name || p.id}
                    {#if p.paneUrl}<span class="tag">GUI</span>{/if}
                  </button>
                </td>
                <td class="mono">{p.version}</td>
                <td>{p.load || "–"}</td>
                <td>
                  <span class="pst" data-tone={st.tone}><i></i>{st.word}</span>
                  {#if run?.since}<span class="dim">
                      {forHow(run.since)}</span
                    >{/if}
                  {#if p.stale}<span class="tag warn">stale</span>{/if}
                </td>
                <td class="num">{p.commands}</td>
                <td class="num" class:bad={(run?.restarts ?? 0) >= 5}
                  >{run ? run.restarts : "–"}</td
                >
                <td>
                  {#if guide}
                    {@const g = standing(guide, p.id)}
                    <button
                      class="gstat"
                      title={g.newer.length
                        ? `${g.newer.length} rule${g.newer.length > 1 ? "s" : ""} newer than what ${p.name || p.id} was built from`
                        : g.build?.day
                          ? `built from a commit of ${g.build.day}, current`
                          : (g.build?.unknownBecause ?? "rig did not say")}
                      onclick={() => onguide(p.id)}
                      ><span class="pst" data-tone={g.tone}
                        ><i></i>{g.word}</span
                      ></button
                    >
                  {:else}<span class="dim">–</span>{/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}

      <div class="below">
        <Deployment {deployment} />
        <footer class="prov">
          <span class="dim">This window</span>
          {#each Object.entries(build ?? {}) as [k, v] (k)}
            <span
              ><span class="dim">{k}</span> <span class="mono">{v}</span></span
            >
          {/each}
        </footer>
      </div>
    {:else}
      <p class="empty">
        The board is where programs and agents put what they are working on. It
        needs rig's panel.put, which is not built yet (plan/55 build order,
        slice 3), so there is nothing to show and nothing is faked.
      </p>
    {/if}
  </div>
</div>
<Notifications
  list={notes}
  loadError={notesError}
  connected={health.connected}
  bind:q={noteQ}
  bind:src={noteSrc}
  onopen={onnote}
  {now}
/>
</div>

<style>
  /* Main on the left, the notifications panel on the right, each owning
     its own scroll. */
  .home {
    display: grid;
    grid-template-columns: minmax(0, 1fr) clamp(280px, 28vw, 360px);
    height: 100%;
    min-height: 0;
  }
  /* The page owns its scroll (requirement 37): the shell hands it the whole
     area, the top four rows keep their height and .scroll takes the rest. */
  .dash {
    min-width: 0;
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    padding: calc(1rem * var(--den)) 1.15rem 0;
    box-sizing: border-box;
  }
  .dash > :not(.scroll) {
    flex: none;
  }

  .head {
    display: flex;
    align-items: baseline;
    gap: 0.9rem;
    flex-wrap: wrap;
    margin-bottom: 0.8rem;
  }
  h1 {
    margin: 0;
    font-size: var(--fs-2);
  }
  .state {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    color: var(--fg-dim);
    font-size: var(--fs--1);
  }
  .state code {
    font-family: var(--mono);
    color: var(--fg);
  }
  .sp {
    flex: 1;
  }
  .mono {
    font-family: var(--mono);
  }
  .dim {
    color: var(--fg-dim);
    font-size: var(--fs--1);
  }

  .skew {
    margin: 0 0 0.8rem;
    padding: 0.5rem 0.8rem;
    border-left: 3px solid var(--sem-bad);
    background: var(--tint);
    font-size: var(--fs--1);
  }

  /* Five figures in one row, each a door to the tab that explains it. */
  .figs {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: 10px;
    margin-bottom: 1rem;
  }
  .fig {
    font: inherit;
    color: inherit;
    text-align: start;
    cursor: pointer;
    display: grid;
    gap: 0.1rem;
    background: var(--bg-2);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.6rem 0.75rem;
  }
  .fig:hover {
    border-color: var(--fg-faint);
  }
  .fig:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }
  .fig .n {
    font-size: var(--fs-2);
    font-weight: 600;
    line-height: 1.1;
  }
  .fig .l {
    color: var(--fg-dim);
    font-size: var(--fs--1);
  }
  .fig[data-tone="good"] .n {
    color: var(--sem-good);
  }
  .fig[data-tone="warn"] .n {
    color: var(--sem-warn);
  }
  .fig[data-tone="bad"] .n {
    color: var(--sem-bad);
  }

  .scroll {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 0.9rem 0 1.2rem;
  }

  .empty {
    color: var(--fg-dim);
    max-width: 62ch;
    margin: 2.5rem auto;
    text-align: center;
  }

  /* ── Needs you ─────────────────────────────────────────────────────── */

  .needs {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .needs li {
    display: grid;
    gap: 0.4rem;
    justify-items: start;
    padding: 0.8rem 0.9rem;
    border-bottom: 1px solid var(--border);
    box-shadow: inset 3px 0 0 var(--sem-warn);
  }
  .needs .who {
    display: flex;
    gap: 0.8rem;
    align-items: baseline;
  }
  .needs p {
    margin: 0;
  }
  .needs .opts {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }
  .needs .refused {
    color: var(--sem-bad);
    font-size: var(--fs--1);
  }

  /* ── Programs ──────────────────────────────────────────────────────── */

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
    white-space: nowrap;
  }
  .progs .num {
    text-align: end;
    font-variant-numeric: tabular-nums;
  }
  .progs td.bad {
    color: var(--sem-bad);
  }
  .progs tr.row {
    cursor: pointer;
  }
  .progs tr.row:hover td {
    background: var(--glow);
  }
  .progs .tag {
    margin-inline-start: 0.5rem;
  }
  .gstat {
    font: inherit;
    color: inherit;
    background: none;
    border: 0;
    padding: 0.1rem 0.2rem;
    border-radius: 6px;
    cursor: pointer;
  }
  .gstat:hover .pst {
    text-decoration: underline;
  }
  .gstat:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }

  .pname {
    font: inherit;
    font-weight: 600;
    color: var(--fg);
    background: none;
    border: 0;
    padding: 0.1rem 0;
    display: inline-flex;
    align-items: center;
    gap: 0.6rem;
    cursor: pointer;
    border-radius: 6px;
  }
  .pname:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }
  .ic {
    width: 26px;
    height: 26px;
    border-radius: 7px;
    display: grid;
    place-items: center;
    background: var(--tint);
    font: 600 var(--fs--2) var(--mono);
  }
  .ic :global(svg) {
    width: 15px;
    height: 15px;
  }

  .below {
    margin-top: 1.5rem;
    display: grid;
    gap: 1rem;
    max-width: 760px;
  }
  .prov {
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem 1.2rem;
    font-size: var(--fs--1);
  }
</style>
