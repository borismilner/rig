## 58. Seeing the screen

**Requirement 1, Boris, 2026-10-27, verbatim:** *"The tool shall be able
to snapshot anything and everything on the system even if it's not in the
foreground and it should be maximally flexible so that it allows AI
agents optimal control."*

It was said after the finding below: an agent on Boris's GNOME Wayland
desktop has no safe way to see a window, and was verifying surfaces
with `xwininfo` and `xprop` or by asking him.

### What the requirement fixes

- **Scope is everything.** Any window, any output, any region, any
  workspace, including windows that are minimised, covered or on another
  workspace. Not only rig's own surfaces.
- **Flexible, for agent control.** The caller chooses the target
  (window by id, title, class or owning program; output; region), the
  format and scale, and whether the pointer is drawn. The answer is data
  (section 3: everything a surface renders, an agent can obtain as data),
  so it returns a file path plus the geometry and target it resolved.
- **Safe to call.** It must never leave a stray window on his screen
  (the 2026-10-01 `gnome-screenshot` failure) and must not hang.

### Supersedes

The standing rule *"never capture his screen"* (section 55, artefact
rules) and the memory that bans `gnome-screenshot`. Boris has now
required capture, so the ban on the capability is lifted. **The ban on
the failure mode stays:** no tool that falls back to X11 and leaves a
square behind.

### Not yet decided

- Section 38: search for a Go library before writing one.

### Ruled, Boris 2026-10-27

- **It is part of rig.** Infrastructural, not a program. `snapper` may
  one day be upgraded to use it; that is not now.
- **Anybody can call it.** No caller allow-list.
- **Every capture is logged** (caller seat, target, time, result), in
  the section 15 usage log.

### Mechanism chosen, and the spike that backs it (2026-10-27)

**A rig-owned GNOME Shell extension that exports a D-Bus capture
service.** Rejected: the portal Screenshot (full screen, prompts), the
portal ScreenCast (a picker, and the dialog only for the focused app) and
`org.gnome.Shell.Screenshot` (refuses non-GNOME callers on 50.1).

**Spike result**, GNOME Shell 50.1, run in an isolated headless Shell
(`tools/shellcapture-spike/`), three overlapping GTK windows of known
colour. `window actor.paint_to_content(null)` then
`Shell.Screenshot.composite_to_stream` (asynchronous: close the stream
only after the promise resolves) produced PNGs whose centre pixel matched
the window colour for:

| Window state | Captured correctly |
|---|---|
| unfocused | yes |
| covered by two other windows | yes |
| on another workspace | yes |
| minimized | yes |

- The PNG is the actor's whole paint, shadow included (428x329 for a
  400x300 frame); the corner is transparent. The frame rect is known, so
  a crop is possible.
- **Not yet tested:** the live session (the extension must be installed
  and the Shell restarted by a log-out, since Wayland cannot reload it);
  fractional scaling and multiple monitors; a window in a fullscreen app;
  Xwayland windows; the cursor; video or screencast.
- The extension is tied to the Shell major version (`shell-version`).
- Section 38 search: no Go library does this, since the work is inside
  the compositor. Reused: `godbus/dbus` on the daemon side.
