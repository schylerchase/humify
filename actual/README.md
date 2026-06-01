# Actual Humify Outputs

Put real Humify audit outputs here before running the evaluator.

Use:

```text
HUMIFY-AI-INSTRUCTIONS.md
prompts/humify-calibration.md
```

Expected filenames:

```text
clean-audit.md
messy-human-audit.md
machine-shaped-audit.md
generated-ignore-audit.md
risky-refactor-audit.md
framework-boilerplate-audit.md
machine-shaped-no-comments-audit.md
generated-header-only-audit.md
hand-edited-generated-audit.md
compatibility-wrapper-audit.md
ugly-stable-audit.md
bug-plus-readability-audit.md
auth-risk-audit.md
powershell-admin-risk-audit.md
massive-cluster-audit.md
```

Refactor-plan calibration outputs belong in `actual-plans/`:

```text
messy-human-plan.md
machine-shaped-plan.md
risky-refactor-plan.md
```

Run:

```powershell
pwsh -NoProfile -File .\shared\tools\humify-evaluate.ps1
```
