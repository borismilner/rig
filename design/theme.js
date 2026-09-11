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

/* ── perceptual separation: CIEDE2000 ───────────────────────────────────
   Contrast answers "can this be read". It says nothing about whether two
   colours shown side by side can be TOLD APART, which is a different
   question with its own measurement. CIEDE2000 is that measurement: the
   CIE's perceptual colour difference, where ~1 is the just-noticeable
   difference, ~5 is "clearly different at a glance" and ~10+ is
   unmistakable across a room. A categorical palette is judged by its
   WORST pair, never its average.                                        */
function hexToLab(hex){
  const n = hex.replace('#','');
  let [r,g,b] = [0,2,4].map(i => s2f(parseInt(n.substr(i,2),16)/255));
  // sRGB D65 -> XYZ, then XYZ -> CIELAB against the D65 white point
  const X = (0.4124564*r + 0.3575761*g + 0.1804375*b) / 0.95047;
  const Y = (0.2126729*r + 0.7151522*g + 0.0721750*b) / 1.00000;
  const Z = (0.0193339*r + 0.1191920*g + 0.9503041*b) / 1.08883;
  const f = t => t > 216/24389 ? Math.cbrt(t) : (841/108)*t + 4/29;
  const [fx,fy,fz] = [f(X),f(Y),f(Z)];
  return [116*fy - 16, 500*(fx-fy), 200*(fy-fz)];
}
export function deltaE2000(hexA, hexB){
  const [L1,a1,b1] = hexToLab(hexA), [L2,a2,b2] = hexToLab(hexB);
  const rad = Math.PI/180, deg = 180/Math.PI;
  const C1 = Math.hypot(a1,b1), C2 = Math.hypot(a2,b2), Cb = (C1+C2)/2;
  const G  = 0.5*(1 - Math.sqrt(Math.pow(Cb,7)/(Math.pow(Cb,7)+Math.pow(25,7))));
  const A1 = (1+G)*a1, A2 = (1+G)*a2;
  const Cp1 = Math.hypot(A1,b1), Cp2 = Math.hypot(A2,b2);
  const hp = (b,a) => { if (b===0 && a===0) return 0;
    const h = Math.atan2(b,a)*deg; return h < 0 ? h+360 : h; };
  const hp1 = hp(b1,A1), hp2 = hp(b2,A2);
  const dLp = L2-L1, dCp = Cp2-Cp1;
  let dhp = 0;
  if (Cp1*Cp2 !== 0){
    dhp = hp2-hp1;
    if (dhp >  180) dhp -= 360;
    if (dhp < -180) dhp += 360;
  }
  const dHp = 2*Math.sqrt(Cp1*Cp2)*Math.sin(dhp/2*rad);
  const Lbp = (L1+L2)/2, Cbp = (Cp1+Cp2)/2;
  let hbp;
  if (Cp1*Cp2 === 0) hbp = hp1+hp2;
  else if (Math.abs(hp1-hp2) <= 180) hbp = (hp1+hp2)/2;
  else hbp = (hp1+hp2 < 360) ? (hp1+hp2+360)/2 : (hp1+hp2-360)/2;
  const T = 1 - 0.17*Math.cos((hbp-30)*rad) + 0.24*Math.cos(2*hbp*rad)
              + 0.32*Math.cos((3*hbp+6)*rad) - 0.20*Math.cos((4*hbp-63)*rad);
  const dTh = 30*Math.exp(-Math.pow((hbp-275)/25, 2));
  const Rc  = 2*Math.sqrt(Math.pow(Cbp,7)/(Math.pow(Cbp,7)+Math.pow(25,7)));
  const Sl  = 1 + (0.015*Math.pow(Lbp-50,2))/Math.sqrt(20+Math.pow(Lbp-50,2));
  const Sc  = 1 + 0.045*Cbp, Sh = 1 + 0.015*Cbp*T;
  const Rt  = -Math.sin(2*dTh*rad)*Rc;
  return Math.sqrt(Math.pow(dLp/Sl,2) + Math.pow(dCp/Sc,2) + Math.pow(dHp/Sh,2)
                 + Rt*(dCp/Sc)*(dHp/Sh));
}
/** The worst pair in a set. A palette is only as separable as this number. */
export function separation(hexes){
  let worst = Infinity, pair = null;
  for (let i=0;i<hexes.length;i++) for (let j=i+1;j<hexes.length;j++){
    const d = deltaE2000(hexes[i], hexes[j]);
    if (d < worst){ worst = d; pair = [i,j]; }
  }
  return {worst, pair};
}

