<!-- The context bar: where you are, and what rig knows about it that a person
     would otherwise have to run a command to learn. Section 11 wants "shelf
     declared 3 of its 20 commands" visible here rather than inferred, which
     is what coverageNote carries.

     It now has three states rather than two, because the window does: the
     dashboard is a destination, an internal GUI is rig's own surface, and
     everything else is a registered program's. -->
<script lang="ts">
  import type { Program } from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";
  import type { InternalGui } from "./guis";

  interface Props {
    /** The dashboard is showing. */
    atHome: boolean;
    /** An internal GUI is showing. Mutually exclusive with `program`. */
    gui: InternalGui | null;
    program: Program | null;
    programCount: number;
    connected: boolean;
  }

  let { atHome, gui, program, programCount, connected }: Props = $props();
</script>

<header class="ctxbar">
  {#if atHome}
    <span class="name">Dashboard</span>
    <span class="note">
      {!connected
        ? "detached: rig is not answering"
        : programCount === 0
          ? "rig is answering and no programs are registered"
          : `${programCount} program${programCount === 1 ? "" : "s"} registered`}
    </span>
    <span class="right">
      <kbd>&uarr;</kbd><kbd>&darr;</kbd> rail
      <kbd>r</kbd> refresh
      <kbd>,</kbd> settings
    </span>
  {:else if gui}
    <span class="name">{gui.title}</span>
    <span class="note">{gui.note}</span>
    <span class="right">
      <kbd>Esc</kbd> dashboard
      <kbd>&larr;</kbd><kbd>&rarr;</kbd> tabs
      <kbd>,</kbd> settings
    </span>
  {:else if program}
    <!-- The id, not the declared name, and the design page does the same. The
         id is what a person types (`rig <app> <cmd>`) and what is unique; a
         declared name is decoration and three programs can share one. -->
    <span class="name">{program.id}</span>
    <span class="ver">{program.version}</span>
    <span class="note">
      {program.coverageNote ||
        `${program.commands} command${program.commands === 1 ? "" : "s"} declared, coverage ${program.coverage}`}
    </span>
    <span class="right">
      {#if program.name && program.name !== program.id}<span
          >{program.name}</span
        >{/if}
      {#if program.hosted}<span>hosted</span>{/if}
      <kbd>Esc</kbd> dashboard
      <kbd>&uarr;</kbd><kbd>&darr;</kbd> rail
    </span>
  {:else}
    <span class="name">rig</span>
    <!-- Not reaching rig and reaching an empty registry are different
         sentences. Saying "no programs are registered" while the pane says the
         daemon is gone is the shell contradicting itself, which is worse than
         either message alone. -->
    <span class="note">
      {!connected
        ? "detached: rig is not answering"
        : programCount === 0
          ? "no programs are registered"
          : "select a program from the rail"}
    </span>
    <span class="right"><kbd>r</kbd> refresh</span>
  {/if}
</header>
