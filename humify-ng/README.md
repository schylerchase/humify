# humify-ng

A massive-codebase untangler that **owns its orchestration loop in deterministic code**. The agent is a worker the binary calls — not the orchestrator — so the fan-out→gather→verify discipline can't drift. Design rationale and full roadmap: [`../HUMIFY-NG-ARCHITECTURE.md`](../HUMIFY-NG-ARCHITECTURE.md).

## Status: stage 1 — `status` (disk contract)

Implemented: the `.humify/` on-disk contract and `humify status`, which **derives** each area's pipeline stage by scanning artifacts. Nothing is stored, so a crash/reset loses no progress (ported from GSD's `roadmap.cjs` cascade).

Not yet built: `map`, `heatmap`, `audit`, `plan`, `execute`, `patchlog` (stages 2–7).

## Build & run

```sh
go build -o humify.exe .
./humify.exe status [--path=DIR] [--json]
go test ./...
```

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
