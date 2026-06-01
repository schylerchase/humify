# Humify

Humify is a readability-first framework for reviewing, unwinding, and refactoring messy codebases. It helps an AI coding agent and a human reviewer find the code that is hard to trust, change, or test, then fix it without losing behavior.

The goal is not prettier code. The goal is to restore human confidence in a codebase: code that is boring to navigate, easy to name, and hard to break by accident.

Humify runs as a skill for OpenAI Codex or Anthropic Claude. You point your agent at this repo and say "Run Humify on this repo." It starts read-only and edits code only after you open the refactor gate.

## What Humify does

- Maps a repository before judging it, so you know what was reviewed and what was skipped.
- Separates ordinary mess from high-risk areas, so cleanup effort goes where it matters.
- Flags machine-shaped maintainability signals with file and line evidence. It does not claim code was AI-generated.
- Scores readability and refactor risk, then turns low scores into safe, tests-first refactor slices.
- Reviews large codebases through maps and heatmaps instead of one impossible pass.
- Uses a stellar reference codebase as positive guidance, not just a list of problems.

## What you get

Humify produces a small set of evidence-backed artifacts. Each one starts from a template in `shared/templates/`.

- `HUMIFY-MAP.md`: the file and ownership map. Languages, entry points, tests, and the generated or vendored paths that were excluded.
- `HUMIFY-HEATMAP.md`: scored areas with a confidence column and honest coverage limits.
- `HUMIFY-AUDIT.md`: findings with file and line evidence, a classification summary, cleared false positives, and a Refactor Readiness Verdict.
- `HUMIFY-PLAN.md`: tests-first refactor slices, produced when a score is low or risk is high.
- `HUMIFY-PATCHLOG.md`: completed slices, the tests that protected them, and the residual risk.

## Install

There is no package to install. Clone the whole repo and point your agent at the adapter for your platform. Both adapters reference the shared core through relative paths, so keep the repo intact.

```bash
git clone https://github.com/schylerchase/humify.git
```

### Codex

The Codex adapter is a plugin defined by `codex/.codex-plugin/plugin.json`, with the skill at `codex/skills/humify/SKILL.md`. The agent interface lives in `codex/skills/humify/agents/openai.yaml`, which allows implicit invocation, so Codex can run Humify without an explicit token.

Point Codex at the `codex/` directory and load the plugin.

### Claude

The Claude adapter is a plugin defined by `claude/.claude-plugin/plugin.json`, with the skill at `claude/skills/humify/SKILL.md`. Claude auto-discovers the skill from that path, so there is no skills field to configure.

Point Claude or Claude Code at the `claude/` directory.

## Use it

Once the skill is loaded, the prompt is:

```text
Run Humify on this repo.
```

To go straight to planning:

```text
Use Humify to create a refactor plan.
```

Humify captures repo state first: git root, branch, and any dirty, staged, untracked, or ignored files. It then builds the map and heatmap, audits the hotspots with evidence, and runs a steelman pass over its own findings. It moves into a no-commit refactor slice only after you opt in and the refactor gate is open.

Warning: if the repo is dirty, Humify stays in audit and plan mode. It will not stash, revert, clean, or commit your work to open the gate. Give it a clean worktree, or tell it to accept the current tree as the baseline.

## How it works

For a normal repo, the audit runs as an ordered set of passes. It excludes generated and vendored code, checks local conventions and domain language, reviews function design and canonical contracts, looks for machine-shaped signals, separates those signals from real refactor risk, selects findings that carry evidence, steelmans them, and checks public readiness.

For a repo too large to read in one pass, Humify uses an eight-stage massive-codebase workflow:

1. Target state and dirty-repo protocol.
2. Repository map.
3. Exclusion map for generated, vendored, and build output.
4. Stratified sampling.
5. Heatmap scoring across nine categories on a 0 to 3 scale.
6. Deep dives on the hotspots.
7. Refactor plan when scores cross the low-score trigger.
8. Coverage steelman before any whole-repo conclusion.

### Classifications

Every reviewed file gets exactly one label. The precedence order, highest first:

1. Excluded generated file. Only with generation evidence.
2. High-risk refactor candidate.
3. Machine-shaped readability risk.
4. Needs targeted cleanup.
5. Clean.

Machine-shaped confidence (None, Low, Medium, High, or Not applicable) is judged separately from refactor risk. A file can be messy and low-risk, or clean-looking and high-risk. Humify does not collapse the two.

### Risk escalation

A file becomes a High-risk refactor candidate only when a refactor could silently change behavior that is irreversible or safety-critical, and that behavior is not protected by tests. That covers money, permissions, deletion, medical or other high-trust data, infrastructure state, data migrations, retry and concurrency logic, and contract drift across surfaces.

Plain reads, imports, and CRUD that simply lack tests are Needs targeted cleanup, not High-risk. A stable compatibility wrapper or an ugly-but-working file with no demonstrated defect is Clean. Age and ugliness are not findings.

## Tests-first planning

A low audit score means slow down and slice deliberately. It does not mean rewrite.

Every plan starts with a characterization harness. The first unit captures current behavior with tests or golden output. Code movement comes after. Each later unit names the exact files, the findings it addresses, its tests, its verification, and how to roll it back on its own.

