// Vendored verbatim from
// ~/.claude/skills/readable-output/references/contrast-audit-svg.js, because CI
// has no home directory to reach into. It already composites fill-opacity and
// every ancestor opacity, which is the hole the DOM half had.
//
// Companion to pass-dom-text.js for SVG diagrams, where a label sits on a
// <rect> rather than a CSS background. Same thresholds, same return shape.
//
// Rewritten 2026-09-05. The previous version had three bugs and all three made
// it LIE, which is worse than having no script:
//   1. It hex'd the fill with m.slice(0,3), dropping alpha. A rect at
//      rgba(r,g,b,.12) was read as fully opaque, so a label on a warm tint over
//      a dark ground measured as passing when it really sits at 1.32:1. It also
//      turned fill:rgba(0,0,0,0) into #000000 and reported 1.00:1 for labels
//      that measure 4.90:1. Seven false positives and two HIDDEN real failures
//      on one page.
//   2. It took only the last overlapping rect, so a translucent rect on top of
//      another was read as if the one underneath were not there.
//   3. It compared getBBox() boxes, which are in each element's own user units,
//      so any transform on an ancestor <g> made the overlap test meaningless.
// Now: alpha is parsed and composited, every overlapping rect is composited in
// paint order over the SVG's own CSS background chain, and overlap is tested in
// screen space with getBoundingClientRect().
(() => {
  const parse = c => {
    if (!c || c === 'none' || c === 'transparent') return null;
    // Chrome returns color-mix() results as "color(srgb r g b / a)" with 0-1 floats
    let m = c.match(/^color\(srgb ([\d.]+) ([\d.]+) ([\d.]+)(?: \/ ([\d.]+))?\)/);
    if (m) return {r:+m[1]*255, g:+m[2]*255, b:+m[3]*255, a:m[4]===undefined?1:+m[4]};
    m = c.match(/[\d.]+/g);
    return m && m.length >= 3
      ? {r:+m[0], g:+m[1], b:+m[2], a:m[3]===undefined?1:+m[3]} : null;
  };
  const over = (f,b) => ({r:f.r*f.a+b.r*(1-f.a), g:f.g*f.a+b.g*(1-f.a),
                          b:f.b*f.a+b.b*(1-f.a), a:1});
  const lum = c => {
    const v = [c.r,c.g,c.b].map(x => x/255)
      .map(x => x <= 0.03928 ? x/12.92 : Math.pow((x+0.055)/1.055, 2.4));
    return 0.2126*v[0] + 0.7152*v[1] + 0.0722*v[2];
  };
  const ratio = (a,b) => { const A=lum(a), B=lum(b), hi=Math.max(A,B), lo=Math.min(A,B);
                           return (hi+0.05)/(lo+0.05); };
  const hex = c => '#' + [c.r,c.g,c.b].map(x =>
                     Math.round(Math.min(255,Math.max(0,x))).toString(16).padStart(2,'0')).join('');

  // an SVG's own background comes from CSS on the <svg> or an ancestor
  const cssBg = el => {
    let acc = {r:0,g:0,b:0,a:0}, n = el;
    while (n && n.nodeType === 1) {
      const c = parse(getComputedStyle(n).backgroundColor);
      if (c && c.a > 0) { acc = acc.a === 0 ? c : over(acc, c); if (acc.a >= 0.999) break; }
      n = n.parentElement;
    }
    const page = parse(getComputedStyle(document.body).backgroundColor) || {r:255,g:255,b:255,a:1};
    return acc.a >= 0.999 ? acc : over(acc, page);
  };
  // effective paint of a shape's fill, including fill-opacity and inherited opacity
  const shapeFill = el => {
    const cs = getComputedStyle(el);
    const f = parse(cs.fill);
    if (!f) return null;
    let a = f.a * (cs.fillOpacity === '' ? 1 : parseFloat(cs.fillOpacity));
    for (let n = el; n && n.nodeType === 1; n = n.parentElement) {
      const o = parseFloat(getComputedStyle(n).opacity);
      if (!isNaN(o)) a *= o;
    }
    return {r:f.r, g:f.g, b:f.b, a:Math.max(0, Math.min(1, a))};
  };
  const hidden = el => {
    for (let n = el; n && n.nodeType === 1; n = n.parentElement) {
      const cs = getComputedStyle(n);
      if (cs.display === 'none' || cs.visibility === 'hidden' || +cs.opacity === 0) return true;
    }
    return false;
  };

  const out = []; let checked = 0, min = Infinity;
  document.querySelectorAll('svg').forEach((svg, si) => {
    const svgBg = cssBg(svg);
    // paint order is document order; take every rect that overlaps, in that order
    const rects = [...svg.querySelectorAll('rect')].map(r => ({el:r, box:r.getBoundingClientRect()}));
    svg.querySelectorAll('text, tspan').forEach(t => {
      if (!t.textContent.trim() || hidden(t)) return;
      // a tspan carries the text; skip the parent <text> if a tspan already covers it
      if (t.tagName === 'text' && t.querySelector('tspan')) return;
      const tb = t.getBoundingClientRect();
      if (!tb.width || !tb.height) return;
      let bg = svgBg;
      for (const {el, box} of rects) {
        if (tb.left < box.right && tb.right > box.left &&
            tb.top < box.bottom && tb.bottom > box.top) {
          const f = shapeFill(el);
          if (f && f.a > 0) bg = over(f, bg);   // composite, do not replace
        }
      }
      const fg0 = shapeFill(t);
      if (!fg0) return;
      const fg = fg0.a < 1 ? over(fg0, bg) : fg0;
      const cs = getComputedStyle(t);
      // An SVG scales with its box, so the computed font-size is in user units and
      // is NOT what lands on screen. Multiply by the element's screen CTM or a
      // diagram rendered at 1.8x reads as failing the 12px floor when it is fine.
      const ctm = t.getScreenCTM();
      const zoom = ctm ? Math.hypot(ctm.a, ctm.b) : 1;
      const size = parseFloat(cs.fontSize) * zoom, bold = +cs.fontWeight >= 700;
      const need = (size >= 24 || (size >= 18.66 && bold)) ? 3 : 4.5;
      const r = ratio(fg, bg);
      checked++; if (r < min) min = r;
      if (r < need) out.push({svg: si, text: t.textContent.trim().slice(0,40),
                              cls: t.getAttribute('class'), fg: hex(fg), bg: hex(bg),
                              px: +size.toFixed(1), ratio: +r.toFixed(2), need});
    });
  });
  return {
    // assert these are the page you meant - a shared browser will hand you another
    page: document.title, href: location.href,
    theme: document.documentElement.getAttribute('data-theme') || 'unset',
    checked, failures: out.length,
    lowest: min === Infinity ? null : +min.toFixed(2),
    bad: out.sort((a,b) => a.ratio - b.ratio)
  };
})()
