# Humify

A safe, deterministic CLI that **reviews a codebase for AI-generated / AI-degraded code smells, scores its human maintainability, and produces a prioritized refactor plan** — analyzing and planning before it ever touches your source.

> Humify analyzes by default and changes source only through `apply`, which is conservative, reversible, and validated.

## Install

```sh
go install humify@latest
```

Or build from source:

```sh
git clone https://github.com/schylerchase/humify
cd humify
go build -o humify .
```

## Commands

```sh
humify analyze [PATH]                              # review the repo → .humify/analysis.json
humify plan    [PATH]                              # rank fixes into HMF-### items → .humify/plan.json
humify verify  [PATH]                              # run detected test/build/lint/typecheck
humify status  [PATH]                              # print current analysis/plan/validation state
humify doctor  [PATH]                              # check wiring, git, stack, and repo readiness
humify apply   --target HMF-### [--dry-run|--yes] [PATH]
```

`PATH` defaults to the current directory. Add `--json` for machine output. Add `--markdown` (analyze/plan) to also write `HUMIFY_REPORT.md` / `HUMIFY_PLAN.md`.

## Recommended workflow

```sh
humify analyze
humify plan
humify apply --target HMF-001 --dry-run
humify apply --target HMF-001 --yes
humify verify
```

## What `analyze` looks for

- **Structure:** giant files, long functions, deep nesting (thresholds tunable via `humify.config.json`: `maxFileLines`, `maxFunctionLines`, `maxNestingDepth`).
- **AI-slop signals:** vague names (`data`, `manager`, `helper`), generic file names, comments that restate the code, leftover `TODO`/`FIXME` markers, and throwaway/empty files.
- **Correctness risk:** swallowed errors (empty `catch`/`if err != nil {}`, `except: pass`) and broad catches (`except Exception`).

Scores five health categories (0–100): **readability, maintainability, correctness risk, testability, efficiency**.

## `apply` safety model

`apply` is the only command that changes source, and it is deliberately timid:

- **Default dry run.** Acts only with `--target HMF-### --yes`.
- **Three automation tiers:**
  - `safe` — apply executes automatically (quarantine stale files, no source edits).
  - `assisted` — humify can help, but a human must review/confirm each edit.
  - `manual` — humify refuses to touch it. Broad exception narrowing, splitting giant files, breaking up long functions all require understanding intent. You do it by hand, then run `humify verify`.
- **Quarantine, not delete.** Safe items are moved (never deleted) into `.humify/delete-me/<plan-id>/` with a `manifest.json`. Restore by moving them back.
- **Validated.** Records validation before and after, and rolls back if a passing check regresses.

## Generated files

```
.humify/
  analysis.json          findings + health scores
  plan.json              ranked HMF-### refactor items
  validation.json        test/build/lint results
  delete-me/<id>/        quarantined files (safe apply only)
  HUMIFY_REPORT.md       optional markdown summary (--markdown)
  HUMIFY_PLAN.md         optional markdown plan (--markdown)
```

## Large codebase workflow

For codebases too large to analyze in one pass, use the untangle pipeline:

```sh
humify untangle heatmap     --target=DIR
humify untangle audit       [--runner=dispatch|spawn] [--agent-cmd=CMD]
humify untangle consolidate
humify untangle plan
humify untangle execute
humify untangle run         --agent-cmd=CMD [--execute]
```

`run` is the autonomous driver: it walks each area through audit → consolidate → plan and optionally through execute, spawning `--agent-cmd` at each agent stage. Pass `--execute` to allow source-modifying stages.

## Safety guarantees

- `analyze`, `plan`, `verify`, `status`, and `doctor` never modify target source.
- Humify writes only under `.humify/`. It never deletes your files.
- `apply` quarantines (never deletes) and is reversible.
- It warns before `apply` if the repo is dirty.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | ok |
| 1 | error / not a humify project |
| 2 | verify failed or apply rolled back |
