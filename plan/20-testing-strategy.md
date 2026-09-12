## 20. Testing strategy

| Layer | Approach |
|---|---|
| Wire contract | 100% coverage, table-driven, plus the golden-wire compatibility test |
| Supervisor and scheduler | `testing/synctest` for all timing. Deterministic, microseconds, no `time.Sleep` in any test |
| Chaos | 10000 randomised kill / hang / flood / restart sequences against `fakeapp`, seed printed on failure |
| Suspend | A real-clock test with an injected `CLOCK_BOOTTIME` jump. `synctest` cannot model suspend, and this laptop suspends nightly, so the mandate above would otherwise make the whole class untestable |
| Isolation | Observational equivalence over two worlds (§14), including the runtime directory, the config tree, the state tree and the process table |
| Authorisation | Two assertions §14 and §12 both rely on and neither could test as prose. **Do Not Disturb does not suppress a gating `ask`:** with DND on, a command needing `confirm` still reaches a surface and still blocks, while a lifecycle notice in the same run goes to the centre (§12, §13a). **An elevation prompt names its caller:** the rendered question carries principal, client kind, pid, command and arguments, and an interactive caller's question arrives on that caller's own surface rather than on whichever is present (§14) |
| Introspection | §14's **three-run battery**, and it lives here rather than in §19's program-side suite: it is a property of *rig* over several clients, and the program under test is refused `introspect` and has no peers to be complete about. Run 1 ungranted A sees nothing of B; **run 2 A sees B completely - a scoped answer where a complete one was owed fails as hard as a leak**, modulo the coverage log; run 3 no `sensitive` value surfaces at any sampling rate |
| Fuzz | Registration parser, JSON Schema inputs, frame decoding, the postMessage bridge |
| CLI | golden transcripts for every command including failures, driven by an exec harness over the built binary. **`testscript` was named here and never adopted - retired 2026-09-11**, landed as `e03c41d` using stdlib `os/exec`, because §20 names a SHAPE and the library was plumbing |
| Generated UI | Golden snapshot per schema shape |
| Frontend | Vitest on the bridge and stores; Playwright driving the real window - click, Esc, Tab, Enter, tray, theme switch, crash and recovery |
| Contrast | Measured in a real browser, **both themes** - the token set is generated (§6), so the gate asserts the generator's output, not one static page. A failing ratio fails the build. **`make contrast` is the gate and CI runs it**; until 2026-09-10 it invoked a tool that had never existed and CI never called it, so the rule was prose |
| Contrast at element scale | **Every kit element, on every ground text lands on, in both themes** (§5h R5). Three things the page-level gate does not see, each found by measurement on 2026-09-10 and each now asserted: a **focus ring** is non-text contrast and owes 3:1 against the surface behind it, not 4.5:1 against nothing; **`opacity` and `color-mix` are invisible to a DOM audit**, which reads the declared colour and not the painted one, so anything dimmed that way is sampled from pixels; and the ground set is the **five** in `theme.js`'s `textGrounds`, tint included, not the four the catalogue draws |
| Integration | The pilot runs the real `shelf` binary in CI, not a mock |
| Gates | 90% on `internal/`, 100% on wire and stub, no call without a deadline, no program id in rig code, no registry handle outside the kernel, no meaningful enum zero, kernel and stub symbol budgets, `make modules-matrix` green, `make bench-size` within the ratchet |

---
