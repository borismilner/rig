## 41. Export and import a project or a case

**RULED BY BORIS 2026-09-17, recorded the turn he said it.** ⛔ **HIS OWN SCOPE
LINE, AND IT IS PART OF THE REQUIREMENT RATHER THAN A FOOTNOTE: *"It is not part
of the MVP."* **

### His words, verbatim

> *"I want a mechanism for import and export projects/cases to and from `rig`
> such that on export it will put all the outputs into a well-organized folder
> with a great internal structure and everything needed for proper future import
> and compress it with the greatest strenth possible creating the smallest
> archive file as possible so that then the import can be directed at this file
> and it could fully restore the state reflected in it into `rig`; Don't forget
> future and backward competability."*

### The shape, in one line

**`rig export <project>` produces ONE file. `rig import <file>` restores the
state that file reflects. Nothing else is needed in between.**

| Clause | His word |
|---|---|
| the unit | **a project OR a case** - §39's two containers, both |
| direction | ⛔ **BOTH.** Import is not the afterthought; he named it first |
| the staging form | a **well-organised folder** with **great internal structure** |
| sufficiency | ⛔ ***"everything needed for proper future import"*** |
| the artefact | **one compressed file**, greatest strength, smallest possible |
| the restore | ⛔ ***"fully restore the state reflected in it"*** |
| ⛔ **compatibility** | ***"Don't forget future and backward"*** |

### ⛔ What "fully restore" has to mean, because the store makes it non-obvious

**§39's record is versioned and append-only, and the brief is DERIVED.** So
there are three candidate readings of *"the state reflected in it"* and they are
different products:

| Reading | What it restores |
|---|---|
| **the heads** | current version of every record. **Cheapest, and it loses history** |
| ⛔ **the whole stream** | every version, every progress step, every link. **A record superseded twice reads back with its FIRST wording** - which is §39's own demonstration of what the record is FOR |
| a rendering | the brief, as text. **Not a restore at all** |

⛔ **THE SECOND IS THE ONLY ONE THAT SATISFIES HIM, AND §39 ALREADY ARGUES IT.**
Slice 1's acceptance is *"a requirement written, superseded twice, and its FIRST
wording read back with the session that wrote it."* **An export that drops
history exports a snapshot and calls it a project.** And provenance travels with
it: a version whose author and session are gone is a version nobody can weigh.

### ⛔ Compatibility is the hard clause, and it pulls against "smallest possible"

**He asked for both in one sentence and they are in genuine tension. Name the
trade; do not resolve it by picking the smaller number.**

- **Maximum compression means the newest format at the highest setting.** That is
  the choice most likely to be unreadable by a rig built two years earlier, and
  it is the one that pulls in the heaviest dependency - **§38's dependency bar
  applies and a search is owed before any library is named.**
- ⛔ **THE ARCHIVE NEEDS ITS OWN VERSION, AND THE STORE'S MODEL IS THE
  PRECEDENT, NOT A NEW INVENTION.** §39 already rules: the store carries a schema
  version separate from §21's wire version, migrations are forward-only and
  idempotent, and ⛔ **an UNKNOWN NEWER schema version REFUSES TO START rather
  than guessing or repairing** - because a downgrade silently opening a newer
  store is how a rollback destroys data. **An import is exactly that situation
  arriving through a file.**
- **So: BACKWARD compatibility is a promise the importer keeps** (an older
  archive imports into a newer rig, through the same forward-only migrations).
  ⛔ **FORWARD compatibility is a REFUSAL, not a best effort** - a newer archive
  meeting an older rig is told so by name and imports nothing. **A partial
  import that looks complete is the worst outcome available here.**
- ⛔ **THE MANIFEST IS WHAT MAKES THE REFUSAL POSSIBLE AND IT IS NOT OPTIONAL.**
  The archive's own version has to be readable **without decompressing the
  payload** - otherwise the compatibility check needs the very format it is
  checking. **A plain manifest at a fixed location, in a format that will not
  move.**

### ⛔ The folder is a deliverable in its own right, not a temp directory

**He described the folder BEFORE the compression, and the order is the
requirement:** *"a well-organized folder with a great internal structure"*.

- **It is the thing a human opens when rig will not start.** §39 owes a degraded
  path and `READINESS.txt`'s SCOPE line already says rig *"owes an export before
  anything is deleted"* - **this is that export.**
- **So the layout is legible on its own**, and the compressed file is a
  transport wrapper around something that was already good.
- ⛔ **AN ARCHIVE WHOSE CONTENTS ONLY rig CAN READ FAILS THE DEGRADED PATH**,
  which is the one case the export exists for.

### What this composes with, so nobody builds it twice

| Already ruled | Bearing on this |
|---|---|
| **records are NEVER deleted**, the retention ladder governs observability (§7) | an export is not a prelude to a delete |
| **the dual run** (§39) - rig and the logbook in parallel, the comparison being the point | **an export is the natural artefact to compare**, and the differential evaluation may well be run over one |
| **B66**, the import's grain, and S3+S8's returned specification | ⛔ **THE SAME WORD, TWO MECHANISMS.** B66 is about importing a DOCUMENT into rig; this is about a rig-native archive. **Do not merge them** - but S3+S8's rule binds both: *a projection carries a field iff an exact whole-set equality with the source can be computed by machine in one command with no human judging* |
| **§38's first standing rule** | ⛔ **SEARCH BEFORE CHOOSING A FORMAT.** Compression and archiving are the most solved problems in this project's scope. *"I did not find one"* is only an answer after a search you can describe |

### The acceptance test, which is one command and a comparison

⛔ **EXPORT A PROJECT, IMPORT IT INTO AN EMPTY ESTATE, AND SHOW THE TWO STORES
ARE SET-EQUAL - BY MACHINE, IN ONE COMMAND, WITH NO HUMAN JUDGING.** Every
record, every version, every link, every step, every provenance field.

**A round trip that a person has to eyeball has not been tested**, and this
project has paid for that reading four times.

### Where it sits

⛔ **NOT ON THE MVP PATH. HIS WORDS.** The MVP is §39 slices 1, 2 and 4. **This
is after it**, and it is not a reason to delay the MVP by one session.
