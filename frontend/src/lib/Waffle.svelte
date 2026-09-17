<!-- One cell per work item, coloured by what has been recorded against it.

     ⛔ WHY A CELL PER ITEM AND NOT A BAR OR A PERCENTAGE. Today every item in
     rig's own project is unstepped, so a stacked bar would draw a
     zero-width segment - which reads as a rendering fault - and a percentage
     would read as a judgement. A field of identical hollow cells reads as
     exactly what it is: a plan with nothing recorded against it. Section 11
     requirement 15 asks for a feel of the product, and this is the product's
     current feel.

     AND IT SURVIVES THE FIX THAT IS IN FLIGHT. Another seat is repairing the
     write path so terminal dispositions stop flattening to `active`. The
     moment states vary, the same grid shows the variation with no change
     here - which is the test a view built around today's uniformity would
     fail.

     UNRECORDED IS HOLLOW, NOT COLOURED. An empty cell is what nothing looks
     like. Giving "not stepped" a hue of its own would make an absence of data
     look like a state somebody chose. -->
<script lang="ts">
  import type { Item } from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";
  import { stepTone } from "./brief";

  interface Props {
    items: Item[];
    /** Smaller cells for the dashboard's summary; the tab uses the default. */
    dense?: boolean;
    /** What a screen reader is told instead of 59 anonymous cells. */
    summary: string;
  }

  let { items, dense = false, summary }: Props = $props();

  // A hard ceiling, because the grid is per-item and a project with ten
  // thousand items would paint ten thousand nodes. The overflow is STATED
  // rather than silently dropped: a truncated figure that does not say it is
  // truncated is a chart that lies.
  const CAP = 600;
  let shown = $derived(items.slice(0, CAP));
  let hidden = $derived(Math.max(0, items.length - CAP));
</script>

<figure class="waffle" class:dense role="img" aria-label={summary}>
  <div class="cells">
    {#each shown as it (it.id)}
      <span
        class="cell"
        data-tone={stepTone(it.state)}
        title={`${it.id} - ${it.state}\n${it.title}`}
      ></span>
    {/each}
  </div>
  {#if hidden}
    <figcaption>
      {hidden} further item{hidden === 1 ? "" : "s"} are not drawn. The counts beside
      this grid include them.
    </figcaption>
  {/if}
</figure>

<style>
  .waffle {
    margin: 0;
  }

  .cells {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  /* The boundary carries the meaning for an unrecorded item, so it is the one
     thing here that has to clear 1.4.11's 3:1. --border-2 is the engine's
     4.5:1 neutral against every ground, which leaves headroom on --panel,
     the ground this actually lands on. --border was measured at 3.03:1 on
     --panel in the token table and is too close to the line for a 14px mark. */
  .cell {
    width: 15px;
    height: 15px;
    border-radius: 3px;
    border: 1.5px solid var(--border-2);
    background: transparent;
    flex: none;
  }

  .dense .cell {
    width: 9px;
    height: 9px;
    border-radius: 2px;
    border-width: 1px;
  }

  /* A recorded state fills the cell AND keeps a boundary in the same hue, so
     the mark still has an edge where the fill is close to the ground. */
  .cell[data-tone="good"] {
    background: var(--sem-good);
    border-color: var(--sem-good);
  }
  .cell[data-tone="progress"] {
    background: var(--sem-progress);
    border-color: var(--sem-progress);
  }
  .cell[data-tone="bad"] {
    background: var(--sem-bad);
    border-color: var(--sem-bad);
  }
  .cell[data-tone="warn"] {
    background: var(--sem-warn);
    border-color: var(--sem-warn);
  }

  figcaption {
    margin-top: 0.5rem;
    font-size: var(--fs--1);
    color: var(--fg-dim);
  }
</style>