A plan is not executable until it passes seven gates: behavior captured, scope mapped to findings, risky behavior tested before movement, each unit independently revertible, public API changes called out, unknown areas listed, and generated code excluded or justified.

## Safety model

- Read-only audit is the default.
- Refactor mode requires explicit opt-in.
- A dirty repo blocks source edits until you accept the baseline or use a clean worktree.
- Generated, vendored, and build artifacts are excluded before scoring.
- Risky behavior needs characterization tests or golden-output capture before any change.
- Humify never commits unless you ask.

## Privacy model

Humify does not publish private run details. Repo names, absolute paths, remotes, customer identifiers, cloud accounts, and exact private findings stay in local run folders.

- `.humify-runs/`, `private/`, `actual/`, and `actual-plans/` are git-ignored.
- `.claude/` is ignored in-repo, so the privacy guarantee does not depend on a contributor's global ignore file.
- Public docs use synthetic or sanitized examples.

Turn useful lessons from private repos into synthetic case studies in `shared/EXAMPLES.md` instead of committing raw findings.

## When not to refactor

Do not run a refactor slice when:

- the repo is dirty and you have not accepted the baseline,
- tests or golden-output checks do not protect the risky behavior,
- generated files could be regenerated from another source,
- the slice mixes unrelated workflows,
- the finding has no file or line evidence,
- the only evidence is "this looks messy."

## Calibration and evaluators

Humify ships with a calibration pack so you can measure whether the framework scores the way it should. `shared/fixtures/` holds the input code. `shared/expected/` holds the gold audit outputs. `shared/expected-plans/` holds the gold plans.

Two evaluators score outputs against those baselines and share one rubric, stored in `shared/tools/humify-fixtures.json` and `humify-plan-fixtures.json`.

Audit scoring is out of 51 (17 fixtures, 3 points each). Plan scoring is out of 33 (11 plans, 3 points each). The readiness threshold is 86 percent: 44 of 51 for audits, 29 of 33 for plans.

Audit readiness bands:

- 86 to 100 percent: ready to use on a real repo.
- 67 to 85 percent: usable with human review.
- 40 to 66 percent: needs framework tuning.
- 0 to 39 percent: not reliable yet.

Plan bands use the same percentages with plan-specific labels, starting at "plan calibration ready."

Run the Python evaluators. They use Python 3 and the standard library only:

```bash
python3 shared/tools/humify_evaluate.py --actual-dir <your-audit-outputs> --expected-dir shared/expected
python3 shared/tools/humify_evaluate_plans.py --actual-plan-dir <your-plan-outputs> --expected-plan-dir shared/expected-plans
```

Add `--fail-below-threshold` to exit non-zero when the score falls under the bar.

PowerShell twins exist for hosts that prefer them:

```powershell
pwsh -NoProfile -File shared/tools/humify-evaluate.ps1 -ActualDir <your-audit-outputs> -ExpectedDir shared/expected
```

One caution about self-tests. Scoring a directory against itself, where actual equals expected, is forced to the maximum and only proves the evaluator is wired correctly. The report labels it that way. The honest metric is a blind run: generate the outputs without reading the expected files, then score them. The self-test goes further and feeds deliberately wrong output to confirm the evaluator can actually fail:

```bash
python3 shared/tools/humify_selftest.py
```

A passing self-test reports that identity wiring holds and that the evaluator fails on bad input.

## Repository layout

```text
shared/   platform-agnostic core, about 90 percent of the content: docs, templates, prompts, fixtures, expected baselines, tools
codex/    Codex adapter: .codex-plugin/plugin.json, skills/humify/SKILL.md, skills/humify/agents/openai.yaml
claude/   Claude adapter: .claude-plugin/plugin.json, skills/humify/SKILL.md
actual/   stub directory for your audit outputs before scoring (contents git-ignored)
```

Key documents in `shared/`:

```text
HUMIFY.md                     the framework standard
HUMIFY-VISION.md              product vision and the main rules
HUMIFY-OPERATOR.md            the operator workflow
HUMIFY-AI-INSTRUCTIONS.md     the full model judgment rubric
MASSIVE-CODEBASE-WORKFLOW.md  the large-repo protocol
REFACTOR-PLAN-PROTOCOL.md     the tests-first planning protocol
STEELMAN-PASS.md              the adversarial check on findings and plans
MODEL-CONTEXT-PACKET.md       the order to assemble model context
HUMIFY-TESTING.md             the calibration workflow
EXAMPLES.md                   concrete review situations
STELLAR-CODEBASES.md          what good code looks like
```

Prompt entry points:

```text
shared/prompts/humify-audit.md             normal codebase audit
shared/prompts/humify-plan.md              refactor plan from an audit
shared/prompts/humify-calibration.md       fixture calibration
shared/prompts/humify-massive-codebase.md  massive-codebase workflow
```

## Example outputs

- Example audit: `shared/expected/registry-drift-audit.md`
- Example refactor plan: `shared/expected-plans/registry-drift-plan.md`
- Positive reference codebase: `shared/examples/stellar-codebase/`

## Steelman every audit

Before you trust an audit or a plan, run it through `shared/STEELMAN-PASS.md`. The pass checks evidence strength, false-positive risk, coverage gaps, safety guardrails, and whether the first refactor slice is small enough to execute. If a finding cannot survive that check, it does not ship.
