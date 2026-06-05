# Humify-NG: Orchestration Architecture for Untangling Massive Codebases

> Generated 2026-06-05 from a multi-agent dissection of GSD (`~/.claude/get-shit-done` + `~/.claude/agents/gsd-*`) and Deep-Plan (`deep-plan-plugin`). All claims verified against installed source files. The dissection found the root cause of the `azure_mapper` humify run failure (25 stranded audit fragments, no consolidated AUDIT, no PATCHLOG) and the GSD mechanisms that fix it.

A standalone CLI (à la `rtk`) that owns deterministic orchestration in code and delegates only judgment to agents — modeled on GSD's deterministic-engine/agent split and its fan-out→barrier→consolidate→verify discipline, with Deep-Plan's code-grounded planning bolted onto the plan stage.

---

## 1. How GSD orchestrates

GSD's core architectural decision is a **hard split between a deterministic Node engine and judgment-only agents**. The engine — `gsd-tools.cjs` (1224-line thin router) dispatching to `lib/*.cjs` — owns every operation where a wrong answer is a *bug*: state mutation, decomposition, git ops, scaffolding, fan-in merge, and verification. Agents own only what is genuinely judgment: reading code, writing plans/summaries, deciding fixes. The boundary is stated explicitly in `execute-phase.md:6`: *"Orchestrator coordinates, not executes."*

**The phase machine is derived, not stored.** `roadmap.cjs:241-292` computes each phase's `disk_status` purely by counting artifact files:

```
summaryCount >= planCount && planCount>0 → complete
summaryCount > 0                         → partial
planCount > 0                            → planned
hasResearch                              → researched
hasContext                               → discussed
else                                     → empty / no_directory
```

`current` phase = first in `{planned, partial}`; `next` = first in `{empty, no_directory, discussed, researched}`. A ROADMAP checkbox can override *upward only* (`roadmap.cjs:277` — handles externally-completed phases). **Consequence: zero stored cursor.** A context reset loses nothing because authoritative state is the file topology, re-scanned on demand. STATE.md is an explicitly bounded (<100 line) *digest/cache*, reconstructable from artifacts — never the source of truth.

**Decomposition is deterministic, in code.** `phase.cjs:cmdPhasePlanIndex` (342-567; topo-level assignment at 444, cycle abort at 500, declared-vs-computed-wave reconciliation at 532) parses every `*-PLAN.md` frontmatter, then runs **Kahn's topological sort** over the `depends_on` DAG to assign each plan a `wave = topo level`. Cycle → hard error naming the cycle members. Output is `{plans[], waves{level→[ids]}}`. **The LLM never decides parallelism — it consumes the waves map.**

**Control flow = fan-out-then-barrier, repeated per wave.** The orchestrator runs waves strictly sequentially; *within* a wave, plans run parallel only if no two share a `files_modified` entry (intra-wave overlap check, `execute-phase.md:438-470` forces sequential on conflict). A wave is a **hard barrier**: all worktrees of wave N merge back AND a post-merge build/test gate passes BEFORE wave N+1 forks.

**Fan-in (consolidation) is deterministic and manifest-driven** — the exact stage humify lacks. At fan-out, the orchestrator atomically appends `{agent_id, worktree_path, branch, expected_base}` to a `WAVE_WORKTREE_MANIFEST` temp file. After the barrier, `worktree-safety.cjs:executeWorktreeWaveCleanupPlan` (388-509) merges each branch back through a **fixed gate sequence**:

```
branch-name match → merge-base == expected_base → no deletions →
clean worktree → git merge --no-ff → worktree remove → branch -D
```

**Fail-closed**: first failed gate sets `status='blocked'`, records the reason, and `pending.push(...entries.slice(i+1))` — pushing every un-consolidated entry to an explicit `pending[]` (lines 416-480). Refuses to run at all if the manifest is missing/empty. **Nothing is silently dropped.**