/* ── the solver: the lightest/darkest neutral that clears a target ──────── */

/* solveNeutralChecked -> {hex, ok}
 *
 * `ok` exists because the search ALWAYS returns a colour. Thirty iterations
 * narrow the bracket whether or not any L clears the target, so on a ladder
 * where nothing can pass, the old signature handed back a failing neutral and
 * said nothing - and section 6's promise is that "rig refuses to apply a token
 * set that fails, naming the token and the ground", which needs a signal to
 * refuse on. checkTheme re-measures every token anyway, so this is belt and
 * braces; what it adds is the DISTINCTION between "this value fails" and "no
 * value exists", which are different messages to put in front of a person.
 */
function solveNeutralChecked(target, grounds, {hue, chroma, dark}){
  // binary search on oklch L. Dark themes want the DIMMEST passing value, so
  // nothing is brighter than it has to be (halation is a real cost, see the
  // readable-output notes); light themes want the LIGHTEST passing value.
  let lo = 0, hi = 1;
  const passes = L => {
    const hex = okhex(L, chroma, hue);
    return grounds.every(g => contrast(hex, g) >= target);
  };
  // Probe both ends first: if neither extreme passes, no interior value will
  // either, and the bracket the loop below reports would be meaningless.
  const anyEnd = passes(0) || passes(1);
  for (let i = 0; i < 30; i++){
    const mid = (lo+hi)/2;
    if (passes(mid)) { if (dark) hi = mid; else lo = mid; }
    else             { if (dark) lo = mid; else hi = mid; }
  }
  const hex = okhex(dark ? hi : lo, chroma, hue);
  return {hex, ok: anyEnd && grounds.every(g => contrast(hex, g) >= target)};
}

