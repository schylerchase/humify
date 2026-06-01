# Humify Heatmap

Repository: `<repo name or path>`
Generated: `<date/time>`
Input map: `HUMIFY-MAP.md`

## Scoring Method

Scores use `0-3` per category.

| Score | Meaning |
| --- | --- |
| 0 | Clear and locally understandable. |
| 1 | Minor friction, safe to maintain. |
| 2 | Meaningful maintainability risk. |
| 3 | High-risk or human-hostile code. |

Categories:

- Readability
- Structure
- Function design
- Domain language
- Boundary hygiene
- Error behavior
- Testability
- Machine-shaped signal strength
- Refactor risk

## Coverage Reminder

- Inventory coverage: `<summary>`
- Sample coverage: `<summary>`
- Deep-dive coverage: `<summary>`
- Unknowns: `<summary>`

## Area Scores

| Area | Representative files | Observed score | Confidence | Primary risk | Evidence | Next action |
| --- | --- | ---: | --- | --- | --- | --- |
| `<area>` | `<paths>` | `<0-27>` | `<High/Medium/Low>` | `<risk>` | `<file/line or code fact>` | `<action>` |

## Hotspot Priority

| Rank | Area | Why now | First safe move |
| ---: | --- | --- | --- |
| 1 | `<area>` | `<score/risk/churn/blocker>` | `<tests/map/deep dive>` |

## Steelman Check

- Strongest evidence: `<best file/line or convention proof>`
- Biggest uncertainty: `<what was not reviewed or remains inferred>`
- Main false-positive risk: `<why this may be less severe>`
- Safety guardrail: `<test, rollback, or scope limit>`
- Decision: `<proceed | narrow scope | gather more evidence>`

