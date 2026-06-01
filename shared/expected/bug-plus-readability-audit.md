# Expected Humify Audit: Bug Plus Readability Fixture

Fixture: `fixtures/bug-plus-readability/saveCustomer.ts`

Classification: Needs targeted cleanup
Machine-shaped confidence: Low
Refactor required: Yes, after bug fix

## Findings

```markdown
## H001. Saved email uses unnormalized input (HIGH)
File: fixtures/bug-plus-readability/saveCustomer.ts
Lines: 10-25
Symptom: The function validates the trimmed email but saves the original untrimmed email.
Causal chain:
1. Line 12 calculates `normalizedEmail` from `input.email.trim()`.
2. Line 18 validates `normalizedEmail`.
3. Line 24 saves `input.email` instead of `normalizedEmail`.
Repro trigger: Save a customer with leading or trailing spaces in the email.
Machine-shaped confidence: Low
Signals: validation/save mismatch, normalization drift
Fix: Add a failing test for trimmed email persistence, then save `normalizedEmail`.

## H002. Validation and persistence are coupled in one function (MEDIUM)
File: fixtures/bug-plus-readability/saveCustomer.ts
Lines: 10-25
Symptom: A validation cleanup can accidentally alter persistence behavior.
Causal chain:
1. The function normalizes fields, validates fields, and writes to the repository.
2. The bug in H001 shows these concerns are already drifting.
3. Future cleanup has to reason about validation and persistence at the same time.
Repro trigger: Add another normalized field before saving.
Machine-shaped confidence: Low
Signals: mixed validation and persistence, bug plus readability risk
Fix: Keep this as a separate finding: fix and test the email bug first, then consider extracting validation if more rules are added.
```

## Cleared Items

- This should not be reported as machine-shaped. The issue is a real bug plus ordinary responsibility mixing.

## Required classification

| Category | Expected |
| --- | --- |
| Overall | Needs targeted cleanup |
| Machine-shaped confidence | Low |
| Refactor required | Yes, after bug fix |

