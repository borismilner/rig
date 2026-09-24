## 48. The storage specification

⛔ **DRAFT. STEP 1 AND A STEP-4 DRAFT OF §45's LOOP FOR B103 (S1), WRITTEN
2026-09-23 EVENING WITHOUT HIM, ON HIS WORD TO DO AS MUCH AS POSSIBLE ALONE.**
§45's 2026-09-23 block binds: *preparing is not approving*. **He has not
approved, reshaped or killed S1, and no subagent is spawned from this section
until he does.** Every row below is the seat's unless it cites a ruling, and a
row he reshapes is edited here, not argued in a handoff.

> **In one line: one engine, one migration runner and one place that opens a
> database, in `rigd`; rig's own two stores become its first two consumers,
> `bbolt` leaves, and the one wire change is the projection B108 measured as
> the largest cost in the estate.** The program-facing `store.*` verbs are
> specified in shape and built the day a program adopts them.

**Drafted for §43's Q1 answered STAYS** (the record mechanism is rig's, the
planner declares its kinds). If he rules GOES, this section changes in one
place, named below, and no code moves either way until he approves.

---

### Step 1. What it is, what a program does with it, what it costs

**What it is.** Today rig has implemented storage twice and offers it to
nobody (§44, B103): `internal/record/store.go` is 661 lines on
`modernc.org/sqlite` with `PRAGMA user_version`, `internal/coord/store.go` is
298 lines on `bbolt` with its own `migrations` map. **Two engines, two
runners, one daemon, one user.** S1 is the one engine, the one runner and the
one opener, with every consumer a namespace of it.

