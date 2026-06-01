# Humify Score

This file is a placeholder describing the score report produced by `shared/tools/humify-evaluate.ps1`.

To generate a real score, create matching audit files under `actual/`, then run:

```powershell
pwsh -NoProfile -File .\shared\tools\humify-evaluate.ps1
```

The evaluator writes a fresh report under `.humify-runs/` (gitignored) by default; pass `-OutputPath` to write elsewhere.

## Summary

Total score: **not run**

Readiness: **not run**

The evaluator now discovers the configured fixture list in `tools/humify-evaluate.ps1` and reports a dynamic maximum score.

## Interpretation

- 86-100%: Ready to use on a real repo.
- 67-85%: Usable with human review.
- 40-66%: Needs framework tuning.
- 0-39%: Not reliable yet.
