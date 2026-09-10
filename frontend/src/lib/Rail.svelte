<!-- The rail: every registered program, in the order rig returned them.
     Section 11 is explicit that it never re-orders under your hand, so the
     order is the registry's and nothing here sorts it. -->
<script lang="ts">
  import type { Program } from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";

  interface Props {
    programs: Program[];
    selected: string | null;
    onselect: (id: string) => void;
  }

  let { programs, selected, onselect }: Props = $props();

  // Measured, not guessed. The notch slides to the selected marker, and the
  // rail's top offset depends on the brand's height and the rail's padding -
  // both token-driven, so a hard-coded 36px goes wrong the first time the
  // density knob moves.
  const MARK_H = 44;
  let marks: HTMLButtonElement[] = $state([]);
  let railTop = $derived(marks[0]?.offsetTop ?? 0);
  let index = $derived(programs.findIndex((p) => p.id === selected));

  // Two letters of monospace, which is what the design system draws. A program
  // that declared no icon gets its first two characters rather than a blank
  // square; section 5e does not make icon mandatory.
  function glyph(p: Program): string {
    return (p.icon || p.id.slice(0, 2)).slice(0, 2);
  }

  // On the buttons rather than on the <nav>: a keydown listener on a
  // non-interactive element is an a11y warning and, worse, a keyboard path that
  // only works when focus happens to be inside it. A marker has focus or it
  // does not.
  function onkeydown(e: KeyboardEvent) {
    if (programs.length === 0) return;
    const step = e.key === "ArrowDown" ? 1 : e.key === "ArrowUp" ? -1 : 0;
    if (step === 0) return;
    e.preventDefault();
    const from = index < 0 ? (step > 0 ? -1 : 0) : index;
    const next = (from + step + programs.length) % programs.length;
    onselect(programs[next].id);
    marks[next]?.focus();
  }
</script>

<nav class="rail" aria-label="Registered programs">
  <div class="brand" title="rig">ri</div>

  {#each programs as p, i (p.id)}
    <button
      class="mark"
      bind:this={marks[i]}
      aria-current={p.id === selected}
      title={`${p.name || p.id} ${p.version}`}
      onclick={() => onselect(p.id)}
      {onkeydown}
    >
      <span class="ico">{glyph(p)}</span>
    </button>
  {/each}

  {#if index >= 0}
    <span class="notch" style={`translate: 0 ${railTop + index * MARK_H}px`}
    ></span>
  {/if}
</nav>
