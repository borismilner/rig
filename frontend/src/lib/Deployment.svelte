<!-- What is actually RUNNING, artefact by artefact, and whether the artefacts
     agree with each other.

     ⛔ BORIS, 2026-09-17: "Should have management panel to do things like
     `make install` in the proper place with ease. And `make install-window`...
     Redeployment should be trivial and automatic." (plan/11, B90.)

     ⛔ THIS IS THE STATE HALF AND IT SHIPS FIRST, WHICH IS A DECISION HE
     DELEGATED RATHER THAN ONE HE STATED. "Trivial and automatic" is a bar on
     the whole act, not a request for two buttons: B89 is the measured case
     where `make install` reported success and the window stayed thirteen hours
     stale, and a panel with two buttons would have produced exactly that,
     because somebody still has to know to press the second one.

     ⛔ IT REPLACED A CARD THAT RESTATED THE PROJECT VIEW. What stood here was
     a waffle, the same verdict sentence the Projects GUI prints, and a link
     reading "Open Projects and cases" - the "strange partial dashboard" he
     named. The rail already lands on that view directly; this page was
     duplicating its destination. -->
<script lang="ts">
  import type { Deployment } from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";

  interface Props {
    deployment: Deployment | null;
  }

  let { deployment }: Props = $props();

  // ⛔ THREE STATES, NOT TWO. "I could not ask" is not "they disagree", and
  // drawing them the same way would be B89's own defect in a new place: a
  // person sent to reinstall a window that is already correct.
  let state = $derived(
    !deployment
      ? "unread"
      : !deployment.reached
        ? "unreached"
        : deployment.agree
          ? "agree"
          : "skew",
  );
</script>

<section class="block deploy" data-state={state}>
  <h2 class="t-sec">What is deployed</h2>

  {#if !deployment}
    <p class="muted">Reading what is running.</p>
  {:else}
    <p class="verdict" data-state={state}>{deployment.verdict}</p>

    <dl class="rows">
      <div class="row">
        <dt>window</dt>
        <dd class="t-num">{deployment.windowVersion || "unstamped"}</dd>
        <dd class="when">{deployment.windowBuilt || ""}</dd>
      </div>
      <div class="row">
        <dt>daemon</dt>
        <dd class="t-num">
          {#if !deployment.reached}
            not answering
          {:else}
            {deployment.daemonVersion || "unstamped"}
          {/if}
        </dd>
        <dd class="when">
          {#if deployment.reached && deployment.epoch}epoch {deployment.epoch}{/if}
        </dd>
      </div>
    </dl>

    <!-- ⛔ THE LIMIT IS STATED ON THE PANEL, NOT ONLY IN THE CODE. An installed
         window has no source tree to read, so "what is built" is a question
         this cannot answer and must not appear to. -->
    <p class="limit">
      These are the artefacts that are running, compared to each other. The
      window cannot see the source tree, so it does not claim to know what is
      built.
    </p>
  {/if}
</section>

<style>
  /* The same card the estate wears, because the two sit side by side in one
     grid row: a bordered list beside a bare column reads as an unfinished
     page rather than as two kinds of thing. */
  .deploy {
    min-width: 0;
    padding: calc(1rem * var(--den)) 1.1rem;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-2);
    display: grid;
    gap: 0.75rem;
    align-content: start;
  }

  .verdict {
    margin: 0;
    font-size: 0.95rem;
    line-height: 1.5;
    padding-inline-start: 0.7rem;
    border-inline-start: 3px solid var(--border);
  }

  /* ⛔ THE SKEW STATE IS THE ONLY LOUD ONE, AND IT IS A BORDER RATHER THAN A
     FILL. A tinted panel behind body text is the hatch defect (B71) in another
     costume: the gate reads the token underneath and the eye reads the blend.
     A border carries no text on it, so nothing has to be measured against it. */
  .verdict[data-state="skew"] {
    border-inline-start-color: var(--h-rust);
    font-weight: 600;
  }
  .verdict[data-state="agree"] {
    border-inline-start-color: var(--h-sage);
  }
  .verdict[data-state="unreached"] {
    border-inline-start-color: var(--h-amber);
  }

  .rows {
    margin: 0;
    display: grid;
    gap: 0.35rem;
  }

  /* ⛔ A VERSION STRING IS NEVER BROKEN ACROSS LINES. `v0.0.0-m0-409-g57c99ca`
     wrapped at its hyphen reads as two shorter versions, and the whole point of
     this panel is that a person can compare two of them at a glance. The
     timestamp is the thing that gives way instead: it is provenance, and it
     stays legible wrapped. */
  .row {
    display: grid;
    grid-template-columns: 4.5rem auto minmax(0, 1fr);
    gap: 0.5rem 0.75rem;
    align-items: baseline;
  }

  .row dd:first-of-type {
    white-space: nowrap;
  }

  dt {
    color: var(--fg-dim);
    font-size: 0.85rem;
  }

  dd {
    margin: 0;
    font-size: 0.9rem;
  }

  .when {
    color: var(--fg-dim);
    font-size: 0.8rem;
    text-align: end;
  }

  .limit {
    margin: 0;
    color: var(--fg-dim);
    font-size: 0.8rem;
    line-height: 1.45;
  }
</style>
