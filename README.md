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
humify verify  [PATH] [--save-baseline|--baseline]  # run detected test/build/lint/typecheck
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

## Sequencing a cleanup session

Humify *ranks* items but does not *sequence* them. To turn the plan into a single next move (do-this-first, the prize, where to stop), see [docs/rank-then-judge.md](docs/rank-then-judge.md) — a reusable prompt pattern, not a command. The sequencing is judgment; humify's verdicts remain the verified facts.

## Baseline-aware verify (for AI editors)

When an agent edits a tree it didn't write, a red `verify` is ambiguous: did the
change break the build, or was the checkout already failing (missing deps, a
pre-existing red test)? Baseline-aware verify removes the guess.

```sh
humify verify --save-baseline   # BEFORE editing: snapshot the pre-edit result
# … the agent edits the source …
humify verify --baseline        # AFTER editing: diff against the snapshot
```

- `--save-baseline` captures the current validation into `.humify/verify-baseline.json`
  and **always exits 0** — an already-red baseline is expected and must not block the
  edit step. It records the HEAD commit and whether the tree was already dirty at save.
- `--baseline` re-runs the checks and classifies each as **newly failing** (a regression
  the change caused), **already failing** (ambient — red before the edit), **fixed**, or
  **indeterminate**. It exits **2 only on a newly-failing check**; ambient failures exit
  0. With no saved baseline it degrades loudly to a plain run.
- It warns when the baseline is **stale** (HEAD moved since the save) or was **saved on a
  dirty tree** (the breakage may already be baked into the baseline).

`verify` only ever reads source and writes under `.humify/`. Add `--json` for the machine
verdict (`newly_failing`, `already_failing`, `fixed`, `indeterminate`, `baseline_stale`,
`baseline_dirty_at_save`).

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
  verify-baseline.json   saved pre-edit snapshot (verify --save-baseline)
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
