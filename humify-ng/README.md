# humify-ng

A massive-codebase untangler that **owns its orchestration loop in deterministic code**. The agent is a worker the binary calls — not the orchestrator — so the fan-out→gather→verify discipline can't drift. Design rationale and full roadmap: [`../HUMIFY-NG-ARCHITECTURE.md`](../HUMIFY-NG-ARCHITECTURE.md).

## Status: stages 1–6 + undo (full pipeline)

**Deterministic core (no agents):**

- **`humify status`** — derives each area's pipeline stage by scanning `.humify/` artifacts. Nothing is stored, so a crash/reset loses no progress (ported from GSD's `roadmap.cjs` cascade).
- **`humify heatmap`** — scans a target codebase, decomposes it into areas (top-level dirs + god-files split out), builds the dependency DAG, partitions areas into dependency-first **waves** via Tarjan SCC + condensation topo-sort (cycles surfaced, not crashed on), scores risk mechanically, and bootstraps `.humify/` (`HEATMAP.md`, area scaffold, `intel/areas.json`, `AUDIT_MANIFEST`).
- **`humify consolidate`** — the fan-in engine, **the stage humify never had**. Gathers every audit fragment named in the manifest into one `AUDIT.md` (content dedup, cross-ref cycle detection, INFO/WARNING/BLOCKER conflict buckets), and **fails closed**: any missing/invalid fragment or unconsolidated area is surfaced as a blocker in `CONFLICTS.md`, exit 2. `AUDIT.md` names only covered areas, so a pending area can never read as audited.
- **`humify patchlog`** — deterministic roll-up of every executed area into `PATCHLOG.md` (flips each to `patched`), recording its merge commit and summary line.
- **`humify undo`** — reverts execute's merge commits newest-first via `git revert` (never `reset`) and clears the commit log.

**Agent stages** — the binary owns the orchestration loop; it writes one prompt per unit of work under `.humify/tmp/`, the agent host spawns the (read-only / worktree-isolated) workers, and the binary re-derives all truth from disk on re-run:

- **`humify audit`** — derives which areas still need an auditor (resumable from disk), then dispatches: writes one auditor prompt per pending area (FORCE stance, read-only, one fragment each). The gather + merge is the `consolidate` stage.
- **`humify plan`** — advances the per-area plan convergence loop: planner → adversarial **read-only** plan-checker, re-planning with feedback until each finding-bearing area has an accepted `PLAN.md` (bounded by `--max-replans`, default 3, with stall detection).
- **`humify execute`** — advances execution one dependency wave at a time: forks an isolated git worktree+branch per planned slice and dispatches executors, then on re-run runs the fail-closed merge barrier, the `--test-cmd` build/test gate, and the verifiers. Requires a git repo at `--root`.

## Build & run

```sh
go build -o humify.exe .
./humify.exe status      [--path=DIR] [--json]
./humify.exe heatmap     --target=DIR [--root=DIR] [--god-loc=N] [--json]
./humify.exe audit       [--root=DIR] [--runner=dispatch] [--json]
./humify.exe consolidate [--root=DIR] [--json]
./humify.exe plan        [--root=DIR] [--max-replans=N] [--json]
./humify.exe execute     [--root=DIR] [--test-cmd=CMD] [--json]
./humify.exe patchlog    [--root=DIR] [--json]
./humify.exe undo        [--root=DIR] [--json]
go test ./...
```

Verified on a real codebase: heatmap decomposes 100+ files into areas/waves with god-files surfaced as top risk; the full loop (author fragments → consolidate → status) flips `audit-incomplete` → `audited` for covered areas while holding the rest `pending`. The consolidation engine was hardened against 7 bugs found by an adversarial review panel (a fail-open id-leak, a cycle-detection corruption, manifest-duplicate fail-closed bypasses, and a false-positive depth cap), each now covered by a regression test.

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
