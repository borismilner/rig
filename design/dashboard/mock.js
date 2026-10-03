// The management dashboard, as a mockup Boris tries in his own browser
// (plan/55). Nothing here talks to rigd: a simulation stands in for the
// programs and agents that will call rig.panel.put, and every push is shown
// on the wire drawer as the approved path would carry it.
import { DEFAULTS, tokens } from '../theme.js';
import { rigCss, guiDoc } from './rig-kit.js';
// Ledger's GUI page, ledger-gui.html, put in here by build.py.
const LEDGER_GUI = __LEDGER_GUI__;
// rig's own commands as it declares them (internal/daemon/self.go), dumped
// by dump-self.sh; and its config schema (internal/config/schema.json).
const RIG_SELF = __RIG_SELF__;
const RIG_SCHEMA = __RIG_SCHEMA__;

// ── theme ──────────────────────────────────────────────────────────────────
function applyTheme() {
  const mode = document.documentElement.getAttribute('data-theme') === 'light' ? 'light' : 'dark';
  const t = tokens(DEFAULTS, mode);
  for (const [k, v] of Object.entries(t)) document.documentElement.style.setProperty(k, v);
  // A registered GUI follows the dashboard's theme.
  for (const f of document.querySelectorAll('#guihost iframe')) f.contentWindow?.postMessage({ type: 'theme', data: mode }, '*');
}
applyTheme();
new MutationObserver(applyTheme).observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme'] });

// ── small DOM helper: text always goes in as text ──────────────────────────
function h(tag, attrs, ...kids) {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === undefined || v === null || v === false) continue;
    if (k === 'class') n.className = v;
    else if (k === 'style') for (const [p, x] of Object.entries(v)) n.style.setProperty(p, x);
    else if (k.startsWith('on')) n.addEventListener(k.slice(2), v);
    else n.setAttribute(k, v === true ? '' : v);
  }
  for (const k of kids.flat()) if (k !== null && k !== undefined && k !== false) n.append(k instanceof Node ? k : String(k));
  return n;
}
const svg = (inner) => {
  const s = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  s.setAttribute('viewBox', '0 0 24 24'); s.setAttribute('fill', 'none'); s.setAttribute('stroke', 'currentColor');
  s.setAttribute('stroke-width', '2'); s.setAttribute('stroke-linecap', 'round'); s.setAttribute('stroke-linejoin', 'round');
  s.setAttribute('aria-hidden', 'true'); s.innerHTML = inner; return s;
};
const ICON = {
  queued: '<circle cx="12" cy="12" r="10"/><path d="M12 6v6l4 2"/>',
  running: '<path d="M21 12a9 9 0 1 1-6.22-8.56"/>',
  done: '<path d="M20 6 9 17l-5-5"/>',
  failed: '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
  waiting: '<path d="M12 8v4"/><path d="M12 16h.01"/><circle cx="12" cy="12" r="10"/>',
  min: '<path d="m6 9 6 6 6-6"/>', max: '<path d="m6 15 6-6 6 6"/>',
  wire: '<path d="M4 12h4l3-8 4 16 3-8h2"/>', reopen: '<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/>',
  play: '<path d="m7 4 13 8-13 8z"/>', pause: '<path d="M8 4v16M16 4v16"/>', step: '<path d="m5 4 10 8-10 8z"/><path d="M19 5v14"/>',
  q: '<circle cx="12" cy="12" r="10"/><path d="M9.1 9a3 3 0 0 1 5.8 1c0 2-3 3-3 3"/><path d="M12 17h.01"/>',
  sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
  x: '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
  folder: '<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>',
  file: '<path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V9z"/><path d="M14 3v6h6"/>',
};
const SEV = { info: 'var(--h-steel)', success: 'var(--h-sage)', warning: 'var(--h-amber)', error: 'var(--h-rust)' };
const RANK = { error: 4, warning: 3, success: 2, info: 1 };
const STATUS = {
  queued: { icon: 'queued', tone: 'var(--fg-dim)' },
  running: { icon: 'running', tone: 'var(--h-teal)', spin: true },
  waiting: { icon: 'waiting', tone: 'var(--h-amber)' },
  done: { icon: 'done', tone: 'var(--h-sage)' },
  failed: { icon: 'failed', tone: 'var(--h-rust)' },
};
const OPEN_CAP = 200;   // open cards rigd holds in memory (plan/55)
const BODY_CAP = 4096;  // bytes a card body may carry

// ── the estate and the writers ─────────────────────────────────────────────
const WRITERS = {
  'rig-lead': { kind: 'agent', project: 'rig' },
  'beacon-seat': { kind: 'agent', project: 'rigged' },
  'storeworker': { kind: 'agent', project: 'rig' },
  'ledger': { kind: 'program', project: 'ledger' },
  'beacon': { kind: 'program', project: 'rigged' },
};
// Every program rig represents, the scanned ones included (requirement 6).
// Mock values: the fields are the ones rigd already holds or plan/54 added.
const RIG = { version: 'v0.54.0-14-gfc28a4c', estate: 'production', up: '2 h 14 min', epoch: 108,
  scan: '~/.local/lib/rig/apps', kept: 6, seats: 3, store: '182 MB of 500 MB' };
const STATE = {
  healthy: 'var(--h-sage)', 'at rest': 'var(--fg-dim)', running: 'var(--h-teal)',
  down: 'var(--h-amber)', quarantined: 'var(--h-rust)', starting: 'var(--h-teal)',
};
const estate = [
  { id: 'beacon', name: 'Beacon', version: '0.3.1', load: 'on call', state: 'at rest', since: '12 min', found: 'scan', path: '~/.local/lib/rig/apps/beacon',
    coverage: 'partial', note: 'cards and the board; no config', commands: [['ask', 'ask the user a question on a card'], ['board.put', 'add or change a card'], ['notify', 'a one-line notice']],
    restarts: 0, calls: 14, lastExit: 'exit 0, idle', binary: '2049:1311742:9.8 MB:10:41', read: '10:41, kept', gui: false },
  { id: 'ledger', name: 'Ledger', version: '1.2.0', load: 'resident', state: 'healthy', since: '2 h', found: 'programs.json', path: '~/.local/bin/ledger',
    coverage: 'full', note: 'everything but streams', commands: [['reconcile', 'match entries against the bank'], ['match', 'match an entry to its bank line'], ['reject', 'mark an entry not a match'], ['entries', 'list entries'], ['export', 'write a CSV']],
    restarts: 0, binary: '2049:1310012:10.2 MB:08:39', read: '08:39, at its start', gui: true },
  { id: 'lantern', name: 'Lantern', version: '0.8.0', load: 'on call', state: 'running', since: '30 s', found: 'scan', path: '~/.local/lib/rig/apps/lantern',
    coverage: 'partial', note: 'search only', commands: [['search', 'full-text search'], ['index', 'reindex a folder']],
    restarts: 0, calls: 1, binary: '2049:1311790:9.3 MB:09:02', read: '09:02, kept', gui: false },
  { id: 'righand', name: 'righand', version: '0.9.4', load: 'resident', state: 'healthy', since: '2 h', found: 'programs.json', path: '~/.local/bin/righand',
    coverage: 'partial', note: 'scripts, windows and where', commands: [['script', 'run a desktop script'], ['windows', 'list windows'], ['where', 'locate a window']],
    restarts: 1, binary: '2049:1310455:7.1 MB:08:39', read: '08:39, at its start', gui: false },
  { id: 'storeworker', name: 'storeworker', version: '0.4.0', load: 'resident', state: 'healthy', since: '41 min', found: 'programs.json', path: '~/.local/bin/storeworker',
    coverage: 'partial', note: 'a demonstration: store, queue, leases', commands: [['run', 'run a queued job'], ['status', 'what it is doing']],
    restarts: 2, binary: '2049:1310460:8.0 MB:10:12', read: '10:12, at a restart', gui: false },
  { id: 'keeper', name: 'keeper', version: 'v3', load: 'resident', state: 'down', since: '20 min', found: 'scan', path: '~/.local/lib/rig/apps/keeper',
    coverage: 'partial', note: 'the wire only', commands: [['greet', 'say hello']],
    restarts: 0, lastExit: 'stopped by a human', binary: '2049:1311802:6.4 MB:10:33', read: '10:33, a declare run', gui: false,
    hint: 'A call is refused with: rig up keeper' },
  { id: 'abacus', name: 'abacus', version: '2.0.1', load: 'resident', state: 'quarantined', stale: true, since: '1 h', found: 'scan', path: '~/.local/lib/rig/apps/abacus',
    coverage: 'partial', note: 'sums and rates', commands: [['sum', 'add a column'], ['rate', 'convert a currency']],
    restarts: 5, lastExit: 'exit 2, five times in 4 min', binary: '2049:1311655:7.7 MB:09:47', read: '09:20, before its rebuild', gui: false,
    hint: 'Its binary changed after it was quarantined: what is listed is stale until rig restart abacus' },
];

// §11 requirement 17: an entry carries a colour, so it is told apart at a glance.
const PROG_HUES = ['--h-steel', '--h-sage', '--h-amber', '--h-teal', '--h-indigo', '--h-rust'];
function progColour(id) { return `var(${PROG_HUES[estate.findIndex((e) => e.id === id) % PROG_HUES.length]})`; }

// ── state ──────────────────────────────────────────────────────────────────
const S = {
  now: Date.now(),
  cards: new Map(),      // id -> card, with every version kept whole
  order: 0,
  dismissed: new Set(),  // sections the user took off
  minimised: false, unseen: [],
  q: '', sev: new Set(), status: new Set(), group: 'writer', since: 'all',
  tabs: [{ id: 'main', title: 'Main', kind: 'main', open: true }],
  current: 'main',
  // A browser under automation (the contrast gate) starts paused, so the
  // page it measures is not redrawn under it.
  playing: !navigator.webdriver, step: 0,
  wire: [], showWire: false,
  history: null,         // { id, at } while the drawer shows a card
  open: null,            // the program row being inspected
  notes: [],             // notifications, newest first
  keepDays: 7,           // requirement 8: evicted past this age; a setting
  nq: '', nsrc: '',      // requirement 9: search, and one source
  evicted: 0,
  guis: {},              // requirement 10: program id -> its registered GUI
  mainTab: 'needs',      // requirement 16: Main's own tabs; 24: Needs you first
  wq: '', wireAll: false, // requirement 18: wire search; 21: one program's wire
  bsrc: '',              // requirement 27: the board's one source
  sq: '', setTab: 'rig', typePath: null, cq: '', cap: 'notify', capArgs: {}, capOut: {}, capConfirm: null, // 19, 28
};
let seq = 0;
const flash = new Set(); // cards changed since the last draw
const clock = () => S.now;
const hhmm = (ms) => new Date(ms).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false });
function ago(ms) {
  const s = Math.max(0, Math.round((clock() - ms) / 1000));
  if (s < 10) return 'just now';
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}

