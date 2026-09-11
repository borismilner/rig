<!-- M1a step 5: the theme, changed from here, with a gate that refuses a set
     nobody could read.

     Section 23's M1a row: "The theme is changed from the settings UI and an
     unreadable set is refused by a gate that runs." The refusal is the
     load-bearing half - a panel that changes the theme and accepts an
     unreadable one has not closed the step.

     WHY THE MODE CONTROL IS NOT A CONVENIENCE. Measured 2026-09-11: setting
     GNOME's colour-scheme to prefer-light and restarting the window left it
     dark, so webkit is not reporting that change through
     prefers-color-scheme and nothing else set data-theme. Light theme was
     UNREACHABLE in the real runtime - only the contrast gate's headless
     Chrome had ever seen it, by flipping the attribute directly. This control
     is the only route to it.

     WHY THESE KNOBS AND NOT THE WHOLE SCHEMA. ui.theme has faces, type,
     shape, hues, surfaces and motion. The two exposed here are the two that
     can produce an unreadable set, which is what the step has to demonstrate:
     the ladder's own text lightness (CHOSEN, not solved, so a slider breaks
     it directly) and the hue lightness (a hue is text wherever --sem-<role>
     is used). The rest are bounded by the schema and cannot make the product
     unreadable, so they wait for M4's real config surface rather than being
     mocked here. -->
<script lang="ts">
  import {
    check,
    explain,
    getTheme,
    defaultTheme,
    tryApply,
    setModeChoice,
    modeChoice,
    type CheckResult,
    type Mode,
    type ModeChoice,
  } from "./theme";

  interface Props {
    mode: Mode;
    onclose: () => void;
    onmode: (m: Mode) => void;
    onthemechange: () => void;
  }
  let { mode, onclose, onmode, onthemechange }: Props = $props();

  const clone = (o: any) => JSON.parse(JSON.stringify(o));

  let choice: ModeChoice = $state(modeChoice());
  // The draft, so a slider can sit on a refused value while the page keeps the
  // last readable one. Applying on every input event with no draft would mean
  // the only way to see a refusal is to already be unable to read it.
  let draft: any = $state(clone(getTheme()));
  let refusal: CheckResult[] = $state([]);
  let applied = $state("");

  // The knobs edit the mode currently on screen, because a ladder is per-mode
  // and editing the one you cannot see is how the other theme becomes a trap.
  let fgL = $derived(draft.surfaces[mode].fg as number);
  let hueL = $derived(draft.hues[mode].L as number);

  // Live verdict on the draft, both modes, without applying anything. This is
  // the same engine call the build gate makes.
  let verdict = $derived(
    (["dark", "light"] as Mode[])
      .map((m) => check(draft, m))
      .filter((r) => !r.ok),
  );

  function setFg(v: number) {
    draft.surfaces[mode].fg = v;
    draft = draft;
  }
  function setHue(v: number) {
    draft.hues[mode].L = v;
    draft = draft;
  }

  function apply() {
    const r = tryApply(draft, mode);
    refusal = r.refused;
    if (r.ok) {
      applied = "applied";
      onthemechange();
      setTimeout(() => (applied = ""), 2400);
    }
  }

  function reset() {
    draft = defaultTheme();
    refusal = [];
    apply();
  }

  function pickMode(c: ModeChoice) {
    choice = c;
    onmode(setModeChoice(c));
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.stopPropagation();
      onclose();
    }
  }
</script>

<!-- A dialog, so Esc and the focus trap are the platform's job rather than
     hand-rolled. The heading is the accessible name. -->
<div
  class="scrim"
  role="presentation"
  onclick={onclose}
  onkeydown={() => {}}
></div>

<div
  class="panel"
  role="dialog"
  aria-modal="true"
  aria-labelledby="settings-title"
  tabindex="-1"
  onkeydown={onkeydown}
