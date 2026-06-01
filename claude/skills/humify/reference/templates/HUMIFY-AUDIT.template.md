# Humify Audit

Repository: `<repo name or path>`
Generated: `<date/time>`
Scope: `<whole repo | selected paths | hotspot>`
Inputs:

- `HUMIFY-MAP.md`
- `HUMIFY-HEATMAP.md`
- `<other context>`

## Coverage Statement

- Inventory coverage: `<what was discovered>`
- Sample coverage: `<what was read as representative sample>`
- Deep-dive coverage: `<what received file/line findings>`
- Unknowns: `<areas not inspected deeply enough to score confidently>`

## Refactor Readiness Verdict

- Source edit gate: `<closed | open>`
- Reason: `<clean repo, dirty repo, missing tests, user did/did not opt in, etc>`
- No-commit trial allowed: `<yes/no>`
- First safe slice: `<smallest behavior-protecting action or "none">`

## Classification Summary

| Area/file | Classification | Machine-shaped confidence | Refactor required | Notes |
| --- | --- | --- | --- | --- |
| `<path>` | `<Clean/Needs targeted cleanup/Machine-shaped readability risk/Excluded generated file/High-risk refactor candidate>` | `<None/Low/Medium/High/Not applicable>` | `<Yes/No/Yes, after tests>` | `<notes>` |

## Findings

```markdown
## H001. <Short title> (<SEVERITY>)
File: <path>
Lines: <exact line or range>
Symptom: <maintainer-visible or user-visible failure>
Causal chain:
1. <trigger or code fact>
2. <intermediate effect>
3. <symptom or risk>
Repro trigger: <specific scenario, or "N/A" when obvious>
Machine-shaped confidence: <None | Low | Medium | High | Not applicable>
Signals: <specific signals, not generic adjectives>
Fix: <minimal safe change, including tests first when needed>
```

## Cleared Items

- `<false positive or intentionally ignored item and why>`

## Verification Evidence

Commands or checks run:

| Check | Result | Evidence |
| --- | --- | --- |
| `<command/manual check>` | `<pass/fail/not run>` | `<summary>` |

Checks not run:

- `<check>` — `<why not>`

## Steelman Check

- Strongest evidence: `<best file/line or convention proof>`
- Biggest uncertainty: `<what was not reviewed or remains inferred>`
- Main false-positive risk: `<why this may be less severe>`
- Safety guardrail: `<test, rollback, or scope limit>`
- Decision: `<proceed | narrow scope | gather more evidence>`

## Plan Trigger

Refactor plan required: `<Yes/No>`

Reason:

- `<low score, high risk, missing tests, multiple related findings, etc>`