// ── the write path, simulated: a verb writes, the store keeps, the bus wakes
// Requirement 21: every line names the program it concerns, so a program's
// GUI can show only its own. rigd knows the caller; the mock reads the text.
let wireQueued = false;
const IDS = new Set([...Object.keys(WRITERS), ...estate.map((e) => e.id)]);
function wire(kind, verb, text, who) {
  if (!who) who = (String(text).match(/[a-z][a-z-]*/g) || []).find((w) => IDS.has(w)) || 'rig';
  S.wire.unshift({ at: clock(), kind, verb, text, who });
  S.wire.length = Math.min(S.wire.length, 120);
  // A line can land between draws (a GUI's frame answers late), so the open
  // wire redraws itself once per frame rather than waiting for the next render.
  if (S.showWire && !wireQueued) { wireQueued = true; requestAnimationFrame(() => { wireQueued = false; renderWire(); }); }
}
function put(from, fields, cardId) {
  if (fields.body && new TextEncoder().encode(fields.body).length > BODY_CAP) {
    wire('refused', 'rig.panel.put', `${from}: body over ${BODY_CAP} bytes; nothing changed`);
    return null;
  }
  const open = [...S.cards.values()].filter((c) => !c.closed).length;
  let c = cardId && S.cards.get(cardId);
  if (!c && open >= OPEN_CAP) {
    wire('refused', 'rig.panel.put', `${from}: ${OPEN_CAP} open cards already; close one first`);
    return null;
  }
  const w = WRITERS[from];
  if (!c) {
    const id = `${from.slice(0, 2)}-${++seq}`;
    c = { id, from, kind: w.kind, project: fields.project || w.project, created: clock(), rev: 0, versions: [], order: ++S.order };
    S.cards.set(id, c);
  }
  const before = { ...c };
  Object.assign(c, fields, { updated: clock(), rev: c.rev + 1 });
  if (fields.status === 'done' || fields.status === 'failed') c.busy = false;
  const snap = { rev: c.rev, at: clock(), title: c.title, status: c.status, severity: c.severity, body: c.body, progress: c.progress, facts: c.facts, closed: c.closed };
  c.versions.push(snap);
  const changed = before.rev ? Object.keys(fields).filter((k) => JSON.stringify(before[k]) !== JSON.stringify(fields[k])).join(', ') : 'new card';
  wire('put', 'rig.panel.put', `${from} -> ${c.id} rev ${c.rev} (${changed})`, from);
  wire('store', 'store', `panel/${c.id}.${c.rev} kept whole`, from);
  wire('bus', 'panel.changed', `-> window`, from);
  S.dismissed.delete(c.from);
  if (S.minimised) S.unseen.push(c.severity || 'info');
  flash.add(c.id);
  return c.id;
}
function close(from, id, fields = {}) {
  put(from, { ...fields, closed: clock() }, id);
}
function notify(from, severity, title, body, at, more = {}) {
  S.notes.unshift({ id: ++seq, from, severity, title, body, at: at ?? clock(), unread: true, ...more });
  S.notes.sort((a, b) => b.at - a.at);
  if (at === undefined) wire('bus', 'rig.notify', `${from}: ${title}`);
}
// Evicted, not hidden: a notification past the age is gone from the panel.
function evict() {
  const cut = clock() - S.keepDays * 86400e3;
  const keep = S.notes.filter((n) => n.at >= cut);
  S.evicted += S.notes.length - keep.length;
  S.notes = keep;
}
function requestTab(from, reason) {
  let t = S.tabs.find((x) => x.id === from);
  if (!t) { t = { id: from, title: from, kind: WRITERS[from].kind }; S.tabs.push(t); }
  Object.assign(t, { open: true, reason, asked: clock(), released: null, fresh: true });
  wire('tab', 'rig.panel.tab', `${from} asks for its own tab: "${reason}"`);
}
function releaseTab(from) {
  const t = S.tabs.find((x) => x.id === from);
  if (t) { t.released = clock(); wire('tab', 'rig.panel.tab', `${from} no longer needs its tab; it stays until you close it`); }
}
// Requirements 10 to 12: a program registers a GUI. Requirement 15:
// registering ADDS its tab, and Main stays the one shown.
function registerGui(from, entry) {
  S.guis[from] = { entry, at: clock() };
  let t = S.tabs.find((x) => x.id === from);
  if (!t) { t = { id: from, title: from, kind: WRITERS[from]?.kind || 'program' }; S.tabs.push(t); }
  Object.assign(t, { open: true, fresh: true });
  wire('tab', 'rig.gui.register', `${from} registers its GUI: ${entry}, styled by rig.css; its tab is added, Main stays in front`);
}
const INTERNAL = { capabilities: 'Capabilities', guidelines: 'Guidelines', settings: 'Settings' };
// Requirement 14: picking a program opens its tab, from the rail or from
// Main's list. A program with no GUI gets rig's own page about it.
function openTab(id) {
  let t = S.tabs.find((x) => x.id === id);
  if (!t) {
    if (INTERNAL[id]) t = { id, title: INTERNAL[id], kind: 'rig' };
    else if (estate.find((x) => x.id === id)) t = { id, title: id, kind: 'program' };
    else return;
    S.tabs.push(t);
  }
  if (!t.open) t.picked = clock();
  Object.assign(t, { open: true, fresh: false });
  S.current = id;
  render();
}

// Ledger, the program behind the GUI: it holds its own state, answers what
// the GUI sends, and pushes to it unasked. Both directions go through rig.
const ledger = {
  rev: 1, statement: 'September statement', entries: 418, matched: 412, lastRun: '', running: null,
  open: [
    { id: 1182, date: '09-14', payee: 'Hetzner Online', amount: '-38.20', bank: 'HETZNER ONLINE GMBH 38.20', why: 'same amount twice', sev: 'warning' },
    { id: 1183, date: '09-14', payee: 'Hetzner Online', amount: '-38.20', bank: 'HETZNER ONLINE GMBH 38.20', why: 'same amount twice', sev: 'warning' },
    { id: 1201, date: '09-22', payee: 'Cafe Neko', amount: '-6.40', bank: '', why: 'no bank line', sev: 'error' },
  ],
  done: [],
};
function ledgerFrame() { return document.querySelector('#guihost iframe[data-gui="ledger"]'); }
const post = (m) => ledgerFrame()?.contentWindow?.postMessage(m, '*');

// Requirement 13: the GUI acts only through rig's verbs. What Ledger
// declares is what rig lets its GUI invoke, and rig checks the arguments
// against it before Ledger sees the call.
const LEDGER_DECLARES = {
  match: { entry: 'int', about: 'match an entry to its bank line' },
  reject: { entry: 'int', about: 'mark an entry not a match' },
};
const LEDGER_JOBS = new Set(['ledger.reconcile']);
const subscribed = new Set();

// Ledger, the program: it keeps its state in its store collection and
// publishes ledger.changed on the bus after each write.
function ledgerWrite(why) {
  wire('store', 'store.put', `ledger/reconcile rev ${++ledger.rev}: ${why}`);
  wire('bus', 'events.publish', `ledger.changed {rev: ${ledger.rev}}`);
  if (subscribed.has('ledger.changed')) post({ type: 'event', topic: 'ledger.changed', data: { rev: ledger.rev } });
}
function ledgerHandles(command, args) {
  const i = ledger.open.findIndex((x) => x.id === args.entry);
  if (i < 0) return { ok: false, error: `entry ${args.entry} is not open` };
  if (command === 'match' && !ledger.open[i].bank) return { ok: false, error: `entry ${args.entry} has no bank line` };
  const [e] = ledger.open.splice(i, 1);
  if (command === 'match') { ledger.matched++; ledger.done.unshift({ id: e.id, at: hhmm(Date.now()), what: 'matched', sev: 'success' }); }
  else ledger.done.unshift({ id: e.id, at: hhmm(Date.now()), what: 'not a match, kept open in the books', sev: 'info' });
  ledgerWrite(`${e.id} ${command === 'match' ? 'matched' : 'not a match'}`);
  notify('ledger', 'info', `Entry ${e.id} ${command === 'match' ? 'matched' : 'marked not a match'}`, 'from its GUI');
  return { ok: true, result: { entry: e.id } };
}
function ledgerReconcile(jobId) {
  ledger.running = 0; ledgerWrite(`job ${jobId} claimed`);
  wire('tab', 'queue.claim', `ledger claims ${jobId}`);
  const step = () => {
    ledger.running = Math.min(1, ledger.running + 0.25);
    wire('tab', 'progress.step', `${jobId} ${Math.round(ledger.running * 100)}%`, 'ledger');
    if (ledger.running >= 1) {
      ledger.running = null; ledger.lastRun = hhmm(Date.now());
      wire('tab', 'queue.complete', `${jobId} done`, 'ledger');
      notify('ledger', 'success', 'Reconciliation finished', `${ledger.matched} matched, ${ledger.open.length} need you`);
      render();
    }
    ledgerWrite(ledger.running === null ? 'run finished' : 'progress');
    if (ledger.running !== null) setTimeout(step, 500);
  };
  setTimeout(step, 500);
}

// rig's side of the bridge. Every call is answered, refusals included.
let jobs = 0;
function rigAnswers(verb, args, reply) {
  const refuse = (why) => { wire('refused', verb, `ledger's GUI: ${why}; nothing reached ledger`); reply(false, why); };
  if (verb === 'invoke') {
    const decl = LEDGER_DECLARES[args?.command];
    if (!decl) return refuse(`ledger declares no command "${String(args?.command).slice(0, 40)}"`);
    if (!Number.isInteger(args.args?.entry)) return refuse('entry must be an integer, as ledger declares');
    wire('acted', 'invoke', `ledger ${args.command} {entry: ${args.args.entry}}, checked against its declaration`);
    setTimeout(() => { const r = ledgerHandles(args.command, { entry: args.args.entry }); if (!r.ok) wire('refused', 'invoke', `ledger answered: ${r.error}`); reply(r.ok, r.ok ? r.result : r.error); render(); }, 400);
    return;
  }
  if (verb === 'store.get') {
    if (args?.key !== 'ledger/reconcile') return refuse('the store key is outside ledger\'s collection');
    wire('store', 'store.get', `ledger/reconcile rev ${ledger.rev}`);
    return reply(true, JSON.parse(JSON.stringify(ledger)));
  }
  if (verb === 'queue.push') {
    if (!LEDGER_JOBS.has(args?.job)) return refuse('not a job ledger takes');
    if (ledger.running !== null) return refuse('a reconcile is already running');
    const id = `job-${++jobs}`;
    wire('acted', 'queue.push', `${args.job} as ${id}`, 'ledger');
    reply(true, { job: id });
    setTimeout(() => ledgerReconcile(id), 300);
    return;
  }
  if (verb === 'toast') {
    const acts = Array.isArray(args?.actions) ? args.actions.filter((a) => typeof a === 'string').slice(0, 3) : [];
    if (typeof args?.title !== 'string' || !acts.length) return refuse('a toast needs a title and actions');
    wire('tab', 'toast', `from ledger: "${args.title.slice(0, 80)}"`);
    askToast('ledger', args.title.slice(0, 200), String(args.body || '').slice(0, 400), acts, (choice) => { wire('tab', 'toast.answer', `"${choice}"`, 'ledger'); reply(true, choice); });
    return;
  }
  refuse(`rig carries no verb "${String(verb).slice(0, 40)}"`);
}
window.addEventListener('message', (e) => {
  const f = ledgerFrame();
  if (!f || e.source !== f.contentWindow) return;
  const m = e.data;
  if (m && m.rig === 'sub' && typeof m.topic === 'string') {
    if (m.topic !== 'ledger.changed') { wire('refused', 'events.wait', 'ledger\'s GUI: a topic ledger does not publish'); return; }
    if (!subscribed.has(m.topic)) { subscribed.add(m.topic); wire('bus', 'events.wait', `ledger's GUI listens for ${m.topic}`); }
    return;
  }
  if (!m || m.rig !== 'call' || !Number.isInteger(m.id) || typeof m.verb !== 'string') { wire('refused', 'bridge', 'ledger\'s GUI sent something rig does not carry; dropped'); return; }
  rigAnswers(m.verb, m.args, (ok, v) => post(ok ? { type: 'reply', id: m.id, ok: true, result: v } : { type: 'reply', id: m.id, ok: false, error: String(v) }));
});

// A rig toast: rig asks the user on the program's behalf.
function askToast(from, title, body, actions, answer) {
  const box = $('asktoast');
  const done = (c) => { box.hidden = true; box.replaceChildren(); document.removeEventListener('keydown', esc, true); answer(c); };
  const esc = (e) => { if (e.key === 'Escape') { e.stopPropagation(); done(actions[actions.length - 1]); } };
  box.replaceChildren(h('div', { class: 'from' }, `${from} asks`), h('b', {}, title), body ? h('div', { class: 'dim' }, body) : null,
    h('div', { class: 'acts' }, actions.map((a, i) => h('button', { class: 'act' + (i === 0 ? ' primary' : ''), onclick: () => done(a) }, a))));
  box.hidden = false;
  document.addEventListener('keydown', esc, true);
  box.querySelector('button').focus();
}

// Requirements 23 to 26: what needs him. A notification linked to a card is
// one item, so deciding it on either surface decides both.
function needsYou() {
  const items = [], linked = new Set();
  for (const n of S.notes) if (n.actions && !n.decided) {
    items.push({ kind: 'note', n, from: n.from, title: n.title, body: n.body, actions: n.actions, at: n.at, sev: n.severity });
    if (n.card) linked.add(n.card);
  }
  for (const c of S.cards.values()) if (!c.closed && c.actions && !c.decided && (c.status === 'waiting' || c.status === 'failed') && !linked.has(c.id))
    items.push({ kind: 'card', c, from: c.from, title: c.title, body: c.body, actions: c.actions, at: c.updated, sev: c.severity });
  return items.sort((a, b) => b.at - a.at);
}
function decideCard(c, choice) {
  if (!c || c.decided) return;
  c.decided = { choice, at: clock() };
  acted(c, choice.toLowerCase());
}
function decideNote(n, choice) {
  if (n.decided) return;
  n.decided = { choice, at: clock() }; n.unread = false;
  wire('acted', 'toast.answer', `-> ${n.from} "${choice}" for "${n.title}"`, n.from);
  if (n.card) decideCard(S.cards.get(n.card), choice);
}
function decide(item, choice) {
  if (item.kind === 'note') decideNote(item.n, choice); else decideCard(item.c, choice);
  render();
}
function noteFor(card) { return S.notes.find((n) => n.card === card.id); }

function acted(card, action) {
  wire('acted', 'panel.acted', `-> ${card.from} {card: ${card.id}, action: ${action}}`);
  toast(`Sent "${action}" to ${card.from}. It answers by updating the card.`);
}

