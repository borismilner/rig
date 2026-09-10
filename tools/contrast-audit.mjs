#!/usr/bin/env node
// The contrast gate. PLAN.md section 20 says a failing ratio fails the build;
// this is what makes that a mechanism rather than a sentence.
//
//   node tools/contrast-audit.mjs design/visual-system.html
//   node tools/contrast-audit.mjs --themes=dark page.html      # one theme
//   node tools/contrast-audit.mjs --only=focus,paint page.html  # one pass
//   node tools/contrast-audit.mjs --open='#t-lab' page.html      # one UI state
//
// Four passes, because one instrument cannot see all four things:
//
//   text   every text node against the ground it is painted on, with opacity
//          groups composited - the declared colour is not what lands on screen
//   svg    <text> against the <rect>s actually behind it, in screen space
//   focus  focus indicators, from pixels, at WCAG 1.4.11's 3:1. No text pass
//          can see one: they walk nodeType 3, and a ring is not a text node
//   paint  anything dimmed or color-mixed, read off the screenshot rather than
//          out of getComputedStyle
//   bound  borders, outlines and rules, also at 1.4.11's 3:1. The other half of
//          what a text pass cannot see, and the reason --border:#556579 sat
//          labelled "verified" while failing on all four surfaces
//
// The history that shaped this: the target used to invoke tools/contrast-audit.mjs
// with --themes and --fail-under, and that file had never existed, so `make
// contrast` exited "Cannot find module" and no workflow called it at all. While
// it was down, rig's focus ring shipped at 2.17:1 dark and 1.48:1 light against
// a 3:1 requirement, and a stopped marker at 2.89:1 dark - none of which the
// text passes would have caught even had they run.
import {resolve, dirname, join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {withChrome, auditPage} from './contrast/headless.mjs';
import {focusPass} from './contrast/pass-focus.mjs';
import {paintPass} from './contrast/pass-paint.mjs';

const HERE = dirname(fileURLToPath(import.meta.url));
const DOM = join(HERE, 'contrast/pass-dom-text.js');
const SVG = join(HERE, 'contrast/pass-svg-text.js');
const BOUND = join(HERE, 'contrast/pass-boundary.js');
const ALL = ['text', 'svg', 'bound', 'focus', 'paint'];

const args = process.argv.slice(2);
const flag = name => {
  const a = args.find(x => x.startsWith(`--${name}=`));
  return a ? a.slice(name.length + 3) : null;
};
const files = args.filter(a => !a.startsWith('-'));
const themes = (flag('themes') || 'dark,light').split(',').map(s => s.trim()).filter(Boolean);
const only = (flag('only') || ALL.join(',')).split(',').map(s => s.trim()).filter(Boolean);
// A page has states, and a static audit only ever measures the one that happens
// to be showing. On this page 29 focus targets live in a panel that is
// translated off-screen until something opens it, so they were reported as
// off-viewport and left unmeasured on every run. --open clicks its way into a
// state first, using the page's own handlers rather than reaching in and
// setting a class, so what is measured is a state a user can actually reach.
const opens = (flag('open') || '').split(',').map(s => s.trim()).filter(Boolean);

// A threshold that applies to everything is the wrong shape: 1.4.3 wants 4.5:1
// for body text and 3:1 once it is large, and 1.4.11 wants 3:1 for a boundary.
// The old target passed --fail-under=4.5; saying so beats ignoring it.
if (flag('fail-under') !== null) {
  console.error('contrast: --fail-under is not accepted. Each rule carries its own\n' +
                '          threshold (1.4.3: 4.5 / 3 by size; 1.4.11: 3). Drop the flag.');
  process.exit(2);
}
const unknown = only.filter(p => !ALL.includes(p));
if (unknown.length) { console.error(`contrast: unknown pass(es): ${unknown.join(', ')}`); process.exit(2); }
if (!files.length) {
  console.error('usage: node tools/contrast-audit.mjs [--themes=dark,light] [--only=text,svg,focus,paint] <file.html> ...');
  process.exit(2);
}

const label = f => f.split('/').slice(-2).join('/');
const pad = (s, n) => String(s).padEnd(n);
const num = (s, n) => String(s).padStart(n);

let failed = 0, silencedTotal = 0;
const hiddenRows = [], notMeasured = [];

await withChrome(async browser => {
  for (const f of files) {
    for (const theme of themes) {
      // A served URL as well as a path. An SPA cannot be audited from disk at
      // all: Chrome refuses a module script over file://, so the page loads,
      // renders nothing and every pass measures zero - which this gate used to
      // call clean. See the zero-measurement guard below.
      const target = /^https?:\/\//.test(f) ? f : 'file://' + resolve(f);
      const page = await auditPage(browser, target, {theme});
      for (const sel of opens) {
        const hit = await page.eval(`(() => {
          const el = document.querySelector(${JSON.stringify(sel)});
          if (!el) return 'missing';
          el.click();
          return 'clicked';
        })()`);
        if (hit === 'missing') {
          console.error(`contrast: --open selector not found: ${sel}`);
          process.exit(2);
        }
        // the click may have started transitions and animations of its own
        await page.quiesce();
        await page.settle();
      }
      const head = `${label(f)} [${theme}]${opens.length ? ' after ' + opens.join(' + ') : ''}`;

      const dom = only.includes('text') ? await page.run(DOM) : null;
      // Measuring nothing is a failure. This gate reported "clean: every pass
      // green in every theme" over a page whose script had not run, on a tree
      // whose history already records the ancestor script finding 0 of 1 real
      // failures while inventing 2. A number of zero is the one result that
      // cannot be trusted, so it is now named and fatal rather than green.
      if (dom && dom.checked === 0) {
        console.log(`${head} MEASURED NOTHING - 0 text nodes on "${dom.page}".` +
                    ' An empty page cannot be clean: check that the page rendered' +
                    ' (an SPA needs a served URL, not a file:// path).');
        failed++; await page.close(); continue;
      }
      // A theme read in the same task as the flip that set it returns the
      // PREVIOUS theme, so assert what was actually measured before printing it.
      if (dom && dom.theme !== theme) {
        console.log(`${head} IDENTITY MISMATCH - measured theme "${dom.theme}" on "${dom.page}"`);
        failed++; await page.close(); continue;
      }
      const svg = only.includes('svg') ? await page.run(SVG) : null;
      const bound = only.includes('bound') ? await page.run(BOUND) : null;
      const focus = only.includes('focus') ? await focusPass(page) : null;
      const paint = only.includes('paint') ? await paintPass(page) : null;

      console.log(`\n${head}  ${dom ? `"${dom.page}"` : ''}`);
      if (dom) {
        const tiny = dom.under12px;
        console.log(`  text  ${num(dom.checked, 4)} nodes    ${num(dom.failures, 3)} fail  ` +
                    `lowest ${num(dom.lowest, 5)}  <12px ${tiny}  ` +
                    `hidden-by-declared ${dom.hidden.length}` +
                    (dom.failures || tiny ? '   <-- LOOK' : ''));
        if (dom.failures || tiny) failed++;
        silencedTotal += dom.silenced;
        for (const b of dom.bad.slice(0, 6)) {
          console.log(`        ${pad(String(b.sel).slice(0, 26), 26)} ${b.fg} on ${b.bg}  ` +
                      `${num(b.ratio, 5)}:1 need ${b.need}  ${b.px}px  opacity ${b.opacity}  "${b.text}"`);
        }
        for (const t of dom.tiny.slice(0, 4)) console.log(`        TINY ${pad(t.sel, 26)} ${t.px}px  "${t.text}"`);
        hiddenRows.push(...dom.hidden.map(h => ({theme, pass: 'text', ...h})));
      }
      if (svg) {
        console.log(`  svg   ${num(svg.checked, 4)} nodes    ${num(svg.failures, 3)} fail  ` +
                    `lowest ${num(svg.lowest ?? '-', 5)}` + (svg.failures ? '   <-- LOOK' : ''));
        if (svg.failures) failed++;
        for (const b of svg.bad.slice(0, 6)) {
          console.log(`        ${pad(String(b.cls).slice(0, 26), 26)} ${b.fg} on ${b.bg}  ` +
                      `${num(b.ratio, 5)}:1 need ${b.need}  ${b.px}px  "${b.text}"`);
        }
      }
      if (bound) {
        console.log(`  bound ${num(bound.checked, 4)} edges    ${num(bound.failures, 3)} fail  ` +
                    `lowest ${num(bound.lowest ?? '-', 5)}  need 3 (WCAG 1.4.11)  ` +
                    `under 3 but decorative ${bound.informational}` +
                    (bound.failures ? '   <-- LOOK' : ''));
        if (bound.failures) failed++;
        silencedTotal += bound.silenced;
        for (const b of bound.bad.slice(0, 6)) {
          console.log(`        ${pad(b.label.slice(0, 26), 26)} ${b.kind} ${b.px}px  ${b.paint}  ` +
                      `out ${num(b.outsideRatio, 5)} in ${num(b.insideRatio, 5)}  ` +
                      `${b.token ? 'token ' + b.token : 'interactive'}`);
        }
        if (bound.bad.length > 6) console.log(`        ... and ${bound.bad.length - 6} more`);
        // Held to 3:1 by the engine and not enforced here would be a
        // contradiction worth seeing, so print what the tokens actually are.
        if (bound.failures) console.log(`        held tokens: --border ${bound.held['border']}, --border-2 ${bound.held['border-2']}`);
      }
      if (focus) {
        console.log(`  focus ${num(focus.examined, 4)} targets  ${num(focus.failures, 3)} fail  ` +
                    `lowest ${num(focus.lowest ?? '-', 5)}  need 3 (WCAG 1.4.11)  ` +
                    `not measured ${focus.skipped.length}` +
                    (focus.failures ? '   <-- LOOK' : ''));
        if (focus.failures) failed++;
        silencedTotal += focus.silenced.length;
        for (const b of focus.findings.slice(0, 6)) {
          console.log(b.kind === 'ring-low'
            ? `        ${pad(b.label.slice(0, 26), 26)} ring ${b.ring} on ${b.ground}  ${num(b.ratio, 5)}:1  ` +
              `state-change ${b.change ?? '-'}  "${b.text}"`
            : `        ${pad(b.label.slice(0, 26), 26)} ${b.kind}: ${b.note}  "${b.text}"`);
        }
        if (focus.findings.length > 6) console.log(`        ... and ${focus.findings.length - 6} more`);
        notMeasured.push(...focus.skipped.map(x => ({theme, pass: 'focus', ...x})));
      }
      if (paint) {
        console.log(`  paint ${num(paint.examined, 4)} marks    ${num(paint.failures, 3)} fail  ` +
                    `lowest ${num(paint.lowest ?? '-', 5)}  hidden-by-declared ${paint.hidden.length}  ` +
                    `not measured ${paint.skipped.length}` +
                    (paint.failures ? '   <-- LOOK' : ''));
        if (paint.failures) failed++;
        silencedTotal += paint.silenced.length;
        for (const b of paint.findings.slice(0, 6)) {
          console.log(`        ${pad(b.label.slice(0, 26), 26)} paints ${b.paint} on ${b.ground}  ` +
                      `${num(b.ratio, 5)}:1 need ${b.need}  declared ${b.declared} would read ${b.declaredRatio}:1`);
        }
        if (paint.findings.length > 6) console.log(`        ... and ${paint.findings.length - 6} more`);
        hiddenRows.push(...paint.hidden.map(h => ({theme, pass: 'paint', ...h})));
        notMeasured.push(...paint.skipped.map(x => ({theme, pass: 'paint', ...x})));
      }
      await page.close();
    }
  }
});

// The rows that make the difference between this gate and the one it replaces:
// a declared-colour audit calls every one of these clean.
if (hiddenRows.length) {
  console.log(`\n${hiddenRows.length} row(s) a declared-colour audit reports as passing and the screen does not:`);
  for (const h of hiddenRows.slice(0, 12)) {
    console.log(`  [${h.theme}] ${h.pass} ${pad((h.sel || h.label || '').slice(0, 28), 28)} ` +
                `painted ${num(h.ratio, 5)}:1, declared ${h.declaredRatio}:1, need ${h.need}`);
  }
}
if (notMeasured.length) {
  console.log(`\n${notMeasured.length} element(s) NOT measured, by pass and reason:`);
  const byReason = {};
  for (const n of notMeasured) {
    const k = `${n.pass} / ${n.kind}${n.onTop ? ' behind ' + n.onTop : ''}`;
    byReason[k] = (byReason[k] || 0) + 1;
  }
  for (const [k, n] of Object.entries(byReason).sort((a, b) => b[1] - a[1])) {
    console.log(`  ${num(n, 4)}  ${k}`);
  }
  console.log('  examples:');
  for (const n of notMeasured.slice(0, 5)) {
    console.log(`    ${pad((n.label || n.sel || '').slice(0, 26), 26)} at ${n.at || '?'}  ` +
                `${n.kind}${n.onTop ? ' behind ' + n.onTop : ''}  "${n.text || ''}"`);
  }
}
if (silencedTotal) {
  console.log(`\n${silencedTotal} finding(s) silenced by data-contrast-ok. A silence is visible here on ` +
              `every run, which is the only reason it is allowed to exist.`);
}
console.log(failed
  ? `\n${failed} page/theme/pass combination(s) FAIL - a failing ratio fails the build (PLAN.md section 20)`
  : '\nclean: every pass green in every theme');
process.exit(failed ? 1 : 0);
