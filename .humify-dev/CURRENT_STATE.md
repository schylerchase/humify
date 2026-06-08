# Humify — Current State

_Reviewed 2026-06-07 before any changes for the "primary agent binary" goal._

> **Update (build complete):** the primary binary now exists in the `humify-ng`
> module. `analyze / plan / verify / apply / status / doctor` all work and are
> proven end-to-end (see "Build Progress" at the bottom). The untangler is
> preserved under `humify untangle <stage>`.

## Current Humify State

The repo `schylerchase/humify` currently ships **two unrelated things**, and the
product the goal asks for (a deterministic `humify` **binary**) is **neither of
them yet**:

1. **A markdown skill/prompt framework** (`shared/`, `claude/`, `codex/`,
   `.agents/`) — Humify-as-a-Claude/Codex-skill. The *agent* does the analysis by
   following docs; there is no executable analyzer. This is the historical product.
2. **`humify-ng/`** — a Go CLI that is a *different* product: an agent-orchestrating
   "massive-codebase untangler" (`status/heatmap/audit/consolidate/plan/execute/
   patchlog/undo/resume/verify`). It dispatches LLM auditors and reconciles their
   output; it does **not** do self-contained static AI-slop detection, and its
   command names (`plan`, `verify`) collide with the goal's required commands.

The goal: a **safe, self-contained Go binary** that statically reviews a target
repo, detects AI-slop, scores maintainability, ranks a refactor plan, runs
validation, and keeps JSON state under `.humify/` — terminal + JSON as the control
plane, markdown optional.

## What Exists

- `humify-ng/` — mature Go module (`go 1.26`), ~20 internal packages, green tests.
  Reusable *ideas/primitives*: file scanning, ignore handling, risk scoring, the
  `.humify/` artifact convention, a trusted validation-command runner (`runGate`).
- `shared/` — methodology docs, markdown templates, 17 calibration fixtures,
  gold audit/plan baselines, Python evaluators. Useful as a **specification of
  what "good" findings/plans look like**, not as code.
- Root `README.md`, `HUMIFY-NG-ARCHITECTURE.md`.

## What Works

- `humify-ng` builds, vets, and tests green (`go ... -race ./...`, this session).
- The markdown framework is internally consistent (not in scope for this goal).

## What Is Missing (the goal)

- A `humify` **binary** with `analyze / plan / verify / apply / status / doctor`.
- Deterministic static analysis: stack/manager/script/entrypoint detection;
  per-file metrics (LOC, function length, nesting, comment ratio); AI-slop signal
  detectors; 5-category scoring; ranked findings + refactor items.
- JSON state: `.humify/analysis.json`, `.humify/plan.json`, `.humify/validation.json`.
- `.humifyignore` support; `.gitignore` + default ignore handling in the new tool.
- Tests for scanning, ignore, detection, scoring, planning, validation detection, CLI.

## What Is Broken

- Nothing in scope is broken — the binary simply does not exist yet.
- (Aside: `humify-ng` has uncommitted spawn-runner changes from earlier this
  session, tested green, awaiting commit approval. Untouched by this goal.)

## Pre-existing Validation Results

| Target | Command | Result |
|---|---|---|
| humify-ng | `go build/vet/test -race ./...` | PASS (this session) |
| repo root | (no build/test for the markdown framework) | n/a |
| Python evaluators | `shared/tools/*.py` (stdlib only) | not run; out of scope |

No pre-existing failures relevant to the new binary.

## Recommended Build Path

Build the product **inside the existing `humify-ng` Go module** (the current
module — no sibling/parallel project). Evolve `humify-ng` into the Humify binary;
preserve the untangler by **namespacing it**, not deleting it.

1. **Resolve command collisions:** the new top-level commands are
   `analyze plan verify apply status doctor`. The untangler's `status/heatmap/
   audit/consolidate/plan/execute/patchlog/undo/resume/verify` move under a single
   `humify untangle <stage>` namespace (handlers renamed `untangle*`). All
   untangler code + tests stay green — restructured, not abandoned.
2. `internal/humify/ignore` — default dirs + `.gitignore`/`.humifyignore` (tested first).
3. `internal/humify/scan` — walk + ignore → source files; `detect` — stack, package
   manager, scripts, entry points, configs, tests, large files.
4. `analyze` — scan → per-file metrics (LOC/func-len/nesting/comment-ratio) → slop
   signals → 5-category scores → `.humify/analysis.json` + terminal summary.
5. `plan` — analysis → ranked `HMF-NNN` items (evidence/risk/benefit/validation/
   automation-safety) → `.humify/plan.json` + next actions. At least one item is a
   deterministic, apply-able action (e.g. quarantine confirmed-stale files).
6. `verify` — detect + run safe test/build/lint/typecheck → `.humify/validation.json`.
7. `status` — render the three JSON states. `doctor` — env/wiring/repo readiness.
8. `apply` — dry-run default; `--target HMF-### --yes` to act; **quarantine over
   delete** → move stale files to `.humify/delete-me/<plan-id>/` + JSON manifest
   (original/new path, reason, plan item, timestamp, validation), then run verify.
