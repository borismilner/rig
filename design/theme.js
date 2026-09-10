/* ═══════════════════════════════════════════════════════════════════════════
   rig - the theme engine.

   Every visual parameter is data, not CSS. A theme is a small object; this
   turns it into the token set the whole product is painted with. That is the
   point: the palette, the faces, the type scale, the density and the corner
   radius are all tweakable AFTER the thing is built, and in rig they arrive
   the same way any other setting does - declared as JSON Schema, layered,
   live-pushed (PLAN.md §6).

   Two rules the engine enforces so a tweak cannot quietly break legibility:

   1. THE HUES ARE GENERATED IN OKLCH, so "six evenly spaced at one lightness
      and one chroma" is arithmetic rather than a promise. Move the lightness
      and all six move together and stay a family.
   2. THE NEUTRALS ARE SOLVED, NOT CHOSEN. --border, --fg-dim and --fg-faint
      are binary-searched to the lightest (dark) or darkest (light) value that
      still clears its WCAG target on EVERY surface it can land on. That is
      how the inherited #556579 border - 2.40:1 on --panel - stops being
      possible to type in.
   ═══════════════════════════════════════════════════════════════════════ */

/* ── colour maths: oklch <-> sRGB, and WCAG 2.x contrast ────────────────── */
const clamp01 = x => Math.min(1, Math.max(0, x));
const f2s = c => c <= 0.0031308 ? 12.92*c : 1.055*Math.pow(c, 1/2.4) - 0.055;
const s2f = c => c <= 0.04045 ? c/12.92 : Math.pow((c+0.055)/1.055, 2.4);

export function oklchToRgb(L, C, hDeg){
  const h = hDeg*Math.PI/180;
  const a = C*Math.cos(h), b = C*Math.sin(h);
  const l_ = L + 0.3963377774*a + 0.2158037573*b;
  const m_ = L - 0.1055613458*a - 0.0638541728*b;
  const s_ = L - 0.0894841775*a - 1.2914855480*b;
  const l = l_**3, m = m_**3, s = s_**3;
  return [
    +4.0767416621*l - 3.3077115913*m + 0.2309699292*s,
    -1.2684380046*l + 2.6097574011*m - 0.3413193965*s,
    -0.0041960863*l - 0.7034186147*m + 1.7076147010*s,
  ];
}
export function rgbToOklch(r, g, b){            // r,g,b linear 0-1
  const l = Math.cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b);
  const m = Math.cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b);
  const s = Math.cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b);
  const L = 0.2104542553*l + 0.7936177850*m - 0.0040720468*s;
  const A = 1.9779984951*l - 2.4285922050*m + 0.4505937099*s;
  const B = 0.0259040371*l + 0.7827717662*m - 0.8086757660*s;
  let h = Math.atan2(B, A)*180/Math.PI; if (h < 0) h += 360;
  return {L, C: Math.hypot(A, B), h};
}
const inGamut = ([r,g,b]) => [r,g,b].every(v => v >= -1e-4 && v <= 1+1e-4);

/** oklch to a hex string, reducing chroma until the colour is representable. */
export function okhex(L, C, h){
  let c = C;
  for (let i = 0; i < 48 && !inGamut(oklchToRgb(L, c, h)); i++) c *= 0.94;
  const [r,g,b] = oklchToRgb(L, c, h).map(v => Math.round(clamp01(f2s(v))*255));
  return '#' + [r,g,b].map(v => v.toString(16).padStart(2,'0')).join('');
}
export function hexToOklch(hex){
  const n = hex.replace('#','');
  const [r,g,b] = [0,2,4].map(i => s2f(parseInt(n.substr(i,2),16)/255));
  return rgbToOklch(r,g,b);
}
export const lumOf = hex => {
  const n = hex.replace('#','');
  const [r,g,b] = [0,2,4].map(i => s2f(parseInt(n.substr(i,2),16)/255));
  return 0.2126*r + 0.7152*g + 0.0722*b;
};
export const contrast = (a, b) => {
  const A = lumOf(a), B = lumOf(b);
  return (Math.max(A,B)+0.05)/(Math.min(A,B)+0.05);
};

/* ── the solver: the lightest/darkest neutral that clears a target ──────── */
function solveNeutral(target, grounds, {hue, chroma, dark}){
  // binary search on oklch L. Dark themes want the DIMMEST passing value, so
  // nothing is brighter than it has to be (halation is a real cost, see
  // readable-output); light themes want the LIGHTEST passing value.
  let lo = 0, hi = 1;
  const passes = L => {
    const hex = okhex(L, chroma, hue);
    return grounds.every(g => contrast(hex, g) >= target);
  };
  for (let i = 0; i < 30; i++){
    const mid = (lo+hi)/2;
    if (dark ? passes(mid) : passes(mid)) { dark ? hi = mid : lo = mid; }
    else { dark ? lo = mid : hi = mid; }
  }
  return okhex(dark ? hi : lo, chroma, hue);
}

