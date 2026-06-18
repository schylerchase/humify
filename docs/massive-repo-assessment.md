# Massive complex-infra repo — humify read-only first-contact assessment

Paste-ready prompt for an AI agent operating INSIDE a large target repo. The agent invokes
the humify binary and reads its `.humify/` JSON itself; humify never spawns an agent (no
`--agent-cmd`). Dual purpose: (A) assess the target, (B) stress-test humify at scale.
Strictly READ-ONLY — no execute/apply/git-write.

Authored by adversarial review (scale, complex-infra, humify-stress, safety/oracle) and
spot-checked against humify source on 2026-06-18: the skipDirs list, `--target`-vs-`--path`
scoping, the verify 5-min timeout, and the exit-code contract were verified accurate; the
synthesis's one false claim (heatmap-no-target "exits 0") was corrected to exit 1.

---

You are an AI coding agent operating INSIDE a large, complex-infrastructure codebase (the "target"). Your job has two halves, both READ-ONLY:

  (A) ASSESS the target — produce a first-contact maintainability assessment.
  (B) STRESS-TEST `humify` itself at massive scale — log every place humify misbehaves.

YOU drive the `humify` binary and read the JSON it writes under `.humify/`. You are the orchestrator AND the auditor — humify never spawns an agent in this run. Time every humify invocation (wall clock); a hang or multi-minute call is itself a half-(B) finding.

=========================================================
ABSOLUTE RULES — READ-ONLY FIRST CONTACT
=========================================================
WHAT "READ-ONLY" MEANS HERE: humify will not EDIT, MOVE, or DELETE source. It does NOT mean no process writes to disk. `humify verify` and `humify plan` (with coverage) SHELL-EXECUTE the target's own build/test commands (go build/vet/test, cargo build/test, npm run build/test, python -m pytest) and a coverage run. These execute arbitrary repo code with your privileges and can drop artifacts in the working tree (a stray `go build` binary in cwd, coverage temp files, a node coverage/ dir, __pycache__). Expect this. Do NOT treat a post-run dirty tree as a finding you must fix, and do NOT clean it up — cleanup is itself a mutation. If executing the repo's build/test code is unacceptable for this target, run `humify plan --no-coverage` and SKIP `humify verify` (see Step 5).

EXHAUSTIVE ALLOWLIST — these are the ONLY humify commands you may run this run:
  `humify doctor`, `humify untangle heatmap`, `humify untangle audit` (DEFAULT runner only), `humify untangle status`, `humify status`, `humify analyze`, `humify plan`, `humify verify` (with `--save-baseline`, read-only). Add `--json` to every command.
The allowlist is CLOSED: any humify subcommand NOT named above is forbidden — absence from the forbidden list below does NOT make a command permitted. Do not run a subcommand you have not confirmed in the allowlist.

FORBIDDEN — do NOT run, for any reason:
  - `humify apply` in ANY form (including bare/dry-run; default is preview but it is not on the allowlist) — touches source intent.
  - `humify undo` (runs `git revert` and rewrites history).
  - `humify untangle execute` and `humify untangle run` in ANY form (with or without `--execute`; `untangle run` is an autonomous driver that REQUIRES `--agent-cmd` — do not even try it "read-only"), `humify untangle resume`.
  - ANY `--agent-cmd=...` ANYWHERE — including `humify untangle audit --runner=spawn` / `--runner=claude` (the spawn runner REQUIRES `--agent-cmd` and would nest a coding agent inside you). Use ONLY the default `dispatch` runner (it just writes prompt files for you to read).
  - `--jobs` does nothing for read-only stages (it only affects the forbidden spawn runner) — do not reach for it to speed anything up.

YOU must also stay read-only. Make NO source edit by hand. Git is READ-ONLY: use only `git status`, `git log`, `git diff`, `git show`, `git rev-parse`, `git ls-files`, `git branch --list/-v`. NEVER `checkout/switch/reset/restore/stash/clean/add/rm/mv/commit/merge/rebase/revert/worktree`. The only writes you may make live under `.humify/` (e.g. a `.humifyignore`, see Step 2).