9. Tests per package; prove e2e (analyze→plan→apply dry→apply yes→verify); README usage.

JSON is the control plane; markdown export is optional/flagged. Do not modify the
markdown framework. Quarantine (never delete) any confirmed-stale files to
`.humify-dev/delete-me/` with a manifest.

---

## Build Progress (2026-06-07/08, this session)

Built the product **inside the `humify-ng` module** (no sibling), reusing the
repo's language/conventions. Untangler preserved under `humify untangle`.

**New packages** (`internal/humify/`): `ignore`, `scan`, `detect`, `analyze`
(metrics + slop + score), `plan`, `verify`, `apply`, `state`. **New CLI:**
`humify.go` (handlers) + `render.go` (terminal/markdown). **main.go** rewired:
product commands top-level, untangler namespaced, `parseArgs` now supports
`--flag value` and `--flag=value`.

**Proven end-to-end** on a fixture (`/tmp/humify-target`): analyze (82/100, 5
findings) → plan (HMF-001 quarantine + 4 manual/assisted) → apply --dry-run →
apply --yes (quarantined `old.bak` → `.humify/delete-me/HMF-001/` + manifest,
validation passed) → verify (go build/vet/test pass) → status → doctor. Safety
guards confirmed: manual item with `--yes` refused; no-flag apply defaults to dry
run; unknown id errors.

**Tests:** per-package unit tests (ignore matching, scan/classify, detection,
metrics, slop signals, scoring, plan ranking, validation detection, apply
quarantine/refusal/rollback, CLI). `go build/vet/test -race ./...` green.

**Self-review:** `humify analyze` on its own module scores 84/100 (only info +
2 minor long-function warnings).

## Adversarial Review + Fixes (this session)

Ran a verify-gated multi-agent review (4 lenses, 3-skeptic default-refute gate).
**10 findings confirmed, all fixed:**

- **[blocker]** multi-line raw strings / template literals leaked braces →
  corrupted nesting/longest-function file-wide. Fixed: rewrote line classification
  into a stateful scanner (`scanBrace`/`scanIndent`) that tracks multi-line strings
  and block comments.
- **[major]** inline/mid-line block comments leaked braces → same scanner fix.
- **[major]** Python docstrings poisoned the indent unit → scanner now strips
  triple-quoted strings before measuring.
- **[major]** `if err != nil {} else {…}` falsely flagged swallowed → empty-block
  check now requires an exact bare `}`.
- **[major]** apply could quarantine an empty-but-significant file (e.g.
  `__init__.py`) → dropped the empty-file stale rule; only throwaway names quarantine.
- **[major]** apply exited 0 after a rollback → `cmdApply` now exits 2 on rollback;
  added `Result.Validated`.
- **[minor]** slop detectors ran on raw text (fired inside strings/comments) → now
  match on cleaned code; `todo_marker` matches comment text only.
- **[minor]** `noisyComment` used substring match → now whole-identifier match.
- **[minor]** documented exit code didn't match behavior → doc corrected.
- **[minor]** markdown `File` column unescaped → now escaped.

Each fix has a regression test. **Coverage gap:** the `ignorescan` review lens
stalled (infra), so ignore/scan got unit-test coverage but not adversarial review;
both are simple and `filepath.WalkDir` does not follow symlinks (no loop risk).
Stray probe `*_test.go` files left by review subagents were removed.

**Finding #11 (follow-up pass, also fixed) — false "validated" signal.** A
leftover NOT-REFUTED verdict on disk flagged a real defect distinct from the 10:
`verify.Run` defaulted `Validation.Passed = true` and, on the no-commands early
return, kept it — so `humify verify` on any ecosystem outside Go/Node/Cargo/pytest
printed "PASSED" having validated nothing, and `apply`'s manifest baked the
self-contradicting `{"ran": false, "passed": true}`. A tool whose value
proposition is "validated refactoring" must not report a vacuous pass. **Fixed**
by adding a top-level `Validation.Validated bool` (= any command actually ran),
forming an honest tri-state (`validated&&passed` real pass · `validated&&!passed`
real fail · `!validated` nothing ran). `passed` keeps its documented
"nothing-that-ran-failed" semantic (so the no-fail-on-unsupported-repo test and the
`cmdVerify` exit-code contract are untouched); render (`overall: NOT VALIDATED`),
status (`NOT VALIDATED`), the manifest (`ValSummary.Passed = validated && passed`),
and `Result.Validated` all derive from the one field. Regression tests:
`TestApplyQuarantineEndToEnd` now asserts the manifest records `Ran:false
Passed:false` in a no-command repo; the verify no-commands test asserts
`!Validated`. Proven e2e: no-command target → "NOT VALIDATED" + honest manifest;
Go module → build/vet/test run → "PASSED".

**Final:** `go build/vet/test -race ./...` green (all test packages). Post-fix
dogfood: Humify scores its own module 85/100 with the prior false positives gone.
Repo cleaned: `/.humify/` dogfood output now gitignored (anchored, so the tracked
`testdata/sample/.humify/` fixtures are untouched); stale `humify.exe` and the
`.humify-review-finding.md` verdict removed (the latter only after its fix landed).
