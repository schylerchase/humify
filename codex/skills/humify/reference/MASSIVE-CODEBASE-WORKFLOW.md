# Humify Massive Codebase Workflow

Use this workflow when the codebase is too large to review in one pass.

The goal is to produce a reliable refactor plan without pretending the model can hold the whole repository in context at once.

## Output Artifacts

A massive-codebase Humify run should produce:

```text
HUMIFY-MAP.md        repository inventory, ownership, hotspots, exclusions
HUMIFY-HEATMAP.md    scored areas and risk clusters
HUMIFY-AUDIT.md      evidence-backed findings for selected hotspots
HUMIFY-PLAN.md       Deep Plan-style refactor slices
HUMIFY-PATCHLOG.md   completed slices, tests, verification, residual risk
```

Start from these templates:

```text
templates/HUMIFY-MAP.template.md
templates/HUMIFY-HEATMAP.template.md
templates/HUMIFY-AUDIT.template.md
templates/HUMIFY-PLAN.template.md
templates/HUMIFY-PATCHLOG.template.md
```

## Stage 0: Target State And Dirty Repo Protocol

Before inventory, capture the target shape and working state.

Record:

- Git root or roots.
- Current branch.
- Dirty and untracked files.
- Whether the path is one repo, a monorepo, or a parent folder containing independent repos.

Rules:

- A dirty repo may be audited read-only.
- Do not refactor a dirty repo until the active changes are intentionally preserved.
- Do not revert, stash, or clean user changes as part of Humify unless explicitly requested.
- If the target contains multiple independent repos, produce separate repo-state notes and avoid pretending it is one codebase.

Dirty state is a planning constraint, not a finding against the code.

## Coverage Principle

Massive-codebase reviews must be honest about coverage.

Do not imply the entire repository was deeply reviewed unless it was. Distinguish:

- **Inventory coverage:** files discovered and categorized.
- **Sample coverage:** representative files read.
- **Deep-dive coverage:** hotspots reviewed with file/line findings.
- **Unknowns:** areas not inspected deeply enough to score confidently.

Every massive-codebase output should include a coverage statement.

## Stage 1: Repository Inventory

Build a file and ownership map before judging quality.

Collect:

- Languages and frameworks.
- Entry points.
- Test directories.
- Generated, vendored, build, migration, and lockfile paths.
- Large files.
- High-churn files if git history is available.
- Dependency boundaries.
- Public APIs.
- CLI commands, app routes, background jobs, or scheduled tasks.

Write findings to `HUMIFY-MAP.md`.

### Inventory Row Template

