// WCAG 2.x relative luminance and contrast ratio, plus the colour parsing every
// pass needs. One definition, because two copies of this drifted once already:
// tools/contrast.py carried its own and reported a different number for the
// same pair.
//
// The parser exists because Chrome does not hand back what was written. A
// color-mix() resolves to "color(srgb r g b / a)" with 0-1 floats, and a naive
// /[\d.]+/ read of that gives r=0.18 instead of r=46 - near-black, so the
// ratio comes out plausible and wrong.

export function parseColour(c) {
  if (!c || c === 'none' || c === 'transparent') return null;
  let m = c.match(/^color\(srgb ([\d.eE+-]+) ([\d.eE+-]+) ([\d.eE+-]+)(?: \/ ([\d.eE+-]+))?\)/);
  if (m) return {r: +m[1] * 255, g: +m[2] * 255, b: +m[3] * 255, a: m[4] === undefined ? 1 : +m[4]};
  m = c.match(/^#([0-9a-fA-F]{3,8})$/);
  if (m) {
    const h = m[1];
    const p = h.length <= 4
      ? [...h].map(x => parseInt(x + x, 16))
      : h.match(/../g).map(x => parseInt(x, 16));
    return {r: p[0], g: p[1], b: p[2], a: p[3] === undefined ? 1 : p[3] / 255};
  }
  m = c.match(/[\d.eE+-]+/g);
  return m && m.length >= 3
    ? {r: +m[0], g: +m[1], b: +m[2], a: m[3] === undefined ? 1 : +m[3]}
    : null;
}

// f over b, both premultiplied out. b must be opaque.
export const over = (f, b) => ({
  r: f.r * f.a + b.r * (1 - f.a),
  g: f.g * f.a + b.g * (1 - f.a),
  b: f.b * f.a + b.b * (1 - f.a),
  a: 1,
});

export function luminance(c) {
  const v = [c.r, c.g, c.b].map(x => x / 255)
    .map(x => (x <= 0.03928 ? x / 12.92 : Math.pow((x + 0.055) / 1.055, 2.4)));
  return 0.2126 * v[0] + 0.7152 * v[1] + 0.0722 * v[2];
}

export function ratio(a, b) {
  const A = luminance(a), B = luminance(b);
  return (Math.max(A, B) + 0.05) / (Math.min(A, B) + 0.05);
}

export const hex = c => '#' + [c.r, c.g, c.b]
  .map(x => Math.round(Math.min(255, Math.max(0, x))).toString(16).padStart(2, '0')).join('');

// WCAG 1.4.3 for text: 3:1 once it is large, 4.5:1 below that.
export const needForText = (px, bold) => (px >= 24 || (px >= 18.66 && bold)) ? 3 : 4.5;

// WCAG 1.4.11. A boundary that has to be perceived owes this, and no text pass
// can see one: contrast-audit.js walks nodeType 3 only, so a rule, a chip edge
// and a focus ring are all invisible to it.
export const NON_TEXT = 3;
