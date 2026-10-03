// What rig hands a program that registers a GUI (plan/55, requirements 10
// to 12): one stylesheet, rig.css, and a two-way bridge. The program writes
// the content; the look is rig's, so every tab feels like one application.
//
// The GUI runs in a sandboxed frame with no access to the dashboard. It
// talks to rig only through the bridge below.
//
// The theme follows the dashboard: rig sets data-theme on the GUI's root,
// and both token sets are already in the stylesheet.

const vars = (t) => Object.entries(t).map(([k, v]) => `${k}:${v}`).join(';');

// rig.css: the tokens for both themes, then the components a GUI may use.
// A GUI carries no stylesheet of its own; these classes are the whole kit.
export function rigCss(dark, light) {
  return `:root{${vars(dark)}}
:root[data-theme="light"]{${vars(light)}}
*{box-sizing:border-box}
html,body{margin:0}
body{background:var(--bg);color:var(--fg);font:var(--fs-0)/var(--lh) var(--sans);letter-spacing:var(--tight-ui)}
button{font:inherit;color:inherit;cursor:pointer}
:focus-visible{outline:2px solid var(--h-steel);outline-offset:2px;border-radius:6px}
.rig-page{padding:18px 22px;display:grid;gap:14px;max-width:1100px}
.rig-row{display:flex;align-items:center;gap:10px;flex-wrap:wrap}
.rig-between{justify-content:space-between}
.rig-h1{margin:0;font:600 var(--fs-2)/1.15 var(--sans)}
.rig-h2{margin:0 0 10px;font:600 var(--fs-1)/1.2 var(--sans)}
.rig-dim{color:var(--fg-dim);margin:0}
.rig-mono{font-family:var(--mono)}
.rig-num{text-align:right;font-variant-numeric:tabular-nums}
.rig-card{background:var(--panel);border:1px solid var(--border);border-radius:var(--radius);padding:14px 16px}
.rig-figures{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:10px}
.rig-fig{background:var(--panel);border:1px solid var(--border);border-radius:var(--radius);padding:10px 14px}
.rig-fig b{display:block;font:600 var(--fs-2)/1.1 var(--sans);font-variant-numeric:tabular-nums}
.rig-fig span{color:var(--fg-dim);font-size:var(--fs--1)}
.rig-btn{background:var(--bg-2);border:1px solid var(--border-2);border-radius:8px;padding:5px 12px}
.rig-btn:hover{background:var(--tint)}
.rig-btn.rig-primary{background:var(--h-steel);color:var(--on-hue);border-color:var(--h-steel)}
.rig-btn:disabled{cursor:progress;color:var(--fg-dim);background:var(--bg-2);border-color:var(--border)}
.rig-table{width:100%;border-collapse:collapse}
.rig-table th{text-align:left;font-weight:600;color:var(--fg-dim);font-size:var(--fs--1);padding:6px 8px;border-bottom:1px solid var(--border)}
.rig-table td{padding:8px;border-bottom:1px solid var(--border);vertical-align:middle}
.rig-badge{display:inline-flex;align-items:center;gap:6px;font-size:var(--fs--1);color:var(--fg)}
.rig-badge::before{content:"";width:8px;height:8px;border-radius:50%;background:var(--sev,var(--fg-dim))}
.rig-badge[data-sev="info"]{--sev:var(--h-steel)}
.rig-badge[data-sev="success"]{--sev:var(--h-sage)}
.rig-badge[data-sev="warning"]{--sev:var(--h-amber)}
.rig-badge[data-sev="error"]{--sev:var(--h-rust)}
.rig-list{margin:0;padding:0;list-style:none;display:grid;gap:6px}
.rig-list li{display:flex;gap:10px;align-items:baseline}
.rig-empty{color:var(--fg-dim);padding:6px 0}
.rig-progress{height:8px;border-radius:4px;background:var(--tint);overflow:hidden;min-width:160px}
.rig-progress i{display:block;height:100%;background:var(--h-steel)}
@media (prefers-reduced-motion:reduce){*{animation:none !important;transition:none !important}}
`;
}

// The bridge, as the GUI sees it (plan/55 requirement 13): the GUI acts on
// its program only through what rig already carries. Each call is a rig
// verb, answered by rig, and rig checks it before the program sees it.
//
//   rig.invoke(command, args)  a command the program declares
//   rig.store.get(key)         the program's own collection
//   rig.events.on(topic, fn)   a bus event the program publishes
//   rig.queue.push(job, args)  long work, with rig's progress
//   rig.toast({title, body, actions})  ask the user; resolves to the choice
//
// Outside rig (opened alone) every call rejects and the GUI shows whatever
// state it was seeded with.
export const BRIDGE = `(function(){
  var seq = 0, waiting = {}, subs = {};
  var inRig = window.parent !== window;
  function call(verb, args) {
    return new Promise(function (ok, no) {
      if (!inRig) { no(new Error('not inside rig')); return; }
      var id = ++seq; waiting[id] = { ok: ok, no: no };
      window.parent.postMessage({ rig: 'call', id: id, verb: verb, args: args }, '*');
    });
  }
  window.rig = {
    invoke: function (command, args) { return call('invoke', { command: command, args: args || {} }); },
    store: { get: function (key) { return call('store.get', { key: key }); } },
    events: { on: function (topic, fn) { (subs[topic] = subs[topic] || []).push(fn); if (inRig) window.parent.postMessage({ rig: 'sub', topic: topic }, '*'); } },
    queue: { push: function (job, args) { return call('queue.push', { job: job, args: args || {} }); } },
    toast: function (t) { return call('toast', t); },
  };
  window.addEventListener('message', function (e) {
    var m = e.data;
    if (e.source !== window.parent || !m || typeof m.type !== 'string') return;
    if (m.type === 'theme') { document.documentElement.setAttribute('data-theme', m.data === 'light' ? 'light' : 'dark'); return; }
    if (m.type === 'reply' && waiting[m.id]) { var w = waiting[m.id]; delete waiting[m.id]; if (m.ok) w.ok(m.result); else w.no(new Error(m.error)); return; }
    if (m.type === 'event') (subs[m.topic] || []).forEach(function (fn) { fn(m.data); });
  });
})();`;

// One GUI document: rig's stylesheet and bridge, then the program's page.
export function guiDoc(css, body, theme, seed) {
  const safe = (s) => s.replace(/<\/(script|style)/gi, '<\\/$1');
  return `<!doctype html><html lang="en" data-theme="${theme === 'light' ? 'light' : 'dark'}"><head><meta charset="utf-8">`
    + `<style>${safe(css)}</style><script>${safe(BRIDGE)}</script>`
    + (seed ? `<script>window.RIG_SEED=${JSON.stringify(seed).replace(/</g, '\\u003c')};</script>` : '')
    + `</head><body>${body}</body></html>`;
}
