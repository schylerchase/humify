# Expected Humify Plan: Auth Risk Fixture

Fixture: `fixtures/auth-risk/canAccessProject.ts`
Audit: `expected/auth-risk-audit.md`

## Score Trigger

Authorization code fails open in a catch block.

## Primary Risk

Changing permission logic without tests can either keep a security bug or accidentally lock out valid users.

## Refactor Stance

Security-preserving with explicit tests. Deny/allow outcomes must be captured before changing the catch block.

## First Safe Slice

Add characterization tests to capture authorization outcomes for anonymous, admin, non-confidential project, confidential member, confidential non-member, and thrown membership lookup before changing behavior.

## Non-Negotiable Gates

| Gate | Status | Evidence |
| --- | --- | --- |
| Behavior gate | Pending | Authorization tests required. |
| Scope gate | Pass | Plan maps to H001. |
| Safety gate | Pending | No refactor before deny/allow matrix. |
| Rollback gate | Pass | Catch-block change can be reverted. |
| Boundary gate | Pass | Function signature unchanged. |
| Coverage gate | Pass | Single fixture file is scoped. |
| Generated-code gate | Pass | Hand-written code. |

## Implementation Units

## Unit 1. Capture authorization outcomes

Goal: Build a security matrix before changing fail-open behavior.

Findings addressed:

- H001

Dependencies:

- None

Files:

- `fixtures/auth-risk/canAccessProject.ts`
- `tests/canAccessProject.test.ts`

Approach:

1. Add tests for each allow/deny case.
2. Add a test showing thrown membership lookup currently grants access.
3. Mark the thrown-case behavior as unsafe current behavior.

Tests:

- Happy path: admin can access.
- Edge case: member can access assigned confidential project.
- Error path: thrown lookup is captured.
- Integration: anonymous and non-member users are denied.

Verification:

- Tests document current behavior before security fix.

Rollback:

- Remove the new test file.

Risk:

- Capturing fail-open behavior may look like endorsement. Label it unsafe.

Done when:

- Authorization behavior matrix exists.

## Unit 2. Change thrown membership lookup to deny

Goal: Make authorization fail closed.

Findings addressed:

- H001

Dependencies:

- Unit 1

Files:

- `fixtures/auth-risk/canAccessProject.ts`
- `tests/canAccessProject.test.ts`

Approach:

1. Change the catch block to return `false`.
2. Update the thrown-case test to expect denial.
3. Keep all valid allow paths passing.

Tests:

- Happy path: admin/member valid access still allowed.
- Edge case: public project remains accessible to members.
- Error path: thrown membership lookup denies.
- Integration: complete deny/allow matrix passes.

Verification:

- Security test suite passes.

Rollback:

- Revert catch behavior and test expectation.

Risk:

- If exceptions previously masked data-shape bugs, fail-closed may expose them. That is acceptable but should be monitored.

Done when:

- Authorization fails closed.

## Steelman Check

- Strongest evidence: catch block returns `true` in authorization logic.
- Biggest uncertainty: whether exceptions are possible with typed callers.
- Main false-positive risk: fixture is contrived, but fail-open security behavior remains high severity.
- Safety guardrail: deny/allow matrix before code change.
- Decision: Proceed with security tests first.
