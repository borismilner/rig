<!-- The rail: the way home, every GUI, and the door to settings.

     ⛔ ITS UNIT IS A GUI, NOT A PROGRAM (section 11 requirement 16, which
     supersedes 12). Boris: "a GUI can be a GUI of a program but it could also
     be a GUI of some internal `rig` functionality like project management and
     things like that." An internal GUI here asserts nothing about the
     registry, which is what leaves section 5's promotion ruling standing.

     ⛔ REQUIREMENT 11 IS DISCHARGED HERE, AND THE PLACE IS THE ANSWER RATHER
     THAN THE CONTROL. Boris: "we can always go back to the main dashboard from
     all places conveniently." Section 11 states the constraint that decides
     where it can live: "a program's own pane draws its own chrome, so the way
     home must not depend on the program cooperating."

     HOW IT IS SOLVED. The shell is `grid-template-columns: 56px 1fr` and this
     rail is the FIRST track; every pane, including a program's full-bleed
     iframe, is painted inside `.main`, the second. They are different grid
     areas, so nothing a program draws can cover this control at any z-index,
     with any position, at any size - the frame has no box on this side to
     paint into. A structural guarantee rather than a stacking-order one, which
     is the difference between a way home and a way home a program can remove.

     ⛔ AND THE KEYBOARD IS THE SECOND DOOR, NOT THE ANSWER. Esc returns to the
     dashboard from the window handler in App.svelte, but keystrokes inside a
     cross-origin frame never reach this document - the same boundary section
     11 already records for :focus-within, measured in webkit2gtk. So while a
     program's page has the keyboard the rail is the ONLY way back, which is
     exactly why it had to be the control that cannot be covered.

     Section 11 is also explicit that the rail never re-orders under your hand,
     so the order is the registry's and nothing here sorts it. -->
<script lang="ts">
  export type RailEntry = {
    id: string;
    title: string;
    /** Two characters, for a program that declared no icon path. */
    glyph: string;
    /** An SVG path for an internal GUI. Mutually exclusive with glyph. */
    icon?: string;
    /* ⛔ A HUE MEMBER NAME, AND ONLY AN INTERNAL GUI HAS ONE.
     *
     * Requirement 17 asks for colours so entries are distinguishable at a
     * glance. Section 11 gives a PROGRAM one ownable hue - and `rigv1.Identity`
     * carries id, name, version, icon and description and no hue at all, which
     * cmd/rigwindow/service.go already records. So a program is achromatic
     * TODAY AND NOT BY CHOICE, and deriving a colour from its id would fake a
     * declaration the program never made to win a screenshot. It stays
     * undefined until the wire carries one.
     */
    hue?: string;
  };

  interface Props {
    entries: RailEntry[];
    selected: string | null;
    /** True while the dashboard is the destination, so the home mark carries
        aria-current and the notch leaves the list. */
    atHome: boolean;
    onselect: (id: string) => void;
    onhome: () => void;
    onsettings: () => void;
  }

  let { entries, selected, atHome, onselect, onhome, onsettings }: Props =
    $props();

  // Measured, not guessed. The notch slides to the selected marker, and the
  // rail's top offset depends on the home mark's height and the rail's
  // padding - both token-driven, so a hard-coded offset goes wrong the first
  // time the density knob moves.
  const MARK_H = 44;
  let marks: HTMLButtonElement[] = $state([]);
  let railTop = $derived(marks[0]?.offsetTop ?? 0);
  let index = $derived(
    atHome ? -1 : entries.findIndex((p) => p.id === selected),
  );

  // On the buttons rather than on the <nav>: a keydown listener on a
  // non-interactive element is an a11y warning and, worse, a keyboard path
  // that only works when focus happens to be inside it. A marker has focus or
  // it does not.
  function onkeydown(e: KeyboardEvent) {
    if (entries.length === 0) return;
    const step = e.key === "ArrowDown" ? 1 : e.key === "ArrowUp" ? -1 : 0;
    if (step === 0) return;
    e.preventDefault();
    const from = index < 0 ? (step > 0 ? -1 : 0) : index;
    const next = (from + step + entries.length) % entries.length;
    onselect(entries[next].id);
    marks[next]?.focus();
  }
</script>

