// Reading a decoded screenshot: what changed, what colour a region actually is,
// and which colour sits next to which. Everything here works on painted pixels,
// so `opacity`, `color-mix`, gradients, blend modes and images are all included
// by construction rather than modelled.
import {ratio} from './wcag.mjs';

export const at = (img, x, y) => {
  const i = (y * img.width + x) * 4;
  return {r: img.data[i], g: img.data[i + 1], b: img.data[i + 2], a: img.data[i + 3] / 255};
};

// Where two same-sized shots differ. tol is per channel: screenshots of a static
// page are deterministic, so anything above a couple of levels is a real change,
// but text antialiasing under a repaint is not worth chasing.
export function diffMask(a, b, tol = 6) {
  if (a.width !== b.width || a.height !== b.height) {
    throw new Error(`diff: ${a.width}x${a.height} against ${b.width}x${b.height}`);
  }
  const n = a.width * a.height;
  const mask = new Uint8Array(n);
  let count = 0;
  for (let i = 0; i < n; i++) {
    const p = i * 4;
    if (Math.abs(a.data[p] - b.data[p]) > tol ||
        Math.abs(a.data[p + 1] - b.data[p + 1]) > tol ||
        Math.abs(a.data[p + 2] - b.data[p + 2]) > tol) { mask[i] = 1; count++; }
  }
  return {mask, count};
}

// The most common exact colour in the selected pixels. Exact, not quantised: a
// quantised mode merges a 2px ring with the antialiased edge either side of it
// and reports a blend that is on screen nowhere.
export function modal(img, select) {
  const seen = new Map();
  const n = img.width * img.height;
  for (let i = 0; i < n; i++) {
    if (select && !select[i]) continue;
    const p = i * 4;
    const key = (img.data[p] << 16) | (img.data[p + 1] << 8) | img.data[p + 2];
    seen.set(key, (seen.get(key) || 0) + 1);
  }
  let best = null, bestN = 0;
  for (const [k, c] of seen) if (c > bestN) { bestN = c; best = k; }
  if (best === null) return null;
  return {colour: {r: (best >> 16) & 255, g: (best >> 8) & 255, b: best & 255, a: 1}, n: bestN,
          share: bestN / (seen.size ? [...seen.values()].reduce((s, x) => s + x, 0) : 1)};
}

// The selected colour that contrasts MOST with `ground`, ignoring colours that
// occur fewer than `floor` times so one stray antialiased pixel cannot speak for
// a ring. Deliberately the page's best case: a gate that fails on the strongest
// pixel of an indicator can never be argued with.
export function bestAgainst(img, select, ground, floor = 4) {
  const seen = new Map();
  const n = img.width * img.height;
  for (let i = 0; i < n; i++) {
    if (select && !select[i]) continue;
    const p = i * 4;
    const key = (img.data[p] << 16) | (img.data[p + 1] << 8) | img.data[p + 2];
    seen.set(key, (seen.get(key) || 0) + 1);
  }
  let best = null, bestR = 0;
  for (const [k, c] of seen) {
    if (c < floor) continue;
    const col = {r: (k >> 16) & 255, g: (k >> 8) & 255, b: k & 255, a: 1};
    const r = ratio(col, ground);
    if (r > bestR) { bestR = r; best = col; }
  }
  return best ? {colour: best, ratio: bestR} : null;
}

// Unchanged pixels within `radius` of a changed one: the colour a ring is
// actually seen against, which is not necessarily the element's own background.
export function adjacentUnchanged(img, mask, radius = 3) {
  const {width: w, height: h} = img;
  const sel = new Uint8Array(w * h);
  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const i = y * w + x;
      if (mask[i]) continue;
      let near = false;
      for (let dy = -radius; dy <= radius && !near; dy++) {
        const yy = y + dy; if (yy < 0 || yy >= h) continue;
        for (let dx = -radius; dx <= radius; dx++) {
          const xx = x + dx; if (xx < 0 || xx >= w) continue;
          if (mask[yy * w + xx]) { near = true; break; }
        }
      }
      if (near) sel[i] = 1;
    }
  }
  return sel;
}

// Pixels inside a rect, and the band of pixels just outside it. Used for a mark
// whose own paint has to be compared with what surrounds it.
export function boxAndBand(img, box, band = 4) {
  const inside = new Uint8Array(img.width * img.height);
  const around = new Uint8Array(img.width * img.height);
  for (let y = 0; y < img.height; y++) {
    for (let x = 0; x < img.width; x++) {
      const i = y * img.width + x;
      const inX = x >= box.x && x < box.x + box.width;
      const inY = y >= box.y && y < box.y + box.height;
      if (inX && inY) inside[i] = 1;
      else if (x >= box.x - band && x < box.x + box.width + band &&
               y >= box.y - band && y < box.y + box.height + band) around[i] = 1;
    }
  }
  return {inside, around};
}

// How many distinct colours the selected pixels hold. One means nothing is
// painted in that region: no border, no glyph, no ring, just the surface behind
// it. Distinguishing that from "focusing this changes nothing" matters, because
// one is an accessibility defect and the other is an element that is laid out
// but not drawn - and accusing the page of the first when it is the second is
// how a gate stops being believed.
export function distinct(img, select, cap = 64) {
  const seen = new Set();
  const n = img.width * img.height;
  for (let i = 0; i < n; i++) {
    if (select && !select[i]) continue;
    const p = i * 4;
    seen.add((img.data[p] << 16) | (img.data[p + 1] << 8) | img.data[p + 2]);
    if (seen.size >= cap) return cap;
  }
  return seen.size;
}