**Verification is a separate code stage, not trusted to agents.** `verify.cjs:cmdVerifyPhaseCompleteness` diffs plan IDs vs SUMMARY IDs on disk → `{complete, incomplete_plans[], orphan_summaries[]}`; a plan without a SUMMARY is a hard error. `cmdValidateConsistency` cross-checks ROADMAP vs disk both directions. `execute-phase.md:5.6` runs build+test *in code* after merge, explicitly citing the **"Generator self-evaluation blind spot"** — agents report PASSED even when the merge breaks the build. The completion signal is **SUMMARY.md existence + visible commits, NOT the agent's report** (anti-pattern called out at `execute-phase.md:20, 910-912`). A reconciliation guard (`execute-phase.md:159-169`) STOPS if production commits exist but SUMMARY.md is missing — the verify-before-consolidate gate.

**The document-merge stage humify is missing already exists in GSD as a reusable model**: `audit.cjs:auditOpenArtifacts` (570) fans out 8 scanners each wrapped in `try/catch` returning a `{scan_error}` sentinel (one broken scanner never aborts the audit), counts real items, and `formatAuditReport` (643) merges all into **ONE severity-grouped report** (red/yellow/blue) with a single "N items require decisions" footer. The agent-layer equivalent is `gsd-doc-classifier` (per-file parallel worker) → `gsd-doc-synthesizer`: the synthesizer runs *once* after all classifiers, loads every classification JSON keyed by `source_path`, applies a precedence lattice (ADR>SPEC>PRD>DOC), runs cross-reference **cycle detection** (DFS three-color, max depth 50), and buckets every contradiction into auto-resolved [INFO] / competing-variants [WARNING] / unresolved [BLOCKER]. *"Synthesized, not concatenated."*

**The plan stage adds a convergence loop.** `plan-phase.md` runs a 4-role pipeline — researcher → pattern-mapper → planner → plan-checker — with a bounded **check-revise-escalate loop** (max 3 iterations + stall detector: `issue_count` not decreasing → escalate). The `gsd-plan-checker` is adversarial (FORCE stance, *"assume every plan set is flawed"*), tool-scoped **read-only** (cannot Write — structurally cannot grade its own work), and every finding carries a mandatory `BLOCKER|WARNING` severity so loop continuation = `count(BLOCKER+WARNING)`. After the LLM loop passes, **deterministic CLI gates** re-verify: requirements-coverage grep, `check.decision-coverage-plan` → `jq -e '.passed==true' || exit 1`, and a `gap-analysis` merged ✓/✗ table. Three independent layers (planner self-audit, adversarial checker, mechanical gates) catch what any one misses.

**Atomicity**: all multi-file state mutations go through `core.cjs:withPlanningLock` for cross-process safety; output >50KB spills to `@file:` temp paths and is transparently re-read, keeping huge results out of the agent token budget.

---

## 2. How Deep-Plan orchestrates

Deep-Plan (`deep-plan-plugin/.../SKILL.md`, v0.3.0) is **not** a competing orchestrator — it is a **single-agent, strictly-sequential deepening of GSD's plan stage**, and it consumes GSD's disk model directly. It is a 10–11 numbered-step linear pipeline (11 with `--review`), no fan-out, no waves:

1. Parse args / auto-detect phase **by reading GSD's derived disk_status** (`gsd-tools.cjs roadmap analyze` → "phase with CONTEXT.md but no PLAN.md").
2. Load GSD context (CONTEXT.md required, else STOP and route to `/gsd-discuss-phase`).
3. Gather GSD intelligence — reads `.planning/intel/{deps,files,apis,stack}.json` + `arch.md` for a **warm start**, with a 24h staleness check.
4. (4-10) CE-style code-grounded research → implementation units (file paths) → test scenarios → risk analysis → write GSD-compatible `PLAN.md`.
5. (11, `--review`) Optional feasibility review to catch deployment/build issues.

**How it complements GSD**: GSD's `gsd-planner` produces plans from strategic context; Deep-Plan replaces that single step with *code-grounded* planning — implementation units with concrete file paths and test scenarios, fed by GSD's pre-computed intel so the agent spends tokens on depth rather than rediscovery. It writes the *same* `PLAN.md` artifact GSD's `execute-phase` consumes, so it slots into the GSD disk contract without modification.

