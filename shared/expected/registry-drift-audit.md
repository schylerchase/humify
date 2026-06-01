# Expected Humify Audit: Registry Drift Fixture

Fixture: `fixtures/registry-drift/cloudResourceRegistry.ts`

Classification: High-risk refactor candidate
Machine-shaped confidence: Medium
Refactor required: Yes, after tests

## Findings

```markdown
## H001. Snapshot diff reads a stale private-link input ID (HIGH)
File: fixtures/registry-drift/cloudResourceRegistry.ts
Lines: 3-29
Symptom: Snapshot comparisons can silently omit private-link resources that were uploaded through the active UI/import path.
Causal chain:
1. The active registry and upload map use `in_private_links`.
2. The snapshot diff registry reads `in_private_endpoints`.
3. Missing textarea values are parsed as empty arrays.
4. Diff output can report no private-link changes because the resources were never loaded.
Repro trigger: Upload `private-links.json`, then build a snapshot context from the active textarea values.
Machine-shaped confidence: Medium
Signals: stale alias, duplicated registry, silent empty fallback, multi-surface contract drift
Fix: Add a characterization test proving every active input reaches snapshot context, then replace duplicated IDs with a canonical resource registry before renaming anything.
```

## Cleared Items

- This is not merely a naming-style issue. The risk is silent resource omission across UI, import, export, and diff contracts.
- Machine-shaped confidence is Medium, not High, because rushed human migration code could produce the same drift.

## Required classification

| Category | Expected |
| --- | --- |
| Overall | High-risk refactor candidate |
| Machine-shaped confidence | Medium |
| Refactor required | Yes, after tests |
