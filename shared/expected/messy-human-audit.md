# Expected Humify Audit: Messy Human Fixture

Fixture: `fixtures/messy-human/importCustomers.ts`

Expected result: **FINDINGS**

```markdown
## H001. Import workflow mixes validation, persistence, and reporting (MEDIUM)
File: fixtures/messy-human/importCustomers.ts
Lines: 9-43
Symptom: A change to customer validation, database writes, or import reporting requires editing the same function.
Causal chain:
1. The function validates rows, normalizes fields, performs create/update database writes, logs skipped rows, and returns counters.
2. These responsibilities change for different reasons.
3. A reader must inspect the whole workflow to understand which behavior belongs to import policy versus persistence mechanics.
Repro trigger: Add a new validation rule or change create/update behavior.
Machine-shaped confidence: Low
Signals: mixed responsibilities, broad dependencies typed as `any`
Fix: Add characterization tests for imported/skipped/update/create outcomes, then extract row validation and customer normalization before changing persistence.
```

## Why this is messy-human, not machine-shaped

- The code has coherent domain terms: customer, import, row, email, active.
- The control flow follows a plausible hand-written import workflow.
- The weakness is responsibility mixing and weak dependency contracts, not generic generated structure.

## Required classification

| Category | Expected |
| --- | --- |
| Overall | Needs targeted cleanup |
| Machine-shaped confidence | Low |
| Refactor required | Yes |
| Characterization tests required before edits | Yes |
