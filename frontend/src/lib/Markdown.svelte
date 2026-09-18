<!-- A record's body, rendered as the markdown it is.

     ⛔ BORIS, 2026-09-18: "Fix the uglyness and fix the rig functionality."
     He had just been shown a requirement whose body is a markdown TABLE, drawn
     under `white-space: pre-wrap` as a wall of pipes with each cell wrapped
     across four lines. 348 of the 1,036 bodies in his store hold a table, 998
     hold bold and 886 hold a code span. `pre-wrap` showed every one of those
     as its own punctuation.

     ⛔ IT PARSES WITH `marked` AND RENDERS WITH SVELTE, AND NOTHING IS EVER
     INJECTED. This is the whole reason a library is here at all, and §38 wants
     the search described, so:

       snarkdown       1 KB, but emits an HTML STRING and has no tables.
                       Fails 33% of the bodies and forces {@html}.
       markdown-it     tokenises, but ~3x the footprint and a plugin model
                       nothing here needs.
       micromark       emits low-level EVENTS, not a tree; a usable tree needs
                       mdast-util-from-markdown on top - two packages.
       marked          a real token TREE from `marked.lexer()`, GFM tables
                       built in, one package, no transitive dependencies.

     ⛔ AND THE LEXER IS THE POINT RATHER THAN THE PARSER. `marked.parse()`
     returns HTML, which would have to be injected with {@html} and sanitised,
     and a sanitiser is a thing that has to be right forever. `marked.lexer()`
     returns data. This file walks it and writes Svelte markup, so there is no
     string that could become an element - which is section 38's "no
     injection-shaped string building", satisfied by construction rather than
     by a check.

     ⛔ LEXING IS LAZY BECAUSE A ROW OWNS IT. Spec holds 756 KB of prose. Only
     an OPEN row mounts this component, so a page of 387 closed rows parses
     nothing. -->
<script lang="ts">
  import { marked } from "marked";
  import Inline from "./Inline.svelte";

  interface Props {
    /** The record's body, verbatim from the store. */
    src?: string;
    /** Block tokens, when a parent is already walking a tree (a list item,
        a blockquote). Exactly one of src and tokens is given. */
    tokens?: any[] | null;
  }

  let { src = "", tokens = null }: Props = $props();

  /* gfm for the tables, and `breaks` OFF deliberately: these bodies are hard
     wrapped at 72-ish columns like every document in this project, so treating
     a newline as a line break would render every paragraph as a ragged column
     exactly as narrow as whoever typed it. */
  let blocks = $derived(
    tokens ?? (src ? marked.lexer(src, { gfm: true, breaks: false }) : []),
  );
</script>

