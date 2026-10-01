## 48. The storage specification

### ⛔ RULED BY BORIS, 2026-09-26: rig supports `store.*`, and owns the queries

⛔ **R5, R6 and R8-R10 ARE SUPERSEDED BY HIS THIRD RULING THE SAME DAY**
("two storage ways", below): there is no background dump and the database is
not in git. R1-R4, R7 and R11-R17 stand as amended there.

**Boris, verbatim:** *"We need to support `store.*`. Programs such as graft
need to store and fetch pieces of information. The program calls rig.
programs don't need to know what engine they use (it is rig's responsibility
to offer anything programs need at the best possible way). The cost of a query
is not that critical for tasks that run once in a few minutes or even hours
sometimes. Rig owns all of that, plus the queries. On top of DB storage (which
is the main storage being used) the data should be dumped in the background
into a well structured filesystem that is well separated among the different
programs and this whole filesystem is to be Git managed with auto commits by
`rig` logic."*

**The requirements, binding:**

| # | Requirement |
|---|---|
| R1 | **rig builds and serves the program-facing `store.*` verbs.** A program stores and fetches through rig; it never opens a database itself |
| R2 | **Engine-agnostic.** No program names or sees the engine; choosing and running it well is rig's responsibility |
| R3 | **rig owns everything about a program's data, the queries included** |
| R4 | **The database is the main storage.** Reads and writes go to it |
| R5 | **rig dumps every program's data, in the background, into a well structured filesystem**, one tree per program, cleanly separated from every other program's |
| R6 | **That whole filesystem is a git repository**, and rig commits to it automatically, by its own logic |
| R7 | **Query cost is not the design driver.** Callers run once in minutes or hours; clarity and correctness come before the microseconds B108 measured |

**What this overrules below:** decision 8's *"specified, NOT BUILT"* for the
`store.*` verbs (graft is the adopter), and the DRAFT banner for that half.
**It supersedes plan/07's "rig owns everything about a database except what is
in it" table**, where the program opened the file and owned its queries.

**Four of the open rows ANSWERED by Boris, 2026-09-26** (put to him as
choices; the answer column is his pick):

