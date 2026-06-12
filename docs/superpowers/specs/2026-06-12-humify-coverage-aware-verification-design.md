# Coverage-aware honest verification

- **Date:** 2026-06-12
- **Status:** Approved design, pre-implementation
- **Scope:** Single implementation plan (one capability)

## Problem

humify's value rests on one word: **verified**. Its safety contract is
quarantine → `verify` → roll back on regression, and it presents a surviving change
as "validated / passed."

A blind dogfood run against the Azure-Mapper repo this week exposed that the word is
hollow on most real code. `humify verify` runs the project's build + test commands; when
a file has **no test that exercises it**, the suite passes identically whether the file
exists or not. So `verify` confirms the code *links*, not that behavior held. On
Azure-Mapper, **6 of 9 live modules were build-only** by this measure — yet humify
reported their checks as fully passed, indistinguishable from genuinely behavior-verified
ones.

This is most dangerous exactly where cleanup is most tempting: removing code that compiles
fine but is reached only at runtime (dynamic dispatch, reflection, a runtime-only entry)
passes the build and, with no behavioral test, sails through `verify`.

## Goal

Make humify **honest about the strength of its own verification**: for every change it
validates, distinguish "a test actually executed this code" from "this code merely still
compiles," and surface that verdict everywhere it currently claims "passed." Truth, not a
proxy — measured by real coverage instrumentation.

## Non-goals (v1)

- Changing who can apply or blocking any change (gating stays additive — see below).
- Line/symbol-level verdicts feeding dead-*function* detection (later; data is captured but
  unused).
- Python coverage (fast-follow after Go + JS/TS).
- Making weak suites strong — humify reports the gap, it does not fix the user's tests.

## Design

### Architecture

A new coverage step in the `verify` package (`internal/humify/verify`) runs the project's
detected test command **under coverage instrumentation** and writes an index to
`.humify/coverage.json`:

```json
{
  "tool": "go|c8|nyc",
  "measured": true,
  "files": {
    "src/modules/report-builder.js": { "covered": false, "lines": [] },
    "src/modules/network-rules.js":  { "covered": true,  "lines": [12, 13, 40] }
  }
}
```

Coverage is produced by `verify` (which `plan` triggers if needed), **automatically
whenever a Provider detects coverage tooling**, suppressible with `--no-coverage` for speed.
It is computed **once** and cached — "does the suite exercise file X" is a property of the
suite, not of any one cleanup. `apply` **reads** the cached `.humify/coverage.json` to label
items; its own baseline/post `verify` runs execute build+test for the rollback gate only and
do **not** recompute coverage. The proven quarantine → verify → rollback path is untouched:
coverage is a read-only input to it, exactly as `dead_module` was an add-only finding.

### Verdict model

Each applyable item's target file gets one honest verdict, computed by crossing the
coverage index with `verify`'s existing build/test results:

| Verdict | Condition |
|---|---|
| `behavior-verified` | build passed **and** the file has ≥1 covered line |
| `build-only` | build passed but the file has **0 coverage** |
| `unmeasured` | coverage tooling absent or failed — stated plainly, never faked |

The verdict is written into `plan.json` items, the apply `Result`, and the quarantine
`manifest.json`. Output stops emitting a bare "validated/passed" for removals and reads,
e.g.: `Quarantined report-builder.js — build-only: no test executed this file.`

### Coverage providers

A narrow interface with per-tool adapters:

```go
type Provider interface {
    Detect(root string) bool                 // is this tool usable here?
    Run(root string) (Report, error)         // run suite under coverage, parse
}
type Report struct {
    Tool  string
    Files map[string]FileCoverage            // repo-relative path -> coverage
}
type FileCoverage struct { Covered bool; Lines []int }
```

v1 adapters:
- **Go** — `go test -coverprofile=<tmp> ./...`, parse the coverprofile (native, trivial;
  also dogfoods humify on itself).
- **JS/TS** — `c8`/`nyc` producing Istanbul JSON or lcov; parse to per-file covered lines.
  This is where the gap was exposed (Azure-Mapper is JS).

When no provider `Detect`s, the report is `measured: false` and every verdict is
`unmeasured`. Never a silent pass.

### Gating (decided: additive)

A `build-only` verdict does **not** change who can apply or block anything. The build and
existing tests did pass; the change still quarantines (and is reversible). humify simply
**warns loudly and records the verdict**. Honesty without friction; the user judges. A
strict "require behavior-verification" mode is a possible later config, explicitly out of
v1.

## Data / integration points

- `internal/humify/verify`: new `coverage.go` (Provider + adapters); produce a `Report` when
  a Provider detects (unless `--no-coverage`); persist `.humify/coverage.json`.
- `internal/humify/state`: register `coverage.json`.
- `internal/humify/plan`: attach `Verification string` (verdict) to applyable items from the
  coverage index.
- `internal/humify/apply`: `Result` + `Manifest` gain a `verification` field; messages name
  the verdict.
- Render/output: replace bare pass wording for removals with the verdict.

## Testing

- **Unit:** verdict crossing as a truth table (covered / uncovered / unmeasured × build
  pass/fail); each adapter's parser against a fixture coverprofile / Istanbul JSON.
- **E2E:** a tiny real Go module with one tested file and one untested-but-compiled file →
  assert `behavior-verified` vs `build-only`. Skips when the go toolchain is absent (matches
  the existing rollback E2E).
- **Dogfood:** run on Azure-Mapper; confirm the 8 quarantined dead modules come back
  `build-only` (no tests) and the unit-tested live modules come back `behavior-verified` —
  reproducing the hand-built map automatically.

## Risks / limitations

- **Coverage run cost** — instrumented test runs are slower. Mitigated by running coverage
  once (baseline), not per-apply.
- **Per-tool parsing fragility** — Istanbul/lcov/coverprofile formats drift; isolate each in
  its adapter with fixture tests, and degrade to `unmeasured` on parse failure rather than
  guessing.
- **Coverage ≠ assertion strength** — a test can execute a file without asserting on it.
  `behavior-verified` means "executed," not "well-tested." This is still strictly more honest
  than today and is documented as such.

## Follow-ons (explicitly later)

- Python (`coverage.py`) adapter.
- Line-level verdicts feeding **dead-function/export** detection (the runRBACChecks /
  `_subFromScope` cases) — the natural next "reversible verified automated cleanup" type.
- Optional strict gating mode in `humify.config.json`.
