<!-- One work-item row, and it OPENS.

     ⛔ BORIS, 2026-09-17, with a screenshot: "I'm not sure if what I'm seeing
     is the short descriptions in this GUI but they are not user friendly and
     clicking them doesn't show the full description. The information must be
     stored in a way that helps the human reviewers... I'm sure they can be
     grouped or at least tagged so that the user can see what relates to what."

     ⛔ CLICKING DID NOTHING BECAUSE THERE WAS NOTHING TO SHOW. The store has
     computed description_short, priority, status, owner and tags since section
     39 row 2, and the daemon's itemToWire copied five fields and dropped all of
     them. The row was a truncated title with an empty record behind it, so a
     detail pane would have rendered the same line twice. The wire carries them
     as of 2026-09-17; this is the surface for them.

     ⛔ IT IS A <button>, NOT A <li> WITH AN ONCLICK. A row that opens is a
     control: it needs the keyboard, a focus ring and an aria-expanded, and the
     rail's own entries already set that precedent. A div with a click handler
     is reachable by exactly one input device.

     ⛔ AND IT IS ONE COMPONENT BECAUSE THERE WERE TWO IDENTICAL LISTS. "Next
     up" and "Open" carried the same four spans; a change to a row had to be
     made twice and the second one is the one that gets forgotten. -->
<script lang="ts">
  import type { Item } from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";
  import { itemTypeView } from "./itemtype";

  interface Props {
    item: Item;
    /** The tone dot's value, decided by the caller that knows the step vocabulary. */
    tone: string;
    /** What the right-hand cell says: an age for unstepped, the state otherwise. */
    trailing: string;
    /** True when the item has never been stepped, which the trailing cell styles. */
    unstepped: boolean;
    /* ⛔ THE MEASUREMENT FIXTURE, ON initialSide's PRECEDENT. An opened row
       is a whole panel of colours - a rule, a description, five fact pairs and
       the tag pills - and the contrast gate cannot click. Without this the
       opened state ships unmeasured, which is this project's named defect: a
       check that cannot fail reads as a pass. It changes how the state is
       REACHED and never the state or the colours. */
    startOpen?: boolean;
  }

  let { item, tone, trailing, unstepped, startOpen = false }: Props = $props();

  let open = $state(startOpen);

  // null when the item is untyped, which is most of them and is honest.
  let ty = $derived(itemTypeView(item.itemType));

  // ⛔ WHETHER THERE IS ANYTHING BEHIND THE ROW AT ALL. A row with an empty
  // record must not offer to open and then show a blank panel - that is the
  // original complaint in a new costume. An untyped, undescribed, unowned item
  // says so on its face instead.
  let hasDetail = $derived(
    !!(
      item.descriptionShort ||
      item.owner ||
      item.priority ||
      item.status ||
      item.targetDate ||
      item.semver ||
      (item.tags && item.tags.length > 0) ||
      item.note
    ),
  );
</script>

