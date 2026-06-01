# Expected Humify Audit: Machine-Shaped No-Comments Fixture

Fixture: `fixtures/machine-shaped-no-comments/formatRecords.ts`

Classification: Machine-shaped readability risk
Machine-shaped confidence: High
Refactor required: Yes

## Findings

```markdown
## H001. Parallel arrays hide the record domain contract (HIGH)
File: fixtures/machine-shaped-no-comments/formatRecords.ts
Lines: 3-35
Symptom: Readers cannot tell which record type is being formatted or whether empty-string defaults are valid domain values.
Causal chain:
1. The function uses generic `formatRecords`, `records`, `config`, and `output` names.
2. Field behavior is encoded through parallel `fields` and `defaults` arrays instead of a named domain contract.
3. Adding or reordering a field can silently pair the wrong default with the wrong output field.
Repro trigger: Add a required field to `fields` but forget to update `defaults` at the same index.
Machine-shaped confidence: High
Signals: generic names, parallel arrays, unknown record contract, untyped config, metadata enrichment mixed into formatting
Fix: Add characterization tests for current field/default behavior, then introduce named input/output types before replacing the parallel arrays.
```

## Cleared Items

- No comments are needed for this to be machine-shaped; the evidence comes from structure, naming, and defaults.

## Required classification

| Category | Expected |
| --- | --- |
| Overall | Machine-shaped readability risk |
| Machine-shaped confidence | High |
| Refactor required | Yes, after characterization |

