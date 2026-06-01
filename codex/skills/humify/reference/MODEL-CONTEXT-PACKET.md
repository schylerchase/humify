# Humify Model Context Packet

Use this file to assemble model context consistently.

The goal is to give the model enough guidance to make good judgment calls without leaking expected calibration answers too early.

## Normal Audit Context Order

For a real codebase audit, provide files in this order:

1. `README.md`
2. `HUMIFY.md`
3. `HUMIFY-AI-INSTRUCTIONS.md`
4. `STEELMAN-PASS.md`
5. `EXAMPLES.md`
6. `STELLAR-CODEBASES.md`
7. relevant stack overlay, if present
8. `templates/HUMIFY-AUDIT.template.md`
9. target repository map or target files

Then run `prompts/humify-audit.md`.

## Massive Codebase Context Order

For a massive codebase:

1. `README.md`
2. `HUMIFY.md`
3. `HUMIFY-AI-INSTRUCTIONS.md`
4. `MASSIVE-CODEBASE-WORKFLOW.md`
5. `STEELMAN-PASS.md`
6. `REFACTOR-PLAN-PROTOCOL.md`
7. `EXAMPLES.md`
8. `STELLAR-CODEBASES.md`
9. `templates/HUMIFY-MAP.template.md`
10. `templates/HUMIFY-HEATMAP.template.md`
11. `templates/HUMIFY-AUDIT.template.md`
12. `templates/HUMIFY-PLAN.template.md`
13. repository inventory and sampled files

Then run `prompts/humify-massive-codebase.md`.

## Refactor Plan Context Order

For planning after an audit:

1. `HUMIFY.md`
2. `HUMIFY-AI-INSTRUCTIONS.md`
3. `REFACTOR-PLAN-PROTOCOL.md`
4. `STEELMAN-PASS.md`
5. `templates/HUMIFY-PLAN.template.md`
6. `HUMIFY-AUDIT.md`
7. `HUMIFY-HEATMAP.md`, if present

Then run `prompts/humify-plan.md`.

## Calibration Context Order

For a true calibration run:

1. `README.md`
2. `HUMIFY.md`
3. `HUMIFY-AI-INSTRUCTIONS.md`
4. `STEELMAN-PASS.md`
5. `EXAMPLES.md`
6. `STELLAR-CODEBASES.md`
7. `HUMIFY-TESTING.md`
8. `templates/HUMIFY-AUDIT.template.md`
9. `templates/HUMIFY-PLAN.template.md`
10. files under `fixtures/`
11. `prompts/humify-calibration.md`

Do **not** read:

- `expected/`
- `expected-plans/`
- `HUMIFY-SCORE.md`
- `HUMIFY-PLAN-SCORE.md`

until after `actual/` and `actual-plans/` have been written.

## Evaluation Context

After the model writes actual outputs, then read:

- `expected/`
- `expected-plans/`
- `tools/humify-evaluate.ps1`
- `tools/humify-evaluate-plans.ps1`

Run:

```powershell
pwsh -NoProfile -File .\shared\tools\humify-evaluate.ps1
pwsh -NoProfile -File .\shared\tools\humify-evaluate-plans.ps1
```

## Context Rules

- Do not include expected outputs during a blind calibration run.
- Do include expected outputs when debugging why a run failed.
- For massive repos, include maps and sampled files before deep findings.
- Prefer exact source files over summarized code when line evidence matters.
- Keep generated/vendor/build outputs out of context unless the task is specifically about exclusion behavior.

## Readiness Criteria

Humify is ready to run on a real repo when:

- audit evaluator self-test passes,
- plan evaluator self-test passes,
- blind calibration score is at least 86% on audits,
- blind calibration score is at least 86% on plans,
- no unsupported origin claims appear,
- low-score or high-risk findings produce tests-first plans,
- massive-codebase output includes coverage limits and unknowns.

