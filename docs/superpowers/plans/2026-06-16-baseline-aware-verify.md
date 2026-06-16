# Baseline-aware verify + blind-canary follow-ons — Implementation Plan

> Execute task-by-task with TDD (red → green → commit). Built on `feat/baseline-verify`.

**Goal:** Close the gap a blind canary found — the standalone `verify` an AI calls after its own edits has no baseline, so it can't tell an ambient failure (e.g. missing deps) from a regression the edit caused. Plus three smaller follow-ons the canary surfaced.

**Why this shape (save-and-compare, NOT a reconstructed baseline):**
Ambient failures and real regressions are both clean non-zero exits — only running the
*same checks before and after* discriminates them. The delta logic (`computeDelta`) already
exists in `apply`; we lift it to `verify`. The baseline itself is captured as a **saved
snapshot** of a verify run, persisted to `.humify/verify-baseline.json`, NOT reconstructed by
checking out HEAD.

Two reasons saved-snapshot wins:
1. **Read-only contract.** `verify` is documented to never touch target source (verify.go
   header). A git-stash / worktree reconstruction would run `checkout`/`clean` on the live
   tree — a contract break. The only thing we write is humify's own `.humify/` state.
2. **Same-environment discriminator.** A fresh-worktree baseline runs in a *different*
   dependency environment than the live tree. For the no-`node_modules` canary the worktree is
   all-red, post is all-red on the same kinds, and every real regression looks ambient.
   Save-and-compare captures the baseline in the **same** environment as post, so the only
   variable between the two runs is the AI's edits. That is the cleanest discriminator.

**The ordering hazard this design must guard:** the baseline is only meaningful if it is
captured *before* the edit. If an AI runs `--save-baseline` after it has already started
editing, the baseline already contains the breakage, the delta finds no change, and
`--baseline` reports a confident "no regression" — strictly worse than no baseline. In
`apply`, humify owns the baseline→edit→post ordering so this can't happen; in the manual/AI
path the ordering is the caller's responsibility, so we guard it: `--save-baseline` records
whether the tree was dirty (and warns), and the *save* step is taught at plan-time in the
AgentSpec, not as a post-edit hint.

---

## Phase 1 — baseline-aware verify (the headline)

### Task 1.1 — move `computeDelta` to the verify package (dedup)
- **Files:** `internal/humify/verify/delta.go` (new) + `delta_test.go` (new), `internal/humify/apply/apply.go`, `internal/humify/apply/apply_test.go`.
- **Scope:** move `computeDelta` (apply.go:514) to `verify` as exported `verify.Delta(baseline, post Validation) (alreadyFailing, newlyFailing, fixed []string)`. Repoint apply's three internal call sites (apply.go:225, 329, and inside `applyValidationNote`:555) to `verify.Delta`, and the direct `computeDelta(...)` calls in `apply_test.go:241-252` to `verify.Delta`. `gate`, `gateOutcome`, `applyValidationNote`, and the manifest writers stay in `apply` (rollback/quarantine policy is apply's). No import cycle — apply already imports verify. No behavior change — pure move.
- **Test first:** the `computeDelta` cases now in `apply_test.go` move to `verify/delta_test.go` as `TestDelta` (same fixtures: fixed / newly / already / indeterminate-dropped). apply's tests stay green via the repoint.

