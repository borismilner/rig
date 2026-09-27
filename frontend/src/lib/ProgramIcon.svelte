<!-- A program's icon from the window's own set (./icons.ts). Stroked, so it
     takes currentColor like the two letters it replaces. -->
<script lang="ts">
  import type { IconNode } from "./icons";

  let { node }: { node: IconNode } = $props();
</script>

<svg
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="2"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
  focusable="false"
>
  {#each node as [tag, attrs], i (i)}
    <svelte:element this={tag} {...attrs} />
  {/each}
</svg>
