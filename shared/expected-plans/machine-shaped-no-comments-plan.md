# Expected Humify Plan: Machine-Shaped No-Comments Fixture

Fixture: `fixtures/machine-shaped-no-comments/formatRecords.ts`
Audit: `expected/machine-shaped-no-comments-audit.md`

## Score Trigger

The fixture is classified as `Machine-shaped readability risk` with High confidence. It uses generic formatting names, parallel arrays, and untyped configuration without narration comments.

## Primary Risk

Field/default behavior can drift silently because the domain contract is encoded by array index.

## Refactor Stance

Behavior-preserving until the record contract is named.

## First Safe Slice

Capture current formatter behavior for parallel array defaults, string trimming, metadata, and missing fields before naming or restructuring.

## Non-Negotiable Gates

| Gate | Status | Evidence |
| --- | --- | --- |
| Behavior gate | Pending | Characterization tests must be added first. |
| Scope gate | Pass | Plan maps to H001. |
| Safety gate | Pending | No array replacement before behavior is pinned. |
| Rollback gate | Pass | Tests and refactor units are separate. |
| Boundary gate | Pending | Callers are unknown in the fixture. |
| Coverage gate | Pass | Single fixture file is scoped. |
| Generated-code gate | Pass | No generated marker. |

## Implementation Units

## Unit 1. Capture current formatter behavior

Goal: Protect current output before replacing generic structure.

Findings addressed:

- H001

Dependencies:

- None

Files:

- `fixtures/machine-shaped-no-comments/formatRecords.ts`
- `tests/formatRecords.characterization.test.ts`

Approach:

1. Add tests for full records, missing fields, null fields, non-string values, and metadata.
2. Assert the current parallel array defaults exactly.
3. Do not rename or extract code yet.

Tests:

- Happy path: full record formats with trimmed strings.
- Edge case: missing fields receive current defaults.
- Error path: null values receive current defaults.
- Integration: metadata is added only when configured.

Verification:

- Characterization tests pass against current code.

Rollback:

- Remove the characterization test file.

Risk:

- Tests may preserve behavior that should later be redesigned; mark it as captured behavior only.

Done when:

- Current formatter behavior is pinned.

## Unit 2. Replace parallel arrays with a named field policy

Goal: Make the domain contract explicit and reduce index-coupling risk.

Findings addressed:

- H001

Dependencies:

- Unit 1

Files:

- `fixtures/machine-shaped-no-comments/formatRecords.ts`

Approach:

1. Introduce a named field policy object that pairs each field with its default.
2. Replace index-based access with key-based iteration.
3. Keep output behavior unchanged.

Tests:

- Happy path: formatted output is unchanged.
- Edge case: missing fields still use captured defaults.
- Error path: null fields remain handled.
- Integration: metadata behavior is unchanged.

Verification:

- Characterization tests pass.

Rollback:

- Restore the parallel arrays.

Risk:

- Naming the policy without real caller context may still be generic. Inspect callers in a real repo.

Done when:

- Defaults are no longer coupled by array index.

## Steelman Check

- Strongest evidence: field/default parallel arrays and generic config hide the domain contract.
- Biggest uncertainty: real callers and domain terms are unknown.
- Main false-positive risk: some low-level serializers intentionally use field maps.
- Safety guardrail: characterization before naming.
- Decision: Proceed with tests first.

