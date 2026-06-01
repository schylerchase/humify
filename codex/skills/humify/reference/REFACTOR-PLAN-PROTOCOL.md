# Humify Refactor Plan Protocol

Use this protocol when a Humify audit scores low or identifies high-risk hotspots.

Low score does not mean "rewrite." It means "slow down, protect behavior, and slice deliberately."

Start from `templates/HUMIFY-PLAN.template.md` unless the user explicitly asks for a different shape.

## Trigger Conditions

Create a refactor plan when any of these are true:

- Area score is 18 or higher.
- Repository calibration score is below the readiness threshold (under ~86% of the reported dynamic maximum).
- Machine-shaped confidence is High in production-edited code.
- Refactor risk is High.
- Tests are missing around important behavior.
- Multiple findings share one workflow or boundary.
- The codebase is too large for direct single-pass cleanup.

## Planning Principles

- Preserve behavior first.
- Slice by workflow or boundary, not by aesthetic category.
- Characterize before extracting.
- Rename before moving when names hide intent.
- Extract pure logic before changing side-effect boundaries.
- Delete only after proving replacement or non-use.
- Keep each unit small enough to review and roll back.

## Non-Negotiable Gates

A Humify refactor plan is not executable until these gates pass:

| Gate | Required proof |
| --- | --- |
| Behavior gate | Current behavior is captured by tests, golden output, fixtures, logs, or documented manual checks. |
| Scope gate | Each unit maps to specific findings and files. |
| Safety gate | Risky behavior has tests before movement. |
| Rollback gate | Each unit can be reverted without losing unrelated work. |
| Boundary gate | Public API changes are explicit and intentional. |
| Coverage gate | Unknown or unsampled areas are listed. |
| Generated-code gate | Generated/vendor files are excluded or explicitly justified. |

## Implementation Unit Template

```markdown
## Unit <N>. <Action-oriented name>

Goal: <readability, safety, or boundary outcome>

Findings addressed:
- H001
- H002

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

## Default Unit Order

1. **Characterization harness**
   - Add golden-output, fixture, integration, or behavior tests.
   - Do not restructure yet.

2. **Domain language alignment**
   - Rename variables, functions, files, or types that hide meaning.
   - Keep public API compatibility unless intentionally changed.

3. **Pure logic extraction**
   - Pull decision logic out of side-effect-heavy functions.
   - Preserve inputs and outputs.

4. **Boundary separation**
   - Move database, network, filesystem, queue, or UI effects behind explicit adapters.
   - Keep orchestration thin.

5. **Duplicate abstraction collapse**
   - Merge parallel helpers or services only after behavior is protected.

6. **Dead code deletion**
   - Delete after references are proven absent or replacement is complete.

7. **Error contract tightening**
   - Make thrown errors, result objects, retries, and logging intentional.

8. **Final readability pass**
   - Remove stale comments, simplify names, and update docs.

## Low-Score Plan Shape

When an area scores low, the plan should start with a summary like:

```markdown
# Humify Refactor Plan

Score trigger: Area scored 21/27 in the heatmap.

Primary risk:
Customer imports mix validation, normalization, persistence, and operator reporting in one workflow.

Refactor stance:
Behavior-preserving. No import semantics should change until characterization tests are in place.

First safe slice:
Add characterization tests for accepted rows, skipped rows, existing customer update, new customer create, and malformed email handling.
```

## What Good Looks Like

Good plan:

- Starts with behavior protection.
- Names exact files.
- Groups related findings.
- Gives rollback for each unit.
- Has visible verification.
- Can be executed by another engineer.
- Follows `templates/HUMIFY-PLAN.template.md`.

Bad plan:

- "Refactor module."
- "Clean up code."
- "Improve architecture."
- "Rewrite service."
- "Make it more readable."
- Any plan that starts by moving code before tests.

## Behavior Preservation Checklist

Before moving logic, confirm:

- Current inputs are known.
- Current outputs are captured.
- Current error behavior is captured.
- Current side effects are identified.
- Logs or operator-facing outputs are identified.
- Existing callers are known.
- Public API compatibility is either preserved or intentionally changed.

## Plan Review Checklist

Before executing a Humify plan, ask:

- Does every unit have a clear goal?
- Does every unit map back to findings?
- Does the first risky unit add tests before movement?
- Are dependencies ordered correctly?
- Can each unit be reverted?
- Are generated files excluded?
- Are broad mechanical changes avoided?
- Is there a clear verification command or observation?

## Plan Rejection Criteria

Reject the plan and revise when:

- the first unit starts with broad file movement,
- the plan contains a "cleanup everything" unit,
- tests are deferred until after risky extraction,
- rollback is "revert the whole PR",
- generated files are modified without explanation,
- the plan changes public behavior without saying so,
- the plan does not identify what remains unknown.

## Example Low-Score Unit Sequence

```markdown
## Unit 1. Capture current customer import behavior

Goal: Protect import semantics before refactoring.
Findings addressed: H001, H002
Dependencies: None
Files:
- `tests/import/customers.characterization.test.ts`
- `fixtures/import/customers/*.csv`
Approach:
1. Add fixture rows for valid, missing-name, invalid-email, existing-customer, and inactive-customer cases.
2. Assert imported/skipped/update/create counts.
3. Assert operator-facing warnings that downstream workflows depend on.
Tests:
- Happy path: valid new customer imports.
- Edge case: inactive status maps to inactive customer.
- Error path: malformed email is skipped and logged.
- Integration: existing customer is updated instead of duplicated.
Verification:
- Characterization test passes against current implementation.
Rollback:
- Remove the new test and fixtures only.
Risk:
- Test may expose ambiguous current behavior. If so, document it before refactor.
Done when:
- Current behavior is pinned without changing production code.
```
