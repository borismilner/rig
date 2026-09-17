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

  /* ⛔ A RECTANGLE IN DECADES, NOT A RAGGED BAND, AND THE REASON IS THAT A
     RAGGED BAND IS NOT DATA.

     It was `flex-wrap` at whatever width the box happened to be, so the run
     re-flowed with the window, no two cells lined up vertically, and the
     figure read as a TEXTURE - something to glance past rather than count.
     `dataviz`'s rule is that a mark has to encode something; a wrapped row of
     identical squares encodes only "there are some".

     Twenty columns, fixed, with the tenth track widened so a gutter falls
     halfway across every row. The grid is then a shape whose area is the
     plan and whose rows can be counted in tens without reading a number.
     59 items is two full rows and nine - visible at a glance, and visibly
     nine short of the third, which is the sort of thing a count beside the
     figure can only assert.

     TWENTY COLUMNS IS FIXED AND NOT RESPONSIVE ON PURPOSE. A grid that
     re-columns on resize gives two different pictures of the same data and
     destroys the only thing this mark is for.

     ⛔ THE COLUMN COUNT IS FIXED; THE CELL SIZE IS NOT, AND THAT DISTINCTION
     IS A DEFECT FOUND BY LOOKING RATHER THAN BY MEASURING. Fixed 15px tracks
     made the figure 387px wide, which fits at 1440 and CLIPS ITS LAST COLUMN
     at 950 - the dashboard's two-column grid does not collapse until 900, so
     between 900 and about 1010 the record panel is narrower than the figure
     in it. Every contrast ratio passed; the picture was cut off. Screenshot
     at 950 before changing any of the numbers below.

     So the tracks are `fr` under a max-width equal to the natural size: at
     full width 1fr resolves to exactly --cell, and under pressure every cell
     shrinks together and stays square on `aspect-ratio`. The decade gutter is
     the tenth track at 1.47fr with its cell at 68% of it, so the gap scales
     with everything else instead of being the one fixed thing left. */
  .waffle {
    --cell: 15px;
    --gap: 4px;
  }

  .waffle.dense {
    --cell: 9px;
  }

  .cells {
    display: grid;
    grid-template-columns: repeat(9, 1fr) 1.47fr repeat(10, 1fr);
    gap: var(--gap);
    /* 20 cells, the gutter's 0.47 of one more, and 20 gaps. */
    max-width: calc(20.47 * var(--cell) + 20 * var(--gap));
  }

  /* The boundary carries the meaning for an unrecorded item, so it is the one
     thing here that has to clear 1.4.11's 3:1. --border-2 is the engine's
     4.5:1 neutral against every ground, which leaves headroom on --panel,
     the ground this actually lands on. --border was measured at 3.03:1 on
     --panel in the token table and is too close to the line for a 14px mark. */
  .cell {
    width: 100%;
    aspect-ratio: 1;
    /* 2px, not 3: at 15px a 3px radius rounds the mark toward a control and
       the legend's own swatch already had to stop looking like a checkbox. */
    border-radius: 2px;
    border: 1.5px solid var(--border-2);
    background: transparent;
  }

  .dense .cell {
    border-width: 1px;
  }

  /* The tenth cell of every row sits in the wide track, left-aligned, so the
     remaining 32% of that track is the decade gutter. */
  .cell:nth-child(20n + 10) {
    width: 68%;
    justify-self: start;
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
