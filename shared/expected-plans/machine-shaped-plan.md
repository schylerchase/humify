# Expected Humify Plan: Machine-Shaped Fixture

Fixture: `fixtures/machine-shaped/processData.ts`
Audit: `expected/machine-shaped-audit.md`

## Score Trigger

The fixture is classified as `Machine-shaped readability risk` with High machine-shaped confidence. The code hides the domain contract behind `processData`, `data`, `item`, `options`, and `any`.

## Primary Risk

The function produces plausible output for malformed input, so downstream behavior can silently depend on manufactured defaults.

## Refactor Stance

Behavior-preserving until the real record contract is named. Do not delete defaults or change metadata semantics until current behavior is captured.

Public behavior changes:

- None during characterization.
- Possible later behavior changes must be explicit after the domain contract is known.

## First Safe Slice

Add characterization tests for current field defaults, trimming behavior, status fallback, metadata inclusion, null rows, and missing options.

## Non-Negotiable Gates

| Gate | Status | Evidence |
| --- | --- | --- |
| Behavior gate | Pending | Current generic behavior must be captured first. |
| Scope gate | Pass | Units map to H001 in `expected/machine-shaped-audit.md`. |
| Safety gate | Pending | Domain renaming waits for characterization. |
| Rollback gate | Pass | Tests, type extraction, and rename can be reverted independently. |
| Boundary gate | Pending | Public callers are unknown in the fixture. |
| Coverage gate | Pass | Single fixture file is the scoped target. |
| Generated-code gate | Pass | File is not marked generated. |

## Implementation Units

## Unit 1. Capture current generic processing behavior

Goal: Preserve observable behavior before naming or restructuring the processor.

Findings addressed:

- H001

Dependencies:

- None

Files:

- `fixtures/machine-shaped/processData.ts`
- `tests/processData.characterization.test.ts`

Approach:

1. Add tests for valid rows, missing `id`, missing `name`, missing `status`, null rows, missing options, and `includeMetadata`.
2. Assert exact output shape, including empty-string defaults and `"unknown"` status.
3. Avoid changing production code in this unit.

Tests:

- Happy path: id, name, and status are copied/trimmed.
- Edge case: null rows are ignored.
- Error path: missing fields receive current defaults.
- Integration: metadata appears only when `includeMetadata === true`.

Verification:

- Characterization tests pass against the current implementation.

Rollback:

- Remove the new characterization test file.

Risk:

- Tests may lock in behavior that should later change. Mark undesirable behavior as captured, not endorsed.

Done when:

- Current generic processor behavior is pinned.

## Unit 2. Name the domain contract

Goal: Replace generic shapes with explicit input and output types.

Findings addressed:

- H001

Dependencies:

- Unit 1

Files:

- `fixtures/machine-shaped/processData.ts`

Approach:

1. Identify or introduce domain names for the record being processed.
2. Replace `any[]` and `any` with named input/options/output types.
3. Keep the exported function name if external callers depend on it; otherwise rename in a later unit.

Tests:

- Happy path: typed input produces same output.
- Edge case: optional fields keep current defaults.
- Error path: null rows remain ignored if current behavior is preserved.
- Integration: characterization tests still pass.

Verification:

- Existing characterization tests pass with types introduced.

Rollback:

- Revert the type additions without changing tests.

Risk:

- The fixture does not reveal the true domain. If this were a real repo, inspect callers before naming.

Done when:

- The function signature documents the record contract instead of using `any`.

## Unit 3. Split field normalization from metadata enrichment

Goal: Make the processing decisions locally readable and remove repeated field blocks.

Findings addressed:

- H001

Dependencies:

- Unit 1
- Unit 2

Files:

- `fixtures/machine-shaped/processData.ts`

Approach:

1. Extract a pure normalizer for the record fields.
2. Extract metadata enrichment as a separate branch.
3. Remove obvious narration comments once function names carry intent.

Tests:

- Happy path: normalized fields match previous output.
- Edge case: missing options produce no metadata.
- Error path: missing fields preserve captured defaults.
- Integration: full characterization suite still passes.

Verification:

- Characterization suite passes and code no longer relies on repeated field-by-field blocks.

Rollback:

- Inline the extracted helper functions.

Risk:

- Extracting too early could hide the need for real domain validation. Keep behavior-preserving until callers are known.

Done when:

- The function has named steps for normalization and enrichment.

## Steelman Check

- Strongest evidence: generic names, `any`, repeated field defaults, narration comments, and hidden metadata behavior all appear in one function.
- Biggest uncertainty: The real domain is unknown from the isolated fixture.
- Main false-positive risk: Some generated adapters intentionally look repetitive; this file has no generated marker.
- Safety guardrail: Characterization tests before naming, extraction, or behavior changes.
- Decision: Proceed with tests first, then domain naming and behavior-preserving extraction.

