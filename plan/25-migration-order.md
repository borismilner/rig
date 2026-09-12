## 25. Migration order

**The unit of progress is a service, not a program** (§5k). "shelf takes config and
notifications" is a valid milestone; a program is never blocked waiting to adopt everything, and
its declaration says `coverage: partial` until it does. The order below is the order programs
*start*, not the order they finish.

Each program keeps working standalone throughout, and gives up its own tray icon when its tray
adoption lands. **State** is what was on disk on 2026-09-10, because rig's value is a function
of how many of these are worth reaching.

**One row already moved, and the direction it moved is the warning.** `graft` was written in as
adopter 6 on the morning of 2026-09-10 and was out of the order by that afternoon, because it
is now waiting on rig rather than adopting it. Nothing here is load-bearing until it is built
and used; the advocate's item 3 asks for this table to be re-checked before M12, and this is
what re-checking looks like.

| Order | Program | State on 2026-09-10 | Why here |
|---|---|---|---|
| 1 | `shelf` | 45 commits, built, used daily | The pilot. Designs the contract against something real |
| 2 | `dispatch` | 45 commits, built | Already Wails v3; proves an existing app becoming a pane |
| 3 | `nudge` | 11 commits, built | Tiny and tray-only. Designs the generated UI, and is the honest test of the hand-written budget in §3 - the *declaration* is generated, and one rich archi command alone measured 160 lines of JSON |
| 4 | `sigs` | 113 commits | The second generated-UI program, so it is designed against two |
| 5 | `snapper` | 197 commits, built | Native capture stays its own window; history and settings come in |
| - | `graft` | 2 commits, **deferred 2026-09-10** | **Not an adopter, and not counted as one.** Its own window/tray milestone was struck the day this row was written, and Boris then deferred graft entirely until rig v1 and the client stub exist. It is a *consumer* of rig's schedule, not a contributor to it: counting it here would let rig's payoff borrow a program that is waiting on rig |
| 7 | `archi` | 237 commits, two built binaries (13M, 19M) | The richest frontend under the theme bridge |
| 8 | `grabbit` | 100 commits, stalled since 2026-08-11 | A stalled project is the best test of whether rig makes finishing cheaper |
| 9 | `devtool` | 65 commits | The endpoint of the argument: unbundle it, each utility its own program |
| 10 | `romsort`, `dedup` | 7 and 2 commits | Deferred until they are real programs |

---