### Task 1.2 — `verify.SaveBaseline` / `LoadBaseline` / `BaselineStale` (snapshot, read-only)
- **Files:** `internal/humify/verify/baseline.go` (new) + `baseline_test.go` (new); repoint git helpers in `internal/humify/apply/apply.go`.
- **Scope:**
  - **Extract the read-only git inspectors to verify** (they belong with verify's "inspect repo state" role and `SaveBaseline` needs them): move `gitHead` → `verify.GitHead(root) (string, bool)` and `repoDirtyExcludingHumify` → `verify.RepoDirtyExcludingHumify(root) bool` from apply to verify; repoint apply's callers to the verify versions. `gitRestore`/`gitDirty` (apply-specific, source-mutating or unused elsewhere) stay in apply. verify already imports `state` (for `state.Dir`), so no new dependency.
  - **Snapshot type:**
    ```go
    type BaselineSnapshot struct {
        Schema    int        `json:"schema"`
        SavedAt   string     `json:"saved_at"`   // RFC3339, injected now
        HeadSHA   string     `json:"head_sha"`   // "" if not a git repo
        Dirty     bool        `json:"dirty"`      // tree dirty (outside .humify) AT SAVE — see ordering hazard
        Result    Validation `json:"result"`
    }
    ```
  - `SaveBaseline(root string, v Validation, now time.Time) error` — stamp `SavedAt`, `HeadSHA` (`GitHead`), `Dirty` (`RepoDirtyExcludingHumify`), marshal to `.humify/verify-baseline.json` (writes ONLY under `.humify/` — create the dir if missing, mirroring how analysis/plan are persisted).
  - `LoadBaseline(root string) (BaselineSnapshot, bool)` — read+unmarshal; ok=false if absent/corrupt.
  - `BaselineStale(snap BaselineSnapshot, root string) bool` — true when `snap.HeadSHA != ""` and current `GitHead` differs (a commit landed between save and verify). Reuses the #10 stale-plan pattern (compare persisted SHA to live).
- **Test first:** `TestSaveLoadBaselineRoundTrip` (save a Validation, load it back, fields intact); `TestBaselineStaleOnHeadChange` (toolchain-gated real git: save at SHA1, commit, assert `BaselineStale` true; no commit → false); `TestSaveBaselineRecordsDirty` (dirty tree at save → snapshot.Dirty true).

### Task 1.3 — wire `--save-baseline` and `--baseline` into the verify command
- **Files:** `humify.go` (`cmdVerify`), `main.go` (flags + usage), `render.go` (delta rendering), `main_test.go`/`render_test.go`.
- **Scope:** two flags on `verify`:
  - `--save-baseline`: run `verify.Run`, then `verify.SaveBaseline`. If the tree is dirty (snapshot.Dirty), print a LOUD warning that the baseline may already contain un-saved edits and to save before editing. Confirm where it was written.
  - `--baseline`: run `verify.Run` (post), `LoadBaseline`. Render via `verify.Delta`:
    - **newly failed** → "test newly failed — caused by your uncommitted change"
    - **already failing** → "build already failing before your change (ambient — run `humify doctor`)"
    - **fixed** → "this change fixed: X"
    - **indeterminate kinds** (post `ExitCode < 0`, dropped by `Delta`) → an explicit "couldn't compare: X (timed out / failed to launch)" line, so a flaky/uninstalled command never silently vanishes and reads as clean.
    - If `BaselineStale` → prepend a staleness warning (baseline predates a commit; re-save).
    - If the saved snapshot was `Dirty` → note the comparison may understate regressions.
  - **Loud graceful-degrade:** `--baseline` with no saved snapshot prints "no saved baseline — can't compare; run `humify verify --save-baseline` before editing" and fall to a plain single run. A quiet degrade is the original gap wearing a success message — keep it loud.
  - Without either flag, `cmdVerify` behavior is unchanged.
- **Test first:** `TestVerifyBaselineRendersDelta` — feed a saved baseline + post with a newly-failing kind; assert output names it as the change's fault, not ambient. `TestVerifyBaselineNoSnapshotIsLoud` — `--baseline` with no snapshot mentions `--save-baseline`. `TestVerifyBaselineIndeterminateShown` — a post indeterminate kind appears as "couldn't compare".

### Task 1.4 — teach the loop: save step in the spec, compare step after
- **Files:** `internal/humify/plan/plan.go` (`buildAgentSpec` / `validationStrategy`), `render.go` (post-plan `next:` hint), `plan_test.go`.
- **Scope:** the mechanism is dead if the AI only learns about `--save-baseline` *after* it edits. So:
  - Behavior-changing items' `AgentSpec` gains an explicit FIRST step: "Before editing, capture a baseline: `humify verify --save-baseline`." The post-edit step becomes `humify verify --baseline`.
  - The post-plan `next:` hint (render.go) recommends the same two-step flow.
  - Pairs with Task 2.1 (behavior-preserving items skip verify and steer to `analyze` instead, so they don't get the baseline steps).
- **Test first:** `TestAgentSpecHasBaselineSaveStep` — a `swallowed_error` (behavior-changing) item's spec mentions `--save-baseline` before `--baseline`.

---

## Phase 2 — blind-canary follow-ons (small)

### Task 2.1 — steer comment/structure items to re-analyze, not verify  *(finding a)*
- **Files:** `internal/humify/plan/plan.go`, `plan_test.go`.
- **Scope:** add `behaviorPreserving bool` to `signalDescriptor` (true for `noisy_comment`, `todo_marker`, `vague_name`). `validationFor` returns a "re-run `humify analyze` to confirm the finding cleared" strategy for those, keeping `verify`(+`--baseline`) for behavior-changing signals.
- **Test first:** `TestValidationStrategyByBehaviorClass` — a `noisy_comment` item's strategy mentions `analyze`; a `swallowed_error` item's mentions `verify`.

### Task 2.2 — narrow `todo_marker` to low-information markers  *(finding b)*
- **Files:** `internal/humify/analyze/slop.go`, `analyze_test.go`.
- **Scope:** `todo_marker` fires only on a bare/vague marker (`// TODO`, `// FIXME: fix this`); exempt a marker that names a concrete constraint or references an issue/ticket (`#123`, `JIRA-`, a URL, or >~6 words naming a reason). Same spirit as `intentional()`/`suppressedAt()` — don't cry wolf on documented deferrals.
- **Test first:** `TestTodoMarkerSkipsDocumentedDeferrals` — bare `// TODO` fires; `// TODO: blocked on app-core.js modularization, see #42` does NOT.

### Task 2.3 — show a "since last analysis" delta  *(finding c)*
- **Files:** `internal/humify/analyze/analyze.go` (or `render.go`/`status`), test.
- **Scope:** when a prior `analysis.json` exists, `analyze`/`status` output a one-line delta (`findings 155→151; todo_marker 4→0`) so small wins are visible even when the headline score is sticky (which is correct for info-only changes). **Read the prior analysis BEFORE overwriting it** (the run persists the new analysis; capture the old summary first). No score-formula change.
- **Test first:** `TestAnalysisDeltaLine` — given a prior summary and a new one, the rendered delta names the changed counts.

---

## Verification
- `go test ./...` green after each commit; `go vet ./...` clean at the end.
- Validate Phase 1 on the canary worktree (no `node_modules`): `verify --save-baseline`, make an edit, `verify --baseline` — build/test must report as **ambient** (failing in baseline too), NOT as the edit's regression — the exact confusion the blind agent hit.

## Out of scope
- Auto-baseline on every `verify` (opt-in flags only — avoids surprise double-runs and a stale on-disk snapshot).
- Changing the health-score formula.
- Reconstructing the baseline from git (rejected: breaks verify's read-only contract and the same-environment property).