**How it differs**: Deep-Plan has **no orchestration discipline of its own** — no fan-out, no barrier, no synthesizer, no convergence loop. It is a producer that leans entirely on GSD for state derivation and on the GSD plan-checker (via `--review`/downstream) for verification. **For Humify-NG, Deep-Plan is the model for ONE stage (plan), not the orchestration spine.** The spine comes from GSD; Deep-Plan contributes the "load pre-computed intel → produce code-grounded implementation units with file paths + test scenarios" recipe for the planner agent.

---

## 3. The one pattern humify is missing

**Name it precisely: humify performs fan-out WITHOUT a barrier, a synthesizer, or a verify stage.** It is "scatter with no gather."

Confirmed against the framework source. `HUMIFY.md` describes a single-model **working loop** (Map→Flag→Judge→Compare→Steelman→Plan→Refactor→Verify). `HUMIFY-OPERATOR.md`'s "Expected Flow" is a linear list of document *productions* run by one model in one pass:

```
2. Build HUMIFY-MAP.md
3. Build HUMIFY-HEATMAP.md
4. Produce HUMIFY-AUDIT.md     ← single instruction, no fan-out, no synthesizer
5. Run the steelman pass
6. Produce HUMIFY-PLAN.md
8. Produce HUMIFY-PATCHLOG.md
```

There is **no concept of N parallel audit fragments, and therefore no stage that merges them.** On the `azure_mapper` run (app-core.js ~19k lines), the model correctly recognized that 19k lines exceeds a single audit pass and *improvised* a fan-out into 25 per-area fragments — a sound context-budget move. But the framework's step 4 is a single "produce HUMIFY-AUDIT.md" with **no matching gather**: no barrier to collect the 25 confirmations, no synthesizer to merge/dedup/bucket them into one `HUMIFY-AUDIT.md`, no verify stage, and so no PATCHLOG (which depends on a consolidated AUDIT). Result: 25 stranded fragments, no AUDIT, no PATCHLOG.

**The GSD mechanisms that fix it:**

| Humify gap | GSD mechanism that fixes it |
|---|---|
| Fan-out with no gather | `gsd-doc-classifier` (per-unit parallel worker, writes `{slug}-{sha256(path)[:8]}.json`) → **`gsd-doc-synthesizer`** spawned *once* after all classifiers; loads every JSON, precedence lattice, cycle detection, conflict bucketing → ONE SYNTHESIS.md |
| No barrier between scatter and gather | `ingest-docs.md:177` "collect one-line confirmations; abort if any classifier errored," THEN spawn synthesizer; the repeated **CODEX RAIL** ("stop working while subagents are live"); `docs-update.md` `TaskOutput` barrier |
| No "is consolidation done?" check | Derived disk_status (`roadmap.cjs:257-278`) + completeness diff (`verify.cjs:cmdVerifyPhaseCompleteness`): `fragments_present AND every fragment id ∈ merged index` |
| No fail-closed merge | `executeWorktreeWaveCleanupPlan` manifest-driven, per-entry gate sequence, `pending[]` on first failure |
| No verify / no bounded retry | `gsd-doc-verifier` (per-doc, adversarial, `{claims_checked,passed,failed}` JSON) → fix-loop `MAX_FIX_ITERATIONS=2`, re-verify ALL, **halt-on-regression**; and the `gsd-plan-checker` convergence loop with stall detector |
| One agent writes and grades its own work | Tool-scope the checker/verifier **read-only** — structurally cannot mutate |

The single most important fix: **a dedicated consolidator stage is mandatory, not optional**, and it must apply explicit precedence + dedup + conflict-bucketing rules in deterministic code — *"synthesized, not concatenated."*

---

## 4. Proposed binary architecture: `humify-ng`

A Rust/Node single binary (matching `rtk`'s deployment shape). The binary owns the orchestration loop *itself* — internalizing what GSD delegates to Markdown workflows (GSD's loop lives in `execute-phase.md`, invoked piecemeal; a standalone binary must own spawn/barrier/collect). Agents are invoked as subprocesses (Claude Code `Agent()`, or Codex/local fallback).

