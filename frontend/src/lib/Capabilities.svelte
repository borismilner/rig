<!-- Capabilities: every command rig and its programs offer, in their own
     words, with a form to try one (plan/55 requirement 28).

     ⛔ BORIS, 2026-10-03: "A panel holds every capability of rig and of each
     registered program, each with its description taken from the binary and
     controls to try it: kind of Swagger UI but for our needs."

     The list is the running daemon's answer (RigService.Capabilities says
     why it is never this binary's), and a call goes out exactly as the
     "What goes on the wire" block shows it. Anything not declared read-only
     is confirmed before it is sent; a command whose effects were never
     declared counts as not read-only.

     ⛔ REQUIREMENT 35: a list he scrolls never jumps back to the top. The
     list is keyed and nothing re-creates it on a selection or a result, so
     its scroll position is the browser's to keep. -->
<script lang="ts">
  import { onMount } from "svelte";
  import * as RigService from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/rigservice.js";
  import type {
    Capability,
    TryResult,
  } from "../../bindings/github.com/borismilner/rig/cmd/rigwindow/models.js";

  interface Props {
    /* Seeded by the measurement fixture; null reads the daemon. */
    fixture: Capability[] | null;
    connected: boolean;
  }
  let { fixture, connected }: Props = $props();

  type Schema = {
    type?: string;
    description?: string;
    enum?: string[];
    properties?: Record<string, Schema>;
    required?: string[];
  };

  // svelte-ignore state_referenced_locally
  let caps: Capability[] = $state(fixture ?? []);
  let loadError = $state("");
  // svelte-ignore state_referenced_locally
  let loading = $state(!fixture);
  let q = $state("");
  let sel = $state("");
  // Per capability: the form's values, a pending confirm, and the last result.
  let values: Record<string, Record<string, unknown>> = $state({});
  let confirming = $state("");
  let busy = $state("");
  let results: Record<string, TryResult & { err?: string }> = $state({});

  const key = (c: Capability) => `${c.owner} ${c.id}`;

  async function load() {
    loading = true;
    loadError = "";
    try {
      caps = (await RigService.Capabilities()) ?? [];
    } catch (e) {
      loadError = String(e);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    if (!fixture) void load();
  });

  let shown = $derived(
    caps.filter(
      (c) =>
        !q ||
        [c.owner, c.id, c.title, c.summary, c.description]
          .join(" ")
          .toLowerCase()
          .includes(q.toLowerCase()),
    ),
  );

  // rig's verbs grouped by their first word (lease_acquire is "lease"),
  // each program's commands under its id.
  function groupOf(c: Capability): string {
    if (c.owner !== "rig") return c.owner;
    const i = c.id.indexOf("_");
    return i > 0 ? `rig ${c.id.slice(0, i)}` : "rig";
  }
  let groups = $derived.by(() => {
    const m = new Map<string, Capability[]>();
    for (const c of shown) {
      const g = groupOf(c);
      if (!m.has(g)) m.set(g, []);
      m.get(g)!.push(c);
    }
    return [...m.entries()];
  });

  let cur = $derived(
    caps.find((c) => key(c) === sel) ?? shown[0] ?? caps[0] ?? null,
  );
  let schema: Schema = $derived.by(() => {
    if (!cur?.args) return {};
    try {
      return JSON.parse(cur.args) as Schema;
    } catch {
      return {};
    }
  });
  let fields = $derived(Object.entries(schema.properties ?? {}));
  let vals = $derived(cur ? (values[key(cur)] ?? {}) : {});
  let missing = $derived(
    (schema.required ?? []).filter((r) => vals[r] === undefined),
  );
  let reads = $derived(cur?.effects === "read-only");

  // The same shape RigService.Try sends, so what is shown is what goes.
  let wire = $derived.by(() => {
    if (!cur) return "";
    const call =
      cur.owner === "rig"
        ? { tool: cur.id, arguments: vals }
        : {
            tool: "invoke",
            arguments: { program: cur.owner, command: cur.id, args: vals },
          };
    return JSON.stringify(call, null, 2);
  });

  function set(name: string, v: unknown) {
    if (!cur) return;
    const k = key(cur);
    const next = { ...(values[k] ?? {}) };
    if (v === undefined || v === "") delete next[name];
    else next[name] = v;
    values[k] = next;
    confirming = "";
  }

  // Free text for an object or an array is parsed as it is typed; the field
  // says when it is not yet valid rather than sending a string.
  let bad: Record<string, boolean> = $state({});
  function setJson(name: string, text: string) {
    if (!text.trim()) {
      bad[name] = false;
      set(name, undefined);
      return;
    }
    try {
      set(name, JSON.parse(text));
      bad[name] = false;
    } catch {
      bad[name] = true;
    }
  }

  async function send() {
    if (!cur || missing.length) return;
    const k = key(cur);
    if (!reads && confirming !== k) {
      confirming = k;
      return;
    }
    confirming = "";
    busy = k;
    try {
      results[k] = await RigService.Try(
        cur.owner,
        cur.id,
        JSON.stringify(vals),
      );
    } catch (e) {
      results[k] = { ok: false, text: "", call: wire, err: String(e) };
    } finally {
      busy = "";
    }
  }

  function pick(c: Capability) {
    sel = key(c);
    confirming = "";
  }

  // Up and down move through the list the way the rail does.
  function onlistkey(e: KeyboardEvent) {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    e.preventDefault();
    // From the button that has the keyboard, which is not always the one
    // selected: Tab can land on any of them.
    const from = (e.currentTarget as HTMLElement).dataset.key;
    const i = shown.findIndex((c) => key(c) === from);
    const n =
      shown[
        Math.max(
          0,
          Math.min(shown.length - 1, i + (e.key === "ArrowDown" ? 1 : -1)),
        )
      ];
    if (!n) return;
    pick(n);
    queueMicrotask(() =>
      document
        .querySelector<HTMLElement>(
          `.caplist [data-key="${CSS.escape(key(n))}"]`,
        )
        ?.focus(),
    );
  }

  let res = $derived(cur ? results[key(cur)] : undefined);
  let rigCount = $derived(caps.filter((c) => c.owner === "rig").length);
