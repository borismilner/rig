## 28. Repository, versioning and the Makefile

### Versioned, three ways, and they are not the same number

| Version | Governs | Changes when |
|---|---|---|
| **Product** | The `rig` binary. Semver, git tag `v1.4.2` | Any release |
| **Wire** | The contract programs speak. Major only, in the path | A breaking contract change, which rig then serves *alongside* every previous version, forever |
| **Registration schema** | The shape of what a program declares | Independently, and old declarations keep parsing |
| **Semantics generation** | What the declared names *mean*: capability meanings, declaration defaults, schema dialect | Whenever any of those changes meaning. Old registrations stay pinned to the generation they declared, and a generation nobody claims is retired (§21) |

`rig version` prints all three plus the build SHA and date. A program's `rig version` mismatch
is never an error, only a line in the Programs view.

### Git

- One repository, `github.com/boris-milner/rig`, git from the first commit.
- Conventional commits, `type(scope): description`, imperative, under 72 characters.
- Every milestone in §23 ends in a tagged commit with the demo recorded in the message.
- `CHANGELOG.md` generated from the commit log at release, never hand-edited.
- Release is a tag; CI builds, runs the full gate, and publishes the artefacts.
- The wire contract lives in `proto/` and generated code is checked in, so a clone builds with
  no code generation step and a diff shows contract changes plainly.

### The Makefile is the interface to the repository

Every important operation has a target, `make help` lists them all with descriptions parsed from
the file itself, and CI runs the same targets a human does. There is no operation documented in
a README that is not a target, because a documented command drifts and a target does not.

| Group | Targets |
|---|---|
| Build | `build` `build-rigd` `build-rig` `install` `run` `dev` `clean` |
| Test | `test` `test-unit` `test-race` `test-chaos` `test-e2e` `fuzz` `cover` |
| Quality | `lint` `fmt` `vet` `contrast` `verify` `audit` `modules` `modules-matrix` `build-minimal` |
| Generate | `generate` `proto` `schema` `types` `docs` |
| Measure | `bench` `bench-ipc` `bench-idle` `bench-scale` `bench-size` `profile` |
| Operate | `doctor` `logs` `apps` `up` `down` |
| Ship | `deps-check` `tidy` `release` `package` `ci` |

---
