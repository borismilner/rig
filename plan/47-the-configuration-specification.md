## 47. The configuration specification

⛔ **DRAFT. STEP 1 AND A STEP-4 DRAFT OF §45's LOOP FOR B105 (S4), WRITTEN
2026-09-23 EVENING WITHOUT HIM, ON HIS WORD TO DO AS MUCH AS POSSIBLE ALONE.**
§45's 2026-09-23 block binds: *preparing is not approving*. **He has not
approved, reshaped or killed S4, and no subagent is spawned from this section
until he does.** Every row below is the seat's unless it cites a ruling, and a
row he reshapes is edited here, not argued in a handoff.

> **In one line: `rigd` resolves every setting from §6's layers, records which
> layer won and which lost for every key, validates a change as a whole, writes
> the merged result to disk with its provenance, and answers `rig config origin
> <key>`.** rig's own settings are the first consumer, and one of them is read
> over the wire, which is the shape every program takes afterwards.

---

### Step 1. What it is, what a program does with it, what it costs

**What it is.** One resolver, in the daemon, over eight layers (§6). Every key
is declared in a JSON Schema before it can be set anywhere. A file, an
environment variable, a flag and a runtime override are four spellings of the
same key, and rig knows which one is in effect and why.

**What a program does with it.** Declares a schema, reads its resolved values
over the control plane at start (§4: *"a config read at startup ... at 6µs,
thousands of times over, is noise"*), and reads the on-disk snapshot when rig
is unreachable (§5g). It never parses a file, never reads `os.Getenv`, and
never carries a second resolver.

**What it costs.**

| Cost | Size |
|---|---|
| a dependency | `knadh/koanf/v2` core plus three providers and one parser, named in §22 since the first draft as INTENDED. **Bytes on `rigd` measured before the row flips to ADOPTED**, §22's own rule |
| a service | `internal/config/`, one package: layers, resolve, provenance, validate, snapshot |
| the wire | two verbs, `rig.config.get` and `rig.config.set`, six messages, additive under §21 |
| the CLI | one verb with five sub-verbs, `rig config get | origin | set | export | diff` |
| what it deletes | the `--log-level` flag block in `cmd/rigd/main.go` and the raw `os.Getenv("RIG_DISPLAY_NAME")` in `cmd/rig/record.go` |

**The honest verdict.** `service-review-2026-09-19.md` rates it *strong, best
value per line of code in the specification*, and this seat agrees with one
correction below: **the first consumer §44 named is smaller than it sounds, and
the code offers a better one.**

---

### ⛔ THE FIRST CONSUMER, READ FROM THE CODE RATHER THAN FROM §44's SENTENCE

§44 S4 says *"rig's own three flags are the first consumer"*. **Measured
2026-09-23 at `5a158a6`: of the three, one is a setting.**

| Today | What it actually is | In this build |
|---|---|---|
| `--log-level` (`cmd/rigd/main.go:89`) | **a setting.** Layered, and live-able in-process: `slog` reads a `LevelVar` on every line | **`log.level`, the first key.** The daemon is its own consumer, in-process |
| `--estate` (`main.go:91`) | **identity, not a setting.** Claimed under a lock at start (§37 precondition 5); `packaging/rigd.service` passes it | ⛔ **NOT a config key. Stays a flag.** `~/.config/rig/rig.toml` is one file for every estate on the machine, so a name in it would name production and development alike, which is the collision §37 exists to prevent |
| `--version` (`main.go:90`) | **an action:** print and exit | stays a flag. An action is not configuration |
| `os.Getenv("RIG_DISPLAY_NAME")` (`cmd/rig/record.go:1653`) | **a setting, read raw from the environment by the CLI**, with a fallback the comment says is *"the half that is tested hardest"* | **`display.name`, the second key, read over the wire.** The CLI becomes the first WIRE consumer, which is the exact shape every program takes |
| `NO_COLOR`, `TERM` (`cmd/rig/brief.go:768`) | terminal conventions every program honours | untouched. Not rig's keys |

**So the first consumer is two keys in two processes, not three flags in one.**
That is a stronger proof than §44 asked for: `log.level` proves layering and
live apply inside the daemon, and `display.name` proves a client reading its
settings over the wire instead of from `os.Getenv`. **§44's acceptance form
holds on both**: the hand-rolled read is DELETED and `git grep` proves it.

**Next consumers, named so nobody proposes them as new:** `kernel.DefaultRemember`
(*"a number somebody picked"*, `internal/kernel/registry.go`),
`cmd/rig/call.go`'s `defaultCallTimeout`, and §13a's house rules table, which
`Kernel.SetRules` already receives whole because *"config is resolved from four
layers and pushed live"*. **None is in this build.** Each is one schema row and
one read when its turn comes.

---

### ⛔ S4 DOES NOT WAIT ON S1, AND §44's ORDER TABLE SAYS IT DOES

§44 orders S1 first because *"config stores layers"*. **In §6's own design the
layers are FILES** - `/etc/rig/rig.toml`, `~/.config/rig/rig.toml`, the
environment, the flags - **and the snapshot is a file by requirement**, since
§5g's stub must read it when the daemon is down. **Nothing in this section
opens a database.** The order in §44 stands as his ordering of attention; as a
build dependency S4 has none on S1, and it can be spawned the day he approves
it, while S1's measurement (B108) and design are still in progress.

---

### The design, decided

⛔ **EVERY ROW BELOW IS THE SEAT'S CALL UNLESS IT CITES A RULING.**

| # | Decision | Why | What it rules out |
|---|---|---|---|
| **1** | **§6's eight layers, verbatim, and the resolver is written for eight from the first commit.** In this build six are populated: built-in defaults, `/etc/rig/rig.toml`, `~/.config/rig/rig.toml`, environment `RIG_*`, flags, runtime override. **The two program layers** (a program's declared defaults, `~/.config/rig/apps/<id>.toml`) **are wired but empty** until the first program adopts the service (§5k) | §6 is the requirement; the layer list is data, not eight code paths. rig's own keys are top-level in `rig.toml`; a program's live under `apps/<id>.toml` | a six-layer resolver that grows two layers later, with a second precedence bug |
| **2** | **Every key is schema-declared, and rig's own schema is one JSON Schema embedded in `internal/config`.** Validation uses **`santhosh-tekuri/jsonschema/v6`, already ADOPTED** (`go.mod`) | §6: *"Every key is schema-declared, so the settings UI is generated."* The validator is in the tree; proposing another is the named failure mode | a second validator; a key that exists only in code |
| **3** | **`koanf` loads each layer into its OWN instance. Provenance is rig's resolver over the per-layer maps, in layer order.** | **Verified against `koanf.go` 2026-09-23: `Load` parses a provider and calls `merge`; precedence is call order and the merged map forgets its source.** A library that answers *"what is the value"* cannot answer *"why"*, and *why* is §6's `origin`. **This is the clause §44 says a flag parser cannot meet, and it is why S4 is a service** | one `koanf` instance with every layer merged in; any `origin` computed by re-reading files at query time |
| **4** | **`origin` reports, for the winner and every loser: the layer, the file (or `env`/`flag`/`runtime`) and the value.** ⛔ **§6 also promises the LINE, and this build does not deliver it** - see the open rows | `parsers/toml/v2` returns a map; no TOML parser in §22's candidate set returns per-key positions on success. A line would be a second, position-aware pass over every file. **File plus dotted key opens the right place with one `grep`** | a hand-written TOML position scanner in this build |
| **5** | **A change set is validated WHOLE, as one resolved document against the schema, before any key is applied. Failure names the key, the layer and the reason, and applies nothing** | §6: *"A change set is validated whole before any of it is sent, so nothing lands half-applied."* §38's fourth rule: the refusal carries a reason a person can act on | applying key by key and reporting the failures |
| **6** | **Environment mapping is BY THE SCHEMA, not by string transform.** For every declared key, its env spelling is derived (`display.name` → `RIG_DISPLAY_NAME`) and looked up; a `RIG_*` variable that matches no key is an orphan | `RIG_DISPLAY_NAME` is `display.name` or `display_name` under a blind `_`→`.` transform, and the two are different keys. Deriving from the declared set makes the mapping total and unambiguous. The `env/v2` provider's `TransformFunc` consults the key set | a transform that guesses the nesting |
| **7** | **An unknown key in a file is an ORPHAN: recorded, reported, never fatal** | §6's `rig loose-ends` clause: *"a renamed key reports its losers."* Refusing would make a key added for a newer binary break an older one reading the same `/etc/rig/rig.toml`. **Orphans appear in `get`, in the snapshot and in `export`** | a typo that vanishes silently; a shared file an older binary cannot open |
| **8** | **The runtime override layer is EPHEMERAL.** `config.set` writes it in the daemon's memory; a restart clears it; the snapshot records it as the winner while it lives | The top layer outranks the unit file's flags. Persisting it is how a `debug` set during one investigation outlives the investigation and beats every file forever. A lasting change is a file edit, and the UI that writes files is M4's settings UI, not this build | a persisted `override.toml` that outranks flags across restarts |
| **9** | **Per key, `set` answers `applied` or `needs-restart`, and the schema says which** through one custom keyword, `x-rig-apply: live | restart`. `log.level` is `live` (a `slog.LevelVar` the handler reads). `display.name` is `live` (the CLI reads it per run) | §6: *"shows accepted, rejected with the program's own reason, or needs-restart"*. **No bus and no push:** a client learns of a change at its next read, which §44 excludes by name and B107 keeps deferred | `config.changed`; any subscription; a push path of its own |
| **10** | **After every resolution `rigd` writes `<estate state dir>/config/resolved.toml`**: the merged values, one comment per key naming the winning layer and file, the orphans, and the schema version. Written to a temp file and renamed. **No secret is in it** because rig holds none (§44) and §5g forbids it | §6: *"Every resolution is written to a snapshot on disk, already merged, with provenance."* §5g's stub reads that file and nothing else. Temp-and-rename is the atomic step the standard library offers | a snapshot a reader can observe half-written; a snapshot with a secret |
| **11** | **`export` prints the same document the snapshot holds, for rig's own keys. `diff <file>` compares a saved export to the live resolution key by key**, exit 0 same, 1 differs, 2 error | §6: *"reproduce and compare the caller's machine."* **`introspect` (§14) is not reached in this build** because rig's own keys are the only keys; it gates estate-wide export the day a program's keys exist | an export that reaches another program's keys without `introspect` |
| **12** | **Two wire verbs.** `rig.config.get` (read-only, idempotent yes) carries values WITH provenance, so `origin` is a rendering of `get` on one key. `rig.config.set` (`writes-files`: it rewrites the snapshot; idempotent yes: the same set leaves the same state) | Fewer verbs, each with a declaration that is right on its own. `EstateRequest`'s precedent: nothing the client could write that the daemon already knows | a third verb whose declaration duplicates `get`'s |
| **13** | **`get` is promoted to the MCP door. `set` is NOT, in this build** | §6: *"C1 owes a grant model before it owes a verb"*, and the partition question (a retheme grant is not a trust-settings grant) is unanswered. An agent may read every key and its origin today, which is C2's read half. **Deferred, not dropped** (§43) | an agent writing a key before anyone ruled what a write needs |
| **14** | **`--log-level` survives as a flag SPELLING, declared by the config service from the schema into the flag layer.** `main.go` declares only `--estate` and `--version` by hand, with the reason in a comment | The unit file and every script keep working. **The acceptance test is that `flag.String("log-level"` is gone from `main.go`**, not that the flag is | breaking `rigd --log-level=debug`; a flag declared twice |
| **15** | **Only `rigd` links `koanf`.** `cmd/rig/layering_test.go` gains the row beside SQLite | §5g: the client tolerates rig's absence and does not reimplement it. A CLI that links a config library is one commit from parsing files itself | a second resolver in the client, the thing §5g deleted |
| **16** | **No settings UI, no `loose-ends` verb, no `apps/<id>.toml` reader with a program behind it, no live push, no `set` over MCP** | §45: nothing he has not looked at. Each is on the milestone it already has | building breadth again |

