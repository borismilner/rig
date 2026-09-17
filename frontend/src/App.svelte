<!-- The shell, and its top level is DASHBOARD-OR-GUI.

     ⛔ BORIS, 2026-09-17, twice in one afternoon, and the second correction
     supersedes part of the first (section 11, requirements 6 replaced, 10, 11,
     13, 16, 17):

       "I think the main GUI show show a general-purpose dashboard with all the
        most important information we'll define in the future. Choosing a
        registered program we'll see the GUI of that program but we can always
        go back to the main dashboard from all places conveniently. [...]
        `rig` settings is a separate GUI and can be accessed both from a
        dedicated GUI button and from the system-tray."

       "Maybe the things that can be selected from the left are not only the
        different programs but we can generalize it into different GUIs. So a
        GUI can be a GUI of a program but it could also be a GUI of some
        internal `rig` functionality like project management and things like
        that. I'd also like these selectables on the left to have icons and
        maybe colors so that they are easily distinguishable."

     So the rail lists GUIs; a GUI is either an internal rig capability's or a
     registered program's; the dashboard is a destination rather than the empty
     state before one is picked; and settings is neither a tab nor a pane.

     ⛔ TWO SHAPES THAT WERE BUILT TODAY AND ARE SUPERSEDED, recorded so nobody
     rebuilds them: a tab bar across the top of the shell, and a rig entry in
     the rail pretending to be a registered program. Requirement 16 retired the
     second and requirement 6 retired the first. The tabs are now inside the
     project/case GUI, one per project or case.

     Nothing here is mocked: an empty rail means an empty registry, and a rail
     that cannot be read says so in the strip. -->
