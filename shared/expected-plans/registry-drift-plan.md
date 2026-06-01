# Expected Humify Plan: Registry Drift Fixture

Fixture: `fixtures/registry-drift/cloudResourceRegistry.ts`
Audit: `expected/registry-drift-audit.md`

## Score Trigger

Registry drift can silently omit uploaded resources from snapshot diff output.

## Primary Risk

Changing IDs directly can break existing saved snapshots while still leaving another surface out of sync.

## Refactor Stance

Behavior-preserving. Preserve old aliases until active UI, upload, export, snapshot, and report contracts are covered.

## First Safe Slice

Capture registry drift behavior with characterization tests before introducing a canonical resource registry.

## Non-Negotiable Gates

| Gate | Status | Evidence |
| --- | --- | --- |
| Behavior gate | Pending | Characterization test required for active input to snapshot context. |
| Scope gate | Pass | Plan maps to H001. |
| Safety gate | Pending | No ID rewrite before fixture coverage. |
| Rollback gate | Pass | Registry extraction can be reverted independently. |
| Boundary gate | Pending | Saved snapshot compatibility aliases must be explicit. |
| Coverage gate | Pass | Single fixture file is scoped. |
| Generated-code gate | Pass | Hand-written fixture. |

## Implementation Units

## Unit 1. Capture active input to snapshot behavior

Goal: Prove every active resource input reaches snapshot diff context before movement.

Findings addressed:

- H001

Dependencies:

- None

Files:

- `fixtures/registry-drift/cloudResourceRegistry.ts`
- `tests/cloudResourceRegistry.test.ts`

Approach:

1. Add a fixture textarea payload for `in_private_links`.
2. Assert that snapshot context currently drops private links.
3. Add the desired coverage expectation as the behavior gate for the next unit.

Tests:

- Happy path: firewalls still reach snapshot context.
- Edge case: private links uploaded through the active ID are visible to the test.
- Error path: missing textarea values stay empty without throwing.
- Integration: active registry IDs are compared against snapshot IDs.

Verification:

- Characterization test exposes the stale registry mismatch.

Rollback:

- Remove the new test file only.

Risk:

- The first test may document broken current behavior. Label it as a characterization failure to fix.

Done when:

- The drift is reproducible in a focused test.

## Unit 2. Introduce a canonical resource registry

Goal: Replace duplicated IDs with one source of truth while preserving compatibility aliases.

Findings addressed:

- H001

Dependencies:

- Unit 1

Files:

- `fixtures/registry-drift/cloudResourceRegistry.ts`
- `tests/cloudResourceRegistry.test.ts`

Approach:

1. Add a canonical registry that contains type, active textarea ID, export file, and legacy aliases.
2. Build upload map and snapshot diff IDs from that registry.
3. Keep `in_private_endpoints` as a legacy alias until saved snapshots can migrate.

Tests:

- Happy path: private links from `in_private_links` reach snapshot context.
- Edge case: legacy `in_private_endpoints` snapshots still parse.
- Error path: invalid or missing values still produce safe empty arrays.
- Integration: every active registry row is reachable from snapshot context.

Verification:

- Registry coverage test passes.

Rollback:

- Revert registry extraction and keep Unit 1 test as a known failure.

Risk:

- Compatibility aliases can hide stale naming longer than desired. Track migration separately.

Done when:

- One canonical registry drives all resource contract surfaces in the fixture.

## Steelman Check

- Strongest evidence: active input and snapshot diff use different IDs for the same resource.
- Biggest uncertainty: real repos may intentionally support multiple aliases.
- Main false-positive risk: old alias may be compatibility behavior, not dead drift.
- Safety guardrail: tests for active IDs and legacy aliases before cleanup.
- Decision: Proceed with characterization before registry extraction.
