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

### ⛔ Requirement, 2026-10-03: rig finds its programs, and each binary says how it loads

**Stated by Boris, 2026-10-03, verbatim:**

> `rig` should have in its configuration the paths it needs to scan for
> applications to load.
>
> Each application found should expose as part of its binary how `rig`
> should load it, for example take `beacon`, it should show that `rig` is
> only to learn its API and not waste memory on keeping it always in
> memory. Others may ask to be loaded and be monitored for crushing and so
> on.

What this changes in the section above:

| Above | Now required |
|---|---|
| each program is one row of `programs.json` | rig's configuration names **directories to scan**; what it finds there is a program |
| `"on_call": true` / `"autostart": true` are set in `programs.json` | **the binary declares its own load mode**: on call (learn the API, hold no memory) or resident (started and supervised for crashes) |

**Status: RULED, 2026-10-03, decision 0265**, all three rows as
recommended:

| Row | Ruling |
|---|---|
| which directories | **dedicated rig directories**: `~/.local/lib/rig/apps` by default, plus any configured. Everything in one is a program. Never `~/.local/bin`: finding a program costs one declare run, and rig does not execute every binary on the PATH |
| where the binary says how to load it | **a load mode in the declaration, read from the hello** on plan/54's declare run: `on_call` or `resident` (started at rigd start, supervised for crashes) |
| what `programs.json` keeps | **overrides only**: args, health budget, disable, or a load mode overriding the binary's. A scanned program needs no row |

This supersedes `"on_call": true` in `programs.json` above as the
source of the mode; the rest of the section stands.

### ⛔ Requirement, 2026-10-03: what rig learned is persisted, and the scan only updates it

**Stated by Boris, 2026-10-03, verbatim:**

> The knowledge about API of the different programs must be persisted so
> that `rig` is not dependant on re-scan every time to have this
> functionality exposed to the users.
> The scan happens in the background and it may update the knowledge if for
> example the application was updated or deleted or anything like that.

What it binds:

| Property | Required |
|---|---|
| **served from what was persisted** | at rigd start, every program found before is listed and callable from its kept declaration at once, before any scan runs |
| **for every scanned program** | on call and resident alike, not only the on-call ones of "The declaration, read once and kept" |
| **the scan is background work** | it never blocks a listing, a describe or a call |
| **the scan updates the knowledge** | a changed binary is re-read; a **deleted** binary's program is removed from what is kept and listed; a new one is added |

Above, an unnamed estate keeps declarations in memory only; that stays, as
an unnamed estate has no state directory to persist into.

### As built, 2026-10-03

Built in 2e92701..6062c28 and demonstrated live on a scratch rigd with
two stub binaries in a scan directory, one declaring on call and one
resident. Exercised: both found with no `programs.json` row; the on-call
one listed `at rest` with no process; `rig napper greet` started it and
was answered; it exited on its own after its idle time and was `AT_REST`,
not a crash; `rig describe` read it while down; after a rigd restart it
was listed again with **no launch**; deleting the resident's binary
removed it from the listing and its kept file. Not exercised live: MCP
`invoke`, and a rebuilt binary (covered by a wire test).

Where the build differs from the text above:

| Above | As built |
|---|---|
| kept at `<estate state dir>/declarations/<id>.pb` | `<id>.hello`. Its first line is the binary's identity and path, so rigd can declare it again with no scan |
| an unnamed estate keeps declarations in memory | it keeps them under its scratch root, with the rest of its storage |
| an undeclared command gets NOT_FOUND | it gets `INVALID`, the answer a running program's caller already got |
| `RESTARTING→STARTING` while a call waits | unused: an on-call program never restarts. A crash or a stall goes straight to `AT_REST` |
| the load mode in the declaration | `Declaration.load`: `LOAD_ON_CALL` or `LOAD_RESIDENT`. Unspecified is resident. A program a scan found is held on call until its declaration is read, so its declare run is never a blind resident start |
| when the scan runs | at rigd start (after what was kept is served) and on each listing, in the background, single-flight. **No timer**, which would cost the idle wakeups §17 measures |
| what a scan forgets | a scanned program whose binary is gone. **Nothing** on a pass that could not read a directory, since that says nothing about deletion |
| `programs.json` | a row with no `path` overrides the scanned program of its id; a row with a path is declared as before. `autostart` or `on_call` in a row overrides the binary |
| config | `programs.scan`, colon-separated, default `~/.local/lib/rig/apps`, applied at restart |
| the idle helper | `client.Idle(d)` and `client.Hold()`, added to the client's surface |

**Two gaps, known:**

- **A resident program is listed only while it runs.** Its declaration is
  kept and it is started from it at rigd start with no scan, but a
  resident that is down is not listed, because a call to it could not
  start it.
- **A resident's changed binary is read at its next start**, not by the
  scan: the scan does not restart a running program to read it.
