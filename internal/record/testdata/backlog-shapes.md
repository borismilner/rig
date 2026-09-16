# A fixture for the backlog parser, and it is NOT a copy of rig's backlog

Hand-written. It exists to hold one row of every SHAPE the real document uses,
so a change to the parser is visible without waiting for somebody to edit the
real backlog. The acceptance demonstration still runs against the real file and
must keep doing so - section 39's test is rig on rig, and a fixture cannot meet
it. This one answers a different question: does the parser still read the same
way it did before it moved.

| # | Item | Evidence | Adopter | State |
|---|---|---|---|---|
| B1 | **A plain open row** | evidence | nobody | **OPEN** |
| B2 | ~~**A struck row, which is how the document closes one**~~ **DONE 2026-01-01** | evidence | a seat | **DONE** |
| B3 | **DONE, a terminal lead in the ITEM cell with no strikethrough** | evidence | a seat | **DONE** |
| B4 | **A row whose work is open** | evidence | nobody | **CLOSED, a terminal lead in the STATE cell only** |
| B5 | **DONE. a full stop after the disposition, which a word scan loses** | evidence | a seat | **DONE** |
| B6 | **REJECTED, the third terminal word** | evidence | a seat | **REJECTED** |
| B7 ~~**A row missing the pipe after its id, so the id cell absorbs the title**~~ | evidence | a seat | **DONE** |
| B8 | **A row whose bold never closes before the pipe | evidence | nobody | **OPEN** |
| B9 | **RETRACTED, the fourth** | evidence | a seat | **RETRACTED** |
| B10 | **An open row whose state cell says something non-terminal** | evidence | nobody | **argued, in flight** |

The row below repeats an id deliberately: the scanner keeps the FIRST and drops
the rest, and nothing else in the suite proves that.

| B1 | **A duplicate id that must be ignored** | evidence | nobody | **DONE** |