// ── the simulation: three agents and two programs at work ──────────────────
const ids = {};
const SCRIPT = [
  () => { ids.plan = put('rig-lead', { title: 'Close plan/54 resident gaps', status: 'running', severity: 'info', busy: true, body: 'Down residents listed; rebuilt binaries read by the scan.', facts: [{ label: 'repo', value: 'rig' }] }); },
  () => { ids.sw = put('storeworker', { title: 'Queue run 418: reindex knowledge base', status: 'running', severity: 'info', progress: 0.1, facts: [{ label: 'items', value: '1,204' }] }); },
  () => { ids.ask = put('beacon-seat', { title: 'Which board font?', status: 'waiting', severity: 'warning', body: 'Two options drawn in the pane. Pick one so I can carry on.', actions: ['Reply', 'Open pane'] }); },
  () => { put('storeworker', { progress: 0.35 }, ids.sw); },
  () => { put('rig-lead', { status: 'running', progress: 0.5, busy: false, body: 'Supervisor half done; 1 of 2 tests red as expected.' }, ids.plan); },
  () => { ids.led = put('ledger', { title: 'Reconciliation finished', status: 'done', severity: 'success', body: '412 entries matched, 3 need a look.', facts: [{ label: 'matched', value: '412' }, { label: 'open', value: '3' }], actions: ['Review'] }); ledger.lastRun = hhmm(Date.now()); ledgerWrite('reconcile finished'); requestTab('ledger', '3 entries need you'); notify('ledger', 'success', 'Reconciliation finished', '3 entries need you'); },
  () => { put('storeworker', { progress: 0.62 }, ids.sw); },
  () => { ids.ci = put('rig-lead', { title: 'make ci', status: 'failed', severity: 'error', body: 'wire golden: Program gained two fields. Re-record with -update.', actions: ['Retry', 'Open log'] }); notify('rig-lead', 'error', 'make ci failed', 'wire golden: Program gained two fields'); },
  () => { put('storeworker', { progress: 0.88 }, ids.sw); },
  () => { put('rig-lead', { status: 'done', severity: 'success', body: 'Re-recorded the golden; all gates green.' }, ids.ci); },
  () => { ids.deploy = put('beacon', { title: 'Card drawn: deploy now?', status: 'waiting', severity: 'warning', body: 'Asked by rig-lead. Options: yes, later.', actions: ['Yes', 'Later'] }); requestTab('beacon', 'a question for you'); notify('beacon', 'warning', 'A question for you', 'deploy now?', undefined, { actions: ['Yes', 'Later'], card: ids.deploy, details: 'rig-lead drew this card through beacon: deploy v0.54 now, or later today. beacon waits for your answer and tells rig-lead.', facts: [['asked by', 'rig-lead'], ['through', 'beacon ask']] }); },
  () => { put('storeworker', { status: 'done', severity: 'success', progress: 1, body: '1,204 items reindexed in 38 s.' }, ids.sw); },
  () => { put('rig-lead', { status: 'done', severity: 'success', progress: 1, body: 'Both gaps closed and demonstrated live.' }, ids.plan); },
  () => { put('beacon-seat', { title: 'Too long a body', body: 'x'.repeat(BODY_CAP + 1) }); },
  () => { close('storeworker', ids.sw); releaseTab('ledger'); },
  () => { ids.sw2 = put('storeworker', { title: 'Queue run 419: backup', status: 'queued', severity: 'info' }); },
  () => { put('storeworker', { status: 'running', busy: true }, ids.sw2); },
];
function tick() {
  S.now += 1000;
  if (S.step < SCRIPT.length) SCRIPT[S.step++]();
  else if (ids.sw2 && S.cards.get(ids.sw2).status === 'running' && Math.random() < 0.3) {
    put('storeworker', { status: 'done', severity: 'success', busy: false, body: 'Backup written.' }, ids.sw2);
  }
  render();
}

// Seed: earlier work, so search and history have something to find, and one
// tab opened before and closed, so "reopen" has a last state to show.
(function seed() {
  const t0 = S.now - 3 * 3600e3;
  S.now = t0;
  const a = put('rig-lead', { title: 'Scan dirs for programs', status: 'running', severity: 'info', progress: 0.3 });
  S.now += 1200e3; put('rig-lead', { progress: 0.8 }, a);
  S.now += 900e3; put('rig-lead', { status: 'done', severity: 'success', progress: 1, body: 'Decision 0265 built and demonstrated.' }, a);
  const b = put('beacon-seat', { title: 'Install path for beacon', status: 'done', severity: 'success', body: '~/.local/lib/rig/apps/beacon' });
  S.now += 600e3; close('beacon-seat', b);
  ledger.lastRun = hhmm(S.now);
  registerGui('ledger', 'gui/index.html');
  requestTab('storeworker', 'run 417 needs a retry decision');
  ids.r417 = put('storeworker', { title: 'Run 417 failed: disk full', status: 'failed', severity: 'error', body: 'Freed 2 GB since. Retry?', actions: ['Retry', 'Leave it'] });
  S.tabs.find((x) => x.id === 'storeworker').open = false;
  const day = 86400e3, now = Date.now();
  notify('rig', 'info', 'rig started', 'production, epoch 108', now - 2.2 * 3600e3);
  notify('rig', 'warning', 'abacus quarantined', 'exit 2, five times in 4 minutes', now - 3600e3);
  notify('rig', 'info', 'keeper stopped', 'by a human, from rig stop', now - 20 * 60e3);
  notify('storeworker', 'error', 'Run 417 failed', 'disk full', now - 3 * 3600e3, { actions: ['Retry', 'Leave it'], card: ids.r417,
    details: 'The backup run stopped when the disk filled at 182 MB written. 2 GB has been freed since, so a retry should finish.', facts: [['run', '417'], ['queue', 'storeworker.backup'], ['exit', 'ENOSPC']] });
  notify('rig', 'warning', 'Restart abacus?', 'quarantined after five exits', now - 55 * 60e3, { actions: ['Restart', 'Leave it'], decided: { choice: 'Leave it', at: now - 50 * 60e3 },
    details: 'abacus exited with code 2 five times in four minutes, so rig quarantined it. Its binary has changed since.', facts: [['restarts', '5'], ['last exit', 'exit 2']] });
  notify('rig', 'info', 'lantern found by the scan', '~/.local/lib/rig/apps/lantern, on call', now - 26 * 3600e3);
  notify('rig', 'success', 'Backup written', '~/rig-backups/production-2026-09-30.tar', now - 3 * day);
  notify('beacon-seat', 'info', 'beacon 0.3.0 installed', 'read by the scan, kept', now - 6 * day);
  notify('rig', 'info', 'storeworker restarted', 'its binary changed on disk', now - 6.9 * day);
  notify('rig', 'warning', 'Disk 90% full', 'the store is at 450 MB', now - 8 * day);
  notify('rig', 'info', 'Deployed v0.53.0', 'make deploy', now - 9 * day);
  notify('ledger', 'info', 'Statement imported', 'September, 418 entries', now - 2 * day);
  notify('righand', 'warning', 'Script paused', 'a window it needed was closed', now - 5 * 3600e3);
  notify('lantern', 'success', 'Index rebuilt', '12,480 files in 2 min', now - 4 * day);
  notify('ledger', 'warning', 'Two duplicates found', 'entries 1182 and 1183', now - 5 * day);
  S.notes.forEach((n) => { n.unread = now - n.at < 3600e3; });
  S.now = Date.now();
  S.wire = [];
  flash.clear();
})();

// ── rendering ──────────────────────────────────────────────────────────────
const $ = (id) => document.getElementById(id);

function worst(list) {
  return list.reduce((w, s) => (RANK[s] || 0) > (RANK[w] || 0) ? s : w, 'info');
}
function visible(c) {
  if (S.bsrc && c.from !== S.bsrc) return false;
  if (S.sev.size && !S.sev.has(c.severity)) return false;
  if (S.status.size && !S.status.has(c.status)) return false;
  if (S.since === '1h' && clock() - c.updated > 3600e3) return false;
  if (S.q) {
    const hay = [c.title, c.body, c.from, c.project, ...(c.facts || []).map((f) => f.label + ' ' + f.value), ...c.versions.map((v) => v.body || '')].join(' ').toLowerCase();
    if (!hay.includes(S.q.toLowerCase())) return false;
  }
  return true;
}

function renderTabs() {
  const bar = $('tabs');
  bar.replaceChildren();
  for (const t of S.tabs.filter((x) => x.open)) {
    const own = [...S.cards.values()].filter((c) => c.from === t.id && !c.closed);
    const tab = h('div', { class: 'tab' + (t.fresh ? ' fresh' : ''), role: 'tab', 'aria-selected': String(S.current === t.id), tabindex: '0',
      onclick: () => { S.current = t.id; t.fresh = false; render(); },
      onkeydown: (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); S.current = t.id; render(); } } },
      t.kind === 'main' || t.kind === 'rig' ? null : h('span', { class: 'dot', style: { '--sev': SEV[worst(own.map((c) => c.severity))] } }),
      h('span', { class: 'who' }, t.title),
      t.kind === 'main' ? null : h('span', { class: 'kind' }, t.kind),
      t.kind === 'main' ? null : h('button', { class: 'x', 'aria-label': `Close ${t.title}'s tab`, onclick: (e) => { e.stopPropagation(); t.open = false; t.closedAt = clock(); if (S.current === t.id) S.current = 'main'; render(); } }, svg(ICON.x)));
    bar.append(tab);
  }
  const closed = S.tabs.filter((x) => !x.open);
  const need = needsYou().length;
  bar.append(h('span', { class: 'spacer' }),
    h('button', { class: 'tool tray' + (need ? ' alert' : ''), id: 'tray', title: need ? `${need} need you: open them` : 'Nothing needs you',
      'aria-label': need ? `rig's tray icon: ${need} need you. Open them` : 'rig\'s tray icon: nothing needs you',
      onclick: () => { S.current = 'main'; S.mainTab = 'needs'; render(); document.querySelector('.subtabs [aria-selected="true"]')?.focus(); } },
      h('span', { class: 'trayic' }, 'r', need ? h('i', {}, String(need)) : null), 'tray (simulated)'),
    h('button', { class: 'tool', id: 'reopenBtn', 'aria-haspopup': 'menu', onclick: (e) => openMenu(e) }, svg(ICON.reopen), 'Tabs ', h('b', {}, String(closed.length))),
    h('button', { class: 'tool', 'aria-label': S.playing ? 'Pause the simulation' : 'Play the simulation', onclick: () => { S.playing = !S.playing; render(); } }, svg(S.playing ? ICON.pause : ICON.play), S.playing ? 'Live' : 'Paused'),
    h('button', { class: 'tool', 'aria-label': 'One step of the simulation', onclick: tick }, svg(ICON.step), 'Step'),
    h('button', { class: 'tool', 'aria-pressed': String(S.showWire), onclick: () => { S.showWire = !S.showWire; render(); } }, svg(ICON.wire), 'Wire'),
    h('button', { class: 'tool', onclick: () => openQuestions() }, svg(ICON.q), 'Open questions ', h('b', {}, String(QUESTIONS.length))),
    h('button', { class: 'tool', 'aria-label': 'Switch light and dark', onclick: () => { const r = document.documentElement; r.setAttribute('data-theme', r.getAttribute('data-theme') === 'light' ? 'dark' : 'light'); } }, svg(ICON.sun)));
}

function openMenu() {
  const m = $('menu');
  const closed = S.tabs.filter((x) => !x.open);
  const item = (t, note) => h('button', { onclick: () => { m.hidden = true; openTab(t.id); } },
    h('span', {}, t.title), h('span', { class: 'dim' }, S.guis[t.id] ? 'its GUI' : t.kind), h('small', {}, note));
  const before = closed.filter((t) => t.asked || t.closedAt);
  m.replaceChildren(
    h('h5', {}, 'Opened before. Reopen one to see its last state.'),
    ...(before.length ? before.map((t) => item(t, (t.asked ? `asked ${hhmm(t.asked)}: "${t.reason}"` : S.guis[t.id] ? `registered ${hhmm(S.guis[t.id].at)}` : 'picked by you') + (t.closedAt ? `, closed ${hhmm(t.closedAt)}` : ''))) : [h('div', { class: 'none' }, 'None yet. A tab appears here once you close it.')]));
  m.hidden = !m.hidden;
  if (!m.hidden) m.querySelector('button')?.focus();
}

