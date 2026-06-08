# Humify

A safe, deterministic CLI that **reviews a codebase for AI-generated / AI-degraded code smells, scores its human maintainability, and produces a prioritized refactor plan** — analyzing and planning before it ever touches your source. JSON state under `.humify/` is the control plane; terminal output and markdown are renderings of it.

> Humify analyzes by default and changes source only through `apply`, which is conservative, reversible, and validated.

## Build

```sh
cd humify-ng && go build -o humify .
```

## Commands

```sh
humify analyze [PATH]                     # review the repo → .humify/analysis.json + summary
humify plan    [PATH]                     # rank fixes into HMF-### items → .humify/plan.json
humify verify  [PATH]                     # run detected test/build/lint/typecheck → .humify/validation.json
humify status  [PATH]                     # print current analysis/plan/validation state
humify doctor  [PATH]                     # check wiring, git, stack, and repo readiness
humify apply   --target HMF-### [--dry-run|--yes] [PATH]
```

`PATH` defaults to the current directory. Add `--json` for machine output, `--markdown` (analyze/plan) to also write `HUMIFY_REPORT.md` / `HUMIFY_PLAN.md`.

### What `analyze` looks for

- **Structure:** giant files, long functions, deep nesting (thresholds tunable via `humify.config.json`: `maxFileLines`, `maxFunctionLines`, `maxNestingDepth`).
- **AI-slop signals:** vague names (`data`, `manager`, `helper`, …), generic dumping-ground file names, comments that merely restate the code, leftover `TODO`/`FIXME` markers, and throwaway/empty files.
- **Correctness risk:** swallowed errors (empty `catch`/`if err != nil {}`, `except: pass`) and broad catches (`except Exception`).

It scores five health categories (0–100, higher is healthier): **readability, maintainability, correctness risk, testability, efficiency**. Efficiency is a deliberately conservative complexity proxy — Humify does not guess at runtime performance.

### Recommended workflow

```sh
humify analyze
humify plan
humify apply --target HMF-001 --dry-run
humify apply --target HMF-001 --yes
humify verify
```

### `apply` safety model

`apply` is the only command that changes source, and it is timid by design:

- **Default dry run.** It acts only with `--target HMF-### --yes`.
- **Safe actions only.** Today it performs the one reversible action: **quarantine** — it *moves* (never deletes) confirmed-stale files into `.humify/delete-me/<plan-id>/` and writes a `manifest.json` (original path, new path, reason, plan item, timestamp, validation result). Restore by moving them back.
- **Validated.** It records validation before and after, and **rolls back** if a previously-passing check regresses.
- **Refuses the rest.** Manual/assisted items (e.g. "stop swallowing errors") are explained, never auto-rewritten.

### Generated files (under `.humify/`)

`analysis.json` · `plan.json` · `validation.json` · `delete-me/<plan-id>/{<files>,manifest.json}` · optionally `HUMIFY_REPORT.md` / `HUMIFY_PLAN.md`.

### Safety guarantees

`analyze`, `plan`, `verify`, `status`, and `doctor` never modify target source. Humify writes only under `.humify/`. It never deletes your files. It warns before `apply` if the repo is dirty. It prefers behavior-preserving changes and never hides validation failures.

### Primary-version limitations

- Metrics are heuristic and language-shallow (brace/indent based), not full parsing.
- `apply` automates only the reversible quarantine; other refactors are reported for a human.
- Two `todo_marker` self-matches appear when Humify analyzes its own detector source — a harmless info-level false positive inherent to scanning regex literals.

---

## `humify untangle` — massive-codebase untangler

The original agent-orchestrated workflow is preserved under the `humify untangle <stage>` namespace (`status/heatmap/audit/consolidate/plan/execute/patchlog/undo/resume/verify`). It owns its orchestration loop in deterministic code; the agent is a worker it calls. Design rationale: [`../HUMIFY-NG-ARCHITECTURE.md`](../HUMIFY-NG-ARCHITECTURE.md). Run `humify untangle help` for its usage.

### Untangler stages — full pipeline + resilience surface

**Deterministic core (no agents):**

