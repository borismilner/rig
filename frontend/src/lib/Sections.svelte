<!-- What rig can and cannot work out about a project, all eleven sections.

     ⛔ THIS COMPONENT EXISTS BECAUSE OF ONE SENTENCE ON THE WIRE: "A caller
     that renders a section without reading its status is the failure this
     field exists to prevent." Six of the eleven sections are unbuilt in the
     daemon today. A UI that drew an empty Drift panel would be telling Boris
     this project has no drift, when the truth is that rig cannot yet know -
     and those two are not the same sentence.

     SO AN UNBUILT SECTION IS A DIFFERENT KIND OF OBJECT ON SCREEN, not a
     paler version of a built one: it is hatched, it carries its own reason in
     full, and it has no count beside it at all. Nothing here can be mistaken
     for a result. The hatch is the load-bearing part rather than the colour,
     because it survives greyscale, a colour-blind reader and a photograph of
     the screen. -->
<script lang="ts">
  import type { Brief } from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";
  import { sectionViews, sectionTally } from "./brief";

  interface Props {
    brief: Brief | null;
    /** What each computed section counted, so a built-and-empty section can
        say "0" where an unbuilt one says nothing at all. */
    counts: Record<string, number | null>;
  }

  let { brief, counts }: Props = $props();

  let views = $derived(sectionViews(brief));
  let t = $derived(sectionTally(brief));
</script>

<div class="sections">
  <p class="scount">
    <strong>{t.computed} of {t.total}</strong> sections are built. rig answers
    the other {t.dark + t.absent} with a reason instead of a result.
  </p>

  <ul>
    {#each views as v (v.name)}
      <li class="sec" data-kind={v.kind}>
        <span class="mark" aria-hidden="true"></span>
        <span class="slabel">{v.label}</span>
        {#if v.kind === "computed"}
          <!-- A built section this window carries no count for says the word
               rather than a dash. "-" was on screen for Features and reads as
               an empty result, which is the one thing this component exists
               to prevent. -->
          {#if counts[v.name] === null || counts[v.name] === undefined}
            <span class="sstate">computed</span>
          {:else}
            <span class="scount-n">{counts[v.name]}</span>
          {/if}
        {:else}
          <span class="sstate"
            >{v.kind === "dark" ? v.state : "not answered"}</span
          >
        {/if}
        {#if v.kind === "dark"}
          <p class="reason">{v.reason}</p>
        {:else if v.kind === "absent"}
          <p class="reason">
            The daemon answered this brief without mentioning this section,
            which usually means it is older than this window. What it would have
            said is unknown.
          </p>
        {/if}
      </li>
    {/each}
  </ul>
</div>

<style>
  .sections {
    min-width: 0;
  }

  .scount {
    margin: 0 0 0.75rem;
    color: var(--fg-dim);
    font-size: var(--fs--1);
  }

  .scount strong {
    color: var(--fg);
  }

  ul {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    gap: 2px;
  }

  .sec {
    display: grid;
    grid-template-columns: 12px 1fr auto;
    align-items: baseline;
    column-gap: 0.6rem;
    padding: 0.42rem 0.6rem;
    border-radius: var(--radius);
    border: 1px solid transparent;
  }

  .sec[data-kind="computed"] {
    background: var(--bg-2);
    border-color: var(--border);
  }

  /* The hatch, and it is the whole signal. A dark section is drawn as
     unavailable ground rather than as an empty container: there is no field
     here to be full or empty. 45-degree bands at 6px are coarse enough to
     survive the window's own scaling and fine enough not to fight the text
     sitting on them. */
  /* ⛔ 30%, AND THE NUMBER IS MEASURED RATHER THAN CHOSEN. It was 55%, which
     puts the reason text at 3.73:1 dark and 3.71:1 light against a 4.5 floor -
     A FAILURE THE CONTRAST GATE STRUCTURALLY CANNOT SEE, because every pass
     reads backgroundColor and a stripe is a background-IMAGE. Measured by
     compositing --border over --bg-2 by hand in the same headless Chrome:

        stripe   --fg-dim dark / light
          55%        3.73 / 3.71     FAILS
          40%        4.54 / 4.46     light fails
          30%        5.16 / 5.01     passes both
          20%        5.86 / 5.63

     Do not push it back up without redoing that arithmetic; a green gate will
     not stop you. */
  .sec[data-kind="dark"],
  .sec[data-kind="absent"] {
    border-color: var(--border);
    border-style: dashed;
    background-color: var(--bg-2);
    background-image: repeating-linear-gradient(
      -45deg,
      transparent 0 5px,
      color-mix(in srgb, var(--border) 30%, transparent) 5px 6px
    );
  }

  .mark {
    width: 10px;
    height: 10px;
    border-radius: 50%;
    align-self: center;
  }

  /* Healthy is the absence of colour (section 11), so a built section is a
     hollow tick-mark's worth of border and nothing more. */
  .sec[data-kind="computed"] .mark {
    border: 1.5px solid var(--fg-dim);
  }

  /* Amber is the warn role, and an unbuilt section IS something the reader has
     to act on: every number near it is missing rather than zero. */
  .sec[data-kind="dark"] .mark,
  .sec[data-kind="absent"] .mark {
    background: var(--sem-warn);
  }

  .slabel {
    color: var(--fg);
  }

  .scount-n {
    font-family: var(--mono);
    color: var(--fg);
    font-variant-numeric: tabular-nums;
  }

  .sstate {
    font-size: var(--fs--1);
    color: var(--fg-dim);
    font-family: var(--mono);
  }

  .reason {
    grid-column: 2 / -1;
    margin: 0.3rem 0 0;
    font-size: var(--fs--1);
    color: var(--fg-dim);
    max-width: 68ch;
  }
</style>
