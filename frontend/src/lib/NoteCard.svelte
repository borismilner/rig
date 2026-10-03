<!-- One notification's details, as a card in the centre of the screen
     (plan/55 requirements 26 and 32).

     ⛔ BORIS, 2026-10-03: "clicking a notification should open a panel with
     the full details and all the actions and options applicable to the
     notification if not yet decided" - and then, on the mockup, a pop-up
     card in the exact centre, not a side panel.

     Undecided, the options answer through rig.toast.reply, the call a bubble
     makes, so the program that asked is told whichever surface answered.
     Decided, they stay on screen disabled, with the choice marked. -->
<script lang="ts">
  import { onMount } from "svelte";
  import type { Note } from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/models.js";
  import { stamp } from "./estate";
  import { answered, sevHue, waiting } from "./notes";

  interface Props {
    note: Note;
    onclose: () => void;
    /* Resolves when rig took the answer; rejects with rig's refusal. */
    onanswer: (reply: string, text: string, dismissed: boolean) => Promise<void>;
    /* "Only X's notifications". */
    onsource: (sender: string) => void;
  }
  let { note, onclose, onanswer, onsource }: Props = $props();

  let dialog: HTMLDialogElement;
  let text = $state("");
  let busy = $state(false);
  let refused = $state("");

  let open = $derived(waiting(note));
  let chosen = $derived(note.answer?.reply ?? "");

  onMount(() => {
    dialog.showModal();
    (
      dialog.querySelector<HTMLElement>(".opts button:not([disabled])") ??
      dialog.querySelector<HTMLElement>("footer .act:last-child")
    )?.focus();
  });

  async function send(reply: string, dismissed = false) {
    busy = true;
    refused = "";
    try {
      await onanswer(reply, note.replyText ? text.trim() : "", dismissed);
    } catch (e) {
      refused = String(e);
    } finally {
      busy = false;
    }
  }

  // A click on the backdrop is a click on the dialog itself.
  function onclick(e: MouseEvent) {
    if (e.target === dialog) dialog.close();
  }
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
<dialog
  bind:this={dialog}
  class="ncard"
  aria-labelledby="ncard-title"
  {onclose}
  {onclick}
>
  <header>
    <span class="sevdot" style="--sev: {sevHue(note.severity)}"></span>
    <h2 id="ncard-title" class="t-sec">{note.title}</h2>
    <button class="act x" aria-label="Close" onclick={() => dialog.close()}
      >&times;</button
    >
  </header>

  <div class="body">
    <dl class="facts">
      <dt>from</dt>
      <dd>{note.sender}</dd>
      <dt>severity</dt>
      <dd>{note.severity || "info"}</dd>
      <dt>at</dt>
      <dd class="mono">{stamp(Date.parse(note.at))}</dd>
      {#if note.suppressed}<dt>shown</dt>
        <dd>no; Do Not Disturb held it, so it went to the record only</dd>{/if}
      <dt>record</dt>
      <dd class="mono">{note.id}</dd>
    </dl>

    {#if note.body}<p class="text">{note.body}</p>{/if}

    {#if !note.asks}
      <h3>Nothing to decide</h3>
      <p class="dim">This notification only informs.</p>
    {:else}
      <h3>{open ? "Your answer" : "Decided"}</h3>
      {#if note.answer}
        <p class="chosen">
          Answered {answered(note)}{note.answer.at
            ? ` at ${stamp(Date.parse(note.answer.at))}`
            : ""}. {note.sender} was told.
        </p>
      {:else if note.forgotten}
        <p class="dim">
          Rig restarted after this was asked, so nobody is waiting for the
          answer any more and it cannot be given here.
        </p>
      {/if}
      {#if note.replyText}
        <label class="free"
          ><span class="dim">Your words</span>
          <textarea
            rows="3"
            bind:value={text}
            disabled={!open || busy}
            placeholder={open ? "Type an answer" : ""}
          ></textarea></label
        >
      {/if}
      <div class="opts">
        {#each note.replies as r, i (r)}
          <button
            class="act"
            class:primary={open && i === 0}
            class:picked={chosen === r}
            disabled={!open || busy}
            aria-pressed={open ? undefined : chosen === r}
            onclick={() => send(r)}>{chosen === r ? `✓ ${r}` : r}</button
          >
        {/each}
        {#if note.replyText && note.replies.length === 0}
          <button
            class="act primary"
            disabled={!open || busy || !text.trim()}
            onclick={() => send("")}>Send</button
          >
        {/if}
        {#if open}
          <button
            class="act"
            disabled={busy}
            onclick={() => send("", true)}
            title="Answer nothing; the program is told it was dismissed"
            >Dismiss</button
          >
        {/if}
      </div>
      {#if refused}<p class="refused" role="alert">{refused}</p>{/if}
    {/if}
  </div>

  <footer>
    <button
      class="act"
      onclick={() => {
        onsource(note.sender);
        dialog.close();
      }}>Only {note.sender}'s notifications</button
    >
    <span class="sp"></span>
    <button class="act" onclick={() => dialog.close()}>Close</button>
  </footer>
</dialog>

<style>
  .ncard {
    margin: auto;
    inset: 0;
    width: min(640px, 94vw);
    max-height: 86vh;
    padding: 0;
    border: 1px solid var(--border-2);
    border-radius: 14px;
    background: var(--panel);
    color: var(--fg);
    box-shadow: var(--shadow-lg);
    flex-direction: column;
  }
  .ncard[open] {
    display: flex;
  }
  .ncard::backdrop {
    background: rgb(0 0 0 / 0.45);
  }
  header {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    padding: 0.9rem 1.1rem;
    border-bottom: 1px solid var(--border);
  }
  h2 {
    margin: 0;
    flex: 1;
    font-size: var(--fs-1);
    overflow-wrap: anywhere;
  }
  .sevdot {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: var(--sev);
    flex: none;
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 1rem 1.1rem;
  }
  .facts {
    display: grid;
    grid-template-columns: 90px minmax(0, 1fr);
    gap: 0.4rem 1rem;
    margin: 0 0 0.8rem;
    font-size: var(--fs--1);
  }
  .facts dt {
    color: var(--fg-dim);
  }
  .facts dd {
    margin: 0;
    overflow-wrap: anywhere;
  }
  .text {
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    margin: 0 0 0.8rem;
  }
  h3 {
    font-size: var(--fs-0);
    margin: 0.4rem 0;
  }
  .chosen {
    margin: 0 0 0.6rem;
    border-left: 3px solid var(--sem-good);
    padding-left: 0.6rem;
  }
  .free {
    display: grid;
    gap: 0.3rem;
    margin-bottom: 0.6rem;
  }
  textarea {
    font: inherit;
    color: var(--fg);
    background: var(--bg);
    border: 1px solid var(--border-2);
    border-radius: 7px;
    padding: 0.4rem 0.5rem;
    resize: vertical;
  }
  .opts {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
  }
  .act.picked:disabled {
    color: var(--fg);
    border-color: var(--sem-good);
    font-weight: 600;
  }
  .refused {
    color: var(--sem-bad);
    font-size: var(--fs--1);
    margin: 0.6rem 0 0;
  }
  .dim {
    color: var(--fg-dim);
    font-size: var(--fs--1);
  }
  .mono {
    font-family: var(--mono);
  }
  footer {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    padding: 0.75rem 1.1rem;
    border-top: 1px solid var(--border);
  }
  .sp {
    flex: 1;
  }
</style>