---

### The schema for rig's own keys, this build

```jsonc
{ "$id": "rig://schema/rig", "type": "object", "additionalProperties": false,
  "properties": {
    "log": { "type": "object", "additionalProperties": false, "properties": {
      "level": { "enum": ["debug", "info", "warn", "error"], "default": "info",
                 "x-rig-apply": "live",
                 "description": "the daemon's slog floor; read on every line" } } },
    "display": { "type": "object", "additionalProperties": false, "properties": {
      "name": { "type": "string", "maxLength": 64, "default": "",
                "x-rig-apply": "live",
                "description": "shown in place of this user's own seat on provenance surfaces; empty falls back to the seat name" } } } } }
```

**Two keys, and that is the point.** `additionalProperties: false` at every
level is what makes decision 7's orphan detection total. A third key arrives
with its consumer, never ahead of it.

---

### The wire

Six messages, appended to `proto/rig/v1/wire.proto`, additive under §21.

```proto
message ConfigGetRequest { string prefix = 1; }        // "" is every key

message ConfigLayerValue {
  string layer = 1;        // default | system | user | env | flag | runtime
  string file = 2;         // the path for a file layer; "" otherwise
  string value_json = 3;
}

message ConfigValue {
  string key = 1;
  ConfigLayerValue winner = 2;
  repeated ConfigLayerValue losers = 3;   // lowest layer first
  string apply = 4;                        // live | restart
}

message ConfigGetResponse {
  repeated ConfigValue values = 1;
  repeated string orphans = 2;             // "<layer>:<file>:<key>"
  string snapshot_path = 3;
}

message ConfigSetRequest { map<string, string> values_json = 1; }

message ConfigSetResponse {
  map<string, string> outcome = 1;         // key -> applied | needs-restart
  string snapshot_path = 2;
}
```