function cardNode(c) {
  const st = STATUS[c.status] || { tone: 'var(--fg-dim)' };
  const asks = c.actions && !c.closed && !c.decided && (c.status === 'waiting' || c.status === 'failed');
  const node = h('button', { class: 'card' + (c.closed ? ' closed' : '') + (asks ? ' asks' : '') + (flash.has(c.id) ? ' flash' : ''), style: { '--sev': SEV[c.severity] || 'var(--border)' },
    'aria-label': `${c.title}, ${c.status || ''}, ${c.severity || ''}. Open its history`, onclick: () => openHistory(c.id) },
    h('div', { class: 'top' },
      h('span', { class: 't' }, c.title),
      c.status ? h('span', { class: 'status' + (st.spin ? ' spin' : ''), style: { '--tone': st.tone } }, st.icon ? svg(ICON[st.icon]) : null, c.status) : null),
    c.severity ? h('div', { class: 'sev' }, c.severity) : null,
    c.body ? h('p', {}, c.body) : null,
    typeof c.progress === 'number' || c.busy ? h('div', { class: 'pct' },
      h('div', { class: 'meter' + (typeof c.progress === 'number' ? '' : ' indet'), role: 'progressbar', 'aria-label': c.title, 'aria-valuenow': typeof c.progress === 'number' ? String(Math.round(c.progress * 100)) : null },
        h('i', { style: typeof c.progress === 'number' ? { width: (c.progress * 100).toFixed(0) + '%' } : {} })),
      typeof c.progress === 'number' ? h('b', {}, Math.round(c.progress * 100) + '%') : null) : null,
    c.facts && c.facts.length ? h('dl', { class: 'facts' }, c.facts.flatMap((f) => [h('dt', {}, f.label), h('dd', {}, f.value)])) : null,
    c.decided ? h('div', { class: 'chosen' }, `You chose "${c.decided.choice}" at ${hhmm(c.decided.at)}`) : null,
    c.actions && !c.closed && !c.decided && c.status !== 'done' ? h('div', { class: 'acts' }, c.actions.map((a) => {
      const go = (e) => { e.stopPropagation(); const n = noteFor(c); if (n) decideNote(n, a); else decideCard(c, a); render(); };
      return h('span', { class: 'act', role: 'button', tabindex: '0', onclick: go,
        onkeydown: (e) => { if (e.key === 'Enter') { e.preventDefault(); go(e); } } }, a); })) : null,
    h('div', { class: 'foot' },
      h('span', {}, 'created ' + hhmm(c.created)),
      h('span', {}, c.closed ? `closed ${hhmm(c.closed)}` : c.rev > 1 ? `${c.rev - 1} change${c.rev === 2 ? '' : 's'}` : 'new'),
      h('span', { class: 'sp' }), h('time', {}, ago(c.updated))));
  return node;
}

function groupKey(c) {
  if (S.group === 'project') return c.project;
  if (S.group === 'severity') return c.severity || 'none';
  return c.from;
}

function renderBoard(container) {
  const all = [...S.cards.values()];
  const shown = all.filter(visible);
  const groups = new Map();
  for (const c of shown) {
    const k = groupKey(c);
    if (S.group === 'writer' && S.dismissed.has(k)) continue;
    if (!groups.has(k)) groups.set(k, []);
    groups.get(k).push(c);
  }
  const panel = h('section', { class: 'panel', 'aria-label': 'Board' });
  const open = all.filter((c) => !c.closed);
  panel.append(h('header', {},
    h('h2', {}, 'Board'), h('span', { class: 'dim' }, `${open.length} open, ${all.length - open.length} closed`),
    h('span', { class: 'sp' }),
    h('button', { class: 'iconbtn', 'aria-label': S.minimised ? 'Restore the board' : 'Minimise the board', onclick: () => { S.minimised = !S.minimised; S.unseen = []; render(); } }, svg(S.minimised ? ICON.max : ICON.min))));
  if (S.minimised) {
    const w = worst(S.unseen);
    panel.append(h('button', { class: 'minbar', onclick: () => { S.minimised = false; S.unseen = []; render(); } },
      S.unseen.length ? h('span', { class: 'badge', style: { '--sev': SEV[w] } }, `${S.unseen.length} new`) : h('span', { class: 'dim' }, 'Nothing new'),
      h('span', { class: 'dim' }, 'The board is minimised. Click to restore it.')));
    container.append(panel);
    return;
  }
  const srcs = [...new Set(all.map((c) => c.from))].sort();
  const openOf = (src) => all.filter((c) => !c.closed && (!src || c.from === src)).length;
  panel.append(h('div', { class: 'srctabs', role: 'tablist', 'aria-label': 'Board sources' },
    ['', ...srcs].map((src) => h('button', { role: 'tab', 'aria-selected': String(S.bsrc === src), onclick: () => { S.bsrc = src; render(); } },
      src || 'Every source', h('span', { class: 'cnt' }, String(openOf(src)))))));
  const search = h('input', { id: 'q', type: 'search', placeholder: 'Search past work: titles, bodies, facts, every version', value: S.q, 'aria-label': 'Search the board',
    oninput: (e) => { S.q = e.target.value; renderView(); $('q')?.focus(); const q = $('q'); if (q) q.setSelectionRange(q.value.length, q.value.length); } });
  const chip = (set, key, label, color) => h('button', { class: 'chip', 'aria-pressed': String(set.has(key)), style: color ? { '--chipc': color } : {},
    onclick: () => { set.has(key) ? set.delete(key) : set.add(key); render(); } }, color ? h('i') : null, label);
  panel.append(h('div', { class: 'filters' },
    h('label', { class: 'search' }, search, h('kbd', {}, '/')),
    ...Object.keys(SEV).map((s) => chip(S.sev, s, s, SEV[s])),
    ...['running', 'waiting', 'failed', 'done'].map((s) => chip(S.status, s, s)),
    h('label', { class: 'dim' }, 'Group ', h('select', { class: 'sel', onchange: (e) => { S.group = e.target.value; render(); } },
      ...[['writer', 'by agent or program'], ['project', 'by project'], ['severity', 'by severity']].map(([v, l]) => h('option', { value: v, selected: S.group === v }, l)))),
    h('label', { class: 'dim' }, 'Time ', h('select', { class: 'sel', onchange: (e) => { S.since = e.target.value; render(); } },
      ...[['all', 'all'], ['1h', 'last hour']].map(([v, l]) => h('option', { value: v, selected: S.since === v }, l))))));
  const board = h('div', { class: 'board' });
  if (!groups.size) board.append(h('div', { class: 'empty' }, S.q || S.sev.size || S.status.size ? 'Nothing matches. Clear a filter to see more.' : 'No cards yet. Programs and agents put them here.'));
  const askN = (list) => list.filter((c) => c.actions && !c.closed && !c.decided && (c.status === 'waiting' || c.status === 'failed')).length;
  const sorted = [...groups.entries()].sort((a, b) => (askN(b[1]) > 0) - (askN(a[1]) > 0) || Math.max(...b[1].map((c) => c.updated)) - Math.max(...a[1].map((c) => c.updated)));
  for (const [k, list] of sorted) {
    list.sort((a, b) => (askN([b]) - askN([a])) || b.order - a.order);
    const live = list.filter((c) => !c.closed);
    const done = list.filter((c) => c.closed);
    const w = WRITERS[k];
    const sect = h('section', { class: 'sect', 'aria-label': k },
      h('header', {},
        h('span', { class: 'worst', style: { '--sev': SEV[worst(live.map((c) => c.severity))] || 'var(--border)' } }),
        h('span', { class: 'name' }, k),
        w ? h('span', { class: 'proj' }, `${w.kind}, ${w.project}`) : null,
        h('span', { class: 'proj' }, `${live.length} open`),
        h('span', { class: 'sp' }),
        S.group === 'writer' ? h('button', { class: 'iconbtn', 'aria-label': `Dismiss ${k}'s section`, title: 'Dismiss this section. It comes back when it writes again.', onclick: () => { S.dismissed.add(k); render(); } }, svg(ICON.x)) : null),
      h('div', { class: 'cards' }, live.map(cardNode)));
    if (done.length) {
      const key = 'showclosed:' + k;
      if (S[key]) sect.querySelector('.cards').append(...done.map(cardNode));
      sect.append(h('div', { class: 'more' }, h('button', { onclick: () => { S[key] = !S[key]; render(); } }, S[key] ? `Hide ${done.length} closed` : `Show ${done.length} closed`)));
    }
    board.append(sect);
  }
  if (S.dismissed.size && S.group === 'writer') board.append(h('div', { class: 'more' }, `${S.dismissed.size} section${S.dismissed.size > 1 ? 's' : ''} dismissed. `,
    h('button', { onclick: () => { S.dismissed.clear(); render(); } }, 'Bring back')));
  // The board's own controls stay put while its cards scroll under them.
  panel.replaceChildren(h('div', { class: 'boardtop' }, ...panel.childNodes));
  panel.append(board);
  container.append(panel);
}

// Requirement 24: everything waiting on him, in one place, newest first,
// each with its options right there.
function renderNeeds(container) {
  const items = needsYou();
  const panel = h('section', { class: 'panel needs', 'aria-label': 'Needs you' });
  if (!items.length) panel.append(h('div', { class: 'empty' }, 'Nothing needs you. When a program or an agent asks, it lands here, and rig\'s tray icon shows it.'));
  for (const it of items) {
    panel.append(h('article', { class: 'need', style: { '--sev': SEV[it.sev] || 'var(--h-amber)' } },
      h('div', { class: 'who' }, h('span', { class: 'src' }, it.from), h('time', { class: 'dim' }, ago(it.at)), h('span', { class: 'dim' }, it.kind === 'note' ? 'a notification' : 'a card on the board')),
      h('b', {}, it.title),
      it.body ? h('div', { class: 'dim' }, it.body) : null,
      h('div', { class: 'acts' }, it.actions.map((a, i) => h('button', { class: 'act' + (i === 0 ? ' primary' : ''), onclick: () => decide(it, a) }, a)),
        it.kind === 'note' ? h('button', { class: 'linkish', onclick: () => openNote(it.n) }, 'Details') : h('button', { class: 'linkish', onclick: () => openHistory(it.c.id) }, 'History'))));
  }
  container.append(panel);
}

function renderSummary(container) {
  const all = [...S.cards.values()].filter((c) => !c.closed || clock() - c.closed < 3600e3);
  const n = (st) => all.filter((c) => c.status === st).length;
  // A figure is a button to the inner tab that explains it.
  const fig = (cls, num, label, to) => h('button', { class: 'fig ' + cls, title: `Show ${{ board: 'the board', programs: 'the programs', needs: 'what needs you' }[to]}`,
    onclick: () => { S.mainTab = to; render(); } }, h('div', { class: 'n' }, String(num)), h('div', { class: 'l' }, label));
  container.append(h('div', { class: 'figs' },
    fig('', estate.length, 'programs', 'programs'),
    fig('good', estate.filter((e) => e.state === 'healthy' || e.state === 'running').length, 'up', 'programs'),
    fig('', estate.filter((e) => e.state === 'at rest').length, 'at rest, on call', 'programs'),
    fig(estate.some((e) => e.state === 'down' || e.state === 'quarantined') ? 'bad' : '', estate.filter((e) => e.state === 'down' || e.state === 'quarantined').length, 'down or quarantined', 'programs'),
    fig('warn', needsYou().length, 'need you', 'needs'),
    fig('run', n('running'), 'agents working', 'board')));
}

// Requirement 6: rig and every program it represents, versions first.
// Requirement 14: picking one opens its tab, where it is inspected.
function renderPrograms(container) {
  const panel = h('section', { class: 'panel', 'aria-label': 'Programs' },
    h('header', {}, h('h2', {}, 'Programs'), h('span', { class: 'dim' }, `${estate.length}, scanned in ${RIG.scan} and declared in programs.json`)));
  const table = h('table', { class: 'progs' },
    h('thead', {}, h('tr', {}, ...['Program', 'Version', 'Load', 'State', 'Commands', 'Restarts', 'Guidelines', 'Found by'].map((x) => h('th', { scope: 'col' }, x)))));
  const body = h('tbody');
  for (const e of estate) {
    const open = S.tabs.some((x) => x.id === e.id && x.open);
    body.append(h('tr', { class: 'row' + (open ? ' open' : '') },
      h('td', {}, h('button', { class: 'pname', title: `Open ${e.name}'s tab`, onclick: () => openTab(e.id) },
        h('span', { class: 'ic', style: { '--c': progColour(e.id) } }, e.id.slice(0, 2)), e.name,
        S.guis[e.id] ? h('span', { class: 'gtag' }, 'GUI') : null)),
      h('td', { class: 'mono' }, e.version),
      h('td', {}, e.load),
      h('td', {}, h('span', { class: 'pst', style: { '--c': STATE[e.state] } }, h('i'), e.state), h('span', { class: 'dim' }, ' ' + e.since),
        e.stale ? h('span', { class: 'stale' }, 'stale') : null),
      h('td', { class: 'num' }, String(e.commands.length)),
      h('td', { class: 'num' + (e.restarts >= 5 ? ' bad' : '') }, String(e.restarts)),
      h('td', {}, guideCell(e)),
      h('td', { class: 'dim' }, e.found)));
  }
  table.append(body);
  panel.append(table);
  container.append(panel);
}

