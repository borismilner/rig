## 12. Toasts

⛔ **TOASTS COME FIRST, RULED BY BORIS 2026-09-24, verbatim** (his elision, marked `[...]`,
is kept as the lead relayed it):

> *"I want the `rig` to be as useful as possible from the start. I want it to be able to show
> different kinds of toasts (different severities, like AgentBox). The toasts must be beautiful
> and shown like speech bubbles coming out of the `rig` system-tray icon if possible. The best
> of the best library/libraries are to be used for that. [...] I also want its libraries to be
> fully verified to be the best of the absolute best and make sure they don't leak
> performance."*

**This moves the toast surface ahead of M9**, where §23 placed it: his word overrides the
milestone order. The severities are AgentBox's five - `info`, `success`, `warning`, `error`,
`urgent`. The shape is a speech bubble whose tail points at rig's tray icon **where the icon's
screen position can be measured**, and at the panel corner the tray sits in where it cannot;
a tail is never drawn at the icon on a guess. The rules below are unchanged and apply to it.


A notification is a designed surface, not a line handed to the desktop. Frameless, always-on-top,
transparent webview windows rig owns and styles. X11 with GNOME Shell as compositor is confirmed
present on this laptop, so an ARGB visual and a real backdrop blur are available.

- Full CSS and the theme tokens, spring physics, live-updating bodies, syntax-highlighted code,
  sparklines, images, markdown.
- **Inline actions wired to the program's own commands.** Answering does not open a window.
- Stacking with collapse; hover fans the deck out.
- Never steals focus, never eats a click meant for what is underneath.
- Dwell scales with reading time: roughly 10s plus 5s per line. Hover pauses.
- **Nothing is ever only a toast.** Everything lands in a notification centre, searchable, with
  the action still live. That is what makes auto-dismiss safe.
- Do Not Disturb and a fullscreen-focus rule. Suppressed toasts go to the centre, never dropped.
- **Do Not Disturb never suppresses an `ask` that gates a command** (§13a, §14). It suppresses
  notices; a question the caller is blocked on is not one. Stated here as well as in §14
  because whoever builds this surface reads this section, and a conformance item asserts it
  rather than leaving it as prose.
- Falls back to `org.freedesktop.Notifications` where the frameless window is unavailable, and
  `rig doctor` says which is live.
- **The notification centre is a service, not part of this surface** (§5h). It is the record of
  record, so `make build-minimal` must not be able to drop it, and the window, the TUI and the
  CLI all read the same one.

### Speech: the same notification, said out loud

> "AgentBox has the `say` functionality that uses kokoro-say. I want to take it to the optimum
> and make it a feature in rig." - Boris, 2026-09-10

**Speech is a renderer on the notification centre, not a new surface.** The `voice` box in
§5h's diagram is an *input* surface - reaching the estate by talking to it - and this is the
opposite direction. Putting speech on the centre is what makes it free: every program that
already sends a notification can be heard without being rebuilt, which is §1's compounding
claim again, and a program that wants to be heard rather than seen sets one field.

**An item speaks only if it carries a `speak` line.** Never the title, never the body, never a
heuristic that shortens one into the other. That rule is AgentBox's and it is why its speech is
usable rather than exhausting: what gets said is something a program wrote *in order to be
said*, and keeping "worth saying" the program's judgement is the whole reason it works.

**The engine contract is a long-lived process, and that is the design rather than an
optimisation.** One line of UTF-8 per utterance on stdin, raw little-endian 16-bit mono PCM on
stdout, and the process stays open between lines. Measured with kokoro-82M on this machine:
**loading the model costs ~3s and an utterance ~375ms**, so a process per sentence puts every
notification seconds behind the thing it is about. rig holds the engine open and pipes it into
a player.

| Piece | Rule |
|---|---|
| Engine | Resolved from config, `kokoro-say` by default. Anything satisfying the stdin/PCM contract works, which is how piper stays a valid answer |
| Player | `pw-play`, `paplay`, `aplay`, `play`, in that order, resolved at startup |
| cgo | **None**, and the daemon holds no audio device open (§22) |
| Residency | A voice model is ~100 MB resident. The pipeline is released after a quiet spell and rebuilt on demand, and **M9 adds its row to §17's table** rather than leaving it off the estate's all-day cost |
| Queue | Bounded, oldest dropped past the bound. Twenty notifications at once must not become a two-minute monologue still reading out the first |
| Do Not Disturb | Suppresses speech exactly as it suppresses a toast - **and never suppresses an `ask` that gates a command** (§14). Same conformance assertion, one line lower |
| Rate | 24 kHz for kokoro against piper's 22.05 kHz. The engine reports its own rate; no caller hard-codes one |
| Missing engine or player | A named degradation in `rig doctor` (§8), never an error. rig without a voice still toasts |