**Values travel as JSON text, not as a `oneof` of scalar types**, because the
schema is JSON Schema and the validator reads JSON; the CLI renders. A refused
`set` is the existing invalid-argument refusal naming key, layer and reason, and
the response is not sent. **Decision 10 binds `snapshot_path`:** the test that
proves it travels owes two mutations, empty and wrong non-empty, because
protojson drops the empty string.

**Declared in `internal/daemon/self.go`** beside `estate`: `get` through the
`readOnly` helper; `set` with `Effects: kernel.EffectsWritesFiles`,
`Idempotent: kernel.Yes`, `Duration: kernel.DurationInstant`, `Shape:
kernel.ShapeUnary`, `Confirms: kernel.No`, `Promote: false`. **Dispatched** from
`daemon.go`'s switch through two arms to `serveConfigGet` and `serveConfigSet`
in `internal/daemon/config.go`.

---

### The commands

```
rig config get [<prefix>] [--json]
    every key under the prefix, its value and the layer that won

rig config origin <key> [--json]
    the winner and every loser: layer, file, value      (M4's demo, §44)

rig config set <key>=<value> [<key>=<value> ...] [--json]
    validated whole; per key prints applied | needs-restart;
    a refusal names the key, the layer and the reason and changes nothing

rig config export [--json]
    the resolved document with a provenance comment per key, plus orphans

rig config diff <file> [--json]
    exit 0 identical, 1 a key differs (each named), 2 unreadable
```