function inspector(e) {
  const act = (label, cmd) => h('button', { class: 'act', onclick: () => toast(`Mockup: this would run ${cmd}.`) }, label);
  return h('div', { class: 'inspect' },
    h('div', {},
      h('h3', {}, 'What it is'),
      h('dl', { class: 'facts' },
        h('dt', {}, 'coverage'), h('dd', {}, `${e.coverage}: ${e.note}`),
        h('dt', {}, 'binary'), h('dd', { class: 'mono' }, e.path),
        h('dt', {}, 'identity'), h('dd', { class: 'mono' }, e.binary),
        h('dt', {}, 'declaration'), h('dd', {}, e.read),
        h('dt', {}, 'own GUI'), h('dd', {}, S.guis[e.id] ? `registered ${hhmm(S.guis[e.id].at)}, ${S.guis[e.id].entry}` : 'none registered'),
        e.calls !== undefined ? [h('dt', {}, 'calls today'), h('dd', {}, String(e.calls))] : null,
        e.lastExit ? [h('dt', {}, 'last exit'), h('dd', {}, e.lastExit)] : null),
      e.hint ? h('p', { class: 'hint' }, e.hint) : null,
      h('div', { class: 'acts' },
        e.state === 'down' ? act('Start', `rig up ${e.id}`) : null,
        e.state === 'quarantined' ? act('Restart', `rig restart ${e.id}`) : null,
        e.state === 'healthy' ? act('Stop', `rig stop ${e.id}`) : null,
        act('Health', `rig health ${e.id}`), act('Describe', `rig describe ${e.id}`))),
    h('div', {},
      h('h3', {}, `Commands (${e.commands.length})`),
      h('ul', { class: 'cmds' }, e.commands.map(([c, d]) => h('li', {}, h('span', { class: 'mono' }, `${e.id} ${c}`), h('span', { class: 'dim' }, d))))));
}

// Requirements 7 and 8: notifications, newest on top, evicted past an age.
function renderNotes(container) {
  evict();
  const unread = S.notes.filter((n) => n.unread).length;
  const days = h('select', { class: 'sel', 'aria-label': 'Evict notifications older than',
    onchange: (e) => { S.keepDays = Number(e.target.value); render(); } },
    ...[[1, '1 day'], [3, '3 days'], [7, '1 week'], [30, '30 days']].map(([v, l]) => h('option', { value: String(v), selected: S.keepDays === v }, l)));
  const sources = [...new Set(S.notes.map((n) => n.from))].sort();
  const q = S.nq.toLowerCase();
  const asks = (n) => Boolean(n.actions && !n.decided);
  const shown = S.notes.filter((n) => (!S.nsrc || n.from === S.nsrc) &&
    (!q || [n.title, n.body, n.from, n.details || ''].join(' ').toLowerCase().includes(q)))
    .sort((a, b) => asks(b) - asks(a) || b.at - a.at);
  const search = h('input', { id: 'nq', type: 'search', placeholder: 'Search notifications', value: S.nq, 'aria-label': 'Search notifications',
    oninput: (e) => { S.nq = e.target.value; render(); const f = $('nq'); f.focus(); f.setSelectionRange(f.value.length, f.value.length); } });
  const src = h('select', { class: 'sel', 'aria-label': 'Only from', onchange: (e) => { S.nsrc = e.target.value; render(); } },
    h('option', { value: '' }, `every source (${S.notes.length})`),
    ...sources.map((x) => h('option', { value: x, selected: S.nsrc === x }, `${x} (${S.notes.filter((n) => n.from === x).length})`)));
  const list = h('ol', { class: 'notes', 'data-scroll': 'notes' }, shown.length ? [] : h('li', { class: 'none' }, 'Nothing matches.'), shown.map((n) => h('li', { class: 'note' + (n.unread ? ' unread' : '') + (asks(n) ? ' asks' : ''), style: { '--sev': SEV[n.severity] },
    onclick: (e) => { if (!e.target.closest('.src')) openNote(n); } },
    h('span', { class: 'bullet' }),
    h('div', {},
      h('button', { class: 'nopen', 'aria-label': `${n.title}: open its details` },
        h('span', { class: 'nt' }, h('b', {}, n.title), h('time', { class: 'dim', title: new Date(n.at).toLocaleString() }, ago(n.at))),
        h('span', { class: 'dim' }, n.body)),
      h('div', { class: 'nmeta' },
        h('button', { class: 'src', title: `Only ${n.from}`, onclick: () => { S.nsrc = n.from; render(); } }, n.from),
        asks(n) ? h('span', { class: 'decide' }, 'needs your decision') : n.decided ? h('span', { class: 'dim' }, `you chose "${n.decided.choice}"`) : null)))));
  container.append(h('aside', { class: 'notepanel', 'aria-label': 'Notifications' },
    h('header', {}, h('h2', {}, 'Notifications'), unread ? h('span', { class: 'badge', style: { '--sev': 'var(--h-steel)' } }, `${unread} new`) : null,
      h('span', { class: 'sp' }), unread ? h('button', { class: 'linkish', onclick: () => { S.notes.forEach((n) => { n.unread = false; }); render(); } }, 'Mark read') : null),
    h('div', { class: 'nfilter' }, h('label', { class: 'search' }, search), h('label', { class: 'dim' }, 'From ', src)),
    list,
    h('footer', {}, h('label', { class: 'dim' }, 'Evict after ', days),
      h('div', { class: 'faint' }, S.evicted ? `${S.evicted} older one${S.evicted > 1 ? 's' : ''} evicted` : 'Nothing evicted yet'))));
}

function renderMain(view) {
  const grid = h('div', { class: 'maingrid' });
  const left = h('div', { class: 'mainleft' });
  left.append(h('div', { class: 'head' }, h('h1', {}, 'rig'), h('span', { class: 'mono dim' }, RIG.version),
    h('span', { class: 'state' }, h('i'), `${RIG.estate}, up ${RIG.up}`),
    h('span', { class: 'faint' }, 'mockup: simulated, nothing here talks to rigd')));
  left.append(h('div', { class: 'rigstrip' }, ...[
    ['epoch', String(RIG.epoch)], ['scan', RIG.scan], ['declarations kept', String(RIG.kept)], ['seats', String(RIG.seats)], ['store', RIG.store],
  ].map(([k, v]) => h('span', {}, h('span', { class: 'dim' }, k + ' '), h('b', {}, v)))));
  renderSummary(left);
  // Requirement 16: Main's parts are tabs inside it, not one long page.
  const open = [...S.cards.values()].filter((c) => !c.closed);
  const need = needsYou().length;
  const inner = [
    ['needs', 'Needs you', String(need), null],
    ['programs', 'Programs', String(estate.length), null],
    ['board', 'Board', `${open.length} open`, null],
  ];
  left.append(h('div', { class: 'subtabs', role: 'tablist', 'aria-label': 'Main' }, inner.map(([id, label, count, warn]) =>
    h('button', { role: 'tab', 'aria-selected': String(S.mainTab === id), onclick: () => { S.mainTab = id; render(); },
      class: id === 'needs' && need ? 'hot' : null,
      onkeydown: (e) => { if (e.key === 'ArrowRight' || e.key === 'ArrowLeft') { e.preventDefault(); const ids = inner.map((x) => x[0]); const i = ids.indexOf(S.mainTab); S.mainTab = ids[(i + (e.key === 'ArrowRight' ? 1 : ids.length - 1)) % ids.length]; render(); document.querySelector('.subtabs [aria-selected="true"]')?.focus(); } } },
      label, h('span', { class: 'cnt' }, count), warn ? h('span', { class: 'warn' }, warn) : null))));
  // Requirement 37: the head, the figures and these tabs stay; this scrolls.
  const body = h('div', { class: 'mainscroll', 'data-scroll': 'main:' + S.mainTab });
  if (S.mainTab === 'board') renderBoard(body); else if (S.mainTab === 'programs') renderPrograms(body); else renderNeeds(body);
  left.append(body);
  grid.append(left);
  renderNotes(grid);
  view.append(grid);
}

function renderTab(view, t) {
  const own = [...S.cards.values()].filter((c) => c.from === t.id).sort((a, b) => b.order - a.order);
  const e = estate.find((x) => x.id === t.id);
  view.append(h('div', { class: 'ptab' },
    h('div', { class: 'head' }, h('h1', {}, e ? e.name : t.title), e ? h('span', { class: 'mono dim' }, e.version) : null, h('span', { class: 'dim' }, t.kind === 'program' ? 'program' : 'agent')),
    h('div', { class: 'reason' },
      t.asked ? h('div', {}, h('b', {}, `${t.title} asked for this tab at ${hhmm(t.asked)}: `), `"${t.reason}"`)
        : h('div', {}, h('b', {}, `You picked ${e ? e.name : t.title} at ${hhmm(t.picked || clock())}.`), ' It has no GUI of its own, so this is rig\'s page about it.'),
      t.released ? h('div', { class: 'frozen' }, `It no longer needs it (since ${hhmm(t.released)}). What you see is its last state. Close the tab and it moves to Reopen.`) : h('div', { class: 'dim' }, 'Close it any time; it moves to Reopen with its last state.')),
    e ? h('section', { class: 'panel', 'aria-label': 'What rig knows' }, inspector(e)) : null,
    e ? h('h2', { class: 'subh' }, 'Its settings') : null,
    e ? settingsTable(e.id, PSET[e.id] || []) : null,
    h('h2', { class: 'subh' }, 'What it pushed'),
    own.length ? h('div', { class: 'items' }, own.map(cardNode)) : h('div', { class: 'empty' }, 'Nothing pushed yet.')));
}

function renderView() {
  const view = $('view');
  // Requirement 35: a redraw never moves a list he scrolled.
  const kept = { ['view:' + S.current]: view.scrollTop };
  for (const el of view.querySelectorAll('[data-scroll]')) kept[el.dataset.scroll] = el.scrollTop;
  view.replaceChildren();
  queueMicrotask(() => {
    if (('view:' + S.current) in kept) view.scrollTop = kept['view:' + S.current];
    for (const el of view.querySelectorAll('[data-scroll]')) if (el.dataset.scroll in kept) el.scrollTop = kept[el.dataset.scroll];
  });
  const t = S.tabs.find((x) => x.id === S.current && x.open) || S.tabs[0];
  S.current = t.id;
  const host = $('guihost');
  const gui = t.kind === 'program' && S.guis[t.id];
  view.classList.toggle('strip', Boolean(gui));
  view.classList.toggle('mainmode', t.kind === 'main');
  host.hidden = !gui;
  for (const f of host.querySelectorAll('iframe')) f.hidden = f.dataset.gui !== t.id;
  if (t.kind === 'main') renderMain(view);
  else if (t.id === 'settings') renderSettings(view);
  else if (t.id === 'capabilities') renderCaps(view);
  else if (t.id === 'guidelines') renderGuide(view);
  else if (gui) renderGuiTab(view, host, t);
  else renderTab(view, t);
}

// A program's own GUI. The strip above it is rig's; the frame is the
// program's page in a sandbox, built once and kept, so a redraw of the
// dashboard never reloads it and closing the tab keeps its last state.
function renderGuiTab(view, host, t) {
  const g = S.guis[t.id];
  const e = estate.find((x) => x.id === t.id);
  view.append(h('div', { class: 'guihead' },
    h('h1', {}, e ? e.name : t.title), e ? h('span', { class: 'mono dim' }, e.version) : null,
    h('span', { class: 'kit' }, 'its own GUI: styled by rig.css, acting through rig'),
    h('span', { class: 'dim' }, t.asked ? `${t.title} asked at ${hhmm(t.asked)}: "${t.reason}"` : `registered ${hhmm(g.at)}`),
    e ? h('button', { class: 'act', onclick: () => openProgSettings(e.id) }, `Settings (${(PSET[e.id] || []).length})`) : null,
    e ? h('button', { class: 'act', onclick: () => openAbout(e) }, 'What rig knows') : null));
  if (!host.querySelector(`iframe[data-gui="${t.id}"]`)) {
    const mode = document.documentElement.getAttribute('data-theme') === 'light' ? 'light' : 'dark';
    const css = rigCss(tokens(DEFAULTS, 'dark'), tokens(DEFAULTS, 'light'));
    const f = h('iframe', { 'data-gui': t.id, title: `${t.title}'s GUI`, sandbox: 'allow-scripts' });
    f.srcdoc = guiDoc(css, LEDGER_GUI, mode, ledger);
    host.append(f);
    wire('tab', 'gui.open', `${t.title}'s GUI loaded in a sandbox with rig.css`);
  }
}

