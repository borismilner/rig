<!-- The pane, at the generated tier and the transport for the program's own.
     Section 11 has three: generated (rig renders from a declared schema), kit
     (the program serves its own HTML from rig's elements) and embedded (its own
     HTML, its own components, the token set and nothing more). Which one a
     program gets turns on one field: an empty pane_url means rig draws the pane
     from what was declared, and a value means the program draws it.

     The frame is cross-origin - the program serves on loopback, this page comes
     from the Wails asset origin - so the window cannot reach into it and the
     token set has to be handed over. postMessage is the only channel, and the
     protocol is four messages:

       page -> window  {rig:1, type:"hello"}   I am up, send the tokens
       window -> page  {rig:1, type:"theme", mode, tokens}
       page -> window  {rig:1, type:"focus"}   the keyboard is mine
       page -> window  {rig:1, type:"blur"}    and now it is not

     The page speaks first, because the window cannot know when the page's
     script is ready and a push timed from this side would race the frame's own
     load. The theme is pushed again on every mode change, which is the shape
     section 6's live push needs anyway. -->
<script lang="ts">
  import type { Program } from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";
  import { tokenSet, type Mode } from "./theme";

  interface Props {
    program: Program | null;
    programs: Program[];
    detail: string;
    connected: boolean;
    mode: Mode;
    // The measurement fixture, and it reaches the two pane states the contrast
    // gate could not otherwise see. Both are real renders of the real CSS: what
    // it replaces is how each state is REACHED, never the state or the colours.
    //
    //   the frame's ring   is drawn from framedFocus, which in the product
    //                      comes from a live page posting {rig:1,type:"focus"}.
    //                      A live port would make the gate depend on a
    //                      background server; a dead port serves no page and so
    //                      reports nothing. Under the fixture the frame's OWN
    //                      focus event sets it - the same state, the same CSS,
    //                      and the ring still appears on focus and goes on blur.
    //
    //                      It is wired this way and not pinned on, and the
    //                      difference is the whole measurement. The focus pass
    //                      works by focusing an element and diffing pixels, and
    //                      a ring that is already lit changes no pixel when the
    //                      frame takes focus. Pinned on, the gate reported the
    //                      iframe as having NO focus indicator while the ring
    //                      was on screen the whole time - and no other pass can
    //                      see it either, because the ring is a ::after and
    //                      the boundary and paint passes read getComputedStyle
    //                      on elements. Pinned on, this fixture rendered a ring
    //                      that nothing measured.
    //
    //   the unserved state needs `settled`, and the grace period below is
    //                      1200ms while the auditor settles at 450 - so a dead
    //                      port ALONE measures a blank frame and calls it
    //                      clean, which is a zero measurement wearing a pass.
    paneFixture: boolean;
  }

  let { program, programs, detail, connected, mode, paneFixture }: Props =
    $props();

  let frame: HTMLIFrameElement | null = $state(null);
  let loaded = $state(false);
  let helloed = $state(false);
  let framedFocus = $state(false);

  // The origin to address, from the URL the program declared. The kernel has
  // already refused anything but a loopback origin at registration, so this is
  // parsing a value that has been checked rather than trusting one.
  let origin = $derived(paneOrigin(program?.paneUrl ?? ""));

  function paneOrigin(u: string): string | null {
    if (!u) return null;
    try {
      return new URL(u).origin;
    } catch {
      return null;
    }
  }

  // Never "*": the target origin is named, so a page that is not the one the
  // program declared cannot be handed this window's token set.
  function post(msg: unknown): void {
    if (!origin) return;
    frame?.contentWindow?.postMessage(msg, origin);
  }

  function onmessage(e: MessageEvent): void {
    // Both checks matter. The origin says who sent it; the source says it came
    // from this pane's own frame rather than from any other page that happens
    // to be served from the same loopback origin.
    if (!origin || e.origin !== origin) return;
    if (!frame || e.source !== frame.contentWindow) return;
    const d = e.data as { rig?: number; type?: string } | null;
    if (!d || d.rig !== 1) return;

    switch (d.type) {
      case "hello":
        helloed = true;
        post({ rig: 1, type: "theme", mode, tokens: tokenSet(mode) });
        break;
      case "focus":
        framedFocus = true;
        break;
      case "blur":
        framedFocus = false;
        break;
    }
  }

  // A new program means a new frame, so nothing carries over from the last one.
  //
  // Keyed on the id STRING and not on the program object, and that distinction
  // is a bug this pane already had: the 3s poll replaces `programs` wholesale,
  // so `program` is a new object every tick even when nothing about it changed.
  // An effect that read the object re-ran every three seconds and cleared these
  // flags under a frame that was still perfectly alive - which also stranded
  // the program's page in the previous theme, because the push below only fires
  // for a page that has said hello, and hello is said once per load.
  let programId = $derived(program?.id ?? null);

  // The fixture's `settled` lands here rather than at the declaration above:
  // initialising a $state from a prop reads it non-reactively and Svelte warns
  // about exactly that. One assignment site, no warning, same result.
  $effect(() => {
    programId;
    loaded = false;
    helloed = false;
    framedFocus = false;
    settled = paneFixture;
  });

  // The grace period exists so a program that is merely slow to answer does not
  // get accused of being absent. Measured: webkit resolves a refused connection
  // well inside this, so the message is not what a working pane flashes on the
  // way in.
  const SETTLE_MS = 1200;

  let settled = $state(false);

  $effect(() => {
    if (paneFixture || !programId || loaded) return;
    const t = setTimeout(() => (settled = true), SETTLE_MS);
    return () => clearTimeout(t);
  });

  // Nothing is being served, and this is the one state the shell can assert
  // rather than guess. Measured in the real webview, all three cases:
  //
  //   a page that speaks the protocol   onload FIRED, hello YES
  //   a page that serves and is silent  onload FIRED, hello no
  //   nothing listening at all          onload no,    hello no
  //
  // So onload is the honest signal and hello is not: a program serving its own
  // static HTML and adopting nothing (which section 11's embedded tier allows)
  // never says hello, and drawing this state over its working page would be a
  // false accusation. What onload cannot separate is a program's own error page
  // from its real one - a 404 is a document, it fires, and it is the program's
  // error to show rather than rig's to report.
  // The fixture forces this rather than waiting for the signal, and the reason
  // is a browser difference that took a measurement to find: the truth table
  // above was measured in webkit2gtk, where a refused connection never fires
  // onload. HEADLESS CHROME, which is what the contrast gate runs, DOES fire
  // onload - it has an error document to load. So `loaded` goes true, the state
  // never draws, and a dead port alone measures a blank frame and calls it
  // clean. The product is unaffected: it runs webkit2gtk. Only the gate needed
  // telling.
  let unpainted = $derived(
    !!program?.paneUrl && (paneFixture || (settled && !loaded)),
  );

  // The theme is pushed on every change, not only on hello: section 6's live
  // push is the same shape, and a program's page must not be the one surface
  // left in the previous mode.
  $effect(() => {
    if (helloed) post({ rig: 1, type: "theme", mode, tokens: tokenSet(mode) });
  });

  // The frame keeps its real loopback origin (allow-same-origin) because the
  // token push in the next slice has to name a target origin, and a sandboxed
  // frame without it gets an opaque origin that nothing can be addressed to.
  // It cannot script this page either way - it is cross-origin with the parent,
  // so it cannot remove its own sandbox. What the sandbox does buy: a program
  // page cannot replace the whole window with a page of its own, open a popup,
  // submit a form or start a download.
  const SANDBOX = "allow-scripts allow-same-origin";
