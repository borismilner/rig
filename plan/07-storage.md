## 7. Storage

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
