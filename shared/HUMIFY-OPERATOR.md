# Humify Operator Guide

Use this when you want Codex to run Humify on a repository.

## Standard Prompt

```text
Run Humify on this repo.
```

## Expected Flow

Humify should run in this order:

1. Capture repo state.
2. Build HUMIFY-MAP.md.
3. Build HUMIFY-HEATMAP.md.
4. Produce HUMIFY-AUDIT.md.
5. Run the steelman pass.
6. Produce HUMIFY-PLAN.md if score or risk requires it.
7. Execute only the first safe no-commit refactor slice when explicitly allowed.
8. Produce HUMIFY-PATCHLOG.md after edits.

## Safety Rules

- Start read-only.
- Do not edit source until a safe first slice is clear.
- Do not refactor risky behavior before tests or characterization evidence.
- Do not score generated, vendored, bundled, build-output, lockfile, SDK, migration, or compiler output.
- Do not claim code is AI-generated without direct proof.
- Do not commit unless explicitly told to commit.
- Keep private run artifacts in ignored folders.

## Calibration Commands

Audit evaluator self-test:

```powershell
pwsh -NoProfile -File .\shared\tools\humify-evaluate.ps1 -ActualDir .\shared\expected -ExpectedDir .\shared\expected -OutputPath .\.humify-runs\humify-score-selftest.md -FailBelowThreshold
```

Plan evaluator self-test:

```powershell
pwsh -NoProfile -File .\shared\tools\humify-evaluate-plans.ps1 -ActualPlanDir .\shared\expected-plans -ExpectedPlanDir .\shared\expected-plans -OutputPath .\.humify-runs\humify-plan-score-selftest.md -FailBelowThreshold
```

Expected results:

```text
Humify score: 51 / 51 - Evaluator wiring OK (identity self-test — not a measure of real-repo accuracy)
Humify plan score: 33 / 33 - Evaluator wiring OK (identity self-test — not a measure of real-repo accuracy)
```
