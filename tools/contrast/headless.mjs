// Headless Chrome harness for the contrast gate.
//
// Its own browser on an ephemeral port, never a shared endpoint: three agents
// on 2026-09-05 recorded readings from another session's tab, one reporting 135
// nodes for a 453-node page. Every result carries the page identity so a number
// can be proved to come from the page it claims.
//
// Adapted from ~/.claude/skills/readable-output/references/headless-audit.mjs.
// It lives in the repository because CI has no home directory to reach into,
// and a gate that only runs on one laptop is not a gate.
import {spawn} from 'node:child_process';
import {mkdtempSync, rmSync, readFileSync, existsSync} from 'node:fs';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {decodePng} from './png.mjs';

// A theme flip animates `color` whenever a `transition` shorthand carries a bare
// duration, and getComputedStyle in the same task reads the animation at t=0 -
// which is the PREVIOUS theme's colour. A whole session's figures were once
// invalidated this way.
const SETTLE = 450;

const CANDIDATES = [
  process.env.CHROME,
  '/usr/bin/google-chrome',
  '/usr/bin/google-chrome-stable',
  '/usr/bin/chromium',
  '/usr/bin/chromium-browser',
  '/snap/bin/chromium',
];

export function findChrome() {
  for (const c of CANDIDATES) if (c && existsSync(c)) return c;
  throw new Error(
    'contrast: no Chrome found. Tried:\n  ' + CANDIDATES.filter(Boolean).join('\n  ') +
    '\nSet CHROME=/path/to/chrome, or install google-chrome-stable.');
}

export async function withChrome(fn, {width = 1440, height = 900} = {}) {
  const bin = findChrome();
  const prof = mkdtempSync(join(tmpdir(), 'rig-contrast-'));
  const chrome = spawn(bin, [
    '--headless=new', '--remote-debugging-port=0', `--user-data-dir=${prof}`,
    '--no-first-run', '--no-default-browser-check', '--disable-gpu',
    '--hide-scrollbars', '--force-device-scale-factor=1',
    '--no-sandbox',                       // CI containers run as root; there is no untrusted content here
    `--window-size=${width},${height}`, 'about:blank',
  ], {stdio: ['ignore', 'pipe', 'pipe']});

  const wsUrl = await new Promise((res, rej) => {
    let buf = '';
    const t = setTimeout(() => rej(new Error('chrome did not report a debug port:\n' + buf)), 20000);
    chrome.stderr.on('data', d => {
      buf += d;
      const m = buf.match(/ws:\/\/[^\s]+/);
      if (m) { clearTimeout(t); res(m[0]); }
    });
    chrome.on('exit', c => { clearTimeout(t); rej(new Error('chrome exited ' + c + '\n' + buf)); });
  });

  const browser = await connect(wsUrl);
  try { return await fn(browser); }
  finally {
    try { chrome.kill('SIGTERM'); } catch {}
    // Chrome keeps writing its profile for a moment after SIGTERM, so an
    // immediate rm races it and throws ENOTEMPTY after the audit already passed.
    for (let i = 0; i < 20; i++) {
      try { rmSync(prof, {recursive: true, force: true}); break; }
      catch { await new Promise(r => setTimeout(r, 100)); }
    }
  }
}

function connect(url) {
  return new Promise((res, rej) => {
    const ws = new WebSocket(url);
    let id = 0; const pending = new Map(); const listeners = [];
    ws.onerror = e => rej(new Error('ws error: ' + e.message));
    ws.onmessage = ev => {
      const m = JSON.parse(ev.data);
      if (m.id && pending.has(m.id)) {
        const {resolve, reject} = pending.get(m.id); pending.delete(m.id);
        m.error ? reject(new Error(m.error.message)) : resolve(m.result);
      } else listeners.forEach(l => l(m));
    };
    ws.onopen = () => res({
      send: (method, params = {}, sessionId) => new Promise((resolve, reject) => {
        const i = ++id; pending.set(i, {resolve, reject});
        ws.send(JSON.stringify({id: i, method, params, ...(sessionId ? {sessionId} : {})}));
      }),
      on: l => listeners.push(l),
      close: () => ws.close(),
    });
  });
}

