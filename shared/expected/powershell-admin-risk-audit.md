# Expected Humify Audit: PowerShell Admin Risk Fixture

Fixture: `fixtures/powershell-admin-risk/Repair-Agent.ps1`

Classification: High-risk refactor candidate
Machine-shaped confidence: Low
Refactor required: Yes, after tests

## Findings

```markdown
## H001. Repair script can reboot when Force is used for unrelated actions (HIGH)
File: fixtures/powershell-admin-risk/Repair-Agent.ps1
Lines: 1-17
Symptom: Running the script with `-Force` can restart the endpoint even when the operator did not pass `-Restart`.
Causal chain:
1. The script uses `-Force` for service and file cleanup semantics.
2. Line 15 treats `$Force` as equivalent to `$Restart`.
3. An operator can trigger a reboot while intending only to force the repair operation.
Repro trigger: Run `Repair-Agent.ps1 -Force` on a production endpoint.
Machine-shaped confidence: Low
Signals: admin automation, reboot risk, operator-facing behavior
Fix: Add dry-run/tests for parameter combinations and operator summary output before separating `-Force` from reboot behavior.
```

## Cleared Items

- This is not primarily a readability problem. It is a high-risk admin automation behavior.

## Required classification

| Category | Expected |
| --- | --- |
| Overall | High-risk refactor candidate |
| Machine-shaped confidence | Low |
| Refactor required | Yes, after tests |