>
  <header>
    <h2 id="settings-title">Theme</h2>
    <button class="x" onclick={onclose} aria-label="Close settings">
      <kbd>Esc</kbd>
    </button>
  </header>

  <section>
    <h3>Mode</h3>
    <div class="seg" role="group" aria-label="Colour mode">
      {#each ["dark", "light", "system"] as const as c}
        <button
          class="opt"
          aria-pressed={choice === c}
          onclick={() => pickMode(c)}>{c}</button
        >
      {/each}
    </div>
    <p class="hint">
      {#if choice === "system"}
        Following the desktop. Verified 2026-09-11: this window does not see a
        desktop change, so <em>system</em> is whatever it read at startup.
      {:else}
        An explicit choice, written as <code>data-theme</code> - the same
        attribute the contrast gate flips to measure both themes.
      {/if}
    </p>
  </section>

  <section>
    <h3>Editing the <span class="mode">{mode}</span> ladder</h3>

    <label>
      <span class="lab">Text lightness</span>
      <input
        type="range"
        min="0.2"
        max="0.98"
        step="0.002"
        value={fgL}
        oninput={(e) => setFg(+e.currentTarget.value)}
      />
      <span class="num">{fgL.toFixed(3)}</span>
    </label>

    <label>
      <span class="lab">Hue lightness</span>
      <input
        type="range"
        min="0.2"
        max="0.95"
        step="0.002"
        value={hueL}
        oninput={(e) => setHue(+e.currentTarget.value)}
      />
      <span class="num">{hueL.toFixed(3)}</span>
    </label>

    <div class="row">
      <button class="go" onclick={apply} disabled={verdict.length > 0}>
        {verdict.length ? "Refused" : "Apply"}
      </button>
      <button class="go flat" onclick={reset}>Reset to default</button>
      {#if applied}<span class="ok">{applied}</span>{/if}
    </div>
  </section>

  <!-- The refusal, and it names the token AND the ground because the fix
       differs by ground: failing only on --tint means the token is used on the
       wrong surface, failing on all five means it is the wrong colour. -->
  {#if verdict.length}
    <section class="refused">
      <h3>
        This set is unreadable, so it will not be applied
      </h3>
      {#each verdict as v}
        <p class="which">in the <strong>{v.mode}</strong> theme:</p>
        <ul>
          {#each explain(v).slice(0, 7) as line}
            <li>{line}</li>
          {/each}
        </ul>
        {#if explain(v).length > 7}
          <p class="more">
            and {explain(v).length - 7} more token/ground pairs
          </p>
        {/if}
      {/each}
      <p class="why">
        Both themes are judged, not just the one on screen: a set that is fine
        here and unreadable in the other mode would be a trap found later.
      </p>
    </section>
  {:else if refusal.length === 0}
    <p class="clean">
      Every token clears its target on every ground it can land on.
    </p>
  {/if}
</div>

<style>
  .scrim {
    position: fixed;
    inset: 0;
    background: color-mix(in srgb, var(--bg) 72%, transparent);
    z-index: 10;
  }
  .panel {
    position: fixed;
    z-index: 11;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    width: min(34rem, calc(100vw - 2rem));
    max-height: min(46rem, calc(100vh - 2rem));
    overflow-y: auto;
    box-sizing: border-box;
    padding: 1.1rem 1.25rem 1.4rem;
    background: var(--panel);
    color: var(--fg);
    border: 1px solid var(--border);
    border-radius: var(--radius, 12px);
    box-shadow: var(--shadow-lg);
  }
  header {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 1rem;
    margin-block-end: 0.4rem;
  }
  h2 {
    margin: 0;
    font-family: var(--disp), serif;
    font-size: var(--fs-1);
    letter-spacing: var(--tight-disp);
  }
  h3 {
    margin: 1.1rem 0 0.5rem;
    font-size: var(--fs--1);
    font-weight: 600;
    color: var(--fg-dim);
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .mode {
    color: var(--fg);
    text-transform: none;
    letter-spacing: 0;
    font-family: var(--mono), monospace;
  }
  .x {
    background: none;
    border: 0;
    cursor: pointer;
    padding: 0.1rem 0.2rem;
    color: var(--fg-faint);
  }
  .x:focus-visible,
  .opt:focus-visible,
  .go:focus-visible,
  input:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w, 3px) var(--border-2);
  }
  kbd {
    font-family: var(--mono), monospace;
    font-size: var(--fs--2);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 0.05rem 0.3rem;
    color: var(--fg-dim);
  }
  .seg {
    display: flex;
    gap: 0.35rem;
  }
  .opt {
    font: inherit;
    font-size: var(--fs--1);
    padding: 0.25rem 0.8rem;
    border-radius: 999px;
    border: 1px solid var(--border);
    background: transparent;
    color: var(--fg-dim);
    cursor: pointer;
  }
  .opt[aria-pressed="true"] {
    border-color: var(--border-2);
    color: var(--fg);
    background: var(--glow);
  }
  .hint,
  .why,
  .more,
  .clean {
    margin: 0.5rem 0 0;
    font-size: var(--fs--1);
    color: var(--fg-dim);
    line-height: var(--lh, 1.55);
  }
  label {
    display: grid;
    grid-template-columns: 9rem 1fr 4rem;
    align-items: center;
    gap: 0.6rem;
    margin-block-end: 0.5rem;
  }
  .lab {
    font-size: var(--fs--1);
    color: var(--fg-dim);
  }
  .num {
    font-family: var(--mono), monospace;
    font-size: var(--fs--2);
    text-align: right;
    color: var(--fg);
  }
  input[type="range"] {
    width: 100%;
    accent-color: var(--border-2);
  }
  .row {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-block-start: 0.9rem;
  }
  .go {
    font: inherit;
    font-size: var(--fs--1);
    padding: 0.3rem 0.9rem;
    border-radius: 8px;
    border: 1px solid var(--border-2);
    background: var(--glow);
    color: var(--fg);
    cursor: pointer;
  }
  .go.flat {
    background: transparent;
    border-color: var(--border);
    color: var(--fg-dim);
  }
  .go:disabled {
    cursor: not-allowed;
    border-color: var(--border);
    color: var(--fg-faint);
    background: transparent;
  }
  .ok {
    font-size: var(--fs--1);
    color: var(--sem-good, var(--fg-dim));
  }
  .refused {
    margin-block-start: 1rem;
    padding: 0.8rem 0.9rem;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: var(--bg-2);
  }
  .refused h3 {
    margin-block-start: 0;
    color: var(--fg);
    text-transform: none;
    letter-spacing: 0;
    font-size: var(--fs-0);
  }
  .which {
    margin: 0.4rem 0 0.2rem;
    font-size: var(--fs--1);
    color: var(--fg-dim);
  }
  ul {
    margin: 0;
    padding-inline-start: 1.1rem;
  }
  li {
    font-family: var(--mono), monospace;
    /* var(--fs--2) carries the scale's own max(12px, ...) floor. A raw em here
       would walk around it, which is how a 10.16px cell reached docket. */
    font-size: var(--fs--2);
    line-height: 1.7;
    color: var(--fg);
  }
</style>
