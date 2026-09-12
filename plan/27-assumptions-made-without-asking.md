## 27. Assumptions made without asking

Flip any of these with a sentence and the plan changes accordingly.

| # | Assumed | The alternative, and what it costs |
|---|---|---|
| 1 | ~~A program with no rig degrades but still runs~~ **Resolved 2026-09-10, and the middle path won.** The stub reimplements nothing; it tolerates absence, reads a snapshot rig wrote, and a program whose declared preconditions are unmet says `unavailable` with a reason instead of running headless for eight hours (§5g) | The two rejected ends: a 300-line un-upgradable copy of the platform inside every binary, or refusing to start and making rig a hard dependency |
| 2 | **rig manages storage, the program opens it** (§7) | Everything over RPC: one audit point and a swappable engine, but ~6µs per query and a contract that must keep up with SQL. Or storage stays entirely with the program, which leaves migrations copy-pasted fifteen times |
| 3 | **rig owns config, storage, secrets, GUI, observability, lifecycle, scheduling and updates** | You named config, storage and GUI. The rest is my proposal; say which to drop |
| 4 | **Programs stay separate binaries by default**, and hosting one inside `rigd` is a deliberate per-program choice (§5j). Partly flipped 2026-09-10 at Boris's instruction | Hosting everything makes rig a monolith and forfeits the defining maximal; hosting nothing loses the case where an extra process is the larger cost |
| 5 | ~~`turret` survives as the name of the UI component~~ **Resolved 2026-09-10: it does not.** The product is rig and the window is the window | Keeping a second name for a component only its owner opens, at the cost of every reader having to learn it |

---
