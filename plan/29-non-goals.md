## 29. Non-goals

- rig does not run business logic. Ever. A program id appearing in rig's code is a bug.
- rig is not required. Every program works without it, at reduced service.
- rig does not host third-party or untrusted programs. The trust model is real; the threat model
  is a mistake in our own code, not an adversary.
- rig does not replace any program's own CLI. `shelf` still works as `shelf`.
- rig does not own data. Programs own their data; rig owns the plumbing around it.
- **rig does not hot-upgrade itself.** Measured at 1.9 ms of marginal benefit per upgrade
  against four defect classes (§18). Notice, restart, resume.
- **rig does not host anything we did not write.** A hosted program (§5j) is compiled into
  `rigd` at build time, because Go has no working dynamic loading and because an in-process
  plugin has no capability boundary. Third-party code is a separate process or it is not run.
- rig does not decide what a program's declaration *means*. It records what was declared, pins
  the generation it was declared under, and refuses what is missing.
- **rig does not carry AgentBox's whole surface across the M16 cutover.** Four surfaces are
  OUT, ruled 2026-09-11, so the supersession in §16 is a deliberate narrowing rather than an
  accident discovered at cutover. The reasoning is in
  `logbook/projects/rig/agentbox-parity-2026-09-11.md`; the rulings are:
  - **Walkthroughs** - a durable step-by-step code review with a persistent board, TL;DRs,
    domains, prose-to-code binds and a glossary. That is a product built on a platform, not
    platform, and it is the largest single surface dropped.
  - **Assignments** - summoning an agent session on a schedule with the whole toolbox. rig
    projects `cron` from a declaration (§5), which covers running a *command* on a schedule.
    **Starting an agent session a human can open and take over is not rig's job.**
  - **Artifacts** - running interactive HTML so a human can answer with a number, a shape or
    a selection. §11's window and §12's toasts are rig's answer to "ask a human something".
  - **`request_review`** - a blocking diff review returning approved plus a comment. The
    small sibling of walkthroughs, and OUT for the same reason.

---