{#each blocks as b}
  {#if b.type === "paragraph"}
    <p><Inline tokens={b.tokens ?? []} /></p>
  {:else if b.type === "heading"}
    <!-- ⛔ h4 AND BELOW, WHATEVER THE DOCUMENT SAID. A record's body sits under
         the row's own title, which is already a heading - promoting a body's
         `##` to an h2 would put a bigger heading inside a smaller one and break
         the document outline the rest of this window keeps. -->
    <p class="h" data-depth={Math.min(b.depth ?? 3, 4)}>
      <Inline tokens={b.tokens ?? []} />
    </p>
  {:else if b.type === "table"}
    <!-- ⛔ A REAL TABLE, AND IT IS THE WHOLE REASON THIS FILE EXISTS. 348
         bodies hold one. Under pre-wrap a 120-character row of pipes wrapped
         to four lines per cell and the columns stopped existing. -->
    <div class="tw">
      <!-- ⛔ THE CELL CAP DEPENDS ON HOW MANY COLUMNS THERE ARE, AND A FIXED
           ONE IS WRONG AT BOTH ENDS. 44ch stops one column eating a wide
           table; on a ONE-column table it squeezed 40-character lines into a
           1,150px panel with 800px empty beside them, which is the same
           ugliness in the other direction. CSS cannot count columns, so the
           count comes from the token. -->
      <table style:--cell-max={b.header.length > 1 ? "44ch" : "68ch"}>
        <thead>
          <tr>
            {#each b.header as h, i}
              <th style:text-align={b.align?.[i] || "start"}>
                <Inline tokens={h.tokens ?? []} />
              </th>
            {/each}
          </tr>
        </thead>
        <tbody>
          {#each b.rows as row}
            <tr>
              {#each row as cell, i}
                <td style:text-align={b.align?.[i] || "start"}>
                  <Inline tokens={cell.tokens ?? []} />
                </td>
              {/each}
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {:else if b.type === "list"}
    {#if b.ordered}
      <ol start={b.start || 1}>
        {#each b.items as it}
          <li><svelte:self tokens={it.tokens ?? []} /></li>
        {/each}
      </ol>
    {:else}
      <ul>
        {#each b.items as it}
          <li><svelte:self tokens={it.tokens ?? []} /></li>
        {/each}
      </ul>
    {/if}
  {:else if b.type === "blockquote"}
    <blockquote><svelte:self tokens={b.tokens ?? []} /></blockquote>
  {:else if b.type === "code"}
    <pre class="code">{b.text}</pre>
  {:else if b.type === "hr"}
    <hr />
  {:else if b.type === "space"}
    <!-- nothing: the gap is the CSS's job, not an empty element's -->
  {:else if b.type === "html"}
    <!-- Same rule as inline: the characters, never the markup. -->
    <pre class="code raw">{b.raw}</pre>
  {:else if b.tokens}
    <p><Inline tokens={b.tokens} /></p>
  {:else if b.text}
    <p>{b.text}</p>
  {/if}
{/each}

<style>
  /* ⛔ THE BODY IS PROSE NOW AND READS IN THE PROSE FACE. It was `--mono` at
     `--fs--1` under pre-wrap, because what was on screen was source rather
     than text. 65ch is the measure the rest of this window uses. */
  p {
    margin: 0 0 0.75em;
    line-height: 1.6;
    max-width: 68ch;
    overflow-wrap: anywhere;
  }

  p:last-child {
    margin-bottom: 0;
  }

  .h {
    font-family: var(--disp);
    font-weight: 650;
    letter-spacing: var(--tight-disp);
    color: var(--fg);
    margin-block: 1em 0.45em;
  }

  .h[data-depth="1"],
  .h[data-depth="2"],
  .h[data-depth="3"] {
    font-size: var(--fs-1);
  }

  .h[data-depth="4"] {
    font-size: var(--fs-0);
  }

  /* ── tables ───────────────────────────────────────────────────────────── */

  /* ⛔ THE SCROLL BELONGS TO THE TABLE AND NOWHERE ELSE. A wide table used to
     push a horizontal bar onto the whole panel, which is the defect Boris
     photographed on 2026-09-17 with an arrow on it - "these scrolls are
     unnecessary". Here the bar is on the table's own box, so it appears only
     when a table is genuinely wider than the column, and the page never moves
     sideways. overflow-y is stated for the reason ProjectCaseGui states it:
     `auto` on one axis computes the other to `auto` too, and a one-pixel
     phantom draws a second set of arrows. */
  .tw {
    overflow-x: auto;
    overflow-y: hidden;
    margin: 0 0 0.85em;
    max-width: 100%;
  }

  table {
    border-collapse: collapse;
    font-size: var(--fs--1);
    line-height: 1.45;
  }

  th,
  td {
    border: 1px solid var(--border);
    padding: 0.3rem 0.6rem;
    vertical-align: top;
    /* A cell wraps rather than forcing the table wider, but a long unbroken
       id in a cell must still break or it sets the column's width alone. */
    overflow-wrap: anywhere;
    max-width: var(--cell-max, 44ch);
  }

  th {
    background: var(--bg-2);
    font-weight: 650;
    color: var(--fg);
    white-space: nowrap;
  }

  /* Zebra is deliberately absent: the rule grid already separates the rows,
     and a tinted band behind body text is the B71 defect - the gate reads the
     token underneath and the eye reads the blend. */

  /* ── the rest ─────────────────────────────────────────────────────────── */

  ul,
  ol {
    margin: 0 0 0.75em;
    padding-inline-start: 1.5em;
    max-width: 68ch;
    line-height: 1.6;
  }

  li {
    margin-bottom: 0.2em;
  }

  /* A rule, not a tinted ground - ItemRow's note carries the reason. */
  blockquote {
    margin: 0 0 0.8em;
    padding-inline-start: 0.9rem;
    border-inline-start: 2px solid var(--border-2);
    color: var(--fg-dim);
    max-width: 68ch;
  }

  .code {
    margin: 0 0 0.8em;
    font-family: var(--mono);
    font-size: var(--fs--1);
    line-height: 1.5;
    color: var(--fg);
    background: var(--bg-2);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.55rem 0.7rem;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    max-width: 92ch;
  }

  .code.raw {
    color: var(--fg-dim);
  }

  hr {
    border: 0;
    border-top: 1px solid var(--border);
    margin: 1.1em 0;
    max-width: 68ch;
  }
</style>
