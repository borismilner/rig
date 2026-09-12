## 12. Toasts

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

---
