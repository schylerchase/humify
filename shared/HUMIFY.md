# Humify Codebase Framework

Humify is a readability-first framework for reviewing, unwinding, and refactoring messy codebases. Its purpose is not to shame a codebase for being machine generated. Its purpose is to identify code that is hard for humans to trust, explain why, and turn that mess into safe refactoring slices.

The working loop is:

1. **Map** the current codebase.
2. **Flag** machine-shaped or human-hostile code with evidence.
3. **Judge** ambiguous cases with the AI instruction layer in `HUMIFY-AI-INSTRUCTIONS.md`.
4. **Compare** against concrete examples and stellar-codebase traits.
5. **Steelman** the claims, risks, coverage, and first safe move.
6. **Plan** safe slices using Deep Plan-style implementation units.
7. **Refactor** in behavior-preserving steps.
8. **Verify** with tests, runtime checks, and a final readability pass.

## Core Standard

Humified code should be boring to navigate, easy to name, and hard to accidentally break.

The standard:

- **Domain language first:** files, modules, functions, and tests use names from the problem domain, not vague implementation words.
- **Explicit boundaries:** side effects live at the edges; core logic is pure or close to pure.
- **Small stable units:** functions do one job, modules own one concept, and entrypoints orchestrate instead of implementing details.
- **Readable control flow:** code reads top-to-bottom with guard clauses, clear branches, and no hidden dependency chains.
- **Local reasoning:** a reader can understand a function without opening five unrelated files.
- **Observable behavior protected:** refactors start with characterization tests or equivalent before logic is moved.
- **No decorative abstraction:** abstractions exist because they reduce real duplication, encode a useful boundary, or clarify a domain concept.
- **Failure is designed:** error paths, empty inputs, retries, cancellation, logging, and permissions are intentional.

## Codebase Shape

Humify does not force one folder structure for every stack. It requires a consistent structure where each layer has a job.

Recommended default shape:

```text
src/
  app/              entrypoints, routing, composition, dependency wiring
  domain/           pure domain concepts, rules, value objects
  features/         user-facing capabilities or workflows
  infrastructure/   databases, APIs, filesystem, queues, platform adapters
  shared/           stable cross-cutting utilities only
  ui/               components, screens, design-system composition
  tests/            integration, fixtures, characterization tests
```

If the existing codebase already has a good convention, keep it. Humify prefers making the current structure coherent over imposing a new layout.

### Structural Rules

- Entrypoints are thin. They parse input, call a workflow, and return output.
- Domain code does not import infrastructure code.
- Infrastructure adapts the outside world into domain-friendly shapes.
- Shared utilities are treated as a last resort, not a dumping ground.
- Feature modules may depend inward on domain and outward through explicit adapters.
- Tests are organized around behavior, not private implementation trivia.
- Generated files, vendored code, build outputs, and migration snapshots are marked clearly and excluded from readability scoring unless they are edited by hand.

## Function Writing Standard

A humified function should answer five questions quickly:

1. What does it do?
2. What does it need?
3. What can it return or throw?
4. What side effects can it cause?
5. What behavior proves it works?

### Function Checklist

- Name describes the domain action or decision.
- Inputs are explicit and typed where the language supports it.
- Output shape is predictable.
- The function has one primary reason to change.
- The happy path is easy to see.
- Edge cases are handled near the top or at the boundary.
- It avoids boolean argument soup. Prefer named options or separate functions.
- It avoids hidden global state.
- It does not mix parsing, validation, business rules, persistence, logging, and presentation in one block.
- It has tests or is covered through a higher-value integration scenario.

### Function Smells

- Generic names like `handleData`, `processItems`, `doThing`, `manager`, or `helper`.
- Long parameter lists with unclear order.
- Repeated null checks because the boundary contract is unclear.
- Over-broad `try/catch` blocks that hide the real failure.
- Comments that restate the code instead of explaining a decision.
- Deep nesting where guard clauses would make the path clearer.
- Functions that return different shapes depending on branch.
- Mixed sync/async behavior without an obvious reason.
- Copy-pasted logic with tiny mutations.

