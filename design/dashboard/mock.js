// The management dashboard, as a mockup Boris tries in his own browser
// (plan/55). Nothing here talks to rigd: a simulation stands in for the
// programs and agents that will call rig.panel.put, and every push is shown
// on the wire drawer as the approved path would carry it.
import { DEFAULTS, tokens } from '../theme.js';

// ── theme ──────────────────────────────────────────────────────────────────
function applyTheme() {
  const mode = document.documentElement.getAttribute('data-theme') === 'light' ? 'light' : 'dark';
  const t = tokens(DEFAULTS, mode);
  for (const [k, v] of Object.entries(t)) document.documentElement.style.setProperty(k, v);
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
    restarts: 0, calls: 14, lastExit: 'exit 0, idle', binary: '2049:1311742:9.8 MB:10:41', read: '10:41, kept', pane: true },
  { id: 'ledger', name: 'Ledger', version: '1.2.0', load: 'resident', state: 'healthy', since: '2 h', found: 'programs.json', path: '~/.local/bin/ledger',
    coverage: 'full', note: 'everything but streams', commands: [['reconcile', 'match entries against the bank'], ['entries', 'list entries'], ['export', 'write a CSV']],
    restarts: 0, binary: '2049:1310012:10.2 MB:08:39', read: '08:39, at its start', pane: true },
  { id: 'lantern', name: 'Lantern', version: '0.8.0', load: 'on call', state: 'running', since: '30 s', found: 'scan', path: '~/.local/lib/rig/apps/lantern',
    coverage: 'partial', note: 'search only', commands: [['search', 'full-text search'], ['index', 'reindex a folder']],
    restarts: 0, calls: 1, binary: '2049:1311790:9.3 MB:09:02', read: '09:02, kept', pane: false },
  { id: 'righand', name: 'righand', version: '0.9.4', load: 'resident', state: 'healthy', since: '2 h', found: 'programs.json', path: '~/.local/bin/righand',
    coverage: 'partial', note: 'scripts, windows and where', commands: [['script', 'run a desktop script'], ['windows', 'list windows'], ['where', 'locate a window']],
    restarts: 1, binary: '2049:1310455:7.1 MB:08:39', read: '08:39, at its start', pane: false },
  { id: 'storeworker', name: 'storeworker', version: '0.4.0', load: 'resident', state: 'healthy', since: '41 min', found: 'programs.json', path: '~/.local/bin/storeworker',
    coverage: 'partial', note: 'a demonstration: store, queue, leases', commands: [['run', 'run a queued job'], ['status', 'what it is doing']],
    restarts: 2, binary: '2049:1310460:8.0 MB:10:12', read: '10:12, at a restart', pane: false },
  { id: 'keeper', name: 'keeper', version: 'v3', load: 'resident', state: 'down', since: '20 min', found: 'scan', path: '~/.local/lib/rig/apps/keeper',
    coverage: 'partial', note: 'the wire only', commands: [['greet', 'say hello']],
    restarts: 0, lastExit: 'stopped by a human', binary: '2049:1311802:6.4 MB:10:33', read: '10:33, a declare run', pane: false,
    hint: 'A call is refused with: rig up keeper' },
  { id: 'abacus', name: 'abacus', version: '2.0.1', load: 'resident', state: 'quarantined', stale: true, since: '1 h', found: 'scan', path: '~/.local/lib/rig/apps/abacus',
    coverage: 'partial', note: 'sums and rates', commands: [['sum', 'add a column'], ['rate', 'convert a currency']],
    restarts: 5, lastExit: 'exit 2, five times in 4 min', binary: '2049:1311655:7.7 MB:09:47', read: '09:20, before its rebuild', pane: false,
    hint: 'Its binary changed after it was quarantined: what is listed is stale until rig restart abacus' },
];

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
function wire(kind, verb, text) {
  S.wire.unshift({ at: clock(), kind, verb, text });
  S.wire.length = Math.min(S.wire.length, 120);
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
  wire('put', 'rig.panel.put', `${from} -> ${c.id} rev ${c.rev} (${changed})`);
  wire('store', 'store', `panel/${c.id}.${c.rev} kept whole`);
  wire('bus', 'panel.changed', `-> window`);
  S.dismissed.delete(c.from);
  if (S.minimised) S.unseen.push(c.severity || 'info');
  flash.add(c.id);
  return c.id;
}
function close(from, id, fields = {}) {
  put(from, { ...fields, closed: clock() }, id);
}
function notify(from, severity, title, body, at) {
  S.notes.unshift({ id: ++seq, from, severity, title, body, at: at ?? clock(), unread: true });
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
  () => { ids.led = put('ledger', { title: 'Reconciliation finished', status: 'done', severity: 'success', body: '412 entries matched, 3 need a look.', facts: [{ label: 'matched', value: '412' }, { label: 'open', value: '3' }], actions: ['Review'] }); requestTab('ledger', '3 entries need you'); notify('ledger', 'success', 'Reconciliation finished', '3 entries need you'); },
  () => { put('storeworker', { progress: 0.62 }, ids.sw); },
  () => { ids.ci = put('rig-lead', { title: 'make ci', status: 'failed', severity: 'error', body: 'wire golden: Program gained two fields. Re-record with -update.', actions: ['Retry', 'Open log'] }); notify('rig-lead', 'error', 'make ci failed', 'wire golden: Program gained two fields'); },
  () => { put('storeworker', { progress: 0.88 }, ids.sw); },
  () => { put('rig-lead', { status: 'done', severity: 'success', body: 'Re-recorded the golden; all gates green.' }, ids.ci); },
  () => { put('beacon', { title: 'Card drawn: deploy now?', status: 'waiting', severity: 'warning', body: 'Asked by rig-lead. Options: yes, later.', actions: ['Yes', 'Later'] }); requestTab('beacon', 'a question for you'); notify('beacon', 'warning', 'A question for you', 'deploy now?'); },
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
  requestTab('storeworker', 'run 417 needs a retry decision');
  put('storeworker', { title: 'Run 417 failed: disk full', status: 'failed', severity: 'error', body: 'Freed 2 GB since. Retry?', actions: ['Retry'] });
  S.tabs.find((x) => x.id === 'storeworker').open = false;
  const day = 86400e3, now = Date.now();
  notify('rig', 'info', 'rig started', 'production, epoch 108', now - 2.2 * 3600e3);
  notify('rig', 'warning', 'abacus quarantined', 'exit 2, five times in 4 minutes', now - 3600e3);
  notify('rig', 'info', 'keeper stopped', 'by a human, from rig stop', now - 20 * 60e3);
  notify('storeworker', 'error', 'Run 417 failed', 'disk full', now - 3 * 3600e3);
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
      t.kind === 'main' ? null : h('span', { class: 'dot', style: { '--sev': SEV[worst(own.map((c) => c.severity))] } }),
      h('span', { class: 'who' }, t.title),
      t.kind === 'main' ? null : h('span', { class: 'kind' }, t.kind),
      t.kind === 'main' ? null : h('button', { class: 'x', 'aria-label': `Close ${t.title}'s tab`, onclick: (e) => { e.stopPropagation(); t.open = false; t.closedAt = clock(); if (S.current === t.id) S.current = 'main'; render(); } }, svg(ICON.x)));
    bar.append(tab);
  }
  const closed = S.tabs.filter((x) => !x.open);
  bar.append(h('span', { class: 'spacer' }),
    h('button', { class: 'tool', id: 'reopenBtn', 'aria-haspopup': 'menu', onclick: (e) => openMenu(e) }, svg(ICON.reopen), 'Reopen ', h('b', {}, String(closed.length))),
    h('button', { class: 'tool', 'aria-label': S.playing ? 'Pause the simulation' : 'Play the simulation', onclick: () => { S.playing = !S.playing; render(); } }, svg(S.playing ? ICON.pause : ICON.play), S.playing ? 'Live' : 'Paused'),
    h('button', { class: 'tool', 'aria-label': 'One step of the simulation', onclick: tick }, svg(ICON.step), 'Step'),
    h('button', { class: 'tool', 'aria-pressed': String(S.showWire), onclick: () => { S.showWire = !S.showWire; render(); } }, svg(ICON.wire), 'Wire'),
    h('button', { class: 'tool', onclick: () => openQuestions() }, svg(ICON.q), 'Open questions ', h('b', {}, '3')),
    h('button', { class: 'tool', 'aria-label': 'Switch light and dark', onclick: () => { const r = document.documentElement; r.setAttribute('data-theme', r.getAttribute('data-theme') === 'light' ? 'dark' : 'light'); } }, svg(ICON.sun)));
}

