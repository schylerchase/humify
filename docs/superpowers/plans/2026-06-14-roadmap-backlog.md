# ROADMAP Backlog Clearance — Implementation Plan

> **For agentic workers:** execute task-by-task with TDD (red → green → commit). One implementer at a time — many items share files; parallel mutation corrupts.

**Goal:** Close every open ROADMAP item (#2, #3, #6–#16) — destructive-path safety, detector correctness, test gaps, and metadata cleanup — with a failing test first for each.

**Architecture:** Single feature branch `fix/roadmap-backlog` off `main`. Sequential execution in 5 phases ordered by file-conflict and the one real dependency (#16 after #9). Atomic commit per item; `go test ./...` green after each.

**Tech Stack:** Go 1.26, zero-dependency, stdlib `testing`.

All 13 items were audited against current source (2026-06-14); every one confirmed open. Line numbers below are post-audit (the ROADMAP's had drifted).

---

## Conflict map (why this order)

| File | Items that touch it |
|------|---------------------|
| `internal/humify/analyze/slop.go` | #3, #7, (#9 repoint) |
| `internal/humify/analyze/metrics.go` | #15 |
| `internal/humify/analyze/analyze_test.go` | #3, #7, #15 |
| `internal/humify/analyze/{analyze,deadmodule,score}.go` | #9 |
| `internal/humify/plan/plan.go` | #8, #13, #16 |
| `internal/humify/plan/plan_test.go` | #8, #9, #12, #13, #16 |
| `internal/humify/apply/apply.go` | #2, #6 |
| `internal/humify/apply/apply_test.go` | #2, #6, #11 |
| `humify.go` | #2 (cmdApply), #10, #14 |
| `main.go`, `run_e2e_test.go` | #2 |
| `render.go` | #10 |

Same-file items run sequentially. `humify.go` is touched in the apply phase (cmdApply) and the status phase (statusView/printStatus) — different functions, separated by phase.

---

## Execution order (front-loaded)

The only hard cross-phase dependency is #9 → #16. apply.go is independent of slop/plan, so the brand-critical safety bugs run **first** — if the run is interrupted, the valuable items are already done:

1. **apply.go** — #6, #2, #11
2. **analyze detectors** — #15, #3, #7
3. **registry** — #9
4. **plan.go** — #13, #8, #12, #16  *(#16 last, depends #9)*
5. **status/render** — #10, #14

## Pre-flight decisions (settled before any code)

- **#2 / `.humify/` vs git (verified):** `gitDirty()` = `git status --porcelain` (respects `.gitignore`); humify only writes `.humify/.gitignore` = `tmp/`, so in a target repo that doesn't ignore `.humify/`, `humify plan` leaves untracked `.humify/*.json` → dirty=true and `git clean -fd` would delete `.humify/` (quarantine + resume state). **Design #2's git ops to exclude `.humify/`:** the refuse-dirty check ignores paths under `.humify/` (a `dirtyExcludingHumify` helper, not bare `gitDirty`), and rollback uses `git clean -fd -e .humify` (preserve state) alongside `git reset --hard <sha>` (tracked only). Optionally also have humify self-ignore its state dir, but the defensive exclusion is the load-bearing fix and is self-contained to #2.
- **#2 exit-code shape:** one `RolledBack` bool can't encode three outcomes (crash→exitError, regression→exitDrift, refuse-dirty→non-zero, nothing-applied). Decision: crash and refuse-dirty return a **non-nil error** → `cmdApply` maps to `exitError`; regression keeps `RolledBack=true` → `exitDrift`. Preserve the quarantine path's existing `exitDrift`-off-`RolledBack` semantics. (Avoids an enum; three outcomes via {error, RolledBack, neither}.)
- **Test isolation (#2/#6):** every test that runs `git reset --hard` / `git clean -fd` MUST assert `root` is the `t.TempDir()` before the destructive op — a misrouted `root` would nuke the real working tree. Cheap guard, catastrophic miss.

---

## Phase — analyze detectors (slop.go / metrics.go)

### Task #15 — delete dead comment/code/blank metrics
- **Files:** `analyze/metrics.go`, `analyze/analyze_test.go`
- **Scope:** `countKinds` output (`m.Code/Comment/Blank`, `CommentRatio`) is read by nothing in production (only `m.LOC` from metrics.go:61 and the structural metrics are consumed by `metricFindings`). Delete `countKinds` (metrics.go:84-100), its call (:62), the `Code/Comment/Blank/CommentRatio` fields (:15-18) and `CommentRatio` calc (:69-71). The delimiter-line miscount becomes moot. Minor JSON schema change (drops `code/comment/blank/comment_ratio` from `metrics`).
- **Test first:** edit `TestMeasureBraceLongestAndNesting` to drop the `m.Comment` assertion (analyze_test.go:36-38); green = package builds with fields gone and surviving metric tests pass. Do NOT add a `comment==3` assertion (that asserts the rejected fix-and-wire path).

### Task #3 — swallowed_error: body-comment FP + assignment idiom miss
- **Files:** `analyze/slop.go`, `analyze/analyze_test.go`
- **Scope (B, small):** widen `goErrIfRe` (slop.go:96) to `^\s*if\s+(?:[^;{]*;\s*)?err\s*!=\s*nil\s*\{\s*$` (allows `if err := g(); err != nil {`). Keep `err` hardcoded. **(A, medium):** make intent detection span-aware for brace langs — honor comments on the *interior* lines i+1..j-1 of a multi-line empty `if err != nil { … }`, NOT by extending `catchLines` to `[i,i+1]` (that regresses single-line catches with a trailing comment). Same-line `emptyCatchRe` path keeps using line i only.
- **Test first (3 asserts, the 3rd is the discriminator):** (1) Go multi-line body-comment swallow → `swallowed_error` ABSENT (fails today); (2) `if err := g(); err != nil {}` → PRESENT (fails today); (3) single-line empty JS catch with a comment on the *following* line → still PRESENT (guards the naive fix).

### Task #7 — broad_catch invisible in all brace languages
- **Files:** `analyze/slop.go`, `analyze/analyze_test.go`
- **Scope:** the brace branch (slop.go:106-120) emits only `swallowed_error`, never `broad_catch`. Add catch-all/over-broad regexes — JS/TS `catch (e) {` / bare `catch {`; Java/C# `catch (Exception|Throwable|RuntimeException …)`; C++ `catch (...)`. Empty broad catch stays `swallowed_error` (more severe, never double-count); bodied broad catch → `broad_catch` unless `suppressedAt()`. **FP guard:** narrow typed catches (`catch (IOException e)`) must NOT fire.
- **Test first:** `TestInspectBraceBroadCatch` — Java/C#/C++/JS broad → `broad_catch` true & `swallowed_error` false; narrow Java → false; empty broad → `swallowed_error` true & `broad_catch` false (exclusivity).

---

## Phase 2 — signal registry (#9), gates #16

### Task #9 — registry-completeness guard
- **Files:** `analyze/analyze.go` (+ `slop.go`, `deadmodule.go`, `score.go` to repoint literals), `plan/plan_test.go`
- **Scope:** signal names are scattered string literals with no source of truth, so a hardcoded list in the test would be tautological. Export a canonical `analyze.Signals()` (placed where detectors live so adding one updates it); repoint emit sites + `score.go countSignal` callers to the constants. Then assert completeness against the maps in plan.
- **Test first:** `TestSignalRegistryCompleteness` in plan_test.go — imports `analyze`, iterates `analyze.Signals()`, asserts `templates[sig]` exists for all, and `signalInstructions[sig]` exists for every non-`safe` template. Must reference the exported list, never a list re-hardcoded in the test. Prove the guard bites by temporarily adding a template-less dummy signal (then revert).

---

## Phase 3 — plan.go

### Task #13 — ban `.humify/` in agent constraints
- **Files:** `plan/plan.go`, `plan/plan_test.go`
- **Scope:** add `.humify/` (and siblings `.humify-dev/`, `-runs/`, `-worktrees/`) to the Constraints block in `buildAgentSpec` (plan.go:301-308). `3169aff` added a generated-dir ban but not this; the scratch-file ban only forbids *creating*, not *modifying* humify's state dir. Safe: humify writes `.humify/delete-me/` itself, never via the agent.
- **Test first:** `TestAgentSpecBansHumifyDir` — build a non-safe item's AgentSpec, assert `strings.Contains(spec, ".humify")` in the Constraints section; also assert the existing `node_modules` ban survives.

### Task #8 — agent-spec evidence must match the worklist
- **Files:** `plan/plan.go`, `plan/plan_test.go`
- **Scope:** `buildAgentSpec` lists *all* `item.Files` under "Files to modify" but caps "Evidence" at 5 (plan.go:225) → spec commands edits to files it gives no evidence for. Fix: emit evidence for every file under "Files to modify" (per-file evidence map), or add an explicit truncation marker. Prefer full evidence — the agent acts on this verbatim. The 5-cap for *human* render may stay.
- **Test first:** `TestAgentSpecEvidenceMatchesWorklist` — one signal, 8+ findings across 8+ files; assert every file under "Files to modify:" also appears under "Evidence".

### Task #12 — buildAgentSpec size-cap + safe short-circuit tests
- **Files:** `plan/plan_test.go`
- **Scope:** pure test add. `agentFileSizeLimit=3000` routes large files to a "too large" excluded section; safe signals short-circuit to `""`. Neither is tested.
- **Test first:** `TestBuildAgentSpecSizeCapAndSafeShortCircuit` — construct `analyze.Analysis` with a `FileScore{Metrics:{LOC:4000}}` + a small file and `long_function` findings; assert big file only in the excluded section (with LOC), small file only in modifiable; assert a `stale_file` item's AgentSpec == `""`.

### Task #16 — unify signal metadata behind one descriptor  *(depends #9)*
- **Files:** `plan/plan.go`, `plan/plan_test.go`
- **Scope:** collapse `templates` + `signalInstructions` + `order()` into one `signalDescriptor` struct and a single `descriptors` registry; repoint `buildItem`, `buildAgentSpec`, `order`, `penaltyBySignal`. Resolves existing drift (missing `dead_module` instruction; dead `stale_file` instruction). Do LAST in the phase, guarded by #9's completeness test + all prior plan tests staying green.
- **Test first:** rewrite #9's completeness assertion as `TestSignalDescriptorRegistryIsComplete` against the single registry (non-safe ⇒ has instruction; no safe-signal dead instruction; every handled signal has an order tier).

---

## Phase 4 — apply.go (destructive paths)

### Task #6 — collision-safe + honest move/restore
- **Files:** `apply/apply.go`, `apply/apply_test.go`
- **Scope:** (A) `move`/`restore` use bare `os.Rename` (apply.go:224, 236) which silently overwrites — guard with an existence check so a prior quarantined copy is never clobbered. (B) `restore` swallows all errors (`_ = os.Rename`), and rollback callers (apply.go:124-127, 133-137, 140-143) assert success unconditionally → reports "No files were moved" even when a file is stranded. Make `restore` return an error; at each call site, on failure set `RolledBack=false`, drop the clean-tree wording, and name the stranded file.
- **Test first:** `TestRestoreFailureReportedHonestly` (force a restore failure → message must NOT claim clean rollback, must name stranded file, quarantined copy still on disk); `TestMoveRefusesToOverwriteExistingQuarantine` (pre-seed destination → sentinel preserved, not clobbered).

### Task #2 — agent-path apply: refuse-dirty + hard rollback + manifest + exit codes  *(the big one, L)*
- **Files:** `apply/apply.go`, `humify.go`, `main.go`, `apply/apply_test.go`, `run_e2e_test.go`
- **Scope:** in `performAgentApply` (apply.go:161-184): (a) refuse when `gitDirty(root)` (already computed at :163, ignored) → state maps to non-zero; (b) capture pre-apply SHA (`git rev-parse HEAD`); (c) on agent crash AND on gate regression, `git reset --hard <sha>` + `git clean -fd`, set `RolledBack=true` so existing `exitDrift` fires — fixes strand-untracked + crash-exits-0 in one move; (d) on success write an agent manifest (SHA, item, validation delta). In `humify.go cmdApply`: crash/refusal/regression all return non-zero (distinct codes; add to main.go:30-32 or at minimum crash→exitError, regression→exitDrift). Preserve quarantine's `exitDrift`-off-`RolledBack` semantics.
- **Test first:** unit — `TestPerformAgentApply_RefusesDirtyRepo`, `_CrashRollsBackAndSignalsFailure` (tracked file restored, untracked gone, non-zero), `_RegressionRollsBackHard`, `_WritesManifestOnSuccess`; E2E in run_e2e_test.go (mirror `buildFakeAgent`) — crash/regression → non-zero exit + clean `git status`.

### Task #11 — agent dry-run preview test
- **Files:** `apply/apply_test.go`
- **Scope:** pure test add. The preview branch (apply.go:83-90: `unsafePermission && agentCmd!="" && !Applyable && (dryRun||!yes)` → returns spec, never runs agent) is uncovered.
- **Test first:** `TestApplyAgentDryRunPreviewDoesNotExecute` — `agentCmd="false"` (exits non-zero if ever run), `dryRun=true, yes=false, unsafePermission=true`; assert `DryRun`, `!Applied`, message contains "would spawn agent" + the spec, and no side effect / agent never invoked.

---

## Phase 5 — status / render

### Task #10 — flag a stale plan
- **Files:** `render.go`, `humify.go`
- **Scope:** `plan.AnalysisAt` is written (plan.go:189) but read nowhere. In `printStatus` (render.go), when both present, compare `p.AnalysisAt` to `a.GeneratedAt`; if they differ, append a "stale (analysis changed since plan)" marker. Optionally mirror a `plan_stale` bool into `statusView`.
- **Test first:** new `render_test.go` (package main) — `TestPrintStatus_FlagsStalePlan` (mismatched timestamps → marker present), `TestPrintStatus_NoStaleWhenMatched` (equal → absent).

### Task #14 — status --json presence flags
- **Files:** `humify.go`
- **Scope:** `statusView` (humify.go:343-355) inserts a key only when present → empty state marshals to bare `{}`, absent indistinguishable from empty (human path already disambiguates). Always set `have_analysis/have_plan/have_validation` booleans.
- **Test first:** `TestStatusView_PresenceFlags` — all-false → the three bools present & false, payload keys absent; `haveA=true` → `have_analysis==true` & `analysis` present.

---

## Protocol
- Branch `fix/roadmap-backlog` off `main`.
- One item at a time: write failing test → confirm red → minimal impl → green → `go test ./...` → commit.
- Conventional commit per item: `fix(scope): …` / `test(scope): …` / `refactor(plan): …`. No attribution lines.
- After all 13: full `go test ./...` + `go vet ./...`, update ROADMAP.md (check items, reconcile #13 note), then finish-branch (merge to main).

## Verification
- `go test ./...` green after every commit.
- `go vet ./...` clean at the end.
- ROADMAP.md: all 13 marked ✅ with one-line resolution notes.
