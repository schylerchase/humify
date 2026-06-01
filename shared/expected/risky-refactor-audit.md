# Expected Humify Audit: Risky Refactor Fixture

Fixture: `fixtures/risky-refactor/applyDiscounts.ts`

Expected result: **FINDINGS**

```markdown
## H001. Discount calculation needs characterization tests before slicing (HIGH)
File: fixtures/risky-refactor/applyDiscounts.ts
Lines: 15-46
Symptom: The function is small enough to tempt cleanup, but discount stacking and category eligibility behavior can change silently during refactor.
Causal chain:
1. The function calculates subtotal, eligible discount totals, minimum-spend gates, and final totals in one flow.
2. Discount behavior depends on interactions between category filters, quantities, percentages, and minimum spend.
3. Extracting helpers without tests could accidentally change stacking behavior or eligibility totals.
Repro trigger: Refactor `eligibleTotal` calculation or change minimum-spend handling.
Machine-shaped confidence: Low
Signals: nested business rules, implicit discount-stacking policy
Fix: Add characterization tests for multiple discounts, category-specific discounts, minimum-spend failures, zero discounts, and expected stacking behavior before extraction.
```

## Why this is not primarily machine-shaped

- Names are domain-specific.
- The structure is compact and plausible.
- The risk comes from hidden business policy, not generic generated patterns.

## Required classification

| Category | Expected |
| --- | --- |
| Overall | High-risk refactor candidate |
| Machine-shaped confidence | Low |
| Refactor required | Yes, after tests |
| Characterization tests required before edits | Yes |