function renderRail() {
  const r = $('railguis');
  r.replaceChildren(...Object.keys(S.guis).map((id) => {
    const e = estate.find((x) => x.id === id);
    return h('button', { class: 'railgui', title: `${e ? e.name : id}: open its tab`, 'aria-label': `${e ? e.name : id}, its GUI`,
      'aria-current': String(S.current === id), style: { '--c': progColour(id) }, onclick: () => openTab(id) }, id.slice(0, 2));
  }));
  $('railMain').setAttribute('aria-current', String(S.current === 'main'));
  for (const id of Object.keys(INTERNAL)) $('rail-' + id).setAttribute('aria-current', String(S.current === id));
}


// ── settings (requirements 19 and 20) ─────────────────────────────────────
// rig's keys come from its real schema; a program's are what it declares
// (§47 requirement 1's program layers). A change is config.set: a runtime
// override until restart for rig's own, as `rig config set` does today.
function schemaKeys(node, prefix = '') {
  return Object.entries(node.properties || {}).flatMap(([k, v]) => v.type === 'object' && v.properties ? schemaKeys(v, prefix + k + '.')
    : [{ key: prefix + k, type: v.type, enum: v.enum, min: v.minimum, def: v.default ?? '', desc: v.description || '' }]);
}
const RSET = [...schemaKeys(RIG_SCHEMA),
  { key: 'dashboard.notifications.keep.days', type: 'integer', min: 1, def: 7, desc: 'notifications older than this leave the dashboard\'s panel (plan/55 requirement 8)', proposed: true }]
  .map((x) => ({ ...x, value: x.def, origin: 'built-in default' }));
const PSET = Object.fromEntries(Object.entries({
  ledger: [['statement.dir', 'string', '~/finance/statements', 'where it reads bank statements from', null, 'dir'], ['rules.file', 'string', '~/finance/ledger-rules.toml', 'its matching rules', null, 'file'], ['match.tolerance.cents', 'integer', 0, 'how far apart two amounts may be and still match'],
    ['reconcile.on.import', 'boolean', true, 'reconcile as soon as a statement is imported'], ['currency', 'string', 'ILS', 'the books\' currency', ['ILS', 'EUR', 'USD']]],
  storeworker: [['runs.parallel', 'integer', 2, 'queued runs it works on at once'], ['retry.max', 'integer', 3, 'retries before a run is reported failed']],
  beacon: [['cards.keep.days', 'integer', 7, 'how long an answered card stays on its board'], ['board.font', 'string', 'Inter', 'the board\'s font', ['Inter', 'IBM Plex Sans']]],
  lantern: [['index.dirs', 'string', '~/me', 'folders it indexes', null, 'dirs']],
  righand: [['countdown.seconds', 'integer', 5, 'the HANDS OFF countdown before a script runs']],
}).map(([id, list]) => [id, list.map(([key, type, def, desc, en, kind]) => ({ key, type, def, desc, enum: en || undefined, kind, min: type === 'integer' ? 0 : undefined, value: def, origin: 'its declared default' }))]));
// What a key holds, so its control fits (requirement 33). rig's schema has
// no such annotation yet; §47 now owes it. Until then the mock names them.
for (const x of RSET) if (x.key === 'programs.scan') x.kind = 'dirs';
function unitOf(key) {
  if (key.endsWith('.bytes') || key.endsWith('.cap')) return 'bytes';
  if (key.endsWith('.ms')) return 'ms';
  if (key.endsWith('.days')) return 'days';
  if (key.endsWith('.seconds')) return 'seconds';
  if (key.endsWith('.cents')) return 'cents';
  return '';
}
function human(n) {
  if (!Number.isFinite(n)) return '';
  const u = ['B', 'KiB', 'MiB', 'GiB']; let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return `= ${n % 1 ? n.toFixed(1) : n} ${u[i]}`;
}
// The OS picker. The real window is native and gets a full path back; a
// browser hands over only the folder's or file's name.
async function pickPath(kind) {
  try {
    if (kind === 'file' && window.showOpenFilePicker) { const [f] = await window.showOpenFilePicker(); return f ? '…/' + f.name : null; }
    if (kind !== 'file' && window.showDirectoryPicker) { const d = await window.showDirectoryPicker(); return d ? '…/' + d.name : null; }
  } catch (e) { return null; }
  return new Promise((res) => {
    const i = h('input', { type: 'file' });
    if (kind !== 'file') i.webkitdirectory = true;
    i.onchange = () => { const f = i.files[0]; res(f ? '…/' + (kind === 'file' ? f.name : f.webkitRelativePath.split('/')[0]) : null); };
    i.click();
  });
}
function pathControl(x, label, set) {
  const list = x.kind === 'dirs' ? String(x.value).split(':').filter(Boolean) : [String(x.value)].filter(Boolean);
  const save = (items) => set(x.kind === 'dirs' ? items.join(':') : items[0] || '');
  const typing = S.typePath === label;
  if (typing) return h('div', { class: 'pathctl' },
    h('input', { class: 'inp', type: 'text', 'aria-label': label, value: String(x.value), onchange: (e) => { S.typePath = null; set(e.target.value); } }),
    h('button', { class: 'linkish', onclick: () => { S.typePath = null; set(x.value); } }, 'Done'));
  return h('div', { class: 'pathctl' },
    h('ul', { class: 'paths' }, list.map((pth, i) => h('li', {}, svg(x.kind === 'file' ? ICON.file : ICON.folder), h('span', { class: 'mono' }, pth),
      x.kind === 'dirs' && list.length > 1 ? h('button', { class: 'iconbtn', 'aria-label': `Remove ${pth}`, onclick: () => save(list.filter((_, j) => j !== i)) }, svg(ICON.x)) : null))),
    h('div', { class: 'row' },
      h('button', { class: 'act', 'aria-label': `${label}: ${x.kind === 'file' ? 'choose a file' : x.kind === 'dirs' ? 'add a folder' : 'choose a folder'}`,
        onclick: async () => { const got = await pickPath(x.kind); if (got) save(x.kind === 'dirs' ? [...list, got] : [got]); } },
        svg(x.kind === 'file' ? ICON.file : ICON.folder), x.kind === 'file' ? 'Choose file…' : x.kind === 'dirs' ? 'Add folder…' : 'Choose folder…'),
      h('button', { class: 'linkish', onclick: () => { S.typePath = label; set(x.value); } }, 'or type it')));
}
function settingsTable(owner, list) {
  if (!list.length) return h('div', { class: 'empty' }, `${owner} declares no settings.`);
  const q = S.sq.toLowerCase();
  const rows = list.filter((x) => !q || (x.key + ' ' + x.desc).toLowerCase().includes(q));
  if (!rows.length) return h('div', { class: 'empty' }, 'Nothing matches.');
  return h('table', { class: 'progs sets' },
    h('thead', {}, h('tr', {}, ...['Key', 'Value', 'Where it comes from', ''].map((x) => h('th', { scope: 'col' }, x)))),
    h('tbody', {}, rows.map((x) => settingRow(owner, x))));
}
function settingRow(owner, x) {
  const origin = h('td', { class: 'dim' }, x.origin);
  const err = h('div', { class: 'err', role: 'alert' });
  const reset = h('button', { class: 'linkish', hidden: x.value === x.def, onclick: () => set(x.def) }, 'Reset');
  const label = `${owner} ${x.key}`;
  function set(v) {
    if (x.type === 'integer' && (!Number.isInteger(v) || (x.min !== undefined && v < x.min))) {
      err.textContent = `Refused: ${x.key} takes a whole number${x.min !== undefined ? ` of at least ${x.min}` : ''}. Nothing changed.`;
      wire('refused', 'config.set', `${label} = ${String(v).slice(0, 40)}: not a valid ${x.type}`, owner); renderWire(); return;
    }
    err.textContent = '';
    x.value = v;
    x.origin = v === x.def ? (owner === 'rig' ? 'built-in default' : 'its declared default') : owner === 'rig' ? 'runtime override, until restart' : `~/.config/rig/apps/${owner}.toml`;
    origin.textContent = x.origin; reset.hidden = v === x.def;
    if (x.kind) row.replaceWith(settingRow(owner, x));
    else if (ctl.type === 'checkbox') ctl.checked = v; else { ctl.value = String(v); if (unit === 'bytes') hint.textContent = human(v); }
    wire('acted', 'config.set', `${label} = ${JSON.stringify(v)}${owner === 'rig' ? '' : `, written to apps/${owner}.toml; ${owner} reads it on its next config.get`}`, owner);
    if (x.key === 'dashboard.notifications.keep.days') { S.keepDays = v; }
    renderWire();
  }
  const unit = x.type === 'integer' ? unitOf(x.key) : '';
  const hint = h('span', { class: 'dim small' }, unit === 'bytes' ? human(Number(x.value)) : '');
  let ctl;
  if (x.kind) ctl = pathControl(x, label, set);
  else if (x.type === 'boolean') ctl = h('input', { type: 'checkbox', 'aria-label': label, onchange: (e) => set(e.target.checked) });
  else if (x.enum) ctl = h('select', { class: 'sel', 'aria-label': label, onchange: (e) => set(e.target.value) }, x.enum.map((o) => h('option', { value: o, selected: o === x.value }, o)));
  else ctl = h('input', { class: 'inp', type: x.type === 'integer' ? 'number' : 'text', 'aria-label': label, value: String(x.value),
    onchange: (e) => set(x.type === 'integer' ? Number(e.target.value) : e.target.value) });
  if (x.type === 'boolean') ctl.checked = Boolean(x.value);
  if (unit) ctl.addEventListener('input', (e) => { if (unit === 'bytes') hint.textContent = human(Number(e.target.value)); });
  const row = h('tr', {},
    h('td', {}, h('div', { class: 'mono' }, x.key, x.proposed ? h('span', { class: 'gtag' }, 'proposed') : null), h('div', { class: 'dim small' }, x.desc), err),
    h('td', {}, unit ? h('div', { class: 'unitctl' }, ctl, h('span', { class: 'dim small' }, unit === 'bytes' ? '' : unit), hint) : ctl), origin, h('td', {}, reset));
  return row;
}
function renderSettings(view) {
  const search = h('input', { id: 'sq', type: 'search', class: 'inp wide', placeholder: 'Search every setting', value: S.sq, 'aria-label': 'Search settings',
    oninput: (e) => { S.sq = e.target.value; renderView(); const f = $('sq'); f.focus(); f.setSelectionRange(f.value.length, f.value.length); } });
  // Requirement 34: rig, then one tab per program. A search counts its hits
  // on every tab, so a match in another tab is not missed.
  const q = S.sq.toLowerCase();
  const hits = (list) => list.filter((x) => !q || (x.key + ' ' + x.desc).toLowerCase().includes(q)).length;
  const owners = [['rig', 'rig', RSET], ...estate.map((e) => [e.id, e.name, PSET[e.id] || []])];
  const cur = owners.find((o) => o[0] === S.setTab) || owners[0];
  const e = estate.find((x) => x.id === cur[0]);
  view.append(h('div', { class: 'ptab' },
    h('div', { class: 'head' }, h('h1', {}, 'Settings'), h('span', { class: 'dim' }, `rig's ${RSET.length} keys, from its schema, and each program's own`), h('span', { class: 'sp' }), search),
    h('div', { class: 'subtabs', role: 'tablist', 'aria-label': 'Whose settings' }, owners.map(([id, name, list]) =>
      h('button', { role: 'tab', 'aria-selected': String(cur[0] === id), onclick: () => { S.setTab = id; renderView(); },
        onkeydown: (ev) => { if (ev.key === 'ArrowRight' || ev.key === 'ArrowLeft') { ev.preventDefault(); const i = owners.findIndex((o) => o[0] === cur[0]); S.setTab = owners[(i + (ev.key === 'ArrowRight' ? 1 : owners.length - 1)) % owners.length][0]; renderView(); document.querySelector('.ptab .subtabs [aria-selected="true"]')?.focus(); } } },
        id === 'rig' ? null : h('span', { class: 'ic', style: { '--c': progColour(id) } }, id.slice(0, 2)), name, h('span', { class: 'cnt' }, String(q ? hits(list) : list.length))))),
    e && S.guis[e.id] ? h('p', { class: 'dim small' }, `${e.name} shows these in its own GUI too, under Settings.`) : null,
    settingsTable(cur[0], cur[2])));
}
function openProgSettings(id) {
  const d = $('drawer');
  S.history = null;
  d.replaceChildren(
    h('header', {}, h('h3', {}, `${id}'s settings`), h('button', { class: 'iconbtn', 'aria-label': 'Close', onclick: () => { d.hidden = true; } }, svg(ICON.x))),
    h('div', { class: 'body' }, h('p', { class: 'dim' }, `What ${id} declares to rig. rig draws them and keeps them, so they look the same here and under Settings.`),
      settingsTable(id, PSET[id] || [])));
  d.hidden = false;
  d.querySelector('.body input, .body select')?.focus();
}

