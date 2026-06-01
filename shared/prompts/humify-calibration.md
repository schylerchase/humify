# Humify Calibration Prompt

Use this prompt to test a model against the Humify fixture pack.

```markdown
You are running Humify calibration.

Read and follow:

- `MODEL-CONTEXT-PACKET.md`
- `HUMIFY.md`
- `HUMIFY-AI-INSTRUCTIONS.md`
- `EXAMPLES.md`
- `STELLAR-CODEBASES.md`
- `STEELMAN-PASS.md`
- `HUMIFY-TESTING.md`
- `templates/HUMIFY-AUDIT.template.md`
- `templates/HUMIFY-PLAN.template.md`

Do not read files under `expected/` until after producing your actual outputs.

Review these fixtures independently:

- `fixtures/clean/invoiceSummary.ts`
- `fixtures/messy-human/importCustomers.ts`
- `fixtures/machine-shaped/processData.ts`
- `fixtures/generated-ignore/client.generated.ts`
- `fixtures/risky-refactor/applyDiscounts.ts`
- `fixtures/framework-boilerplate/nextRoute.ts`
- `fixtures/machine-shaped-no-comments/formatRecords.ts`
- `fixtures/generated-header-only/apiClient.ts`
- `fixtures/hand-edited-generated/apiClient.generated.ts`
- `fixtures/compatibility-wrapper/legacyCustomerApi.ts`
- `fixtures/ugly-stable/legacyChecksum.ts`
- `fixtures/bug-plus-readability/saveCustomer.ts`
- `fixtures/auth-risk/canAccessProject.ts`
- `fixtures/powershell-admin-risk/Repair-Agent.ps1`
- `fixtures/massive-cluster/reportWorkflow.ts`

Write these files:

- `actual/clean-audit.md`
- `actual/messy-human-audit.md`
- `actual/machine-shaped-audit.md`
- `actual/generated-ignore-audit.md`
- `actual/risky-refactor-audit.md`
- `actual/framework-boilerplate-audit.md`
- `actual/machine-shaped-no-comments-audit.md`
- `actual/generated-header-only-audit.md`
- `actual/hand-edited-generated-audit.md`
- `actual/compatibility-wrapper-audit.md`
- `actual/ugly-stable-audit.md`
- `actual/bug-plus-readability-audit.md`
- `actual/auth-risk-audit.md`
- `actual/powershell-admin-risk-audit.md`
- `actual/massive-cluster-audit.md`

Then write these plan files:

- `actual-plans/messy-human-plan.md`
- `actual-plans/machine-shaped-plan.md`
- `actual-plans/risky-refactor-plan.md`
- `actual-plans/machine-shaped-no-comments-plan.md`
- `actual-plans/hand-edited-generated-plan.md`
- `actual-plans/bug-plus-readability-plan.md`
- `actual-plans/auth-risk-plan.md`
- `actual-plans/powershell-admin-risk-plan.md`
- `actual-plans/massive-cluster-plan.md`

For each file, include:

- Classification
- Machine-shaped confidence
- Refactor required
- Findings or "No findings expected"
- Cleared Items
- Deep Plan Units only when refactor planning is required
- For plan files, use `templates/HUMIFY-PLAN.template.md`

After writing the actual outputs, run:

`pwsh -NoProfile -File .\shared\tools\humify-evaluate.ps1`

If the score is below the readiness threshold (under ~86% of the reported maximum), inspect the report and explain which judgment failed.
```