<nav class="rail" aria-label="rig">
  <!-- The home mark is a figure rather than two letters, because two letters
       are already what an entry looks like and one more would read as another
       GUI rather than as the way out of all of them. -->
  <button
    class="home"
    aria-current={atHome}
    aria-label="Dashboard"
    title="Dashboard"
    onclick={onhome}
  >
    <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false">
      <rect x="1" y="1" width="6" height="6" rx="1.4" />
      <rect x="9" y="1" width="6" height="6" rx="1.4" />
      <rect x="1" y="9" width="6" height="6" rx="1.4" />
      <rect x="9" y="9" width="6" height="6" rx="1.4" />
    </svg>
  </button>

  <span class="sep" aria-hidden="true"></span>

  {#each entries as p, i (p.id)}
    <button
      class="mark"
      class:hued={!!p.hue}
      bind:this={marks[i]}
      aria-current={!atHome && p.id === selected}
      title={p.title}
      onclick={() => onselect(p.id)}
      {onkeydown}
      style={p.hue ? `--mhue: var(--h-${p.hue})` : undefined}
    >
      {#if p.icon}
        <span class="ico art">
          <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false">
            <path d={p.icon} />
          </svg>
        </span>
      {:else}
        <span class="ico">{p.glyph}</span>
      {/if}
    </button>
  {/each}

  {#if index >= 0}
    <span
      class="notch"
      style={`translate: 0 ${railTop + index * MARK_H}px; ${
        entries[index]?.hue ? `background: var(--h-${entries[index].hue})` : ""
      }`}
    ></span>
  {/if}

  <!-- ⛔ REQUIREMENT 13's FIRST DOOR. Boris: "rig settings is a separate GUI
       and can be accessed both from a dedicated GUI button and from the
       system-tray." Settings shipped keyboard-only at M1a step 5 and the
       recorded reason was that the context bar had no room for another
       control - so the room was found here instead, in the one strip that is
       on screen everywhere and that no pane can cover. The keyboard path
       stays: it was never the problem. -->
  <button
    class="gear"
    aria-label="Settings"
    title="Settings (,)"
    onclick={onsettings}
  >
    <svg viewBox="0 0 16 16" aria-hidden="true" focusable="false">
      <circle cx="8" cy="8" r="2.4" />
      <path
        d="M8 .9l1 1.9 2.1-.5.6 2.1 2 .8-1.1 1.8 1.1 1.8-2 .8-.6 2.1-2.1-.5-1 1.9-1-1.9-2.1.5-.6-2.1-2-.8L2.4 8 1.3 6.2l2-.8.6-2.1 2.1.5z"
      />
    </svg>
  </button>
</nav>

<style>
  /* Both shell controls share the marker's geometry so the rail reads as one
     column, and differ in what they draw so they do not read as GUIs. */
  .home,
  .gear {
    width: 56px;
    height: 40px;
    display: grid;
    place-items: center;
    border: 0;
    background: none;
    padding: 0;
    cursor: pointer;
    position: relative;
  }

  .home svg,
  .gear svg {
    width: 17px;
    height: 17px;
    fill: var(--fg-dim);
    transition: fill 0.2s;
  }

  .home:hover svg,
  .gear:hover svg {
    fill: var(--fg);
  }

  .home[aria-current="true"] svg {
    fill: var(--fg);
  }

  /* The current-destination mark, in the same language as the entry notch: a
     bar on the rail's leading edge. Drawn here rather than reusing .notch
     because the notch animates between entry positions and home is not in
     that sequence - sliding it up to the top and back would say the dashboard
     was one of the GUIs. It stays achromatic: no GUI owns the dashboard, so
     nothing owns its colour. */
  .home[aria-current="true"]::before {
    content: "";
    position: absolute;
    inset-inline-start: 0;
    top: 50%;
    translate: 0 -50%;
    width: 3px;
    height: 22px;
    background: var(--fg-dim);
    border-radius: 0 3px 3px 0;
  }

  .home:focus-visible,
  .gear:focus-visible {
    outline: none;
  }

  .home:focus-visible svg,
  .gear:focus-visible svg {
    fill: var(--fg);
  }

  .home:focus-visible::after,
  .gear:focus-visible::after {
    content: "";
    position: absolute;
    inset: 4px 10px;
    border-radius: 8px;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }

  .sep {
    width: 22px;
    height: 1px;
    background: var(--border);
    margin: 0.3rem 0 0.45rem;
  }

  /* Pushes settings to the foot of the rail. Section 11's chrome budget is a
     number that gets defended: this adds no track and no width, only two
     controls inside the 56px the rail already had. */
  .gear {
    margin-top: auto;
  }

  /* An icon entry, for an internal GUI. Same 28px tile as a program's two
     letters, so the column stays a column. */
  .ico.art {
    display: grid;
    place-items: center;
  }

  .ico.art svg {
    width: 15px;
    height: 15px;
    fill: currentColor;
  }

  /* ── the owned colour, requirement 17 ──────────────────────────────────
     Only .hued entries have --mhue, and only internal GUIs are .hued. A
     program entry inherits every rule in app.css unchanged and stays
     achromatic - which is the product's actual state rather than a default
     nobody chose, because the wire carries no hue for a program.

     THE COLOUR IS ON THE GLYPH AND NOT ON THE BORDER AT REST. A hue is gated
     as text by design/theme.js and is safe as a mark on --panel; a border has
     to clear 1.4.11's 3:1 on its own, and mixing a hue into --border would
     move a measured token to an unmeasured one for no gain. The border stays
     the token the engine solved. */
  .mark.hued .ico {
    color: var(--mhue);
  }

  .mark.hued:hover .ico {
    border-color: var(--mhue);
  }

  .mark.hued[aria-current="true"] .ico {
    color: var(--mhue);
    border-color: var(--mhue);
    background: color-mix(in srgb, var(--mhue) 13%, var(--panel));
    box-shadow: 0 0 20px -4px color-mix(in srgb, var(--mhue) 75%, transparent);
  }

  .mark.hued[aria-current="true"]:focus-visible .ico {
    box-shadow:
      0 0 20px -4px color-mix(in srgb, var(--mhue) 75%, transparent),
      0 0 0 var(--ring-w) var(--hue);
  }
</style>