- **`humify status`** — derives each area's pipeline stage by scanning `.humify/` artifacts. Nothing is stored, so a crash/reset loses no progress (ported from GSD's `roadmap.cjs` cascade).
- **`humify heatmap`** — scans a target codebase, decomposes it into areas (top-level dirs + god-files split out), builds the dependency DAG, partitions areas into dependency-first **waves** via Tarjan SCC + condensation topo-sort (cycles surfaced, not crashed on), scores risk mechanically, and bootstraps `.humify/` (`HEATMAP.md`, area scaffold, `intel/areas.json`, `AUDIT_MANIFEST`).
- **`humify consolidate`** — the fan-in engine, **the stage humify never had**. Gathers every audit fragment named in the manifest into one `AUDIT.md` (content dedup, cross-ref cycle detection, INFO/WARNING/BLOCKER conflict buckets), and **fails closed**: any missing/invalid fragment or unconsolidated area is surfaced as a blocker in `CONFLICTS.md`, exit 2. `AUDIT.md` names only covered areas, so a pending area can never read as audited.
- **`humify patchlog`** — deterministic roll-up of every executed area into `PATCHLOG.md` (flips each to `patched`), recording its merge commit and summary line.
- **`humify undo`** — reverts execute's merge commits newest-first via `git revert` (never `reset`) and clears the commit log.

**Agent stages** — the binary owns the orchestration loop; it writes one prompt per unit of work under `.humify/tmp/`, the agent host spawns the (read-only / worktree-isolated) workers, and the binary re-derives all truth from disk on re-run:

