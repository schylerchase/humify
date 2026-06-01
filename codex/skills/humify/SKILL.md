---
name: humify
description: Run the Humify codebase rehabilitation workflow. Use when the user asks to run Humify, review a messy codebase, detect machine-shaped maintainability signals, map a large repo, produce HUMIFY-MAP/HEATMAP/AUDIT/PLAN/PATCHLOG artifacts, or plan/refactor code safely with tests before movement.
---

# Humify

Use Humify to turn messy codebases into evidence-backed audit artifacts and safe refactor slices.

Run Humify as a guided session. Show and explain what you find at each stage in the chat, and check in before moving on. The audit and plan files are a byproduct; the value is the explanation. Follow `reference/GUIDED-RUN.md` for how to present findings and where to checkpoint.

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

Run this as a guided session. Before each stage, say in one line what you are about to do and why. After each stage, show and explain what you found, then check in. Follow `reference/GUIDED-RUN.md` for the presentation format and checkpoints.

1. Load context: read `reference/MODEL-CONTEXT-PACKET.md`, then the core docs it points to (`reference/HUMIFY.md`, `reference/HUMIFY-AI-INSTRUCTIONS.md`, `reference/EXAMPLES.md`, `reference/STELLAR-CODEBASES.md`, `reference/STEELMAN-PASS.md`) and the audit template `reference/templates/HUMIFY-AUDIT.template.md`.
2. Map the repo, then present the lay of the land: what it is, entry points, what you excluded and why, and where you will look hardest. Save `HUMIFY-MAP.md` as a byproduct.
3. Score the hotspots, then present the heatmap: each hot area with its score, confidence, and a one-line reason. For massive repos, first read `reference/MASSIVE-CODEBASE-WORKFLOW.md` and the map, heatmap, and plan templates.
4. Audit the hotspots, then present each finding with file and line evidence and why it matters, plus the cleared items. Run `reference/STEELMAN-PASS.md` over the findings before presenting.
5. Give the Refactor Readiness Verdict and the first thing you would do, then check in: offer a refactor plan, more detail, or stop. Do not continue without the user.
6. If asked to plan, read `reference/REFACTOR-PLAN-PROTOCOL.md` and `reference/templates/HUMIFY-PLAN.template.md`, present the tests-first slices, explain the first safe slice, then check in before any edit.
7. Refactor only after explicit opt-in and an open gate. Narrate each slice, keep it no-commit, show before and after, and verify.

Produce the artifacts `HUMIFY-MAP.md`, `HUMIFY-HEATMAP.md`, `HUMIFY-AUDIT.md`, `HUMIFY-PLAN.md` when triggered, and `HUMIFY-PATCHLOG.md` after edits, but lead with the explanation in the chat.

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
