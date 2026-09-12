## 19. The conformance suite

`rig verify ./program` is the gate, and `pkg/rigtest` is the same battery as a Go package so a
program's own CI runs it. This is what makes the contract real.

1. Handshake, wire version negotiation, refusal on mismatch
2. Declaration is valid against the registration schema and claims nothing undeclared
3. Health responds under load and while a command is hung
4. Config schema is valid JSON Schema 2020-12 and round-trips unchanged
5. Config rejection carries a reason a human can act on
6. Every command's argument schema validates, and bad arguments are refused, not half-run
7. Cancellation honoured within 1s, with a terminal event on every path
8. 100 concurrent invocations stay correct and bounded
9. Deadlines honoured: a call given 100ms returns or errors within 200ms
10. Data paging is stable under concurrent writes; subscriptions drop nothing
11. Every undeclared capability call is denied and logged
12. Fuzzed inputs on every message do not panic the program
13. SIGTERM shuts down cleanly inside the grace period
14. SIGKILL mid-command leaves no corrupt state
15. Log hygiene: no secrets, no unbounded lines, valid JSON where claimed
16. **Golden wire: every retained fixture, not the last release.** One frozen conformance
    fixture per wire major, built the day that major ships and archived with its vendored
    source, run against HEAD. Each asserts a recorded **behaviour transcript** - which default
    applied, whether a confirm fired, what each enum decoded to - because a successful round
    trip proves nothing about meaning
17. Runs correctly with rig absent: the tolerant client, the resolved snapshot, one typed
    `unavailable` error, and no second implementation of anything (§5g)
18. **Declaration completeness:** `effects`, `idempotent` and `sensitive` present on every
    command, `coverage` present, `semantics_gen` present. A missing safety field is a refusal,
    not a default
19. **Nothing sensitive is recorded.** A known token passed through a declared-sensitive field
    appears in no segment, in no encoding, at any sampling rate
20. **Renderer fidelity:** every declared schema shape either has a renderer that can express
    it, or an explicit terminal fallback. Asserting that *a* renderer exists is not the test
21. **House rules are enforced in the invoker**, proven by running one denied command through
    every surface from one test
22. **A started program's environment is the constructed allowlist and nothing more**, read
    from `/proc/self/environ` inside a program rig started (§18). Both halves are asserted:
    every variable §18 lists is present and correct, and **no variable outside the list
    survives**, including any `RIG_*` name that is not a registration handle. No grant can
    appear here because no grant exists as a variable (§14) - the item stands so that a future
    reader who reintroduces one fails a test rather than a review
23. **A registered program is scoped on its own connection, and stays scoped.** From inside a
    started program: every estate-wide view returns its own scope only, for the life of the
    connection, and no handshake, reconnect or capability grant changes that (§14). This is
    the item a hosted program must pass, and the reason the predicate is on the connection
    rather than on the environment
24. **A program's declared element list is complete and resolvable** (§5h R3). Every name in it
    exists in the kit at the declared generation, and a name that does not **refuses the
    registration** rather than reaching a page. Item 18 asserts declaration completeness for
    `effects`, `idempotent`, `sensitive`, `coverage` and `semantics_gen`; the element list is
    the sixth and was enforced by review until 2026-09-10
25. **Every kit element renders, in the window, at every generation rig still serves**, and the
    pane holding it declares a terminal fallback (item 20's disjunction, applied to a tier that
    has no declared schema shape). **Asserting that the element exists is not the test**: it is
    rendered and read back. This item is what stops R2 being satisfied by a stub
26. **A hosted program passes items 1-25 unchanged**, from the same package, with no branch in
    the suite. Item 22 is the one that would have been impossible under the old design - a
    hosted plugin is compiled into `rigd` and shares its environment, so it is exempted from
    the allowlist half and held to item 23 instead, which is the assertion that actually
    binds it (§5j)

`fakeapp` is the misbehaving reference program and ships in the repo. It hangs, crashes, leaks,
floods, lies about its schema, ignores cancellation and returns garbage, each on a flag.

**It also behaves, once, in the hardest way any program will.** `fakeapp longrun` is the
`interactive-stream` case from §9: it runs for as long as it is told, emits 20,000 frames with
a cursor, stops in the middle to ask a permission question that has to be answerable from a
toast, reports a per-run cost as a typed field, and parks rather than fails while it waits.
That case exists so the shape is proved before the program that needs it is written - the
alternative is discovering the contract's gaps from the program, which is when they are
expensive.

---