**`display.name` is read by `cmd/rig/record.go`'s `displaySeat` through one
`config.get` per process**, cached, on the connection the verb already holds. An
unreachable daemon falls back to the seat name exactly as an unset variable does
today, so the fallback the comment calls *"the half that is tested hardest"*
keeps its test.

---

### Files the seat owns, to be written into `COORDINATION.md` before the first write

The seat is **`config`**, a subagent. **These rows are PROPOSED; the lead writes
them into `COORDINATION.md` the turn he approves S4, and not before.**

| Path | New? | Note |
|---|---|---|
| `internal/config/` | **NEW** | layers, resolve, provenance, validate, env mapping, snapshot, the embedded schema. Imports `koanf` and the validator; **imports neither `internal/record` nor `internal/daemon`** |
| `internal/daemon/config.go`, `config_test.go` | **NEW** | `serveConfigGet`, `serveConfigSet`, the `slog.LevelVar` |
| `internal/daemon/daemon.go` - two dispatch arms | **grant, two `case` lines** | the lead's file; nothing else in it moves |
| `internal/daemon/self.go` - two declaration blocks | **grant, two blocks** | the lead's file |
| `proto/rig/v1/wire.proto`, `wire.pb.go` | **grant, six messages, appended** | `make proto`, committed WITH the handler that reads them |
| `internal/paths/paths.go` - two functions | **grant** | `UserConfigFile()` under `$XDG_CONFIG_HOME`, `SystemConfigFile()` at `/etc/rig/rig.toml`. The snapshot lives under `EstateStateDir(name)`, which exists |
| `cmd/rigd/main.go` - the `--log-level` block | **grant, one block removed, one comment added** | decision 14. **Resolution errors before the logger exists go to stderr at `info`**, the one place a bootstrap default is honest |
| `cmd/rig/config.go`, `config_test.go` | **NEW** | the five sub-verbs |
| `cmd/rig/record.go` - `displaySeat` | **grant, one function** | decision 12's consumer |
| `cmd/rig/layering_test.go` - one row | **grant** | decision 15 |
| `cmd/rig/main.go`, `complete.go`, `testdata/exec/config-*.golden` | **entries yes, helpers no** | the `cli` seat's precedent |
| `go.mod`, `go.sum` - the `koanf` lines | **under `rig-makefile`**, taken and released in one action, committed with the first import | §22's row is the LEAD's: the seat reports the measured bytes, the lead writes ADOPTED |
| `size-ratchet.json` | **under `rig-makefile`, its own commit** | moved by the measured number with the reason, never silently |

