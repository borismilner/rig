<!-- The inline half of a record body: emphasis, code spans, the rest.

     ⛔ IT TAKES TOKENS, NOT A STRING, AND IT EMITS MARKUP, NOT HTML. Nothing
     in this file writes a string into the document - see Markdown.svelte's
     header for why that is the whole design and not a precaution. -->
<script lang="ts">
  import type { Tokens } from "marked";

  interface Props {
    /** marked's inline tokens. Typed loosely because the union is large and
        every arm this file does not name falls through to its raw text. */
    tokens: any[];
  }

  let { tokens }: Props = $props();
</script>

{#each tokens as t}
  {#if t.type === "strong"}
    <strong><svelte:self tokens={t.tokens ?? []} /></strong>
  {:else if t.type === "em"}
    <em><svelte:self tokens={t.tokens ?? []} /></em>
  {:else if t.type === "del"}
    <del><svelte:self tokens={t.tokens ?? []} /></del>
  {:else if t.type === "codespan"}
    <code>{t.text}</code>
  {:else if t.type === "br"}
    <br />
  {:else if t.type === "link"}
    <!-- ⛔ NOT AN <a>, AND THAT IS HONESTY RATHER THAN CAUTION. This window
         has no navigation and no browser hand-off, so an anchor here would
         look like a thing you can follow and do nothing when you click it.
         The text reads as text and the target is shown beside it, which is
         what a reader needs in order to go and look. Measured: 0 of the 1,036
         bodies in his store contain a link at all, so this arm exists to be
         correct rather than because it is on screen. -->
    <svelte:self tokens={t.tokens ?? []} /><span class="href">{t.href}</span>
  {:else if t.type === "html"}
    <!-- ⛔ RAW HTML IS SHOWN AS THE CHARACTERS IT IS. 56 of his bodies contain
         a `<`, and every one of them MENTIONS markup rather than intending it -
         `<script>` inside a sentence about injection, `1 < 2` in a comparison.
         Rendering it would be both a lie about the document and the one
         injection this file otherwise cannot have. -->
    <span class="raw">{t.raw}</span>
  {:else if t.tokens}
    <svelte:self tokens={t.tokens} />
  {:else}{t.text ?? t.raw ?? ""}{/if}
{/each}

<style>
  strong {
    font-weight: 650;
    color: var(--fg);
  }

  em {
    font-style: italic;
  }

  del {
    text-decoration: line-through;
    color: var(--fg-dim);
  }

  /* A code span is a different KIND of thing, not a louder one: the mono face
     and a hairline ground say so without spending a hue, which this window
     reserves for status. */
  code {
    font-family: var(--mono);
    font-size: 0.92em;
    background: var(--bg-2);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 0.02em 0.32em;
    overflow-wrap: anywhere;
  }

  .href {
    font-family: var(--mono);
    font-size: 0.85em;
    color: var(--fg-dim);
    margin-inline-start: 0.35em;
    overflow-wrap: anywhere;
  }

  .raw {
    font-family: var(--mono);
    color: var(--fg-dim);
    overflow-wrap: anywhere;
  }
</style>
