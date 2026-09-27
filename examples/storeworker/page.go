package main

// The page storeworker serves in its rig pane: one tab per part of rig
// (plan/48, R39). Every tab says in plain view what it shows, gives the
// controls to try it, shows the live state, and lists the rig calls that
// tab's part made, so each click can be followed into rig's verbs.
//
// It is the embedded tier, as cmd/lantern is: its own markup and CSS, and
// pane.js for the window's theme tokens. Everything the page renders from
// the server goes through esc() or textContent.
//
//nolint:misspell // `color` is a CSS property name, which is American by spec.
const page = `<!doctype html>
<meta charset="utf-8">
<title>storeworker</title>
<style>
  html:not([data-rig-painted]) { visibility: hidden }
  body { margin: 0; background: var(--panel, #fff); color: var(--fg, #111);
         font-family: var(--ui, system-ui), sans-serif; font-size: var(--fs-0, 14px) }
  header { padding: .75rem 1.25rem .5rem; border-bottom: 1px solid var(--border, #999) }
  header h1 { font-size: 1.2rem; margin: 0 }
  #strip { color: var(--fg-dim, #444); font-family: var(--mono, monospace); margin-top: .25rem }
  nav { display: flex; flex-wrap: wrap; gap: .25rem; padding: .5rem 1.25rem 0;
        border-bottom: 1px solid var(--border, #999) }
  nav button { border: 1px solid var(--border, #999); border-bottom: none;
               border-radius: 6px 6px 0 0; background: var(--bg, #f6f6f6) }
  nav button[aria-selected="true"] { background: var(--panel, #fff); font-weight: 600;
               border-color: var(--hue, var(--h-sage, green)) }
  main { padding: 1rem 1.25rem; max-width: 64rem }
  .what { border-left: 4px solid var(--hue, var(--h-sage, green)); padding: .25rem .75rem;
          margin: 0 0 1rem; background: var(--bg, #f6f6f6) }
  .what p { margin: .35rem 0 }
  .what b { font-weight: 600 }
  h2 { font-size: 1rem; margin: 1.25rem 0 .5rem }
  .try { display: flex; flex-wrap: wrap; gap: .5rem; align-items: center; margin: .25rem 0 }
  button { font: inherit; padding: .3rem .75rem; border-radius: 6px; cursor: pointer;
           border: 1px solid var(--border, #999); background: var(--bg, #f6f6f6);
           color: var(--fg, #111) }
  button.go { border-color: var(--hue, var(--h-sage, green)) }
  input, select, textarea { font: inherit; padding: .25rem .4rem; border-radius: 4px;
           border: 1px solid var(--border, #999); background: var(--panel, #fff);
           color: var(--fg, #111) }
  textarea { width: 30rem; height: 3.5rem }
  table { border-collapse: collapse; width: 100% }
  th, td { text-align: left; padding: .2rem .5rem; border-bottom: 1px solid var(--border, #ccc);
           vertical-align: top }
  th { color: var(--fg-dim, #444); font-weight: 600 }
  code, .mono { font-family: var(--mono, monospace) }
  .dim { color: var(--fg-dim, #444) }
  .bad { color: var(--h-rose, #a00) }
  #answer { font-family: var(--mono, monospace); white-space: pre-wrap; margin: .5rem 0;
            padding: .4rem .6rem; border: 1px dashed var(--border, #999); min-height: 1.2rem }
  .words { margin: 0 0 1rem; display: grid; grid-template-columns: max-content 1fr;
           gap: .15rem .75rem; font-size: .92em }
  .words dt { font-weight: 600; font-family: var(--mono, monospace) }
  .words dd { margin: 0; color: var(--fg-dim, #444) }
  .words-head { font-size: .9rem; margin: 0 0 .35rem }
  footer { margin-top: 1.5rem; border-top: 1px solid var(--border, #999); padding-top: .5rem }
  footer td { font-family: var(--mono, monospace); font-size: .9em }
</style>

<header>
  <h1>Storeworker <span class="dim">a fake program, one tab per part of rig</span></h1>
  <div id="strip">connecting</div>
</header>
<nav id="tabs" role="tablist"></nav>
<main>
  <section class="what" id="what"></section>
  <section id="words"></section>
  <div id="controls"></div>
  <div id="answer" aria-live="polite" class="dim">Answers from your clicks show here.</div>
  <div id="view"></div>
  <footer><h2>Rig calls for this tab <span class="dim">newest first</span></h2>
    <p class="dim">This view is read every 2 s with <span id="reads"></span>; those reads are not listed.</p>
    <div id="calls"></div></footer>
</main>

<script type="module">
import { rigPane } from "/rig/pane.js";

let theme = "not yet";
rigPane({ onTheme(mode) { theme = mode; } });

const esc = (v) => String(v ?? "").replace(/[&<>"']/g,
  (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
const $ = (id) => document.getElementById(id);
const table = (heads, rows) => rows.length === 0
  ? '<p class="dim">nothing yet</p>'
  : "<table>" + (heads.some((h) => h) ? "<tr>" + heads.map((h) => "<th>" + esc(h) + "</th>").join("") + "</tr>" : "") +
    rows.map((r) => "<tr>" + r.map((c) => "<td>" + c + "</td>").join("") + "</tr>").join("") + "</table>";
const err = (d, k) => d[k + "_error"] ? '<p class="bad">' + esc(d[k + "_error"]) + "</p>" : "";

// Every rig word the page uses, in plain terms (plan/48, R40). Each tab lists
// the ones it uses under its explanation; the last tab lists them all.
const WORDS = {
  "rig": "The coordination service this page shows. Programs and agent sessions connect to it to find each other, and to share storage, queues, leases, notifications and mail.",
  "rigd": "rig's background process, the daemon. Everything connects to it over a unix socket; the rig command at a terminal is just another client.",
  "estate": "One running rig with its own directory of state (yours is ~/.rig). There is one production estate and at most one development estate, and exactly one rigd per estate. rig estate names the one you are on.",
  "epoch": "A number rigd raises every time it starts. Leases and claims carry the epoch they were granted in, so one from before a restart is known to be stale.",
  "wire": "The message format programs speak to rigd. Its version (v1) is checked when a program says hello.",
  "connection": "One socket session with rigd. storeworker opens two: its own, which is registered, and one that acts as you, unregistered, the way a terminal is.",
  "program": "Software that registers with rig and declares commands. storeworker is one; it is fake, standing in for a real one such as graft.",
  "hello": "The first message a program sends rig: its declaration.",
  "declaration": "What a program tells rig about itself at hello: its id, its commands, their argument schemas and effects, and its pane.",
  "command": "An action a program declares. Anyone may call it through rig, at a terminal too: rig storeworker runs.",
  "verb": "An action rig itself offers, such as store.put or lease.acquire. Programs call verbs on rig; rig routes commands to programs.",
  "schema": "The declared shape of a command's arguments. rig checks every call against it before the program sees it.",
  "effects": "What a command says it does: read only, writes files, network, destructive, or drives input. A command that asks to confirm refuses unless the call says yes.",
  "routed": "rig passing a call to the program that declared the command, and the answer back to the caller.",
  "pane": "A web page a program offers for rig's window. This page is storeworker's pane.",
  "embedded tier": "A pane that brings its own HTML and takes only the window's colours and fonts, through pane.js.",
  "theme tokens": "The window's colours and fonts, handed to a pane as CSS variables so it matches the window.",
  "seat": "A named address in rig's roster, such as storeworker. A program or agent session takes one with announce. Mail goes to the seat, not the process, so it survives a restart.",
  "announce": "Taking a seat and saying what it is for.",
  "roster": "Every seat currently held on this estate, with its purpose and activity. The peers verb reads it.",
  "activity": "The one line a seat shows saying what it is doing right now.",
  "mail": "A durable message to a seat. It waits in the seat's inbox until read, even if nobody is in the seat.",
  "inbox": "A seat's unread mail.",
  "store": "rig's document database. Each program has its own, a SQLite file no other program can read or write.",
  "collection": "A named group of documents in a program's store, like a table. storeworker keeps runs and counts.",
  "document": "One JSON record in a collection, with an id and a version.",
  "version": "A count of the writes to one document. A write names the version it read.",
  "stale write": "A write naming an older version than the stored one. rig refuses it (CODE_CONFLICT), so two writers never silently overwrite each other.",
  "transact": "Several store writes applied together: all of them or none.",
  "query": "Finding documents by their fields. count_only answers with the number and no documents.",
  "queue": "A named list of tasks, handed out one at a time to whoever claims.",
  "task": "One item on a queue. Here, its payload is a run id.",
  "idempotency key": "A name given to a push. Pushing the same key twice still queues it only once, so a retry is safe.",
  "claim": "Taking the next task off a queue. A claim is a lease: if it is not renewed in time, the task goes back for someone else.",
  "lease": "A named right that one holder has at a time, for a limited time. Others wait or are refused. storeworker:gpu lets one run use the gpu at a time.",
  "ttl": "Time to live: how long a lease lasts unless it is renewed.",
  "renew": "Extending a lease before its ttl runs out, a heartbeat that says the holder is still alive.",
  "token": "A number that grows with every grant of a lease. A holder that lost the lease and comes back late carries an old token, so its late write can be refused.",
  "holder": "Who holds a lease right now.",
  "toast": "A notification rig's tray shows on the desktop.",
  "severity": "How loud a toast is: info, success, warning, error or urgent.",
  "tray": "rig's icon in the desktop panel. It draws the toasts.",
  "do not disturb": "When on, the tray holds every toast except urgent ones until it is turned off.",
  "free files": "Files a program writes itself, in a place rig chooses, rather than documents in its store. Transcripts, reports.",
  "kind": "A category of free file, such as a transcript. The layout maps each kind to a place.",
  "layout": "rig's map from each kind of free file to the folder it belongs in.",
  "index": "rig's search index over free files. files.index adds a file; files.unindexed lists files written or changed since.",
  "lesson": "A short shared note in rig's knowledge base, which every program and agent can search.",
  "export": "The store written out as text, one JSON document per line sorted by id, and committed to rig's exports git repository.",
  "import": "Putting a store back as it was at an export. The Restore button does this.",
  "snapshot": "A copy of the store rig takes before an import replaces it, so nothing is lost for good.",
  "supervised": "Started by rig with rig up, from programs.json. rig restarts it after a crash or a reboot, and reads its health reports.",
  "health report": "What a supervised program tells rig: its marker, what it is waiting on, and any question it is parked on. rig health shows them.",
  "marker": "A number that moves as the program makes progress. If it stops moving, rig can tell stuck from busy.",
  "parked": "Waiting on a person's answer. rig health shows the program as PARKED, with the question.",
  "run": "storeworker's own word, not rig's: one fake assignment it works through, step by step.",
  "budget": "storeworker's own word: the cost above which a run waits for your yes before it starts.",
  "worker": "storeworker's own word: its loop that claims runs from the queue and works them one at a time.",
};

// The parts of rig a program does not use, so no tab can show them.
const ELSEWHERE = [
  ["the window", "rig's desktop app, which shows the panes of every program, this one included."],
  ["the rig command", "the terminal client: rig health, rig up, rig estate, and every program's commands."],
  ["the continuity record", "the project memory agent sessions keep in rig: records, the links between them, work notes and progress, so a later session picks the work up."],
  ["MCP", "how an agent session reaches rig: every verb on this page is also an MCP tool."],
  ["backups", "rig backup copies the whole estate, and restore puts one back."],
];

const wordsOn = (terms) => '<h2 class="words-head">Words on this tab</h2><dl class="words">' +
  terms.map((w) => "<dt>" + esc(w) + "</dt><dd>" + esc(WORDS[w] ?? "not explained yet") + "</dd>").join("") + "</dl>";

const TABS = [
  { id: "program", terms: ["rig", "rigd", "estate", "program", "hello", "declaration", "command", "schema", "effects", "routed", "connection", "wire", "pane", "embedded tier", "theme tokens", "seat", "verb"], reads: "nothing, the commands are its own declaration", name: "The program", verbs: ["routed in", "storeworker."],
    what: "<p><b>Registering a program with rig.</b> On start storeworker said hello with a declaration: its identity, its five commands with their effects, and a pane URL. That URL is this page: the <b>embedded tier</b>, its own HTML, taking only the window's theme tokens through pane.js.</p>" +
      "<p><b>Try it:</b> call one of its commands <b>through rig</b>. Your call leaves on a second connection, rig checks the arguments against the schema the program declared, routes the call back here, and the footer shows it arrive as <i>routed in</i>. Empty the arguments to see rig refuse them before storeworker ever sees them. The same line works at a terminal.</p>",
    controls: '<div class="try"><select id="cmd"></select><input id="args" size="50" value="{}" aria-label="arguments as JSON"><button class="go" data-do="invoke">Call through rig</button></div>',
    render(d) {
      const sel = $("cmd");
      if (sel && sel.options.length === 0) {
        for (const c of d.commands || []) sel.add(new Option(c.id, c.id));
        const ex = { assign: '{"title":"from the program tab","prompt":"three lines"}', runs: '{"state":"done"}', answer: '{"run":"ID","yes":true}' };
        sel.onchange = () => { $("args").value = ex[sel.value] || "{}"; };
        sel.onchange();
      }
      const unthemed = document.documentElement.hasAttribute("data-rig-unthemed");
      return "<p>Theme from the window: <b>" + esc(unthemed && theme === "not yet" ? "none, this page is open outside rig's window" : theme) + "</b>. Seat <code>" + esc(d.seat) + "</code>, program <code>" + esc(d.program) + "</code>.</p>" +
        table(["command", "what it does", "effects", "at a terminal"], (d.commands || []).map((c) =>
          ["<code>" + esc(c.id) + "</code>", esc(c.summary), esc(c.effects) + (c.confirms ? ", asks to confirm" : ""), "<code>" + esc(c.example) + "</code>"]));
    } },
  { id: "store", terms: ["store", "collection", "document", "version", "stale write", "transact", "query", "run"], reads: "store.query (the runs and a count_only per state), store.collections", name: "Store", verbs: ["store."],
    what: "<p><b>rig's document store.</b> Each program gets its own SQLite database behind <code>store.*</code>. Every document has a version; a write names the version it read, and rig refuses it if someone wrote in between.</p>" +
      "<p><b>Try it:</b> assign a run (one <code>store.transact</code> writes the run and the day's count together), filter by state (<code>store.query</code>), and press <i>stale write</i> on a row to see rig refuse a write at an old version.</p>",
    controls: '<div class="try"><input id="title" placeholder="title" value="summarise the inbox"><input id="prompt" placeholder="prompt" value="three lines"><button class="go" data-do="assign">Assign a run</button></div>' +
      '<div class="try">Show: <select id="state"><option value="">every state</option><option>queued</option><option>running</option><option>waiting</option><option>done</option><option>failed</option><option>denied</option></select></div>',
    query: () => "state=" + encodeURIComponent($("state")?.value || ""),
    render(d) {
      const r = d.runs || {};
      const counts = Object.entries(r.counts || {}).map(([k, v]) => esc(k) + " " + esc(v)).join(", ");
      return err(d, "runs") + "<p>Counts (<code>count_only</code> queries): " + counts + "</p>" +
        table(["run", "title", "state", "version", "cost", ""], (r.runs || []).map((x) =>
          ["<code>" + esc(x.id) + "</code>", esc(x.title), esc(x.state), esc(x.version), "$" + Number(x.cost).toFixed(2),
           '<button data-do="stale-write" data-run="' + esc(x.id) + '">stale write</button>'])) +
        "<h2>Collections</h2>" + err(d, "collections") +
        table(["collection", "documents"], (d.collections || []).map((c) => [esc(c.name), esc(c.documents)]));
    } },
  { id: "queue", terms: ["queue", "task", "idempotency key", "claim", "lease", "renew", "ttl", "worker", "run"], reads: "queue.list", name: "Queue", verbs: ["queue.", "lease.renew"],
    what: "<p><b>rig's work queue.</b> Assigning pushes the run id with an idempotency key, so a retried push never queues it twice. The worker claims one task at a time; a claim is a lease it keeps alive with <code>lease.renew</code>, and a claim not renewed goes back to the queue.</p>" +
      "<p><b>Try it:</b> queue three quick runs and watch them go from queued to claimed to done, one at a time.</p>",
    controls: '<div class="try"><button class="go" data-do="assign-three">Queue three quick runs</button></div>',
    render(d) {
      return "<p>Queue <code>" + esc(d.queue) + "</code>, " + esc(d.done ?? 0) + " done. Worker's claim: <code>" + esc(d.worker.claim || "none") + "</code></p>" + err(d, "tasks") +
        table(["task", "run", "state", "attempts", "held by"], (d.tasks || []).map((t) =>
          ["<code>" + esc(t.id) + "</code>", "<code>" + esc(t.key) + "</code>", esc(t.state), esc(t.attempts), esc(t.holder)]));
    } },
  { id: "leases", terms: ["lease", "holder", "ttl", "token", "epoch", "claim", "worker"], reads: "lease.list", name: "Leases", verbs: ["lease."],
    what: "<p><b>rig's leases.</b> A named lease is held by one holder at a time, with a ttl and a token that grows on every grant. The worker takes <code>storeworker:gpu</code> for each run, so only one run uses the gpu across every worker and anyone else who asks.</p>" +
      "<p><b>Try it:</b> hold the gpu lease <b>yourself</b>, then assign a run in the Store tab. The worker waits for you and says so; release it and the run goes on.</p>",
    controls: '<div class="try">Hold for <input id="secs" type="number" min="5" max="120" value="30" style="width:4rem"> s <button class="go" data-do="gpu-hold">Hold the gpu lease as you</button><button data-do="gpu-release">Release it</button><button data-do="assign">Assign a run</button></div>',
    render(d) {
      return "<p>You hold it: <b>" + (d.you_hold ? "yes" : "no") + "</b>. The worker: " + esc(d.worker.activity) + "</p>" + err(d, "leases") +
        table(["lease", "state", "holder", "token", "left"], (d.leases || [])
          .filter((l) => l.state !== "free" || !l.name.startsWith("queue/"))
          .map((l) => ["<code>" + esc(l.name) + "</code>", esc(l.state), esc(l.holder), esc(l.token),
            l.state === "free" ? "" : Math.round((l.remaining_ms || 0) / 1000) + " s"])) +
        '<p class="dim">A queue claim is a lease too: <code>queue/...</code> rows are the worker\'s claims, hidden once done.</p>';
    } },
  { id: "asking", terms: ["run", "budget", "parked", "toast", "severity", "health report", "document"], reads: "store.query for state eq waiting", name: "Asking you", verbs: ["health.report", "notify", "store.put"],
    what: "<p><b>A program asking its user, with what rig has today.</b> A run costing more than the budget parks: it is stored as <i>waiting</i>, an <b>urgent toast</b> reaches the tray, and <code>health.report</code> carries the question, so <code>rig health</code> shows it as PARKED.</p>" +
      "<p><b>Try it:</b> assign an expensive run, watch the toast, then answer here, or at a terminal with the line in the toast. rig has no reply-to-a-toast yet, so the answer comes back through this program's own command.</p>",
    controls: '<div class="try"><button class="go" data-do="assign-expensive">Assign an expensive run</button></div>',
    render(d) {
      const w = (d.waiting || {}).runs || [];
      return "<p>Budget $" + Number(d.budget).toFixed(2) + ". Parked now: " + esc(d.worker.parked || "nothing") + "</p>" + err(d, "waiting") +
        table(["run", "question", "your answer"], w.map((x) =>
          ["<code>" + esc(x.id) + "</code>", esc(x.question),
           '<button class="go" data-do="answer" data-run="' + esc(x.id) + '" data-yes="1">Yes, run it</button> <button data-do="answer" data-run="' + esc(x.id) + '">No</button>'])) +
        "<p class=\"dim\">At a terminal: <code>rig storeworker answer --args '{\"run\":\"ID\",\"yes\":true}'</code></p>";
    } },
  { id: "toasts", terms: ["toast", "severity", "tray", "do not disturb"], reads: "toast.dnd, as a query", name: "Toasts", verbs: ["notify", "toast."],
    what: "<p><b>rig's tray notifications.</b> A program files a toast with a severity; the tray draws it, and Do Not Disturb holds all but urgent. storeworker toasts when a run is queued, done, failed, or needs you.</p>" +
      "<p><b>Try it:</b> send one of each severity and watch the tray.</p>",
    controls: '<div class="try">' + ["info", "success", "warning", "error", "urgent"].map((s) =>
      '<button data-do="toast" data-sev="' + s + '">' + s + "</button>").join("") + "</div>",
    render(d) {
      const dnd = d.dnd ? (d.dnd.on ? "on, " + esc(d.dnd.suppressed) + " held" : "off") : "unknown";
      return "<p>Do Not Disturb (<code>toast.dnd</code> query): <b>" + dnd + "</b></p>" + err(d, "dnd") +
        table(["at", "severity", "title"], (d.toasts || []).slice().reverse().map((t) => [esc(t.at), esc(t.severity), esc(t.title)]));
    } },
  { id: "files", terms: ["free files", "kind", "layout", "index", "run"], reads: "files.search, files.unindexed, files.layout", name: "Files", verbs: ["files."],
    what: "<p><b>rig's free files.</b> A program asks <code>files.place</code> where a file of a kind goes, writes it there itself, and indexes it with <code>files.index</code> so a search by anyone finds it. Each run's transcript lands this way.</p>" +
      "<p><b>Try it:</b> search the index. After a run or two, its transcript is a hit.</p>",
    controls: '<div class="try"><input id="q" value="storeworker" aria-label="search words"><button class="go" data-do="refresh">Search</button></div>',
    query: () => "q=" + encodeURIComponent($("q")?.value || ""),
    render(d) {
      return "<h2>Hits (<code>files.search</code>)</h2>" + err(d, "hits") +
        table(["path", "title", "snippet"], (d.hits || []).map((h) => ["<code>" + esc(h.path) + "</code>", esc(h.title), esc(h.snippet)])) +
        "<h2>Written but not indexed (<code>files.unindexed</code>)</h2>" + err(d, "unindexed") +
        table(["path", "state"], (d.unindexed || []).map((f) => ["<code>" + esc(f.path) + "</code>", esc(f.state)])) +
        "<h2>Where each kind goes (<code>files.layout</code>)</h2>" + err(d, "layout") +
        table(["kind", "place"], (d.layout || []).map((k) => [esc(k.name), "<code>" + esc(k.place) + "</code>"]));
    } },
  { id: "lessons", terms: ["lesson", "run"], reads: "knowledge.search", name: "Lessons", verbs: ["knowledge."],
    what: "<p><b>rig's shared lessons.</b> What one program learns, every program and agent can search. A run whose prompt says <i>fail</i> fails, and the worker records a lesson about it with <code>knowledge.add</code>.</p>" +
      "<p><b>Try it:</b> assign a failing run and search for it, or add a lesson of your own.</p>",
    controls: '<div class="try"><button class="go" data-do="assign-failing">Assign a failing run</button><input id="q" value="storeworker" aria-label="search words"><button data-do="refresh">Search</button></div>' +
      '<div class="try"><input id="ltitle" placeholder="lesson title" size="30"><input id="lsummary" placeholder="one line" size="40"><button data-do="lesson">Add a lesson</button></div>',
    query: () => "q=" + encodeURIComponent($("q")?.value || ""),
    render(d) {
      return err(d, "hits") + table(["title", "summary", "snippet"], (d.hits || []).map((h) => [esc(h.title), esc(h.summary), esc(h.snippet)]));
    } },
  { id: "mail", terms: ["seat", "announce", "roster", "activity", "mail", "inbox"], reads: "peers, message.inbox", name: "Mail", verbs: ["message.", "peers", "announce"],
    what: "<p><b>rig's roster and mail.</b> The worker took a seat with <code>announce</code>; <code>peers</code> lists every seat on this rig. Mail to a seat is durable: it waits in the inbox until read. A run assigned with a <i>reply to</i> seat mails that seat when it ends.</p>" +
      "<p><b>Try it:</b> send mail to a seat, including storeworker's own, and read its inbox below.</p>",
    controls: '<div class="try">To <input id="to" size="16"> <input id="subject" placeholder="subject" value="hello from the Mail tab"><input id="body" placeholder="body" value="Sent by you, through rig." size="30"><button class="go" data-do="mail">Send</button></div>',
    render(d) {
      if ($("to") && !$("to").value) $("to").value = d.seat;
      return "<h2>Seats (<code>peers</code>)</h2>" + err(d, "peers") +
        table(["seat", "purpose", "state", "doing"], (d.peers || []).map((p) => ["<code>" + esc(p.seat) + "</code>", esc(p.purpose), esc(p.state), esc(p.activity)])) +
        "<h2>" + esc(d.seat) + "'s inbox (<code>message.inbox</code>)</h2>" + err(d, "inbox") +
        table(["at", "from", "subject", "body"], (d.inbox || []).map((m) => [esc(m.sent), esc(m.from), esc(m.subject), esc(m.body)]));
    } },
  { id: "export", terms: ["export", "import", "snapshot", "collection", "document", "version"], reads: "the export's own runs.jsonl, read from disk", name: "Export", verbs: ["store.export", "store.import"],
    what: "<p><b>The store as text in git.</b> <code>store.export</code> writes each collection as one JSON line per document, sorted by id, and commits it to rig's exports repository. <code>store.import</code> puts the store back as it was at the export, after rig snapshots the store it replaces.</p>" +
      "<p><b>Try it:</b> export, assign a run, then restore: the new run is gone and the snapshot keeps it.</p>",
    controls: '<div class="try"><button class="go" data-do="export">Export</button><button data-do="assign">Assign a run</button><button id="restore" data-do="restore">Restore from the export</button></div>',
    render(d) {
      const e = d.export || {};
      return table(["", ""], [["note", esc(e.note || "not exported yet")], ["commit", "<code>" + esc(e.commit) + "</code>"],
        ["directory", "<code>" + esc(e.dir) + "</code>"], ["snapshot before the restore", "<code>" + esc(e.snapshot) + "</code>"]]) +
        "<h2>runs.jsonl, first lines</h2><pre class=\"mono\">" + esc((d.preview || []).join("\n")) + "</pre>";
    } },
  { id: "health", terms: ["supervised", "health report", "marker", "parked", "activity", "worker"], reads: "nothing from rig, the worker's own state", name: "Health", verbs: ["health.report", "activity", "announce"],
    what: "<p><b>rig's supervision.</b> A program rig started with <code>rig up</code> reports its health: a marker that moves as it makes progress, what it waits on, and a question it is parked on. <code>rig health</code> reads them, and a marker that stops moving is how rig tells stuck from busy.</p>" +
      "<p><b>Try it:</b> run <code>rig health</code> at a terminal while a run goes through. Started by hand, rig refuses the reports, and this tab says so.</p>",
    controls: "",
    render(d) {
      const w = d.worker || {};
      return table(["", ""], [["marker (runs finished since it started)", esc(w.done)], ["activity", esc(w.activity)], ["parked on", esc(w.parked || "nothing")],
        ["health reports", d.health ? '<span class="bad">' + esc(d.health) + "</span>" : "accepted"]]) +
        "<h2>To run it supervised</h2><p>Add it to <code>programs.json</code> under <code>$XDG_CONFIG_HOME/rig/</code>, then <code>rig up " + esc(d.program) + "</code>:</p>" +
        '<pre class="mono">{"programs": [{"id": "' + esc(d.program) + '", "path": "/full/path/to/storeworker"}]}</pre>';
    } },
  { id: "words", reads: "nothing, the words are part of the page", name: "Every word", verbs: [], terms: [],
    what: "<p><b>Every rig word this page uses, in one place.</b> Each tab also lists the ones it uses, under its explanation. Words marked as storeworker's own belong to this fake program, not to rig.</p>" +
      "<p>Below them, the parts of rig a program does not use, so no tab can show them.</p>",
    controls: "",
    render() {
      const where = (w) => TABS.filter((t) => t.terms.includes(w)).map((t) => t.name).join(", ");
      return table(["word", "what it means", "shown on"], Object.keys(WORDS).sort().map((w) =>
          ["<b>" + esc(w) + "</b>", esc(WORDS[w]), esc(where(w))])) +
        "<h2>Parts of rig not shown here</h2>" +
        table(["part", "what it is"], ELSEWHERE.map(([n, d]) => ["<b>" + esc(n) + "</b>", esc(d)]));
    } },
];

let current = TABS[0];
let busy = false;

function drawTabs() {
  $("tabs").innerHTML = TABS.map((t) => '<button role="tab" data-tab="' + t.id + '" aria-selected="' + (t === current) + '">' + esc(t.name) + "</button>").join("");
}

function show(t) {
  current = t;
  drawTabs();
  $("what").innerHTML = t.what;
  $("words").innerHTML = t.terms.length ? wordsOn(t.terms) : "";
  $("controls").innerHTML = t.controls;
  $("reads").textContent = t.reads;
  $("view").innerHTML = "";
  $("answer").textContent = "Answers from your clicks show here.";
  $("answer").className = "dim";
  $("answer").hidden = !t.controls; // nothing to click, nothing to answer
  document.querySelector("footer").hidden = t.verbs.length === 0;
  refresh();
}

async function api(path, init) {
  const r = await fetch(path, { ...init, headers: { "X-Storeworker": "1", "content-type": "application/json" } });
  if (!r.ok) throw new Error(r.status + " " + (await r.text()));
  return r.json();
}

async function refresh() {
  if (busy) return;
  busy = true;
  const t = current;
  try {
    const d = await api("/api/tab/" + t.id + "?" + (t.query ? t.query() : ""));
    if (t !== current) return;
    if (d.error) { $("view").innerHTML = '<p class="bad">' + esc(d.error) + "</p>"; return; }
    $("strip").textContent = "worker: " + (d.worker.activity || "starting") + "   |   runs finished: " + d.worker.done;
    $("view").innerHTML = t.render(d);
    const mine = (d.calls || []).filter((c) => t.verbs.some((v) => c.verb.startsWith(v))).reverse().slice(0, 12);
    $("calls").innerHTML = table(["at", "verb", "what"], mine.map((c) =>
      [esc(c.at), "<b>" + esc(c.verb) + "</b>", (c.failed ? '<span class="bad">' : "<span>") + esc(c.note) + "</span>"]));
  } catch (e) {
    $("strip").textContent = "cannot reach storeworker: " + e.message;
  } finally {
    busy = false;
  }
}

function args(b) {
  const v = (id) => $(id)?.value || "";
  switch (b.dataset.do) {
    case "assign": return { Title: v("title") || "a run from the " + current.name + " tab", Prompt: v("prompt") };
    case "answer": return { Run: b.dataset.run, Yes: b.dataset.yes === "1" };
    case "stale-write": return { Run: b.dataset.run };
    case "gpu-hold": return { Seconds: Number(v("secs")) };
    case "toast": return { Severity: b.dataset.sev };
    case "mail": return { To: v("to"), Subject: v("subject"), Body: v("body") };
    case "lesson": return { Title: v("ltitle"), Summary: v("lsummary"), Body: v("lsummary") };
    case "invoke": return { Command: v("cmd"), Args: v("args") };
  }
  return {};
}

document.addEventListener("click", async (ev) => {
  const b = ev.target.closest("button");
  if (!b) return;
  if (b.dataset.tab) { show(TABS.find((t) => t.id === b.dataset.tab)); return; }
  const act = b.dataset.do;
  if (!act) return;
  if (act === "refresh") { refresh(); return; }
  // Restore replaces the store, so it takes a second press, in place.
  if (act === "restore" && b.dataset.armed !== "1") {
    b.dataset.armed = "1";
    b.textContent = "Press again to replace the store";
    setTimeout(() => { b.dataset.armed = ""; b.textContent = "Restore from the export"; }, 4000);
    return;
  }
  const out = $("answer");
  out.className = "";
  out.textContent = act + " ...";
  try {
    const d = await api("/api/do/" + act, { method: "POST", body: JSON.stringify(args(b)) });
    out.className = d.error ? "bad" : "";
    out.textContent = d.error ? "refused: " + d.error : JSON.stringify(d, null, 1);
  } catch (e) {
    out.className = "bad";
    out.textContent = e.message;
  }
  refresh();
});

show(TABS[0]);
// A hidden pane reads nothing: no rig calls while nobody is looking.
setInterval(() => { if (!document.hidden) refresh(); }, 2000);
document.addEventListener("visibilitychange", () => { if (!document.hidden) refresh(); });
</script>
`
