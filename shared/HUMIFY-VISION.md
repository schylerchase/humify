# Humify Vision

Humify is a framework for turning messy codebases into humane codebases.

The end goal is simple:

Make code easier for humans to understand, trust, test, and safely refactor without losing behavior.

## What Humify Does

Humify reviews a codebase in stages:

1. Map the repo.
2. Exclude generated, vendored, bundled, and build-output files.
3. Find risky workflows and hard-to-read areas.
4. Separate machine-shaped signals from actual refactor risk.
5. Produce evidence-backed findings with file and line references.
6. Turn low scores into a safe, tests-first refactor plan.
7. Execute only small, reversible refactor slices.
8. Verify behavior and record what changed.

## What Humify Is Looking For

Humify looks for code that is hard for humans to reason about:

- Generic names that hide domain meaning
- Large files with mixed responsibilities
- Duplicated registries or stale aliases
- UI, import, export, diff, or persistence surfaces that disagree
- Plausible-looking code that misses edge cases
- Weak or missing tests around risky behavior
- Old domain names left behind after a migration
- Machine-shaped repetition that makes code harder to trust

Humify does not try to prove code was AI-generated. It uses "machine-shaped" to describe maintainability signals, not origin.

## The Main Rule

Do not refactor risky code before behavior is protected.

If code handles money, permissions, deletion, medical data, infrastructure, imports, exports, migrations, retries, or operator-facing reports, the first step is characterization tests or golden-output capture.

## Expected Outputs

A real Humify run should produce:

- HUMIFY-MAP.md: repo inventory, entrypoints, tests, exclusions, risky areas
- HUMIFY-HEATMAP.md: scored hotspots with confidence and coverage limits
- HUMIFY-AUDIT.md: evidence-backed findings and cleared false positives
- HUMIFY-PLAN.md: tests-first refactor slices when risk or score requires it
- HUMIFY-PATCHLOG.md: completed slices, verification, and remaining risk

## Product End State

Eventually, Humify should feel like a Codex plugin or skill where the user can say:

Run Humify on this repo.

The plugin should start read-only, produce the map, heatmap, audit, and plan, then only edit code when a safe first slice is clear.

## Success Criteria

Humify succeeds when it can:

1. Review messy repos without unsupported AI-origin claims.
2. Identify both ugly code and risky-but-human code.
3. Exclude generated and build artifacts correctly.
4. Handle massive repos without pretending full coverage.
5. Produce findings with evidence.
6. Create tests-first refactor plans.
7. Execute small no-commit refactor trials safely.
8. Preserve private repo details.
9. Convert lessons from real runs into reusable examples.

## Final Goal

A humified codebase has clearer names, smaller functions, explicit boundaries, fewer hidden contracts, tests around risky behavior, and a refactor trail future maintainers can trust.

The point is not to make code prettier.

The point is to restore human confidence.
