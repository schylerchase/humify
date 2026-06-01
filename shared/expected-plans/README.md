# Expected Humify Refactor Plans

These files define what a good Humify planning pass should produce after the audit fixtures.

They are not exact wording requirements. They define required planning behavior:

- risky behavior is protected before refactor,
- findings map to implementation units,
- units name exact files,
- tests, verification, rollback, and steelman checks are present,
- the plan does not start with broad movement or rewrite advice.
- fixture-specific edge cases remain visible in the first safe slice.

Use these with `REFACTOR-PLAN-PROTOCOL.md` and `templates/HUMIFY-PLAN.template.md`.