### 4.1 Command surface

```
humify map           # fan-out codebase mappers → consolidated MAP.md + intel/*.json
humify heatmap       # deterministic risk scoring over MAP → HEATMAP.md (ranks areas)
humify audit         # fan-out per-area auditors → BARRIER → synthesizer → ONE AUDIT.md
humify plan          # deep-plan-style: per-slice planners → plan-checker convergence loop
humify execute       # wave-parallel executor fan-out → fail-closed merge barrier → verify
humify patchlog      # deterministic roll-up of executed slices → PATCHLOG.md
humify status        # re-derive lifecycle state from disk (no stored cursor)
humify resume        # consume HANDOFF.json if present, else disk-derived resume
humify verify <stage># standalone re-run of any stage's deterministic gate
humify undo          # git revert --no-commit from .manifest.json (never reset)
```

`status`/`resume`/`verify` are the resilience surface; every other command is auto-resumable and re-derives its own "am I done?" answer from disk.

### 4.2 Phase state machine (derived, modeled on `roadmap.cjs`)

Per area (a unit from the heatmap), `disk_status` computed by counting artifacts — **never stored**:

```
audit fragment exists + AUDIT.md covers its id   → audited
PLAN slices exist + plan-check passed sentinel    → planned
SUMMARY per slice + commits + verify passed        → executed
PATCHLOG entry exists                              → patched
```

`humify status` = scan tree, apply rule, print per-area table + global progress %. This is the humify gap closed at the framework level: **`fragments_present(N) AND merged_output_missing ⇒ stage = "audit-incomplete", auto-resume into synthesizer.`**

### 4.3 Deterministic code (the binary) vs delegated judgment (agents)

