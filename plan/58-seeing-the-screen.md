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

### Not yet decided (open, to be ruled in a decision file)

- Mechanism on GNOME Wayland: the `xdg-desktop-portal` Screenshot and
  ScreenCast interfaces, GNOME Shell's own interface, or a screencast
  stream. **Unverified**: whether each prompts, whether each can reach
  an unfocused or covered window, and whether any needs a Shell
  extension. A spike must answer this before a design is written.
- Section 38: search for a Go library before writing one.

### Ruled, Boris 2026-10-27

- **It is part of rig.** Infrastructural, not a program. `snapper` may
  one day be upgraded to use it; that is not now.
- **Anybody can call it.** No caller allow-list.
- **Every capture is logged** (caller seat, target, time, result), in
  the section 15 usage log.
