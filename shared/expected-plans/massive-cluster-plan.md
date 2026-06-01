# Expected Humify Plan: Massive Cluster Fixture

Fixture: `fixtures/massive-cluster/reportWorkflow.ts`
Audit: `expected/massive-cluster-audit.md`

## Score Trigger

The audit clusters a large workflow finding: parsing, validation, enrichment, totals, rendering, and persistence are fused.

## Primary Risk

Report behavior can change in several dimensions during a cleanup, especially rejected rows, totals, rendered HTML, and saved metadata.

## Refactor Stance

Behavior-preserving. Treat this as a workflow cluster and add golden output before extracting units.

## First Safe Slice

Capture report workflow output with golden tests for accepted rows, rejected rows, totals, rendered HTML, saved metadata, and operator warnings before extraction.

## Non-Negotiable Gates

| Gate | Status | Evidence |
| --- | --- | --- |
| Behavior gate | Pending | Golden report output tests required. |
| Scope gate | Pass | Plan maps to H001. |
| Safety gate | Pending | No extraction before golden workflow output. |
| Rollback gate | Pass | Units can be reverted independently. |
| Boundary gate | Pending | DB/renderer adapter boundaries need naming. |
| Coverage gate | Pass | Single cluster fixture is scoped. |
| Generated-code gate | Pass | Hand-written code. |

## Implementation Units

## Unit 1. Capture report workflow output

Goal: Protect current end-to-end report behavior.

Findings addressed:

- H001

Dependencies:

- None

Files:

- `fixtures/massive-cluster/reportWorkflow.ts`
- `tests/reportWorkflow.golden.test.ts`
- `tests/fixtures/reportWorkflow/*.json`

Approach:

1. Add fixtures for valid rows, missing account ID, disabled account, and missing account lookup.
2. Mock database and renderer dependencies.
3. Assert returned HTML, saved report metadata, totals, row count, rejected count, and warnings.

Tests:

- Happy path: valid rows enrich and render.
- Edge case: disabled account is rejected.
- Error path: missing account lookup is skipped and logged.
- Integration: report save receives expected metadata.

Verification:

- Golden workflow tests pass before production code changes.

Rollback:

- Remove golden tests and fixtures.

Risk:

- Golden output can over-specify formatting. Keep snapshots focused on behavior.

Done when:

- Current workflow output is pinned.

## Unit 2. Extract row validation and normalization

Goal: Separate row policy from enrichment and rendering.

Findings addressed:

- H001

Dependencies:

- Unit 1

Files:

- `fixtures/massive-cluster/reportWorkflow.ts`
- `fixtures/massive-cluster/reportRows.ts`

Approach:

1. Extract validation/normalization for accepted and rejected rows.
2. Preserve warning behavior.
3. Keep database enrichment in the workflow for now.

Tests:

- Happy path: valid rows normalize fields.
- Edge case: default region remains `"unknown"`.
- Error path: rejected rows keep current warning behavior.
- Integration: golden workflow tests still pass.

Verification:

- Unit tests and golden tests pass.

Rollback:

- Inline row validation.

Risk:

- Moving warnings can change operator-facing behavior. Preserve warning strings.

Done when:

- Row policy is named and tested.

## Unit 3. Extract totals and persistence boundaries

Goal: Make reporting calculations and side effects explicit.

Findings addressed:

- H001

Dependencies:

- Unit 1
- Unit 2

Files:

- `fixtures/massive-cluster/reportWorkflow.ts`
- `fixtures/massive-cluster/reportTotals.ts`
- `fixtures/massive-cluster/reportPersistence.ts`

Approach:

1. Extract regional totals calculation as pure logic.
2. Wrap report saving in a small persistence helper or adapter.
3. Keep renderer call visible in orchestration.

Tests:

- Happy path: regional totals match golden output.
- Edge case: missing amount defaults to zero.
- Error path: missing account remains skipped.
- Integration: golden workflow tests still pass.

Verification:

- Golden tests prove behavior did not move.

Rollback:

- Inline totals and persistence helpers.

Risk:

- Too many extra helpers can make navigation worse. Stop when ownership is clear.

Done when:

- Workflow reads as validate, enrich, total, render, save.

## Steelman Check

- Strongest evidence: one function owns five workflow phases and broad dependencies.
- Biggest uncertainty: real renderer and database contracts are mocked in the fixture.
- Main false-positive risk: small repos may accept a single workflow function; this fixture is intentionally clustered.
- Safety guardrail: golden output before extraction.
- Decision: Proceed with cluster-level plan, not duplicate findings.