FLAG/SCOPING IS COMMAND-SPECIFIC (do not mix these up):
  - `untangle heatmap` / `untangle audit` REQUIRE and scope via `--target=DIR`.
  - `analyze` / `plan` / `verify` / `doctor` scope via `--path=DIR` or a positional path (default `.`). They IGNORE `--target` silently — passing `--target` to them does NOT scope them; the WHOLE repo is scanned.
  - For `apply` (forbidden anyway) `--target` means an HMF-### id, a third meaning.
Run every command from the repo root and use the right flag per command.

NO STAGE EXCEPT verify HAS A TIMEOUT. humify only self-aborts inside `verify`/coverage (each command capped at 5 minutes). scan/heatmap/analyze/plan-analyze can run to completion or hang forever. So YOU impose a wall-clock cap on every read-only stage: run each as `timeout 300 humify <stage> ... --json` (or launch it in the background and watch elapsed time). If a stage trips your cap, kill it and record, as a half-(B) PERF-CLIFF finding: the exact stage, elapsed seconds at kill, the last stdout/stderr line printed, and whether any partial artifact (.humify/HEATMAP.md, .humify/intel/areas.json, .humify/analysis.json, .humify/plan.json) was written before the kill. A killed stage is a finding, not a humify success.

TWO INDEPENDENT SUBSYSTEMS — divergence between them is BY DESIGN, never a bug:
  - The untangle/heatmap layer (Steps 2–3) decomposes the tree into areas + waves.
  - The top-level analyze/plan layer (Steps 4–6) does whole-repo HMF-### ranking.
