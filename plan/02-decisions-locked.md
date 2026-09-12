## 2. Decisions locked

- **Name:** rig. CLI `rig`, daemon `rigd`, module `github.com/boris-milner/rig`. The UI shell
  UI shell component is **the window**. `turret` was chosen for it after the lathe turret that
  carries many tools and rotates the right one into place, and dropped on 2026-09-10: the
  product is rig, one name is enough, and a second name for a component only Boris ever opens
  buys nothing. §27 assumption 5 is resolved by this line. The command is `rig window`.
- **Two binaries, not one.** `rigd` is the daemon; `rig` is the CLI and TUI. Measured: keeping
  the terminal stack out of the daemon recovers **8.89 MiB resident and 27 ms of cold start**,
  and it is a build-graph change only (§17). **Minus an unmeasured keyring term**: that rung
  bundled go-keyring with four TUI libraries, and §22 puts go-keyring in the daemon because
  §13's per-program scoping is unenforceable anywhere else.
- **No imports.** An in-house program does not link any rig code beyond a dumb pipe (§5d). All
  behaviour lives in the daemon, so all behaviour upgrades without a rebuild.
- **One socket, binary frames, and no gRPC.** Measured at 6.2µs round trip and 1.8M one-way
  records/second on this laptop (§4), which is far more than anything here needs. A gRPC server
  on a unix socket costs **9.80 MiB resident** before it carries a byte, which is 58% of the
  whole footprint budget for HTTP/2 machinery a local socket does not need. So: protobuf
  messages, hand-framed, length-prefixed. No shared memory, no io_uring, no zero-copy codec.
  That complexity is not bought either.
- **Two directions.** Apps call rig for supply (config, storage, notify, UI). rig calls apps for
  control (start, stop, invoke, query, reload). Both over the same connection.
- **Declarative wins.** Where an app *describes* something, rig can improve it forever. Where an
  app *calls* something, that call is frozen. So the contract is biased hard toward description.
- **Commands declare properties; surfaces declare requirements.** A declaration never names a
  surface. It says what the command *is* - interactive, streaming, needs a display, how long it
  runs, whether it confirms - and each surface says what it can carry. rig computes the
  projection. This is the only construction under which a surface added in 2028 matches a
  declaration written in 2026, and it costs one enum today (§5e).
- **Adoption is per service, and coverage is declared.** A program adopts one service at a time
  and declares only the commands worth projecting. Every declaration carries `coverage`, which
  defaults to `partial`, and every surface renders it, because a surface that implies
  completeness lies (§5k).
- **A program may run inside rig, and pays four prices for it.** Most in-house programs are
  separate binaries. A hosted plugin is compiled into `rigd`, declares and projects identically,
  and gives up independent upgrade, crash containment, OS-level capability enforcement and a
  zero footprint contribution to do it (§5j).
- **The client tolerates rig's absence; it does not reimplement rig.** There is no second
  implementation of config, storage, secrets or logging inside any program. The stub reconnects,
  queues, and returns one typed `unavailable` error. rig writes an already-resolved snapshot to
  disk for the few things that must survive its absence (§5g). This replaces the 300-line
  fallback this plan carried before the attack, which was policy in the one component that can
  never be upgraded.
- **No hot upgrade.** Measured, both paths built: once clients are told rig is going, the
  handover's entire marginal benefit over a plain restart is **1.9 ms per upgrade**, which at
  weekly upgrades is 5.2 seconds a year. It was priced at four defect classes, including a
  silent replay path less safe than `kill -9`. Lifecycle notice plus a sub-100 ms restart
  delivers the requirement instead (§5g, §18).
- **There is an authorization layer.** `house rules` says which kinds of caller may run which
  kinds of command, and which must ask first. It lives in the kernel's invoker, so no surface can
  forget it, and the default rule set carries only the two `url` rules, so nothing already
  working breaks (§13).
- **Operating is a credential, not a uid, and reading is a different credential.** Without the
  first correction every client on a one-user machine satisfies the operator predicate, which
  is exactly how the compound secret leak worked. Without the second, the fix locks Boris's own
  agents out of the estate they exist to read (§14, §15). `introspect` arrives in the
  environment of what he starts; `operate` is **asked for, not carried** (§14).
- **Nothing sensitive is recorded.** Redaction is declared at registration as JSON pointers and
  compiled once into byte spans over the wire encoding: **82.5 ns** in the hot path, against
  3100 ns for redacting at record time. Anything the `secrets` service returns is never recorded
  at all, only the key name (§15).
- **Time is CLOCK_BOOTTIME, and every deadline is absolute.** Lease, budget and expiry arithmetic
  in the daemon runs on boottime, stored as `boot_id + deadline`, never as a remaining TTL and
  never on the client's clock. This laptop suspends nightly and Go's monotonic clock does not
  advance across suspend (§16).
- **Storage: rig manages, the app opens.** rig owns location, migrations, backup, integrity and
  retention; the app opens the file and runs its own queries at full speed. Also an assumption
  in §27.
- **The peers service supersedes AgentBox.** rig builds the best inter-client coordination
  substrate it can (§16), proves it against four gates, and only then do the agents move off
  AgentBox. AgentBox keeps running untouched until that happens. Decided 2026-09-10.
- **rig drives the desktop, and the driver is its own binary.** AgentBox's synthetic input comes
  over as `hand`, a program that declares its commands like any other, in `cmd/righand` and not
  in `rigd` (§5m). It is the **fourth** binary, after `rigd`, `rig` and `rigwindow`, and the line
  above about two binaries is untouched by it: that decision is about keeping the terminal stack
  out of the daemon, not a cap on how many processes rig ships. The test each new one has to pass
  is the window's - a dependency the daemon must not link - and this is the second thing to pass
  it. Measured: the X11 dependency alone is **1,015,911 bytes**, and the successor
  backend is a C library, so a daemon that absorbed the driver would need cgo - which is the same
  argument §17 already makes for keeping the window out. The consequence is the point: because
  `righand` is reachable only through rig, the display becomes the **one lease in this estate a
  fencing token can genuinely fence**, which §16 says of no other and now states as an
  exception.
- **History is buffered and never fsynced.** Losing under a second of history to a power cut is
  acceptable and was agreed; durability is what makes event logging expensive, so it is not
  bought. A process crash loses at most one flush interval, because the kernel already holds the
  rest.
- **An agent Boris runs may read every part of rig.** Introspection is complete for it, not
  scoped: the estate-wide views, the full capability map, any client's history, every trace,
  every config resolution. It is acting for him, and a picture of the estate that is silently
  partial is worse to him than no picture. §14 said the opposite by implication and named the
  contradiction without deciding it - "seven things that sat outside the old six-view list, one
  of which §9 tells an agent to read first". This line decides it. The credential splits in two
  (§14): **reading everything and acting on everything are different grants**, and only the
  first is handed out by default. Decided 2026-09-10.
- **Linux and X11 first**, on this laptop. Nothing knowingly non-portable, and no cross-platform
  claim until it is tested.
- **Dependencies:** best library for the job, newest published version, **and a measured binary
  cost**. Hand-rolling needs a product reason; so does any dependency that moves the size budget
  in §17, because in Go resident memory tracks binary size.

---
