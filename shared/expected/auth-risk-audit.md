# Expected Humify Audit: Auth Risk Fixture

Fixture: `fixtures/auth-risk/canAccessProject.ts`

Classification: High-risk refactor candidate
Machine-shaped confidence: Low
Refactor required: Yes, after tests

## Findings

```markdown
## H001. Authorization check fails open when membership lookup throws (HIGH)
File: fixtures/auth-risk/canAccessProject.ts
Lines: 12-30
Symptom: A malformed or unexpected membership list can grant access to a confidential project.
Causal chain:
1. Confidential project access depends on `user.projectIds?.includes(project.id)`.
2. The catch block returns `true` if that check throws.
3. Authorization failure turns into access granted instead of access denied.
Repro trigger: Pass a user object whose `projectIds.includes` throws for a confidential project.
Machine-shaped confidence: Low
Signals: permission logic, fail-open catch, security-sensitive behavior
Fix: Add security characterization tests for admin, member, non-member, anonymous, and thrown membership lookup before changing the catch to deny.
```

## Cleared Items

- The function is short and domain-named. The issue is authorization risk, not machine-shaped structure.

## Required classification

| Category | Expected |
| --- | --- |
| Overall | High-risk refactor candidate |
| Machine-shaped confidence | Low |
| Refactor required | Yes, after tests |