export async function auditPage(browser, fileUrl, {theme = null, width = 1440, height = 900} = {}) {
  const {targetId} = await browser.send('Target.createTarget', {url: 'about:blank'});
  const {sessionId} = await browser.send('Target.attachToTarget', {targetId, flatten: true});
  const S = (m, p) => browser.send(m, p, sessionId);
  await S('Page.enable'); await S('Runtime.enable');
  await S('Emulation.setDeviceMetricsOverride', {width, height, deviceScaleFactor: 1, mobile: false});

  const loaded = new Promise(res => {
    browser.on(m => { if (m.sessionId === sessionId && m.method === 'Page.loadEventFired') res(); });
  });
  await S('Page.navigate', {url: fileUrl});
  await loaded;

  const evalIn = async (expr, awaitPromise = false) => {
    const r = await S('Runtime.evaluate', {expression: expr, returnByValue: true, awaitPromise});
    if (r.exceptionDetails) {
      throw new Error(JSON.stringify(r.exceptionDetails.exception?.description || r.exceptionDetails));
    }
    return r.result.value;
  };

  if (theme) await evalIn(`document.documentElement.setAttribute('data-theme','${theme}')`);
  else       await evalIn(`document.documentElement.removeAttribute('data-theme')`);
  await settle(evalIn);
  await quiesce(evalIn);
  await settle(evalIn);   // one frame after the stylesheet lands

  return {
    run: async scriptPath => evalIn(readFileSync(scriptPath, 'utf8')),
    eval: evalIn,
    settle: () => settle(evalIn),
    quiesce: () => quiesce(evalIn),

    // Wait only as long as this element's own transition, plus two frames. The
    // 450ms settle is for a theme flip; paying it twice per focusable element
    // put the gate at five and a half minutes on one page.
    // quiesce() has already turned transitions off, so a focus state lands in
    // the next frame and there is nothing to wait out. Two frames, not 450ms:
    // paying the theme-flip settle twice per focusable element put this gate at
    // five and a half minutes on one page.
    paint: async () => evalIn(
      `new Promise(r => requestAnimationFrame(() => requestAnimationFrame(r)))`, true),

    // Put the element in the viewport and report where it is now. Every clip
    // below is taken with captureBeyondViewport off, so this is what makes a
    // sticky or fixed element measurable at all - see the note on shot().
    reveal: async selector => evalIn(`(() => {
      const el = document.querySelector(${JSON.stringify(selector)});
      if (!el) return null;
      el.scrollIntoView({block: 'center', inline: 'center', behavior: 'instant'});
      const r = el.getBoundingClientRect();
      const cx = r.left + r.width / 2, cy = r.top + r.height / 2;
      // Do NOT clamp this point into the viewport. Clamping tests some other
      // element at the edge instead, and 29 focus targets sitting off-screen at
      // x=1779 were reported as "occluded behind div" - a wrong reason, which
      // is worse than no reason because it sends the reader looking for an
      // overlay that does not exist.
      const within = cx >= 0 && cx < innerWidth && cy >= 0 && cy < innerHeight;
      const hit = within ? document.elementFromPoint(cx, cy) : null;
      return {
        x: r.left + scrollX, y: r.top + scrollY, width: r.width, height: r.height,
        inViewport: r.bottom > 0 && r.top < innerHeight && r.right > 0 && r.left < innerWidth,
        // Something else on top means the pixels in this box are not this
        // element's, and every colour read out of them belongs to the overlay.
        occluded: within && !!hit && !(el === hit || el.contains(hit) || hit.contains(el)),
        offViewport: !within,
        // A fixed element is painted relative to the VIEWPORT, and every clip
        // here is in page coordinates, so an open fixed panel lands in the
        // capture of anything at those coordinates whether or not it is
        // anywhere near it. That produced a "no focus indicator" against a
        // button whose ring is fine: the shot was of the panel. A hit test on
        // the centre point cannot see it, because the panel may cover only part
        // of the box. So: does any fixed element intersect this rect at all.
        underOverlay: (() => {
          for (const o of document.querySelectorAll('*')) {
            if (o === el || o.contains(el) || el.contains(o)) continue;
            const cs = getComputedStyle(o);
            if (cs.position !== 'fixed') continue;
            if (cs.visibility === 'hidden' || cs.display === 'none' || +cs.opacity === 0) continue;
            const b = o.getBoundingClientRect();
            if (b.width < 1 || b.height < 1) continue;
            if (b.left < r.right && b.right > r.left && b.top < r.bottom && b.bottom > r.top) {
              return o.id ? '#' + o.id : o.tagName.toLowerCase();
            }
          }
          return null;
        })(),
        onTop: hit ? (hit.id ? '#' + hit.id : hit.tagName.toLowerCase()) : null,
      };
    })()`),

    // A real Tab keypress, not el.focus(). Chrome only matches :focus-visible
    // once it believes the user is navigating by keyboard, so a page whose ring
    // is on :focus-visible paints NOTHING under programmatic focus and the pass
    // would report every element as having no indicator. selftest/focus.html
    // holds that case so the instrument cannot regress into it silently.
    keyboardModality: async () => {
      for (const type of ['keyDown', 'keyUp']) {
        await S('Input.dispatchKeyEvent', {
          type, key: 'Tab', code: 'Tab', windowsVirtualKeyCode: 9, nativeVirtualKeyCode: 9, text: '\t',
        });
      }
      await settle(evalIn);
    },

    // Pixels, in document coordinates. captureBeyondViewport is what makes a
    // clip below the fold return the element rather than a slice of whatever
    // happens to be scrolled into view.
    // captureBeyondViewport resizes the viewport to the whole document, which
    // moves every fixed element to the top of the page and un-sticks every
    // sticky one. A clip computed from getBoundingClientRect then points at
    // whatever is at those page coordinates instead - which reads back as a
    // uniform slab of background, and a uniform slab measures 1.00:1. So:
    // reveal() scrolls the element into the viewport, and the shot is of the
    // viewport as it stands.
    shot: async clip => {
      const params = {format: 'png', captureBeyondViewport: false, optimizeForSpeed: true};
      if (clip) params.clip = {x: clip.x, y: clip.y, width: clip.width, height: clip.height, scale: 1};
      const {data} = await S('Page.captureScreenshot', params);
      return decodePng(Buffer.from(data, 'base64'));
    },

    // The same capture, undecoded, for when a human has to look at it. A clean
    // audit is not a clean page: every number can pass while a swatch reads as
    // an empty checkbox or a pin sits on the word it annotates.
    shotPng: async clip => {
      const params = {format: 'png', captureBeyondViewport: false};
      if (clip) params.clip = {x: clip.x, y: clip.y, width: clip.width, height: clip.height, scale: 1};
      const {data} = await S('Page.captureScreenshot', params);
      return Buffer.from(data, 'base64');
    },

    close: () => S('Target.closeTarget', {targetId}).catch(() => {}),
  };
}

