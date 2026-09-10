// Boundaries: borders, outlines and rules, at WCAG 1.4.11's 3:1.
//
// The gap this closes, in the readable-output skill's own words: the text audit
// "walks text nodes only (nodeType === 3), so a UI-component boundary, a rule,
// a chip edge and a focus ring are all invisible to it and it will report a
// page clean while every border on it fails". The focus ring got its own pass
// because it only exists in a state. This is the other half, and it is the
// reason `--border:#556579` sat labelled "verified" while failing 3:1 on all
// four surfaces (2.89 / 2.59 / 2.40 / 2.41).
//
// A boundary has two sides and only has to be visible from one of them: a card
// edge that disappears into the page but separates cleanly from the card's own
// fill is still an edge. So the test is max(outside, inside) >= 3, which is the
// page's best case - a failure on the best case cannot be argued down.
//
// What FAILS versus what is only reported: a decorative hairline is not a
// component boundary, and a gate that fails every one of them gets switched
// off. Two classes fail. Interactive elements, whose edge is how you find the
// control. And any edge painted in --border or --border-2, because the theme
// engine binary-searches exactly those two tokens to 3.0 and a use that lands
// under it contradicts the engine's own claim. Everything else is counted and
// listed.
//
// The compositing maths is duplicated from pass-dom-text.js on purpose: this
// runs inside the page over CDP, where there is nothing to import from.
(() => {
  const parse = c => {
    if (!c || c === 'none' || c === 'transparent') return null;
    let m = c.match(/^color\(srgb ([\d.eE+-]+) ([\d.eE+-]+) ([\d.eE+-]+)(?: \/ ([\d.eE+-]+))?\)/);
    if (m) return {r:+m[1]*255, g:+m[2]*255, b:+m[3]*255, a:m[4]===undefined?1:+m[4]};
    // Hex, which the other passes never need: a resolved `color` or
    // `border-color` comes back as rgb(), but a CUSTOM PROPERTY comes back as
    // the token that was specified, and theme.js writes these as #rrggbb. The
    // numeric fallback below reads "#767676" as one number and returns null, so
    // --border was never recognised and the whole token-enforced class of
    // finding silently did not exist. selftest/boundary.html asserts the token
    // is read, which is how this was caught.
    m = c.match(/^#([0-9a-fA-F]{3,8})$/);
    if (m) {
      const h = m[1];
      const parts = h.length <= 4
        ? [...h].map(x => parseInt(x + x, 16))
        : h.match(/../g).map(x => parseInt(x, 16));
      return {r: parts[0], g: parts[1], b: parts[2], a: parts[3] === undefined ? 1 : parts[3] / 255};
    }
    m = c.match(/[\d.eE+-]+/g);
    return m && m.length >= 3 ? {r:+m[0], g:+m[1], b:+m[2], a:m[3]===undefined?1:+m[3]} : null;
  };
  const CLEAR = {r:0, g:0, b:0, a:0};
  const CANVAS = {r:255, g:255, b:255, a:1};
  const srcOver = (f, b) => {
    const a = f.a + b.a * (1 - f.a);
    if (a <= 0) return CLEAR;
    const mix = (fc, bc) => (fc * f.a + bc * b.a * (1 - f.a)) / a;
    return {r: mix(f.r, b.r), g: mix(f.g, b.g), b: mix(f.b, b.b), a};
  };
  const dim = (c, o) => ({r:c.r, g:c.g, b:c.b, a:c.a * o});
  const lum = c => {
    const v = [c.r,c.g,c.b].map(x => x/255)
      .map(x => x <= 0.03928 ? x/12.92 : Math.pow((x+0.055)/1.055, 2.4));
    return 0.2126*v[0] + 0.7152*v[1] + 0.0722*v[2];
  };
  const ratio = (a,b) => { const A=lum(a), B=lum(b); return (Math.max(A,B)+0.05)/(Math.min(A,B)+0.05); };
  const hex = c => '#' + [c.r,c.g,c.b].map(x =>
    Math.round(Math.min(255,Math.max(0,x))).toString(16).padStart(2,'0')).join('');
  const same = (a, b) => a && b && Math.abs(a.r-b.r) < 1.5 && Math.abs(a.g-b.g) < 1.5 && Math.abs(a.b-b.b) < 1.5;

  // Background stack from `start` upwards, each layer dimmed by any opacity
  // group it passes through. `stopAt` is exclusive, so the element's own
  // background can be left out when what is wanted is the colour BESIDE it.
  const stack = (start, includeSelf) => {
    let acc = CLEAR;
    let n = start;
    if (!includeSelf) {
      const o = parseFloat(getComputedStyle(n).opacity);
      n = n.parentElement;
      if (!isNaN(o) && o < 1) acc = dim(acc, o);
    }
    for (; n && n.nodeType === 1; n = n.parentElement) {
      const cs = getComputedStyle(n);
      const own = parse(cs.backgroundColor) || CLEAR;
      if (own.a > 0) acc = srcOver(acc, own);
      const o = parseFloat(cs.opacity);
      if (!isNaN(o) && o < 1) acc = dim(acc, o);
    }
    return srcOver(acc, CANVAS);
  };
  const groupOpacity = el => {
    let a = 1;
    for (let n = el; n && n.nodeType === 1; n = n.parentElement) {
      const o = parseFloat(getComputedStyle(n).opacity);
      if (!isNaN(o)) a *= o;
    }
    return a;
  };
  const hidden = el => {
    for (let n = el; n && n.nodeType === 1; n = n.parentElement) {
      const cs = getComputedStyle(n);
      if (cs.display === 'none' || cs.visibility === 'hidden' || +cs.opacity === 0) return true;
    }
    return false;
  };

  const INTERACTIVE = 'a[href], area[href], button, input, select, textarea, summary, ' +
                      '[tabindex]:not([tabindex="-1"]), [contenteditable="true"], [role="button"], ' +
                      '[role="tab"], [role="checkbox"], [role="switch"]';
  const rootStyle = getComputedStyle(document.documentElement);
  const HELD = {
    'border':   parse(rootStyle.getPropertyValue('--border').trim()),
    'border-2': parse(rootStyle.getPropertyValue('--border-2').trim()),
  };
  const heldToken = c => {
    for (const [name, v] of Object.entries(HELD)) if (same(c, v)) return '--' + name;
    return null;
  };

  const SIDES = [['Top','top'], ['Right','right'], ['Bottom','bottom'], ['Left','left']];
  const rows = [], fails = [], silenced = [];
  let checked = 0, min = Infinity;

  document.querySelectorAll('body *').forEach(el => {
    if (hidden(el) || el.closest('[aria-hidden="true"]')) return;
    const cs = getComputedStyle(el);
    const box = el.getBoundingClientRect();
    if (box.width < 1 || box.height < 1) return;
    const op = groupOpacity(el);
    if (op <= 0.004) return;

    const outside = stack(el, false);
    const inside  = stack(el, true);
    const ok = el.getAttribute('data-contrast-ok');
    // The element IS the control, not merely inside one. `closest()` here made
    // every <td> of a table sitting in a focusable scroll container an
    // enforced boundary, and 30 row separators became build failures - which
    // is how a gate gets a reputation for crying wolf and then gets a
    // --fail-under bolted onto it.
    const interactive = el.matches(INTERACTIVE);
    const label = el.tagName.toLowerCase() + (el.id ? '#' + el.id : '') +
      (typeof el.className === 'string' && el.className.trim()
        ? '.' + el.className.trim().split(/\s+/).slice(0, 2).join('.') : '');

    const edges = [];
    for (const [Side, side] of SIDES) {
      const w = parseFloat(cs['border' + Side + 'Width']);
      if (!(w > 0)) continue;
      if (cs['border' + Side + 'Style'] === 'none' || cs['border' + Side + 'Style'] === 'hidden') continue;
      const col = parse(cs['border' + Side + 'Color']);
      if (!col || col.a <= 0) continue;
      edges.push({kind: 'border-' + side, w, col});
    }
    const ow = parseFloat(cs.outlineWidth);
    if (ow > 0 && cs.outlineStyle !== 'none') {
      const col = parse(cs.outlineColor);
      // an outline is painted OUTSIDE the box, so both of its sides are the
      // colour beside the element - never the element's own fill
      if (col && col.a > 0) edges.push({kind: 'outline', w: ow, col, outer: true});
    }
    if (!edges.length) return;

    // A four-sided border in one colour is one boundary, not four findings.
    const seen = new Set();
    for (const e of edges) {
      // the element's background paints UNDER its border by default, so a
      // translucent border composites over the fill and not over the neighbour
      const behind = e.outer ? outside : inside;
      let painted = srcOver(e.col, behind);
      if (op < 1) {
        // the whole subtree is composited afterwards, so the edge and both its
        // grounds are dimmed together
        painted = srcOver(dim(e.col, op), behind);
      }
      const rOut = ratio(painted, outside), rIn = ratio(painted, e.outer ? outside : inside);
      const best = Math.max(rOut, rIn);
      const token = heldToken(e.col);
      const key = hex(painted) + '|' + (token || '') + '|' + best.toFixed(2);
      if (seen.has(key)) continue;
      seen.add(key);

      checked++;
      if (best < min) min = best;
      const row = {
        label, kind: e.kind, px: +e.w.toFixed(1),
        paint: hex(painted), outside: hex(outside), inside: hex(inside),
        outsideRatio: +rOut.toFixed(2), insideRatio: +rIn.toFixed(2),
        ratio: +best.toFixed(2), need: 3, token, interactive, opacity: +op.toFixed(3),
        // why this row is or is not allowed to fail the build
        enforced: !!(interactive || token),
        ok: ok || null,
      };
      rows.push(row);
      if (best < 3 && row.enforced) (ok ? silenced : fails).push(row);
    }
  });

  fails.sort((a, b) => a.ratio - b.ratio);
  const informational = rows.filter(r => r.ratio < 3 && !r.enforced);
  return {
    page: document.title, href: location.href,
    theme: document.documentElement.getAttribute('data-theme') || 'unset',
    checked, failures: fails.length, bad: fails, silenced: silenced.length, silencedRows: silenced,
    lowest: min === Infinity ? null : +min.toFixed(2),
    // under 3:1 but not held to it: a decorative hairline is not a component
    // boundary, and these are listed so the judgement stays visible
    informational: informational.length,
    informationalRows: informational.sort((a, b) => a.ratio - b.ratio).slice(0, 10),
    held: Object.fromEntries(Object.entries(HELD).map(([k, v]) => [k, v ? hex(v) : null])),
  };
})()