```markdown
| Area | Paths | Role | Generated/excluded? | Tests found? | Notes |
| --- | --- | --- | --- | --- | --- |
| Customer import | `src/import/**` | import workflow | no | `tests/import/**` | high churn, handles external data |
```

### Inventory Questions

- What are the primary user-facing workflows?
- Where do requests enter the system?
- Where are side effects performed?
- Where does domain logic appear to live?
- Which files are not meant to be edited by humans?
- Which areas lack tests?
- Which areas are risky because of money, permissions, data deletion, imports, exports, or infrastructure?

## Stage 2: Exclusion Map

Before scoring, exclude files that should not receive Humify findings.

Exclude by default:

- `node_modules/`, `vendor/`, `dist/`, `build/`, coverage output.
- Lockfiles.
- Minified bundles.
- Generated clients.
- Generated migrations or snapshots.
- Compiled output.
- Files with generated headers.

Document the exclusion pattern and one example path in `HUMIFY-MAP.md`.

Do not hide excluded paths entirely. Humans need to know what was skipped.

## Stage 3: Stratified Sampling

Do not sample randomly only. Review representative files from each risk stratum.

Recommended strata:

| Stratum | Examples | Why it matters |
| --- | --- | --- |
| Entrypoints | routes, commands, handlers, jobs | Shows orchestration and boundary hygiene. |
| Domain logic | pricing, permissions, validation, state transitions | Highest behavior-change risk. |
| Infrastructure | database, API clients, filesystem, queues | Side effects and error behavior live here. |
| UI workflows | screens, components, state hooks | Reveals user-facing edge states. |
| Tests | unit, integration, e2e, fixtures | Shows whether refactors are protected. |
| Large files | top 10 by line count | Likely mixed responsibilities. |
| High-churn files | top 10 by commit frequency | Pain points and active risk. |
| Low-convention files | naming/layout outliers | Likely drift or machine-shaped patches. |
| Contract registries | input IDs, file maps, export/import names, snapshot keys | Finds silent workflow loss when surfaces drift. |
| Active-source authority | generated output, extracted modules, loaded scripts | Prevents reviewing a stale module as if it were the runtime source. |

For each stratum, pick:

- 2-5 representative files for small repos.
- 5-10 representative files for medium repos.
- 10-20 representative files for large repos.

If token budget is tight, prefer entrypoints, domain logic, high-churn files, and tests.

For apps with import/export, upload, snapshot, report, or generated-download workflows, always sample the contract registry path. These workflows often fail by omission: a resource exists in one surface but is absent from the file map, diff context, report builder, or active UI.

### Sampling Ledger

Record sampled and unsampled areas:

```markdown
## Sampling Ledger

Sampled:
- `src/import/customers.ts` — high-churn import workflow
- `tests/import/customers.test.ts` — behavior protection check

Not deeply sampled:
- `src/reports/**` — lower operational risk in this pass
- `src/legacy/**` — requires separate legacy review
```

## Stage 4: Heatmap Scoring

Score areas, not only files.

Use this score:

| Score | Meaning |
| --- | --- |
| 0 | Clear and locally understandable. |
| 1 | Minor friction, safe to maintain. |
| 2 | Meaningful maintainability risk. |
| 3 | High-risk or human-hostile code. |

Score each area:

- Readability
- Structure
- Function design
- Domain language
- Boundary hygiene
- Error behavior
- Testability
- Machine-shaped signal strength
- Refactor risk

Keep machine-shaped signal strength separate from refactor risk. A large, human-shaped workflow can have low machine-shaped confidence and high refactor risk. A tiny generic helper can have machine-shaped signals and low behavior risk. Generated code can look machine-shaped and still be excluded.

Write to `HUMIFY-HEATMAP.md`.

Use two score columns:

- **Observed score:** based on files actually read.
- **Confidence:** high, medium, or low based on sample quality.

Never give a high-confidence area score from filename inspection alone.

### Heatmap Row Template

```markdown
| Area | Representative files | Observed score | Confidence | Primary risk | Evidence | Next action |
| --- | --- | ---: | --- | --- | --- | --- |
| Customer import | `src/import/customers.ts`, `tests/import.test.ts` | 18 | High | Mixed validation/persistence | validation and writes in same function | Add characterization tests, then extract validation |
```

## Stage 5: Hotspot Deep Dives

Deep dive only the highest-value areas.

Choose hotspots when:

- Total area score is 12 or higher.
- The area is high-churn.
- The area handles money, permissions, deletion, imports, exports, or infrastructure.
- The area blocks other refactors.
- The area has high machine-shaped confidence and weak tests.

For each hotspot, produce normal Humify findings in `HUMIFY-AUDIT.md`.

Cap findings:

- Small repo: 5-10 findings.
- Medium repo: 10-20 findings.
- Large repo: 20-40 findings.

If there are more, cluster them instead of listing every instance.

### Hotspot Packet Template

```markdown
## Hotspot: <workflow or boundary>

Why selected:
- <score, churn, risk, user workflow, or dependency reason>

Files read:
- <path>

Findings:
- H001
- H002

Steelman check:
- Strongest evidence: <file/line or convention proof>
- Main uncertainty: <unreviewed area or inferred behavior>
- Safety guardrail: <test or rollback>
```

## Stage 6: Low-Score Trigger

If any area scores 18+ or the total repository score is below the readiness threshold, do not start refactoring directly.

Instead:

1. Produce `HUMIFY-PLAN.md`.
2. Require characterization tests for risky areas.
3. Split the work into implementation units.
4. Identify rollback paths.
5. Verify one slice at a time.

Low score means "plan first," not "rewrite everything."

## Stage 6.5: Coverage Steelman

Before writing a repository-level conclusion, run `STEELMAN-PASS.md`.

The conclusion must say:

- what was inventoried,
- what was sampled,
- what was deeply reviewed,
- what remains unknown,
- which claims are observed versus inferred,
- what first safe slice should happen next.

If the unknowns are too large, narrow the conclusion to the reviewed hotspots.

## Stage 7: Refactor Plan

Group findings into implementation units.

Good unit grouping:

- By workflow.
- By boundary.
- By behavior risk.
- By dependency order.

Bad unit grouping:

- By smell type across the whole repo.
- By "clean all files."
- By a mechanical rename across unrelated areas.
- By deleting code before proving it is unused.

## Massive Repo Prompt

Use this prompt shape:

```markdown
You are running Humify on a massive codebase.

Read:
- `HUMIFY.md`
- `HUMIFY-AI-INSTRUCTIONS.md`
- `EXAMPLES.md`
- `MASSIVE-CODEBASE-WORKFLOW.md`
- `REFACTOR-PLAN-PROTOCOL.md`

Do not attempt to review every file deeply.

First produce:
1. Repository inventory
2. Exclusion map
3. Sampling plan
4. Heatmap

Then deep-dive only the highest-risk hotspots and produce:
1. `HUMIFY-AUDIT.md`
2. `HUMIFY-PLAN.md` if scores are low or refactor risk is high

Rules:
- Every finding needs file/line evidence.
- Generated/vendor/build outputs must be excluded.
- Low score triggers planning, not direct refactor.
- Risky behavior requires characterization tests before movement.
- Cluster repeated issues instead of spamming duplicate findings.
```

## Stop Conditions

Stop and ask for user input when:

- A hotspot is business-critical and behavior is ambiguous.
- Existing tests contradict apparent behavior.
- Generated code appears hand-edited.
- The safest first step requires production data, credentials, or destructive actions.
- The requested scope would mix unrelated workflows into one refactor.

## Completion Criteria

A massive-codebase Humify review is complete when:

- Exclusions are documented.
- Sampling is justified.
- Coverage gaps are explicit.
- Hotspots are scored.
- Findings cite file and line evidence.
- Low-scoring areas are converted into implementation units.
- First refactor slice is safe, testable, and reversible.

## Resume Protocol

For very large repos, Humify can run over multiple sessions.

At the end of each session, append:

```markdown
## Resume State

Reviewed:
- <areas>

Open hotspots:
- <areas>

Do not re-review:
- <areas already cleared>

Next pass should inspect:
- <specific paths and why>
```

This prevents the model from starting over and helps later passes compound evidence.