</script>

<svelte:window {onmessage} />

<main class="pane" class:own={!!program?.paneUrl} class:held={framedFocus}>
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
  {:else if program.paneUrl}
    <!-- Keyed on the id so switching programs builds a new frame rather than
         pointing the old one somewhere else, which would leave the previous
         program's page in this frame's session history. -->
    {#key program.id}
      <iframe
        bind:this={frame}
        src={program.paneUrl}
        title={`${program.id}'s own pane`}
        sandbox={SANDBOX}
        referrerpolicy="no-referrer"
        onload={() => (loaded = true)}
        onfocus={paneFixture ? () => (framedFocus = true) : undefined}
        onblur={paneFixture ? () => (framedFocus = false) : undefined}
      ></iframe>
      {#if unpainted}
        <div class="unserved">
          <h2>{program.id} is not serving its pane</h2>
          <p class="lead">
            It declared one at <code>{program.paneUrl}</code> and nothing answered
            there. The program is registered, so it is talking to rig over the socket;
            it is the page that is missing.
          </p>
        </div>
      {/if}
    {/key}
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
      Everything above came over the socket from rig's registry. This program
      declared no pane of its own, which is what puts it on the generated tier.
    </p>
  {/if}
</main>

<style>
  /* An own pane gets the whole area, with none of the shell's padding: the
     program draws to the edges or it does not draw to them, and that is its
     decision rather than the shell's. */
  .pane.own {
    position: relative;
    display: grid;
    padding: 0;
    overflow: hidden;
  }

  /* On top of the frame, which is safe precisely because this state is only
     drawn when onload never fired: there is no document under it to cover. */
  .unserved {
    position: absolute;
    inset: 0;
    z-index: 2;
    display: grid;
    align-content: center;
    justify-items: center;
    padding: 0 1.15rem;
    background: var(--panel);
    text-align: center;
  }

  .unserved .lead {
    margin: 0;
  }

  /* The ring the shell could not draw in CSS in slice 1: :focus-within does not
     cross a cross-origin frame boundary in webkit2gtk, so the page reports its
     own focus and this is drawn from that. A page that adopts nothing gets no
     ring, which is honest - the shell genuinely does not know where the
     keyboard is inside it.

     Inset, because the frame is flush to the pane's edges and an outset shadow
     would land outside the visible area. Same width and colour tokens as the
     rail's, so this window has one focus ring and not two. */
  .pane.own.held::after {
    content: "";
    position: absolute;
    inset: 0;
    z-index: 3;
    pointer-events: none;
    outline: var(--ring-w) solid var(--hue);
    outline-offset: calc(-1 * var(--ring-w));
  }

  iframe {
    width: 100%;
    height: 100%;
    border: 0;
    background: var(--bg-2);
  }

  /* TWO things this frame should do and cannot do in CSS, both measured in
     the real webview rather than reasoned about. Neither is worth retrying
     from this side; both land with the postMessage handshake, because a
     handshake is positive evidence and CSS here is trying to detect absence.

     1. An unpainted pane is silently blank. A program that declares a pane and
        serves nothing gets an empty pane with no message of any kind - which
        reads the same as an intentionally empty page, or a broken shell. The
        obvious fix, a message behind a transparent frame, does not work:
        webkit2gtk paints an opaque base background for a frame with no
        document, so the message never shows through. Detecting the failure
        directly is not available either - onerror does not fire for a refused
        connection or an HTTP error, and onload fires for an error document.
        So the shell has to hear from the page that it is there, and draw this
        state when it does not. KNOWN DEFECT until then.

     2. No focus ring, for the same shape of reason. While the keyboard is inside the program's page the shell should
     say so, and in this webview it cannot say it in CSS:

       - a button inside the frame takes focus from a click and draws its own
         ring, so focus really is in the nested document;
       - the parent's .pane.own:focus-within does NOT match while it is there,
         so webkit2gtk 2.52.6 does not carry :focus-within across a
         cross-origin frame boundary. iframe:focus does not match either.
       - four Tab presses from a rail marker never reached that button, so the
         frame is not obviously in the shell's sequential focus order.

     A ring that never lights is worse than no ring, so there is none here.
     The program's page has to report its own focus and blur, which is the same
     postMessage channel the token push needs, so the ring lands with the
     handshake rather than before it.

     What this does NOT change: the arrows. ArrowUp/ArrowDown stop walking the
     rail after a click anywhere in the pane, frame or no frame - Rail.svelte
     binds keydown on the markers, so a marker has focus or it does not. That
     is step 2's focus model, measured identically on the generated tier. */

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