| # | Question | His answer |
|---|---|---|
| R8 | Layout of a program's tree | **One file per collection**: `<program>/<collection>.<ext>`. He chose it over one file per row, which a seat recommended |
| R9 | Commit cadence | **Batched on an interval**: one commit per program per interval, and only when something changed. Never one commit per write |
| R10 | Rebuild a database from the tree | **Yes, a real restore path.** The tree is a second full copy, so SQLite never holds the only copy (plan/07's rule is met this way) |
| R11 | A declared-sensitive field | **Written as-is.** The tree is a full copy, secrets included; nothing is redacted from it |

⛔ **R11 makes the tree as secret as the database.** A seat recommended
leaving sensitive fields out and he chose otherwise, so it holds. What follows
from it, for whoever builds this: the tree's file modes match the store's
(owner only), and **no rig logic pushes it anywhere** without his ruling.

### ⛔ RULED BY BORIS SECOND, 2026-09-26, later the same day: each program's own free tree, and one configurable root

**Boris, verbatim:** *"Each program should have its own filesystem under that
filesystem that I've mentioned, into which it can write freely and read
freely, these writes and reads do not have to go through rig but rig is the one
holding the pointer to the root folder for each of its users (programs) so that
I/we can control its exact placement on the disk; By the way, by default, the
files as well as DB files are to be sored in a proper path under ~/.rig but this
path must be configurable so that we can move it to arbitraty other place. The
separation between `rig` internal storage such as the DB files and
configurations and so on, and the freely used files I've mentioned here is to be
well separated, and these files are too to be comitted from time to time by
`rig`."*

| # | Requirement |
|---|---|
| R12 | **Every program has its own directory inside the git-managed filesystem**, which it reads and writes freely and directly, **not through rig** |
| R13 | **rig holds the pointer to each program's root folder**: a program asks rig where its directory is, and never computes or hard-codes a path. That is what lets Boris decide the exact placement on disk |
| R14 | **By default, rig keeps everything under `~/.rig`**: the database files and the program files both |
| R15 | **That root is configurable**, so everything can move to any other place |
| R16 | **rig's internal storage (database files, configuration and the like) is well separated from the programs' free files** |
| R17 | **rig commits the free files too, from time to time**, as it does the store's dump (R6, R9) |

⛔ **R14 SUPERSEDES THE DEFAULT IN plan/37 PRECONDITION 2**, which put estate
state under `$XDG_STATE_HOME/rig/estates/<name>/` (today
`~/.local/state/rig/estates/production/record.db`). **The live production store
is at the old path**, so moving the default owes a migration of his data, done
with a snapshot first (plan/46), never a silent re-open of an empty store.

**Open, each with a seat's recommendation:**

| Question | Recommendation |
|---|---|
| Two estates (production, development) under one root | one subtree per estate, `~/.rig/estates/<name>/`, since plan/37 forbids them sharing state |
| How the root is configured | one setting, read by `rigd` at start (a flag and an environment variable until plan/47's configuration service exists), reported by `rig estate` |
| Where a program's dump and its free files sit relative to each other | side by side in the program's directory but in separate subdirectories, so a program writing freely can never overwrite the dump rig writes |
| One git repository for all programs, or one per program | one per estate, as he said "this whole filesystem"; each program a top-level directory in it |

### ⛔ RULED BY BORIS THIRD, 2026-09-26: two storage ways, and export instead of a dump

**Boris, verbatim:** *"Wait so lets do it smarter. Two storage ways: 1) DB 2)
Freely accessible text files. No need to duplicate the DB content into files but
they must be exportable to files. The DB is not to be Git managed, but their
contents can be exported into textual representation in to a different
filesystem structure than the freely accessible text files."*

**The model now, binding:**

| # | Storage way | Rule |
|---|---|---|
| R18 | **1. The database** | a program stores and fetches through `store.*` (R1-R4). **Not git-managed. Not duplicated into files in the background** |
| R19 | **1a. Its export** | the database's contents **must be exportable** to a textual representation, **into a filesystem structure separate from the free files** |
| R20 | **2. Free text files** | each program's own directory, read and written directly (R12), its root held by rig (R13), committed by rig from time to time (R17) |

**What each earlier row becomes:**

| Row | Now |
|---|---|
| R5 background dump, R6 git over it | **gone**: replaced by an export (R19), and the database is out of git |
| R8 one file per collection | **carried to the export's layout**, where it still reads as his choice |
| R9 batched commits on an interval | **carried to the free files** (R17), the only git-managed tree left |
| R10 restore from the tree | **open again**: whether an export can be imported back is a new question |
| R11 sensitive fields as-is | **carried to the export**; the free files hold whatever the program writes |
| R14-R16 | stand: one configurable root, default `~/.rig`, internal storage separated from free files. The export is a third, separate area |

**His answers the same day, verbatim:** *"Exported contents are also git
managed and when exported they are also auto comitted. Exports happen ad-hoc
when the user chooses. The export can be loaded back in."*

| # | Requirement |
|---|---|
| R21 | **The export area is git-managed**, and rig commits an export automatically when it is written |
| R22 | **An export happens ad hoc, when the user chooses**: never on a timer, never in the background |
| R23 | **An export can be loaded back in.** Export and import together are a restore path (R10 answered yes again, for exports) |

**Answered 2026-09-27, put to him as choices:** R35, the export format is
**JSON Lines, one document per line, sorted by id**; R36, `files/` and
`exports/` are **two separate git repositories**.

**Still open:** where the tree lives (recommend under the estate's state
directory, `programs/<id>/`), the interval's length, the file format inside a
collection file (recommend JSON Lines sorted by id, so a one-row change is a
one-line diff), and whether it is ever pushed.

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

### ⛔ Boris, 2026-09-26: the shared lessons are a free folder too

**Boris, verbatim:** *"Remember we talked about lessons that are shared among all agents/programs
that they can write? We can implement it simply as another such free-accessed
folder that everybody can ask to write to or read from."*

| # | Requirement |
|---|---|
| R24 | **One shared free folder under `files/`**, beside the per-program ones, that **every program and agent may ask rig for and then read and write directly.** The shared lessons (plan/40) live there. rig commits it like the rest of `files/` |

plan/40 carries the tension with its index requirement and a seat's
recommendation for it.

**Boris, the same day, verbatim:** *"Keep the index on top of the folder - exactly right! Everything agents write
into the freely-accessible-filesystem must be managed (by those same agents) in
the rig index/knowledge-base, this way it can be easily accessible to them and
maybe others later without reading everything every time."*

| # | Requirement |
|---|---|
| R25 | **The index sits on top of the folder.** The files are the source; rig's search index is built from them, and a search answers a snippet, never the whole body (plan/40's rule, kept) |
| R26 | **Everything an agent writes into the free filesystem is entered in rig's index by that same agent**, per-program folders and the shared folder alike, so it can be found later, by that agent or others, without reading everything |

| R27 | **Agents link any two items of knowledge with a reason for the link** (records, lessons, indexed files), making a knowledge graph. Specified in plan/39 G4 |

**A seat's recommendation for R26, not his ruling:** the writer calls one verb
after writing (path, title, one-line summary, tags); rig indexes the file's
text under that entry. rig also lists, at each commit, every file with no entry
or an entry older than the file, so a missed registration is visible rather
than silently unfindable.

### ⛔ Boris on the free-file layout, 2026-09-26: rig controls the layout of the free files

**Boris, verbatim:** *"`rig` also controls the structure where the agents/programs save their
freely written files. There can be different files, such as agent
documentation about something or it can be resources for something (such as
downloaded files / images / PDFs / Code and so on)... When agent wants to save
such files it consults `rig` so that everyone is clear about where files go and
where they can be accessible from. This should allow me as the author and the
user of `rig` to control the layout of things and change things with time
easily."*

| # | Requirement |
|---|---|
| R28 | **rig owns the structure inside `files/`.** Files come in kinds - an agent's documentation about something, and resources for something (downloads, images, PDFs, code, and so on) |
| R29 | **An agent or program that wants to save a file asks rig where it goes**, and writes it there. So everyone knows where files go and where to find them. The write itself stays direct (R12) |
| R30 | **Boris controls the layout and can change it over time, easily.** The layout is his to edit, not compiled into rig or into any program |

**A seat's recommendation, not his ruling:** the layout is one text file under
`internal/` that Boris edits, mapping each kind to a place (for example
`docs/<subject>/`, `resources/<type>/<subject>/`); `files.place` answers a path
from kind, subject and file name; a layout change is applied by rig, which
moves the existing files (`git mv`, so history follows) and rewrites their
index entries, so nothing a search or a link points at breaks.

**Answered by Boris the same day** (put to him as choices):

| # | Question | His answer |
|---|---|---|
| R31 | Top level of `files/` | **By kind, shared**: `docs/<subject>/`, `resources/<type>/<subject>/`, `lessons/`, and `programs/<id>/` for a program's private working files. The writer is recorded in the index, not in the path |
| R32 | Big binary files | **Verbatim:** *"Binary files we'll find a better way to backup - no need to manage them in Git."* **Binary files are never committed**; they are stored and indexed. Their backup is a separate, later question |
| R33 | A kind the layout does not name | **Refused, with the list of kinds that exist.** Only Boris adds kinds |
| R34 | Commit cadence of the free files | **Boris, 2026-09-26, verbatim:** *"Git should not commit more often than once per 5 minutes where this time iterval is configurable like all other aspects of rig should be."* **Default 5 minutes; never two commits closer together than the interval**, on a tick, across a restart or on shutdown. rigd `--files-commit-every` until plan/47's resolver carries it |

**R32 as built, a seat's reading:** "binary" is decided by content, as git
itself does (a NUL byte in the first 8,000 bytes), not by extension; rig's
committer stages text files only, and `files/.gitignore` is not relied on,
because a program can write one.

**R32 supersedes R17 and R20 for binaries**: those rows committed everything
under `files/`; now it is text only.

### P9 build specification: `store.*`, free files, export and import

⛔ **DRAFT, written 2026-09-26 from R1-R23. The rulings above are his; every
"recommend" and every D-row below is a seat's until he confirms it.** It
replaces "The program-facing API, in shape" further down, which was a sketch.

**The three areas under one root** (R14-R16, R18-R21):

```
<root>/estates/<name>/            root: --root, else RIG_ROOT, else ~/.rig
├─ internal/                      rig only, never in git
│  └─ programs/<id>.db            store.* database, one per program
├─ files/                         git repository; programs write directly
│  └─ <id>/                       rig commits on an interval (R17, R9)
└─ exports/                       git repository; ad hoc, auto-committed
   └─ <id>/<collection>.jsonl     (R19, R21-R23, R8)
```

#### Decisions, each a seat's recommendation until he confirms

| # | Decision | Why |
|---|---|---|
| D1 | **A collection holds JSON documents keyed by id**, created on first write. No declared schema in this build | graft can adopt without a declaration format that does not exist yet (§5e's `data` block is unbuilt). A declared schema can be added later without changing a caller |
| D2 | **A program's namespace is taken from its connection**, never from an argument. A terminal or an agent names the program explicitly | a program can never read or write another program's data. The terminal and the agent door are Boris and his seats, who may inspect anything, as with every other verb |
| D3 | **Every write carries a version and is compare-and-swap**, as `record.put` already is | two graft workers cannot silently overwrite each other |
| D4 | **No caller string reaches SQL text.** Field paths and values are bound parameters (`json_extract(doc, ?)`); collection names and ids match `[a-z0-9][a-z0-9._-]{0,63}` | §38's safe-code rule. The same pattern keeps an export path inside `exports/<id>/` |
| D5 | **git is driven by exec'ing the system `git`** with a fixed argv, `-c core.hooksPath=/dev/null`, rig as the author, never a shell | §38 search: `go-git/go-git` (pure Go) and the `git` binary. go-git adds several MB to `rigd` for four commands; exec is already the pattern in `cmd/tagcheck`. A missing `git` is a clear refusal, not a crash |
| D6 | **The root is a flag and an environment variable** until plan/47's configuration service exists, and `rig estate` reports it | R15 needs it now; S4 is deferred |
| D6a | **One daemon per storage root.** rigd locks `<estate areas>/internal/rigd.lock` before it opens anything, and refuses with exit 8 when another daemon holds it; a second service (`rig-team.service`) needs its own `RIG_ROOT` | added 2026-09-26 (rig `f7f73bd`): the estate claim is keyed by the state home, so two services both won it and both defaulted to `~/.rig`, one git repository and one index |
| D7 | **The existing `record.db` and `coord.db` do NOT move in this build.** Moving his live store to `~/.rig` is its own step, taken with his go and a snapshot first | R14's default binds the new areas at once; his data moves only when he says so |
| D7a | **A root that is GIVEN moves the estate's databases too.** With `--root` or `RIG_ROOT` set and `XDG_STATE_HOME` unset, `record.db`, `coord.db`, `sound.json` and the backups go to `<root>/state/`. `XDG_STATE_HOME`, when set, still names them; with neither, the old path stands (D7). The name claim does not move | Boris, 2026-10-01: *"Fix the isolation gap in rig"*. A scratch rigd with `RIG_ROOT=/tmp/...` and `--estate=development` wrote 400 records into his real development estate, because R15's one setting moved the areas and not the databases. Production sets neither variable and `rig-team` sets both, so no live store moves |
| D8 | **An import replaces the named collections whole**, after a snapshot of the program's database | a merge of a stale export into live data is the silent-loss case |

#### Verbs (each one is an MCP tool by the P8 test; no exclusions)

| Verb | Effects | Carries |
|---|---|---|
| `store.put` | write | collection, id, expected version (0 = create), document |
| `store.get` | read | collection, **many** ids |
| `store.query` | read | collection, `where` (field, op, value; AND only), `fields`, `order`, `limit`, `count_only` |
| `store.delete` | destructive | collection, id, expected version |
| `store.transact` | write | puts and deletes, all or none |
| `store.collections` | read | the program's collections, row counts, bytes |
| `store.export` | write | program, collections (empty = all); writes JSONL sorted by id, commits |
| `store.import` | destructive | program, collections; snapshot, replace, report counts |
| `files.root` | read | the caller's own directory under `files/`, or the shared one, created if missing |
| `files.place` | read | kind, subject, file name; answers where the file goes under the current layout (R29) |
| `files.layout` | read | the layout in force: every kind and its place (R30) |
| `files.relayout` | write | apply an edited layout: move files with `git mv`, rewrite their index entries, one commit (R30) |
| `files.index` | write | path under `files/`, title, one-line summary, tags; rig indexes the file's text (R26) |
| `files.search` | read | words; answers path, title, summary, snippet, score, never a body (R25) |
| `files.unindexed` | read | files with no index entry, or an entry older than the file |

CLI: `rig store {put,get,query,delete,collections,export,import}` and
`rig files root`.

#### Slices, each committed and gated on its own

| # | Slice | Done when |
|---|---|---|
| 1 | `internal/paths`: root resolution and the three areas; unnamed estate gets a temporary root | `rig estate` prints the root; `RIG_ROOT` and `--root` both move it |
| 2 | `internal/store`: open, migrations, documents, CAS, query | unit tests below pass |
| 3 | wire, daemon arms, CLI and MCP for the store verbs | a program and an agent both put and query; a program cannot reach another's collection |
| 4 | `files/`: `files.root`, git init, the interval committer | a file a program writes appears in a rig commit within one interval |
| 4b | the index over `files/` (R25, R26): `files.index`, `files.search`, `files.unindexed`; `knowledge.*` becomes a view of the shared lessons folder, and the lessons now in the database move there once | a file written and indexed is found by a search that returns no body; an unindexed file is listed |
| 5 | export and import | round trip: export, delete rows, import, identical documents and versions |
| 6 | `examples/storeworker`: a fake adopter standing in for graft. **R37, Boris 2026-09-27, verbatim:** *"make sure the fake program showcases as many of the features of rig as possible and that it interacts with me as the user."* | demonstrated live on a private estate: stores, queries, writes a file, is exported and re-imported; **and it uses as many of rig's program-facing features as it can, and Boris drives it and is asked by it, as its user. **R38, Boris 2026-09-27, verbatim:** *"For each part of the fake program GUI there should be something I can click or hover or something that explains shortly what part of rig it showcases."* So it has a GUI in the window, and every part of it carries a short explanation of the rig feature behind it. **R39, Boris 2026-09-27, verbatim, REPLACES R38's hover-or-click form:** *"Lets make the fake application to have tabbed GUI, each tab explains what it showcases and lets the user (me) interact to experience what is being showcases. That is instead of having to hover or click anything; This way it is much more straightforward."* So one tab per rig feature, its explanation shown on the tab without any hover or click, and controls on the same tab to try it. **R40, Boris 2026-09-27, verbatim:** *"make sure the storeworker explains all the terms used wherever they are applicable, things such estage, seat, lease and all that so that I can understand all of rig functionality, capabilities and features through it."* ("estage" is estate.) So every rig term a tab uses is explained on that tab, in plain view as R39 asks, and one place lists every term; the parts of rig a program cannot showcase are named there too, so the page covers all of rig. **R41, Boris 2026-09-27, verbatim:** *"When all finished, adjust the storeworker to showcase accordingly."* After toast replies (plan/12), the knowledge-base name (plan/40) and the capital R (plan/30), storeworker shows each of them: a run over budget asks through a reply toast, the Toasts tab sends one with buttons and one with free text. **And the Leases tab he reported as strange (2026-09-27, screenshot: the gpu lease ORPHANED at -176 s, the worker waiting forever): a hold that ends must release the lease, because rig never frees an expired lease whose holder is alive** |

**Slices 1-3 as built, 2026-09-26** (rig `125f74c`, `ea3267d`, `c49441e`).
Seat choices made while building, each his to overrule:

| Choice | Why |
|---|---|
| `op` is a string (`eq`..`ge`), a value is JSON text | an agent on the MCP door writes `"eq"` and `"done"`, not an enum name or base64 |
| `offset` added to `store.query` | a page cut by the frame budget (768 KiB) says `more`, and the caller needs a way to the next one |
| an unscoped caller (terminal, agent) must name `program`; a program naming another is DENIED | D2 as specified |
| a terminal's or agent's READ of a program that never wrote is NOT_FOUND and creates nothing | a typo on a read would otherwise leave an empty database under the misspelled name |
| an unnamed estate serves the store under its scratch root | the demo and the tests need it; the scratch root is disposable |
| ceilings: 500 ops per transaction, 32 terms per query list | bounds on every caller list, the safe-code rule |
| database files are created 0600 before sqlite opens them | sqlite would make them 0644 and its WAL copies the mode |

**Slice 4 as built, 2026-09-26** (rig `5135aba`). Seat choices, his to
overrule:

| Choice | Why |
|---|---|
| **one commit per pass for the whole `files/` tree**, not one per program as R9 reads | R31 lays `files/` out by kind, so most paths belong to no one program; the index (4b) records the writer |
| a text file over 10 MiB is left out like a binary | a huge log in git history is carried by every clone forever |
| a symlink is committed as the link, its target never read | a link must not decide what rig reads |
| git runs with hooks, fsmonitor, signing and the user's and system gitconfig all off; pathspecs literal | programs write `files/` freely, `.git/` included, and nothing there may run as rig |
| a terminal's or agent's `files.root` for a program makes nothing | as for store reads: the name is not proved |
| an interval under 1 second is refused at start | the committer would run git back to back |

**Not closed:** a program can still edit `files/.git/config` (a filter
driver, for one). The settings above close what runs on `status` and
`commit`; the rest is the deferred threat model, not this build.

**Slice 4b part a as built, 2026-09-26** (rig `406ec32`): the index over
`files/`, as `files.index`, `files.search` and `files.unindexed` on the
wire, MCP and CLI. Seat choices, his to overrule:

| Choice | Why |
|---|---|
| FTS5 in `internal/files-index.db`, never inside `files/` | nothing a program writes may edit the index, and git never carries it |
| every path is read through Go's `os.Root`; `..`, absolute, `.git/` and a final symlink are refused | a path or a link must not reach outside `files/`; `os.Root` is the standard library's answer |
| the whole `files/` tree is indexable, `programs/<id>/` included | R26 covers everything an agent writes there |
| the writer is the registered program, else the caller's seat; an unseated caller is refused | as `knowledge.add`: an entry says who wrote it |
| a binary is found by title, summary and tags; text past 1 MiB is not searched | the index stays small; the entry says so |
| "stale" is a changed size or mtime, git's own test | hashing every file on each listing costs a full read |
| indexing a path whose file is gone drops its entry; the listing reports it as `gone` | the index follows the area without a separate verb |
| a directory a program made unreadable is skipped by the listing | one program must not break the listing for all |
| title weighs 4, summary and tags 2, body 1 in the ranking | a writer's own words outrank incidental text |

**Slice 4b part b as built, 2026-09-26** (rig `d42840b`): the layout, as
`files.place`, `files.layout` and `files.relayout` on the wire, MCP and CLI.
Seat choices, his to overrule:

| Choice | Why |
|---|---|
| the layout is `internal/layout.txt`, one `kind place/` line each; placeholders `{subject}`, `{type}`, `{program}`, each a whole directory | the seat recommendation above, made concrete; one directory per value keeps a caller's value a name, never a path |
| rig keeps `internal/layout.applied` beside it and answers `files.place` from that copy until `files.relayout` runs | an edit must not send writers somewhere the existing files are not; relayout needs the old layout to find them |
| each kind has its own top-level directory, and the kind `programs` must exist with `{program}` | kinds never overlap, so a file belongs to one kind; `files.root` takes a program's directory from it |
| relayout is all or nothing, refused whole if a file would be stranded (kind removed with files, file not laid out, new placeholder with no value) or overwritten; a failure part-way moves everything back | nothing a search or a writer relies on may be lost silently |
| relayout makes **no commit of its own**; the moves land in the next interval commit, where git reads a text file's delete and add as a rename | R34 outranks the "one commit" of the verb table above; `git mv` stages the same thing |
| `files.place` makes nothing, and ignores a value the kind's place does not take | the write stays direct (R12); a caller can send every value it has |
| `files.relayout` is declared destructive | it moves files other callers were told about |

**Not closed:** a program holding a path from before a relayout keeps writing
to the old place; it must ask `files.root` or `files.place` again. Files left
in a directory no kind claims are not moved or reported by relayout.

**Slice 4b part c as built, 2026-09-26** (rig `0eac8ba`): `knowledge.*` is
a view of the shared lessons folder (R24, R25). Same verbs, same MCP tools,
same CLI. Seat choices, his to overrule:

| Choice | Why |
|---|---|
| a lesson is `lessons/<id>.md` with a `---` header (id, title, summary, tags, seat, session, epoch, created), then the body | the file alone rebuilds the index, and an edit by hand is searched as it now reads |
| the id is the file name; a moved lesson keeps its record-store id | links and notes that name a lesson stay good |
| `knowledge.search` first refreshes the index from the folder: new or changed files with a header are indexed, gone ones dropped | the folder is the source; a file written or deleted by hand is seen at the next search |
| a file in the folder with no header is still a lesson, named by its writer's `files.index` entry | R26: the writer describes a file |
| at start rigd copies the record store's lessons into the folder, once, and leaves a marker `internal/lessons.moved`; a file that exists is never overwritten and the table is kept as the backup | non-destructive, so it needs no separate step; the marker stops a lesson deleted from the folder coming back at the next start |
| the folder is the layout's `lessons` kind; lessons need a storage root, and work on an unnamed estate that has one | one place decides where files go (R28) |

**Deploying runs the copy on the live lessons.** Boris's go is owed first.

**Not closed:** a search snippet can show header text, since the header is
indexed with the body. For the few milliseconds between rigd's MCP listener
opening and the files opening, an MCP lesson call answers "unavailable";
socket callers never see it.

**Slice 5 as built, 2026-09-27** (rig `2c8ba62`): `store.export` and
`store.import`, on the wire, `rig store export|import [collection]...` and
two MCP tools. Demonstrated live on a private rigd. Seat choices, his to
overrule:

| Choice | Why |
|---|---|
| a line is `{"id","version","updated_ns","doc"}`, HTML characters not escaped | the version and time come back on import, so a caller holding version 7 still holds it; `<` stays `<`, so an export then import gives the same bytes |
| all collections are read in one read transaction that does not take the write lock | one moment of the store, and no writer waits on an export |
| a whole export removes the file of a collection the store no longer has; a named export touches only what it names | the directory is the store; git keeps the history |
| an unchanged export makes no commit, and says so | R9's rule for the free files, kept |
| the exports repository has no size limit on a file (the free files skip over 10 MB) | an export left out of git for its size is an export not kept (R21) |
| import checks every line of every file first (unknown fields, id pattern, version 1 or more, a JSON object, each id once), then snapshots, then replaces in one transaction | one bad line refuses the whole import with nothing written and no snapshot |
| the snapshot is `internal/snapshots/store/<program>-<UTC ns>.db`, owner-only, never pruned | it is the undo of an import; pruning is his call |
| collections the import does not name are left alone and listed | D8 replaces what is named, not the store |
| export and import run one at a time per daemon | one export's commit never carries another's half-written files |
| an export may not create a store; an import may | exporting a program that never wrote is a typo; importing onto an empty machine is the restore path (R23) |

**Not closed:** a write landing between the snapshot and the replace is in
neither. An import holds the whole export in memory, so an export of
several GB would need streaming. An import by a terminal that names a
program with no export still creates an empty database for it.

**Slice 6 as built, 2026-09-27** (rig `ee59cbb`): `examples/storeworker`, an
embedded-tier program (its own page, pane.js for the tokens) that runs fake
assignments the way graft runs real ones. Build it with
`make build-storeworker`; it needs a named estate for its queue and lease.
Its pane has one tab per part of rig (R39):

| Tab | What Boris tries | rig verbs |
|---|---|---|
| The program | call a command through rig, or with empty arguments to see rig refuse them against the declared schema | hello, routed commands |
| Store | assign; filter by state; a stale write refused | `store.transact`, `query`, `put`, `collections` |
| Queue | three quick runs, claimed one at a time | `queue.push`, `claim`, `complete`, `lease.renew` |
| Leases | hold the gpu lease himself; the worker waits | `lease.acquire`, `release`, `list` |
| Asking you | an expensive run parks until he answers | `health.report` parked, urgent `notify` |
| Toasts | one of each severity | `notify`, `toast.dnd` |
| Files | search the transcripts | `files.place`, `index`, `search`, `unindexed`, `layout` |
| Lessons | a failing run records a lesson | `knowledge.add`, `search` |
| Mail | send to a seat, read the inbox | `announce`, `peers`, `message.send`, `inbox` |
| Export | export, assign, restore: the run is gone | `store.export`, `import` |
| Health | the marker and the parked question | `health.report`, `activity` |

Seat choices, his to overrule:

| Choice | Why |
|---|---|
| every tab shows its explanation, its controls, its live state, and the rig calls it made | he sees each click turn into verbs |
| the page's own 2 s polling reads are not listed; the footer names them instead | listed, they pushed every click's calls off the list |
| a second, unregistered connection acts as him: his lease, his commands through rig | rig then sees a caller other than the worker, as it would from a terminal |
| the page API needs its own Host and an `X-Storeworker` header | a loopback server is reachable from any page he opens |
| restore refuses without `{"yes":true}`; the page asks by a second press | the command declares it confirms, so it must |
| a run redelivered after a restart starts over, and asks again if over budget | otherwise it stays running forever |
| the test starts its own daemon with a storage root | `clienttest` gives none, so no store; `client/` is the lead's |

**Demonstrated live, 2026-09-27**, on a private `development` estate
(`--root`, private runtime, state and config), started by `rig up` from a
private `programs.json`, and driven in headless Chrome over every tab:

```
$ rig health
storeworker  healthy  ...  PARKED: Run r0926-215012-433 (an expensive run)
    will cost $2.10, over the $1.00 budget. Go ahead?; waiting on Boris's answer ...
$ rig storeworker restore
rig: storeworker.restore: CODE_DENIED: storeworker: restore replaces the whole
    store with its last export, and needs yes to go ahead
       fix command   rig storeworker restore --args '{"yes":true}'
```

The stale write answered `CODE_CONFLICT: ... is at version 4 and the call
expected 3`; holding the gpu lease as him made the worker wait, and on
release it took the lease at token 7; a parked run asked again after
`rig restart storeworker`; a restore under a running run gave a *Lost* toast.

**Not tried:** the pane inside rig's own window, so the theme hand-off
(the page said "none, this page is open outside rig's window"); the tray
drawing the toasts; slices 5 and 6 are not deployed.

**R40 as built, 2026-09-27:** each tab lists the rig words it uses under
its explanation, in plain view; an *Every word* tab lists all of them with
the tabs that use them, marks storeworker's own words (run, budget, worker)
as not rig's, and names the parts of rig no program can show (the window,
the `rig` command, the continuity record, MCP, backups). A test fails on a
word a tab uses and the list does not explain; red control: an unexplained
word added to a tab. Exercised live over all 12 tabs: no word missing.

**What it costs, measured 2026-09-27 on production, supervised:** 13 MB
resident, 1 CPU tick (10 ms) in 60 s idle. Idle, the worker asks
`queue.claim` every 3 s. The open page reads its tab every 2 s, and since
this build reads nothing while hidden: 3 reads in 6 s visible, 0 hidden.

#### Tests owed, each with the red control that proves it bites

| Test | Red control |
|---|---|
| a second writer with a stale version is refused | drop the version predicate |
| program A cannot read or write program B's collection | take the namespace from the argument |
| a hostile collection name (`../x`, `a/b`) is refused before disk or SQL | skip the name check |
| a field path with a quote cannot alter the query | build the path into SQL text |
| `transact` with one failing step leaves nothing written | commit per step |
| export is sorted by id, byte-stable across two runs | iterate the map |
| export then import round-trips documents and versions | reset versions on import |
| import snapshots the database first | skip the snapshot |
| the committer commits nothing when nothing changed | commit unconditionally |
| a repository hook cannot run | drop `core.hooksPath` |
| a search never returns a file's body | return the body |
| a file changed after its entry is listed by `files.unindexed` | compare paths only |
| `files.index` refuses a path outside `files/` (`../`, a symlink out) | join the path unchecked |
| every new verb is an MCP tool | the P8 test, unchanged |

**Acceptance:** slice 6 demonstrated live, with the commands and their output
in this section; `make ci` and `make lint` 0 at the final sha.

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
(in `~/me/projects/docket` since §50 move 8, run as below) unchanged from
the sha before the seat's first commit, `make deps-check` 0
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

### As built, 2026-10-01 (Boris: *"Just do it"*, decision 0251)

| Slice | Commit | State |
|---|---|---|
| 1, one opener and runner | `d6568fb` | `internal/store/sqlite.go` (`OpenDB`, `Migrate`, `Schema`); record, files index and program stores register with it |
| 2, `fields` on `record.query` | `7a553f8` | field 5, `--fields`; projected before the page is sized |
| 3, `rig store list` | `f291e02` | record, coord, `programs/<id>`; a registered program is refused |
| 4, coord off `bbolt` | `0b76df5` | leases, mail and queues on the one runner, values the same JSON as the buckets held; rigd refuses a bbolt `coord.db` by name; `cmd/coordconvert` carries epoch, mail counter, trim lines and queue sequences once, original kept as `coord.db.bbolt`; `tools/coord-cutover.sh` converted the three live files and deployed (`750f150`, epoch 83 to 85). Then `bbolt`, `coordconvert` and the script left the tree: acceptance 1 and 6 |
| 4a, `--json --fields` | `0d944f0` | unrequested `body` and `provenance` omitted; live: 43,734 to 28,479 bytes |

**Acceptance at `0013f87`, 2026-10-01:**

| # | Result |
|---|---|
| 1 | passes: `sql.Open` only in `internal/store`; `go.mod` has no `bbolt`. `PRAGMA user_version` remains in tests and in `internal/record/snapshot.go`, which reads a backup snapshot's version, not a second runner |
| 2 | passes (measured at `7a553f8`, above) |
| 3 | passes live: record v3, coord v1, programs/storeworker |
| 4 | passes: 10.7x on the wire; `--json` 43,734 to 28,479 bytes after `0d944f0` |
| 5 | passes in `make ci`; no live `rig backup` was run, it is his to ask for. coord is outside the archive by §46 decision 5, so the engine change does not reach it |
| 6 | passes: §22's row reads REMOVED, -323,584 bytes on `rigd`; `make deps-check` 0 |
| 7 | passes. `make ci` 0, `make lint` 0, `make proto` idempotent. `rigseed --check` exits 0 on the commit after `17ed01f`, which fixed the four heading collisions below |

**Found, not caused, by S1, and fixed under §38f:**

- **The size ratchet had drifted on all seven rows** since its last
  record on 2026-09-24 (rigd +1,314,816, rig +966,656, rigwindow
  +732,856). Every row was shipped work; re-recorded at `17ed01f`, the
  commit names what grew.
- **`rigseed --check` was not runnable as written:** the seeder moved to
  docket (§50 move 8), and `BACKLOG.md` and `DECISIONS.md` are now
  indexes it cannot parse. It runs as: build `docket/cmd/rigseed`, feed
  it `rig logbook cat BACKLOG.md` and `DECISIONS.md`, against a scratch
  `development` rigd with `RIG_ROOT` and a short `XDG_RUNTIME_DIR` of its
  own. Seeding takes 13 s.
- **It then failed on four heading collisions,** so four requirements
  never reached the store: §12 "Toasts, built" and §48's second
  "RULED BY BORIS", third "RULED BY BORIS", and second "Boris". Each now
  has its own lead words; `--check` exits 0.

**One coord test changed, a finding by the slice's own bar:**
`TestANewerSchemaRefusesToOpen` planted its newer schema through bbolt;
it now plants it with `PRAGMA user_version`. Its claim is unchanged.

**Measured on a `VACUUM INTO` copy of production at `7a553f8`:**
acceptance 2 holds (version 3, no migration, 1,330 live heads = 1,332
less 2 retracted). Acceptance 4: rig's 135 work items are 213,180 bytes
in full and 19,949 with `--fields title,status`, 10.7x.

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
