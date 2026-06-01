# Expected Humify Plan: Messy Human Fixture

Fixture: `fixtures/messy-human/importCustomers.ts`
Audit: `expected/messy-human-audit.md`

## Score Trigger

The fixture is classified as `Needs targeted cleanup` with refactor required. The function mixes validation, normalization, persistence, logging, and import reporting in one workflow.

## Primary Risk

Changing customer validation can accidentally change persistence behavior because the current workflow owns both import policy and database writes.

## Refactor Stance

Behavior-preserving. Import semantics should not change until accepted, skipped, created, and updated outcomes are characterized.

Public behavior changes:

- None expected.

## First Safe Slice

Add characterization tests for imported/skipped counts, invalid rows, existing-customer update, new-customer create, and warning behavior.

## Non-Negotiable Gates

| Gate | Status | Evidence |
| --- | --- | --- |
| Behavior gate | Pending | Characterization tests must be added first. |
| Scope gate | Pass | Unit maps to H001 in `expected/messy-human-audit.md`. |
| Safety gate | Pending | No extraction until import outcomes are pinned. |
| Rollback gate | Pass | Tests and helper extraction can be reverted independently. |
| Boundary gate | Pass | No public API change expected. |
| Coverage gate | Pass | Single fixture file is the scoped target. |
| Generated-code gate | Pass | File is hand-written fixture code. |

## Implementation Units

## Unit 1. Capture customer import behavior

Goal: Protect current import semantics before structure changes.

Findings addressed:

- H001

Dependencies:

- None

Files:

- `fixtures/messy-human/importCustomers.ts`
- `tests/importCustomers.characterization.test.ts`

Approach:

1. Add test doubles for `db.customers.findByEmail`, `db.customers.update`, `db.customers.create`, and `logger.warn`.
2. Cover valid new customer, existing customer, missing name, invalid email, and inactive status rows.
3. Assert returned `{ imported, skipped }` counts and warning calls.

Tests:

- Happy path: valid new customer is normalized and created.
- Edge case: inactive status maps to `active: false`.
- Error path: missing name and invalid email are skipped and logged.
- Integration: existing customer is updated instead of duplicated.

Verification:

- Characterization tests pass against the current implementation without production-code changes.

Rollback:

- Remove the new test file only.

Risk:

- Tests may expose ambiguous current behavior. Document ambiguity before changing code.

Done when:

- Current import behavior is pinned by tests.

## Unit 2. Extract customer row validation and normalization

Goal: Separate import policy from persistence mechanics.

Findings addressed:

- H001

Dependencies:

- Unit 1

Files:

- `fixtures/messy-human/importCustomers.ts`
- `fixtures/messy-human/customerImportPolicy.ts`
- `tests/customerImportPolicy.test.ts`

Approach:

1. Extract validation into a function that returns either a normalized customer or a skip reason.
2. Keep database create/update behavior inside `importCustomers`.
3. Preserve existing warning text unless intentionally changed later.

Tests:

- Happy path: valid rows normalize name, email, and active status.
- Edge case: inactive status maps to inactive.
- Error path: missing name and invalid email return skip reasons.
- Integration: existing import characterization tests still pass.

Verification:

- Policy tests and characterization tests pass.

Rollback:

- Inline the extracted policy and remove its tests.

Risk:

- Normalization can drift if tests do not assert exact output shape.

Done when:

- Validation/normalization are named and tested separately from persistence.

## Steelman Check

- Strongest evidence: `importCustomers` handles validation, logging, create/update writes, and reporting in one function.
- Biggest uncertainty: Real repository dependency interfaces are not present in the fixture.
- Main false-positive risk: The function is short enough that extraction could be overkill in a tiny script.
- Safety guardrail: Characterization tests come before extraction.
- Decision: Proceed with tests first, then targeted extraction.

