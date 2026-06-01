# Humify Plan Score

This file is a placeholder describing the plan score report produced by `shared/tools/humify-evaluate-plans.ps1`.

To generate a real score, create matching refactor-plan files under `actual-plans/`, then run:

```powershell
pwsh -NoProfile -File .\shared\tools\humify-evaluate-plans.ps1
```

The evaluator writes a fresh report under `.humify-runs/` (gitignored) by default; pass `-OutputPath` to write elsewhere.

## Summary

Total score: **not run**

Readiness: **not run**

The evaluator now discovers the configured plan list in `tools/humify-evaluate-plans.ps1` and reports a dynamic maximum score.

## Interpretation

- 86-100%: Plan calibration ready.
- 67-85%: Usable with human review.
- 40-66%: Needs planning guidance tuning.
- 0-39%: Not reliable yet.
