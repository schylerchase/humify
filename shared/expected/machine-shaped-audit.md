# Expected Humify Audit: Machine-Shaped Fixture

Fixture: `fixtures/machine-shaped/processData.ts`

Expected result: **FINDINGS**

```markdown
## H001. Generic processor hides the domain contract (HIGH)
File: fixtures/machine-shaped/processData.ts
Lines: 1-48
Symptom: Readers cannot tell what kind of records are being processed or which fields are required by the real workflow.
Causal chain:
1. The function accepts `any[]` and `any`, then writes generic `processedItem` objects.
2. Field handling is repeated with vague defaults instead of a named domain contract.
3. Callers can pass malformed data and still receive plausible but semantically unsafe output.
Repro trigger: Add a required field or change the meaning of `status`.
Machine-shaped confidence: High
Signals: vague names, repetitive field blocks, obvious comments, weak typing, defensive checks for every field, unexplained metadata defaults
Fix: Identify the real record type, replace `processData` with a domain-named normalizer, and add tests for required fields, optional fields, and metadata behavior.
```

## Required classification

| Category | Expected |
| --- | --- |
| Overall | Machine-shaped readability risk |
| Machine-shaped confidence | High |
| Refactor required | Yes |
| Characterization tests required before edits | Yes |