**Not the seat's:** `plan/`, `PLAN.md`, `frontend/`, `internal/kernel`,
`internal/wire`, `internal/record`, the rest of `internal/daemon/` and
`cmd/rig/`, `packaging/`, and every logbook file outside its agent-work
directory.

---

### The tests owed, and the red control that proves each one bites

**The four-clause form, §37.** Every row's red control is run and written into
the seat's `FINDINGS.md` at a named sha.

| Test | Red control |
|---|---|
| every adjacent layer pair, lower loses to higher, six pairs | swap two loads; the pair's test must go red |
| `origin` names the winner's layer and file and EVERY loser in order | drop one loser from the response |
| a two-key `set` with one invalid value applies neither | apply sequentially; the first key must not have moved |
| an unknown key in a file is an orphan and the file still loads | make it fatal |
| `RIG_DISPLAY_NAME` resolves `display.name`; `RIG_DISPLAY_NAM` is an orphan | blind `_`→`.` transform |
| `set log.level=debug` makes a `Debug` line appear on the handler's log within the same process | leave the `LevelVar` unwired |
| `--log-level=warn` on the command line beats `RIG_LOG_LEVEL=debug` | reverse the two loads |
| the snapshot is never observed half-written | write in place |
| `snapshot_path` travels: empty and wrong non-empty | protojson's dropped empty string |
| a refusal names key, layer and reason | return the validator's raw error |
| `cmd/rig` links no `koanf` | import it from `cmd/rig/config.go` |
| `rigd --estate` and `--version` still parse and act; `packaging/rigd.service` unchanged | remove either flag |