// ── capabilities (requirement 28): every verb, its words from the binary ──
const PARGS = {
  'ledger match': { type: 'object', required: ['entry'], properties: { entry: { type: 'integer', description: 'the entry id' } } },
  'ledger reject': { type: 'object', required: ['entry'], properties: { entry: { type: 'integer', description: 'the entry id' } } },
  'ledger export': { type: 'object', properties: { month: { type: 'string', description: 'YYYY-MM' } } },
  'lantern search': { type: 'object', required: ['q'], properties: { q: { type: 'string' }, limit: { type: 'integer' } } },
  'abacus rate': { type: 'object', required: ['from', 'to'], properties: { from: { type: 'string', enum: ['ILS', 'EUR', 'USD'] }, to: { type: 'string', enum: ['ILS', 'EUR', 'USD'] }, amount: { type: 'number' } } },
};
const CAPS = [
  ...RIG_SELF.map((c) => ({ owner: 'rig', key: c.ID, id: c.ID, title: c.Title, summary: c.Summary, desc: c.Description, returns: c.Returns, effects: c.Effects, args: c.Args, real: true })),
  ...estate.flatMap((e) => e.commands.map(([c, d]) => ({ owner: e.id, key: `${e.id} ${c}`, id: c, title: c, summary: d, desc: `${d}. (Mock: in rig this text is ${e.name}'s own declaration, read from its binary.)`,
    returns: '', effects: /^(list|entries|search|status|where|windows|sum|rate)$/.test(c) ? 'read-only' : 'writes-files', args: PARGS[`${e.id} ${c}`] || null }))),
];
const EFFECT_TONE = { 'read-only': 'var(--h-sage)', 'writes-files': 'var(--h-amber)', destructive: 'var(--h-rust)' };
function capGroup(c) { return c.owner === 'rig' ? 'rig ' + (c.id.includes('.') ? c.id.split('.')[0] : 'core') : c.owner; }
function argField(cap, name, sch, required) {
  const vals = S.capArgs[cap.key] = S.capArgs[cap.key] || {};
  const label = name + (required ? ' *' : '');
  const store = (v) => { if (v === '' || v === undefined) delete vals[name]; else vals[name] = v; renderCapPreview(cap); };
  let ctl;
  if (sch.type === 'boolean') { ctl = h('input', { type: 'checkbox', onchange: (e) => store(e.target.checked || undefined) }); ctl.checked = Boolean(vals[name]); }
  else if (sch.enum) ctl = h('select', { class: 'sel', onchange: (e) => store(e.target.value) }, h('option', { value: '' }, '(none)'), sch.enum.map((o) => h('option', { value: o, selected: vals[name] === o }, o)));
  else if (sch.type === 'integer' || sch.type === 'number') ctl = h('input', { class: 'inp', type: 'number', value: vals[name] ?? '', oninput: (e) => store(e.target.value === '' ? '' : Number(e.target.value)) });
  else if (sch.type === 'array') ctl = h('input', { class: 'inp', type: 'text', placeholder: 'comma, separated', value: (vals[name] || []).join(', '), oninput: (e) => store(e.target.value ? e.target.value.split(',').map((x) => x.trim()).filter(Boolean) : '') });
  else if (sch.type === 'object') ctl = h('textarea', { class: 'inp', rows: '3', placeholder: '{ JSON }', oninput: (e) => { try { store(e.target.value ? JSON.parse(e.target.value) : ''); e.target.removeAttribute('aria-invalid'); } catch { e.target.setAttribute('aria-invalid', 'true'); } } }, vals[name] ? JSON.stringify(vals[name]) : '');
  else ctl = h('input', { class: 'inp', type: 'text', value: vals[name] ?? '', oninput: (e) => store(e.target.value) });
  ctl.setAttribute('aria-label', `${cap.id} ${name}`);
  return h('label', { class: 'arg' }, h('span', { class: 'mono' }, label), h('span', { class: 'dim small' }, [sch.type || 'any', sch.description].filter(Boolean).join(': ')), ctl);
}
function capCall(cap) {
  const args = S.capArgs[cap.key] || {};
  return cap.owner === 'rig' ? { verb: cap.id, args } : { verb: 'invoke', args: { program: cap.owner, command: cap.id, args } };
}
function renderCapPreview(cap) {
  const pre = $('capcall');
  if (pre) pre.textContent = JSON.stringify(capCall(cap), null, 2);
}
function tryCap(cap) {
  const args = S.capArgs[cap.key] || {};
  const missing = (cap.args?.required || []).filter((r) => args[r] === undefined);
  if (missing.length) { S.capOut[cap.key] = { ok: false, text: `Refused before sending: ${missing.join(', ')} ${missing.length > 1 ? 'are' : 'is'} required.` }; renderView(); return; }
  if (cap.effects !== 'read-only' && S.capConfirm !== cap.key) { S.capConfirm = cap.key; renderView(); return; }
  S.capConfirm = null;
  wire('acted', cap.owner === 'rig' ? cap.id : 'invoke', `from Capabilities: ${cap.owner === 'rig' ? '' : cap.owner + ' '}${cap.id} ${JSON.stringify(args)}`, cap.owner === 'rig' ? 'rig' : cap.owner);
  S.capOut[cap.key] = { ok: true, text: JSON.stringify({ mockup: 'nothing reached rigd', verb: capCall(cap).verb, would_return: cap.returns || 'what the program answers' }, null, 2) };
  renderView(); renderWire();
}
function renderCaps(view) {
  const q = S.cq.toLowerCase();
  const list = CAPS.filter((c) => !q || [c.key, c.title, c.summary, c.desc].join(' ').toLowerCase().includes(q));
  const groups = new Map();
  for (const c of list) { const g = capGroup(c); if (!groups.has(g)) groups.set(g, []); groups.get(g).push(c); }
  const cap = CAPS.find((c) => c.key === S.cap) || CAPS[0];
  const search = h('input', { id: 'cq', type: 'search', class: 'inp', placeholder: `Search ${CAPS.length} capabilities`, value: S.cq, 'aria-label': 'Search capabilities',
    oninput: (e) => { S.cq = e.target.value; renderView(); const f = $('cq'); f.focus(); f.setSelectionRange(f.value.length, f.value.length); } });
  const left = h('nav', { class: 'caplist', 'data-scroll': 'caps', 'aria-label': 'Capabilities' }, search,
    list.length ? [...groups.entries()].map(([g, cs]) => h('div', {}, h('h5', {}, g, h('span', { class: 'dim' }, ` ${cs.length}`)),
      cs.map((c) => h('button', { 'aria-current': String(c.key === cap.key), onclick: () => { S.cap = c.key; renderView(); $('capdetail')?.focus(); } },
        h('i', { style: { '--c': EFFECT_TONE[c.effects] || 'var(--fg-faint)' }, title: c.effects }), h('span', { class: 'mono' }, c.id), h('span', { class: 'dim' }, c.summary))))) : h('div', { class: 'empty' }, 'Nothing matches.'));
  const props = Object.entries(cap.args?.properties || {});
  const out = S.capOut[cap.key];
  const right = h('section', { class: 'capdetail panel', id: 'capdetail', tabindex: '-1', 'aria-label': cap.key },
    h('header', {}, h('h2', { class: 'mono' }, cap.owner === 'rig' ? cap.id : `${cap.owner} ${cap.id}`), h('span', { class: 'eff', style: { '--c': EFFECT_TONE[cap.effects] || 'var(--fg-faint)' } }, cap.effects),
      h('span', { class: 'sp' }), h('span', { class: 'dim small' }, cap.real ? 'words from rig\'s own declaration' : 'mock words; rig reads them from the program')),
    h('div', { class: 'capbody' },
      h('p', {}, h('b', {}, cap.summary)), h('p', {}, cap.desc), cap.returns ? h('p', { class: 'dim' }, h('b', {}, 'Returns: '), cap.returns) : null,
      h('h3', {}, 'Try it'),
      props.length ? h('div', { class: 'args' }, props.map(([n, sch]) => argField(cap, n, sch, (cap.args.required || []).includes(n)))) : h('p', { class: 'dim' }, 'It takes no arguments.'),
      h('h4', { class: 'dim small' }, 'What goes on the wire'),
      h('pre', { class: 'call', id: 'capcall' }, JSON.stringify(capCall(cap), null, 2)),
      S.capConfirm === cap.key ? h('div', { class: 'confirm', role: 'alert' }, h('b', {}, `This one ${cap.effects === 'destructive' ? 'destroys' : 'changes'} state (${cap.effects}).`), ' Run it?',
        h('div', { class: 'acts' }, h('button', { class: 'act primary', onclick: () => tryCap(cap) }, 'Run it'), h('button', { class: 'act', onclick: () => { S.capConfirm = null; renderView(); } }, 'Cancel')))
        : h('div', { class: 'acts' }, h('button', { class: 'act primary', onclick: () => tryCap(cap) }, 'Send')),
      out ? h('pre', { class: 'call ' + (out.ok ? 'ok' : 'bad') }, out.text) : null));
  view.append(h('div', { class: 'ptab caps' },
    h('div', { class: 'head' }, h('h1', {}, 'Capabilities'), h('span', { class: 'dim' }, `${CAPS.filter((c) => c.real).length} of rig's own, and every registered program's commands`)),
    h('div', { class: 'capgrid' }, left, right)));
}

// ── guidelines (requirement 29): what a program must be, each rule dated ──
// Dates are when the rule entered rig's code (git), or the day he ruled it
// where nothing is built yet.
const GUIDE = [
  { id: 'G1', date: '2026-09-10', who: 'programs', state: 'in force', title: 'Declare every command in full', body: 'Each command states its effects, idempotence, sensitive fields, interactivity, streaming, display need, duration and confirmation. A registration missing any of them is refused.', cite: 'internal/kernel/declaration.go' },
  { id: 'G2', date: '2026-09-11', who: 'programs', state: 'in force', title: 'Name the kit elements your page uses', body: 'The element list is part of the registration. Config may take elements away, never add one.', cite: 'plan/05 §5h R3' },
  { id: 'A1', date: '2026-09-16', who: 'agents', state: 'in force', title: 'Announce a seat and say what you are for', body: 'Take a seat with announce before anything else, and keep set_activity current.', cite: 'internal/mcpserver' },
  { id: 'A2', date: '2026-09-25', who: 'agents', state: 'in force', title: 'Keep your notes in rig', body: 'Working notes go through worknote, so a successor finds them after a restart.', cite: 'internal/mcpserver' },
  { id: 'G3', date: '2026-10-01', who: 'programs', state: 'in force', title: 'Declare the events you publish', body: 'A program publishes only kinds it declared, under its own id (ledger.changed). Bare kinds are rig\'s.', cite: 'plan/52 E3' },
  { id: 'G4', date: '2026-10-03', who: 'programs', state: 'in force', title: 'Declare how you load', body: 'Resident, or on call. An on-call program tells rig when it is idle (client.Idle) and when it must not be stopped (client.Hold).', cite: 'plan/54, decision 0265' },
  { id: 'G5', date: '2026-10-03', who: 'programs', state: 'ruled, not built', title: 'A GUI wears rig.css and acts through rig', body: 'A program\'s GUI carries no stylesheet of its own and reaches its program only through rig\'s verbs: invoke, the store, the bus, the queue, toasts.', cite: 'plan/55 requirements 12 and 13' },
  { id: 'G6', date: '2026-10-03', who: 'programs', state: 'ruled, not built', title: 'Declare your settings as a schema', body: 'rig shows them under Settings and in your GUI, changes them, and keeps where each value came from.', cite: 'plan/55 requirement 20, plan/47' },
];
const GUIDE_REV = GUIDE.map((g) => g.date).sort().at(-1);
const BUILT_TO = { beacon: '2026-10-01', ledger: '2026-10-03', lantern: '2026-10-02', righand: '2026-09-27', storeworker: '2026-10-03', keeper: '2026-10-01', abacus: '2026-09-20' };
function newerFor(id) { return GUIDE.filter((g) => g.who === 'programs' && g.date > (BUILT_TO[id] || '0')); }
function guideCell(e) {
  const n = newerFor(e.id).length;
  return h('button', { class: 'gstat' + (n ? ' behind' : ''), title: n ? `${n} rule${n > 1 ? 's' : ''} newer than what ${e.name} was built to` : `built to the current guidelines, ${GUIDE_REV}`,
    onclick: () => { S.gfocus = e.id; openTab('guidelines'); } }, n ? `${n} newer` : 'current');
}
function renderGuide(view) {
  const f = S.gfocus;
  view.append(h('div', { class: 'ptab' },
    h('div', { class: 'head' }, h('h1', {}, 'Guidelines'), h('span', { class: 'mono dim' }, `revision ${GUIDE_REV}`),
      h('span', { class: 'dim' }, 'What a program or an agent must be to work with rig. Every rule carries the day it took effect.')),
    h('section', { class: 'panel' }, h('header', {}, h('h2', {}, 'Programs against the guidelines'), h('span', { class: 'dim' }, 'mock: the revision each was built to')),
      h('table', { class: 'progs' }, h('thead', {}, h('tr', {}, ...['Program', 'Built to', 'Rules since then'].map((x) => h('th', { scope: 'col' }, x)))),
        h('tbody', {}, estate.map((e) => { const nw = newerFor(e.id); return h('tr', { class: f === e.id ? 'open' : '' },
          h('td', {}, e.name), h('td', { class: 'mono' }, BUILT_TO[e.id] || 'unknown'),
          h('td', {}, nw.length ? nw.map((g) => h('span', { class: 'gtag warnt' }, g.id)) : h('span', { class: 'dim' }, 'none: current'))); })))),
    h('h2', { class: 'subh' }, 'The rules, newest first'),
    h('ol', { class: 'rules' }, [...GUIDE].sort((a, b) => b.date.localeCompare(a.date) || b.id.localeCompare(a.id)).map((g) => {
      const behind = g.who === 'programs' ? estate.filter((e) => g.date > (BUILT_TO[e.id] || '0')).map((e) => e.name) : [];
      return h('li', { class: f && behind.includes(estate.find((e) => e.id === f)?.name) ? 'hit' : '' },
        h('div', { class: 'rhead' }, h('span', { class: 'mono' }, g.id), h('time', { class: 'mono' }, g.date), h('b', {}, g.title),
          h('span', { class: 'gtag' }, g.who), h('span', { class: 'gtag' + (g.state === 'in force' ? '' : ' warnt') }, g.state)),
        h('p', {}, g.body), h('div', { class: 'dim small' }, g.cite, behind.length ? ` · built before it: ${behind.join(', ')}` : '')); }))));
}

