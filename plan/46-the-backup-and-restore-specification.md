## 46. The backup and restore specification

**Step 4 of §45's loop for B104, written 2026-09-23.** He chose backup and
restore as the first capability on 2026-09-20 and §44 S2 states what it must
do. **This section is the build specification a subagent implements from
unattended**, in the shape §45's *"what the subagent is owed"* asks for: a
written specification, an acceptance test stated before the start, named
files, and persistence to disk as it goes. ⛔ **Nothing in it is a conversation
summary.** Where a choice was the seat's rather than his, the row says so.

> **In one line: `rig backup` writes one archive of a named estate's own
> persistent state, and `rig restore` puts it back onto an estate where `rigd`
> has never run.** rig's own state only. A program's database is S1's and M11's
> per-program backup, not this.

---

### ⛔ WHY THE HAND `cp` WAS NEVER A BACKUP, MEASURED 2026-09-23

The record store runs in WAL mode (`internal/record/store.go`, *"a reader is
never blocked behind a writer"*). **In WAL mode a committed write lives in
`record.db-wal` until a checkpoint folds it into `record.db`**, and a copy of
the main file alone carries none of the frames still in the WAL.

| File in the production estate | Size | Modified |
|---|---|---|
| `record.db` | 6.7 MB | **19 Sep 13:28** |
| `record.db-wal` | **4.2 MB** | **23 Sep 06:29** |

**Every write of the last four days is in the WAL and not in the file the hand
copies took.** The five `record.db.before-*` files in `backups/` are copies of
the main file only; whether each was complete at the moment it was taken cannot
be known now. **The live store is intact - this is not a loss - but it is the
whole reason the command exists rather than a convenience.** A backup of a WAL
database is taken through SQLite, never through the filesystem.

---

### The design, decided

⛔ **EVERY ROW BELOW IS THE SEAT'S CALL UNLESS IT CITES A RULING.** They were put
to him on 2026-09-23 with this section; a row he reshapes is edited here, not
argued in a handoff.

| # | Decision | Why | What it rules out |
|---|---|---|---|
| **1** | **`rigd` takes the snapshot, not the CLI.** `rig backup` calls a new verb `backup.create` on the control plane | **§22: only `rigd` links `modernc.org/sqlite`**, and `cmd/rig/layering_test.go` is the gate that keeps it so. The daemon already holds the store open | a CLI that opens the database. The bulk plane §44 named for S2 is not spent: the CLI and the daemon share one filesystem, so the archive is written to disk and the wire carries its manifest |
| **2** | **The snapshot is `VACUUM INTO ?`**, the path bound as a parameter, never spliced | Transactional: the output is a consistent snapshot of the database under WAL, writers are not blocked, the result has no `-wal` or `-shm` sidecar and is compacted. SQLite 3.53.4 in modernc v1.59.0 carries it. **§38 search:** the named fallback is modernc's online backup API (`backup.go`, `Backup.Step`), which needs a raw driver connection and is the second choice for that reason | a hand-written page copier; any `fmt.Sprintf` into SQL |
| **3** | **The archive is `archive/tar` in `compress/gzip`, both standard library.** Zero new dependencies | The whole estate is under 7 MB. **§38 search:** `klauspost/compress/zstd` would need a §22 row and a footprint measurement for a benefit these sizes do not need; `mholt/archiver` pulls every format for one. **§7's "maximal force" is the retention archive of history segments, a different artefact, and is not spent here** | a new `go.mod` line. A seat that believes one is needed sends the lead a message, not a diff |
| **4** | **Members, in order: `manifest.json`, then `record.db`.** Nothing else today | §7's table marks four rows yes; **only the record store exists in code**. The config tree is three flags, the audit log and the notification centre are unbuilt. **Each joins as a member the day it exists**, additive, and restore refuses a member name it does not know - so an older binary refuses a newer archive rather than half-restoring it | a member list that can grow silently; a restore that skips what it does not understand |
| **5** | **Not archived:** `coord.db`, `record.db-wal`, `record.db-shm`, `last-announced-version`, the `.pid` claim | `coord.db` is §7's *"rebuilt at start"* row - leases, epoch and presence are live-session state, and restoring them onto a fresh machine restores dead leases. The WAL and shm are folded into the snapshot by decision 2. The version file is the window's. The claim is a lock | a restore that revives a lease |
| **6** | **The archive lands in `$XDG_STATE_HOME/rig/backups/<estate>-<UTC stamp>.tar.gz` and the caller never names a path.** The daemon chooses the name, the CLI prints it | **§38's fourth rule: no caller path joined unvalidated.** An empty request has nothing to validate. Copying the archive off the machine is the user's, and a plain `tar.gz` opens anywhere. The directory already exists; `paths` gains one function for it | an `--out` flag on the wire; a `path` field in the request |
| **7** | **Restore is OFFLINE.** `rig restore --estate <name> <archive>`, and it takes the estate's own claim (`instance.AcquireName(paths.EstateLock(name), name)`) for the duration | It is the same claim `rigd` takes, so a daemon holding the estate on ANY runtime directory produces the same refusal, and two restores cannot interleave. **The CLI links neither store**, so the restore is file placement and verification; the daemon migrates on its next open, which `Store.start()` already does | a restore into a running daemon; a `backup.restore` verb |
| **8** | **Existing state is never deleted. Without `--force` restore refuses; with it the estate directory is renamed aside WHOLE** to `<name>.replaced-<UTC stamp>` | ⛔ **File by file is unsafe:** a stale `record.db-wal` beside a fresh `record.db` is replayed into it on the next open. The directory moves as one, and the user removes it when satisfied | `os.RemoveAll` anywhere in the restore path |
| **9** | **All or nothing.** Extract into a sibling `<name>.restoring-<UTC stamp>` under the same parent, verify every member's SHA-256 against the manifest, then two renames | A failure at any step leaves the target untouched and the partial directory named for what it is. Two renames on one filesystem are the atomic step the standard library offers | a partially written estate that opens |
| **10** | **The manifest carries `schema_version`, and restore refuses one newer than the restoring binary's `record.SchemaVersion`** | The migration runner goes forward only. An OLDER version is accepted and migrated at the daemon's next open | a store that lies about itself, which `store.go` names as the silent failure |
| **11** | **`backup.create` on an unnamed estate is refused.** ⛔ **Corrected 2026-09-23 on the backup seat's finding F1:** since the scratch store landed (2026-09-17) an unnamed estate opens `record.OpenScratch()` and `recordStore` no longer refuses it, so the refusal is `d.estate == ""`, checked after `recordStore`, in `recordStore`'s own wording | §37: an unnamed estate keeps no persistent state, so there is nothing to back up. The scratch store is discarded at every start by design | a backup of a store that will not exist tomorrow |
| **12** | **No MCP tool, no `backup.list`, no schedule, no encryption, no off-machine copy** | The agent door does not re-seed the store; `ls` lists the directory; rig has no ticker (§44's scheduling row) and holds no secret (§44's secrets row). **Deferred, not dropped** - §43's correction | building what he has not looked at, §45 |

---

### The manifest

`manifest.json`, first member, indented, keys sorted. **Every value is set by the
daemon; nothing here is read off the request.**

| Key | Type | What |
|---|---|---|
| `format` | int | **1.** Bumped only when a reader of the old format could not read the new |
| `estate` | string | the estate name the daemon runs |
| `created_unix_nano` | int | when the snapshot was taken, the daemon's clock |
| `rig_version`, `wire` | string | what `rigd --version` prints, so a reader can name the binary that wrote it |
| `schema_version` | int | `record.SchemaVersion` at snapshot time, read from the snapshot with `PRAGMA user_version`, not from the constant |
| `records`, `heads` | int | row count and head count in the snapshot, so a restore can be checked by a query |
| `members[]` | `{name, bytes, sha256}` | every member after this one, in archive order |

**`heads` is the number the acceptance test compares**, because `record.query`
answers heads and the row count includes every superseded version (gen 22's
finding: 2,568 rows, 1,057 heads, and a document that said "2,538 records" was
counting rows).

---

### The wire

Two messages, appended to `proto/rig/v1/wire.proto`, additive under §21.

```proto
message BackupCreateRequest {}

message BackupCreateResponse {
  string path = 1;              // where the archive landed
  string sha256 = 2;            // of the whole archive file
  uint64 bytes = 3;
  uint64 schema_version = 4;
  uint64 records = 5;
  uint64 heads = 6;
  int64  created_unix_nano = 7;
}
```

**The request is empty on `EstateRequest`'s precedent:** anything a client could
write into it would be a second place for the client and the daemon to disagree.
⛔ **DECISION 6 binds `path` and `sha256`:** the test that proves they travel owes
TWO mutations each, empty and wrong non-empty, because protojson drops the empty
string.

**Declared in `internal/daemon/self.go`** beside `record.put`: `Effects:
kernel.EffectsWritesFiles`, `Idempotent: kernel.No` (two calls, two archives),
`Duration: kernel.DurationSeconds`, `Shape: kernel.ShapeUnary`, `Confirms:
kernel.No`. **The declaration is the visual half** - §44: a service whose data
is declared correctly gets its view for free, and a hand-written backup screen
is the sign the declaration is wrong. Nothing in `frontend/` is owed.

**Dispatched** from `daemon.go`'s switch through one arm to a new
`serveBackupCreate` in `internal/daemon/backup.go`, on `serveRecord`'s pattern:
`recordStore` first, then the estate-name check (decision 11 as corrected), so
the store-did-not-open refusal stays in one place.

---

### The commands

```
rig backup [--json] [--timeout=30s]
    prints the archive path, its size, sha256, records and heads

rig restore --estate <name> [--force] [--json] <archive>
    refuses a positional count other than one, an invalid estate name
    (paths.ValidEstateName), a held estate, existing state without --force,
    an unknown member, a sha256 mismatch, a newer schema
    on success prints where the estate now is and the check to run:
      rigd --estate <name>; rig record query ... expect <heads> heads
```

**`restore` never connects to a daemon.** `backup` connects like `estate` does
and uses `call(ctx, c, "rig.backup.create", ...)`; an unreachable socket is
`noDaemon(err)`, not a guess.

---

### Files the seat owns, and the rows were written before the first write

`COORDINATION.md` carries the binding table. The seat is **`backup`**, a
subagent, and every path below is its for as long as it exists.

| Path | New? | Note |
|---|---|---|
| `internal/backup/` | **NEW** | the archive: write, read, verify, restore. **Standard library only. It imports neither `internal/record` nor `modernc.org/sqlite`**, because `cmd/rig` imports it |
| `internal/record/snapshot.go`, `snapshot_test.go` | **NEW, a two-file grant into the lead's package** | `(*Store).Snapshot(ctx, dst) (Snapshot, error)` - `VACUUM INTO`, then `user_version`, row and head counts read from the snapshot |
| `internal/daemon/backup.go`, `backup_test.go` | **NEW** | `serveBackupCreate` |
| `internal/daemon/daemon.go` - the one dispatch arm | **grant, one `case` line** | the lead's file; nothing else in it moves |
| `internal/daemon/self.go` - the one declaration block | **grant, one block** | the lead's file; nothing else in it moves |
| `proto/rig/v1/wire.proto`, `wire.pb.go` | **grant, the two messages, appended** | regenerated with `make proto`, **committed with the handler that reads them** - a message set written ahead of its handler is unverified, `record.go` says why |
| `internal/paths/paths.go` - one function, `BackupDir()` | **grant, one function** | `$XDG_STATE_HOME/rig/backups`. `backend-3`'s row is `owner_gone`; the row moves back the turn that seat is respawned |
| `cmd/rig/backup.go`, `restore.go`, their `_test.go` | **NEW** | the two verbs |
| `cmd/rig/main.go`, `complete.go`, `testdata/exec/backup-*.golden`, `restore-*.golden` | **entries yes, helpers no** | the `cli` seat's precedent. A helper the verbs need lives in the seat's own files |
| `logbook/projects/rig/agent-work/<its dir>/` | seeded by the hook | `STATUS.md`, `FINDINGS.md` |

**Not the seat's, and a need for one is a message to the lead:** `Makefile`,
`go.mod`, `go.sum`, `size-ratchet.json` (see the gates below for the one
exception), `plan/`, `PLAN.md`, `frontend/`, `internal/kernel`, `internal/wire`,
the rest of `internal/daemon/` and `internal/record/`, and every logbook file
outside its agent-work directory.

---

### The tests owed, and the red control that proves each one bites

**The four-clause form, §37: a test that passes is half the evidence; the
mutation that turns it red is the other half.** Every row's red control is run,
and what it printed goes in the commit body.

| # | Test | Red control |
|---|---|---|
| 1 | a snapshot of a store holding N records at `SchemaVersion` opens with N records, the same `user_version`, and `PRAGMA integrity_check` answers `ok` | truncate the snapshot by one page: integrity fails |
| 2 | a snapshot taken while another connection holds an open write transaction returns without error and holds either N or N+1 records, never a torn state | none owed: consistency is `VACUUM INTO`'s guarantee. The test's job is that no `SQLITE_BUSY` escapes |
| 3 | write an archive, read it back: members, sizes and sha256 match the manifest; `heads` matches a query on the restored file | flip one byte of `record.db` inside the tar: restore refuses and **the target directory does not exist afterwards** |
| 4 | a tar carrying `../x`, an absolute name, `record.db-wal`, or any name outside the known set is refused **before any byte is written** | adversarial inputs are the control |
| 5 | restore against an estate whose claim a test holds is refused with `NameHeldError`, nothing written | release the claim: the same restore succeeds |
| 6 | restore over existing state refuses without `--force`; with it, the old directory exists whole under `<name>.replaced-*` and the old `record.db` bytes are unchanged | assert the old file is byte-identical, not merely present |
| 7 | a manifest with `schema_version = record.SchemaVersion + 1` is refused; `record.SchemaVersion - 1` is accepted | the two numbers are the control |
| 8 | **end to end, the acceptance test:** `rigd --estate production` on a private `XDG_STATE_HOME` AND a private `XDG_RUNTIME_DIR`, K records put, `rig backup`, `rig restore --estate development` under a SECOND private `XDG_STATE_HOME`, `rigd --estate development` there, `rig record query` shows K heads and `manifest.heads` is K. ⛔ **CORRECTED 2026-09-24: this row named estates `a` and `b`, and `rigd` refuses both** - the closed set is §37's, Boris 2026-09-11, checked at `cmd/rigd`'s flag (the seat's finding A, its `FINDINGS.md` §10). The two names it accepts are safe here ONLY under a private `XDG_STATE_HOME`, and the machine's own estates are proven untouched by size and mtime before and after. **Performed by the lead 2026-09-24 with binaries built at `452f0f4`:** 5 heads through, the restored estate opened by `rigd --estate development` and queried to 5 | restore the same archive into `development` again without `--force`: refused, nothing written. **And while a live `rigd` holds it: refused, the claim and its pid named** (run 2026-09-24, the first time that arm met a real daemon) |
| 9 | `cmd/rig/layering_test.go` stays green, and `go list -deps ./cmd/rig \| grep -c modernc` prints 0 | ⛔ **CORRECTED 2026-09-24 on the backup seat's finding F5, measured 2026-09-23:** importing `internal/record` from `internal/backup` in a detached copy leaves `layering_test.go` GREEN - it names neither the store nor the driver - while `grep -c modernc` prints **8**. The controls that DO fire: `internal/backup`'s `TestThisPackageLinksNothingButTheStandardLibrary` (14 packages named, slice 2) and the `modernc` assertion in `cmd/rig/restore_test.go` (slice 4). The first sentence of this row was a red control that could not go red |
| 10 | the `BackupCreateResponse` string fields travel: two mutations each per DECISION 6 | empty, and a wrong non-empty value |
| 11 | CLI transcripts on `exec_test.go`'s pattern: `backup-no-daemon`, `backup-refuses-a-positional`, `restore-usage`, `restore-refuses-a-bad-estate-name`, `restore-refuses-two-archives` | the goldens are the control |

⛔ **THE ISOLATION TRAP, AND IT HAS BITTEN TWICE:** a private `XDG_RUNTIME_DIR`
isolates the socket and NOT the store. **Set `XDG_STATE_HOME` under the
scratchpad as well**, print the resolved store path before every destructive
step, and prove the production store's count and mtime are unchanged afterwards.
`COORDINATION.md`, *"a private XDG_RUNTIME_DIR isolates the socket and not the
store"*.

---

### The gates, before every commit

| Gate | Expect | Note |
|---|---|---|
| `make ci` | 0 | `fmt-check vet lint-house test-race schema-check deps-check theme-gate`. **`lint-house` is in `ci` and `lint` is not** |
| `make lint` | 0 | CI runs it as its own step, so red here still blocks a push |
| `make proto` then `git status --porcelain proto/` | empty | the committed `wire.pb.go` is what the tool generates |
| `go run ./cmd/rigseed --check` | 0 | untouched by this work; run so the claim is measured rather than assumed |
| `make bench-size` | **may go red, and that is not a defect** | ⛔ **It is outside `ci` by his 2026-09-12 ruling and is NOT reintroduced.** `rig` and `rigd` both grow here. If red, re-record **only the rows this work moved** with `sizeratchet --update --bin build/<name>` under the `rig-makefile` lock, in the same commit, with the growth attributed in the body - rule 7 |
| `nocontextfree` | passes | `context.Background()` only as the argument of `WithTimeout` and its three siblings; `//rig:allow` needs a reason and expires |

**Gate in a detached worktree at a named sha**, `COORDINATION.md`'s recipe, and
never mutate the shared tree: a red control is run in `/tmp/mut-backup`, not in
the checkout every other seat compiles.

---

### Persistence, commits, and the report owed

- **`STATUS.md` is checkpointed after each slice lands, not at the end:** the
  snapshot, the archive, the wire and handler, the two CLI verbs, the end-to-end
  demonstration. **`FINDINGS.md` carries every command run and what it printed,
  at a named sha, with `git status --porcelain` beside each** - a measurement in
  a shared tree names whether the tree was clean.
- **One commit per slice, `type(scope): description`, imperative, under 72
  characters.** The body carries the evidence the lead copies into
  `four-clause-passes.md`; the seat certifies nothing itself.
- **Stage and commit in one shell action, by name, and count the staged files
  first.** Never `git add -A`, never `stash`, never `checkout --` a path that is
  not in the table above.
- ⛔ **THE ARMED REPORT, §37:** what this work cost in time or tokens and which
  planned capability would have removed the cost, in `STATUS.md`'s `Unresolved`
  block, addressed to the lead. *"I derived this by hand three times"* is a
  report; *"it would be nice if"* is not.
- **Nothing in `plan/` or the logbook outside the agent-work directory is
  written by the seat.** A defect in this section is a finding in `STATUS.md`,
  and the lead edits the section.

---

### Acceptance, restated so it can be checked rather than read

1. **`rig backup` on a named estate prints an archive path**, and `tar tzf` on
   it lists `manifest.json` then `record.db` and nothing else.
2. **`rig restore --estate <fresh> <archive>` onto an estate `rigd` has never
   run, then `rigd --estate <fresh>`, then `rig record query`** returns exactly
   `manifest.heads` heads. **The restore is the test, not the backup** - §44.
   **`<fresh>` is `development` under a PRIVATE `XDG_STATE_HOME`**, because
   §37's closed set leaves `rigd` no other name to open (corrected 2026-09-24;
   test row 8 carries the run).
3. **`cmd/rig` links no SQLite**, by the layering test and by `go list`.
4. `make ci` 0, `make lint` 0, `rigseed --check` 0, `make proto` idempotent.
5. **The production demonstration waits for his redeploy.** A redeploy quiesces
   every peer and its trigger is an accumulation HE judges (`COORDINATION.md`);
   the seat demonstrates on its own estates and the lead runs `rig backup`
   against production the first time the deployed daemon carries the verb.
   **The `cp` in a seat's shell history is replaced the day that happens**, and
   the next re-seed's notes name the command.

---

### Found by the build, 2026-09-24, and where each went

| Finding | State |
|---|---|
| **`rig restore` accepts every lexically valid name and `rigd` opens two.** A restore into `b` exits 0 and prints `rigd --estate b`, which exits 1 (the seat's finding B). The set is closed at `cmd/rigd`'s flag by design, §37, and `cmd/rig` cannot import it | **B114**, the lead's: one package both binaries read, so the restore refuses what the daemon will. No ruling needed; the set does not move |
| **The check a restore printed counted lines, not heads.** `rig record query --json` answers on ONE line, so `grep -c '"id"'` printed 1 for every non-empty store | **fixed** the same day: the check is the human query's last line, which counts heads |
| **`rig --help` listed neither verb** | **fixed**, same commit |
| **`run`'s cyclomatic complexity is 25 of 25**, which is why the two verbs share one dispatch arm | **B115**, the lead's |
| **Eight `internal/daemon` tests opened the machine's own estates** from `make ci` in the shared tree, found by the lead's gate run at `452f0f4` | **fixed at `a9c7ab8`**: a package `TestMain` and a guard test. `cmd/rig` tests still inherit the shell's `XDG_RUNTIME_DIR` (the seat's finding, its `FINDINGS.md` §8) - B87 carries the class |

### What this section does not change

- **§7's table stands**, four rows marked yes; three are unbuilt and join the
  archive when they exist. **§44 S2 stands** and this is its build shape.
- **The tray acceptance test is untouched.**
- **The capability after B104 is his to choose**, §45. A seat that reads B103
  or B105 out of the backlog while this runs has taken back that decision.
- **Deferral is never deletion**, §43: every row in decision 12 is still rig's
  to build on the milestone it already has.