<script lang="ts">
  import { onMount } from "svelte";
  import * as RigService from "../bindings/github.com/boris-milner/rig/cmd/rigwindow/rigservice.js";
  import type {
    Deployment,
    Health,
    Program,
  } from "../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";
  import { applyTheme, preferredMode, watchMode } from "./lib/theme";
  import type { Mode } from "./lib/theme";
  import Rail from "./lib/Rail.svelte";
  import type { RailEntry } from "./lib/Rail.svelte";
  import ContextBar from "./lib/ContextBar.svelte";
  import Pane from "./lib/Pane.svelte";
  import StatusStrip from "./lib/StatusStrip.svelte";
  import Settings from "./lib/Settings.svelte";
  import Dashboard from "./lib/Dashboard.svelte";
  import ProjectCaseGui from "./lib/ProjectCaseGui.svelte";
  import { INTERNAL_GUIS, internalGui, PROJECT_CASE_GUI } from "./lib/guis";
  import { createRigStore } from "./lib/rigstore.svelte";
  import { PROGRAMS, BUILD, BRIEF, DEPLOYMENT } from "./lib/fixtures";

  /* ── the fixtures, and none of them is a mock of the product path ────────
   *
   * The contrast gate is why they exist: a browser cannot reach the Wails
   * runtime, so an audited page has an empty rail, no brief and no panel
   * content, and every pass measures zero. This project has already shipped
   * that as a clean run. Each flag seeds one surface the gate could not
   * otherwise reach; the runtime is present in the real window and none of
   * these is on unless it is asked for in the URL.
   *
   *   ?fixture=1        the rail and a generated detail pane
   *   ?pane=1           a program's own pane, and its unserved state
   *   ?settings=1       the settings panel over a seeded rail
   *   ?dash=1           the dashboard, which is the default destination
   *   ?gui=projects     the project/case GUI, projects side, one project tab
   *   ?gui=cases        the same GUI on its cases side, which has no records
   *
   * ⛔ THE LAST THREE ARE NOT IN `make contrast-window` YET. That target lives
   * in the Makefile, which this seat does not own. Until the URLs are added to
   * it the gate measures the shell and NOT these surfaces, which is the exact
   * defect the fixture mechanism exists to close. They were measured by hand
   * instead; the numbers are in the agent-work STATUS.md.
   */
  const params = new URLSearchParams(location.search);
  const paneFixture = params.get("pane") === "1";
  const settingsFixture = params.get("settings") === "1";
  const dashFixture = params.get("dash") === "1";
  const guiParam = params.get("gui");
  const guiFixture = guiParam === "projects" || guiParam === "cases";
  const railFixture = params.get("fixture") === "1";
  const fixture =
    railFixture || paneFixture || settingsFixture || dashFixture || guiFixture;

  let programs: Program[] = $state(fixture ? PROGRAMS : []);
  let health: Health = $state(
    fixture
      ? {
          connected: true,
          socket: "/run/user/1000/rig/rigd.sock",
          detail: "",
          programs: PROGRAMS.length,
        }
      : { connected: false, socket: "", detail: "", programs: 0 },
  );
  let build: Record<string, string> | null = $state(fixture ? BUILD : null);
  // What is running, artefact by artefact. Read once beside Build, for the
  // same reason: it does not change while the window is open, and a poll would
  // be asking the daemon a question whose answer only a redeploy can move.
  let deployment: Deployment | null = $state(fixture ? DEPLOYMENT : null);

  // WHERE YOU ARE, as two pieces rather than one. `atHome` is not
  // `selected === null`: the dashboard is a destination in its own right, and
  // leaving a GUI for it must not forget which GUI you were in.
  let atHome = $state(!(paneFixture || guiFixture || railFixture));
  let selected: string | null = $state(
    paneFixture
      ? "quarry"
      : guiFixture
        ? PROJECT_CASE_GUI.id
        : fixture
          ? "graft"
          : null,
  );
  let lastRead = $state(fixture ? "09:53:41" : "");

  // rig's own project and case records. One store shared by the dashboard and
  // by the project/case GUI, so two surfaces cannot show two answers with no
  // way to tell which is older. There is no poll on it - see
  // rigstore.svelte.ts, which says why at length.
  const rig = createRigStore();
  if (fixture) rig.seed([], "rig", BRIEF, "09:53:41");

  // Held rather than only applied, because the pane has to push the token set
  // into a program's own page and a mode change has to reach it too. One
  // source: this is the same mode applyTheme is called with.
  let mode: Mode = $state(preferredMode());

  // M1a step 5, and now requirement 13's second door. Opened with "," and from
  // the rail's gear, closed with Esc or its own button. The keyboard path was
  // never the problem; what was missing was the control, and the rail is where
  // the room was found.
  let settingsOpen = $state(settingsFixture);
  // Bumped whenever the live theme changes, so Pane re-pushes the token set to
  // every program. Without it the window changes colour and the panes keep the
  // set they were handed, which is the one bug a shell-wide theme must not
  // have.
  let themeGen = $state(0);

  let gui = $derived(atHome ? null : internalGui(selected));
  let current = $derived(
    gui ? null : (programs.find((p) => p.id === selected) ?? null),
  );

  /* ⛔ THE OWNED HUE, AND IT IS SET HERE RATHER THAN THROUGH applyTheme.
   *
   * Section 11: "The owned hue is the only saturated colour on screen while
   * you are in it. A host that wears the colour of whatever it is holding."
   * theme.ts's applyWith hard-codes --hue to --fg and the settings panel calls
   * it on every apply and every mode change, so routing an owned hue through
   * applyTheme(mode, hue) would be silently reset the first time anyone
   * touched settings. Set on the shell's own element, these inherit to
   * everything inside it - and the settings dialog is a SIBLING of .shell, so
   * it stays achromatic and theme.ts is untouched.
   *
   * A PROGRAM NEVER GETS ONE. rigv1.Identity carries no hue (requirement 17's
   * own table), so a program's GUI leaves the shell achromatic - which is the
   * documented default rather than a gap, and is not faked from its id.
   */
  let shellHue = $derived(
    gui ? `--hue: var(--h-${gui.hue}); --ohue: var(--o-${gui.hue})` : "",
  );

  // Internal GUIs first, then the registry in the order rig returned it.
  // Section 11: the rail never re-orders under your hand, so nothing sorts.
  let entries: RailEntry[] = $derived([
    ...INTERNAL_GUIS.map((g) => ({
      id: g.id,
      title: g.title,
      glyph: "",
      icon: g.icon,
      hue: g.hue,
    })),
    ...programs.map((p) => ({
      id: p.id,
      title: `${p.name || p.id} ${p.version}`,
      glyph: (p.icon || p.id.slice(0, 2)).slice(0, 2),
    })),
  ]);

  function goHome() {
    atHome = true;
  }

  function pick(id: string) {
    atHome = false;
    selected = id;
  }

  async function refresh() {
    // Health is the one call that must not throw its way out of here: if it
    // does, every later line is skipped and the shell sits in its initial
    // state, which looks exactly like "still loading" and says nothing.
    let h: Health;
    try {
      h = await RigService.Health();
    } catch (e) {
      programs = [];
      if (!internalGui(selected)) selected = null;
      health = {
        connected: false,
        socket: health.socket,
        detail: `the window could not reach its own service: ${String(e)}`,
        programs: 0,
      };
      lastRead = stamp();
      return;
    }
    health = h;

    if (!h.connected) {
      // Not clearing the rail would leave the last good read on screen looking
      // live, which is the failure mode a status strip cannot rescue. An
      // internal GUI stays: it is this shell's, not the registry's, so a dead
      // daemon does not remove it - what it removes is the data behind it,
      // which that GUI reports for itself.
      programs = [];
      if (!internalGui(selected)) selected = null;
      lastRead = stamp();
      return;
    }

    try {
      programs = (await RigService.Programs()) ?? [];
    } catch (e) {
      programs = [];
      health = { ...h, connected: false, detail: String(e) };
    }

    // A selection survives a refresh unless the program it names has gone.
    // Falling back to the dashboard rather than to another program: picking
    // one on a person's behalf is the rail re-ordering under their hand by
    // another route.
    if (
      selected &&
      !internalGui(selected) &&
      !programs.some((p) => p.id === selected)
    ) {
      selected = null;
      atHome = true;
    }
    lastRead = stamp();
  }

  function stamp(): string {
    return new Date().toLocaleTimeString([], { hour12: false });
  }

  // A poll, and it is a stopgap with a date on it rather than a design.
  // Section 5h's event stream lands at M4 with config.changed as its first
  // kind, and the registry changing is exactly the second kind; until then the
  // rail would otherwise not notice a program registering. It stops while the
  // window is hidden, because section 17 budgets the daemon's wakeups and a
  // background window has no business costing any.
  //
  // ⛔ IT COVERS THE REGISTRY AND NOTHING ELSE. The project and case records
  // are NOT polled: requirement 8 is deferred by his own ruling, and extending
  // this timer to that data would be the poll-behind-an-event-API trap section
  // 11 names by name.
  const POLL_MS = 3000;
  let timer: ReturnType<typeof setInterval> | undefined;

  function startPolling() {
    if (timer !== undefined) return;
    timer = setInterval(refresh, POLL_MS);
  }

  function stopPolling() {
    if (timer === undefined) return;
    clearInterval(timer);
    timer = undefined;
  }

  function onvisibility() {
    if (document.hidden) {
      stopPolling();
    } else {
      void refresh();
      startPolling();
    }
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      // Settings first: it is the thing most recently opened, and Esc closing
      // the panel is the path M1a step 5 shipped.
      if (settingsOpen) {
        settingsOpen = false;
        return;
      }
      // ⛔ THE SECOND DOOR HOME, AND IT IS NOT THE LOAD-BEARING ONE. While a
      // program's own page has the keyboard this handler never fires - a
      // cross-origin frame does not forward keystrokes to its parent, the same
      // boundary section 11 records for :focus-within. The rail's home mark is
      // the way back that cannot be taken away; this is the convenience.
      if (!atHome) {
        goHome();
        return;
      }
    }
    // A single-letter shortcut must not fire while the panel has the keyboard:
    // "r" inside a settings field would refresh the rail mid-edit.
    if (settingsOpen) return;
    // Nor while any text control has it.
    const t = e.target as HTMLElement | null;
    if (t && /^(INPUT|SELECT|TEXTAREA)$/.test(t.tagName)) return;
    if (e.key === "," && !e.ctrlKey && !e.metaKey && !e.altKey) {
      settingsOpen = true;
      return;
    }
    if (e.key === "r" && !e.ctrlKey && !e.metaKey && !e.altKey) void refresh();
  }

  onMount(() => {
    applyTheme(mode);
    const unwatch = watchMode((m) => {
      mode = m;
      applyTheme(m);
    });
    if (fixture) {
      // Nothing to poll and nothing to read: the point is a page that holds
      // still, and a failing read would empty the rail mid-measurement.
      return unwatch;
    }
    void refresh();
    RigService.Build()
      .then((b) => (build = b))
      .catch(() => (build = null));
    RigService.Deployment()
      .then((d) => (deployment = d))
      .catch(() => (deployment = null));
    startPolling();
    document.addEventListener("visibilitychange", onvisibility);
    return () => {
      stopPolling();
      unwatch();
      document.removeEventListener("visibilitychange", onvisibility);
    };
  });
