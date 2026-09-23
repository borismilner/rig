## 49. The logging specification

⛔ **DRAFT. STEP 1 AND A STEP-4 DRAFT OF §45's LOOP FOR B106 (S3), WRITTEN
2026-09-23 EVENING WITHOUT HIM, ON HIS WORD TO DO AS MUCH AS POSSIBLE ALONE.**
§45's 2026-09-23 block binds: *preparing is not approving*. **He has not
approved, reshaped or killed S3, and no subagent is spawned from this section
until he does.** Every row below is the seat's unless it cites a ruling, and a
row he reshapes is edited here, not argued in a handoff. **§44 puts S3 LAST of
the four, and this section does not move it**: it is written now so his
approval is one word when its turn comes.

> **In one line: `rigd` keeps one buffered, never-fsynced segment store per
> estate; its own `slog` records and every program's log records land in it,
> every dispatched call lands beside them with declared-sensitive fields
> blanked on the way in, and `rig logs` reads the whole estate back merged by
> time with its gaps shown.** rig's own daemon is the first consumer, and the
> hand-rolled stderr handler it replaces is deleted.

---

### Step 1. What it is, what a program does with it, what it costs

**What it is.** §5's `observe` kernel service, cut to the half §44 names:
**ingest, storage and the read side for LOGS.** Tracing and metrics stay in M5
and are not assumed in (§44's own ⛔). The **call log** of §15 IS in, for the
reason under decision 2. Programs never open a file and never see a segment;
they send records and ask questions.

**What a program does with it.**

| Program | Does | Gets |
|---|---|---|
| any program | points its logger at rig, one stream per connection, records batched | its logs merged with everyone else's, kept by §7's ladder, gone from its own stderr concerns |
| an agent Boris runs | `rig logs --since 1h --client shelf` | one time-ordered view across `rigd` and every program, with a **gap band** where records were lost or sampled (§15) |
| a person on the terminal | `rig logs -f` | the estate tailed live, journald's per-unit wall gone |
| the planner (§43) | `rig history --client X` when M5 lands | what a session's calls were and what came back, **minus** everything declared sensitive |

**What it costs.** Measured before this section, never estimated here:

| Line | Number | Source |
|---|---|---|
| a `slog` record today, stdlib handler | 627 ns | §8 |
| history append, one writer | 110.8 ns | §8, §15 |
| redaction per sensitive field, compiled | 82.5 ns | §15 |
| the whole recorded call, budget | **< 2 µs** (1757 ns measured) | §8, §15 |
| a verb round trip over the socket | 24.25 µs | B108, 2026-09-23 |
| a one-way socket write | 561 ns, 1.8M records/s | §4 |
| all history buffers, resident | **2 MB** | §15, §17 (cut from 8) |
| interning dictionary, resident | 512 KB | §15 |
| disk, default | 500 MB call log, separate audit budget | §15 |

**The one number that shapes the wire:** a record acknowledged per call would
cost 24.25 µs against a 2 µs budget, so **ingest is one-way by construction**
(decision 4). §4 already says logging needs no shared memory, and B108's
socket measurement agrees: batching, not memory, is the lever.

**The honest verdict.** Worth it on one condition the service review of
2026-09-19 already stated: *"Must beat journald on the merged view, or it is
a rewrite."* The merged, gap-labelled, redacted, per-principal view is what
journald cannot give a multi-process estate, and it is the acceptance test.
**It is also the only one of the four whose failure is a leak rather than a
gap** (§44), which is why it is last and why redaction is inside it, not
after it.

---

### What rig uses today, measured 2026-09-23 against `a51ec64`

| Thing | State | Where |
|---|---|---|
| logger call sites through one `*slog.Logger` | **21**: `cmd/rigd/main.go` 5, `daemon.go` 8, `mcp_listen.go` 5, `authorize.go` 2, `args.go` 1 | `grep -E '\.(Info\|Warn\|Error\|Debug)\('`, tests excluded |
| the handler | **one**, `slog.NewTextHandler(os.Stderr, ...)`, level from `--log-level` | `cmd/rigd/main.go:108` |
| the daemon's logger | `Config.Log *slog.Logger`; `New` defaults to `slog.DiscardHandler` | `internal/daemon/daemon.go:49`, `:217` |
| where it ends up | the journal. `packaging/rigd.service` sets no `StandardOutput`, so stderr is swept up per unit | `packaging/rigd.service:121` |
| `sensitive` declaration | **plumbed, enforced nowhere.** `Command.sensitive = 7` is copied at registration; rig's own verbs declare `[]string{}` | `wire.proto`, `registration.go:156`, `self.go` |
| compiled redaction spans, history, segments, coverage log | ⛔ **none exists** | `git grep -i 'redact\|segment\|coverage' internal/` returns no code |
| a one-way frame | ⛔ **none.** Kinds are REQUEST, RESPONSE, STREAM_DATA, STREAM_END, ERROR, CANCEL | `wire.proto` `FrameKind` |
| an interactive stream handler | ⛔ **none.** `SHAPE_INTERACTIVE_STREAM` is declared and mapped; the daemon reads STREAM_DATA and dispatches it to nothing | `registration.go:58`, `frame.go:157` |
| the `introspect` predicate | **built.** Starts true, `hello` turns it off for a program | `principal.go:51`, `:88` |
| a logs directory | ⛔ none. `paths` has `StateDir`, `EstateStateDir`, `EstateScratchDir` | `internal/paths/paths.go` |
| zstd | ⛔ not a dependency | `go.mod` |

**B106's row says 34 `slog.` sites, counted 2026-09-19 by a different grep.**
The 21 above count method calls on the logger, which is what a handler swap
touches; both are one handler away from rig, and that is the fact that matters.

### ⛔ S3 WAITS ON NOTHING THAT IS NOT BUILT

- **Not on S1.** §15 makes segments FILES by locked design: ring, `write(2)`,
  no fsync, zstd at rotation. **§48's one-engine rule governs the namespaces
  that are tables**; the history was carved out of it before §48 was written
  and this section says so where §48 does not. A log in SQLite would fsync at
  every checkpoint and lose the whole reason it is affordable.
- **Not on S4.** S3's knobs are §6 keys the day S4 lands and constants until
  then (decision 12). `log.level` is S4's first consumer and S3 reads the same
  `slog.LevelVar`.
- **Not on tracing, metrics or the bus.** §44's ⛔ stands.

---

### Design. Sixteen decisions, every one the seat's

| # | Decision | Reason |
|---|---|---|
| **1** | **One segment store per estate, under `<estate state dir>/logs/`**: `segments/`, `archive/`, `coverage.log`, `pins`. An unnamed estate (`d.estate == ""`) keeps the ring only and writes no file | §7 storage is per estate; §46 decision 11 already refuses the unnamed estate a file it cannot name |
| **2** | **Two record kinds share the store: LOG records and CALL records.** The call record is written at dispatch with the compiled span list applied | §44's acceptance names *a declared-sensitive field*; the only declaration in rig is on a `Command`, so the third clause is untestable unless dispatched calls are recorded. One writer, one ring, one ladder for both |
| **3** | **`rigd`'s own records enter in-process**: a `slog.Handler` that appends to the ring. No socket, no encoding to the wire | the first consumer pays 110.8 ns, not 24.25 µs; §8's 627 ns is the ceiling to beat |
| **4** | **Program ingest is ONE-WAY on an existing shape**: `rig.logs.ingest` opens a `SHAPE_INTERACTIVE_STREAM`; each client `STREAM_DATA` frame carries a `LogBatch`; the daemon never answers a batch; `STREAM_END` closes. **No new frame kind** | a new `FrameKind` is a wire-major change and §5's conformance suite freezes the vocabulary; the interactive shape exists and has waited for its first handler. A batch amortises the 561 ns write and B108's 24.25 µs RT never occurs |
| **5** | **§15's ring, verbatim**: sharded per client, flush armed by the FIRST record then 250 ms or 256 KB, `write(2)`, never fsync, 2 MB total | locked by §2 and §7; the 250 ms ticker measured 45.5 wakeups/s against §17's budget of one, so armed, not ticking |
| **6** | **Interned, fixed-width records; the id→name dictionary is written INTO the segment header at rotation**; client ids interned per principal | §15: a segment decodes standalone, so deleting the index destroys nothing and a rename coexists with its old spelling |
| **7** | **A payload beyond an inline cap goes to the segment's blob area by offset**; the cap is `logs.call.payload_cap`, default 4096 B, and the full size is always recorded | B108's `project.brief` answer is 195,275 B; a fixed-width row cannot hold it and a 2 MB ring cannot afford ten of them |
| **8** | **Redaction is compiled at registration per `(method, wire version)` and applied at the ring append, never at read.** The compiler turns each JSON pointer into a matcher over the payload's encoding that finds the value's byte span without materialising the payload; a command whose payload has no pointer space takes §15's option 2 (wholly sensitive: size, age, owner, never contents) | §15's invariant, stated as write-side; 82.5 ns compiled against 3100 ns decoded. **The seat measures its compiler against < 100 ns per field, and a form that misses the budget falls to option 2 for that command, recorded in the coverage log** |
| **9** | **Anything `secrets.*` returns is never recorded, only the key name**, before any declaration is consulted | §15: a field not declared sensitive IS recorded, so the default for a secret cannot be left to a declaration |
| **10** | **The coverage log is in S3, not after it**: `(from_ts, to_ts, client or *, cause, retained/total)` appended on the cold path for sampling, ring overwrite, segment unlink and unclean close. **`rig logs` renders a gap band and refuses "every record" without stating coverage** | §15: without it four loss paths are indistinguishable from "nothing happened" |
| **11** | **Retention is §7's ladder over §15's three ceilings, in this order: per-client RATE ceiling, then SIZE, then AGE.** Compress ONCE at rotation at maximal force; at one month the closed segment MOVES to `archive/`; at three months it is unlinked **unless pinned**. `rig logs pin <segment>` is the minimal pin | one noisy client cannot evict the estate's record (§15); compressing twice buys nothing, and §7's "maximal force" is met at rotation. **§7 calls the pin a whole capability; S3 ships the file-level form and the query-level form waits for his word** |
| **12** | **Knobs are constants until S4 lands, then §6 keys under `logs.`**: `buffer.bytes` 2 MiB, `flush.ms` 250, `flush.bytes` 262144, `retention.bytes` 500 MiB, `call.payload_cap` 4096, `archive.after` 30d, `delete.after` 90d | §15 and §7 fix the defaults; S3 must not wait on S4 to exist |
| **13** | **`rig logs` reads through the principal filter**: a program sees its own records; a holder of `introspect` sees every client's. Opening another client's records writes a coalesced audit entry, one per principal per view per minute | §14 and §15, both built or ruled; the predicate exists in `principal.go` |
| **14** | **`rigd` tees WARN and above to stderr until the store has opened, and nothing after** | a crash before the segments exist must still reach the journal; after that, a second copy is the hand-rolled version §44 says gets deleted |
| **15** | **The program-side `slog.Handler` is SPECIFIED here and built when a program adopts it**; the ingest verb IS built, exercised by a second connection in `rigd`'s own tests | §48's rule for `store.*`: no adopter, no build; the verb has an adopter (the test) and the handler has none |
| **16** | **Segments are NOT in `rig backup`** | §46 archives `record.db`; the history ages out by design and 500 MB in every archive is the wrong trade. His to reverse |

### The library rule (§38), applied before a line is written

| Need | Candidates searched | Call |
|---|---|---|
| structured logging | `log/slog` | **stdlib, already in use.** §22 |
| zstd at rotation | `github.com/klauspost/compress/zstd` (pure Go); `github.com/DataDog/zstd` (cgo); `github.com/valyala/gozstd` (cgo) | **klauspost.** §22 keeps the daemon cgo-free. **The seat measures its cost against §17's RSS rungs before merging** |
| the segment format | `parquet-go/parquet-go`, Apache Arrow Go, `prometheus/tsdb` chunks, `nakabonne/tstorage` | **none.** §15's layout (dictionary in the segment, rebuildable index, fixed-width interned rows, coverage beside) is what every candidate lacks as-is, and each brings a dependency tree §17 cannot carry. **This is the one place S3 writes its own encoding, and this row is the search that permits it** |
| JSON pointer | `internal/` already resolves pointers for §5e declarations, or `github.com/santhosh-tekuri/jsonschema/v6` which is ADOPTED and carries one | **reuse the adopted one** |
| a log query language | none searched: the read side is filters, not a language | `--since --until --client --level --grep`, and `--json` |

---

### Wire. Three verbs, no new frame kind

```proto
// rig.logs.ingest - SHAPE_INTERACTIVE_STREAM. The REQUEST carries LogsIngestOpen;
// every client STREAM_DATA carries one LogBatch; the daemon sends nothing until
// STREAM_END or an ERROR. A batch is never acknowledged.
message LogsIngestOpen  { string logger = 1; }         // free text, interned
message LogRecord {
  int64  unix_nanos = 1;
  int32  level      = 2;                                // slog.Level as int
  string message    = 3;
  map<string,string> attrs = 4;                         // flattened, values as text
}
message LogBatch       { repeated LogRecord records = 1; uint32 dropped_before = 2; }

// rig.logs.query - SHAPE_STREAM. Records stream back time-ordered, gap bands
// as GapBand frames in sequence, then STREAM_END. Filtered by the caller's
// principal unless it holds introspect.
message LogsQueryRequest {
  int64  since_unix_nanos = 1;  int64 until_unix_nanos = 2;
  repeated string clients = 3;  int32 min_level = 4;
  string grep = 5;              bool follow = 6;       uint32 limit = 7;
}
message GapBand { int64 from = 1; int64 to = 2; string client = 3;
                  string cause = 4; uint64 retained = 5; uint64 total = 6; }
message LogsQueryFrame { oneof item { LogRecord record = 1; GapBand gap = 2; } string client = 3; }

// rig.logs.coverage - SHAPE_UNARY. The coverage log for a range, so a view can
// state what it does not know before it answers.
message LogsCoverageRequest  { int64 since_unix_nanos = 1; int64 until_unix_nanos = 2; }
message LogsCoverageResponse { repeated GapBand gaps = 1; uint64 segments = 2; uint64 bytes = 3; }
```

**Call records have no ingest verb**: they are written by dispatch itself
(decision 2) and read by §15's `rig history`, which is M5's and NOT built here.
S3 writes the record so that M5 has something to read; the read command is a
row in the open table.

**MCP promotion.** `rig.logs.query` is promoted: *what did shelf log in the
last hour* is a question an agent asks. `ingest` and `coverage` are not.

### Commands

| Command | Does |
|---|---|
| `rig logs [--since D] [--until T] [--client X ...] [--level L] [--grep RE] [--json]` | the merged view, time-ordered, one line per record with time, client, level, message, attrs. **Every gap renders as a band line** |
| `rig logs -f` | follow; a `LogsQueryRequest{follow:true}` that never ends |
| `rig logs coverage [--since D]` | the gaps and the segment count for the range |
| `rig logs pin <segment>` / `unpin` | §7's minimal pin |

---

### Files for seat `logging`, so `COORDINATION.md` has a row to copy

| Path | Grant |
|---|---|
| `internal/observe/` | **NEW, owns it.** `ring.go`, `segment.go`, `intern.go`, `writer.go`, `redact.go`, `coverage.go`, `retention.go`, `query.go`, `handler.go` (the `slog.Handler`), tests |
| `internal/daemon/observe.go` | **NEW.** The three verbs and the interactive-stream handler, which is the daemon's first |
| `internal/daemon/daemon.go` | **grant, two hooks**: the call record at `dispatch` (`:484`), the store opened after `recordStore` and before `serve` |
| `internal/daemon/self.go` | **grant**: declare the three verbs, `sensitive` empty |
| `proto/rig/v1/wire.proto`, `wire.pb.go` | **grant**: the messages above, appended, no renumbering. **Wait for B104's last commit: the backup seat holds this file today** |
| `internal/paths/paths.go` | **grant**: `EstateLogsDir(name)`. Same wait |
| `cmd/rigd/main.go` | **grant**: the handler swap at `:108` and decision 14's tee |
| `cmd/rig/logs.go` | **NEW** |
| `cmd/rig/layering_test.go` | **grant**: only `rigd` links the writer; the CLI links the query client |
| `go.mod`, `go.sum` | klauspost zstd, **under `rig-makefile`** |
| `plan/49-*.md` | corrections come back as findings, as §46's F1 did; the seat does not edit the plan |

### Tests, each with its red control

| Test | Passes when | Red control (must FAIL the assertion) |
|---|---|---|
| **the leak** | a fake program declares `sensitive: ["/token"]`, is called with a known token, the ring is flushed; `grep -r` over `logs/` finds the token in **zero** files | same call, `sensitive: []`: the token IS found, so the grep can see what it looks for |
| **the merged view** | two clients with interleaved timestamps; `rig logs` output is time-ordered with the client column | one client's clock skewed: ordering follows the record's time, not arrival |
| **the gap** | sampling forced on; `rig logs` prints a band with `retained/total`; `--json` carries it | coverage log removed: the command refuses to answer rather than answering clean |
| **standalone decode** | the index deleted; a segment decodes to identical output | header stripped: decode fails loudly, not with the wrong names |
| **the crash bound** | `SIGKILL` mid-burst; records lost are under one second's worth | fsync inserted: the append benchmark blows its budget, proving the test measures the trade |
| **the ceilings** | one client at 100k records/s cannot evict a quiet client's segments | rate ceiling disabled: the quiet client's segment is unlinked |
| **the budget** | `BenchmarkAppend` p99 at 16 clients < 200 ns; handler record < 627 ns; redaction < 100 ns per field | `make bench-idle` fails the build on a miss, as §15 requires |
| **the tee** | before the store opens a WARN reaches stderr; after, `rigd`'s stderr is silent under load | store open removed: stderr fills, so the tee is measured and not assumed |

### Acceptance, checkable by `git grep` and not by reading (§44's form)

1. `git grep 'slog.NewTextHandler' cmd/rigd` returns **nothing** after the
   store has opened; the hand-rolled handler is deleted, not wrapped.
2. `rigd`'s own records: `rig logs --client rigd --since 1m` prints the
   daemon's startup lines read back from a segment, not from stderr.
3. **The leak test above passes and its red control fails**, on the same
   binary, in `make ci`.
4. `rig logs` across `rigd` plus one test program is time-ordered with a client
   column and a gap band, which journald cannot render for this estate.
5. Every ceiling in §15's budget table is asserted in `make bench-idle`.
6. **Nothing in `internal/observe` imports SQLite, bbolt or the wire**: the
   store is files and the daemon is its only writer.

### Gates before a subagent is spawned

- **His step 3**, batched with S1 and S4 (§45's 2026-09-23 block).
- **B104's last commit**: `wire.proto` and `paths.go` are the backup seat's
  today, and §45's recommendation is one BUILD at a time on the shared tree.
- **S1 and S4 first, in §44's order.** S3 is last and stays last.

### Open rows. His, each with a recommendation

| Row | Recommendation | If he rules the other way |
|---|---|---|
| **is the CALL record in S3?** | **YES** (decision 2): the acceptance's third clause needs it and the writer is shared | S3 shrinks to log records plus `rig logs`; the leak clause is deleted from §44's acceptance, and M5 writes the call log later on the same store |
| **compress once at rotation, or again at one month?** | **once, maximal, at rotation** (decision 11) | a second pass at one month costs CPU for bytes already at the floor; the seat measures the difference before arguing |
| **the stderr tee** | **WARN+ until the store opens, nothing after** (decision 14) | keeping a full tee means the journal and rig disagree on what was logged, and §44's delete clause is not met |
| **segments in `rig backup`?** | **no** (decision 16) | §46 grows a second member and every archive carries up to 500 MB |
| **`rig history` in S3?** | **no**: the record is written here, the reader is M5's | the reader is one query over the same segments; if he wants it now it is one more command, not a design change |
| **the pin** | **file-level `rig logs pin <segment>`** now; §7's full capability later | the query-level pin needs a pinned-range index the segment format does not carry yet |