## Machine-Shaped Code Detection

Humify treats machine-generated-code detection as an evidence-based maintainability review. The flag is **machine-shaped**, not a claim of origin, unless there is direct proof.

### Confidence Levels

| Confidence | Meaning |
| --- | --- |
| High | Multiple strong signals and the code conflicts with local conventions. |
| Medium | Several signals, but could also be rushed human code. |
| Low | One or two weak signals; flag as readability risk only. |

### Signals

- Repetitive structure across unrelated files.
- Vague names that avoid domain language.
- Large functions that narrate every step but do not isolate concepts.
- Comments that sound generic or restate obvious statements.
- Defensive branches for impossible states with no domain reason.
- Inconsistent style compared with neighboring code.
- Utilities created before there is real reuse.
- Parallel abstractions that almost match existing ones.
- Error handling that logs everything but lets the caller keep going unsafely.
- Tests that assert implementation details or only prove mocks were called.
- UI or data code that is visually/functionally plausible but misses important edge states.
- Dead code, unused options, or unreachable branches left behind.

### Non-Signals

Do not flag code as machine-shaped just because it is:

- Verbose.
- New.
- Written in a common tutorial style.
- Formatted by a tool.
- Boilerplate required by a framework.
- Generated by a known compiler, ORM, SDK, or migration tool and not meant to be edited.

## Humify Review Pass

The review pass produces findings, not fixes. Each finding must explain the human cost and the refactor route.

For multi-finding reports, use this structure:

```markdown
## H001. Generic workflow hides domain behavior (MEDIUM)
File: src/features/import/process.ts
Lines: 44-131
Symptom: Readers cannot tell which import rules are business decisions versus parsing details.
Causal chain:
1. The function reads the file, normalizes rows, validates business rules, writes records, and logs results.
2. Each step uses generic names like `data`, `result`, and `items`.
3. A reader must simulate the whole function to find the actual domain decisions.
Repro trigger: Any change to import validation or persistence requires editing the same function.
Machine-shaped confidence: Medium
Signals: vague names, mixed responsibilities, repetitive defensive branches
Fix: Extract parsing, validation, and persistence into named units. Add characterization tests around accepted/rejected rows first.
```

Required review categories:

- **Structure:** unclear ownership, misplaced code, circular dependencies.
- **Function design:** mixed responsibilities, unreadable signatures, hidden side effects.
- **Domain language:** names that do not match the business problem.
- **Boundary hygiene:** parsing, validation, persistence, network, or UI concerns mixed together.
- **Error behavior:** swallowed failures, noisy logs, unsafe fallthrough.
- **Testing:** missing characterization tests, mock-only tests, untested edge paths.
- **Machine-shaped signals:** evidence-based confidence and exact signals.

## Deep Plan Refactoring Pass

After review, Humify converts findings into Deep Plan-style implementation units. The goal is to avoid giant refactor PRs and create safe slices.

Each unit must include:

- **Name:** action-oriented slice name.
- **Goal:** the readability or safety outcome.
- **Findings addressed:** finding IDs from the review pass.
- **Dependencies:** what must land first.
- **Files:** exact files expected to change.
- **Approach:** how behavior is preserved while structure changes.
- **Tests:** happy path, edge case, error path, and integration coverage.
- **Verification:** observable evidence that the unit is complete.
- **Rollback:** how to back out if behavior changes unexpectedly.

### Slice Ordering

Use this default order:

1. Add characterization tests or golden-output checks.
2. Rename concepts to match domain language.
3. Extract pure decision logic from side-effect-heavy code.
4. Move side effects to adapters or boundary modules.
5. Collapse duplicate abstractions.
6. Delete dead code and unreachable branches.
7. Tighten error handling and result contracts.
8. Run final readability and behavior verification.

### Slice Size Rule

