/* ═══════════════════════════════════════════════════════════════════════════
   The README artwork, generated from the same theme engine the product uses.

   The point is that none of these colours are typed. `tokens()` solves the
   neutrals against every surface they land on and generates the hue family in
   oklch, so the banner cannot drift from the visual system and cannot contain
   a value that would fail the contrast gate in the product.

     node design/readme-art.mjs      # rewrites design/readme/*.svg

   Two files per picture, one per theme. GitHub picks between them with
   <picture media="(prefers-color-scheme: dark)">, which is the only mechanism
   it honours in a README.
   ═══════════════════════════════════════════════════════════════════════ */
import { writeFileSync, mkdirSync } from 'node:fs';
import { tokens, DEFAULTS } from './theme.js';

const OUT = new URL('./readme/', import.meta.url);
mkdirSync(OUT, { recursive: true });

/* Font stacks come from the tokens, so the SVG asks for Fraunces, Inter Tight
   and JetBrains Mono first and falls back the same way the product does. A
   README SVG cannot load a webfont, so the fallbacks are what most readers
   actually see, and they are chosen to keep the same proportions. */
const esc = s => String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
const r2 = n => Math.round(n * 100) / 100;
const pol = (cx, cy, r, deg) => [r2(cx + r*Math.cos(deg*Math.PI/180)), r2(cy + r*Math.sin(deg*Math.PI/180))];

const PROGRAMS = [
  { name: 'shelf',   hue: 'sage'   },
  { name: 'graft',   hue: 'teal'   },
  { name: 'archi',   hue: 'steel'  },
  { name: 'snapper', hue: 'indigo' },
  { name: 'nudge',   hue: 'lilac'  },
  { name: 'grabbit', hue: 'sage'   },
];

const SERVICES = [
  ['registry', 'what each program declared'],
  ['config',   'layers, schema, provenance, live push'],
  ['store',    'location, migrations, backup, integrity'],
  ['secrets',  'keyring, namespaced per program'],
  ['observe',  'log, trace and metric, merged'],
  ['control',  'start, stop, invoke, health'],
  ['schedule', 'one scheduler for the whole estate'],
  ['bus',      'events between programs, by grant'],
  ['rules',    'who may run what, what must ask first'],
  ['hosted',   'programs with no binary of their own'],
];

const SURFACES = [
  ['CLI',      'terminal'],
  ['MCP',      'agents'],
  ['HTTP',     'scripts'],
  ['Window',   'a mouse'],
  ['Tray',     'presence'],
  ['Toast',    'attention'],
  ['Palette',  'keyboard'],
  ['Schedule', 'time'],
  ['URL',      'links'],
];

/* ── the mark: identity goes in, uniformity comes out ────────────────────
   Six coloured threads converge on one neutral hub, and a neutral fan leaves
   it. That is the whole thesis of the project in one glyph, so it is worth
   drawing rather than describing. */
function mark(t, cx, cy) {
  const out = [];
  const R_IN = 74, R_HUB = 15;
  PROGRAMS.forEach((p, i) => {
    const a = 200 + i * (140 / (PROGRAMS.length - 1));
    const [x, y] = pol(cx, cy, R_IN, a);
    const [hx, hy] = pol(cx, cy, R_HUB + 5, a);
    out.push(`<line x1="${x}" y1="${y}" x2="${hx}" y2="${hy}" stroke="${t[`--h-${p.hue}`]}" stroke-width="1.4" stroke-linecap="round" opacity=".78"/>`);
    out.push(`<circle cx="${x}" cy="${y}" r="4.2" fill="${t[`--h-${p.hue}`]}"/>`);
  });
  for (let i = 0; i < SURFACES.length; i++) {
    const a = 22 + i * (136 / (SURFACES.length - 1));
    const [x1, y1] = pol(cx, cy, R_HUB + 6, a);
    const [x2, y2] = pol(cx, cy, R_IN - 8, a);
    const [dx, dy] = pol(cx, cy, R_IN - 4, a);
    out.push(`<line x1="${x1}" y1="${y1}" x2="${x2}" y2="${y2}" stroke="${t['--border']}" stroke-width="1.1" stroke-linecap="round"/>`);
    out.push(`<circle cx="${dx}" cy="${dy}" r="2" fill="${t['--border-2']}"/>`);
  }
  out.push(`<rect x="${cx-R_HUB}" y="${cy-R_HUB}" width="${R_HUB*2}" height="${R_HUB*2}" rx="7" fill="${t['--panel']}" stroke="${t['--border-2']}" stroke-width="1.6"/>`);
  return out.join('\n    ');
}

