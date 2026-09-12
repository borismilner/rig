## 10. The terminal client

Everything rig can do is reachable from a terminal, and the terminal is not the poor relation of
the window. A headless server, an SSH session and a container get the whole product.

**Three faces, because scripts, humans and agents want different things.**

| Face | Shape | For |
|---|---|---|
| `rig <app> <cmd>` | One shot, no interaction, exit codes, `--json` on everything | Scripts, Makefiles, cron, agents |
| `rig shell` | Line-oriented, completion, history, inline `describe`, `?` help | A human exploring, fast |
| `rig tui` | Full screen, panes, live tails, generated forms | A human working, or watching |

### `rig tui`

Mirrors every view the window has, because both are surfaces reading the same registry. Same
data, different renderer.

```
┌ rig ───────────────────────────────────────────────── 6 up  0 degraded ┐
│ ● shelf     │ shelf                                      ● ok  1.2.0   │
│ ● graft     ├────────────────────────────────────────────────────────  │
│ ▲ dispatch  │ commands   reindex · search · stats · gc                 │
│ ● archi     │ config     8 keys, 2 overridden                          │
│ ○ snapper   │ storage    412 MB · migrated 19 · backed up 2h ago       │
│ ● nudge     │ health     ok, checked 4s ago                            │
│             │                                                          │
│ [1] progs   │ 14:22:03 shelf  index  scanned 1,204 files               │
│ [2] logs    │ 14:22:03 graft  run    nightly-audit started             │
│ [3] calls   │ 14:22:04 shelf  index  done in 812ms                     │
│ [4] traces  │                                                          │
│ [5] config  ├────────────────────────────────────────────────────────  │
│ [6] sched   │ :  invoke shelf.reindex --since 7d                       │
└─────────────┴──────────────────────────────────── ? help   q quit ─────┘
```

### The forms come from the same schema as the window

A command's argument schema renders as a Svelte form in the window and as a `huh` form in the
terminal. One declaration, two renderers, and a program that adds an argument gets both. This is
the renderer architecture from §5h proving itself on its first real case.

### What agents actually use

Being honest about which face serves whom: **a full-screen TUI is hostile to an agent** - ANSI
escapes, redraws, cursor state. So the agent affordance in the terminal is not the TUI, it is:

- `--json` on **every** command, every view and every error, returning exactly what the MCP tool
  returns
- `rig describe <anything>` with the full declaration, examples and effects
- Structured errors that name the failed precondition and the command that fixes it
- `rig shell --batch --json`, which reads commands on stdin and writes one JSON object per line,
  for a scripted session that needs state

An agent that only knows **list**, **describe** and **invoke** can operate the entire estate.

**Those are the MCP TOOL names, and this line used to write them as CLI verbs -
`rig --json list` and so on - which do not exist.** Corrected 2026-09-11, found
by typing the line rather than by reading it. `rig list` today dispatches as a
program named `list`; there is no `describe` and no `invoke` verb at all.

**The CLI spelling and the MCP tool name are DIFFERENT SURFACES OF ONE
OPERATION, and §10's rule is about the OBJECT, not the spelling.** *"`--json`
returns exactly what the MCP tool returns"* constrains what comes back, and
deliberately does not require the two surfaces to be typed the same way - an MCP
tool name is a flat identifier in a tool list, and a CLI is a verb over a
namespaced estate.

| Operation | MCP tool | CLI today | CLI when M2 lands |
|---|---|---|---|
| enumerate the estate | `list` | `rig apps list` | `rig apps list` |
| one thing in full | `describe` | **does not exist** | `rig describe <anything>`, above |
| run a command | `invoke` | `rig <app> <cmd>` | `rig <app> <cmd>` |

**`invoke` is the one worth noticing: the CLI has had it since M1 and it is not
spelled like a verb at all**, because `rig shelf reindex` is the whole point of
the front door. An agent reading this plan for CLI syntax would have gone
looking for a command that was never going to exist.

**These three are M2 slices 2 and 3**, so the surfaces arrive together and the
table's last column is a commitment rather than a description.

### rig's own failures carry a code, and it cannot collide with the daemon's

**`--json` on every error means EVERY error, including the ones the daemon never
saw.** rigd unreachable, argv that did not parse, a client-side timeout: these
returned prose while every daemon refusal returned an object, which is the
defect §10's own rule exists to prevent. **The error most worth structuring is
"rigd is not running", because it is the one an agent is likeliest to meet
first.**

**Two vocabularies, disjoint by construction rather than by care.** Every code
the daemon can send is a value of the wire `Code` enum and is spelled `CODE_*`.
rig's own are spelled `RIG_*`. **So the sets cannot collide however many codes
either side adds later, and neither side has to check** - which is the
difference between a convention and a mechanism.

| Code | Means |
|---|---|
| `RIG_NO_DAEMON` | rig could not reach rigd at all. **Carries the socket path as `actual` and the best `fix_command` rig owns** |
| `RIG_NO_SUCH_PROGRAM` | the registry rig read does not name that program. **No `fix_command`**: rig cannot start a program, and §9 would rather say nothing than print a line that does not run |
| `RIG_NO_SUCH_COMMAND` | that program's declaration does not name that command. **Carries coverage in `actual`** (§5k), because "it declares ping, purge, reindex, and its coverage is partial" is what tells a caller whether to look again or look elsewhere |
| `RIG_BAD_ARGUMENT` | argv did not parse, or contradicted itself, before anything left rig |
| `RIG_TIMEOUT` | rig stopped waiting, **and the call may still be running** |
| `RIG_BAD_RESULT` | an answer arrived that the declaration did not promise |

**`RIG_TIMEOUT` is deliberately NOT the daemon's deadline code, and collapsing
them would lose the distinction that matters.** A daemon deadline says the call
missed the deadline the daemon supervises. `RIG_TIMEOUT` says the client ran out
of patience **and the work may still be in flight.** For an agent deciding
whether to retry, those are opposite facts, and a retry against the first is
safe where a retry against the second is a duplicate execution.

### The rule that keeps the terminal first-class

**A view ships in the TUI when its data lands.** Once the window exists at M1a, no view ships in
one without the other. The TUI is written as a surface plugin like any other, so it costs one
module rather than a parallel product.

**And the claim is scoped, because the unscoped version is false.** "Everything the window can
do, the terminal can do" holds for **generated** panes - the ones rig draws from a declared
schema - and cannot hold for embedded ones. archi's pane is an iframe over a hand-built SVG
canvas; there is no TUI renderer for it and there can be none, and writing one would put archi's
business logic inside rig, which §5h calls a bug. Six of eleven programs in §25 are tier-two.

So the test in §3 asserts: **every generated view has a TUI renderer, and every pane a program
draws itself declares a terminal fallback** - the commands, state and data behind it, reachable
and runnable from the terminal even though the canvas is not drawable there. A program that
offers such a pane and no fallback fails `rig verify`. **That covers the kit tier as well as
the embedded one**, and it has to say so: §11 has three tiers, this test used to name two, and
a kit element cannot carry a fallback of its own because the fallback is per pane and only the
program knows what is behind it (§5h R2).

---
