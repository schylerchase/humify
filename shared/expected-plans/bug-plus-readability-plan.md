# Expected Humify Plan: Bug Plus Readability Fixture

Fixture: `fixtures/bug-plus-readability/saveCustomer.ts`
Audit: `expected/bug-plus-readability-audit.md`

## Score Trigger

The audit separates a real email normalization bug from a readability concern.

## Primary Risk

Refactoring validation and persistence before fixing the bug can hide the behavior change.

## Refactor Stance

Fix the bug first with a failing test, then consider targeted cleanup.

## First Safe Slice

Lock failing email normalization behavior with a test and fix the save payload before cleanup.

## Non-Negotiable Gates

| Gate | Status | Evidence |
| --- | --- | --- |
| Behavior gate | Pending | Failing test for trimmed saved email required. |
| Scope gate | Pass | Units map to H001 and H002. |
| Safety gate | Pending | Cleanup waits until bug fix lands. |
| Rollback gate | Pass | Bug fix and cleanup are separate. |
| Boundary gate | Pass | Public API unchanged. |
| Coverage gate | Pass | Single fixture file is scoped. |
| Generated-code gate | Pass | Hand-written code. |

## Implementation Units

## Unit 1. Lock failing email normalization behavior

Goal: Fix the observed bug before readability cleanup.

Findings addressed:

- H001

Dependencies:

- None

Files:

- `fixtures/bug-plus-readability/saveCustomer.ts`
- `tests/saveCustomer.test.ts`

Approach:

1. Add a failing test proving saved email should be trimmed.
2. Change repository payload to use `normalizedEmail`.
3. Keep validation structure unchanged.

Tests:

- Happy path: trimmed email is saved.
- Edge case: leading/trailing spaces are removed.
- Error path: invalid email still throws.
- Integration: repository receives normalized payload.

Verification:

- The new failing test passes after the one-line fix.

Rollback:

- Revert the test and payload change.

Risk:

- If callers expect untrimmed email, this is a behavior change. Confirm domain expectation in real repos.

Done when:

- Email normalization bug is fixed and tested.

## Unit 2. Keep validation cleanup separate

Goal: Avoid mixing bug fix and structural cleanup.

Findings addressed:

- H002

Dependencies:

- Unit 1

Files:

- `fixtures/bug-plus-readability/saveCustomer.ts`

Approach:

1. Only extract validation if more rules exist or tests show growing complexity.
2. If extracted, keep repository write behavior unchanged.
3. Do not combine this with the bug fix commit.

Tests:

- Happy path: valid customer still saves.
- Edge case: blank name still throws.
- Error path: invalid email still throws.
- Integration: repository payload remains normalized.

Verification:

- Existing tests pass after any extraction.

Rollback:

- Inline validation helper.

Risk:

- Extraction may be unnecessary for this small fixture. Prefer no-op if bug fix resolves the maintenance issue.

Done when:

- Cleanup is either intentionally skipped or performed in a separate behavior-preserving step.

## Steelman Check

- Strongest evidence: line 12 validates `normalizedEmail`; line 24 saves `input.email`.
- Biggest uncertainty: whether trimming persisted email is intended in every product context.
- Main false-positive risk: input email preservation could be deliberate, though validation using trimmed email argues against that.
- Safety guardrail: failing test before one-line bug fix.
- Decision: Fix bug first; do not refactor first.