/* ── the default theme. Every number here is a knob. ────────────────────── */
export const DEFAULTS = {
  faces: {
    display: 'Fraunces',
    ui:      'Inter Tight',
    mono:    'JetBrains Mono',
  },
  type: { base: 16, scale: 1.26, uiTight: -0.011, dispTight: -0.024, lineHeight: 1.55 },
  shape:{ radius: 12, density: 1, gut: 1.6 },
  hues: {                       // the family, generated not typed
    count: 6,
    names: ['steel','sage','teal','lilac','clay','sand'],
    angles:[250, 145, 185, 310, 50, 95],
    rotate: 0,
    dark:  { L: 0.800, C: 0.070 },
    light: { L: 0.470, C: 0.085 },
  },
  surfaces: {
    hue: 252, chroma: 0.022,
    // every surface is its own knob, because "one step" cannot express a
    // light theme, where the panel goes UP toward white and the recessed
    // grounds go DOWN away from it
    dark:  { bg:0.215, bg2:0.248, panel:0.280, glow:0.292, tint:0.330, fg:0.918 },
    light: { bg:0.968, bg2:0.995, panel:1.000, glow:0.936, tint:0.922, fg:0.200 },
  },
  motion: true,
};

/* ── build a full token set from a theme object ─────────────────────────── */
export function tokens(theme, mode){
  const dark = mode === 'dark';
  const s  = theme.surfaces, sm = dark ? s.dark : s.light;
  const H  = dark ? theme.hues.dark : theme.hues.light;
  const N  = (L) => okhex(Math.min(1, Math.max(0, L)), s.chroma, s.hue);

  const bg    = N(sm.bg);
  const bg2   = N(sm.bg2);
  const panel = N(sm.panel);
  const glow  = N(sm.glow);
  const tint  = N(sm.tint);
  const grounds = [bg, bg2, panel, glow];

  // solved, not chosen. fg-dim is held one step stronger than fg-faint so the
  // hierarchy survives a tweak instead of collapsing into one colour.
  const fg      = N(sm.fg);
  const fgDim   = solveNeutral(5.6, grounds, {hue:s.hue, chroma:s.chroma*1.5, dark});
  const fgFaint = solveNeutral(4.5, grounds, {hue:s.hue, chroma:s.chroma*1.5, dark});
  const border  = solveNeutral(3.0, grounds, {hue:s.hue, chroma:s.chroma*1.8, dark});
  const border2 = solveNeutral(4.5, grounds, {hue:s.hue, chroma:s.chroma*1.8, dark});

  const hue = {};
  theme.hues.names.forEach((n,i) => {
    const a = (theme.hues.angles[i] + theme.hues.rotate + 360) % 360;
    hue[n] = okhex(H.L, H.C, a);
    hue['o-'+n] = `oklch(${(H.L*100).toFixed(1)}% ${H.C.toFixed(3)} ${a.toFixed(0)})`;
  });

  const t = theme.type, sc = t.scale;
  return {
    '--bg':bg, '--bg-2':bg2, '--panel':panel, '--glow':glow, '--tint':tint,
    '--fg':fg, '--fg-dim':fgDim, '--fg-faint':fgFaint,
    '--border':border, '--border-2':border2,
    '--on-hue': dark ? '#0d1117' : '#ffffff',
    ...Object.fromEntries(theme.hues.names.flatMap(n => [
      [`--h-${n}`, hue[n]], [`--o-${n}`, hue['o-'+n]],
    ])),
    '--disp': `"${theme.faces.display}", ui-serif, Georgia, serif`,
    '--sans': `"${theme.faces.ui}", Cantarell, ui-sans-serif, system-ui, sans-serif`,
    '--mono': `"${theme.faces.mono}", ui-monospace, monospace`,
    '--fs-0': `max(12px, ${(t.base/16).toFixed(3)}rem)`,
    '--fs--1':`max(12px, ${(t.base/16/sc).toFixed(3)}rem)`,
    '--fs--2':`max(12px, ${(t.base/16/sc/sc).toFixed(3)}rem)`,
    '--fs-1': `${(t.base/16*sc).toFixed(3)}rem`,
    '--fs-2': `${(t.base/16*sc*sc).toFixed(3)}rem`,
    '--fs-3': `${(t.base/16*sc*sc*sc).toFixed(3)}rem`,
    '--fs-hero': `clamp(2.5rem, 6.2vw, ${(t.base/16*Math.pow(sc,5)).toFixed(2)}rem)`,
    '--lh': String(t.lineHeight),
    '--tight-ui': `${t.uiTight}em`,
    '--tight-disp': `${t.dispTight}em`,
    '--radius': `${theme.shape.radius}px`,
    '--den': String(theme.shape.density),
    '--gut': `${theme.shape.gut}rem`,
    '--motion': theme.motion ? '1' : '0',
  };
}

export function apply(theme, mode, root = document.documentElement){
  const t = tokens(theme, mode);
  for (const [k,v] of Object.entries(t)) root.style.setProperty(k, v);
  root.dataset.theme = mode;
  root.style.setProperty('color-scheme', mode);
  return t;
}

/** The same token set as a rig config fragment, so a tweak here is a paste. */
export function toToml(theme, mode){
  const t = tokens(theme, mode);
  const lines = [`# rig.toml - [ui.theme.${mode}]`, `[ui.theme]`,
    `display_face = "${theme.faces.display}"`,
    `ui_face      = "${theme.faces.ui}"`,
    `mono_face    = "${theme.faces.mono}"`,
    `base_size    = ${theme.type.base}`,
    `type_scale   = ${theme.type.scale}`,
    `radius       = ${theme.shape.radius}`,
    `density      = ${theme.shape.density}`,
    `hue_rotate   = ${theme.hues.rotate}`,
    `motion       = ${theme.motion}`,
    ``, `[ui.theme.${mode}.tokens]   # generated, and every neutral is solved`];
  for (const [k,v] of Object.entries(t))
    if (k.startsWith('--') && v.startsWith('#')) lines.push(`${k.slice(2).replace(/-/g,'_')} = "${v}"`);
  return lines.join('\n');
}