**Where "the optimum" is more than a port.** Each of these is something the centre gives it
that a standalone `say` binary cannot:

- **One queue for the estate.** Fifteen programs speaking through one arbiter cannot talk over
  each other. Today each caller races every other caller for the same sound device.
- **Severity and Do Not Disturb are already decided** for the toast. Speech inherits that
  decision instead of growing a second, differently-wrong copy of it.
- **Everything spoken is in the centre**, searchable, with its action still live. §12's
  "nothing is ever only a toast" applies unchanged, and it is what makes a missed utterance
  recoverable - which speech needs more than toasts do, because there is no scrollback for
  sound.
- **It is auditable**: what was said, on whose behalf, through which surface, in the one log.
- **Voice, speed and language are declared config keys** (§6), so they are generated into the
  settings UI and pushed live like everything else, rather than being three environment
  variables a person has to know about.

**It lands at M9, with the centre it renders, and it cannot land earlier** - there is nothing
to be a renderer *of* until M9. That also satisfies the sequencing that was asked for: M6 has
rig starting with the session (§5l), M9 has it speaking, and neither is a special case built to
make the ordering work.

**The AgentBox cutover is M16 and is not part of M9.** Repointing every agent's instructions
from AgentBox's `speak` to rig's is a change to a dozen instruction files and an estate-wide
behaviour change, so it belongs with the rest of the AgentBox supersession - after rig's speech
has survived M12's pilot week of daily use. Shipping the renderer and repointing the estate are
two decisions, and collapsing them is how the second one gets made by accident.

## ⛔ A toast on the first run after a redeployment. Boris, 2026-09-18.

> *"When first run after a redeployment `rig` should show a toast informing the
> user of the update."*

**THE REQUIREMENT AND ITS CLAUSES ARE IN §28**, with the deployment target that
triggers it, because the trigger is what makes it hard to get right: once and
not every run, naming what changed, and NOT wearing the shape of an error.
**This heading exists so a reader of the toast surface finds it.**

## Toasts, built 2026-09-24, and the three rulings they were built to

The lead ruled three open questions on 2026-09-24, relaying the owner:

1. **Footprint.** The renderer is an on-demand child process, the window's
   pattern. The tray holds one long poll on `rig.toast.wait` and nothing
   else. The first toast starts `rigwindow --toasts`, which exits when its
   last bubble leaves, so the tray's idle footprint does not change. Measured
   under Xvfb with software GL: the renderer and its WebKit processes hold
   about 250 to 440 MB RSS while a toast is up, and all of it returns when the
   renderer exits (one success toast: gone after 18s). When the renderer
   cannot start, or dies at start, the tray sends the toast to
   `org.freedesktop.Notifications`.
2. **"Lands in a record".** `rig.notify` makes the daemon write the
   notification record itself, kind `notification`, project `notifications`,
   with the sender (a registered program's id, else the caller's seat) as
   provenance. Programs still get no record-write permission.
3. **Position.** Linux gives no tray icon position: Wails' `bounds()` returns
   an empty rectangle there and fyne drops the click coordinates. So **the
   bubbles anchor at the corner of the monitor work area where the tray sits,
   top-right on GNOME, and the first bubble's tail points into that corner,
   not at the icon.** This is the "if possible" in his words, answered
   plainly: it is not possible to aim at the icon itself today.

What each severity looks like, in the theme's own semantic hues:

| Severity | Drawn as | Leaves |
|---|---|---|
| `info` | dark panel, steel-blue edge and an `i` disc | after 10s plus 5s per line |
| `success` | dark panel, sage-green edge and a check disc | the same |
| `warning` | dark panel, amber edge and a `!` disc | the same |
| `error` | dark panel, rust edge, rust title and a cross disc | after twice as long |
| `urgent` | filled rust bubble, dark text, a pulsing ring | only when clicked |

Hover pauses every countdown; a click dismisses a bubble. The newest bubble
is nearest the corner. `rig notify <severity> <title> [--body B]` sends one
from a shell.

**Do Not Disturb, built 2026-09-24.** `rig.toast.dnd` turns it on or off
(`rig dnd on|off|status`, and a checkbox on the tray menu that shows the
daemon's state). While it is on, a notification is filed in the record with a
`suppressed` field and is not drawn; **an urgent one is always drawn**. It
lives in the daemon's memory, so a restart turns it off rather than bringing
a daemon back silent. The fullscreen-focus rule is not built.

**Not built yet**, and each is a line of the list above: inline actions wired
to a program's commands, deck collapse and fan-out, markdown and code in a
body, the notification centre as a surface (the record holds
every notification; nothing lists them yet), and speech. The backdrop is
transparent only under a compositor; Xvfb has none, so the demo screenshots
show the window's own ground around the bubbles.

---