</script>

<div class="caps">
  <header class="head">
    <h1 class="t-sec">Capabilities</h1>
    <span class="dim"
      >{rigCount} of rig's own and {caps.length - rigCount} from registered programs,
      as the running daemon declares them</span
    >
    <span class="sp"></span>
    {#if !fixture}<button class="act" onclick={load} disabled={loading}
        >{loading ? "Reading…" : "Read again"}</button
      >{/if}
  </header>

  {#if loadError}
    <p class="empty">
      Rig did not answer the list: {loadError}
    </p>
  {:else if !connected && !fixture}
    <p class="empty">Rig is not answering, so there is nothing to list.</p>
  {:else if loading && caps.length === 0}
    <p class="empty">Reading what rig and its programs offer.</p>
  {:else}
    <div class="grid">
      <nav class="caplist" aria-label="Capabilities">
        <input
          type="search"
          class="inp"
          placeholder="Search {caps.length} capabilities"
          aria-label="Search capabilities"
          bind:value={q}
        />
        <div class="scroll">
          {#each groups as [g, cs] (g)}
            <h2>{g} <span class="dim">{cs.length}</span></h2>
            {#each cs as c (key(c))}
              <button
                data-key={key(c)}
                aria-current={!!cur && key(c) === key(cur)}
                onclick={() => pick(c)}
                onkeydown={onlistkey}
              >
                <i
                  data-effect={c.effects || "unsaid"}
                  title={c.effects || "effects not declared"}
                ></i>
                <span class="mono">{c.id}</span>
                <span class="dim">{c.summary}</span>
              </button>
            {/each}
          {:else}
            <p class="empty">Nothing matches.</p>
          {/each}
        </div>
      </nav>

      {#if cur}
        <section class="detail" aria-label={key(cur)}>
          <header>
            <h2 class="mono">
              {cur.owner === "rig" ? cur.id : `${cur.owner} ${cur.id}`}
            </h2>
            <span class="tag" data-effect={cur.effects || "unsaid"}
              >{cur.effects || "effects not declared"}</span
            >
          </header>
          <div class="body">
            {#if cur.title}<p class="ttl">{cur.title}</p>{/if}
            <p>
              {cur.description || cur.summary || "No description declared."}
            </p>
            {#if cur.returns}<p class="dim">
                <b>Returns:</b>
                {cur.returns}
              </p>{/if}

            <h3>Try it</h3>
            {#if fields.length === 0}
              <p class="dim">It takes no arguments.</p>
            {:else}
              <div class="args">
                {#each fields as [name, s] (`${key(cur)}/${name}`)}
                  {@const req = (schema.required ?? []).includes(name)}
                  {@const id = `arg-${name}`}
                  <label class="mono" for={id}
                    >{name}{#if req}<span class="req" title="required">
                        *</span
                      >{/if}</label
                  >
                  <span class="ctl">
                    {#if s.type === "boolean"}
                      <button
                        {id}
                        role="switch"
                        class="switch"
                        aria-checked={vals[name] === true}
                        onclick={() =>
                          set(name, vals[name] === true ? undefined : true)}
                        ><i></i>{vals[name] === true ? "On" : "Off"}</button
                      >
                    {:else if s.enum}
                      <select
                        {id}
                        class="inp"
                        value={(vals[name] as string) ?? ""}
                        onchange={(e) => set(name, e.currentTarget.value)}
                      >
                        <option value="">(none)</option>
                        {#each s.enum as o (o)}<option value={o}>{o}</option
                          >{/each}
                      </select>
                    {:else if s.type === "integer" || s.type === "number"}
                      <input
                        {id}
                        class="inp"
                        type="number"
                        step={s.type === "integer" ? 1 : "any"}
                        value={(vals[name] as number) ?? ""}
                        oninput={(e) => {
                          const v = e.currentTarget.valueAsNumber;
                          set(name, Number.isFinite(v) ? v : undefined);
                        }}
                      />
                    {:else if s.type === "array" || s.type === "object"}
                      <input
                        {id}
                        class="inp mono"
                        aria-invalid={bad[name] || undefined}
                        value={vals[name] !== undefined
                          ? JSON.stringify(vals[name])
                          : ""}
                        placeholder={s.type === "array"
                          ? '["a", "b"]'
                          : '{ "key": "value" }'}
                        oninput={(e) => setJson(name, e.currentTarget.value)}
                      />
                    {:else}
                      <input
                        {id}
                        class="inp"
                        value={(vals[name] as string) ?? ""}
                        oninput={(e) => set(name, e.currentTarget.value)}
                      />
                    {/if}
                  </span>
                  <span class="dim"
                    >{[s.type || "any", s.description]
                      .filter(Boolean)
                      .join(": ")}</span
                  >
                {/each}
              </div>
            {/if}

            <h4 class="dim">What goes on the wire</h4>
            <pre class="call">{wire}</pre>

            <div class="send">
              {#if confirming === key(cur)}
                <div class="confirm" role="alert">
                  <b
                    >{cur.effects === "destructive"
                      ? "This one destroys state."
                      : cur.effects
                        ? cur.effects === "changes state"
                          ? "This one changes state."
                          : `This one changes state (${cur.effects}).`
                        : "This one never said what it changes."}</b
                  >
                  Send it?
                  <button class="act primary" onclick={send}>Send it</button>
                  <button class="act" onclick={() => (confirming = "")}
                    >Cancel</button
                  >
                </div>
              {:else}
                <button
                  class="act primary"
                  disabled={missing.length > 0 || busy === key(cur)}
                  onclick={send}
                  >{busy === key(cur) ? "Waiting for rig…" : "Send"}</button
                >
                {#if missing.length}<span class="dim"
                    >{missing.join(", ")}
                    {missing.length > 1 ? "are" : "is"} required</span
                  >{/if}
              {/if}
            </div>

            {#if res}
              <h4 class="dim">
                {res.err
                  ? "It did not reach rig"
                  : res.ok
                    ? "Rig answered"
                    : "Rig refused"}
              </h4>
              <pre class="call" class:bad={!res.ok}>{res.err ||
                  res.text ||
                  "(an empty answer)"}</pre>
            {/if}
          </div>
        </section>
      {/if}
    </div>
  {/if}
</div>

<style>
  /* The GUI owns its scroll (requirement 37): the list and the detail each
     scroll on their own, and the header stays. */
  .caps {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    padding: calc(1rem * var(--den)) 1.15rem 0;
    box-sizing: border-box;
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: 0.9rem;
    flex-wrap: wrap;
    margin-bottom: 0.9rem;
    flex: none;
  }
  h1 {
    margin: 0;
    font-size: var(--fs-2);
  }
  .sp {
    flex: 1;
  }
  .dim {
    color: var(--fg-dim);
    font-size: var(--fs--1);
  }
  .mono {
    font-family: var(--mono);
  }
  .empty {
    color: var(--fg-dim);
    text-align: center;
    margin: 2.5rem auto;
    max-width: 62ch;
  }

  .grid {
    flex: 1;
    min-height: 0;
    display: grid;
    grid-template-columns: minmax(280px, 380px) minmax(0, 1fr);
    gap: 1rem;
    padding-bottom: 1rem;
  }

  .inp {
    font: inherit;
    font-size: var(--fs--1);
    color: var(--fg);
    background: var(--bg-2);
    border: 1px solid var(--border-2);
    border-radius: 7px;
    padding: 0.3rem 0.55rem;
    width: 100%;
    box-sizing: border-box;
  }
  .inp:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }
  .inp[aria-invalid="true"] {
    border-color: var(--sem-bad);
  }

  .caplist {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
    min-height: 0;
  }
  .caplist .scroll {
    flex: 1;
    min-height: 0;
    overflow: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 0.3rem;
  }
  .caplist h2 {
    margin: 0.6rem 0.4rem 0.2rem;
    font: 600 var(--fs--1) var(--sans);
    color: var(--fg-dim);
  }
  .caplist button {
    font: inherit;
    font-size: var(--fs--1);
    color: var(--fg);
    background: none;
    border: 0;
    border-radius: 6px;
    width: 100%;
    text-align: start;
    display: grid;
    grid-template-columns: 8px auto minmax(0, 1fr);
    align-items: center;
    gap: 0.5rem;
    padding: 0.3rem 0.45rem;
    cursor: pointer;
  }
  .caplist button .dim {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .caplist button:hover {
    background: var(--tint);
  }
  .caplist button[aria-current="true"] {
    background: var(--glow);
    box-shadow: inset 2px 0 0 var(--hue);
  }
  .caplist button:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }

  /* The effect as a dot in the list and a tag in the detail: green reads,
     amber changes, red destroys, hollow never said. */
  i[data-effect] {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--sem-warn);
  }
  i[data-effect="read-only"] {
    background: var(--sem-good);
  }
  i[data-effect="destructive"],
  i[data-effect="drives-input"] {
    background: var(--sem-bad);
  }
  i[data-effect="unsaid"] {
    background: transparent;
    border: 1.5px solid var(--fg-dim);
    box-sizing: border-box;
  }
  .tag[data-effect="read-only"] {
    color: var(--sem-good);
    border-color: var(--sem-good);
  }
  .tag[data-effect="destructive"],
  .tag[data-effect="drives-input"] {
    color: var(--sem-bad);
    border-color: var(--sem-bad);
  }
  .tag[data-effect="changes state"],
  .tag[data-effect="writes-files"],
  .tag[data-effect="network"] {
    color: var(--sem-warn);
    border-color: var(--sem-warn);
  }

  .detail {
    min-height: 0;
    display: flex;
    flex-direction: column;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-2);
  }
  .detail > header {
    display: flex;
    align-items: center;
    gap: 0.7rem;
    padding: 0.7rem 1rem;
    border-bottom: 1px solid var(--border);
    flex: none;
  }
  .detail h2 {
    margin: 0;
    font-size: var(--fs-1);
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 0.8rem 1rem 1.2rem;
  }
  .body p {
    margin: 0 0 0.6rem;
    max-width: 75ch;
  }
  .ttl {
    font-weight: 600;
  }
  h3 {
    margin: 1.2rem 0 0.6rem;
    font-size: var(--fs-0);
  }
  h4 {
    margin: 1rem 0 0.35rem;
    font-weight: 600;
  }

  .args {
    display: grid;
    grid-template-columns: max-content minmax(180px, 300px) minmax(0, 1fr);
    gap: 0.5rem 0.9rem;
    align-items: center;
    font-size: var(--fs--1);
  }
  .req {
    color: var(--sem-warn);
  }

  .switch {
    font: inherit;
    font-size: var(--fs--1);
    color: var(--fg);
    background: none;
    border: 1px solid var(--border-2);
    border-radius: 999px;
    padding: 0.15rem 0.6rem 0.15rem 0.2rem;
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    cursor: pointer;
  }
  .switch i {
    width: 26px;
    height: 16px;
    border-radius: 999px;
    background: var(--border-2);
    position: relative;
  }
  .switch i::after {
    content: "";
    position: absolute;
    top: 2px;
    left: 2px;
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: var(--panel);
    transition: left 0.12s;
  }
  .switch[aria-checked="true"] i {
    background: var(--h-steel);
  }
  .switch[aria-checked="true"] i::after {
    left: 12px;
  }
  .switch:focus-visible {
    outline: none;
    box-shadow: 0 0 0 var(--ring-w) var(--hue);
  }

  .call {
    margin: 0;
    padding: 0.6rem 0.8rem;
    font: var(--fs--1) / 1.5 var(--mono);
    background: var(--panel);
    border: 1px solid var(--border);
    border-radius: 8px;
    white-space: pre-wrap;
    overflow-wrap: anywhere;
    max-height: 22rem;
    overflow: auto;
  }
  .call.bad {
    border-color: var(--sem-bad);
  }

  .send {
    margin-top: 0.8rem;
    display: flex;
    align-items: center;
    gap: 0.7rem;
    min-height: 2.2rem;
  }
  .confirm {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    flex-wrap: wrap;
    padding: 0.4rem 0.7rem;
    border-left: 3px solid var(--sem-warn);
    background: var(--tint);
  }
</style>
