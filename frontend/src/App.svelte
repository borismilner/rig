<!-- M1a step 2: the shell over the live wire.
     Rail, context bar, pane and status strip (section 23's M1a row), with the
     rail drawn from rig's own registry over the socket. Nothing here is
     mocked: an empty rail means an empty registry, and a rail that cannot be
     read says so in the strip. -->
<script lang="ts">
  import { onMount } from "svelte";
  import * as RigService from "../bindings/github.com/boris-milner/rig/cmd/rigwindow/rigservice.js";
  import type {
    Health,
    Program,
  } from "../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";
  import { applyTheme, preferredMode, watchMode } from "./lib/theme";
  import type { Mode } from "./lib/theme";
  import Rail from "./lib/Rail.svelte";
  import ContextBar from "./lib/ContextBar.svelte";
  import Pane from "./lib/Pane.svelte";
  import StatusStrip from "./lib/StatusStrip.svelte";
  import Settings from "./lib/Settings.svelte";

  // A measurement fixture, and it is not a mock of the product path.
  //
  // The contrast gate is the reason it exists. The rail's focus ring is exactly
  // what section 20's gate was rebuilt for - it shipped at 2.17:1 dark once -
  // and a browser cannot reach the Wails runtime, so an audited page has an
  // empty rail and the gate measures zero focus targets. ?fixture=1 seeds
  // programs so the markers, the notch and their focus rings are on screen to
  // be measured. It changes nothing in the window: the runtime is present
  // there, and the flag has to be asked for in the URL.
  const FIXTURE: Program[] = [
    {
      id: "shelf",
      name: "shelf",
      version: "v1.4.0",
      icon: "sh",
      description: "the library",
      coverage: "full",
      coverageNote: "3 of 20 commands declared",
      services: ["storage", "logs"],
      hosted: false,
      commands: 3,
      paneUrl: "",
    },
    {
      id: "graft",
      name: "graft",
      version: "v0.9.1",
      icon: "gr",
      description: "the grafter",
      coverage: "partial",
      coverageNote: "",
      services: [],
      hosted: false,
      commands: 7,
      paneUrl: "",
    },
    {
      id: "snapper",
      name: "snapper",
      version: "v2.0.0",
      icon: "sn",
      description: "screen capture",
      coverage: "partial",
      coverageNote: "",
      services: [],
      hosted: true,
      commands: 1,
      paneUrl: "",
    },
    // The pane fixture's program, and the port is DEAD on purpose.
    //
    // Nothing is listening on 7399 and nothing should be: onload never fires,
    // so the unserved state renders, and the gate measures its real colours
    // with no background server for the gate to depend on. A live port would
    // make this measurement flaky by construction.
    {
      id: "quarry",
      name: "quarry",
      version: "v0.2.0",
      icon: "qu",
      description: "declares a pane and serves nothing",
      coverage: "partial",
      coverageNote: "",
      services: [],
      hosted: false,
      commands: 2,
      paneUrl: "http://127.0.0.1:7399/",
    },
  ];

  // Two fixtures, because one page cannot show both a detail pane and a
  // program's own pane - the pane draws whatever is selected. ?fixture=1 is the
  // rail and the detail list; ?pane=1 is the two pane states the gate could not
  // otherwise reach. Both are audited, so neither costs the other its coverage.
  const params = new URLSearchParams(location.search);
  const paneFixture = params.get("pane") === "1";
  const fixture = params.get("fixture") === "1" || paneFixture;

  let programs: Program[] = $state(fixture ? FIXTURE : []);
  let health: Health = $state(
    fixture
      ? {
          connected: true,
          socket: "/run/user/1000/rig/rigd.sock",
          detail: "",
          programs: FIXTURE.length,
        }
      : { connected: false, socket: "", detail: "", programs: 0 },
  );
  let build: Record<string, string> | null = $state(null);
  let selected: string | null = $state(
    paneFixture ? "quarry" : fixture ? "graft" : null,
  );
  let lastRead = $state("");

  // Held rather than only applied, because the pane has to push the token set
  // into a program's own page and a mode change has to reach it too. One
  // source: this is the same mode applyTheme is called with.
  let mode: Mode = $state(preferredMode());

  // M1a step 5. Opened with "," and closed with Esc, both from the window
  // handler below, so the panel is reachable without a pointer - which is the
  // only way it is reachable at all while the context bar has no room for
  // another control.
  let settingsOpen = $state(false);
  // Bumped whenever the live theme changes, so Pane re-pushes the token set to
  // every program. Without it the window changes colour and the panes keep the
  // set they were handed, which is the one bug a shell-wide theme must not
  // have.
  let themeGen = $state(0);

  let current = $derived(programs.find((p) => p.id === selected) ?? null);

  async function refresh() {
    // Health is the one call that must not throw its way out of here: if it
    // does, every later line is skipped and the shell sits in its initial
    // state, which looks exactly like "still loading" and says nothing.
    let h: Health;
    try {
      h = await RigService.Health();
    } catch (e) {
      programs = [];
      selected = null;
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
      // live, which is the failure mode a status strip cannot rescue.
      programs = [];
      selected = null;
      lastRead = stamp();
      return;
    }

    try {
      programs = (await RigService.Programs()) ?? [];
    } catch (e) {
      programs = [];
      health = { ...h, connected: false, detail: String(e) };
    }

    // The rail never re-orders under your hand (section 11), and a selection
    // survives a refresh unless the program it names has gone.
    if (selected && !programs.some((p) => p.id === selected)) selected = null;
    if (!selected && programs.length > 0) selected = programs[0].id;
    lastRead = stamp();
  }

  function stamp(): string {
    return new Date().toLocaleTimeString([], { hour12: false });
  }

  // A poll, and it is a stopgap with a date on it rather than a design. Section
  // 5h's event stream lands at M4 with config.changed as its first kind, and
  // the registry changing is exactly the second kind; until then the rail would
  // otherwise not notice a program registering. It stops while the window is
  // hidden, because section 17 budgets the daemon's wakeups and a background
  // window has no business costing any.
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
    if (e.key === "Escape" && settingsOpen) {
      settingsOpen = false;
      return;
    }
    // A single-letter shortcut must not fire while the panel has the keyboard:
    // "r" inside a settings field would refresh the rail mid-edit.
    if (settingsOpen) return;
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

<div class="shell">
  <Rail {programs} {selected} onselect={(id) => (selected = id)} />

  <div class="main">
    <ContextBar
      program={current}
      programCount={programs.length}
      connected={health.connected}
    />
    <Pane
      program={current}
      {programs}
      detail={health.detail}
      connected={health.connected}
      {mode}
      {themeGen}
      {paneFixture}
    />
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
