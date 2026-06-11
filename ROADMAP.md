# Humify Improvement Roadmap

From a code-grounded, adversarially-verified audit of the codebase. Items are tiered
by impact ÷ effort. Locations are `file:line` at time of audit.

Status: ✅ done · ⬜ open

---

## High value

- ✅ **1. Indeterminate baseline silently disabled the regression gate.**
  `apply/apply.go`. A timed-out / un-launchable baseline command (`ExitCode < 0`) was
  bucketed as "already failing", so a real post-failure on that kind was waved through.
  Fixed: `gate()` now returns `OK | Regressed | Unverifiable`; an indeterminate baseline
  is never treated as a clean failure. Truth-table tests in `apply_test.go` (`TestGate`).

- ⬜ **2. Agent-path apply is destructive & dishonest on failure** *(the big one — next)*.
  `apply/apply.go` `performAgentApply`, exit mapping `humify.go`. On regression it advises
  `git checkout -- .` which strands the new untracked files the agent created; an agent
  crash returns `nil` → **exits 0**; no audit manifest for the agent path. Fix as one change:
  capture a pre-apply git SHA → `git reset --hard <sha> + git clean -fd`; refuse the agent
  path when the repo is already dirty (`RepoDirty` is computed and ignored); distinct
  non-zero exit codes (reconcile with `humify.go` keying `exitDrift` off `RolledBack`);
  write an agent manifest. E2E regression + error-exit tests. **(L)**

- ✅ **4. Typo'd flag became a no-op dry run that exited 0.** `main.go parseArgs` silently
  dropped unknown `-`flags. Fixed: unrecognized flags now error (`exitError`); `-h`/`--help`
  preserved.

- ✅ **5. `apply --json` emitted PascalCase** while the rest of the surface is snake_case.
  Fixed: snake_case json tags on `apply.Result`.

- ⬜ **3. `swallowed_error` detector — false-positives documented code, misses common idiom.**
  `analyze/slop.go:96,173-200`. `intentional()` doesn't scan brace-language *body* comments,
  so `if err != nil { // ignore: best effort }` still fires the harshest signal; and
  `goErrIfRe` misses `if err := f(); err != nil {}`. **(M + S)** — core product promise.

## Worth doing
- ⬜ **6.** Quarantine `move`/`restore` can overwrite the only reversible copy and reports a
  false "clean tree" when restore fails (`apply.go:195-215`). **(M)**
- ⬜ **7.** `broad_catch` only fires for Go/Python — invisible in Java/C++/C#/JS/TS (`slop.go:114-138`). **(M)**
- ⬜ **8.** Plan binds the agent worklist to a 5-item *display* cap → scope oversell (`plan.go:197,215`). **(M)**
- ⬜ **9.** New detector signals silently dropped if they lack a template — add a registry-completeness test (`plan.go:158`). **(S)**
- ⬜ **10.** Status shows stale scores; `plan.AnalysisAt` is written but read nowhere (`render.go:151`). **(S)**
- ⬜ **11–12.** Test gaps: agent dry-run preview never runs the agent; `buildAgentSpec` size-cap/safe short-circuit untested. **(S/M)**

## Nice to have
- ⬜ **13.** Agent constraints don't ban `.humify/` (`plan.go:267`). **(S)**
- ⬜ **14.** `status --json` emits bare `{}` with no presence flags. **(S)**
- ⬜ **15.** Comment metrics computed/persisted but read by nothing; `countKinds` miscounts delimiter lines (`analyze/metrics.go`). **(S)**
- ⬜ **16.** Signal metadata fragmented across `templates`/`signalInstructions`/`order` — unify behind one descriptor (do #9's guard first). **(M)**

## Couplings to watch
- #2's exit-code scheme and `RolledBack` renaming interact at the `exitDrift` mapping — design together.
- #9 (cheap guard) and #16 (proper refactor) target the same registry fragmentation — guard first.
- #2's pre-apply git SHA serves both the real revert and the audit manifest — capture once.
