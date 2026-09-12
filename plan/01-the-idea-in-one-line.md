## 1. The idea, in one line

**An app declares what it can do, once. rig projects that onto every way anyone might reach it.**

```
       shelf    graft    archi   snapper   nudge   grabbit
         └────────┴────────┴────────┴────────┴────────┘
                            │  declares once:
                            │  commands · config schema · state · data · events
                            ▼
                 ┌───────────────────────┐
                 │          rig          │
                 │  one registry, one    │
                 │  truth, one contract  │
                 └───────────────────────┘
                            │  projected onto every surface
   ┌──────┬──────┬──────┬───┴──┬──────┬──────┬──────┬──────┐
  GUI    tray   CLI    MCP    HTTP  palette cron   URL   toast
  pane   item  subcmd  tool   route  entry   job  scheme action
   │      │      │      │      │      │      │      │      │
 window hotkey terminal agents scripts keys  time  links  phone
```

The right-hand row is free. An app writes nothing to get a CLI subcommand, an MCP tool, a tray
entry, a schedulable job or a button inside a toast. It declared its commands; rig did the rest.

**And it compounds.** Add a new surface to rig in 2028 - a voice interface, a phone app, a new
UI - and every program already has it, without being touched.

---
