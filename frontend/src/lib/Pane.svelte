<!-- The pane, at the generated tier and the transport for the program's own.
     Section 11 has three: generated (rig renders from a declared schema), kit
     (the program serves its own HTML from rig's elements) and embedded (its own
     HTML, its own components, the token set and nothing more). Which one a
     program gets turns on one field: an empty pane_url means rig draws the pane
     from what was declared, and a value means the program draws it.

     What is here is the generated tier in full, plus the frame that carries an
     own pane. What is NOT here yet is the token set: that frame is cross-origin
     (the program serves on loopback, this page comes from the Wails asset
     origin), so the window cannot reach into it and the program currently gets
     no tokens at all. postMessage is the only channel and it is the next
     slice. Until then an own pane renders in the program's own colours, which
     is honest about what has been built rather than about what is intended. -->
<script lang="ts">
  import type { Program } from "../../bindings/github.com/boris-milner/rig/cmd/rigwindow/models.js";

  interface Props {
    program: Program | null;
    programs: Program[];
    detail: string;
    connected: boolean;
  }

  let { program, programs, detail, connected }: Props = $props();

  // The frame keeps its real loopback origin (allow-same-origin) because the
  // token push in the next slice has to name a target origin, and a sandboxed
  // frame without it gets an opaque origin that nothing can be addressed to.
  // It cannot script this page either way - it is cross-origin with the parent,
  // so it cannot remove its own sandbox. What the sandbox does buy: a program
  // page cannot replace the whole window with a page of its own, open a popup,
  // submit a form or start a download.
  const SANDBOX = "allow-scripts allow-same-origin";
</script>

<main class="pane" class:own={!!program?.paneUrl}>
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
        src={program.paneUrl}
        title={`${program.id}'s own pane`}
        sandbox={SANDBOX}
        referrerpolicy="no-referrer"
      ></iframe>
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
    display: grid;
    padding: 0;
    overflow: hidden;
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
