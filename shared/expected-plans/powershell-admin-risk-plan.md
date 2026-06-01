# Expected Humify Plan: PowerShell Admin Risk Fixture

Fixture: `fixtures/powershell-admin-risk/Repair-Agent.ps1`
Audit: `expected/powershell-admin-risk-audit.md`

## Score Trigger

Admin remediation script can reboot when `-Force` is passed.

## Primary Risk

Operators may trigger a restart while intending only to force repair actions.

## Refactor Stance

No-reboot by default. Separate repair force from restart intent after dry-run and parameter behavior are captured.

## First Safe Slice

Add characterization dry-run tests to capture admin script safety behavior for `-Force`, `-Restart`, and default invocation before changing restart logic.

## Non-Negotiable Gates

| Gate | Status | Evidence |
| --- | --- | --- |
| Behavior gate | Pending | Parameter behavior tests required. |
| Scope gate | Pass | Plan maps to H001. |
| Safety gate | Pending | No restart logic change before dry-run coverage. |
| Rollback gate | Pass | Parameter change can be reverted. |
| Boundary gate | Pass | Script parameters remain explicit. |
| Coverage gate | Pass | Single fixture script is scoped. |
| Generated-code gate | Pass | Hand-written script. |

## Implementation Units

## Unit 1. Capture admin script safety behavior

Goal: Prove current reboot behavior and protect operator-facing output.

Findings addressed:

- H001

Dependencies:

- None

Files:

- `fixtures/powershell-admin-risk/Repair-Agent.ps1`
- `tests/Repair-Agent.Tests.ps1`

Approach:

1. Add Pester tests or a dry-run harness for default, `-Force`, `-Restart`, and `-Force -Restart`.
2. Mock `Restart-Computer`, service commands, and file deletion.
3. Assert `-Force` currently triggers restart and mark it unsafe.

Tests:

- Happy path: repair commands run without restart by default.
- Edge case: `-Force` behavior is captured.
- Error path: service stop errors do not hide summary output.
- Integration: `-Restart` is the only intended reboot path after fix.

Verification:

- Dry-run/Pester tests capture current behavior without touching services.

Rollback:

- Remove the test harness.

Risk:

- Tests must not execute destructive commands. Mock all system operations.

Done when:

- Parameter behavior is pinned safely.

## Unit 2. Separate Force from Restart

Goal: Preserve no-reboot default and require explicit restart intent.

Findings addressed:

- H001

Dependencies:

- Unit 1

Files:

- `fixtures/powershell-admin-risk/Repair-Agent.ps1`
- `tests/Repair-Agent.Tests.ps1`

Approach:

1. Change restart condition to `$Restart` only.
2. Add operator summary showing whether reboot was requested.
3. Keep `-Force` for repair operations only.

Tests:

- Happy path: `-Force` does not reboot.
- Edge case: `-Restart` does reboot through mocked command.
- Error path: repair failures still report operator summary.
- Integration: all system commands remain mocked in tests.

Verification:

- Pester/dry-run tests pass and no real restart command runs.

Rollback:

- Restore previous restart condition.

Risk:

- Existing automation may rely on `-Force` causing restart. Document behavior change.

Done when:

- Reboot requires explicit `-Restart`.

## Steelman Check

- Strongest evidence: line 15 treats `$Force` as reboot intent.
- Biggest uncertainty: existing callers may rely on the old behavior.
- Main false-positive risk: some admin scripts intentionally overload `-Force`, but reboot deserves explicit intent.
- Safety guardrail: dry-run tests and mocked system commands.
- Decision: Proceed with safety harness first.