**Binary does, in code (no LLM):**
- **Decomposition**: shard the codebase into areas by file/module + dependency edges; build the area DAG; Kahn topo-sort → audit/execute **waves**; cycle = hard error naming members (port `phase.cjs:cmdPhasePlanIndex`).
- **Heatmap scoring**: deterministic risk = f(LOC, cyclomatic complexity, churn from `git log`, fan-in/fan-out, test coverage gaps). Ranks which areas get audited first and how finely they shard. (Humify's HEATMAP is currently a judgment doc; make the *ranking* mechanical, the *interpretation* agentic.)
- **Fan-out dispatch**: write the `AUDIT_MANIFEST` (`{area_id, agent_id, fragment_path, expected_inputs}`) BEFORE spawning; spawn workers; **barrier**: await all, fail-fast if any returned non-OK.
- **Consolidation merge** (port `gsd-doc-synthesizer` logic to CODE): load all fragments by glob, index by stable key, apply precedence comparator, graph-DFS cycle detection, bucket conflicts (INFO/WARNING/BLOCKER). Emit ONE `AUDIT.md` + machine-parseable `CONFLICTS.md` with fixed-format counts (`### BLOCKERS ({N})`).
- **Gates**: requirement/area-coverage diff (`exit 1` on any uncovered area); post-execute build+test (`TEST_EXIT==0` gates state advance); `verify-completeness` (every fragment id ∈ AUDIT index).
- **Fail-closed merge** of execution worktrees (port `executeWorktreeWaveCleanupPlan`): per-entry gate sequence, `pending[]` on first failure.
- **State/git**: atomic state writes under a planning lock (`flock`/atomic-rename — **enforce single-writer with real locks, not prose**); per-stage commit hashes → `.manifest.json`; `git revert --no-commit` rollback.
- **Bounded convergence loop**: drive plan-check and verify-fix loops in code (max-iters, stall detector via `issue_count` monotonicity, halt-on-regression) — loop state **persisted to disk**, not held in agent memory.
- **Structured I/O**: every command emits `{ok, reason_code, ...}` JSON; `--pick` field extractor; `@file:` spill >N KB.

**Agents do (judgment only):**
- `mapper` (focus-parameterized: tech/arch/quality/concerns) — read code, write per-focus MAP fragment.
- `auditor` (per-area, fan-out N) — read assigned area, write ONE `audit/fragments/{area_id}.json` with findings carrying mandatory severity; return one-line confirmation. **FORCE stance** ("assume every area contains human-hostile code until evidence says otherwise"), tool-scoped read-only+Write-fragment.
- `synthesizer` — *invoked by binary after the merge has already deduped/bucketed mechanically*; the agent's job is narrative summary + judgment calls the comparator flagged as competing-variants, NOT the merge itself.
- `planner` (deep-plan recipe) — read AUDIT slice + pre-computed intel, write implementation-unit PLAN with file paths + characterization-test scenarios.
- `plan-checker` — adversarial, **read-only tool scope**, multi-dimension verify, mandatory `BLOCKER|WARNING` severity.
- `executor` (per-slice, wave-parallel) — refactor in isolated worktree, write+commit SUMMARY *before narration*, HEAD-assertion gate (refuse to commit off a `humify-slice-*` branch).
- `verifier` — adversarial filesystem fact-check + behavior-preservation check, emit `{claims_checked, passed, failed[]}` JSON.

### 4.4 On-disk layout (modeled on `.planning/`)

```
.humify/
  PROJECT.md                  # identity, target codebase, conventions
  HEATMAP.md                  # ranked areas (deterministic scores + agent notes)
  STATE.md                    # <100-line digest CACHE, reconstructable from artifacts
  manifest.json               # per-stage commit hashes (rollback)
  intel/                      # deterministic: deps.json, files.json, apis.json, complexity.json
  areas/
    NN-<area>/
      NN-MAP.md
      NN-AUDIT-fragment.json  # one per auditor (collision-safe: {slug}-{sha256[:8]})
      NN-MM-PLAN.md           # one per slice
      NN-MM-SUMMARY.md        # executor proof-of-work (join key = filename stem)
      NN-VERIFY.json          # verifier output
  AUDIT.md                    # THE consolidated audit (synthesizer output) ← the missing artifact
  CONFLICTS.md                # bucketed, machine-parseable counts
  PATCHLOG.md                 # deterministic roll-up of executed slices
  tmp/
    AUDIT_MANIFEST.json       # transient fan-in source of truth (fail-closed if missing)
    HANDOFF.json              # one-shot resume cursor, deleted after consume
```

**Join key in filenames**: `NN-MM-PLAN.md ↔ NN-MM-SUMMARY.md`. "Is execution complete?" = set-difference over filenames, no DB. **Single-writer for roll-ups** (AUDIT.md, PATCHLOG.md, STATE.md = binary only); **many-writer for disjoint fragments** (each worker owns a uniquely-named file). Every consolidated finding carries `source: {fragment}` for provenance/traceable dedup.

### 4.5 Where the barriers go

```
map:     [fan-out mappers] ─BARRIER→ merge fragments → MAP.md + intel/*.json
heatmap: deterministic scoring (no agents) → HEATMAP.md  [ranks + shards areas]
audit:   write AUDIT_MANIFEST → [fan-out auditors, wave by topo level]
            ─BARRIER (await all, fail-fast)→
            deterministic merge+dedup+cycle-detect+bucket
            → AUDIT.md + CONFLICTS.md
            ─GATE: BLOCKERS>0 ⇒ stop & surface; every area covered? else exit 1
plan:    [planner per slice] → [plan-checker] ─CONVERGENCE LOOP (max 3, stall-detect)→
            ─GATE: deterministic coverage diff (jq -e .passed || exit 1)
execute: [executor fan-out, wave by topo level]
            ─BARRIER: fail-closed worktree merge (pending[] on failure)
            ─GATE: build+test (TEST_EXIT==0) BEFORE next wave
            → [verifier per slice] ─FIX-LOOP (max 2, re-verify all, halt-on-regression)
patchlog: deterministic roll-up (no agents) → PATCHLOG.md
```

Two barriers are load-bearing and are exactly what humify omitted: **the audit barrier+synthesizer** (turns 25 fragments into one AUDIT) and **the execute merge barrier+verify** (turns N parallel refactors into one verified, build-passing tree).

---

## 5. Concrete next steps (ordered build sequence)

1. **Lock the disk contract first.** Implement `.humify/` layout + the derived state machine (`humify status`) — port `roadmap.cjs:241-292` exactly (count-and-classify, no stored status, filename-stem join key). This is the foundation everything resumes off; build it before any agent code. Add `core`-equivalent primitives: `{ok,reason_code}` JSON output, `@file:` spill, atomic-write + real file lock.
2. **Build the deterministic decomposition + heatmap.** Area sharding, dependency-edge extraction, Kahn topo-sort → waves with cycle detection (port `phase.cjs:cmdPhasePlanIndex`), plus the mechanical risk scorer (LOC/complexity/churn/coverage). Output `HEATMAP.md` + `intel/*.json`. No agents yet — testable in isolation against a fixture repo.
3. **Build the consolidation engine in code** (the humify fix). Port `gsd-doc-synthesizer`'s precedence lattice + DFS cycle detection + INFO/WARNING/BLOCKER bucketing to deterministic code; add the manifest-driven fail-closed barrier (`AUDIT_MANIFEST` + `pending[]`, port `executeWorktreeWaveCleanupPlan`) and `verify-completeness` (fragment ids ⊆ AUDIT index). Unit-test on hand-authored fragments *before* wiring agents — this is where the failure lived.
4. **Wire the `audit` stage end-to-end** with the auditor agent (FORCE stance, read-only+fragment-Write, collision-safe filenames) → barrier → step-3 engine → `AUDIT.md`. Run it against `azure_mapper`'s app-core.js and confirm a single AUDIT.md drops, not 25 strays.
5. **Add the convergence loops.** `plan` stage: planner (deep-plan intel-warm recipe) → read-only plan-checker → bounded loop (max 3, stall-detect, **state persisted to disk**) → deterministic coverage gate. Reuse the same loop machinery for the `execute` verify-fix loop (max 2, halt-on-regression).
6. **Build `execute` + `patchlog`.** Wave-parallel executor in worktrees (HEAD-assertion gate, commit-before-narrate), fail-closed merge barrier, build/test gate, then deterministic PATCHLOG roll-up + `manifest.json` commit hashes + `humify undo`.
7. **Resume + handoff.** `HANDOFF.json` (binary-generated, structural — not LLM prose), consume-and-delete; verify `resume` is correct with HANDOFF absent (disk-derived fallback). Add a `--forensic` consistency audit across surfaces.

Build order rationale: stages 1-3 are pure deterministic code with no agent dependency and contain the entire bug class humify hit — get them correct and tested first; agents (4-6) only ever produce per-fragment artifacts and the binary re-derives all truth from disk.

---

## Confidence / provenance notes

- **High confidence (read first-hand):** GSD engine split, derived disk_status state machine (`roadmap.cjs:241-292`), Kahn topo-sort decomposition (`phase.cjs`), fail-closed worktree merge (`worktree-safety.cjs:416-480`), `audit.cjs` consolidate pattern, the classifier→synthesizer fan-out→consolidate→verify pattern (`gsd-doc-*` agents + `ingest-docs.md`/`docs-update.md`), plan-phase convergence loop (`plan-phase.md`, `gsd-plan-checker.md`).
- **Medium confidence:** Deep-Plan's exact step numbering (10 vs 11) and the GSD `gsd-sdk` JSON shapes (`init.resume`, `state-snapshot`) — inferred from consumers, not from the emitting TS/CJS.
- **Known gaps in the source model:** GSD's consolidation handles modest fragment counts (4 mappers, one-per-doc classifiers); it has **no hierarchical/tree merge** for very large fragment sets (humify's 25+). A massive-codebase consolidator likely needs **map-reduce-style staged consolidation** that GSD does not demonstrate — design this fresh. GSD's single-writer discipline is prose-enforced and has failed in practice (data-loss incidents referenced at `execute-phase.md:594-598`); the binary must enforce it with real locks.