function wireScope() {
  const t = S.tabs.find((x) => x.id === S.current && x.open);
  return t && t.kind === 'program' ? t.id : null;
}
function renderWire() {
  $('wire').hidden = !S.showWire;
  const prog = wireScope(), only = prog && !S.wireAll, q = S.wq.toLowerCase();
  const rows = S.wire.filter((w) => (!only || w.who === prog) && (!q || [w.verb, w.text, w.who].join(' ').toLowerCase().includes(q)));
  $('wireScope').replaceChildren(
    prog ? h('button', { class: 'chip', 'aria-pressed': String(only), title: only ? 'Show every program\'s lines' : `Show only ${prog}'s lines`, onclick: () => { S.wireAll = !S.wireAll; renderWire(); } }, only ? `only ${prog}` : 'every program') : h('span', { class: 'dim' }, 'every program'),
    h('span', { class: 'dim' }, `${rows.length} of ${S.wire.length}`));
  $('wireLog').replaceChildren(...(rows.length ? rows.map((w) => h('li', {}, h('span', { class: 'faint' }, hhmm(w.at)), h('span', { class: 'k-' + w.kind }, w.verb), h('span', {}, w.text))) : [h('li', { class: 'dim' }, 'Nothing matches.')]));
}

function openHistory(id) {
  const c = S.cards.get(id);
  S.history = { id, at: c.versions.length - 1 };
  renderHistory();
}
function renderHistory() {
  const d = $('drawer');
  if (!S.history) { d.hidden = true; return; }
  const c = S.cards.get(S.history.id);
  const i = S.history.at, v = c.versions[i], prev = c.versions[i - 1] || {};
  const mark = (k, node) => (i > 0 && JSON.stringify(prev[k]) !== JSON.stringify(v[k])) ? h('span', { class: 'changed' }, node) : node;
  d.replaceChildren(
    h('header', {}, h('h3', {}, c.title), h('button', { class: 'iconbtn', 'aria-label': 'Close the history', onclick: () => { S.history = null; renderHistory(); } }, svg(ICON.x))),
    h('div', { class: 'body' },
      h('p', { class: 'dim' }, `${c.from}, ${c.kind}. Every version is kept whole. Step with the arrow keys.`),
      h('div', { class: 'vers', role: 'tablist', 'aria-label': 'Versions' }, c.versions.map((x, j) => h('button', { 'aria-current': String(j === i), onclick: () => { S.history.at = j; renderHistory(); } }, `rev ${x.rev}`))),
      h('dl', { class: 'facts' },
        h('dt', {}, 'at'), h('dd', {}, hhmm(v.at)),
        h('dt', {}, 'status'), h('dd', {}, mark('status', v.status || '-')),
        h('dt', {}, 'severity'), h('dd', {}, mark('severity', v.severity || '-')),
        h('dt', {}, 'progress'), h('dd', {}, mark('progress', typeof v.progress === 'number' ? Math.round(v.progress * 100) + '%' : '-')),
        h('dt', {}, 'body'), h('dd', {}, mark('body', v.body || '-')),
        ...(v.facts || []).flatMap((f) => [h('dt', {}, f.label), h('dd', {}, f.value)]),
        h('dt', {}, 'closed'), h('dd', {}, mark('closed', v.closed ? hhmm(v.closed) : '-'))),
      c.actions && !c.closed ? h('div', { class: 'acts', style: { marginTop: '16px' } }, c.actions.map((a) => h('button', { class: 'act', onclick: () => { acted(c, a.toLowerCase()); render(); } }, a))) : null));
  d.hidden = false;
  d.querySelector('[aria-current="true"]')?.focus();
}

// Requirement 32: a notification's details are a card in the centre of the
// screen, modal, closed by its button, Esc, or a click outside it.
function openNote(n) {
  const m = $('modal');
  S.noteOpen = n.id;
  n.unread = false;
  const close = () => { S.noteOpen = null; m.close(); };
  const opts = n.actions ? h('div', { class: 'acts' }, n.actions.map((a, i) => {
    const chosen = n.decided && n.decided.choice === a;
    return h('button', { class: 'act' + (chosen ? ' chosen-act' : '') + (!n.decided && i === 0 ? ' primary' : ''), disabled: Boolean(n.decided), 'aria-pressed': n.decided ? String(chosen) : null,
      onclick: () => { decideNote(n, a); render(); openNote(n); } }, chosen ? `✓ ${a}` : a); })) : null;
  m.replaceChildren(
    h('header', {}, h('span', { class: 'sevdot', style: { '--sev': SEV[n.severity] } }), h('h3', { id: 'modalTitle' }, n.title),
      h('button', { class: 'iconbtn', 'aria-label': 'Close', onclick: close }, svg(ICON.x))),
    h('div', { class: 'body' },
      h('dl', { class: 'facts' },
        h('dt', {}, 'from'), h('dd', {}, n.from),
        h('dt', {}, 'severity'), h('dd', {}, n.severity),
        h('dt', {}, 'at'), h('dd', {}, new Date(n.at).toLocaleString()),
        ...(n.facts || []).flatMap(([k, v]) => [h('dt', {}, k), h('dd', {}, v)])),
      h('p', {}, n.body),
      n.details ? h('p', { class: 'dim' }, n.details) : null,
      h('h4', { class: 'subh' }, n.actions ? (n.decided ? 'Decided' : 'Your decision') : 'Nothing to decide'),
      n.decided ? h('p', { class: 'chosen' }, `You chose "${n.decided.choice}" at ${new Date(n.decided.at).toLocaleTimeString()}. ${n.from} was told.`) : null,
      n.actions ? opts : h('p', { class: 'dim' }, 'This notification only informs.')),
    h('footer', {},
      h('button', { class: 'act', onclick: () => { S.nsrc = n.from; close(); render(); } }, `Only ${n.from}'s notifications`),
      h('span', { class: 'sp' }), h('button', { class: 'act', onclick: close }, 'Close')));
  if (!m.open) m.showModal();
  (m.querySelector('.body button:not([disabled])') || m.querySelector('footer .act:last-child')).focus();
}
$('modal').addEventListener('click', (e) => { if (e.target === $('modal')) { S.noteOpen = null; $('modal').close(); } });
$('modal').addEventListener('close', () => { S.noteOpen = null; render(); });

function openAbout(e) {
  const d = $('drawer');
  S.history = null;
  d.replaceChildren(
    h('header', {}, h('h3', {}, `${e.name}: what rig knows`), h('button', { class: 'iconbtn', 'aria-label': 'Close', onclick: () => { d.hidden = true; } }, svg(ICON.x))),
    h('div', { class: 'body' }, inspector(e)));
  d.hidden = false;
  d.querySelector('button')?.focus();
}

// Open questions go to him one at a time, each with its choice shown in
// the page rather than described (plan/55, after requirement 15).
const QUESTIONS = [
  { q: 'Is the board in the right place?',
    body: 'Main has three inner tabs: Needs you, Programs and Board. The board of agents\' cards is the third, with a tab per source inside it. Is that its place?',
    show: ['Show me the board tab', () => { S.current = 'main'; S.mainTab = 'board'; }] },
  { q: 'Do Ledger\'s buttons use the right rig capability?',
    body: 'Open Ledger, turn on the Wire, and press Match, Not a match and Reconcile again. Each line on the Wire names the rig capability that carried it: invoke, toast, queue, store, the bus.',
    show: ['Open Ledger with the Wire on', () => { S.showWire = true; openTab('ledger'); }] },
  { q: 'May agents\' cards be on the dashboard at all?',
    body: 'Section 11 rule 20 keeps agent chatter off the dashboard. I read "chatter" as the stream of every agent call, which is not shown here. The board only holds cards an agent writes to you on purpose. Does that reading hold?',
    show: ['Show me a card an agent wrote', () => { S.current = 'main'; S.mainTab = 'board'; }] },
];
function openQuestions(i = S.qi || 0) {
  const d = $('drawer');
  S.history = null; S.qi = i;
  const q = QUESTIONS[i];
  d.replaceChildren(
    h('header', {}, h('h3', {}, `Question ${i + 1} of ${QUESTIONS.length}`), h('button', { class: 'iconbtn', 'aria-label': 'Close', onclick: () => { d.hidden = true; } }, svg(ICON.x))),
    h('div', { class: 'body' },
      h('p', {}, h('b', {}, q.q)),
      h('p', { class: 'dim' }, q.body),
      h('div', { class: 'acts', style: { marginTop: '14px' } },
        h('button', { class: 'act', onclick: () => { q.show[1](); render(); openQuestions(i); } }, q.show[0])),
      h('div', { class: 'acts', style: { marginTop: '20px' } },
        i > 0 ? h('button', { class: 'act', onclick: () => openQuestions(i - 1) }, 'Previous') : null,
        i < QUESTIONS.length - 1 ? h('button', { class: 'act', onclick: () => openQuestions(i + 1) }, 'Next question') : null)));
  d.hidden = false;
  d.querySelector('.body button')?.focus();
}

let toastTimer;
function toast(text) {
  const t = $('toast');
  t.textContent = text; t.hidden = false;
  clearTimeout(toastTimer); toastTimer = setTimeout(() => { t.hidden = true; }, 2600);
}

function render() {
  renderRail();
  renderTabs();
  renderView();
  renderWire();
  flash.clear();
}

// ── keyboard ───────────────────────────────────────────────────────────────
document.addEventListener('keydown', (e) => {
  const typing = e.target instanceof HTMLInputElement || e.target instanceof HTMLSelectElement || e.target instanceof HTMLTextAreaElement;
  if (e.key === '/' && !typing) { e.preventDefault(); $('q')?.focus(); return; }
  if ($('modal').open) return;   // the dialog answers its own Esc
  if (e.key === 'Escape') {
    if (!$('menu').hidden) { $('menu').hidden = true; $('reopenBtn')?.focus(); return; }
    if (!$('drawer').hidden) { S.history = null; $('drawer').hidden = true; return; }
    if (typing && e.target.id === 'q') { S.q = ''; render(); return; }
  }
  if (S.history && !$('drawer').hidden && (e.key === 'ArrowLeft' || e.key === 'ArrowRight')) {
    const c = S.cards.get(S.history.id);
    S.history.at = Math.max(0, Math.min(c.versions.length - 1, S.history.at + (e.key === 'ArrowRight' ? 1 : -1)));
    renderHistory();
  }
});
document.addEventListener('click', (e) => {
  const m = $('menu');
  if (!m.hidden && !m.contains(e.target) && e.target.closest('#reopenBtn') === null) m.hidden = true;
});
$('wireClose').addEventListener('click', () => { S.showWire = false; render(); });
$('wq').addEventListener('input', (e) => { S.wq = e.target.value; renderWire(); });
$('railMain').addEventListener('click', () => { S.current = 'main'; render(); });
for (const id of Object.keys(INTERNAL)) $('rail-' + id).addEventListener('click', () => openTab(id));

render();
// While a field has the keyboard, the page is not redrawn under it.
setInterval(() => {
  const a = document.activeElement;
  const busy = a instanceof HTMLInputElement || a instanceof HTMLSelectElement || a instanceof HTMLTextAreaElement;
  if (S.playing && !busy) tick();
}, 2200);
window.__mock = { S, tick, render };