</script>

<svelte:window on:keydown={onkeydown} />

<div class="shell" style={shellHue}>
  <Rail
    {entries}
    {selected}
    {atHome}
    onselect={pick}
    onhome={goHome}
    onsettings={() => (settingsOpen = true)}
  />

  <div class="main">
    <ContextBar
      {atHome}
      {gui}
      program={current}
      programCount={programs.length}
      connected={health.connected}
    />

    {#if atHome}
      <div class="pane">
        <Dashboard
          {health}
          {programs}
          {build}
          store={rig}
          {lastRead}
          {deployment}
          onselect={pick}
        />
      </div>
    {:else if gui}
      <!-- An internal GUI owns the whole pane area and draws its own internal
           layout, which is what makes the tabs one level down rather than
           shell chrome. -->
      <div class="pane bleed">
        <ProjectCaseGui
          store={rig}
          initialSide={guiParam === "cases" ? "cases" : "projects"}
          openRows={guiFixture}
        />
      </div>
    {:else}
      <Pane
        program={current}
        {programs}
        detail={health.detail}
        connected={health.connected}
        {mode}
        {themeGen}
        {paneFixture}
      />
    {/if}

    <StatusStrip
      connected={health.connected}
      socket={health.socket}
      programCount={programs.length}
      {build}
      {lastRead}
    />
  </div>
</div>

{#if settingsOpen}
  <Settings
    {mode}
    onclose={() => (settingsOpen = false)}
    onmode={(m) => (mode = m)}
    onthemechange={() => (themeGen += 1)}
  />
{/if}
