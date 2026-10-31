// Run from the repository root: node tools/countdown-ui-test.mjs.
// Exercises the real toast page against a controlled renderer endpoint.
// The daemon's wire tests separately prove which states permit execution.
import {createServer} from 'node:http';
import {readFileSync, mkdirSync, writeFileSync} from 'node:fs';
import assert from 'node:assert/strict';
import {withChrome, auditPage} from './contrast/headless.mjs';
import {paintPass} from './contrast/pass-paint.mjs';

let supplied = false, receipts = 0, replies = 0, failStop = true;
let severity = 'warning';
const server = createServer(async (req, res) => {
  for await (const unused of req) { /* drain the small test request */ }
  if (req.url === '/') {
    res.setHeader('Content-Type', 'text/html');
    res.end(readFileSync('cmd/rigwindow/toast.html'));
    return;
  }
  if (req.url.startsWith('/toast/poll')) {
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify({edge: 'top', max_h: 700, toasts: supplied ? [] : [{
      record_id: 'one', title: 'Handing off the current work',
      body: 'The successor will continue from the saved checkpoint.',
      severity, sender: 'worker', replies: ['Stop'], countdown_seconds: 15,
    }]}));
    supplied = true;
    return;
  }
  if (req.url === '/toast/presented') {
    receipts++;
    res.end(JSON.stringify({state: 'open', expires_ms: Date.now() + 15000}));
    return;
  }
  if (req.url === '/toast/reply') {
    replies++;
    res.statusCode = failStop ? 409 : 204;
    res.end(failStop ? 'Temporary failure; Stop was not recorded' : '');
    return;
  }
  res.statusCode = 404;
  res.end();
});
await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
try {
  await withChrome(async browser => {
    for (const level of ['warning', 'urgent']) {
      severity = level;
      supplied = false;
      receipts = 0;
      replies = 0;
      failStop = true;
      const p = await auditPage(browser, `http://127.0.0.1:${server.address().port}/`, {width: 420, height: 700});
      try {
        // The urgent pulse never ends. It must not hold the paint receipt.
        assert.equal(receipts, 1, `${level}: one presentation receipt`);
        const label = await p.eval('document.querySelector(".left").textContent');
        assert.match(label, /Proceeds in .* unless stopped/);
        const bounds = await p.eval(`(() => {
          const c = document.querySelector('.toast').getBoundingClientRect();
          const l = document.querySelector('.left').getBoundingClientRect();
          return {card: c.right, label: l.right};
        })()`);
        assert.ok(bounds.label <= bounds.card, 'countdown label fits card');
        await p.eval('document.querySelector(".left").setAttribute("data-contrast-mark", "")');
        // The card uses an opaque gradient: inspect its painted pixels,
        // rather than a DOM audit that treats the gradient as transparent.
        const audit = await paintPass(p);
        const countdown = audit.measured.find(row => row.label === 'div.left.countdown');
        assert.ok(countdown, 'countdown was measured');
        assert.ok(countdown.ratio >= 4.5, `countdown contrast ${countdown.ratio}`);
        if (level === 'warning') {
          mkdirSync('build', {recursive: true});
          writeFileSync('build/countdown-ui.png', await p.shotPng());
        }
        await p.eval('document.querySelector("button.close").click()');
        await p.settle();
        assert.equal(await p.eval('document.querySelector(".slot").classList.contains("leaving")'), false, 'failed Stop remains visible');
        assert.match(await p.eval('document.querySelector(".fail").textContent'), /Temporary failure/);
        failStop = false;
        await p.eval('document.querySelector("button.reply").click()');
        await p.paint();
        assert.equal(await p.eval('document.querySelector(".said").textContent'), 'Stopped');
        assert.equal(replies, 2);
        // Both wire outcomes have separate presentation, even if the
        // retraction reaches the page before its answer watcher does.
        for (const [answer, expected] of [
          [{expired: true}, 'Countdown ended'],
          [{withdrawn: true, by: 'worker'}, 'Withdrawn by worker'],
          [{dismissed: true}, 'Stopped'],
        ]) {
          await p.eval(`(() => {
            const slot = document.querySelector('.slot');
            slot._answered = false;
            answered(slot, ${JSON.stringify(answer)});
          })()`);
          assert.equal(await p.eval('document.querySelector(".said").textContent'), expected);
        }
        console.log(`${level}: receipt, Stop retry, dismissal, expiry, withdrawal; countdown contrast ${countdown.ratio}:1`);
      } finally {
        await p.close();
      }
    }
  });
} finally {
  server.close();
}