// A page that animates measures differently depending on when you look, and
// rig's own page is worse than that: `section > .wrap > *` carries
// `animation: rise linear both` on `animation-timeline: view()`, so opacity is
// a function of SCROLL POSITION, not of time. Measured cold, 714 of its 866
// text nodes sit at opacity 0 because they have not been scrolled past yet, and
// an audit that reads them anyway reports 831 nodes it cannot see.
//
// So the gate quiesces the page instead of waiting for it: animations and
// transitions off, remaining ones cancelled, everything at its base value. That
// is the state a reader eventually sees, it is identical on every run, and it
// is the only state a build gate can be held to. Anything whose ONLY styling
// comes from an animation is measured unstyled - the count is reported so that
// trade is visible rather than assumed.
function quiesce(evalIn) {
  return evalIn(`(() => {
    const id = 'rig-contrast-quiesce';
    if (!document.getElementById(id)) {
      const st = document.createElement('style');
      st.id = id;
      st.textContent = '*, *::before, *::after {' +
        'animation: none !important; transition: none !important;' +
        'animation-timeline: none !important; view-transition-name: none !important;' +
        // scroll-behavior: smooth makes scrollIntoView asynchronous, so the rect
        // read on the next line is the PRE-scroll one and every pixel coordinate
        // taken from it points at whatever used to be there. It cost 101 of 104
        // focus targets, all reported as occluded behind something else.
        'scroll-behavior: auto !important; }';
      document.head.appendChild(st);
    }
    let cancelled = 0;
    for (const a of document.getAnimations()) {
      try { a.cancel(); cancelled++; } catch (e) { /* detached effect */ }
    }
    return {cancelled};
  })()`);
}

function settle(evalIn) {
  return evalIn(
    `new Promise(r=>setTimeout(()=>requestAnimationFrame(()=>requestAnimationFrame(r)),${SETTLE}))`,
    true);
}
