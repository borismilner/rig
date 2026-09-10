<!-- Step 1 of M1a and nothing more: the window exists, it is its own process,
     and it is painted in the design system's own --bg rather than the
     scaffolder's navy. The rail, context bar, pane and status strip are step 2
     and are deliberately not faked here - a screenshot of this must not be
     mistakable for the shell (section 24). -->
<script lang="ts">
  const steps = [
    { n: 1, what: "The window, scaffolded and building", state: "here" },
    { n: 2, what: "The shell over the live wire", state: "next" },
    { n: 3, what: "One fake application", state: "next" },
    { n: 4, what: "The second, on a different shape", state: "next" },
    { n: 5, what: "The theme changed from the settings UI", state: "next" },
  ];
</script>

<main>
  <h1>rig</h1>
  <p class="sub">
    The window is up. It is a separate process, and it draws nothing yet.
  </p>

  <ol>
    {#each steps as step (step.n)}
      <li class:here={step.state === "here"}>
        <span class="n">{step.n}</span>
        <span class="what">{step.what}</span>
      </li>
    {/each}
  </ol>
</main>

<style>
  main {
    display: flex;
    flex-direction: column;
    gap: 1.5rem;
    padding: 3rem;
    max-width: 40rem;
  }

  h1 {
    font-size: 2rem;
    font-weight: 500;
    letter-spacing: -0.011em;
    margin: 0;
  }

  .sub {
    margin: 0;
    opacity: 0.72;
  }

  ol {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  /* 0.58, not 0.5, and the numerals do not dim again on top of it. Measured
     with design/theme.js's own contrast(): 0.5 puts the dimmed rows at 4.37:1
     against --bg and a second 0.6 on the numeral compounded to 0.3, which is
     2.39:1 - a fail twice over. Nested opacity is the bug class; one dim level
     that clears 4.5:1 is the fix. Section 6's solved --fg-dim replaces this
     the moment the theme is live. */
  li {
    display: flex;
    gap: 0.75rem;
    align-items: baseline;
    opacity: 0.58;
  }

  li.here {
    opacity: 1;
  }

  .n {
    font-variant-numeric: tabular-nums;
  }
</style>
