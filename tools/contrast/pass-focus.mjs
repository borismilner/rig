// Focus indicators, measured from pixels.
//
// No text pass can see one. contrast-audit.js walks nodeType 3 only, and a grep
// of it for `outline` or `box-shadow` returns nothing, so a page whose every
// focus ring sits at 1.48:1 is reported clean. WCAG 1.4.11 puts a focus
// indicator at 3:1 against what it is seen against, which is a different rule
// from 1.4.3 and needs a different instrument.
//
// Method: Tab to set keyboard modality, then for each focusable element take
// one shot focused and one blurred, diff them, and read the ring out of the
// pixels that changed. Nothing is assumed about how the ring is drawn - an
// outline, a box-shadow, a border swap and a background change all show up the
// same way, which is the point.
import {ratio, hex, NON_TEXT} from './wcag.mjs';
import {diffMask, modal, bestAgainst, adjacentUnchanged} from './pixels.mjs';

const PAD = 12;          // outline-offset and small shadow spreads live out here

const ENUMERATE = `(() => {
  const SEL = 'a[href], area[href], button, input:not([type="hidden"]), select, textarea,' +
              ' summary, iframe, [tabindex]:not([tabindex="-1"]), [contenteditable="true"]';
  const label = el => {
    const cls = typeof el.className === 'string' && el.className.trim()
      ? '.' + el.className.trim().split(/\\s+/).slice(0, 2).join('.') : '';
    return el.tagName.toLowerCase() + (el.id ? '#' + el.id : '') + cls;
  };
  const out = [];
  document.querySelectorAll(SEL).forEach(el => {
    if (el.disabled || el.closest('[aria-hidden="true"]')) return;
    const cs = getComputedStyle(el);
    if (cs.display === 'none' || cs.visibility === 'hidden' || +cs.opacity === 0) return;
    const r = el.getBoundingClientRect();
    if (r.width < 1 || r.height < 1) return;
    el.setAttribute('data-rig-focus', String(out.length));
    out.push({
      i: out.length, label: label(el),
      text: (el.textContent || '').trim().slice(0, 28),
      ok: el.getAttribute('data-contrast-ok') || null,
    });
  });
  return {count: out.length, targets: out};
})()`;

const sel = i => `[data-rig-focus="${i}"]`;

const FOCUS = i => `(() => {
  const el = document.querySelector('[data-rig-focus="${i}"]');
  if (!el) return false;
  el.focus({preventScroll: false});
  return document.activeElement === el;
})()`;

const BLUR = `(() => { const a = document.activeElement; if (a && a.blur) a.blur(); return true; })()`;

// A ring pushed off the element by outline-offset lands outside the element's
// own box, so the clip has to be wider than the element. Clamped at the document
// origin because a clip with a negative x comes back shifted, not padded, and
// every pixel coordinate after that is off by the amount it was clamped.
const clip = r => {
  const x = Math.max(0, Math.floor(r.x - PAD));
  const y = Math.max(0, Math.floor(r.y - PAD));
  return {
    x, y,
    width: Math.max(1, Math.ceil(r.x + r.width + PAD) - x),
    height: Math.max(1, Math.ceil(r.y + r.height + PAD) - y),
  };
};

export async function focusPass(page, {limit = 0} = {}) {
  await page.keyboardModality();

  const {count, targets} = await page.eval(ENUMERATE);
  const list = limit ? targets.slice(0, limit) : targets;
  const findings = [], silenced = [], measured = [];

  const skipped = [];
  for (const t of list) {
    const took = await page.eval(FOCUS(t.i));
    if (!took) { findings.push({...t, kind: 'unfocusable', note: 'element refused focus'}); continue; }
    await page.paint();
    const rFocused = await page.reveal(sel(t.i));
    if (!rFocused) continue;
    // Not a contrast result either way: the pixels in this box belong to
    // something else, so reading a ring out of them would invent a number.
    if (rFocused.occluded || rFocused.offViewport || !rFocused.inViewport) {
      skipped.push({...t, onTop: rFocused.onTop,
                    at: `${Math.round(rFocused.x)},${Math.round(rFocused.y)}`,
                    kind: rFocused.occluded ? 'occluded' : 'off-viewport'});
      continue;
    }
    const box = clip(rFocused);
    const shotFocused = await page.shot(box);

    await page.eval(BLUR);
    await page.paint();
    const rBlurred = await page.reveal(sel(t.i));
    // A sticky or fixed element can move between the two shots, and then the
    // diff is of two different regions and every pixel of it is noise.
    if (Math.abs(rBlurred.x - rFocused.x) > 0.5 || Math.abs(rBlurred.y - rFocused.y) > 0.5) {
      findings.push({...t, kind: 'moved', note: 'element moved between shots; not measured'});
      continue;
    }
    const shotBlurred = await page.shot(box);

    const {mask, count: changed} = diffMask(shotBlurred, shotFocused);
    if (changed === 0) {
      const f = {...t, kind: 'no-indicator', changed: 0,
                 note: 'focusing this element changes no pixel within ' + PAD + 'px of it'};
      (t.ok ? silenced : findings).push(f);
      continue;
    }

    // What the ring is seen against: unchanged pixels next to the ones that
    // changed. Not the element's declared background - a ring drawn outside the
    // box is seen against whatever the box happens to sit on.
    const groundSel = adjacentUnchanged(shotFocused, mask, 3);
    const ground = modal(shotFocused, groundSel);
    if (!ground) { findings.push({...t, kind: 'no-ground', note: 'no unchanged pixels adjacent to the change'}); continue; }

    const best = bestAgainst(shotFocused, mask, ground.colour);
    const core = modal(shotFocused, mask);
    if (!best) { findings.push({...t, kind: 'ring-too-small', changed, note: 'no ring colour covers 4 pixels'}); continue; }

    // The state change itself, WCAG 2.4.13's other half: the same pixels before
    // and after. Reported, not gated - 1.4.11 is the rule the numbers in the
    // handoff were taken against.
    const wasCore = modal(shotBlurred, mask);
    const change = wasCore ? ratio(best.colour, wasCore.colour) : null;

    const row = {
      ...t, changed,
      ring: hex(best.colour), core: hex(core.colour), ground: hex(ground.colour),
      ratio: +best.ratio.toFixed(2),
      change: change === null ? null : +change.toFixed(2),
      need: NON_TEXT,
    };
    measured.push(row);
    if (best.ratio < NON_TEXT) (t.ok ? silenced : findings).push({...row, kind: 'ring-low'});
  }

  return {
    pass: 'focus', focusable: count, examined: list.length,
    measured, failures: findings.length, findings,
    silenced, skipped,
    lowest: measured.length ? +Math.min(...measured.map(m => m.ratio)).toFixed(2) : null,
  };
}
