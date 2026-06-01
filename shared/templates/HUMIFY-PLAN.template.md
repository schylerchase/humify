# Humify Refactor Plan

Repository: `<repo name or path>`
Generated: `<date/time>`
Inputs:

- `HUMIFY-AUDIT.md`
- `HUMIFY-HEATMAP.md`
- `REFACTOR-PLAN-PROTOCOL.md`
- `STEELMAN-PASS.md`

## Score Trigger

`<Area/repo score or finding trigger that requires a plan>`

## Primary Risk

`<one paragraph explaining the main human/change risk>`

## Refactor Stance

Behavior-preserving unless explicitly stated otherwise.

Public behavior changes:

- `<none or exact intentional change>`

## Refactor Readiness Gate

| Gate | Status | Evidence |
| --- | --- | --- |
| Explicit opt-in gate | `<Open/Closed>` | `<user requested refactor slice or not>` |
| Dirty repo gate | `<Open/Closed>` | `<git status or baseline acceptance>` |
| No-commit gate | `<Open/Closed>` | `<no commit permission unless explicitly granted>` |
| Generated artifact gate | `<Open/Closed>` | `<generated/vendor/build exclusions>` |
| Privacy gate | `<Open/Closed>` | `<private output location or public-safe summary>` |

## First Safe Slice

`<the smallest useful action that protects behavior or reduces uncertainty>`

## Non-Negotiable Gates

| Gate | Status | Evidence |
| --- | --- | --- |
| Behavior gate | `<Pass/Fail/Pending>` | `<tests/golden/manual check>` |
| Scope gate | `<Pass/Fail/Pending>` | `<finding-to-unit mapping>` |
| Safety gate | `<Pass/Fail/Pending>` | `<tests before movement>` |
| Rollback gate | `<Pass/Fail/Pending>` | `<unit rollback>` |
| Boundary gate | `<Pass/Fail/Pending>` | `<public API note>` |
| Coverage gate | `<Pass/Fail/Pending>` | `<unknowns listed>` |
| Generated-code gate | `<Pass/Fail/Pending>` | `<exclusion note>` |
| Dirty repo gate | `<Pass/Fail/Pending>` | `<clean worktree or explicit baseline>` |
| No-commit gate | `<Pass/Fail/Pending>` | `<no commit unless explicitly requested>` |

## Implementation Units

```markdown
## Unit <N>. <Action-oriented name>

Goal: <readability, safety, or boundary outcome>

Findings addressed:
- H001

Dependencies:
- <tests, prior unit, decision, or none>

Files:
- <exact path>

Approach:
1. <step>
2. <step>
3. <step>

Tests:
- Happy path: <scenario>
- Edge case: <scenario>
- Error path: <scenario>
- Integration: <scenario>

Verification:
- <observable proof>

Rollback:
- <how to revert safely>

Risk:
- <what can go wrong>

Done when:
- <specific completion signal>
```

## Deferred Items

| Finding/area | Reason deferred | Revisit trigger |
| --- | --- | --- |
| `<H### or area>` | `<reason>` | `<trigger>` |

## Steelman Check

- Strongest evidence: `<best file/line or convention proof>`
- Biggest uncertainty: `<what was not reviewed or remains inferred>`
- Main false-positive risk: `<why this may be less severe>`
- Safety guardrail: `<test, rollback, or scope limit>`
- Decision: `<proceed | narrow scope | gather more evidence>`

## Execution Order

1. `<Unit 1>`
2. `<Unit 2>`

## Completion Criteria

- `<criteria>`
