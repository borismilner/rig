## 21. Versioning

- The wire is versioned by major in the path. rig serves **every** wire version it has ever
  shipped; that is the promise that makes independent upgrade real.
- **One frozen conformance fixture per wire major**, built the day that major ships, archived
  with its full vendored source tree so it can be rebuilt in 2035, and **retained forever**. The
  previous plan archived only the last release, and "N works against N+1" repeated is not
  "1 works against N".
- **One fixture's shape is declared unproven, and this is where that has to be said.** The
  `interactive-stream` case (§9, §19) is modelled on `graft run`, a program that does not
  exist yet, and its hardest row - an inbound question on an outbound stream - has no working
  implementation anywhere in the estate: the graft session measured Claude Code 2.1.267
  emitting *no* permission event under `--permission-prompt-tool`, and agentbox runs
  `bypassPermissions` for the same reason. **Retained forever applies to a fixture, not to a
  guess**, so this one may be corrected once a real answered prompt exists, and the correction
  reads as planned rather than as a broken promise. Everything else in §9's five rows is
  ordinary and stands.
- **Each fixture asserts a behaviour transcript**, recorded at ship time: which default applied,
  whether a confirm fired, what each enum decoded to. A round-trip test passes while a new
  `effects` value silently decodes to zero on an old binary and a destructive command reports
  itself read-only.
- **Meaningful enum zero is banned.** Every proto enum reserves `*_UNSPECIFIED = 0`; an unknown
  value is a hard refusal at the daemon boundary, never a zero-value fall-through. Lint gate,
  §20.
- Capability differences are negotiated at connect: the program says what it supports, rig uses
  what it has, and the gap is visible in the Programs view rather than a failure.
- The stub's public surface is enumerated in one file with a symbol budget in `make ci`, and
  every connection reports its `stub_build`, so rig can name every program carrying an old pipe.

### The version that is not the wire: `semantics_gen`

"Old declarations keep parsing" is not the property that matters. Turning `network` from a
best-effort proxy into a real namespace breaks a 2027 declaration that parses perfectly - the
bytes are fine and the meaning moved. Nothing in a wire major can express that.

So a registration carries **one integer, `semantics_gen`**, distinct from the wire major. It
pins, for that program, for its lifetime: what every capability name means, what every
declaration default is, and which JSON Schema dialect and validator behaviour apply to its
schemas. rig carries a generation table instead of pretending meanings are immutable. It is the
cheapest option available, and the only version number that can ever be *retired* - once no
registration claims a generation, its row goes.

**Measured 2026-09-11: neither version number is ever COMPARED, so "upgrade"
is today exactly "daemon restart".** Two findings, from reading every use:

- **The wire version is announced and never compared.** It is read in one place
  and written into the handshake response. An old program and a new daemon
  transact regardless.
- **`semantics_gen` is validated and never compared against a previous
  registration.** Validation refuses a non-positive value, registration logs it,
  and the projection carries it - but nothing compares one registration's
  generation to the last one's, **which is the comparison the field's name
  implies.**

**So the generation table above is specified and unbuilt**, and the state
ownership matrix has two cells it cannot fill for that reason: "what does an
upgrade do that a restart does not" is undecidable from the code because today
the answer is "nothing". **Whether it should stay nothing is an open question
and it is Boris's**, because it decides whether a program can be running against
a meaning that has moved underneath it.

### The escape hatch, which costs nothing now and everything later

Serving every wire version forever, with no stated end, quietly grows the test matrix without
limit and leaves old programs hollowing out unannounced. There are 2 hits for deprecation
language in this document and neither is a policy. Three sentences fix it:

1. **A support window ships with the version.** Every wire major declares, in this plan, on the
   day it ships, the date it moves from *supported* to *frozen*. A date, not a feeling.
2. **Two states, explicitly.** *Supported* means new services are reachable and the full
   conformance suite applies. *Frozen* means it still connects and existing behaviour still
   works, and it is permanently excluded from new services and new conformance items. That caps
   the matrix at (supported majors x 22), which is the number that has to stay small.
3. **`rig doctor` names every connected program still speaking a frozen major**, and the
   Programs view carries the state. The alarm already exists; it just has to be wired.

Nothing is ever switched off. A frozen major keeps working forever - it simply stops growing,
in public, on a schedule.