---

### The gates, before every commit

`make ci` 0, `make lint` 0, `make proto` idempotent, `rigseed --check` 0
(this build writes no record), `make deps-check` 0 once the §22 row exists,
`make bench-size` green after the ratchet moves in its own commit. **The byte
cost of `koanf` on `rigd` is measured against the ratchet's baseline and
reported in `FINDINGS.md` before the §22 row is asked for.**

---

### Acceptance, restated so it can be checked rather than read

1. **`git grep -n 'flag.String("log-level"' cmd/rigd` prints nothing, and
   `git grep -n 'Getenv("RIG_DISPLAY_NAME")' cmd/rig` prints nothing.** §44's
   form: the hand-rolled version is DELETED.
2. **`RIG_LOG_LEVEL=debug rigd --estate <throwaway>`, then
   `rig config origin log.level`** names `env` as the winner and `default` as
   the loser, with the values.
3. **`rig config set log.level=warn` prints `applied`**, and a debug line that
   was being written stops in the same process, with no restart.
4. **`rig config set log.level=loud display.name=x` is refused whole**, naming
   `log.level` and the enum, and `rig config get` shows both keys unchanged.
5. **`rig config export > f && rig config diff f` exits 0**; restarting the
   daemon with `RIG_DISPLAY_NAME=B` set, `rig config diff f` exits 1 naming
   `display.name` alone.
6. **`rig record query` renders `B` in place of this user's seat** on a
   provenance surface, and renders the seat when the variable is unset.
7. **`cmd/rig` links no `koanf` and no SQLite**, by the layering test and by
   `go list`.
8. **§22's `koanf` row reads ADOPTED with the measured bytes on `rigd`**, and
   `make deps-check` is 0.
9. `make ci` 0, `make lint` 0, `make proto` idempotent, `rigseed --check` 0.

---

### ⛔ OPEN, AND EACH IS HIS. PARKED WITH A RECOMMENDATION AND WITH WHAT PROCEEDS MEANWHILE

| Row | Recommendation | Proceeds meanwhile |
|---|---|---|
| **§6 promises file AND LINE in `origin`; decision 4 delivers file and key** | **Narrow §6 to file and key.** No parser in the candidate set yields positions on success, and file plus dotted key is one `grep` from the line | the build reports file and key; a line is additive later |
| **`--estate` stays a flag and is not a key** (first-consumer table) | agree, for §37's collision reason | nothing waits |
| **the runtime override is ephemeral** (decision 8) | ephemeral. The persistent path is the file layer, written by M4's settings UI | nothing waits |
| **`set` is not on the MCP door until C1's grant model** (decision 13) | agree. C1 is its own row and its first question is whether config-write is one capability or a partition | `get` is promoted; an agent reads everything |
| **`display.name` as the second key, or hold at one** | **take it.** It is the only wire-shaped consumer the tree offers, and the wire shape is what every program inherits | nothing waits |
| **spawn order once approved: S4 may run before S1 lands** | **yes.** S4 opens no database (the finding above), and one build subagent at a time on the shared tree is §45's still-open row; this seat's recommendation there is one BUILD at a time, measurements beside it | B104 finishes first either way |

---

### What this section does not change

- **§6 stands in full**, including C1 and C2 as requirements on their
  milestone; the one clause this build narrows is in the open rows for him.
- **§44 S4 stands** and this is its build shape. §44's order stands as his
  ordering of attention; the dependency finding above changes when S4 CAN be
  spawned, not where he placed it.
- **B107, the event bus, is deferred and not deleted** (§43). Decision 9 is
  where this section stops short of it.
- **The tray acceptance test is untouched.**
- **The capability after B104 is his to choose**, §45. This section is
  preparation for that choice and nothing more.