**What a program does with it.** Declares its collections in the `data` block
of its declaration (§5e: *column schema, filters, sorts*), and then puts, gets
and queries through rig, **never naming the engine** (Boris, 2026-09-19: *"the
projects will be agnostic to whether the storage used is SQLite or Postgres or
anything else"*). rig runs its migration before it starts (M11), backs it up
(§7), and shows it in a store browser (M11).

**What it costs.**

| Cost | Size |
|---|---|
| a package | `internal/store/`: open, WAL pragmas, the migration runner, namespace paths, the ephemeral flag |
| two rewrites | `internal/record/store.go` loses its opener and runner; `internal/coord/store.go` moves from `bbolt` to the service |
| a dependency LEAVES | `go.etcd.io/bbolt`, **-352,256 bytes on `rigd`** by §22's own measurement; the row is re-measured on removal |
| the wire | one additive field, `fields` on `RecordQueryRequest`; one read-only verb, `rig.store.list`, two messages |
| the CLI | `rig store list`; `--fields` on `rig record query` |

**The honest verdict.** The in-process half is plumbing rig already runs twice
and is low risk. ⛔ **The program-facing half has NO ADOPTER**: no in-house
program stores anything through rig, the fake adopters persist nothing
(`cmd/fakeapp`, `cmd/abacus`, `cmd/ledger` write JSON to stdout), and §5h's
gate is *"nothing here ships without a program that adopts it"*. **So this
section builds what rig uses and specifies the rest**, which is his ordering
test applied honestly rather than a service shipped to nobody a second time.

---

### ⛔ WHAT B108 MEASURED, AND IT DECIDES THE SHAPE. 2026-09-23, at `5a158a6`

**§44 forbade picking a transport before the count existed.** The count exists:
`logbook/projects/rig/agent-work/measure-b108-brief-cost-over-a-socket/FINDINGS.md`,
on a `VACUUM INTO` copy of production (3,095 records, 1,164 heads, 772 links;
project rig at 131 work items and 1,010 governing records).

| Measured | Number |
|---|---|
| SQL statements per `project.brief` | **231**, of which **220** are one `LinksFrom` per work item, returning **zero rows** |
| dependency depth, the minimum sequential round trips | **2** |
| `Store.Brief` in process, median | **27.379 ms** |
| `rig brief rig` end to end, median | 36.467 ms |
| a `rigd` verb round trip, connected client | **24.25 µs**, against §4's 6.19 µs raw socket floor re-taken the same day |
| the naive shape, 231 round trips, MEASURED | 1.483 ms at the floor, 5.6 ms at the verb cost |
| the batched shape, 2 round trips, MEASURED | **12.036 µs** |
| the answer on the wire | **195,275 bytes** of protobuf |
| ⛔ **marshal and unmarshal of that answer** | ⛔ **3.555 ms**, **69x** the 51.75 µs the socket needs to move the bytes |
| bytes read by four statements that use three fields per row | **1.77 MiB**, 90% of everything read |

**The verdict against §44's four outcomes:**

| §44's outcome | Measured |
|---|---|
| nothing beyond one round trip per brief | true of today's in-process brief; not S1's question |
| batching alone | ⛔ **no**: depth 2 costs 12 µs, **and the record verbs cannot express a batch** - `record.query` has no projection and no aggregate, `record.refs` takes one id |
| **a streamed or pushed-down query API** | ✅ **this one, and not for the expected reason.** The transport is never the bottleneck; **serialising a full answer is** |
| shared memory | ⛔ **no.** It removes at most 1.4 ms of a 27.4 ms derivation and none of the 3.6 ms of serialisation. **§4's escalation rule is not met** |

⛔ **PROJECTION BEATS BATCHING, AND IT IS THE CHEAPEST CHANGE ON THE TABLE.** A
`fields` list on `record.query` cuts 1.77 MiB to tens of kilobytes and about
3.5 ms of encoding; batching cuts 1.4 ms of transport. **Every agent that lists
work items over the MCP door, and every `rig record query`, pays the full
answer today.** That is rig using its own wire storage surface, so the
projection passes his ordering test and is in this build.

**Two more B108 named and this build defers**, because their consumer arrives
with the extraction: a multi-id form of `record.refs` (the 220 independent
lookups have to be able to say they are independent, or depth 2 is
unreachable), and an aggregate the store can push down for the per-group
maximum that `latestSteps` needs (*"or `latestSteps` leaves the store and the
582x comes with it"*). **Both are specified in the shape table below and built
when the brief moves.**

---

### The design, decided

⛔ **EVERY ROW BELOW IS THE SEAT'S CALL UNLESS IT CITES A RULING.**

| # | Decision | Why | What it rules out |
|---|---|---|---|
| **1** | **One engine: `modernc.org/sqlite`. `bbolt` leaves.** `internal/coord` becomes a namespace on the service: leases and the epoch are one table each, compare-and-swap is a versioned `UPDATE ... WHERE version = ?`, exactly as `record.put` does it today | §44: *"S1 is where the two-engine question gets settled, before writing code."* B28 ran the described search and ruled SQLite alone; `bbolt` predates it and was never re-argued. §22 already measures its cost. **§38's search is B28's**, cited rather than re-run | a second engine behind the service; a hand-rolled lease file. **A coord need SQLite cannot meet is a finding to the lead, not a second `go.mod` line** |
| **2** | **One file per namespace, under `paths.EstateStateDir(name)`, and today's names and paths do not move**: `record.db`, `coord.db` | §46 decision 4 archives `record.db` by name and decision 5 excludes `coord.db` because it is rebuilt at start. **One shared file would make that exclusion impossible.** A program's namespace is `programs/<id>.db` under the same estate directory | moving `record.db`; a single estate database |
| **3** | **One migration runner, in `internal/store`:** read `PRAGMA user_version`, refuse a future version with the existing `FutureSchemaError` shape, apply forward steps in ONE transaction, stamp. **A namespace registers its steps as data**, the form `internal/coord`'s `migrations` map already has | §44 S5: *"the part that is written twice and the part whose failure is silent."* Both current runners are correct; the service is the third writing, **and then the two are deleted** | a runner per consumer; a step outside the transaction |
| **4** | **The unnamed estate's scratch store stays ephemeral and stays in the runtime directory.** The service carries `Ephemeral` on the handle, as `record.Store` does today | §37: persistent state needs a name. `record.OpenScratch`'s reasoning moves into the service unchanged | a scratch store that persists |
| **5** | **In-process consumers get a namespaced `*sql.DB` handle and keep their SQL.** The agnostic ruling is about PROGRAMS; rig is the engine's owner | `brief.go` is 1,412 lines of SQL and §39 records a 582x difference between two of its formulations. Rewriting it against an abstract API inside the process that owns the engine buys nothing and risks that number. **Under Q1 GOES this row is the one that changes**: the brief leaves and goes over the wire | rewriting the record store's queries; SQL crossing the socket |
| **6** | **The wire change of this build is `repeated string fields` on `RecordQueryRequest`.** Empty means every field, as today (§21 additive). Non-empty returns `id`, `version`, `kind`, `project` and the named fields only; `body` travels only when named | B108's largest measured cost. **The consumer exists today**: the MCP door's `record.query` and `rig record query` | a projection that changes the meaning of an old request |
| **7** | **`rig.store.list`, read-only:** every namespace on this estate with its file, bytes, schema version and whether it is ephemeral | §44: every service has a visual half, and *"a service whose data is declared correctly gets its view for free"*. This is the store browser M11 names, at the CLI tier. Nothing in `frontend/` is owed | a hand-written browser screen |
| **8** | **The program-facing `store.*` verbs are SPECIFIED below and NOT BUILT here.** They land with their first adopter | §5h: *"nothing here ships without a program that adopts it."* The planner is the intended first adopter (§43) and `shelf` is the review's proposal; **neither exists as an adopter today.** Building them now repeats §44's finding: storage offered to nobody | a verb set with zero callers; **and the deadlock in which S1 waits for the planner and the planner waits for S1** is broken by this row |
| **9** | **`store.watch` is not specified here.** It is the `events` bus from the storage side (B107) | §44's ordering test: rig uses no bus. **Deferred, not dropped** | a storage-specific notification path |
| **10** | **S1 spawns after B104's last commit, never beside it** | `internal/record/snapshot.go` is the backup seat's and reaches `Store.db`, which this build moves behind a handle. §45's open row, one BUILD at a time, has its first case here | two seats in one file |
| **11** | **The record's kinds, verbs and importers do not move in this build** | §43 Q1 is his. This section is drafted for STAYS and says where GOES bites (decision 5) | pre-empting his ruling with code |

---

### The program-facing API, in shape. NOT BUILT IN THIS SECTION

**Recorded here because B108's lesson has to be where the future designer
looks**, and because the planner's extraction specification needs the shape to
be written against. Every row is a sketch; the section that builds it decides.

| Verb | Carries | Pushed down, because B108 |
|---|---|---|
| `store.put` | collection, id, version (compare-and-swap), row | - |
| `store.get` | collection, ids (**many**, not one) | the 220 independent lookups |
| `store.query` | collection, `where` (field, op, value; a conjunction), **`fields`**, `order`, `limit`, **`aggregate`** (count; max or latest per group) | the 1.77 MiB; the per-group maximum |
| `store.delete` | collection, id, version | - |
| `store.transact` | a list of puts and deletes, applied whole or not at all | depth 2 in one round trip |

**A collection is declared, not created**: the `data` block of the declaration
carries the column schema (§5e), rig creates and migrates the table, and a
query naming an undeclared field is refused at the boundary like any other bad
argument. **Nothing in the shape names SQLite**, and that is the test each row
passes before it is built.

---

### The wire, this build

```proto
// appended to RecordQueryRequest, additive under section 21
repeated string fields = 5;   // empty: every field, as before

message StoreListRequest {}

message StoreNamespace {
  string name = 1;             // record | coord | programs/<id>
  string path = 2;
  uint64 bytes = 3;
  uint64 schema_version = 4;
  bool ephemeral = 5;
}

message StoreListResponse { repeated StoreNamespace namespaces = 1; }
```

**Declared in `internal/daemon/self.go`** through the `readOnly` helper.
**Dispatched** from `daemon.go`'s switch through one arm to `serveStoreList`
in `internal/daemon/store.go`. ⛔ **`ephemeral` is a bool and protojson drops a
false; the test that proves it travels owes the true case and the false case.**

---

### The commands

```
rig store list [--json]
    every namespace on this estate: name, path, bytes, schema version,
    ephemeral

rig record query ... [--fields title,status,...]
    the named fields only; the four fixed ones always travel
```

---

### Files the seat owns, to be written into `COORDINATION.md` before the first write

The seat is **`storage`**, a subagent. **These rows are PROPOSED; the lead
writes them into `COORDINATION.md` the turn he approves S1, and not before.**

| Path | New? | Note |
|---|---|---|
| `internal/store/` | **NEW** | open, WAL pragmas, the runner, namespace paths, `Ephemeral`. **The only package that imports `modernc.org/sqlite`** |
| `internal/record/store.go` | **grant: `Open`, `OpenScratch`, `open`, `start` and the runner** | the rest of the package, `brief.go` above all, is untouched. `SchemaVersion` and the steps stay here as the namespace's registration |
| `internal/record/snapshot.go` | **grant, the `db` reach only** | the backup seat's file, landed; edited only where the handle changes |
| `internal/coord/store.go`, `store_test.go` | **rewrite** | leases, epoch, CAS on the service; the `migrations` map becomes registered steps |
| `cmd/rigd/main.go` - the `coord.Open` call | **grant, one call site** | if the signature changes |
| `proto/rig/v1/wire.proto`, `wire.pb.go` | **grant: one field, two messages, appended** | `make proto`, committed with the handler that reads them |
| `internal/daemon/record.go` - the query handler | **grant, the `fields` branch** | the lead's file; nothing else in it moves |
| `internal/daemon/store.go`, `store_test.go` | **NEW** | `serveStoreList` |
| `internal/daemon/daemon.go`, `self.go` | **grant, one arm, one block** | |
| `cmd/rig/store.go`, `store_test.go` | **NEW** | `rig store list` |
| `cmd/rig/record.go` - the query verb's flags | **grant, one flag** | `--fields` |
| `cmd/rig/main.go`, `complete.go`, `testdata/exec/store-*.golden` | **entries yes, helpers no** | the `cli` seat's precedent |
| `go.mod`, `go.sum` - the `bbolt` lines REMOVED | **under `rig-makefile`**, committed with the last import's removal | §22's rows are the LEAD's |
| `size-ratchet.json` | **under `rig-makefile`, its own commit** | moved DOWN by the measured number, with the reason |

**Not the seat's:** `plan/`, `PLAN.md`, `frontend/`, `internal/kernel`,
`internal/wire`, `internal/backup`, the rest of `internal/record/` and
`internal/daemon/` and `cmd/rig/`, `packaging/`, and every logbook file outside
its agent-work directory.

---

### The tests owed, and the red control that proves each one bites

**The four-clause form, §37.** Every row's red control is run and written into
the seat's `FINDINGS.md` at a named sha.

| Test | Red control |
|---|---|
| the runner refuses a future `user_version` with `FutureSchemaError` | accept it |
| forward steps and the stamp are one transaction: a failing step leaves the version unchanged | stamp before the steps |
| coord compare-and-swap on the service: two writers, one loses, nothing interleaves | drop the version predicate |
| a lease expires on the service as it did on `bbolt`; the existing coord tests pass unchanged | shorten nothing; a coord test that had to change is a finding |
| the unnamed estate's store is ephemeral and reports it | persist it |
| `record.query` with `fields` returns the four fixed fields and the named ones only | return `body` unasked |
| `record.query` with no `fields` returns what it returned at `5a158a6`, byte for byte on a fixture | drop one field |
| `ephemeral` travels: true and false | protojson's dropped false |
| `store.list` names `record` and `coord` on a named estate, with the versions the files carry | hardcode the list |
| B104's restore acceptance (§46) passes at this build's final sha | skip it |
| `cmd/rig` links no SQLite and no `bbolt` | import either |

**One measurement is owed as evidence, on the B108 copy of production or a
fixture of the same shape:** the bytes of a `record.query` for rig's work items
with `fields=title,status` against the same query without, at a named sha.
**The number is reported; B108 predicts an order of magnitude.**

---

### The gates, before every commit

`make ci` 0, `make lint` 0, `make proto` idempotent, `rigseed --check`
unchanged from the sha before the seat's first commit, `make deps-check` 0
once the lead has moved §22's rows, `make bench-size` green after the ratchet
moves DOWN in its own commit.

---

### Acceptance, restated so it can be checked rather than read

1. **`git grep -n 'sql.Open\|bolt.Open' internal/ cmd/` prints lines in
   `internal/store/` only.** `git grep -n 'PRAGMA user_version'
   internal/record internal/coord` prints nothing. `go.mod` has no `bbolt`.
   §44's form: the two hand-rolled versions are DELETED.
2. **A `VACUUM INTO` copy of production opens under the new runner with no
   migration** (versions unchanged) **and `rig record query` returns the same
   head count as before.** B108's method; production itself is never opened.
3. **`rig store list` on a named estate** shows `record` and `coord`, their
   files and their schema versions; on an unnamed estate it shows the scratch
   store marked ephemeral.
4. **`rig record query --fields title,status` returns the named fields**, and
   the measured byte ratio against the unprojected query is reported.
5. **`rig backup` and `rig restore` (§46) pass their own acceptance at this
   build's final sha.**
6. **§22's `bbolt` row reads REMOVED with the measured bytes on `rigd`**, the
   SQLite row is unchanged, and `make deps-check` is 0.
7. `make ci` 0, `make lint` 0, `make proto` idempotent, `rigseed --check`
   unchanged.

---

### ⛔ OPEN, AND EACH IS HIS. PARKED WITH A RECOMMENDATION AND WITH WHAT PROCEEDS MEANWHILE

| Row | Recommendation | Proceeds meanwhile |
|---|---|---|
| **`bbolt` leaves and coord moves onto SQLite** (decision 1) | **yes.** One engine is what §44 owes, B28's search already ran, and the byte cost is a removal | nothing waits |
| **the `store.*` verbs are specified, not built, until an adopter exists** (decision 8) | **yes.** Building them now is §44's finding repeated. The planner's extraction is where the first adopter appears, and this row is what keeps S1 and the planner from waiting on each other | the shape table stands for the extraction specification to be written against |
| **§43 Q1** | **STAYS** (§43's recommendation table). This section is drafted for it; GOES changes decision 5 and nothing else in the code | B108 is done; the finding holds either way |
| **the wire pushdown in this build is `fields` alone;** multi-id refs and the aggregate wait for the brief to move | **agree.** `fields` has a consumer today; the other two do not | both are in the shape table. ✅ **multi-id `record.refs` BUILT 2026-09-24 at `b0da977`**, the brief having moved |
| **S1 spawns after B104's last commit** (decision 10) | **yes**, and it is the first case of §45's one-build-at-a-time recommendation | B104 finishes |

---

### What this section does not change

- **§44 S1 stands** and this is its build shape. **S5, the runner, is decision
  3.** The per-program namespace is decision 2; the migration before a program
  starts is M11's demo and arrives with the first program.
- **§46 stands.** `record.db` does not move, and `coord.db` stays out of the
  archive.
- **§39's design survives**: the nouns, the provenance, the grain, the
  compare-and-swap. Decision 6 adds a projection and changes no meaning.
- **B107, the event bus, is deferred and not deleted** (§43).
- **The tray acceptance test is untouched.**
- **The capability after B104 is his to choose**, §45. This section is
  preparation for that choice and nothing more.