- **`humify audit`** — derives which areas still need an auditor (resumable from disk), then dispatches: writes one auditor prompt per pending area (FORCE stance, read-only, one fragment each). The gather + merge is the `consolidate` stage. With `--runner=spawn` the binary also **runs the agents itself** — see [Autonomous spawn runner](#autonomous-spawn-runner) below.
- **`humify plan`** — advances the per-area plan convergence loop: planner → adversarial **read-only** plan-checker, re-planning with feedback until each finding-bearing area has an accepted `PLAN.md` (bounded by `--max-replans`, default 3, with stall detection).
- **`humify execute`** — advances execution one dependency wave at a time: forks an isolated git worktree+branch per planned slice and dispatches executors, then on re-run runs the fail-closed merge barrier, the `--test-cmd` build/test gate, and the verifiers. Requires a git repo at `--root`.

**Resilience surface** — answers "what next?" and "is this stage really done?" deterministically, so a multi-stage run survives context resets and orchestrator restarts:

- **`humify resume`** — names the next step in the pipeline (advisory: it prints the command to run, never runs it). **Disk is authoritative**: it derives the step from on-disk artifacts and only *enriches* it with the one-shot `HANDOFF.json` cursor a dispatching command left behind. If that cursor still agrees with disk it adds the exact prompts to spawn; if it disagrees — because the agents it dispatched have since advanced the disk — the cursor is flagged stale and disk wins. So `resume` is correct even with `HANDOFF.json` absent or stale, which is the case that matters after a reset.
- **`humify verify [STAGE]`** — re-runs a stage's deterministic gate read-only, without doing its work (`heatmap audit consolidate plan execute patchlog`; omit `STAGE` for the whole pipeline). Exit 2 on any incomplete gate, so CI or a wrapping loop can branch on completeness rather than trust an agent's self-report.

## Build & run

```sh
go build -o humify.exe .
./humify.exe status      [--path=DIR] [--json]
./humify.exe heatmap     --target=DIR [--root=DIR] [--god-loc=N] [--json]
./humify.exe audit       [--root=DIR] [--runner=dispatch|spawn] [--agent-cmd=CMD] [--jobs=N] [--timeout=DUR] [--json]
./humify.exe consolidate [--root=DIR] [--json]
./humify.exe plan        [--root=DIR] [--max-replans=N] [--json]
./humify.exe execute     [--root=DIR] [--test-cmd=CMD] [--json]
./humify.exe patchlog    [--root=DIR] [--json]
./humify.exe undo        [--root=DIR] [--json]
./humify.exe resume      [--path=DIR] [--root=DIR] [--json]
./humify.exe verify      [STAGE] [--path=DIR] [--root=DIR] [--json]
go test ./...
```

Verified on a real codebase: heatmap decomposes 100+ files into areas/waves with god-files surfaced as top risk; the full loop (author fragments → consolidate → status) flips `audit-incomplete` → `audited` for covered areas while holding the rest `pending`. The consolidation engine was hardened against 7 bugs found by an adversarial review panel (a fail-open id-leak, a cycle-detection corruption, manifest-duplicate fail-closed bypasses, and a false-positive depth cap), each now covered by a regression test.

## Autonomous spawn runner

Every agent stage is autonomous in its *orchestration* but, by default, not its *spawning*: it writes prompts under `.humify/tmp/` and hands back to an external host. `audit --runner=spawn` closes that gap for the audit stage — the binary writes the prompts, runs an operator-supplied agent once per pending area (capped concurrency, a per-agent timeout), waits on all of them (the barrier), then re-derives which fragments actually appeared.

The agent is told (in the prompt, delivered on **stdin**) exactly which fragment file to write. A trivial "agent" that fabricates a valid empty-findings fragment proves the spawn→barrier→verify loop with no LLM/network — this is the real end-to-end test:

```sh
# writer.sh — a stand-in auditor: read the prompt, write the fragment it names
prompt=$(cat)
f=$(printf '%s' "$prompt" | grep -oE '\.humify/areas/[^`]*-AUDIT-fragment\.json' | head -1)
a=$(basename "$(dirname "$f")")
mkdir -p "$(dirname "$f")"
printf '{"area_id":"%s","findings":[]}' "$a" > "$f"
```

```sh
./humify.exe audit --runner=spawn --agent-cmd="sh writer.sh" --jobs=4 --root=DIR
./humify.exe resume --root=DIR          # → advances to: humify consolidate
```

- **`--agent-cmd=CMD`** (required) — the prompt is piped on **stdin**, never interpolated into the command line, so a crafted prompt cannot inject shell. The command is operator-supplied and trusted, the same trust model as execute's `--test-cmd`; do not wire it to a value read out of the target repo without sandboxing.
- **`--jobs=N`** (default 4) — max agents in flight at once. Auditors are read-only and independent, so they may overlap, but not unboundedly.
- **`--timeout=DUR`** (default 10m, e.g. `90s`/`10m`/`1h`) — per-agent wall-clock cap. An LLM agent's signature failure is to *hang*, not exit, so an unbounded wait would freeze the whole stage; a timed-out agent is killed and its area lands in `failed`.
- **Fail-closed:** after the barrier the binary re-validates each fragment on disk (parse + mandatory-severity contract + area-id match). Any area whose agent ran but left no valid fragment — including a timed-out hang — is reported in `failed` and the stage **exits 2**, the same drift signal as a missing fragment, never a silent pass. Re-running `audit` retries only the stragglers (already-valid areas are skipped).

> `--agent-cmd='claude -p'` is an **illustrative** example, not a verified one: confirm against your CLI whether it reads the prompt from stdin and whether a headless auditor needs a permission/allowed-tools flag to write its fragment — without one it may block waiting for input and hit `--timeout`.

## The derived state cascade

Per area under `.humify/areas/NN-<slug>/`, highest reached stage wins:

| Status | Condition |
|---|---|
| `patched` | area id appears in `.humify/PATCHLOG.md` |
| `executed` | `plans > 0` and `summaries >= plans` |
| `planned` | `*-PLAN.md` present |
| `audited` | audit fragment present **and** area id covered by `.humify/AUDIT.md` |
| `audit-incomplete` | audit fragment present but **not** in `AUDIT.md` |
| `mapped` | `*-MAP.md` present |
| `empty` / `no_directory` | otherwise |

`audit-incomplete` is the key state GSD doesn't model: fragments exist on disk but the consolidated `AUDIT.md` never gathered them — the exact failure that stranded 25 fragments on the `azure_mapper` run.

## Exit codes

`0` clean · `1` not a humify project · `2` drift (an area is `audit-incomplete`).

Exit 2 makes the drift machine-detectable: CI or a wrapping loop can branch on it and auto-resume into the (future) `audit` consolidation stage.
