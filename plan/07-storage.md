## 7. Storage

### ⛔ THE RETENTION LADDER, AND HE IS RIGHT THAT HE SAID THE FIRST HALF BEFORE

**BORIS, 2026-09-17, verbatim, recorded the turn he said it:** *"As I already
said in the past, I want everything to be persisted but without harming
performance so some level of risk of data-loss is acceptable to allow
buffering. I don't want persistance to bloat the disk-space so information that
is older than a month is to be compressed with the maximal force and stored in
an archive and information older than 3 months shall be deleted unless pinned
by the user."*

✅ **HE DID SAY THE BUFFERING HALF BEFORE AND IT IS ALREADY LOCKED.** Checked
rather than assumed: **§2, decisions-locked** - *"History is buffered and never
fsynced. Losing under a second of history to a power cut is [the accepted
trade]"*. **So the performance-over-durability trade is not new, it is
CONFIRMED**, and this is now the second time he has stated it.

⛔ **WHAT IS NEW IS THE LADDER AND THE PIN.**

| Age | What happens | New? |
|---|---|---|
| fresh | buffered, not fsynced. Sub-second loss on a power cut is accepted | ✅ already §2 |
| **> 1 month** | **compressed with MAXIMAL force and moved to an archive** | ⛔ **NEW** |
| **> 3 months** | **DELETED** | ⛔ **NEW** |
| **pinned by the user** | **never deleted** | ⛔ **NEW, and it is a whole capability** |

**Partial precedent, so the ladder is not landing on nothing:** §7 already says
history segments *"age out in 30 days anyway"* and §15 already specifies
**retention by size, age AND rate**. **The one-month rung matches the 30 days
that already exist; the three-month rung and the pin do not exist at all.**

### ⛔ THE COLLISION THAT MUST BE SETTLED BEFORE ANY OF THIS IS BUILT

⛔ **HE SAID "EVERYTHING", AND ONE HOUR EARLIER HE SAID "NOTHING IS EVER
LOST".** §9's A1, his words: *"The AI agent working with `rig` should manage its
scratch-pads in `rig` **so that nothing is ever lost**"*. **A three-month delete
is a data-loss path, and `record` is the kind it would hurt most.**

⛔ **THE TWO READINGS LEAD TO MATERIALLY DIFFERENT PRODUCTS AND A SEAT MUST NOT
PICK ONE SILENTLY:**

| Reading | What it means |
|---|---|
| **the ladder governs OBSERVABILITY** - history, traces, agent interactions, the high-volume streams | The continuity record is exempt. Consistent with A1, with §39's whole purpose, and with the fact that every rung he named is about **disk bloat**, which records do not cause: **68 work-item bodies are 6,374 bytes TOTAL** |
| **the ladder governs EVERYTHING, records included** | Then a decision made in March is gone in June unless somebody pinned it, and §39's continuity record cannot be the thing it is specified to be |

⛔ **THE LEAD'S READING IS THE FIRST, AND IT IS A READING RATHER THAN A
RULING.** The evidence for it is that every rung he named is a disk-space
remedy, and the record store is measured in kilobytes while history is measured
in hundreds of megabytes - **so the ladder aimed at records would cost the
project its memory and save nothing.** ⛔ **PUT IT TO HIM. Do not build the
delete rung against records on this reading, and do not narrow his word
"everything" in any document without his answer** - a silent narrowing of his
scope is the exact move this project has recorded four times.

### The pin is a capability, not a flag

**"Unless pinned by the user" introduces a user action that outranks an
automatic process**, and it owes: a surface to pin from (the GUI and the CLI),
a record of WHO pinned and WHEN, visibility of what is pinned before the delete
runs rather than after, and an answer for what happens when the archive rung
has already compressed something that is later pinned. **It is also the natural
answer to S9's one-copy risk**, which currently has no user-facing control at
all.

**"Maximal force" is a real specification and §38 binds it:** do not write a
compressor. Name the candidates, measure them on real segments, and choose -
`zstd --ultra -22` and `xz -9e` are the obvious two, and the decompression cost
on a cold archive read is part of the measurement, not a footnote.

rig owns everything about a database except what is in it.

| rig owns | The program owns |
|---|---|
| Where the file lives, and telling the program | Its schema |
| Running migrations before the program starts | Writing the migrations |
| Backup, restore, retention, integrity check | Its queries and transactions |
| A browsable viewer in the UI | Its own indexes |
| Reporting size, growth and last backup | |

The program opens the file directly, so a query costs what SQLite costs and not what a socket
costs. The tedious half - migration running, backup, integrity, retention - upgrades
independently, which is the half that is copy-pasted fifteen times today.

Nothing may hold the only copy of anything in SQLite. Files on disk are the record; the database
is an index and is rebuildable. **That rule binds rig's own storage too**, and §15 breaks it
unless the history's interning dictionary travels inside the segments rather than in the index -
see §15, where the plan's previous advice to delete the index and rescan destroyed data.

### rig backs itself up, or the rule above is a rule for other programs

The row two tables up sells backup, restore, retention and integrity to fifteen programs, and
until this section nothing said what happens to rig's own state. Two of the things it holds
are records **of record** with no source to rebuild from - the audit log and the notification
centre - so for those the rule above is not merely unmet, it is unmeetable by rescanning.

| rig's own state | Rebuildable from | Backed up |
|---|---|---|
| Config tree | nothing - it is hand-edited source | **yes**, and it is the one thing whose loss cannot be worked around |
| History segments | nothing, but they age out in 30 days anyway | no. Retention already says they are disposable |
| **Audit log** | nothing. It is the record | **yes** |
| **Notification centre** | nothing. §12 makes it the record of record | **yes** |
| Registry, peers WAL, resolved snapshots | live registration and re-resolution | no. Rebuilt at start |
| A program's database | the program's own files, per the rule above | already covered, per program |

**One command, and it is the same one programs get.** `rig backup` writes the four rows marked
yes to one archive; `rig restore` puts them back on a machine where `rigd` has never run. That
is the missing half of M15's "fresh machine to a working rig in one command" - which as
written installs the software and carries none of the history. **M11**, beside the per-program
backup it already ships, because building it twice is the alternative.

---
