# Expected Humify Audit: Hand-Edited Generated Fixture

Fixture: `fixtures/hand-edited-generated/apiClient.generated.ts`

Classification: High-risk refactor candidate
Machine-shaped confidence: Low
Refactor required: Yes, after tests

## Findings

```markdown
## H001. Local patch inside generated client will be lost on regeneration (HIGH)
File: fixtures/hand-edited-generated/apiClient.generated.ts
Lines: 11-20
Symptom: The null-status fallback can disappear when the API client is regenerated.
Causal chain:
1. Lines 1-3 mark the file as generated and not meant for manual edits.
2. Lines 15-18 add a local hand-edited fallback inside that generated file.
3. Regenerating the client can overwrite the fallback and change ticket status behavior.
Repro trigger: Regenerate the OpenAPI client after the local patch is added.
Machine-shaped confidence: Low
Signals: generated file marker, hand-edited local patch, regeneration risk
Fix: Add characterization tests for null status behavior, then move the fallback into a hand-edited adapter around the generated client.
```

## Cleared Items

- The generated portions should still be excluded from readability scoring; only the hand-edited patch is actionable.

## Required classification

| Category | Expected |
| --- | --- |
| Overall | High-risk refactor candidate |
| Machine-shaped confidence | Low |
| Refactor required | Yes, after tests |

