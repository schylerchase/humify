# Expected Humify Plan: Hand-Edited Generated Fixture

Fixture: `fixtures/hand-edited-generated/apiClient.generated.ts`
Audit: `expected/hand-edited-generated-audit.md`

## Score Trigger

A generated file contains a local hand-edited patch. The patch may disappear on regeneration.

## Primary Risk

The null-status fallback is real behavior living in a file explicitly marked not to edit.

## Refactor Stance

Behavior-preserving. Move the patch to a hand-edited adapter only after current behavior is captured.

## First Safe Slice

Capture patched client behavior for null status and normal status before regeneration or adapter extraction.

## Non-Negotiable Gates

| Gate | Status | Evidence |
| --- | --- | --- |
| Behavior gate | Pending | Tests for null-status fallback required. |
| Scope gate | Pass | Plan maps to H001. |
| Safety gate | Pending | No regeneration before tests. |
| Rollback gate | Pass | Adapter extraction can be reverted. |
| Boundary gate | Pass | Generated client boundary is explicit. |
| Coverage gate | Pass | Single fixture file is scoped. |
| Generated-code gate | Pending | Generated body excluded; hand-edited patch targeted. |

## Implementation Units

## Unit 1. Capture patched client behavior

Goal: Preserve the hand-edited generated-file behavior before moving it.

Findings addressed:

- H001

Dependencies:

- None

Files:

- `fixtures/hand-edited-generated/apiClient.generated.ts`
- `tests/ticketClient.characterization.test.ts`

Approach:

1. Mock `fetch` for ticket responses with null, missing, and normal status.
2. Assert current fallback to `"open"`.
3. Document that this is a regeneration hazard.

Tests:

- Happy path: normal status is preserved.
- Edge case: null status becomes `"open"`.
- Error path: missing status follows current fallback.
- Integration: caller still receives `TicketDto`.

Verification:

- Characterization tests pass before any movement.

Rollback:

- Remove the test file.

Risk:

- Tests can make the generated-file patch look intentional. Label it as captured current behavior.

Done when:

- The local patch behavior is pinned.

## Unit 2. Move fallback into a hand-edited adapter

Goal: Keep generated code replaceable while preserving ticket behavior.

Findings addressed:

- H001

Dependencies:

- Unit 1

Files:

- `fixtures/hand-edited-generated/apiClient.generated.ts`
- `fixtures/hand-edited-generated/ticketClientAdapter.ts`
- `tests/ticketClient.characterization.test.ts`

Approach:

1. Add a hand-edited adapter that calls the generated client.
2. Move null-status fallback into the adapter.
3. Remove local patch from generated file.

Tests:

- Happy path: adapter preserves normal status.
- Edge case: adapter maps null status to `"open"`.
- Error path: generated client remains replaceable.
- Integration: callers use the adapter.

Verification:

- Characterization tests pass through the adapter.

Rollback:

- Restore the local patch and remove the adapter.

Risk:

- Callers may still import the generated client directly. Search and migrate callers in real repos.

Done when:

- Generated file contains no local patch and behavior lives in hand-edited code.

## Steelman Check

- Strongest evidence: generated header plus local patch lines 15-18.
- Biggest uncertainty: real import graph is absent from the fixture.
- Main false-positive risk: some teams intentionally patch generated fixtures, but production clients should not.
- Safety guardrail: tests before moving patch.
- Decision: Proceed with adapter extraction after characterization.

