## 33. What the blind-spot sweep changed, 2026-09-10

An eleventh seat ran after the fixes and after the advocate, pointed at what the other ten did
not look at rather than at more of what they did. It returned **19 findings**; the record is
`logbook/projects/rig/agent-work/attack-2026-09-10-s9-blindspot-sweep/FINDINGS.md`.

**Its verdict on the method is the part that generalises.** Ten seats drove ten surfaces to
local optima, so the residue is entirely in the joints - and the joints got *worse*, because
each fix moved cost or risk across a boundary its seat did not own. The footprint fix exported
the GUI to processes nobody budgeted; the isolation fix put its key somewhere every program
could read it. The class no single-surface seat can produce at all is the class with no
surface: backup, disk, format versioning, uninstall, accessibility, unattended operation.

### Applied

| # | Was | Now |
|---|---|---|
| B1 | `introspect` in a uid-readable file, so any program could read every other program's history | Nothing is delivered. It is **decided at connect time** from whether the connection registered as a program (§14). Went through an environment-variable draft first, which round 2 killed |
| B3 | `operate` reached only whatever launched `rigd` - systemd - so no human surface could act | Not a credential at all: `confirm` per call for anyone present, a named `house rules` entry for anything unattended (§14). The unattended *token* survived the first fix and round 2's D9 killed that too |
| B4 | The `house rules` caller vocabulary named only connections | `schedule`, `bus` and `url` added, each minting a principal; `url` defaults to read-only plus confirm (§13a) |
| B5 | §5a put `secrets` in the daemon; §22 linked go-keyring only into the CLI | `rigd` owns the keyring. §17's 8.89 MiB rung is now a design target until `bench-size` splits it (§17, §22) |
| B6 | "Exactly one rig per user" - §16's load-bearing premise - with zero enforcement | `flock` on a pidfile before bind, at M0, with a chaos test (§5f) |
| B10 | §7's rule binds rig's own storage; nothing backed rig up | `rig backup` / `rig restore`, four rows of state named, at M11 (§7) |
| B18 | The client stub "does exactly four things" over a list of five, against a §3 gate | Corrected (§5d) |
| B2 | §17 measured `rigd` and called it rig, while three sections imply an unnamed resident GUI process | The gap and the three-way decision are stated, and M8 owns it (§17, §23) |

### Round 2, over the introspection change itself

The sweep ran twice. The second pass was pointed at the full-introspection rewrite (§32) an
hour after it landed, and returned 15 findings against text written the same day. All are
applied:

| # | Was | Now |
|---|---|---|
| D1, D2, D3 | `introspect` delivered in `RIG_INTROSPECT` from Boris's shell - underspecified, inherited by every descendant of a shell, and stale after the first daemon restart with no way to rewrite it | **Nothing is delivered.** Decided at connect time from whether the connection registered as a program (§14) |
| D4 | Conformance item 22 said "items 1-21", so the environment test excluded hosted programs - the one class running inside `rigd` | Items reordered: 22 is the environment allowlist, 23 the connection predicate, and **24 is the hosted program passing 1-23** (§19) |
| D5 | Item 24 asserted a property of *rig* from inside the *program-side* suite, over peers a single-program run does not have | Moved to §20's Introspection row, beside §14's three-run battery |
| D6 | Run 2's "a missing row is a failure" collided head-on with §15's four declared lossy paths | The assertion is on the **coverage entry**, not the row: a gap the coverage log declares is a pass (§14) |
| D7 | §9 promised a field that says it "exists and was never recorded"; §15 blanks bytes, and a blank cannot say that | The answer carries the declaration's `sensitive` **pointer list** from the registry. No wire change, and 82.5 ns still holds (§9) |
| D8 | §16 forbids holding a lock across a question to a human, which forbade the motivating elevation case | Stated exception: gating the lease-holder's **own** call freezes its lease expiry for the duration (§16) |
| D9 | The unattended `operate` path promised a file to `make deploy`, which descends from Boris's shell and cannot read it - B3 recreated one row lower | `operate` stops being a credential. A named `house rules` entry authorises unattended callers (§14) |
| D10 | "rig builds the child's environment" named no contents, silently breaking the keyring, the D-Bus fallback and every `needs_display` command | The allowlist is enumerated, is config, and conformance item 22 asserts both halves (§18) |
| D11 | §15 and §31 still described the credential as "a token minted into a 0600 file" - the design §14 exists to reject | Both corrected (§15, §31) |
| D12 | Every elevation is audited "with the rule that fired", and no rule fires on an elevation | `origin` is `rule` or `elevation` (§13a) |
| D13 | The Do Not Disturb exception lived in §14, which needs it, not §12, which implements it | Stated in §12 too, as a conformance assertion (§12) |
| D14 | Nothing bound an elevation prompt to the call that caused it, so a background agent's request could be answered as if it came from the terminal in view | The prompt carries principal, client kind, pid, command and arguments, and routes to the requesting caller's surface first (§14) |
| D15 | M1 shipped the grants; M5 ships the redaction that §14 calls their precondition | M1's row states the dependency: complete history reading is gated on M5 (§23) |

**Round 1's B1 and B3 are withdrawn as fixed** in the same file, and the ten other round-1
findings stand unchanged in the bank below.

### Banked for the M3 gate, deliberately

Twelve findings are real and none of them changes M0 or M1: no version on any on-disk format
while the wire is versioned forever; two filesystem registry mirrors §5f's fix left open; no
disk budget and ENOSPC missing from the coverage log's causes - **partly answered since, because `disk.pressure` at M5 gives rig free space and stall pressure per watched path (§5h); the budget policy itself is still owed**; no policy for `ask` at 3am
beyond the one §14 now sets for `operate`; accessibility absent except contrast and reduced
motion; two budgets whose own cited measurements fail them; no uninstall and no update
rollback; the keyring's availability off a desktop session; a "phone" surface promised four
times and specified nowhere; no scheduler library and no DST rule; no default health interval,
which silently sets three budgets in three sections; and the id namespace an earlier seat
accepted and nobody wrote.

**They are banked rather than dropped**, and §24's M3 gate is where they are
answered - because the honest reading of a twelve-item list on an unbuilt plan is that it is
evidence about the plan's size, not a queue of chores.

### What the sweep did not attack

The measurements themselves - quoted numbers were reconciled against each other, not re-run.
Localisation. `design/theme.js`, so §6's claim that its schema is exactly what the engine
implements is unverified by anything but the engine. Multi-seat in the strong sense. And the
four things §31 already lists.


---