A slice is too big if:

- It changes behavior and structure at the same time.
- It touches unrelated features.
- It requires reviewers to understand the whole codebase.
- It cannot be verified with a small set of tests or runtime checks.
- It has no clean rollback path.

## Humify Scorecard

Score each category from 0 to 3.

| Score | Meaning |
| --- | --- |
| 0 | Clear and locally understandable. |
| 1 | Minor friction, but safe to maintain. |
| 2 | Repeated confusion or elevated change risk. |
| 3 | Human-hostile code likely to cause bugs during changes. |

Categories:

- Readability
- Structure
- Function design
- Domain language
- Boundary hygiene
- Error behavior
- Testability
- Machine-shaped signal strength

Suggested interpretation:

- **0-5:** no refactor needed beyond opportunistic cleanup.
- **6-11:** targeted cleanup slices.
- **12-17:** planned refactor with characterization tests.
- **18+:** high-risk area; Deep Plan required before edits.

## Definition of Done

A humify refactor is done when:

- Existing behavior is preserved unless an intentional behavior change is documented.
- Tests or runtime checks cover the old behavior and the refactored path.
- Naming uses the domain language consistently.
- Functions have clear inputs, outputs, and side-effect boundaries.
- Deleted code is proven unused or replaced.
- Error paths are explicit.
- The final diff reduces cognitive load instead of moving complexity around.
- The audit report links each fix back to the original finding.

## Standard Artifacts

Use these files when applying Humify to a repository:

```text
HUMIFY-AUDIT.md       evidence-based findings and machine-shaped-code flags
HUMIFY-PLAN.md        Deep Plan-style implementation units and verification
HUMIFY-PATCHLOG.md    completed slices, tests run, behavior changes, residual risk
```

Start new artifacts from:

```text
templates/HUMIFY-MAP.template.md
templates/HUMIFY-HEATMAP.template.md
templates/HUMIFY-AUDIT.template.md
templates/HUMIFY-PLAN.template.md
templates/HUMIFY-PATCHLOG.template.md
```

Use these files when calibrating or prompting an AI model:

```text
HUMIFY-AI-INSTRUCTIONS.md       model judgment rubric
EXAMPLES.md                     concrete situations for model guidance
STELLAR-CODEBASES.md            positive examples to imitate
MASSIVE-CODEBASE-WORKFLOW.md    large-repo mapping and heatmap protocol
REFACTOR-PLAN-PROTOCOL.md       low-score refactor planning protocol
STEELMAN-PASS.md                adversarial strengthening pass
MODEL-CONTEXT-PACKET.md         context assembly and blind calibration order
HUMIFY-TESTING.md               calibration workflow
HUMIFY-SCORE.md                 evaluator output
HUMIFY-PLAN-SCORE.md            plan evaluator output
expected-plans/                 expected plan outputs for calibration
prompts/humify-audit.md         reusable audit prompt
prompts/humify-plan.md          reusable planning prompt
prompts/humify-calibration.md   fixture calibration prompt
prompts/humify-massive-codebase.md massive-codebase prompt
```

## Misuse Guardrails

Humify is useful only when its claims stay constrained.

Do not use Humify to:

- prove code origin from style alone,
- justify a whole-repo rewrite,
- enforce one preferred architecture over a coherent local convention,
- rank developers,
- delete generated or compatibility code without ownership proof,
- refactor risky behavior before tests or characterization evidence.

When in doubt, run `STEELMAN-PASS.md` and downgrade the claim.

If the repository already uses GSD or Deep Plan:

1. Convert `HUMIFY-AUDIT.md` into phase context.
2. Put scope decisions and constraints into `CONTEXT.md`.
3. Run Deep Plan against that phase.
4. Keep `HUMIFY-PLAN.md` as the human-readable refactor plan or merge its units into the generated plan.

## Operating Principle

Humify is not "make code prettier." It is a controlled process for making code understandable enough that a human can safely change it later.
