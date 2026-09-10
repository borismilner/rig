// Painted colour, for everything the DOM cannot be asked about.
//
// contrast-audit.js reads `getComputedStyle(el).color`. That is the DECLARED
// colour, and it is not what lands on screen: an ancestor `opacity` composites
// the whole subtree afterwards, so a marker declared at #dae1e8 inside a
// wrapper at opacity .5 is measured as if it were fully opaque. Measured on
// rig's own page: a stopped marker paints 2.89:1 dark and 2.24:1 light while
// the audit reports zero failures. Line 45 of that script skips an element only
// when `+cs.opacity === 0`; every fraction in between is read as 1.
//
// So this pass takes the pixels. It reports `declared` beside `paint` on every
// row, because the gap between the two is the bug, and a row where they agree
// is evidence the cheaper pass can be trusted for that node.
import {ratio, parseColour, hex, needForText, NON_TEXT} from './wcag.mjs';
import {modal, bestAgainst, boxAndBand} from './pixels.mjs';

const BAND = 5;                 // how far outside a shape to look for its ground
const MAX_SHAPE_AREA = 40000;   // 200x200: bigger than this is a surface, not a mark

// Candidates: anything the declared-colour passes get wrong by construction.
// Not "every element", because a gate that reports every tinted panel drowns
// the three defects it exists to find.
const INVENTORY = `(() => {
  const label = el => {
    const cls = typeof el.className === 'string' && el.className.trim()
      ? '.' + el.className.trim().split(/\\s+/).slice(0, 2).join('.') : '';
    return el.tagName.toLowerCase() + (el.id ? '#' + el.id : '') + cls;
  };
  const groupOpacity = el => {
    let a = 1;
    for (let n = el; n && n.nodeType === 1; n = n.parentElement) {
      const o = parseFloat(getComputedStyle(n).opacity);
      if (!isNaN(o)) a *= o;
    }
    return a;
  };
  const mixed = v => typeof v === 'string' && v.startsWith('color(');
  const hidden = el => {
    for (let n = el; n && n.nodeType === 1; n = n.parentElement) {
      const cs = getComputedStyle(n);
      if (cs.display === 'none' || cs.visibility === 'hidden' || +cs.opacity === 0) return true;
    }
    return false;
  };

  const out = [];
  document.querySelectorAll('body *').forEach(el => {
    if (el.closest('[aria-hidden="true"]')) return;
    if (hidden(el)) return;
    const cs = getComputedStyle(el);
    const op = groupOpacity(el);
    const usesMix = mixed(cs.color) || mixed(cs.backgroundColor) ||
                    mixed(cs.borderTopColor) || mixed(cs.outlineColor) || mixed(cs.fill);
    const asked = el.hasAttribute('data-contrast-mark');
    if (!asked && op >= 0.999 && !usesMix) return;

    const own = [...el.childNodes].filter(n => n.nodeType === 3)
                  .map(n => n.textContent.trim()).join(' ').trim();
    const r = el.getBoundingClientRect();
    if (r.width < 2 || r.height < 2) return;

    const kind = own ? 'text' : 'shape';
    if (kind === 'shape' && !asked && r.width * r.height > ${MAX_SHAPE_AREA}) return;
    if (kind === 'shape') {
      // a shape with nothing of its own to paint is not a mark
      const paints = cs.backgroundColor !== 'rgba(0, 0, 0, 0)' ||
                     parseFloat(cs.borderTopWidth) > 0 || parseFloat(cs.outlineWidth) > 0 ||
                     (cs.fill && cs.fill !== 'none');
      if (!paints) return;
    }

    el.setAttribute('data-rig-paint', String(out.length));
    out.push({
      i: out.length, label: label(el), kind,
      text: own.slice(0, 28),
      opacity: +op.toFixed(3),
      usesMix,
      declared: kind === 'text' ? cs.color : (cs.backgroundColor !== 'rgba(0, 0, 0, 0)' ? cs.backgroundColor : cs.borderTopColor),
      px: parseFloat(cs.fontSize),
      bold: +cs.fontWeight >= 700,
      ok: el.getAttribute('data-contrast-ok') || null,
      rect: {x: r.left + scrollX, y: r.top + scrollY, width: r.width, height: r.height},
    });
  });
  return {count: out.length, marks: out};
})()`;

const sel = i => `[data-rig-paint="${i}"]`;

export async function paintPass(page, {limit = 0} = {}) {
  const {count, marks} = await page.eval(INVENTORY);
  const list = limit ? marks.slice(0, limit) : marks;
  const measured = [], findings = [], silenced = [], skipped = [];

  for (const m of list) {
    const r = await page.reveal(sel(m.i));
    if (!r) continue;
    if (r.occluded || r.offViewport || !r.inViewport) {
      skipped.push({...m, rect: undefined, onTop: r.onTop,
                    at: `${Math.round(r.x)},${Math.round(r.y)}`,
                    kind: r.occluded ? 'occluded' : 'off-viewport'});
      continue;
    }
    const x = Math.max(0, Math.floor(r.x - BAND));
    const y = Math.max(0, Math.floor(r.y - BAND));
    const clip = {
      x, y,
      width: Math.max(1, Math.ceil(r.x + r.width + BAND) - x),
      height: Math.max(1, Math.ceil(r.y + r.height + BAND) - y),
    };
    const img = await page.shot(clip);
    // the element's box inside the shot, which is offset by however much the
    // clamp at the document origin moved the clip
    const box = {
      x: Math.round(r.x - x), y: Math.round(r.y - y),
      width: Math.max(1, Math.round(r.width)), height: Math.max(1, Math.round(r.height)),
    };
    const {inside, around} = boxAndBand(img, box, BAND);

    // Text is read against the background INSIDE its own box; a shape is read
    // against the band just outside it. Both then take the most contrasting
    // colour actually present, which is the page's best case - a failure on the
    // best case cannot be argued down.
    const groundSel = m.kind === 'text' ? inside : around;
    const g = modal(img, groundSel);
    if (!g) continue;
    const best = bestAgainst(img, inside, g.colour);
    if (!best) continue;
    // Every pixel in the box is the ground colour, so this element paints
    // nothing here. That is an instrument result, not a 1.00:1 contrast
    // failure, and reporting it as one is how a gate loses its credibility.
    if (best.ratio <= 1.005) {
      skipped.push({...m, rect: undefined, kind: 'not-painted',
                    note: `every pixel in the box is ${hex(g.colour)}`});
      continue;
    }

    const declared = parseColour(m.declared);
    const need = m.kind === 'text' ? needForText(m.px, m.bold) : NON_TEXT;
    const row = {
      ...m, rect: undefined,
      paint: hex(best.colour), ground: hex(g.colour),
      declared: declared ? hex(declared) : m.declared,
      ratio: +best.ratio.toFixed(2), need,
      declaredRatio: declared ? +ratio(declared, g.colour).toFixed(2) : null,
    };
    row.hiddenByDeclared = row.declaredRatio !== null && row.declaredRatio >= need && row.ratio < need;
    measured.push(row);
    if (best.ratio < need) (m.ok ? silenced : findings).push({...row, kind: row.kind + '-low'});
  }

  return {
    pass: 'paint', candidates: count, examined: list.length,
    measured, failures: findings.length, findings, silenced, skipped,
    // the rows where the declared colour passes and the painted colour does not:
    // exactly what a DOM-only gate certifies as clean
    hidden: measured.filter(m => m.hiddenByDeclared),
    lowest: measured.length ? +Math.min(...measured.map(m => m.ratio)).toFixed(2) : null,
  };
}
