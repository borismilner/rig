## 54. A program started when it is called

**Ruled by Boris, 2026-10-02 (decision 0263, "Approve A"):** a declared
program can hold no memory at rest. A call to one of its commands makes rig
start it, wait for it to register, and route the call; the program exits on
its own when it has nothing left to do. Its commands stay under its name
(`rig beacon ask`, MCP `invoke`): *"yes, exactly - that's how I envisioned
it."* Backlog 0018. First adopter: `~/me/projects/rigged/beacon`.

**Status: APPROVED by Boris, 2026-10-02:** *"Seems good - do it"*, said of
this section as drafted, with the three open rows at the end ruled as
recommended. Decision 0264.

### ⛔ The program decides when it is idle, not rig

**rig cannot see idleness from outside, and beacon is the proof.** A notify
card with action buttons stays on screen after its call has returned, so a
beacon with no call in flight may still be showing something. A rig that
stopped it "after five idle minutes" would take a card off his screen.

So the program exits when it has nothing left, and rig reads that exit:

| How it ended | rig reads it as | Then |
|---|---|---|
| exit 0, no call in flight | **at rest**, the normal end | nothing; the next call starts it |
| exit 0 while a call is in flight | a crash | the call fails, naming the exit |
| non-zero exit, or a signal rig did not send | a crash | counted against §18's restart budget |
| stopped by rig (`rig stop`, shutdown) | a stop | at rest |

The Go client library gets a helper for the common case: exit after a set
time with no call in flight and nothing held by the program. beacon holds
while a card is up.

### `programs.json`

```json
{"id": "beacon", "path": "/home/boris-milner/.local/bin/beacon", "on_call": true}
```

- `on_call` is a boolean beside `autostart`, which it matches. **The two
  are refused together:** "start at rigd start" and "start when called"
  contradict each other.
- Everything else (args, health, budget) means what it means today.

### The declaration, read once and kept

rig needs the commands while the program is down, for `rig apps list`,
`rig describe` and MCP. The declaration is taken from the program's own
hello, so **a program needs no special mode** to be read:

1. **The declare run.** rig starts the binary, takes the declaration from
   its hello, then stops it through the normal shutdown path (lifecycle
   notice, SIGTERM, grace).
2. **Kept** at `<estate state dir>/declarations/<id>.pb`, together with the
   binary's identity: device, inode, size and mtime. An unnamed estate
   keeps it in memory only.
3. **Re-read** when that identity no longer matches. `go build`,
   `make install` and a package upgrade each change it. The check is one
   `stat`, made:
   - at rigd start;
   - when a listing or describe reads the program;
   - before each start.

   A mismatch at rest runs a declare run in the background. Until it ends,
   the listing shows the old commands marked "binary changed, re-reading".
4. **Every hello refreshes it.** A start on call registers with the current
   declaration anyway, so the declaration is never staler than the last run.

### A call while it is down

- **One start, many callers.** Calls that arrive while it starts wait
  behind the same start; a second process is never launched.
- **The wait is bounded by `health.register`**, 30 s by default. Its time
  counts against the call's own deadline, and neither is extended.
- **A failed start fails the call with what happened**, not with
  "not connected":

  | Field | Value |
  |---|---|
  | code | `UNAVAILABLE` |
  | message | `beacon did not start: exited 1 before registering` (or `did not register in 30s`) |
  | detail | the last stderr lines it wrote |
  | fix | `rig health beacon`, `rig logs --client beacon` |

- **A command the program does not declare** is refused from the cached
  declaration **without starting it**. The answer is the same NOT_FOUND it
  gets today.
- **§14's visibility check runs first, unchanged.** A caller that may not
  see beacon cannot start it.

### §18's state machine gains one state

| State | Means |
|---|---|
| `AT_REST` | declared on call, not running, declaration kept. Not a failure |

| From | To | Trigger | Caused by |
|---|---|---|---|
| - | `AT_REST` | rigd starts, or the declare run ends | rig |
| `AT_REST` | `STARTING` | a call to a declared command | **the caller** |
| `HEALTHY` | `AT_REST` | exit 0 with no call in flight, or a stop | the program, or a human |
| `HEALTHY` | `AT_REST` | a crash with no call waiting | the program; it counts in the budget |
| `RESTARTING` | `STARTING` | as today, only while a call is waiting | rig's restart budget |

- **A crash does not relaunch a program nobody is calling.** That would be
  a resident process by another route.
- **The restart budget still counts.** Past it the program is
  `QUARANTINED`, and a call is refused at once with the history, not
  started.
- **§18's "idle-and-not-blocked is unhealthy" does not apply** to an
  on-call program at rest. While it runs, it does.

### What each surface shows at rest

| Surface | Shows |
|---|---|
| `rig apps list` | the program, its commands, state `at rest` |
| `rig describe beacon` | the full declaration, from the cache, with its age |
| `rig health` | `AT_REST`, last start and exit, the restart budget used |
| MCP `list` / `invoke` | as for a running program; `invoke` starts it |

### Tests, each with its red control

| Property | Red control |
|---|---|
| a stub declared on call is not running at rest | the same stub with `autostart` is |
| a call starts it and is answered | `on_call` removed: the call gets NOT_FOUND |
| ten concurrent first calls launch one process | starts counted, with single-flight disabled in the test |
| exit 0 when idle is `AT_REST`, not a crash | exit 1 counts against the budget |
| a changed binary is re-read | the identity check disabled: the old commands stay |
| a failed start names its exit and stderr | — |
| an undeclared command does not start it | — |
| `apps list` and `describe` show its commands while it is down | — |

**Done when** a stub on a scratch estate:
- is not running at rest;
- starts on `rig <stub> <cmd>` and answers it;
- exits on its own when idle;
- shows its commands in `rig apps list` and `rig describe` while it is
  down.

### The three rows put to him, ruled as recommended

| Row | Recommendation |
|---|---|
| the field: `"on_call": true` or `"start": "on_call"` | **`on_call`**, matching `autostart`, so existing files keep their meaning |
| should rig also stop an on-call program after a long idle, as a backstop? | **no.** rig cannot see what beacon holds; `rig health` showing "running, no call for N minutes" is enough |
| the first-ever read: at rigd start, or on the first listing? | **at rigd start, in the background.** The listing is never empty for want of a read |