function banner(mode) {
  const t = tokens(DEFAULTS, mode);
  const W = 900, H = 220;
  const swatches = DEFAULTS.hues.members.map((m, i) =>
    `<rect x="${58 + i*26}" y="197" width="18" height="6" rx="3" fill="${t[`--h-${m.name}`]}"/>`).join('\n    ');
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" role="img" aria-label="rig - the platform every in-house program runs on">
  <rect x=".5" y=".5" width="${W-1}" height="${H-1}" rx="16" fill="${t['--bg']}" stroke="${t['--border']}" stroke-opacity=".45"/>
  <g font-family='${esc(t['--sans'])}'>
    <text x="58" y="48" font-size="10.5" letter-spacing="1.7" fill="${t['--fg-faint']}">SPECIFICATION AND DESIGN PROTOTYPE</text>
    <text x="56" y="118" font-family='${esc(t['--disp'])}' font-size="72" font-weight="600" letter-spacing="-1.8" fill="${t['--fg']}">rig</text>
    <text x="58" y="160" font-size="18" fill="${t['--fg-dim']}">The platform every in-house program runs on.</text>
    <text x="58" y="184" font-size="13" fill="${t['--fg-faint']}">Programs declare once. rig projects that onto every surface.</text>
    ${swatches}
  </g>
  <g>
    ${mark(t, 742, 110)}
  </g>
</svg>
`;
}

function diagram(mode) {
  const t = tokens(DEFAULTS, mode);
  const W = 900, H = 600;
  const o = [];
  const sans = esc(t['--sans']), mono = esc(t['--mono']), disp = esc(t['--disp']);

  /* programs, and the coloured threads down into the hub */
  const PW = 116, PGAP = 16, PY = 28, PH = 34;
  const px0 = (W - (PROGRAMS.length*PW + (PROGRAMS.length-1)*PGAP)) / 2;
  const CONV = [450, 120];
  PROGRAMS.forEach((p, i) => {
    const x = px0 + i*(PW+PGAP), cx = x + PW/2;
    o.push(`<linearGradient id="thread${i}" x1="0" y1="0" x2="0" y2="1">`
         + `<stop offset="0" stop-color="${t[`--h-${p.hue}`]}"/>`
         + `<stop offset="1" stop-color="${t['--border']}"/></linearGradient>`);
    o.push(`<path d="M${r2(cx)} ${PY+PH} C ${r2(cx)} ${PY+PH+34}, ${CONV[0]} ${CONV[1]-34}, ${CONV[0]} ${CONV[1]}" fill="none" stroke="url(#thread${i})" stroke-width="1.5" opacity=".85"/>`);
    o.push(`<rect x="${r2(x)}" y="${PY}" width="${PW}" height="${PH}" rx="9" fill="${t['--bg-2']}" stroke="${t['--border']}" stroke-opacity=".7"/>`);
    o.push(`<circle cx="${r2(x+17)}" cy="${PY+PH/2}" r="4" fill="${t[`--h-${p.hue}`]}"/>`);
    o.push(`<text x="${r2(x+30)}" y="${PY+PH/2+4.5}" font-family='${mono}' font-size="12.5" fill="${t['--fg']}">${p.name}</text>`);
  });

  o.push(`<text x="450" y="146" text-anchor="middle" font-family='${mono}' font-size="10" letter-spacing="1.5" fill="${t['--fg-faint']}">DECLARES ONCE</text>`);
  o.push(`<text x="450" y="167" text-anchor="middle" font-family='${sans}' font-size="12.5" fill="${t['--fg-dim']}">commands · config schema · state · data · events</text>`);
  o.push(`<path d="M450 178 L450 192 M445 187 L450 192 L455 187" fill="none" stroke="${t['--border-2']}" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>`);

  /* the daemon */
  const RX = 90, RY = 200, RW = 720, RH = 208;
  o.push(`<rect x="${RX}" y="${RY}" width="${RW}" height="${RH}" rx="14" fill="${t['--panel']}" stroke="${t['--border']}"/>`);
  o.push(`<text x="${RX+28}" y="${RY+44}" font-family='${disp}' font-size="30" font-weight="600" letter-spacing="-.7" fill="${t['--fg']}">rig</text>`);
  o.push(`<text x="${RX+RW-28}" y="${RY+42}" text-anchor="end" font-family='${sans}' font-size="12" fill="${t['--fg-faint']}">one daemon, no GUI, upgrades on its own</text>`);
  o.push(`<line x1="${RX+28}" y1="${RY+60}" x2="${RX+RW-28}" y2="${RY+60}" stroke="${t['--border']}" stroke-opacity=".55"/>`);
  SERVICES.forEach(([name, gloss], i) => {
    const col = i % 2, row = (i - col) / 2;
    const x = RX + 28 + col*344, y = RY + 88 + row*25;
    o.push(`<text x="${x}" y="${y}" font-family='${mono}' font-size="12" fill="${t['--fg']}">${esc(name)}</text>`);
    o.push(`<text x="${x+78}" y="${y}" font-family='${sans}' font-size="11" fill="${t['--fg-faint']}">${esc(gloss)}</text>`);
  });

  o.push(`<text x="450" y="${RY+RH+22}" text-anchor="middle" font-family='${mono}' font-size="11" fill="${t['--fg-dim']}">one unix socket · length-prefixed protobuf · 6.2 µs round trip</text>`);

  /* the fan, and the surfaces it lands on */
  const FAN = [450, RY+RH+34];
  const SW = 88, SGAP = 8, SY = 484, SH = 32;
  const sx0 = (W - (SURFACES.length*SW + (SURFACES.length-1)*SGAP)) / 2;
  SURFACES.forEach(([name, reach], i) => {
    const x = sx0 + i*(SW+SGAP), cx = x + SW/2;
    o.push(`<path d="M${FAN[0]} ${FAN[1]} C ${FAN[0]} ${FAN[1]+26}, ${r2(cx)} ${SY-26}, ${r2(cx)} ${SY}" fill="none" stroke="${t['--border']}" stroke-width="1.1" opacity=".8"/>`);
    o.push(`<rect x="${r2(x)}" y="${SY}" width="${SW}" height="${SH}" rx="8" fill="${t['--bg-2']}" stroke="${t['--border']}" stroke-opacity=".7"/>`);
    o.push(`<text x="${r2(cx)}" y="${SY+SH/2+4.2}" text-anchor="middle" font-family='${mono}' font-size="11.5" fill="${t['--fg']}">${esc(name)}</text>`);
    o.push(`<text x="${r2(cx)}" y="${SY+SH+18}" text-anchor="middle" font-family='${sans}' font-size="10.5" fill="${t['--fg-faint']}">${esc(reach)}</text>`);
  });

  o.push(`<text x="450" y="568" text-anchor="middle" font-family='${sans}' font-size="12.5" fill="${t['--fg-dim']}">An app writes nothing to get any of these. It declared its commands; rig did the rest.</text>`);

  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${W} ${H}" width="${W}" height="${H}" role="img" aria-label="Six programs declare once into rig, which projects the declaration onto nine surfaces">
  <rect x=".5" y=".5" width="${W-1}" height="${H-1}" rx="16" fill="${t['--bg']}" stroke="${t['--border']}" stroke-opacity=".45"/>
  ${o.join('\n  ')}
</svg>
`;
}

for (const mode of ['dark', 'light']) {
  writeFileSync(new URL(`banner-${mode}.svg`, OUT), banner(mode));
  writeFileSync(new URL(`projection-${mode}.svg`, OUT), diagram(mode));
}
console.log('wrote design/readme/{banner,projection}-{dark,light}.svg');
