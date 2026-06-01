# Humify Testing Pack

This pack tests whether Humify can make consistent, evidence-based judgments before it is used on a real codebase.

The goal is not to prove that a reviewer agrees with every word. The goal is to prove that Humify:

1. Flags real maintainability risk.
2. Avoids false claims of machine-generated origin.
3. Excludes generated files correctly.
4. Requires behavior tests before risky refactors.
5. Produces findings that can become safe Deep Plan implementation units.

For model guidance, use:

```text
MODEL-CONTEXT-PACKET.md
EXAMPLES.md
STELLAR-CODEBASES.md
STEELMAN-PASS.md
MASSIVE-CODEBASE-WORKFLOW.md
REFACTOR-PLAN-PROTOCOL.md
```

## Files

```text
fixtures/
  clean/invoiceSummary.ts
  messy-human/importCustomers.ts
  machine-shaped/processData.ts
  generated-ignore/client.generated.ts
  risky-refactor/applyDiscounts.ts
  framework-boilerplate/nextRoute.ts
  machine-shaped-no-comments/formatRecords.ts
  generated-header-only/apiClient.ts
  hand-edited-generated/apiClient.generated.ts
  compatibility-wrapper/legacyCustomerApi.ts
  ugly-stable/legacyChecksum.ts
  bug-plus-readability/saveCustomer.ts
  auth-risk/canAccessProject.ts
  powershell-admin-risk/Repair-Agent.ps1
  massive-cluster/reportWorkflow.ts
  registry-drift/cloudResourceRegistry.ts
  high-trust-stale-data/medicationPrices.ts

expected/
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
  registry-drift-audit.md
  high-trust-stale-data-audit.md

expected-plans/
  messy-human-plan.md
  machine-shaped-plan.md
  risky-refactor-plan.md
  machine-shaped-no-comments-plan.md
  hand-edited-generated-plan.md
  bug-plus-readability-plan.md
  auth-risk-plan.md
  powershell-admin-risk-plan.md
  massive-cluster-plan.md
  registry-drift-plan.md
  high-trust-stale-data-plan.md

templates/
  HUMIFY-MAP.template.md
  HUMIFY-HEATMAP.template.md
  HUMIFY-AUDIT.template.md
  HUMIFY-PLAN.template.md
  HUMIFY-PATCHLOG.template.md
```

## Calibration Procedure

Run Humify against each fixture independently using the model instructions:

```text
MODEL-CONTEXT-PACKET.md
HUMIFY-AI-INSTRUCTIONS.md
EXAMPLES.md
STELLAR-CODEBASES.md
STEELMAN-PASS.md
prompts/humify-calibration.md
```

For each fixture, produce:

```text
actual/
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
  registry-drift-audit.md
  high-trust-stale-data-audit.md
```

For fixtures that require refactor planning, also produce:

```text
actual-plans/
  messy-human-plan.md
  machine-shaped-plan.md
  risky-refactor-plan.md
  machine-shaped-no-comments-plan.md
  hand-edited-generated-plan.md
  bug-plus-readability-plan.md
  auth-risk-plan.md
  powershell-admin-risk-plan.md
  massive-cluster-plan.md
  registry-drift-plan.md
  high-trust-stale-data-plan.md
```

Then compare each `actual` file to the matching `expected` file.

Then compare each `actual-plans` file to the matching `expected-plans` file.

The exact wording may differ. The classification, evidence, confidence, required next step, implementation-unit order, tests-first behavior, verification, rollback, and steelman check should match.

## Automated Evaluation

After creating `actual/*.md`, run:

```powershell
pwsh -NoProfile -File .\shared\tools\humify-evaluate.ps1
```

The evaluator writes `HUMIFY-SCORE.md`.

After creating `actual-plans/*.md`, run:

```powershell
pwsh -NoProfile -File .\shared\tools\humify-evaluate-plans.ps1
```

The plan evaluator writes `HUMIFY-PLAN-SCORE.md`.

To self-test the evaluator against the expected outputs:

```powershell
pwsh -NoProfile -File .\shared\tools\humify-evaluate.ps1 -ActualDir .\shared\expected -ExpectedDir .\shared\expected -OutputPath .\.humify-runs\humify-score-selftest.md
```

Expected self-test result: `51 / 51`.

To self-test the plan evaluator against the expected plan outputs:

```powershell
pwsh -NoProfile -File .\shared\tools\humify-evaluate-plans.ps1 -ActualPlanDir .\shared\expected-plans -ExpectedPlanDir .\shared\expected-plans -OutputPath .\.humify-runs\humify-plan-score-selftest.md
```

Expected self-test result: `33 / 33`.

## Privacy Guardrails

When running Humify on a real repository, keep private material out of public output:

- Generated artifact exclusion: fixtures and prompts must exclude generated/vendor/build artifacts before scoring.
- Privacy scan: public files must not contain private repo names, absolute local paths, personal handles, remotes, cloud account IDs, customer identifiers, or proprietary run findings.
- Keep run output ignored: directories such as `.humify-runs/`, `actual/`, and `actual-plans/` should stay out of version control so private run artifacts are never committed.

## Required Audit Checks

Every non-empty finding must include:

- Finding ID
- Severity
- File path
- Exact line or line range
- User-visible symptom
- Causal chain
- Repro trigger when non-obvious
- Machine-shaped confidence
- Signals
- Minimal fix

Every file must receive one of these classifications:

- `Clean`
- `Needs targeted cleanup`
- `Machine-shaped readability risk`
- `Excluded generated file`
- `High-risk refactor candidate`

## Pass Criteria

Humify passes the pack when all of these are true:

- The clean fixture has no defect finding.
- The messy-human fixture is flagged for maintainability but not overclaimed as machine-generated.
- The machine-shaped fixture receives high machine-shaped confidence.
- The generated fixture is excluded before readability scoring.
- The risky-refactor fixture requires characterization tests before refactor.
- Findings include exact file and line evidence.
- Refactor advice is sliced and behavior-preserving.

## Failure Criteria

Humify fails the pack if any of these happen:

- It claims code is machine-generated without proof.
- It misses the generated-file exclusion.
- It recommends refactoring risky behavior before tests.
- It produces broad advice without file/line evidence.
- It treats the clean fixture as defective without a concrete, causal bug or maintainability risk.
- It cannot explain why messy-human differs from machine-shaped.

## Scoring

Score each fixture from 0 to 3.

| Score | Meaning |
| --- | --- |
| 0 | Incorrect classification or unsafe advice. |
| 1 | Correct broad category, but weak evidence or missing next step. |
| 2 | Mostly correct with minor wording or completeness issues. |
| 3 | Correct classification, evidence, confidence, and safe next action. |

Current maximum score: 51.

Current plan score maximum: 33.

Recommended threshold:

- **86-100%:** Ready to use on a real repo.
- **67-85%:** Usable with human review.
- **40-66%:** Needs framework tuning.
- **0-39%:** Not reliable yet.

Plan scoring threshold:

- **86-100%:** Plan calibration ready.
- **67-85%:** Usable with human review.
- **40-66%:** Needs planning guidance tuning.
- **0-39%:** Not reliable yet.

## Deep Plan Conversion Test

After the audit pass, convert findings from these three fixtures into implementation units:

- `messy-human/importCustomers.ts`
- `machine-shaped/processData.ts`
- `risky-refactor/applyDiscounts.ts`

Each implementation unit must include:

- Name
- Goal
- Findings addressed
- Dependencies
- Files
- Approach
- Tests
- Verification
- Rollback

The conversion passes only if:

- Characterization tests come before behavior-preserving refactors.
- Machine-shaped cleanup starts by discovering or naming the real domain contract.
- Risky discount behavior is protected before extraction.
- No unit mixes unrelated fixture concerns.
- Each plan follows `templates/HUMIFY-PLAN.template.md`.
- Each plan includes non-negotiable gates and a steelman check.
- Every high-risk unit starts with characterization or behavior protection.
- Every unit has rollback and verification.

## Suggested Actual Output Template

```markdown
# Actual Humify Audit: <Fixture Name>

Fixture: `<path>`

Classification: <classification>
Machine-shaped confidence: <None | Low | Medium | High | Not applicable>
Refactor required: <Yes | No | Yes, after tests>

## Findings

<findings or "No findings expected.">

## Cleared Items

- <false positive or intentionally ignored item>

## Deep Plan Units

<only include when findings require refactor planning>
```

## Regression Rule

When Humify rules change, rerun this pack. A change is only an improvement if it increases precision without making the framework more reckless.
