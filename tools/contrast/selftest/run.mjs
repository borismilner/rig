// Known-answer tests for the contrast gate's own instruments.
//
// The reason this exists, in one line from the readable-output skill: the SVG
// audit shipped for months dropping alpha, and on its own self-test it found
// 0 of 1 real failures while inventing 2. `make contrast` runs this FIRST, so
// the gate cannot report on a page through a broken instrument.
import {dirname, join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {ratio, parseColour, hex} from '../wcag.mjs';
import {withChrome, auditPage} from '../headless.mjs';
import {focusPass} from '../pass-focus.mjs';
import {paintPass} from '../pass-paint.mjs';

const HERE = dirname(fileURLToPath(import.meta.url));
let failed = 0;

const eq = (name, got, want) => {
  const ok = got === want;
  if (!ok) failed++;
  console.log(`  ${ok ? 'ok  ' : 'FAIL'} ${name.padEnd(52)} got ${JSON.stringify(got)} want ${JSON.stringify(want)}`);
};
const near = (name, got, want, tol) => {
  const ok = got !== null && Math.abs(got - want) <= tol;
  if (!ok) failed++;
  console.log(`  ${ok ? 'ok  ' : 'FAIL'} ${name.padEnd(52)} got ${got} want ${want} +/-${tol}`);
};

const nearColour = (name, got, want, tol) => {
  const a = got && parseColour(got), b = parseColour(want);
  const ok = !!a && Math.abs(a.r - b.r) <= tol && Math.abs(a.g - b.g) <= tol && Math.abs(a.b - b.b) <= tol;
  if (!ok) failed++;
  console.log(`  ${ok ? 'ok  ' : 'FAIL'} ${name.padEnd(52)} got ${got} want ${want} +/-${tol}/channel`);
};

// 1. The arithmetic, against values that are published and checkable.
console.log('wcag arithmetic');
near('white on black', ratio(parseColour('#ffffff'), parseColour('#000000')), 21.00, 0.005);
near('#767676 on white', ratio(parseColour('#767676'), parseColour('#ffffff')), 4.54, 0.01);
near('#949494 on white', ratio(parseColour('#949494'), parseColour('#ffffff')), 3.03, 0.01);
near('#c0c0c0 on white', ratio(parseColour('#c0c0c0'), parseColour('#ffffff')), 1.82, 0.01);
// The trap that made a whole set of readings near-black: Chrome returns a
// color-mix() as 0-1 floats, and /[\d.]+/ reads 0.18 as 0.18 of 255.
eq('color(srgb ...) parsed as 0-1 floats', hex(parseColour('color(srgb 0.5 0.5 0.5)')), '#808080');
near('color(srgb ...) alpha kept', parseColour('color(srgb 0 0 0 / 0.36)').a, 0.36, 1e-9);

await withChrome(async browser => {
  // 2. The pixel pipeline: decode a screenshot and assert the colour is exact.
  console.log('\nfocus pass, known answers');
  const page = await auditPage(browser, 'file://' + join(HERE, 'focus.html'));
  eq('page identity', await page.eval('document.title'), 'focus pass selftest');

  const f = await focusPass(page);
  const by = id => f.measured.find(m => m.label.startsWith('button#' + id));

  eq('five focusables found', f.focusable, 5);
  // Exact colours, not ratios: if the pixels come back right the arithmetic is
  // already covered above, and an exact colour cannot be fudged by a tolerance.
  eq('#ok ring colour read from pixels', by('ok')?.ring, '#949494');
  eq('#ok ground read from pixels', by('ok')?.ground, '#ffffff');
  near('#ok ratio', by('ok')?.ratio ?? null, 3.03, 0.02);
  eq('#low ring colour', by('low')?.ring, '#c0c0c0');
  near('#low ratio', by('low')?.ratio ?? null, 1.82, 0.02);

  // THE modality test. A ring on :focus-visible paints nothing under a bare
  // el.focus() unless Chrome already believes the user is on the keyboard, and
  // a pass that got this wrong would report every element as unstyled.
  eq(':focus-visible ring is seen at all', !!by('visible'), true);
  eq(':focus-visible ring colour', by('visible')?.ring, '#949494');

  const kinds = Object.fromEntries(f.findings.map(x => [x.label.replace(/^button#/, ''), x.kind]));
  eq('#none reported as no-indicator', kinds.none, 'no-indicator');
  eq('#low reported as ring-low', kinds.low, 'ring-low');
  eq('#ok not reported', kinds.ok, undefined);
  eq('data-contrast-ok silences, does not hide', f.silenced.length, 1);
  eq('silenced one is #silenced', f.silenced[0]?.label, 'button#silenced');
  eq('two failures, no more', f.failures, 2);
  await page.close();

  // 3. The opacity blind spot, which is the one the DOM audit cannot see.
  console.log('\npaint pass, known answers');
  const p2 = await auditPage(browser, 'file://' + join(HERE, 'paint.html'));
  eq('page identity', await p2.eval('document.title'), 'paint pass selftest');
  const p = await paintPass(p2);
  const mark = id => p.measured.find(m => m.label.includes('#' + id));

  // The row the whole pass exists for: declared 21.00:1, painted 2.85:1.
  nearColour('#dim painted colour', mark('dim')?.paint, '#999999', 1);
  near('#dim painted ratio', mark('dim')?.ratio ?? null, 2.85, 0.05);
  eq('#dim declared colour kept', mark('dim')?.declared, '#000000');
  near('#dim declared ratio', mark('dim')?.declaredRatio ?? null, 21.00, 0.05);
  eq('#dim flagged as hidden by the declared colour', mark('dim')?.hiddenByDeclared, true);
  eq('#dim is a failure', p.findings.some(x => x.label.includes('#dim')), true);

  // and the control: the pass must not simply fail everything it looks at
  near('#solid painted ratio', mark('solid')?.ratio ?? null, 21.00, 0.05);
  eq('#solid is not a failure', p.findings.some(x => x.label.includes('#solid')), false);
  eq('#solid not flagged as hidden', mark('solid')?.hiddenByDeclared, false);

  nearColour('#mixed painted colour', mark('mixed')?.paint, '#cccccc', 1);
  eq('#mixed declared and painted agree', mark('mixed')?.declared, '#cccccc');
  eq('#mixed not flagged as hidden, the DOM pass can see it', mark('mixed')?.hiddenByDeclared, false);

  nearColour('#chip shape colour off the band', mark('chip')?.paint, '#b0b0b0', 1);
  eq('#chip needs 3, not 4.5', mark('chip')?.need, 3);
  await p2.close();
});

console.log(failed ? `\n${failed} selftest assertion(s) FAILED - do not trust the gate` : '\nselftest: all assertions pass');
process.exit(failed ? 1 : 0);