<li class="row" class:open>
  <button
    type="button"
    class="head"
    aria-expanded={open}
    disabled={!hasDetail}
    onclick={() => (open = !open)}
  >
    <span class="dotm" data-tone={tone}></span>
    <!-- ⛔ THE TYPE SLOT IS ALWAYS PRESENT, EVEN WHEN EMPTY. A column that
         appears only for typed rows would shift every other row sideways as
         soon as one item is classified, and the lists are read by scanning a
         column. An untyped row reserves the space and draws nothing. -->
    <span class="ity" title={ty ? ty.label : "untyped"}>
      {#if ty && ty.icon}
        <svg viewBox="0 0 16 16" aria-hidden="true" style:color={ty.hue ? `var(--h-${ty.hue})` : "inherit"}>
          <path d={ty.icon} />
        </svg>
      {/if}
    </span>
    <span class="iid">{item.id}</span>
    <span class="it">{item.title}</span>
    <span class="ist" class:none={unstepped}>{trailing}</span>
    <!-- The affordance is drawn only when there is something to open, so the
         row's own appearance answers "is there more here" before a click. -->
    <span class="chev" aria-hidden="true">{hasDetail ? (open ? "−" : "+") : ""}</span>
  </button>

  {#if open}
    <div class="detail">
      {#if item.descriptionShort}
        <p class="desc">{item.descriptionShort}</p>
      {/if}

      <dl class="facts">
        <div>
          <dt>type</dt>
          <dd>{ty ? ty.label : "untyped"}</dd>
        </div>
        {#if item.status}
          <div><dt>status</dt><dd>{item.status}</dd></div>
        {/if}
        {#if item.owner}
          <div><dt>owner</dt><dd>{item.owner}</dd></div>
        {/if}
        {#if item.priority}
          <div><dt>priority</dt><dd>{item.priority}</dd></div>
        {/if}
        {#if item.targetDate}
          <div><dt>target</dt><dd>{item.targetDate}</dd></div>
        {/if}
        {#if item.semver}
          <div><dt>semver</dt><dd>{item.semver}</dd></div>
        {/if}
      </dl>

      {#if item.tags && item.tags.length > 0}
        <ul class="tags">
          {#each item.tags as t (t)}
            <li>{t}</li>
          {/each}
        </ul>
      {/if}

      {#if item.note}
        <p class="note"><span class="nlabel">latest step</span> {item.note}</p>
      {/if}
    </div>
  {/if}
</li>

<style>
  .row {
    display: block;
  }

  .head {
    /* The same five-column grid the two lists used, plus the affordance. */
    display: grid;
    grid-template-columns: 2.2rem 1.1rem minmax(7ch, auto) minmax(0, 1fr) auto 1.2rem;
    gap: 0.5rem;
    align-items: baseline;
    width: 100%;
    font: inherit;
    text-align: start;
    background: none;
    border: 0;
    border-radius: 4px;
    padding: 0.28rem 0.3rem;
    color: inherit;
    cursor: pointer;
  }

  .head:disabled {
    cursor: default;
  }

  .head:not(:disabled):hover {
    background: var(--bg-2);
  }

  .head:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }


  /* ⛔ CARRIED VERBATIM FROM PlanVsExec's SCOPED CSS, because Svelte scopes
     styles to the component holding the markup and the markup moved. These are
     the same values, not a re-derivation: the tone dot, the mono id, the
     ellipsised title and the neutral state cell. */
  .dotm {
    width: 12px;
    height: 6px;
    border-radius: 2px;
    border: 1.5px solid var(--border-2);
    align-self: center;
  }

  .dotm[data-tone="good"] {
    background: var(--sem-good);
    border-color: var(--sem-good);
  }
  .dotm[data-tone="progress"] {
    background: var(--sem-progress);
    border-color: var(--sem-progress);
  }
  .dotm[data-tone="bad"] {
    background: var(--sem-bad);
    border-color: var(--sem-bad);
  }
  .dotm[data-tone="warn"] {
    background: var(--sem-warn);
    border-color: var(--sem-warn);
  }

  .iid {
    font-family: var(--mono);
    color: var(--fg-dim);
  }

  .it {
    color: var(--fg);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .ist {
    font-family: var(--mono);
    color: var(--fg-dim);
    white-space: nowrap;
  }

  /* ⛔ NOT AMBER, AND THE REASON IS A COUNT - the real window showed 54 rows
     of identical amber down one column and the hue became the list's noise.
     The warn hue is spent ONCE, on the headline zero. Preserved on the move. */
  .ist.none {
    color: var(--fg-dim);
  }

  .ity {
    display: grid;
    place-items: center;
    align-self: center;
  }

  .ity svg {
    width: 11px;
    height: 11px;
    fill: currentColor;
  }

  /* A type with no hue of its own stays dim, so an unrecognised classification
     never reads louder than bug or idea. */
  .ity svg:not([style*="--h-"]) {
    color: var(--fg-dim);
  }

  .chev {
    color: var(--fg-dim);
    font-size: 0.85rem;
    text-align: center;
  }

  /* ⛔ THE OPEN PANEL IS INSET WITH A RULE, NOT A TINTED GROUND. A tinted
     background behind body text is the B71 defect: the contrast gate reads the
     token underneath a background-image or a blend and reports clean while the
     eye reads something else. A left rule carries no text on it. */
  .detail {
    margin: 0.1rem 0 0.6rem 2.7rem;
    padding-inline-start: 0.85rem;
    border-inline-start: 2px solid var(--border-2);
    display: grid;
    gap: 0.5rem;
  }

  .desc {
    margin: 0;
    font-size: var(--fs--1);
    line-height: 1.5;
    max-width: 68ch;
  }

  .facts {
    margin: 0;
    display: flex;
    flex-wrap: wrap;
    gap: 0.3rem 1.2rem;
  }

  .facts div {
    display: flex;
    gap: 0.4rem;
    align-items: baseline;
  }

  dt {
    color: var(--fg-dim);
    font-size: 0.78rem;
  }

  dd {
    margin: 0;
    font-size: 0.82rem;
  }

  .tags {
    margin: 0;
    padding: 0;
    list-style: none;
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
  }

  .tags li {
    font-size: 0.75rem;
    color: var(--fg-dim);
    border: 1px solid var(--border);
    border-radius: 999px;
    padding: 0.05rem 0.5rem;
  }

  .note {
    margin: 0;
    font-size: var(--fs--1);
    line-height: 1.5;
    max-width: 68ch;
  }

  .nlabel {
    color: var(--fg-dim);
    font-size: 0.78rem;
    margin-inline-end: 0.3rem;
  }
</style>
