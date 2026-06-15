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

- ✅ **2. Agent-path apply is destructive & dishonest on failure.** `apply/apply.go`
  `performAgentApply`. Fixed: requires a clean git repo (dirty-check excludes `.humify/`),
  captures the pre-apply commit, and on crash OR regression runs `git reset --hard <sha>`
  + `git clean -fd -e .humify` to fully restore. Crash/refuse exit non-zero (error), a
  regression is drift (`RolledBack`→`exitDrift`), success writes `.humify/apply/<id>.json`
  with the base SHA. Unit + toolchain-gated E2E tests.

- ✅ **4. Typo'd flag became a no-op dry run that exited 0.** `main.go parseArgs` silently
  dropped unknown `-`flags. Fixed: unrecognized flags now error (`exitError`); `-h`/`--help`
  preserved.

- ✅ **5. `apply --json` emitted PascalCase** while the rest of the surface is snake_case.
  Fixed: snake_case json tags on `apply.Result`.

- ✅ **3. `swallowed_error` detector — false-positives documented code, misses common idiom.**
  `analyze/slop.go`. Fixed: intent detection now spans the whole block (i..closeLine), so
  the multi-line `if err != nil { // ignore }` body comment is honored while a single-line
  catch is not excused by a following-line comment; `goErrIfRe` accepts an optional
  assignment header (`if err := f(); err != nil {}`).

## Worth doing
- ✅ **6.** Quarantine `move`/`restore` overwrote the only reversible copy and reported a
  false "clean tree" when restore failed. Fixed: `move` refuses an existing destination;
  `restore` returns an error naming stranded files; rollback callers no longer claim a clean
  tree on a failed restore (`apply.go`).
- ✅ **7.** `broad_catch` was Python/Ruby-only. Fixed: detects catch-all handlers in JS/TS
  (any catch), Java/C# (`Exception`/`Throwable`/`RuntimeException`/`Error`), and C++
  (`catch(...)`); narrow typed catches don't fire; promise `.catch()` excluded (`slop.go`).
- ✅ **8.** Agent worklist oversold scope vs a 5-item evidence cap. Fixed: `buildAgentSpec`
  emits evidence for every modifiable file via `evidenceFor`; the 5-cap stays only for the
  human view (`plan.go`).
- ✅ **9.** New detector signals could be silently dropped. Fixed: `analyze.Signals()`
  canonical registry + `TestSignalDescriptorRegistryIsComplete` (proven to fail on an
  unregistered signal).
- ✅ **10.** `plan.AnalysisAt` was written but read nowhere. Fixed: `printStatus` flags the
  plan stale when the on-disk analysis changed since (`render.go`).
- ✅ **11–12.** Test gaps closed: agent dry-run preview (never runs the agent) and
  `buildAgentSpec` size-cap / safe short-circuit are now covered.

## Nice to have
- ✅ **13.** Agent constraints now ban `.humify/` and its siblings (`plan.go` buildAgentSpec).
- ✅ **14.** `status --json` now emits `have_analysis`/`have_plan`/`have_validation` presence
  flags (absent vs empty distinguishable) (`humify.go`).
- ✅ **15.** Removed the unread `code`/`comment`/`blank`/`comment_ratio` metrics (the
  delimiter miscount became moot) (`analyze/metrics.go`).
- ✅ **16.** Signal metadata unified behind one `signalDescriptor` registry keyed by the
  `analyze.Signal*` constants; resolved the missing-`dead_module` / dead-`stale_file`
  instruction drift (`plan.go`).

## Couplings (resolved)
- #2 reused the existing `{error, RolledBack}` outcomes rather than a new exit code: crash/refuse → error→`exitError`, regression → `RolledBack`→`exitDrift`. No `main.go` change needed.
- #9's guard landed first; #16 then collapsed the three maps and re-expressed the guard against the single registry.
- #2's pre-apply git SHA is captured once and serves both the hard rollback and the audit manifest.
