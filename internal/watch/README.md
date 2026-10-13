# Filesystem watch engine

`watch` is an estate-local event source. It does not register wire verbs,
load production settings, or start itself inside rigd.

The owner holds the estate's single-instance lock, calls `OpenState`,
constructs `New` with its event-bus and coverage-log callbacks, applies
resolved settings with `Configure`, and calls `Run`. Cancel the context or
call `Close`, wait for `Run` to return, then close `State`. Errors require
the owner to report the failure and restart with `New` after fixing it.

`Declare` adds or replaces one group; `Configure` replaces all groups.
Removing a group discards its pending batch and delivery. `Config` returns
an independent copy. The JSON `ReadConfig` helper is for embedding and
tests; production should adapt the existing resolver into `Configure`.

```json
{
  "max_watches": 8192,
  "max_entries": 100000,
  "watches": [
    {
      "id": "buddy-notes",
      "paths": ["/srv/notes"],
      "ignore": ["*.tmp", "generated"],
      "debounce_ms": 900000,
      "description": "Consolidation input"
    }
  ]
}
```

Paths are absolute files, directories, or `filepath.Match` globs (`*`,
`?`, `[class]`). Directories, including glob matches, are recursive.
`**` and escapes are refused. Symlinks are entries, never traversed;
symlink ancestors are refused. A missing root is tracked through its
nearest existing directory. Directories are subscribed before their
children are enumerated, including populated directories moved in.

Every group excludes `.git`, `node_modules`, `build`, `dist`, `out`,
`target`, `.cache`, `__pycache__`, `.pytest_cache`, `.mypy_cache`,
`.ruff_cache`, `.next`, `.nuxt`, `.parcel-cache`, and `.turbo`.
The optional per-group `ignore` list adds basename globs using
`filepath.Match` (`*`, `?`, `[class]`). Patterns match any component of
the absolute path, including the root and its ancestors, and exclude the
whole matching subtree. Slashes, escapes, `**`, empty patterns and invalid
globs are refused. Omitted, null and empty lists all keep the defaults;
there is no disable-defaults switch. Gitignore files and negation are not
interpreted. Each group accepts at most 256 patterns of 255 bytes each.

Excluded entries consume no snapshot budget or recursive subscriptions.
Overlapping groups may still share subscriptions when one needs a path
another excludes; notification and snapshot filtering stay per group.

Each matching notification publishes `rig.watch.<id>.changed` after its
snapshot and pending debounce are saved together. The quiet period
defaults to 15 minutes and restarts on every matching notification.
After quiet, `rig.watch.<id>.fired` carries the batch count and timestamps.
It does not include file contents or a potentially unbounded path list.

Before publishing a fire, the engine saves an immutable delivery record.
Successful publication precedes deleting that record. Failure or a crash
can therefore repeat a fire with the same ID; consumers can deduplicate
by ID. Changes after that delivery was prepared form a separate batch.
The contract ends at successful callback acceptance: bus retention and
subscriber recovery remain the bus's responsibility. Individual changed
notifications are not replayed after publication failure; their durable
batch still fires.

`Publish` and `ReportGap` must honor their context and must not call back
into the watcher. Operations and callbacks are serialized. Callbacks
should hand work to the existing bus/log rather than perform slow work.
The injected backend factory must return a fresh empty backend each time.

Startup rescans preserve unchanged pending deadlines and detect snapshot
differences. Overflow declares a coverage gap, reconciles subscriptions,
and invalidates every group even if its final snapshot is unchanged.
Explicit `Rescan` and configuration replacement also report coverage.
Snapshots compare size, modification time, mode, and symlink target;
they cannot reconstruct transient offline changes or edits that preserve
all that metadata. A rescan is not a content audit.

`Usage` reports this engine's subscriptions and configured ceiling,
plus Linux's per-user `max_user_watches` when available. It does not
measure other processes' usage. Parent sentinels and overlapping paths
share subscriptions within a backend. Broad globs require watching
their static-prefix tree and may reach the ceiling. Snapshots are also
bounded across all groups, including overlapping entries.

Replacement configuration is prepared before swapping, so both old and
new backend subscriptions must fit the ceiling temporarily. A failed
validation, scan, registration, or database transaction preserves the
old configuration. An error after the commit, such as a failed callback,
leaves the new configuration saved. The engine excludes its own SQLite
database and journal files from notifications and snapshots.

Deadlines are persisted wall-clock times. Connect `Wake` to the daemon's
resume notification; while a deadline is pending, it is also rechecked
at least once per minute. Quiet periods are not replayed repeatedly after
suspend or restart.

State uses rig's SQLite opener and migrations in the named estate's
`watch.db`. Schema 3 protects ignore settings and immutable deliveries
from older readers; schemas 1 and 2 upgrade without changing SQL tables.
Before production
use, the daemon integration must include this namespace in its state
inventory and backups, register the watch event family, connect coverage
and resume hooks, and expose settings through the existing resolver.
