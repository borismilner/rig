// Text contrast for a rendered page, run inside the page over CDP.
//
// Vendored from the author's own contrast-audit reference script, with one
// hole closed. That original skips an element only when `+cs.opacity === 0`, so
// `opacity: .5` is measured as if it were fully opaque, and it reports the
// DECLARED colour's ratio. On rig's own page that is the difference between a
// clean report and a marker painting at 2.89:1.
//
// The model here is the real one: a subtree with `opacity` is rendered, then
// multiplied, then composited over whatever its parent painted underneath. So
// the walk carries the glyph AND the background it sits on up through every
// group, dimming both at each boundary, and reports the pair that lands on
// screen. `declaredRatio` is kept beside it, because the gap between the two is
// the bug and a row where they agree is evidence this pass can be believed for
// that node.
(() => {
  const parse = c => {
    if (!c || c === 'none' || c === 'transparent') return null;
    // Chrome returns a color-mix() as "color(srgb r g b / a)" with 0-1 floats,
    // and a naive /[\d.]+/ read of that gives near-black for every one of them.
    let m = c.match(/^color\(srgb ([\d.eE+-]+) ([\d.eE+-]+) ([\d.eE+-]+)(?: \/ ([\d.eE+-]+))?\)/);
    if (m) return {r:+m[1]*255, g:+m[2]*255, b:+m[3]*255, a:m[4]===undefined?1:+m[4]};
    m = c.match(/[\d.eE+-]+/g);
    return m && m.length >= 3 ? {r:+m[0], g:+m[1], b:+m[2], a:m[3]===undefined?1:+m[3]} : null;
  };
  const CLEAR = {r:0, g:0, b:0, a:0};
  // general Porter-Duff source-over, both operands translucent
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
  // Beyond <html> is the canvas. Chrome paints it with html's or body's
  // background when either has one, and those are picked up by the walk itself.
  const CANVAS = {r:255, g:255, b:255, a:1};

  // The glyph and the ground it lands on, both carried up through every opacity
  // group. Returns painted colours, not declared ones.
  const painted = el => {
    let fg = parse(getComputedStyle(el).color) || {r:0,g:0,b:0,a:1};
    let bg = CLEAR;
    for (let n = el; n && n.nodeType === 1; n = n.parentElement) {
      const cs = getComputedStyle(n);
      const own = parse(cs.backgroundColor) || CLEAR;
      if (own.a > 0) { fg = srcOver(fg, own); bg = srcOver(bg, own); }
      const o = parseFloat(cs.opacity);
      if (!isNaN(o) && o < 1) { fg = dim(fg, o); bg = dim(bg, o); }
    }
    return {fg: srcOver(fg, CANVAS), bg: srcOver(bg, CANVAS)};
  };
  // what the old script measured: declared colour, backgrounds composited, no
  // opacity anywhere
  const declared = el => {
    let acc = CLEAR;
    for (let n = el; n && n.nodeType === 1; n = n.parentElement) {
      const c = parse(getComputedStyle(n).backgroundColor);
      if (c && c.a > 0) { acc = srcOver(acc, c); if (acc.a >= 0.999) break; }
    }
    const bg = srcOver(acc, CANVAS);
    const f = parse(getComputedStyle(el).color) || {r:0,g:0,b:0,a:1};
    return {fg: srcOver(f, bg), bg};
  };
  const groupOpacity = el => {
    let a = 1;
    for (let n = el; n && n.nodeType === 1; n = n.parentElement) {
      const o = parseFloat(getComputedStyle(n).opacity);
      if (!isNaN(o)) a *= o;
    }
    return a;
  };

  const bad = [], tiny = [], dimmed = [];
  let checked = 0, min = Infinity;
  document.querySelectorAll('body *').forEach(el => {
    // only an element's OWN text, so a wrapper is not blamed for a child
    const txt = [...el.childNodes].filter(n => n.nodeType === 3)
                  .map(n => n.textContent.trim()).join(' ').trim();
    if (!txt) return;
    const cs = getComputedStyle(el);
    if (cs.display === 'none' || cs.visibility === 'hidden') return;
    const op = groupOpacity(el);
    if (op <= 0.004) return;              // invisible; 0 was the old test
    const p = painted(el), d = declared(el);

    // Inside an SVG the computed font-size is in user units: a diagram rendered
    // at 1.8x its viewBox puts 9px text on screen at 16px. Without the screen
    // CTM the walk reports scaled-up diagrams as failing the readable floor.
    const ctm = el.ownerSVGElement && el.getScreenCTM ? el.getScreenCTM() : null;
    const zoom = ctm ? Math.hypot(ctm.a, ctm.b) : 1;
    const size = parseFloat(cs.fontSize) * zoom, bold = +cs.fontWeight >= 700;
    const need = (size >= 24 || (size >= 18.66 && bold)) ? 3 : 4.5;
    const r = ratio(p.fg, p.bg), rd = ratio(d.fg, d.bg);
    checked++; if (r < min) min = r;

    const sel = (typeof el.className === 'string' && el.className) || el.tagName;
    // bug 6: a nested em collapses a size below the readable floor
    if (size < 12) tiny.push({text: txt.slice(0,30), sel, px: +size.toFixed(2)});
    const row = {text: txt.slice(0,40), sel, fg: hex(p.fg), bg: hex(p.bg),
                 px: +size.toFixed(1), ratio: +r.toFixed(2), need,
                 declaredRatio: +rd.toFixed(2), opacity: +op.toFixed(3),
                 ok: el.getAttribute('data-contrast-ok') || null};
    if (r < need) bad.push(row);
    // rows a declared-colour audit calls clean and the screen does not
    if (rd >= need && r < need) dimmed.push(row);
  });
  bad.sort((a,b) => a.ratio - b.ratio);
  return {
    // Assert these are the page and theme you meant before believing a number.
    // A shared browser hands you another session's tab, and a theme read in the
    // same task as the flip that set it returns the PREVIOUS theme.
    page: document.title, href: location.href,
    theme: document.documentElement.getAttribute('data-theme') || 'unset',
    scheme: matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light',
    checked, failures: bad.filter(b => !b.ok).length, silenced: bad.filter(b => b.ok).length,
    lowest: min === Infinity ? null : +min.toFixed(2),
    bad: bad.filter(b => !b.ok), silencedRows: bad.filter(b => b.ok),
    hidden: dimmed, under12px: tiny.length, tiny
  };
})()
