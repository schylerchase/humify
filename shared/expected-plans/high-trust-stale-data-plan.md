# Expected Humify Plan: High-Trust Stale Data Fixture

Fixture: `fixtures/high-trust-stale-data/medicationPrices.ts`
Audit: `expected/high-trust-stale-data-audit.md`

## Score Trigger

High-trust medication pricing data is presented with confidence wording while freshness metadata is hidden.

## Primary Risk

Changing labels or data handling without tests can either preserve misleading confidence wording or accidentally alter pricing display semantics.

## Refactor Stance

Trust-preserving and behavior-preserving. Price values should not change until freshness display behavior is covered.

## First Safe Slice

Add display tests for current, stale, and unknown source dates before changing the verified price wording.

## Non-Negotiable Gates

| Gate | Status | Evidence |
| --- | --- | --- |
| Behavior gate | Pending | Freshness display tests required. |
| Scope gate | Pass | Plan maps to H001. |
| Safety gate | Pending | No trust wording change before tests. |
| Rollback gate | Pass | Rendering change is localized. |
| Boundary gate | Pass | Public price input shape can remain compatible. |
| Coverage gate | Pass | Single fixture file is scoped. |
| Generated-code gate | Pass | Hand-written fixture. |

## Implementation Units

## Unit 1. Capture pricing freshness display behavior

Goal: Protect high-trust UI semantics before changing confidence wording.

Findings addressed:

- H001

Dependencies:

- None

Files:

- `fixtures/high-trust-stale-data/medicationPrices.ts`
- `tests/medicationPrices.test.ts`

Approach:

1. Add tests for current, stale, and unknown `lastUpdated` values.
2. Assert that rendered output includes source and freshness metadata.
3. Keep price formatting unchanged.

Tests:

- Happy path: current source date shows verified with date.
- Edge case: stale source date shows stale-aware wording.
- Error path: missing date does not claim verification.
- Integration: price, source, and date render together.

Verification:

- Freshness behavior is captured without changing price values.

Rollback:

- Remove the new tests only.

Risk:

- Tests may require product wording decisions. Mark ambiguous wording as pending user decision.

Done when:

- Current, stale, and unknown freshness scenarios are covered.

## Unit 2. Make trust wording date-aware

Goal: Prevent stale curated data from appearing currently verified.

Findings addressed:

- H001

Dependencies:

- Unit 1

Files:

- `fixtures/high-trust-stale-data/medicationPrices.ts`
- `tests/medicationPrices.test.ts`

Approach:

1. Include `lastUpdated` in the rendered card.
2. Change "Verified price" to date-aware wording.
3. Add stale and unknown labels without changing numeric price formatting.

Tests:

- Happy path: fresh price displays date-aware verified wording.
- Edge case: stale price displays stale-aware wording.
- Error path: missing date displays unknown freshness.
- Integration: existing price string remains unchanged.

Verification:

- Freshness test matrix passes.

Rollback:

- Revert render wording and test expectations.

Risk:

- Product may need stricter freshness thresholds for real medication data.

Done when:

- Confidence wording cannot outlive source freshness metadata.

## Steelman Check

- Strongest evidence: `lastUpdated` exists but rendered output omits it while saying "Verified price."
- Biggest uncertainty: exact freshness threshold is product-specific.
- Main false-positive risk: fixture is simplified and may not represent a full app's provenance UI.
- Safety guardrail: tests preserve price display while changing trust wording.
- Decision: Proceed with tests before wording changes.
