## 8. Observability

rig is where "highly introspectable" is actually delivered, and it is delivered once for
everything rather than fifteen times badly.

| View | Shows |
|---|---|
| Programs | Every one: pid, uptime, restarts with reasons, RSS, CPU, wire version, health history |
| Calls | Live call log both directions: method, duration, status, size, trace id, expandable payload |
| Traces | One action as a span tree across processes, whatever surface started it. Exports OTLP |
| Logs | Merged, one colour per program, level and text filters, follow, jump to the trace |
| Events | A live tap on the bus: publisher, topic, payload, who received, who was denied |
| Config | Every key, the winning layer and every losing value. Searchable |
| Schedule | What is due, what fired, what failed, what it cost |
| Audit | Every action taken anywhere, by whom, through which surface. One log for the estate, with its coverage stated (§5k) |
| Loose ends | What stopped pointing at anything after a declaration changed. See below |
| Doctor | Versions, drift, orphaned registrations, quarantined programs, capability warnings, disk, permissions, stub build vintages, programs on a frozen wire major |

Every view above states its coverage, and none of them answers a question about history without
saying what it could not see (§15).

### `rig doctor` names every dependency, including the ones that are missing

The Doctor row above is what rig can see about *itself*. It says nothing about what rig needs
**from the machine**, and rig degrades against roughly a dozen external things that are
individually optional and collectively the difference between a working estate and a puzzling
one.

**A degradation nobody can see is the failure mode.** §5g already promises a capability can be
`unavailable` with a reason that `rig doctor` can see; §12 already promises `rig doctor` says
whether the frameless toast or the freedesktop fallback is live. Those are two instances of a
rule that was never stated once, and stating it once is what makes the rest of the plan's
"degrades gracefully" claims checkable.

**Three classes, and the distinction is what makes the output usable:**

| Class | If it is missing | Examples |
|---|---|---|
| **Required** | rig refuses to start and names which one | `$XDG_RUNTIME_DIR`, a writable socket directory, the store path |
| **Optional** | a **named** feature degrades to a **named** fallback | speech engine, audio player, keyring, a compositor with an ARGB visual |
| **Build-time** | nothing at runtime; reported only under `--dev` | `protoc`, `wails3`, node, Chrome for the contrast gate |

**Every optional row prints four things:** what is missing, what stops working, what happens
instead, and the one command that installs it. `keyring: not found` has told a person less than
they knew before they ran it.

**Kernel facilities are dependencies too, and they were being assumed.** Each was probed on this
laptop on 2026-09-10 and each gets a row, because each is a silent absence rather than a loud
one:

| Facility | Used by | Here |
|---|---|---|
| PSI read side | `disk.pressure`, `memory.pressure` (§5h) | present |
| PSI **triggers** | nothing, deliberately | **rejected, `EINVAL`** - which is why rig polls (§5h) |
| `pidfd_open` | `process.exited` (§5h) | present |
| `PIDFD_INFO_EXIT` | would give a non-child's exit code | **absent** - which is why the code is `unknown` (§5h) |
| inotify `max_user_watches` | `fs.changed` (§5h) | 524288 |
| inotify `max_queued_events` | `fs.changed` (§5h) | **16384**, which one `git checkout` overruns |

**`rig doctor --json` carries the same content**, because an agent asking why a capability is
missing needs the answer in a form it can act on, and §14 already grants `introspect` for
exactly that.

**The check ships with `doctor` at M5, and every later milestone adds its own rows rather than
a second checker**: M8 the compositor and the tray, M9 the speech engine and the audio player,
M11 the keyring. A milestone that introduces an optional dependency and no doctor row has not
finished.

### Loose ends

When a declaration changes, rig reports what in the rest of the estate stopped pointing at
anything: schedule entries, bus rules, capability grants, tray entries, promoted MCP tools,
`rig://` routes, config overrides. `rig loose-ends` on the CLI, a view in the window and the
TUI, and a line in `rig doctor`.

It is cheap - the registry already holds both sides - and it catches a real bug class the rest
of the plan cannot. A renamed config key does not make its override *lose*; the override simply
stops existing under that spelling, so `rig config origin` shows no loser and the setting
silently reverts to a default.

### What a recorded call actually costs

§15's budget covers the history append alone. This section mandates two more recordings per
call, and the plan used to contradict itself about the total:

| Recording | Measured | Mandated by |
|---|---|---|
| History append, one writer | 110.8 ns | §15 |
| OpenTelemetry span | 1019 ns, and a *rejected* span still costs 195.6 ns | this section |
| `slog` record | 627 ns | this section |
| **Total per call** | **1757 ns** | 28% on top of §4's 6.2µs wire |

The budget in §15 is stated against that total, not against one third of it. Sampling does not
rescue the span, because a rejected span is not free.

---
