## 28. Repository, versioning and the Makefile

### Versioned, three ways, and they are not the same number

| Version | Governs | Changes when |
|---|---|---|
| **Product** | The `rig` binary. Semver, git tag `v1.4.2` | Any release |
| **Wire** | The contract programs speak. Major only, in the path | A breaking contract change, which rig then serves *alongside* every previous version, forever |
| **Registration schema** | The shape of what a program declares | Independently, and old declarations keep parsing |
| **Semantics generation** | What the declared names *mean*: capability meanings, declaration defaults, schema dialect | Whenever any of those changes meaning. Old registrations stay pinned to the generation they declared, and a generation nobody claims is retired (§21) |

`rig version` prints all three plus the build SHA and date. A program's `rig version` mismatch
is never an error, only a line in the Programs view.

### Git

- One repository, `github.com/borismilner/rig`, git from the first commit.
- Conventional commits, `type(scope): description`, imperative, under 72 characters.
- Every milestone in §23 ends in a tagged commit with the demo recorded in the message.
- `CHANGELOG.md` generated from the commit log at release, never hand-edited.
- Release is a tag; CI builds, runs the full gate, and publishes the artefacts.
- **The tag format, fixed 2026-09-24 (integration gap 9).** A release tag versions the one module
  and is what a program pins. Canonical `vMAJOR.MINOR.PATCH`; major `v0` or `v1`, because the
  module path has no `/vN` suffix and Go cannot fetch a v2+ tag without one; pre-releases only
  `-mN` (a milestone) and `-rc.N`; no build metadata; annotated, so the message carries the
  demo. `make tag-check` (`cmd/tagcheck`, in `make ci`) fails on any `v` tag that breaks this.
  How a program pins is in `docs/programs.md`, "Pinning a version".
- The wire contract lives in `proto/` and generated code is checked in, so a clone builds with
  no code generation step and a diff shows contract changes plainly.

### The Makefile is the interface to the repository

Every important operation has a target, `make help` lists them all with descriptions parsed from
the file itself, and CI runs the same targets a human does. There is no operation documented in
a README that is not a target, because a documented command drifts and a target does not.

| Group | Targets |
|---|---|
| Build | `build` `build-rigd` `build-rig` `install` `run` `dev` `clean` |
| Test | `test` `test-unit` `test-race` `test-chaos` `test-e2e` `fuzz` `cover` |
| Quality | `lint` `fmt` `vet` `contrast` `verify` `audit` `modules` `modules-matrix` `build-minimal` |
| Generate | `generate` `proto` `schema` `types` `docs` |
| Measure | `bench` `bench-ipc` `bench-idle` `bench-scale` `bench-size` `profile` |
| Operate | `doctor` `logs` `apps` `up` `down` |
| Ship | `deps-check` `tidy` `release` `package` `ci` |

---

### ⛔ `make install` replaces a live deployment, and it does so without losing anything

**RULED BY BORIS, 2026-09-17, in his own words:**

> *"`make install` must be robust, meaning if there is already one that is
> deployed it should close it elegantly and replace; Use all precautions and
> make sure this is 100% robust and doesn't cause any losses or problems."*

