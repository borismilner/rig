<!-- The pane, at its first tier only.
     Section 11 has three: generated (rig renders from a declared schema), kit
     (the program serves its own HTML from rig's elements) and embedded (its own
     HTML, its own components, the token set and nothing more). This is the
     bottom of the generated tier and nothing else: what the program declared,
     drawn by rig, with no frontend code from the program at all. Steps 3 and 4
     bring the kit tier, and they are what decide the element list. -->
<script lang="ts">
  import type { Program } from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";

  interface Props {
    program: Program | null;
    programs: Program[];
    detail: string;
    connected: boolean;
  }

  let { program, programs, detail, connected }: Props = $props();
</script>

<main class="pane">
  {#if !connected}
    <h2>rig is not answering</h2>
    <p class="lead">
      The window is up and the daemon is not, which is section 5g's fourth
      state. Nothing here is stale data: the rail is empty because the registry
      could not be read.
    </p>
    <pre class="detail">{detail}</pre>
    <p class="lead">
      Start it with <code>rigd</code>, then press <kbd>r</kbd>.
    </p>
  {:else if programs.length === 0}
    <h2>no programs are registered</h2>
    <p class="lead">
      rig is answering and its registry is empty. A program appears here the
      moment it registers, which is registration over the real wire rather than
      anything drawn in advance.
    </p>
  {:else if !program}
    <h2>{programs.length} registered</h2>
    <p class="lead">Pick one from the rail.</p>
  {:else}
    <dl>
      <dt>id</dt>
      <dd class="mono">{program.id}</dd>

      <dt>name</dt>
      <dd>{program.name || "-"}</dd>

      <dt>version</dt>
      <dd class="mono">{program.version || "-"}</dd>

      <dt>description</dt>
      <dd>{program.description || "-"}</dd>

      <dt>coverage</dt>
      <dd>
        {program.coverage}{#if program.coverageNote}, {program.coverageNote}{/if}
      </dd>

      <dt>commands</dt>
      <dd class="mono">{program.commands}</dd>

      <dt>services</dt>
      <dd class="mono">
        {program.services && program.services.length
          ? program.services.join(" ")
          : "none declared"}
      </dd>

      <dt>hosted</dt>
      <dd class="mono">{program.hosted}</dd>
    </dl>

    <p class="lead">
      Everything above came over the socket from rig's registry. No pane content
      of the program's own exists yet: that is the kit tier, and step 3 is the
      first program to serve any.
    </p>
  {/if}
</main>

<style>
  h2 {
    margin: 0 0 0.5rem;
    font-family: var(--disp);
    font-size: var(--fs-1);
    font-weight: 600;
    letter-spacing: var(--tight-disp);
  }

  .lead {
    margin: 0 0 0.9rem;
    color: var(--fg-dim);
    max-width: 68ch;
  }

  .detail {
    margin: 0 0 0.9rem;
    padding: 0.6rem 0.75rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-2);
    color: var(--fg-dim);
    font-family: var(--mono);
    font-size: var(--fs--1);
    white-space: pre-wrap;
    overflow-x: auto;
  }

  dl {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 0.35rem 1.1rem;
    margin: 0 0 1.1rem;
    align-items: baseline;
  }

  dt {
    color: var(--fg-faint);
    font-size: var(--fs--1);
    font-family: var(--mono);
  }

  dd {
    margin: 0;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .mono {
    font-family: var(--mono);
    font-size: var(--fs--1);
  }

  code {
    font-family: var(--mono);
    font-size: var(--fs--1);
    color: var(--fg);
  }
</style>
