# Expected Humify Plan: Risky Refactor Fixture

Fixture: `fixtures/risky-refactor/applyDiscounts.ts`
Audit: `expected/risky-refactor-audit.md`

## Score Trigger

The fixture is classified as `High-risk refactor candidate`. It is not primarily machine-shaped, but discount stacking, category eligibility, and minimum-spend behavior can change silently during cleanup.

## Primary Risk

The function looks simple enough to refactor immediately, but it encodes customer-facing pricing behavior. A helper extraction could accidentally change totals.

## Refactor Stance

Behavior-preserving. No discount semantics should change until current behavior is characterized, including behavior that may look questionable.

Public behavior changes:

- None expected.

## First Safe Slice

Add characterization tests for multiple discounts, category-specific discounts, minimum-spend failures, zero discounts, quantity handling, and current stacking behavior.

## Non-Negotiable Gates

| Gate | Status | Evidence |
| --- | --- | --- |
| Behavior gate | Pending | Pricing behavior must be captured first. |
| Scope gate | Pass | Units map to H001 in `expected/risky-refactor-audit.md`. |
| Safety gate | Pending | No extraction before pricing tests. |
| Rollback gate | Pass | Tests and helper extraction can be reverted independently. |
| Boundary gate | Pass | No public API change expected. |
| Coverage gate | Pass | Single fixture file is the scoped target. |
| Generated-code gate | Pass | File is hand-written fixture code. |

## Implementation Units

## Unit 1. Capture current discount behavior

Goal: Protect pricing semantics before readability changes.

Findings addressed:

- H001

Dependencies:

- None

Files:

- `fixtures/risky-refactor/applyDiscounts.ts`
- `tests/applyDiscounts.characterization.test.ts`

Approach:

1. Add tests for subtotal, no discounts, one discount, multiple discounts, category-specific discount, and minimum-spend skip.
2. Assert exact `subtotal`, `discountTotal`, and `total` values.
3. Include a test that documents current stacking behavior when multiple discounts apply.

Tests:

- Happy path: one discount reduces total as currently implemented.
- Edge case: no discounts returns subtotal and zero discount total.
- Error path: minimum spend not met skips discount.
- Integration: multiple discounts stack according to current behavior.

Verification:

- Characterization tests pass against the current implementation.

Rollback:

- Remove the characterization test file only.

Risk:

- Tests may reveal current behavior that product later wants to change. Capture first, decide later.

Done when:

- Pricing behavior is pinned by tests.

## Unit 2. Extract eligible total calculation

Goal: Make category and minimum-spend behavior easier to inspect without changing totals.

Findings addressed:

- H001

Dependencies:

- Unit 1

Files:

- `fixtures/risky-refactor/applyDiscounts.ts`
- `tests/applyDiscounts.characterization.test.ts`

Approach:

1. Extract `calculateEligibleTotal(items, discount)`.
2. Keep the public `applyDiscounts` return shape unchanged.
3. Re-run characterization tests to prove totals are unchanged.

Tests:

- Happy path: category-matching items produce expected eligible total through public output.
- Edge case: discount without category applies to all items.
- Error path: minimum spend remains enforced in the same place as current behavior.
- Integration: full discount characterization suite still passes.

Verification:

- Characterization tests pass with identical totals.

Rollback:

- Inline `calculateEligibleTotal`.

Risk:

- Moving the minimum-spend check into the helper would change responsibility. Keep that decision explicit.

Done when:

- Eligibility calculation is named and current behavior is preserved.

## Unit 3. Document or isolate discount stacking policy

Goal: Make discount stacking an explicit policy instead of an accidental loop behavior.

Findings addressed:

- H001

Dependencies:

- Unit 1
- Unit 2

Files:

- `fixtures/risky-refactor/applyDiscounts.ts`
- `tests/applyDiscounts.characterization.test.ts`

Approach:

1. Introduce a named helper or local comment only if it clarifies the current stacking policy.
2. Keep behavior unchanged unless a separate product decision approves a behavior change.
3. If behavior should change, create a new finding/plan rather than mixing it into the refactor.

Tests:

- Happy path: stacking behavior remains captured.
- Edge case: multiple category-specific discounts behave as before.
- Error path: invalid or empty discount list retains current behavior.
- Integration: public return shape remains `{ subtotal, discountTotal, total }`.

Verification:

- Characterization tests pass and the stacking policy is named.

Rollback:

- Remove the helper/comment and restore the prior loop structure.

Risk:

- Naming a policy can make accidental behavior look intentional. Mark it as "current behavior" unless confirmed.

Done when:

- A reader can identify where stacking behavior lives without simulating the whole function.

## Steelman Check

- Strongest evidence: `applyDiscounts` calculates subtotal, category eligibility, minimum-spend behavior, and stacking in one flow.
- Biggest uncertainty: Product intent for stacking and rounding is unknown.
- Main false-positive risk: The code is compact and readable enough that extraction may not be worth it unless future changes are expected.
- Safety guardrail: Characterization tests before any extraction.
- Decision: Proceed only with behavior tests first; refactor after pricing semantics are pinned.