function openMenu() {
  const m = $('menu');
  const closed = S.tabs.filter((x) => !x.open);
  m.replaceChildren(h('h5', {}, 'Tabs opened before. Reopen one to see its last state.'),
    ...(closed.length ? closed.map((t) => h('button', { onclick: () => { t.open = true; t.fresh = false; S.current = t.id; m.hidden = true; render(); } },
      h('span', {}, t.title), h('span', { class: 'dim' }, t.kind),
      h('small', {}, `asked ${hhmm(t.asked)}: "${t.reason}"` + (t.closedAt ? `, closed ${hhmm(t.closedAt)}` : '')))) : [h('div', { class: 'none' }, 'None yet. A tab appears here once you close it.')]));
  m.hidden = !m.hidden;
  if (!m.hidden) m.querySelector('button')?.focus();
}

function cardNode(c) {
  const st = STATUS[c.status] || { tone: 'var(--fg-dim)' };
  const node = h('button', { class: 'card' + (c.closed ? ' closed' : '') + (flash.has(c.id) ? ' flash' : ''), style: { '--sev': SEV[c.severity] || 'var(--border)' },
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
    c.actions && !c.closed && c.status !== 'done' ? h('div', { class: 'acts' }, c.actions.map((a) => h('span', { class: 'act', role: 'button', tabindex: '0',
      onclick: (e) => { e.stopPropagation(); acted(c, a.toLowerCase()); render(); },
      onkeydown: (e) => { if (e.key === 'Enter') { e.stopPropagation(); e.preventDefault(); acted(c, a.toLowerCase()); render(); } } }, a))) : null,
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
  const sorted = [...groups.entries()].sort((a, b) => Math.max(...b[1].map((c) => c.updated)) - Math.max(...a[1].map((c) => c.updated)));
  for (const [k, list] of sorted) {
    list.sort((a, b) => b.order - a.order);
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
  panel.append(board);
  container.append(panel);
}

function renderSummary(container) {
  const all = [...S.cards.values()].filter((c) => !c.closed || clock() - c.closed < 3600e3);
  const n = (st) => all.filter((c) => c.status === st).length;
  const fig = (cls, num, label) => h('div', { class: 'fig ' + cls }, h('div', { class: 'n' }, String(num)), h('div', { class: 'l' }, label));
  container.append(h('div', { class: 'figs' },
    fig('', estate.length, 'programs'),
    fig('good', estate.filter((e) => e.state === 'healthy' || e.state === 'running').length, 'up'),
    fig('', estate.filter((e) => e.state === 'at rest').length, 'at rest, on call'),
    fig(estate.some((e) => e.state === 'down' || e.state === 'quarantined') ? 'bad' : '', estate.filter((e) => e.state === 'down' || e.state === 'quarantined').length, 'down or quarantined'),
    fig('warn', n('waiting'), 'waiting on you'),
    fig('run', n('running'), 'agents working')));
}

// Requirement 6: rig and every program it represents, versions first, each
// open to inspection. A row opens in place, so the list never moves.
function renderPrograms(container) {
  const panel = h('section', { class: 'panel', 'aria-label': 'Programs' },
    h('header', {}, h('h2', {}, 'Programs'), h('span', { class: 'dim' }, `${estate.length}, scanned in ${RIG.scan} and declared in programs.json`)));
  const table = h('table', { class: 'progs' },
    h('thead', {}, h('tr', {}, ...['Program', 'Version', 'Load', 'State', 'Commands', 'Restarts', 'Found by'].map((x) => h('th', { scope: 'col' }, x)))));
  const body = h('tbody');
  for (const e of estate) {
    const open = S.open === e.id;
    body.append(h('tr', { class: 'row' + (open ? ' open' : '') },
      h('td', {}, h('button', { class: 'pname', 'aria-expanded': String(open), onclick: () => { S.open = open ? null : e.id; render(); } },
        h('span', { class: 'ic' }, e.id.slice(0, 2)), e.name)),
      h('td', { class: 'mono' }, e.version),
      h('td', {}, e.load),
      h('td', {}, h('span', { class: 'pst', style: { '--c': STATE[e.state] } }, h('i'), e.state), h('span', { class: 'dim' }, ' ' + e.since),
        e.stale ? h('span', { class: 'stale' }, 'stale') : null),
      h('td', { class: 'num' }, String(e.commands.length)),
      h('td', { class: 'num' + (e.restarts >= 5 ? ' bad' : '') }, String(e.restarts)),
      h('td', { class: 'dim' }, e.found)));
    if (open) body.append(h('tr', { class: 'insp' }, h('td', { colspan: '7' }, inspector(e))));
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
        h('dt', {}, 'own pane'), h('dd', {}, e.pane ? 'yes' : 'no'),
        e.calls !== undefined ? [h('dt', {}, 'calls today'), h('dd', {}, String(e.calls))] : null,
        e.lastExit ? [h('dt', {}, 'last exit'), h('dd', {}, e.lastExit)] : null),
      e.hint ? h('p', { class: 'hint' }, e.hint) : null,
      h('div', { class: 'acts' },
        e.state === 'down' ? act('Start', `rig up ${e.id}`) : null,
        e.state === 'quarantined' ? act('Restart', `rig restart ${e.id}`) : null,
        e.state === 'healthy' ? act('Stop', `rig stop ${e.id}`) : null,
        act('Health', `rig health ${e.id}`), act('Describe', `rig describe ${e.id}`), e.pane ? act('Open its pane', `the ${e.id} pane`) : null)),
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
  const shown = S.notes.filter((n) => (!S.nsrc || n.from === S.nsrc) &&
    (!q || [n.title, n.body, n.from].join(' ').toLowerCase().includes(q)));
  const search = h('input', { id: 'nq', type: 'search', placeholder: 'Search notifications', value: S.nq, 'aria-label': 'Search notifications',
    oninput: (e) => { S.nq = e.target.value; render(); const f = $('nq'); f.focus(); f.setSelectionRange(f.value.length, f.value.length); } });
  const src = h('select', { class: 'sel', 'aria-label': 'Only from', onchange: (e) => { S.nsrc = e.target.value; render(); } },
    h('option', { value: '' }, `every source (${S.notes.length})`),
    ...sources.map((x) => h('option', { value: x, selected: S.nsrc === x }, `${x} (${S.notes.filter((n) => n.from === x).length})`)));
  const list = h('ol', { class: 'notes' }, shown.length ? [] : h('li', { class: 'none' }, 'Nothing matches.'), shown.map((n) => h('li', { class: n.unread ? 'unread' : '', style: { '--sev': SEV[n.severity] } },
    h('span', { class: 'bullet' }),
    h('div', {},
      h('div', { class: 'nt' }, h('b', {}, n.title), h('time', { class: 'dim', title: new Date(n.at).toLocaleString() }, ago(n.at))),
      h('div', { class: 'dim' }, n.body),
      h('button', { class: 'src', title: `Only ${n.from}`, onclick: () => { S.nsrc = n.from; render(); } }, n.from)))));
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
  renderPrograms(left);
  const boardWrap = h('div', { style: { marginTop: '16px' } });
  renderBoard(boardWrap);
  left.append(boardWrap);
  grid.append(left);
  renderNotes(grid);
  view.append(grid);
}

function renderTab(view, t) {
  const own = [...S.cards.values()].filter((c) => c.from === t.id).sort((a, b) => b.order - a.order);
  view.append(h('div', { class: 'ptab' },
    h('div', { class: 'head' }, h('h1', {}, t.title), h('span', { class: 'dim' }, t.kind === 'program' ? 'program' : 'agent')),
    h('div', { class: 'reason' },
      h('div', {}, h('b', {}, `${t.title} asked for this tab at ${hhmm(t.asked)}: `), `"${t.reason}"`),
      t.released ? h('div', { class: 'frozen' }, `It no longer needs it (since ${hhmm(t.released)}). What you see is its last state. Close the tab and it moves to Reopen.`) : h('div', { class: 'dim' }, 'Close it any time; it moves to Reopen with its last state.')),
    own.length ? h('div', { class: 'items' }, own.map(cardNode)) : h('div', { class: 'empty' }, 'Nothing pushed yet.')));
}

function renderView() {
  const view = $('view');
  view.replaceChildren();
  const t = S.tabs.find((x) => x.id === S.current && x.open) || S.tabs[0];
  S.current = t.id;
  if (t.kind === 'main') renderMain(view); else renderTab(view, t);
}

function renderWire() {
  $('wire').hidden = !S.showWire;
  $('wireLog').replaceChildren(...S.wire.map((w) => h('li', {}, h('span', { class: 'faint' }, hhmm(w.at)), h('span', { class: 'k-' + w.kind }, w.verb), h('span', {}, w.text))));
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

function openQuestions() {
  const d = $('drawer');
  S.history = null;
  d.replaceChildren(
    h('header', {}, h('h3', {}, 'Open questions for you'), h('button', { class: 'iconbtn', 'aria-label': 'Close', onclick: () => { d.hidden = true; } }, svg(ICON.x))),
    h('div', { class: 'body' }, h('ol', { class: 'q' },
      h('li', {}, h('b', {}, 'Tabs and the rail: one place or two?'), 'Programs you pick live in the rail (section 11). This mockup puts tabs a program ASKS for in a strip above the page. Should picking a program in the rail open its tab too, or do the two stay separate?'),
      h('li', {}, h('b', {}, 'Is the board allowed on the dashboard?'), 'Section 11 rule 20 keeps agent chatter off the dashboard. I read that as the stream of all agent calls, and the board as cards agents write to you on purpose, so both hold. Is that right?'),
      h('li', {}, h('b', {}, 'Where does the board sit?'), 'Here it is on Main, under the programs, with notifications on the right. Is that its place, or should it be a tab of its own?'))));
  d.hidden = false;
  d.querySelector('button')?.focus();
}

let toastTimer;
function toast(text) {
  const t = $('toast');
  t.textContent = text; t.hidden = false;
  clearTimeout(toastTimer); toastTimer = setTimeout(() => { t.hidden = true; }, 2600);
}

function render() {
  renderTabs();
  renderView();
  renderWire();
  flash.clear();
}

// ── keyboard ───────────────────────────────────────────────────────────────
document.addEventListener('keydown', (e) => {
  const typing = e.target instanceof HTMLInputElement || e.target instanceof HTMLSelectElement;
  if (e.key === '/' && !typing) { e.preventDefault(); $('q')?.focus(); return; }
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

render();
// While a field has the keyboard, the page is not redrawn under it.
setInterval(() => {
  const busy = document.activeElement instanceof HTMLInputElement;
  if (S.playing && !busy) tick();
}, 2200);
window.__mock = { S, tick, render };