function solveNeutral(target, grounds, opts){
  return solveNeutralChecked(target, grounds, opts).hex;
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
  /* THE FAMILY, and it carries meaning as well as identity.
     Red is bad and green is good everywhere a person has ever used a
     computer, so those two angles are ANCHORED and the optimiser is not
     allowed to move them. `role` says what a member means; `identity: false`
     means no program may ever own it, because a shell that turns red when
     you open a program is telling you something untrue.

     The free angles are placed to maximise the WORST pairwise CIEDE2000 in
     the set, which is the measurement for "can these be told apart", and is
     a different question from contrast. */
  hues: {
    members: [
      {name:'rust',  angle: 27, role:'bad',      identity:false, anchored:true },
      {name:'amber', angle: 78, role:'warn',     identity:false, anchored:true },
      {name:'sage',  angle:148, role:'good',     identity:true,  anchored:true },
      {name:'teal',  angle:196, role:'progress', identity:true,  anchored:false},
      {name:'steel', angle:252, role:'info',     identity:true,  anchored:true },
      {name:'indigo',angle:294, role:'-',        identity:true,  anchored:false},
      {name:'lilac', angle:345, role:'-',        identity:true,  anchored:false},
    ],
    rotate: 0,
    dark:  { L: 0.800, C: 0.098 },
    light: { L: 0.470, C: 0.110 },
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
  // Two ground sets, because --tint is a surface text lands on and a boundary
  // never does. It is the extreme of the ladder in both themes, so a token
  // solved against the other four and then used on it is a token solved
  // against the wrong question - which is exactly how a 3.95:1 shipped.
  const grounds     = [bg, bg2, panel, glow];
  const textGrounds = [bg, bg2, panel, glow, tint];

  // solved, not chosen. fg-dim is held one step stronger than fg-faint so the
  // hierarchy survives a tweak instead of collapsing into one colour.
  const fg      = N(sm.fg);
  const fgDim   = solveNeutral(5.6, textGrounds, {hue:s.hue, chroma:s.chroma*1.5, dark});
  const fgFaint = solveNeutral(4.5, textGrounds, {hue:s.hue, chroma:s.chroma*1.5, dark});
  const border  = solveNeutral(3.0, grounds,     {hue:s.hue, chroma:s.chroma*1.8, dark});
  const border2 = solveNeutral(4.5, grounds,     {hue:s.hue, chroma:s.chroma*1.8, dark});

  const hue = {};
  theme.hues.members.forEach(m => {
    const a = (m.angle + theme.hues.rotate + 360) % 360;
    hue[m.name] = okhex(H.L, H.C, a);
    hue['o-'+m.name] = `oklch(${(H.L*100).toFixed(1)}% ${H.C.toFixed(3)} ${a.toFixed(0)})`;
  });

  const t = theme.type, sc = t.scale;
  return {
    '--bg':bg, '--bg-2':bg2, '--panel':panel, '--glow':glow, '--tint':tint,
    '--fg':fg, '--fg-dim':fgDim, '--fg-faint':fgFaint,
    '--border':border, '--border-2':border2,
    '--on-hue': dark ? '#0d1117' : '#ffffff',
    ...Object.fromEntries(theme.hues.members.flatMap(m => [
      [`--h-${m.name}`, hue[m.name]], [`--o-${m.name}`, hue['o-'+m.name]],
    ])),
    // semantic aliases, so a surface asks for meaning and never for a colour
    ...Object.fromEntries(theme.hues.members.filter(m => m.role !== '-').map(m =>
      [`--sem-${m.role}`, hue[m.name]])),
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

/* ── the gate: refuse a token set a person could not read ────────────────
 *
 * Section 6: "rig refuses to apply a token set that fails, naming the token
 * and the ground, so a theme cannot silently produce an unreadable product."
 * This is that refusal, and it lives in the engine rather than in the window
 * so that the window, the design page and CI all judge a theme the same way.
 *
 * TWO GROUND SETS, because they answer different questions. Text lands on all
 * five surfaces including `tint`; a boundary never lands on `tint`. Solving or
 * checking a text token against the boundary four is how a 3.95:1 reaches a
 * page that audits clean - section 6 rule 2, and the reason the split is
 * duplicated here rather than shared with tokens(): each is meant to be
 * readable on its own.
 *
 * WHAT IS CHECKED, and why each one can actually fail:
 *
 *   --fg           4.5  on the 5 text grounds. CHOSEN from the ladder, not
 *                       solved, so it is the one a settings box can break
 *                       directly. Measured: dragging dark fg L from 0.918 to
 *                       0.40 takes it to 1.52-1.91:1 on every ground.
 *   --fg-dim       5.6  solved, but the solver returns a colour whether or not
 *   --fg-faint     4.5  one passes, so the result is re-measured here.
 *   --border       3.0  on the 4 boundary grounds (WCAG 1.4.11).
 *   --border-2     4.5
 *   --h-*          4.5  on the 5 text grounds. A hue is TEXT wherever
 *                       --sem-<role> or --hue is used as a colour, and the
 *                       hues come from an L/C pair a person can set: dropping
 *                       dark L from 0.800 to 0.35 puts all seven at 1.03-1.13
 *                       on --tint.
 *   --on-hue       4.5  against each hue, since that is text ON a hue ground.
 *
 * WHAT IS DELIBERATELY NOT CHECKED. Composited surfaces - a
 * `color-mix(in srgb, var(--hue) 13%, transparent)` callout, for instance -
 * are not tokens and cannot be resolved without a layout engine. The browser
 * contrast gate measures those on the real page, which is the right division:
 * this gate judges the token set, that one judges what gets painted. Neither
 * subsumes the other and a theme needs both.
 *
 * Rejection names the token AND the ground (section 6 rule 3): "contrast too
 * low" without the ground is not actionable, because the fix differs depending
 * on which surface it failed against.
 */
export function checkTheme(theme, mode){
  const k = tokens(theme, mode);
  const g = n => [n, k['--' + n]];
  const textGrounds  = ['bg', 'bg-2', 'panel', 'glow', 'tint'].map(g);
  const boundGrounds = ['bg', 'bg-2', 'panel', 'glow'].map(g);

  const findings = [];
  const check = (token, need, grounds, kind) => {
    const fg = k[token];
    if (!fg) return;
    for (const [name, hex] of grounds){
      const ratio = contrast(fg, hex);
      if (ratio < need - 0.005){
        findings.push({token, ground: '--' + name, ratio: Math.round(ratio*100)/100,
                       need, kind, colour: fg});
      }
    }
  };

  check('--fg',       4.5, textGrounds,  'text');
  check('--fg-dim',   5.6, textGrounds,  'text');
  check('--fg-faint', 4.5, textGrounds,  'text');
  check('--border',   3.0, boundGrounds, 'boundary');
  check('--border-2', 4.5, boundGrounds, 'boundary');

  for (const m of theme.hues.members) check('--h-' + m.name, 4.5, textGrounds, 'text');

  // Text ON a hue, which is the mirror of the row above and fails separately:
  // --on-hue is a fixed near-black or white, so a hue L chosen in the middle
  // of the range can leave nothing readable on top of it.
  check('--on-hue', 4.5, theme.hues.members.map(m => [
    'h-' + m.name, k['--h-' + m.name]]), 'text');

  // Named separately from the ratio findings: "no value exists" and "this
  // value fails" call for different fixes - the first means the ladder itself
  // has no room, the second means this token is wrong.
  const s = theme.surfaces, sm = mode === 'dark' ? s.dark : s.light;
  const dark = mode === 'dark';
  const N = (L) => okhex(Math.min(1, Math.max(0, L)), s.chroma, s.hue);
  const tg = ['bg', 'bg2', 'panel', 'glow', 'tint'].map(n => N(sm[n]));
  const bg4 = ['bg', 'bg2', 'panel', 'glow'].map(n => N(sm[n]));
  const unsolvable = [];
  for (const [token, target, grounds, chromaMul] of [
    ['--fg-dim',   5.6, tg,  1.5], ['--fg-faint', 4.5, tg,  1.5],
    ['--border',   3.0, bg4, 1.8], ['--border-2', 4.5, bg4, 1.8],
  ]){
    const r = solveNeutralChecked(target, grounds,
                                  {hue: s.hue, chroma: s.chroma*chromaMul, dark});
    if (!r.ok) unsolvable.push({token, need: target});
  }

  return {ok: findings.length === 0 && unsolvable.length === 0, findings, unsolvable, mode};
}

/* One line per finding, for a terminal or a settings panel. */
export function explainCheck(result){
  const out = [];
  for (const u of result.unsolvable){
    out.push(`${u.token}: no value clears ${u.need.toFixed(1)}:1 on every ground ` +
             `in this ladder - the surfaces are too close together`);
  }
  for (const f of result.findings){
    out.push(`${f.token} (${f.colour}) on ${f.ground}: ${f.ratio.toFixed(2)}:1, ` +
             `needs ${f.need.toFixed(1)} as ${f.kind}`);
  }
  return out;
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


/* ── place the free hues for maximum worst-pair separation ──────────────
   Anchored members (red = bad, green = good, blue = info, amber = warn) do
   not move: their meaning is worth more than a point of deltaE. Everything
   else is searched, coarse then fine, maximising the WORST pair - which is
   the only statistic that matters for a categorical set, because the palette
   fails at its closest pair and nowhere else.                            */
export function optimiseHues(theme, mode){
  const H = mode === 'dark' ? theme.hues.dark : theme.hues.light;
  const ms = theme.hues.members;
  const hexes = () => ms.map(m => okhex(H.L, H.C, (m.angle+360)%360));
  const score = () => separation(hexes()).worst;
  const free = ms.map((m,i)=>[m,i]).filter(([m])=>!m.anchored);
  let best = score();
  for (const step of [12, 5, 2, 1]){
    let moved = true, guard = 0;
    while (moved && guard++ < 60){
      moved = false;
      for (const [m] of free){
        for (const d of [step, -step]){
          const was = m.angle;
          m.angle = (m.angle + d + 360) % 360;
          const now = score();
          if (now > best + 1e-6){ best = now; moved = true; }
          else m.angle = was;
        }
      }
    }
  }
  return best;
}