They use different packages and different scan/skip logic, so they can group files differently and report different file counts on the SAME repo. Also: `humify plan` (HMF-### items) is NOT `humify untangle plan` (a per-area wave stage) — use only `humify plan` in Step 4.

=========================================================
STEP 0 — humify readiness
=========================================================
Run `humify doctor --json`. Confirm: git present, stack detected, repo readable. If doctor reports it is not a humify project or cannot read the tree, record why and STOP — everything downstream depends on it.

=========================================================
STEP 1 — GIT RECONNAISSANCE (before the mid-refactor lens)
=========================================================
Establish ground truth from git (read-only verbs only):
  - current branch vs default branch; ahead/behind divergence
  - `git status` cleanliness
  - churn: `git log --oneline -30`, most-changed paths
  - DUAL implementations (old + new side by side), feature-flagged swaps, half-deleted modules — signs of an in-flight refactor.
Decide and write down ONE thing: IS THIS REPO MID-REFACTOR? Base it ONLY on git evidence. If clean on the default branch with no divergence and no dual implementations, state plainly "not mid-refactor" and SKIP the Step 7 lens — do not invent one.

=========================================================
STEP 2 — THE SCALE MAP (primary map; not whole-repo analyze)
=========================================================
Run `humify untangle heatmap --target=. --json`. It decomposes the tree into areas, builds the area dependency graph, partitions areas into dependency-first parallel "waves" (topological sort), surfaces import cycles, and ranks areas by risk. It writes `.humify/HEATMAP.md` AND the machine-readable record at `.humify/intel/areas.json`.

READ `.humify/intel/areas.json` yourself — do NOT trust the one-line summary. Capture these EXACT keys (there is no "scanned" key): `source_files` (the file count), `areas`, `edges`, `waves`, `cycles`, plus per-area rows carrying `wave`, `in_cycle`, `max_file_loc`, and risk score/LOC. If `source_files` is absent or zero while the tree clearly has source, that itself is a half-(B) finding.

CRITICAL SCAN-SCOPE FACT (governs Steps 2, 3 and the count check in Step 8): the heatmap/audit scan path does NOT read `.gitignore` or `.humifyignore`. It prunes ONLY a fixed directory-name list: node_modules, .git, dist, build, vendor, .humify, testdata, coverage, .next, out, target, bin, obj, .venv, __pycache__, .idea, .vscode. Any generated/vendored/build tree whose directory name is NOT on that list (e.g. gen/, proto/, third_party/, .nuxt, `venv` without a dot, a custom build dir named only in .gitignore) IS scanned and counted as source. (`.humifyignore`/`.gitignore` ARE honored by analyze/plan/verify/doctor — a different scan path — but NOT by heatmap/audit.) Before trusting area/wave/file counts: list top-level dirs and flag any large generated tree NOT on that fixed list; its inclusion is a humify limitation — log it for (B). For analyze/plan only, if this repo keeps large generated/fixture/vendored trees under non-standard, non-gitignored names, you MAY write a `.humifyignore` (writes only under your read-only scope) listing those dir names so analyze/plan don't waste minutes; record any dir you ignored. Do not expect it to shrink the heatmap scan.

IF A STAGE IS SLOW OR TRIPS YOUR CAP, narrow and re-run rather than give up: pick the single biggest top-level area and re-run scoped — heatmap with `--target=<subdir>`, analyze/plan with `--path=<subdir>`. Record that you sampled a sub-tree, so the assessment's counts are clearly PARTIAL.

OOM IS A SPECIFIC, NARROW VECTOR — not a general "too many files" risk. Per-file contents are read one at a time and dropped; thousands of normal files cause single-threaded SLOWNESS, not OOM. The only OOM trigger is a SINGLE oversized file that escapes binary/minified detection and gets read uncapped (heatmap reads every source file TWICE — once for LOC/branches, once for import extraction — which also explains why heatmap is often the slowest stage). If a stage dies with out-of-memory / killed-9, hunt for one pathological large non-minified file (check the largest files by size) and report it as the trigger. If it merely runs long, report single-threaded slowness, not OOM.

=========================================================
STEP 3 — AUDIT FAN-OUT (you are the auditor)
=========================================================
Run `humify untangle audit --json` with the DEFAULT runner (no `--runner`, no `--agent-cmd`). The default `dispatch` runner only WRITES one read-only auditor prompt per pending area under `.humify/tmp/` and reports the prompt paths — it spawns nothing. Read those prompt files yourself; each scopes exactly one area's file slice. Use them to guide YOUR read-only inspection of the highest-risk areas from Step 2. Areas in the same wave are independent; later waves depend on earlier ones.

PREREQUISITE: audit depends on heatmap intel. If audit returns `reason_code:"no_intel"`, that is a MISSING PREREQUISITE (re-run heatmap first), NOT a humify bug. Only log it as a bug if it returns `no_intel` AFTER a heatmap that reported `source_files`>0.

=========================================================
STEP 4 — FINDINGS + RANKED PLAN
=========================================================
Run `humify analyze --json` (whole-repo smell scan + five health scores: readability, maintainability, correctness risk, testability, efficiency) and `humify plan --json` (ranks findings into HMF-### items; runs analyze first if needed). Read `.humify/analysis.json` (its `files` array is worst-first) and `.humify/plan.json` (HMF-### items in rank order; each carries `signal`, `action`, `applyable`, `verification`). Do NOT apply anything — report the ranking only.

PLAN COVERAGE IS THE MOST LIKELY STALL. `humify plan` (with coverage) runs the instrumented suite (go test -coverprofile ./..., or c8 for JS) capped at 5 minutes per command — on a massive repo this is the single likeliest hang point, and on timeout it ABORTS (does not hang forever) and every item falls back to `unmeasured`. If it trips your cap, you may re-run `humify plan --no-coverage` to skip instrumentation; then state explicitly that EVERY plan item is `unmeasured` because coverage was skipped, and log the original timeout as a half-(B) perf finding.

=========================================================
STEP 5 — BASELINE VERIFY (honest verdict)
=========================================================
(SKIP this step entirely if you decided in the ABSOLUTE RULES not to execute the repo's build/test code.)
Run `humify verify --save-baseline --json`. This snapshots the pre-edit validation result into `.humify/verify-baseline.json` and ALWAYS exits 0 — an already-red checkout is expected and must NOT be read as a failure you caused. You will not edit source, so there is no `--baseline` re-run; the baseline-classification verdicts (newly_failing / already_failing / fixed / indeterminate) only apply to an after-edit diff and are out of scope here. Report the RAW validation state honestly:
  - Read `validated` and `passed`. A result with `validated=false` and `passed=true` is a VACUOUS pass (nothing ran) — never report it as green.
  - STRONGEST no-oracle signal: a single result of shape `{kind:"all", skipped:true, reason:"no validation commands detected ..."}` means humify found NO validation surface at all. Report it as such, not as a routine skip.
  - List which check kinds actually ran vs were skipped. Kinds: test, build, vet (Go-ONLY — its absence on a JS/Python target is EXPECTED, not under-coverage), lint, typecheck. Declared-but-skipped scripts are scope gaps, not passes.
  - Note any check that timed out (exit code -1; humify caps each command at 5 minutes) — a timeout is a perf observation about this repo, not a regression you caused.
VERIFY DETECTS COMMANDS AT THE REPO ROOT ONLY (root go.mod / Cargo.toml / package.json scripts / Makefile / pytest config). On a monorepo with manifests in SUBDIRECTORIES this yields zero commands and a vacuous pass. When `validated=false`, do NOT conclude "no test surface" — run `find . -name go.mod -o -name package.json -o -name Cargo.toml -o -name pyproject.toml -o -name pom.xml -o -name build.gradle` (pruning vendor/node_modules). If any manifest exists below root, humify UNDER-DETECTED the validation surface: record it as a half-(B) finding DISTINCT from a genuinely untested repo. (Even with a root go.mod, `go test ./...` skips nested go modules.)

=========================================================
STEP 6 — WEIGHT COVERAGE VERDICTS (a DIFFERENT system from Step 5)
=========================================================
This is the per-plan-item COVERAGE verdict, read off each `plan.json` item's `verification` field — do NOT confuse it with Step 5's baseline classification.

DECIDE FIRST: does a behavior oracle exist at all? It exists ONLY if Step 5 shows a `test` kind with ran=true AND passed=true. Coverage instrumentation is wired ONLY for Go (go test -cover) and JS (via c8, and only if c8 is installed). On Rust/Java/C#/Ruby/C++/PHP, or a JS repo without c8, EVERY item is `unmeasured` by design — a humify coverage gap, not a property of the code. If NO oracle exists (non-Go/non-JS target, c8 absent, no tests, or the suite is red): report every item as RANKED, SAFETY-UNVERIFIABLE — the rank gives ORDERING, not safety — and state plainly that humify could not measure behavior on this target. Never present an `unmeasured` or `build-only` item as safe-to-apply.

If an oracle DOES exist, weight each applyable item by its verdict, but trust each only as far as these caveats allow:
  - `behavior-verified` is NOT self-certifying. The coverage run ignores the test suite's exit code, so a file touched by a suite that errored/panicked partway is still stamped covered. If Step 5 shows `test` ran=false or passed=false, DOWNGRADE every verdict — including behavior-verified — to no-trustworthy-oracle.
  - `build-only` does NOT mean "it compiles." It means: measured run, this file not exercised — reason unknown (could be unexercised, OR its package failed to compile, OR the suite died early, OR there are zero tests). Compilation is certified ONLY by Step 5's build/vet kinds, never by the coverage verdict.
  - `unmeasured` = no coverage tooling could run — no safety signal at all.
An item is only as trustworthy as its weakest file (a multi-file item is `build-only` if even one file was not executed).
(For a single "do-this-first / the prize / where to stop" read, apply the rank-then-judge pattern in your head over the ranked items; humify deliberately does not provide this as a command — never persist it.)

=========================================================
STEP 7 — MID-REFACTOR LENS (only if Step 1 said mid-refactor)
=========================================================
ONLY if Step 1's git evidence showed an in-flight refactor, classify the notable findings into:
  - OLD-LEFTOVER — code the refactor is replacing, not yet removed.
  - NEW-UNWIRED — new code added but not yet connected/called.
  - INTRODUCED — smell introduced BY the in-flight work (worse than before).
  - PRE-EXISTING — predates the refactor; ambient debt.
Tie EVERY classification to a git fact (commit, branch divergence, dual implementation). If Step 1 said "not mid-refactor," skip this entirely.

=========================================================
STEP 8 — HUMIFY MISBEHAVIOR LOG (half B — the stress test)
=========================================================
For each check, decide BUG / EXPECTED / N-A using the discriminator, and cite the JSON counts and timings you actually read.

  1. SINGLE WAVE / ZERO EDGES — get the language taxonomy right before judging. humify RESOLVES intra-repo edges from only (a) relative imports beginning with `.` in JS/TS, and (b) Go module-path imports via go.mod. Beyond that:
     - Python relative imports (`from .x import`) ARE extracted but do NOT resolve (the token cleans to a directory, never the sibling file) — Python collapses to one wave BY DESIGN.
     - Rust (`use a::b;`), C# (`using X;`), and Ruby (`require 'x'`, no parens) are NOT EVEN EXTRACTED (extraction matches only quoted `import`, `require(...)`, `from..import`, and `#include`) — say "not extracted," not "extracted-but-unresolved."
     - SCANNED-BUT-NEVER-RESOLVED languages (.php .swift .kt .scala .sh .ps1 .lua .vue .svelte .mjs .cjs) get areas, LOC, and risk scores but ALWAYS zero edges and one wave — EXPECTED, not a bug.
     So edges==0 is EXPECTED for any non-Go, non-(JS/TS-with-relative-imports) target. The ONLY loud-regression case is: root go.mod present AND intra-repo Go imports exist AND edges==0. Before flagging it, CONFIRM: pick two Go files you can see import each other across packages (grep the module-path import), then check whether the corresponding area→area pair produced an `edge`. Flag loudly only if a confirmed cross-package import produced NO edge. A genuinely flat single-package Go repo legitimately has edges==0. For a JS/TS repo, before calling edges==0 a bug, grep for `from './` / `require('./`; if the repo imports only via path-aliases or bare package names, edges==0 is expected.
  2. NONSENSICAL DECOMPOSITION / MEGA-AREA. Areas are grouped by TOP-LEVEL directory (plus each god-file as its own area). A repo that nests everything under one dir (src/, packages/, app/) collapses into one enormous dir-area — a known decomposition limitation, not a crash. Report the area LOC distribution; a single 50k-LOC area is the limitation surfacing, distinct from a hang or wrong output.
  3. MISSED IMPORT CYCLES. If you see obvious circular imports in a language humify RESOLVES (Go, or JS/TS-relative) but `cycles` is empty, that is a real miss — flag it. Cycles in unresolved/unextracted languages won't appear; that is the same coverage gap as #1, not a bug.
  4. CRASH / STACK OVERFLOW — aim at the right engine. Stack-overflow risk lives ONLY in cycle detection (Tarjan, recursive). The wave partition is iterative (Kahn) and CANNOT overflow. And because recursion traverses AREAS (top-level dirs + god-files), not files, real repos cannot nest deeply enough to overflow Go's ~1 GB stack — treat overflow as THEORETICAL. If you see a panic, it is far likelier an uncapped-read OOM (see Step 2) or another bug: capture full stderr and attribute it to cycle detection, not the wave engine, and do not pre-label it "stack overflow."
  5. PERF CLIFF / HANG. Time every call. Flag any stage that runs minutes or appears to hang (most likely: heatmap's double-read scan on a huge tree, or `plan` coverage stamping). On a kill, capture the protocol from the ABSOLUTE RULES (stage, elapsed seconds, last stdout/stderr line, partial artifacts). A per-command verify timeout (exit -1) is a perf data point, not a regression. (Note: git churn is internally capped, so it is not a scale concern — don't chase it.)
  6. DISHONEST COUNTS / EXIT CODES. Cross-check `source_files` (from .humify/intel/areas.json — NOT a "scanned" field) against `git ls-files | wc -l`. Do NOT allow for .gitignore/.humifyignore here — the heatmap path ignores both. Reconcile ONLY against the fixed skipDirs list (node_modules, .git, dist, build, vendor, .humify, testdata, coverage, .next, out, target, bin, obj, .venv, __pycache__, .idea, .vscode) plus any `*.min.*` bundles. A tree that .gitignore excludes but skipDirs does not will INFLATE humify's count above a .gitignore-aware tool — that gap IS the finding, not noise to subtract. Only an unexplained residual is a finding. Separately: analyze/plan use a DIFFERENT prune list (adds .nuxt/.turbo/.cache/venv and honors .gitignore; drops bin/obj/.idea/.vscode/testdata), so analyze's file universe and heatmap's `source_files` legitimately differ on the same repo — record the divergence, do not flag it as dishonest. Optional confirmation probe: create a top-level dir NOT in skipDirs (e.g. `generated/`), drop a source file in it, add `generated/` to .humifyignore, re-run `humify untangle heatmap --target=.`, and confirm the file is STILL counted — that confirms heatmap ignores .humifyignore; log with before/after counts. EXIT-CODE HONESTY: humify exit codes are 0=ok, 1=error (not-a-project / usage error / read failure), 2=drift/incomplete. A usage error such as `humify untangle heatmap` with no --target exits 1 (verified in source), NOT 0 — do not expect exit-0 usage errors. Still, do not infer success from the exit code alone: confirm the expected JSON (`ok:true` and the data keys) is present. A 0 exit alongside `ok:false` or missing data keys WOULD be a dishonest-output finding.
  7. VERIFY UNDER-COVERAGE — name the precise mechanism. humify surfaces NON-default validation scripts ONLY when they are colon-namespaced npm siblings of build/lint/typecheck/test (e.g. `test:unit`, `lint:ci`). It does NOT surface hyphenated or differently-named scripts (`test-integration`, `test-ci`, `e2e`, `check`, `ci`), Makefile targets, tox/nox envs, or shell test scripts — those are NEITHER run NOR reported skipped; they are invisible to the verdict. Manually grep package.json scripts, the Makefile, and CI config for test/build commands that are NOT colon-siblings; every such command is a verify blind spot — log it as under-coverage with the exact script name. Under-coverage = a green verdict narrower than the repo's real validation surface (combine with the subdir-manifest under-detection from Step 5).
  8. ARTIFACTS OUTSIDE .humify/. After any `humify verify` or `humify plan` (non --no-coverage) run, look for NEW files outside `.humify/` (stray build binary, bundled `dist/`, coverage temp/dir, `test-results/`, `__pycache__`). DO NOT use plain `git status --porcelain` alone — the biggest leak (a bundler's `dist/*.bundle.js`, `meta.json`, test-result caches) is usually GITIGNORED and so invisible to it. Detect leaks by BOTH: (a) `git status --porcelain --ignored` (shows ignored writes too), and (b) record file mtimes before the run and re-list anything modified after (e.g. `find . -newer /tmp/humify-preverify-stamp -not -path './.humify/*' -not -path './.git/*'` after `touch /tmp/humify-preverify-stamp` just before the run). Any such write is humify executing repo build/test code and leaking artifacts — record it as a half-(B) finding (an EXPECTED side effect of running project code, but a real caveat on the read-only claim; the gitignored writes make the leak LARGER than `git status` implies). Do NOT delete them.

=========================================================
DELIVERABLE
=========================================================
Return TWO things, then STOP (no fix-planning, no apply):
  (A) ASSESSMENT — scale (files/areas/waves/cycles; mark PARTIAL if you sampled a sub-tree), top risk areas with evidence, health scores, the ranked HMF-### items weighted by coverage verdict (or marked RANKED, SAFETY-UNVERIFIABLE if no oracle exists), the honest verify state, and (if mid-refactor) the four-bucket classification.
  (B) HUMIFY MISBEHAVIOR LOG — each of the 8 checks marked BUG / EXPECTED / N-A with the JSON counts, timings, and any crash stderr that justify it.
Keep humify's verified facts (scores, ranks, verdicts, counts) separate from your judgment. Run no forbidden command. Modify no source.
