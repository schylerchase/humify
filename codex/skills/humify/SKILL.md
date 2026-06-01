---
name: humify
description: Run the Humify codebase rehabilitation workflow. Use when the user asks to run Humify, review a messy codebase, detect machine-shaped maintainability signals, map a large repo, produce HUMIFY-MAP/HEATMAP/AUDIT/PLAN/PATCHLOG artifacts, or plan/refactor code safely with tests before movement.
---

# Humify

Use Humify to turn messy codebases into evidence-backed audit artifacts and safe refactor slices.

This skill is self-contained. Its methodology docs and templates ship under `reference/` (relative to this skill directory), so the paths below resolve no matter which repo you run Humify on.

## Operating Rules

- Start read-only unless the user explicitly asks for a refactor slice.
- Refactor mode requires explicit opt-in. A request to audit, review, map, score, or plan does not grant source-edit permission.
- Capture repo state before judging code: git root, branch, dirty files, staged files, unstaged files, untracked files, ignored local artifacts, and nested repos.
- Treat a dirty repo as audit-only by default. Block source edits until the worktree is clean, a separate clean worktree is used, or the user explicitly accepts the current dirty state as the refactor baseline.
- Never claim code is AI-generated unless direct evidence exists. Use `machine-shaped` for maintainability signals.
- Exclude generated, vendored, bundled, build-output, lockfile, SDK, migration, and compiler-produced files unless the user says they are hand-edited.
- Require file and line evidence for every finding.
- Separate machine-shaped confidence from refactor risk.
- Require characterization tests, golden-output capture, fixtures, logs, or manual checks before refactoring risky behavior.
- Private run artifacts stay in ignored local run folders by default. Public docs may use only generic examples or intentionally sanitized synthetic examples.
- Reports must not include private repo names, absolute local paths, remotes, secrets, account/customer identifiers, or private training targets unless the user explicitly requests local/private output.
- Do not commit unless the user explicitly asks.

## Workflow

1. Read `reference/MODEL-CONTEXT-PACKET.md` to choose the right context order.
2. For normal audits, read `reference/HUMIFY.md`, `reference/HUMIFY-AI-INSTRUCTIONS.md`, `reference/EXAMPLES.md`, `reference/STELLAR-CODEBASES.md`, `reference/STEELMAN-PASS.md`, and `reference/templates/HUMIFY-AUDIT.template.md`.
3. For massive repos, also read `reference/MASSIVE-CODEBASE-WORKFLOW.md` and the map, heatmap, and plan templates in `reference/templates/`.
4. For refactor planning, read `reference/REFACTOR-PLAN-PROTOCOL.md` and `reference/templates/HUMIFY-PLAN.template.md`.
5. Produce artifacts in this order: `HUMIFY-MAP.md`, `HUMIFY-HEATMAP.md`, `HUMIFY-AUDIT.md`, `HUMIFY-PLAN.md` when triggered, and `HUMIFY-PATCHLOG.md` after edits.
6. Run `reference/STEELMAN-PASS.md` before finalizing high-confidence claims, low-score plans, or massive-repo conclusions.

The standard user flow is: map the repo, exclude generated/vendor artifacts, score readability and refactor risk, identify machine-shaped or generated code, produce evidence-backed findings, produce a refactor plan if scores are low, gate refactor by repo cleanliness and explicit opt-in, run only approved no-commit slices, verify with tests, then summarize before/after behavior.

## Refactor Gate

Only execute a refactor slice after the plan identifies:

- exact findings addressed,
- exact files,
- behavior protection,
- tests or manual verification,
- rollback path,
- residual risk.

The first slice should usually add characterization tests or golden-output checks. Prefer no-commit trials until the user approves saving work.

If the repo is dirty, the refactor gate is closed unless the user explicitly accepts the dirty tree as the baseline. Do not stash, revert, clean, commit, or rearrange the user's existing work as part of opening the gate.

## Calibration and source

The calibration pack (fixtures, expected baselines, and the Python and PowerShell evaluators) lives in the Humify source repository, not in the installed plugin. To calibrate the framework or contribute, clone `github.com/schylerchase/humify` and use the tools under `shared/tools/`.
