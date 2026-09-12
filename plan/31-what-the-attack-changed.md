## 31. What the attack changed

Ten adversarial seats were run against this document at `73d7ff4` on 2026-09-10, one per
failure surface, plus one pointed the other way at features the shape makes possible. All ten
returned; every citation in the attack record was independently re-checked and held. The record
is `logbook/projects/rig/attacks/2026-09-10-rig-architecture.md`; each seat's full findings, and
the four measurement harnesses, are in `logbook/projects/rig/agent-work/`.

**Verdict: the architecture survived, and the fixes were substantial.** Nothing found argued for
abandoning it. The daemon shape, the projection model and the single-serialization-point
advantage all held. What failed was almost entirely the layer below: budgets set without
measurement, enforcement mechanisms that could not detect what they promised to prevent, and one
security composition that no single surface owned.

### The finding that mattered most, because no seat could see it alone

**Any agent could read any secret.** History recorded what every call returned, verbatim, with
zero redaction in 1104 lines (`grep -ci redact` returned 0). `secrets.get` is a call. And every
client satisfied the operator predicate, because it was "the uid that runs the daemon" and there
is one daemon per uid. So one `rig history --client=X` handed over another program's token. The
storage half and the isolation half were each a finding of their own; the harm existed only in
their product.

### What changed

| Area | Was | Now |
|---|---|---|
| Projection | A command named the surfaces it appears on | Commands declare **properties**, surfaces declare requirements, rig computes it (§5e) |
| The stub | A 300-line un-upgradable copy of config, storage, secrets and logging | Tolerant client + resolved snapshot + lifecycle notices (§5g) |
| Hot upgrade | A locked requirement | **Not built.** Measured at 1.9 ms marginal benefit against four defect classes (§18) |
| Authorization | Absent. Zero such language in the document | `house rules`, in the kernel's invoker (§13a) |
| The operator | The uid running the daemon | **Nothing is minted at all.** `introspect` is decided at connect time from whether the connection registered as a program, and `operate` is an authorisation decision per call (§14). Two rejected drafts - a 0600 file, then an environment variable - are recorded there so neither is reached for again |
| Redaction | Absent | Declared at registration, compiled to byte spans, 82.5 ns (§15) |
| Leases | A TTL and a fencing token | A liveness witness, two-step expiry, `rig peers run`, and an honest statement of what a token can fence (§16) |
| Time | Unnamed | `CLOCK_BOOTTIME`, absolute deadlines, a resume grace epoch (§16) |
| Wire compatibility | One archived binary, round-trip asserted | A frozen fixture per major kept forever, behaviour transcripts, banned enum zero, `semantics_gen`, and a support window with a date (§21) |
| Isolation proof | A two-client content test | Observational equivalence over two worlds (§14) |
| Footprint | Seven budget lines, three unreachable | Rebuilt from a measured 12-rung ladder, plus a binary-size ratchet (§17) |
| Modularity gates | Two of four could not fail | `modules-matrix`, symbol budgets, analyzers under every tag set and on the frontend (§5i) |
| Adoption | Implicitly all-or-nothing | Per service, `coverage: partial` by default (§5k) |
| Where a program runs | Always its own binary | Hosted plugins exist, with four costs stated (§5j) |

### What was not attacked

The peers service's *feature list* (as opposed to its semantics), the wire numbers in §4, the
visual system, and the plan's own milestone ordering. The `advocate` pass has since run
(`logbook/projects/rig/advocate/2026-09-10-post-fix.md`); the blind-spot sweep over this
applied diff was run afterwards and is recorded in the attack file.

---