⛔ **THIS IS A REQUIREMENT ON THE MVP'S SECOND AND THIRD CONDITIONS AT ONCE** -
the deployment (B49) and the robustness bar (B53, *"terminations must be
graceful"*). **An install that leaves a stale daemon running is a failed
termination in the one place nobody looks**, because nothing has crashed.

**THE DEFECT THAT PRODUCED IT, MEASURED 2026-09-17.** `make install` wrote new
binaries and ran `daemon-reload`, **and left the running daemon untouched.** So
`rig estate` dialled `v0.0.0-m0-334-g992e086` while the tree was **14 commits
ahead**, and the live daemon served **six of the nine record verbs**. ⛔ **Every
*"all nine verbs answer"* sentence written that day was true of a tree and false
of the machine, and the target's own output said `installed` without qualifying
which of the two it meant.**

#### The seven clauses, each one a thing that can go wrong

| # | Clause | Why it is not optional |
|---|---|---|
| 1 | **VERIFY THE NEW BUILD BEFORE TOUCHING THE OLD ONE** | a broken build must not be able to remove a working deployment. Nothing is replaced until the new binary exists and answers `--version` |
| 2 | **STOP GRACEFULLY, NEVER KILL** | `rigd` drains on SIGTERM through `signal.NotifyContext`, which is what closes the record store, releases the flock and removes the socket. **`systemctl stop` sends SIGTERM and is therefore the elegant path**; `SIGKILL` is what B56 is still unanswered about |
| 3 | **WAIT FOR THE STOP, BOUNDED, AND FAIL LOUDLY IF IT DOES NOT COME** | a replace that races a draining daemon is the half-written state the requirement names. A timeout is a REFUSAL to install, not a licence to proceed |
| 4 | **REPLACE ATOMICALLY - write beside, then `rename(2)`** | `install` over a running executable can return `ETXTBSY` or leave a partially written file that is neither version. A rename is atomic and leaves any still-running process holding its own inode |
| 5 | **KEEP THE PREVIOUS BINARY AND ROLL BACK IF THE NEW ONE DOES NOT COME UP** | ⛔ **the one outcome that is not allowed is a machine with no working daemon.** Verification is a live dial, not an exit code |
| 6 | **VERIFY BY DIALLING, AND COMPARE THE BUILD** | `rig estate` must report the sha that was just installed. **An `active` unit is not evidence** - that is precisely how the 14-commit skew survived |
| 7 | ⛔ **TOUCH ONLY `rigd.service`** | `rig-team.service` is a separate deployment on its own `XDG_STATE_HOME`, and peers run private daemons on their own `XDG_RUNTIME_DIR`. **Stopping "the daemon" by pattern rather than by unit name would take a peer's work with it** |

#### What it must NOT do, and each of these is a real hazard

- ⛔ **IT MUST NOT TOUCH `rigwindow`.** `make install-window` is a separate
  target and stays one. The tray is on Boris's screen; `rigwindow` is
  `Wants=rigd.service` rather than `Requires=`, so it survives the daemon stop
  by design and shows its detached text for the gap. **`pollEstate`'s detached
  branch holds the icon deliberately** - *"a stale icon is ambiguous, a stale
  VERSION is a lie"*.
- ⛔ **IT MUST NOT ENABLE ANYTHING.** Enabling changes when a session starts a
  daemon and stays the human's call. **Replacing what is already deployed is not
  the same act as deciding to deploy**, and only the first is granted here.
- **It must not start a daemon that was not running.** If nothing is deployed,
  install the files and say so; the requirement is about REPLACEMENT.

### ⛔ One target redeploys EVERYTHING, because several calls is the interface he has to type

**RULED BY BORIS, 2026-09-18, in his own words:**

> *"We should have a Make target that builds and re-deploys everything; It's
> easier that doing multiple Make calls."*

> *"Re-deployment should elegantly gracefully close the live instances and
> deploy the new ones instead of them."*

**IT IS A COMPOSITE, NOT A REPLACEMENT.** `make install` and
`make install-window` keep their contracts and their separate existence - the
seven clauses above bind unchanged, and so does clause 7's rule that only
`rigd.service` is touched by the daemon half. **What changes is the INTERFACE:
one target he types, which runs both halves in the order that leaves nothing
stale.**

⛔ **THIS NARROWS THE OLDER RULE AND DOES NOT CANCEL IT.** *"`make install` must
not touch `rigwindow`"* stands: the daemon half still touches only its own unit.
**What is no longer true is that the human has to remember the second call.**
The window was left out of the composite nowhere by design - it was left out
because nobody had asked for one target.

| Clause | Why |
|---|---|
| **EVERY LIVE INSTANCE CLOSES GRACEFULLY** - the daemon AND the window | his word is *"elegantly gracefully"*, and the window is a live instance too. A tray left running against a replaced binary is the skew defect with the surfaces swapped |
| **THE ORDER IS BUILD, VERIFY, THEN REPLACE** | clause 1 above, applied to both halves at once: **nothing is stopped until BOTH new binaries exist and answer** |
| **IT REPORTS WHAT IT REPLACED, BY VERSION, FOR EACH** | *"installed"* without a version is the sentence that let a 14-commit skew survive. Two halves means two dials |
| **A HALF THAT WAS NOT DEPLOYED IS INSTALLED AND SAID SO** | the replacement rule is about what is live; it must not refuse because one half was never running |

### ⛔ The first `rig` after a redeployment tells the human it happened

**RULED BY BORIS, 2026-09-18, in his own words:**

> *"When first run after a redeployment `rig` should show a toast informing the
> user of the update."*

**IT IS A TOAST (§12), AND ITS TRIGGER IS A DEPLOYMENT**, which is why it is
recorded here as well. **The condition is a CHANGE the human has not been told
about yet, not a version mismatch** - §21's skew warning already fires when the
client and daemon disagree, and that is a different sentence for a different
fault.

- **Once, not every run.** What makes it "first" is state that survives the
  run, so the second invocation is silent.
- **It names what changed, or at least what it moved from and to.** A toast
  saying "updated" and nothing else is the `installed` defect again.
- ⛔ **IT IS NOT AN ERROR AND MUST NOT READ AS ONE.** The human asked for it,
  so the deployment succeeding is the good news the toast carries.

### ✅ Pushing is authorised standing, by Boris, 2026-09-17. No seat needs to ask again.

> *"I permit you to push anytime you see fit."*

⛔ **IT IS A STANDING GRANT AND ITS SCOPE IS STATED IN THE WORDS THEMSELVES** -
*"anytime"*. **Recorded the turn he said it**, because the four requirements this
project lost were each lost by waiting for a handoff.

| | |
|---|---|
| what it lifts | **the requirement to ask before `git push origin main`.** Three lead generations carried the push as an open question to him; **it is closed** |
| what it does NOT lift | **the judgement about WHEN.** *"As you see fit"* is permission, not instruction. Push at a clean gate, never mid-edit |
| what it does NOT touch | ⛔ **`make install`, which is a DIFFERENT denial and is still live to a seat.** Do not collapse the two - they were denied separately and only this one is resolved |

⛔ **WHY THIS ENTRY EXISTS AT ALL, and it is the cheapest lesson here: the count
kept climbing while the question sat unanswered.** It was 115 commits ahead, then
116, then 119, then 0 after a push, **then 17 again within a day.** Each figure
was true when stated and every one of them expired silently. **A closed row is
not a closed class** - the push was closed correctly by generation 13 and the
condition recurred anyway. ⛔ **So do not treat "the push landed" as a durable
fact. Measure with `git ls-remote origin main` against HEAD, never against a
local `origin/main` ref a fetch has not touched.**
