## 26. Open questions

1. **Does a transparent, frameless, always-on-top, focus-refusing webview work under GNOME Shell
   on X11 from Wails v3 beta?** Load-bearing for §12 and unverified. Compositor and ARGB visual
   confirmed present on this laptop; what is not confirmed is whether Wails reaches the
   override-redirect and input-shape hints. Decided at **M9** by building one first. Fallback:
   the toast layer becomes its own small GTK4 or Gio process rig drives.
2. **Does the Wails v3 beta tray behave on X11 here?** Decided at **M8**. `fyne.io/systray` is
   the fallback and six programs already use it.
3. **Generated forms: `@sjsf/form` or hand-rolled?** Decided at M8. The table, action and
   progress surfaces are ours either way.
4. **How hard is network capability enforcement worth making?** Namespaces are real enforcement
   and real complexity. M12 decides; until then it is declared and honestly labelled.
5. **Does `snapper`'s capture window belong in rig's window at all?** Probably not, and the
   plan assumes not. Note that this is now *answered by the declaration* rather than by naming
   a program: `capture-region` declares `needs_display` and `interactive`, and the pane surface
   states what it can carry (§5e).
6. **Which programs, if any, are worth hosting inside `rigd` rather than as binaries?** (§5j)
   Nothing is hosted until one is actually cheaper that way. Decided per program, and the four
   costs are stated so the decision is not made by accident.
7. **What is the first wire major's support window?** §21 requires a date on the day v1 ships.
   It does not exist yet because v1 has not shipped.
8. **Can §18's supervisor be expressed as a declared state machine?** (§5h) The whole case for
   a state machine service rests on this, and it is answerable from the design rather than from
   code. If yes, the service is real and gets a milestone at the §24 gate; if no, it is struck
   and the three machines rig hard-codes stay hard-coded. Nothing else adopts it today, so
   there is no second candidate to fall back on.
9. **Which real program adopts the element kit, and what does it cost that program?**
   (§5h, §11) **Asked and answered once already, and the answer was none.** Measured
   2026-09-10: `archi` has zero tables, `dispatch` has one call site and its own table has no
   behaviour to lose, `snapper` serves no HTML at all, and replacing working UI with rig's is
   effort with no feature at the end of it - the same reason §25's stalled programs are
   stalled. The kit's adopters are now M1a's fake applications, which cannot answer this
   question because they were written to use it. **The question stays open until a real
   program adopts or M8 arrives, and if M8 arrives first the kit is struck** (§24).
10. **Do the three `← open` services survive the §24 gate?** A job queue, a cache and state
   machines are drawn in §5h and owned by no milestone. §5h says nothing ships without a
   program that adopts it, so the question is really "which program", and for all three there
   is no answer yet. **The file-watcher was the fourth and is now answered**: it needed no
   program adopter, because it is a source on the `events` service rather than a service a
   program calls, and it lands at M13 with the bus (§5h).

11. ~~**Does `effects` need a fifth value for driving input?**~~ **Decided yes and shipped**,
   while the wire was still unfrozen, because §21 makes a new value a hard refusal at an older
   daemon's boundary and the cheap moment is before the first major. `EFFECTS_DRIVES_INPUT = 5`
   sits above `destructive`, so a rule can say "may delete its own files, may not type into my
   windows". **It also found a defect it would otherwise have caused**: the invoker floored an
   unresolvable ref at the `destructive` literal, which stopped being the top of the order, so an
   opaque call would have slipped under a rule denying the new level. There is now an
   `EffectsCeiling` constant and a test that fails when a named value appears above it.
12. **Can a redaction pointer address every element of an array?** (§15, §5m) Every
   declared-sensitive field in this plan is at a fixed path, and `/steps/*/text` is not: the
   number of steps is known per call. Whether the compiled-span construction expresses a wildcard
   at 82.5 ns, or at all, decides whether the hand can log a redacted script or must fall back to
   declaring the whole `steps` array sensitive and recording only its length and each step's op.
   The fallback is what AgentBox does today, so nothing is lost by taking it, but the answer
   changes §15 rather than §5m and should be measured there.

---
