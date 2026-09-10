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
  import { applyTheme } from "./lib/theme";
  import Rail from "./lib/Rail.svelte";
  import ContextBar from "./lib/ContextBar.svelte";
  import Pane from "./lib/Pane.svelte";
  import StatusStrip from "./lib/StatusStrip.svelte";

  let programs: Program[] = $state([]);
  let health: Health = $state({
    connected: false,
    socket: "",
    detail: "",
    programs: 0,
  });
  let build: Record<string, string> | null = $state(null);
  let selected: string | null = $state(null);
  let lastRead = $state("");

  let current = $derived(programs.find((p) => p.id === selected) ?? null);

  async function refresh() {
    const h = await RigService.Health();
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
    if (e.key === "r" && !e.ctrlKey && !e.metaKey && !e.altKey) void refresh();
  }

  onMount(() => {
    applyTheme("dark");
    void refresh();
    RigService.Build().then((b) => (build = b));
    startPolling();
    document.addEventListener("visibilitychange", onvisibility);
    return () => {
      stopPolling();
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
