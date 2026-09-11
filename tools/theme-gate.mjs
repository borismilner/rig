// The theme gate: refuse a token set a person could not read.
//
// Section 6 says rig "refuses to apply a token set that fails, naming the token
// and the ground, so a theme cannot silently produce an unreadable product".
// checkTheme in design/theme.js is that judgement; this is the half that RUNS -
// over the shipped default, in both themes, on every build.
//
// It is not a second implementation of the contrast gate and does not replace
// it. The division:
//
//   make contrast / contrast-window   what gets PAINTED, in a real browser.
//                                     Sees composited grounds, color-mix(),
//                                     focus rings, actual font sizes.
//   this gate                         the TOKEN SET, arithmetically, before a
//                                     browser exists. Sees every surface a
//                                     token can land on, including the ones no
//                                     current page happens to use.
//
// Neither subsumes the other. A page can paint clean while the token set has a
// pair no page has reached yet, and a token set can be arithmetically sound
// while a page composites it into something unreadable.
//
// --selftest first, and for the same reason contrast-selftest exists: this
// gate's whole job is to say no, so an instrument that cannot say no is worse
// than no instrument. The cases include deliberately unreadable themes and the
// run fails if any of them is called clean.
import {DEFAULTS, checkTheme, explainCheck} from '../design/theme.js';

const MODES = ['dark', 'light'];
const clone = o => JSON.parse(JSON.stringify(o));

function judge(theme, label) {
  let ok = true;
  const lines = [];
  for (const mode of MODES) {
    const r = checkTheme(theme, mode);
    if (!r.ok) {
      ok = false;
      for (const l of explainCheck(r)) lines.push(`  ${mode.padEnd(5)} ${l}`);
    }
  }
  return {ok, lines, label};
}

/* ── the known answers ───────────────────────────────────────────────────
 * Each case says what a person could plausibly type and what must happen.
 * The broken ones matter more than the clean one: a gate that passes
 * everything is indistinguishable from a gate that is switched off.
 */
function selftest() {
  const cases = [];

  cases.push(['the shipped default is readable', DEFAULTS, true, []]);

  // --fg is CHOSEN from the ladder rather than solved, so it is the one knob a
  // settings box breaks directly.
  const fgDark = clone(DEFAULTS); fgDark.surfaces.dark.fg = 0.40;
  cases.push(['dark --fg dragged toward the panel', fgDark, false, ['--fg', '--panel']]);

  const fgLight = clone(DEFAULTS); fgLight.surfaces.light.fg = 0.80;
  cases.push(['light --fg dragged toward the panel', fgLight, false, ['--fg']]);

  // One lightness for every surface AND the text: 1.00:1 everywhere.
  const flat = clone(DEFAULTS);
  flat.surfaces.dark = {bg: .5, bg2: .5, panel: .5, glow: .5, tint: .5, fg: .5};
  cases.push(['dark ladder collapsed to one lightness', flat, false, ['1.00:1']]);

  // A hue is TEXT wherever --sem-<role> or --hue is used as a colour.
  const dimHue = clone(DEFAULTS); dimHue.hues.dark = {L: 0.35, C: 0.098};
  cases.push(['dark hues dimmed to L 0.35', dimHue, false, ['--h-sage', '--tint']]);

  // The mirror: the hue is fine as text, but --on-hue cannot sit on it.
  const paleHue = clone(DEFAULTS); paleHue.hues.light = {L: 0.88, C: 0.11};
  cases.push(['light hues pushed to L 0.88', paleHue, false, ['--on-hue']]);

  // Knobs this gate must NOT judge. It measures colour; type size and radius
  // are gated elsewhere (the schema bounds them, and the browser gate reads
  // computed font sizes).
  const shape = clone(DEFAULTS);
  shape.type.base = 22; shape.type.scale = 1.45; shape.shape.radius = 0;
  cases.push(['non-colour knobs at their extremes', shape, true, []]);

  let wrong = 0;
  console.log('theme gate, known answers');
  for (const [label, theme, wantOk, mentions] of cases) {
    const r = judge(theme, label);
    const text = r.lines.join('\n');
    const missing = mentions.filter(m => !text.includes(m));
    const good = r.ok === wantOk && missing.length === 0;
    if (!good) wrong++;
    console.log('  %s %s', good ? 'ok  ' : 'WRONG', label.padEnd(42) +
                ' refused=' + String(!r.ok).padEnd(5) + ' want=' + String(!wantOk));
    if (!good && missing.length) {
      console.log('        the refusal never named: %s', missing.join(', '));
      console.log('        it said:\n%s', r.lines.slice(0, 4).join('\n') || '        (nothing)');
    }
  }
  if (wrong) {
    console.error('\ntheme gate: %d of %d known answers wrong. The instrument is not ' +
                  'trustworthy, so no verdict it gives about a real theme is either.',
                  wrong, cases.length);
    return 2;
  }
  console.log('  %d known answers, all correct\n', cases.length);
  return 0;
}

/* ── the gate proper ─────────────────────────────────────────────────────── */
function gate() {
  const r = judge(DEFAULTS, 'the shipped default theme');
  if (r.ok) {
    console.log('theme gate: the default token set is readable in both themes.');
    console.log('  checked --fg, --fg-dim, --fg-faint on five text grounds;');
    console.log('  --border, --border-2 on four boundary grounds;');
    console.log('  every --h-* as text, and --on-hue against every hue.');
    return 0;
  }
  console.error('theme gate: the default token set is NOT readable.\n');
  for (const l of r.lines) console.error(l);
  console.error('\nEvery line names the token and the ground it failed on, because the fix\n' +
                'differs by ground: a token failing only on --tint is a token used on the\n' +
                'wrong surface, and one failing on all five is the wrong colour.');
  return 1;
}

const args = process.argv.slice(2);
process.exit(args.includes('--selftest') ? selftest() || gate() : gate());
