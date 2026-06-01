# Humify Massive Codebase Prompt

Use this prompt for repositories too large to review directly.

```markdown
You are running Humify on a massive codebase.

Read and follow:

- `MODEL-CONTEXT-PACKET.md`
- `HUMIFY.md`
- `HUMIFY-AI-INSTRUCTIONS.md`
- `EXAMPLES.md`
- `MASSIVE-CODEBASE-WORKFLOW.md`
- `REFACTOR-PLAN-PROTOCOL.md`
- `STEELMAN-PASS.md`
- `templates/HUMIFY-MAP.template.md`
- `templates/HUMIFY-HEATMAP.template.md`
- `templates/HUMIFY-AUDIT.template.md`
- `templates/HUMIFY-PLAN.template.md`

Objective:

Produce a reliable repository map, heatmap, hotspot audit, and refactor plan without pretending the whole codebase fits in one context window.

Default mode:

- Start read-only.
- Capture git state, including staged, unstaged, untracked, ignored, and nested repo state.
- If the repo is dirty, keep the refactor gate closed unless the user explicitly accepts the current tree as the baseline.
- Do not commit.
- Keep private run artifacts in ignored folders. Public artifacts must be synthetic or sanitized.

Process:

1. Inventory the repository.
2. Exclude generated/vendor/build output.
3. Build a stratified sampling plan.
4. Score areas into a heatmap.
5. Deep-dive only the highest-value hotspots.
6. Run a coverage steelman pass.
7. Produce `HUMIFY-AUDIT.md`.
8. Produce `HUMIFY-PLAN.md` when scores are low or refactor risk is high.

Required artifacts:

- `HUMIFY-MAP.md`
- `HUMIFY-HEATMAP.md`
- `HUMIFY-AUDIT.md`
- `HUMIFY-PLAN.md` when triggered

Rules:

- Every finding needs file and line evidence.
- Do not claim AI generation without direct evidence.
- Use `machine-shaped` for maintainability signals.
- Cluster repeated issues instead of spamming duplicates.
- Characterization tests come before risky movement.
- Low score triggers planning, not direct rewrite.
- Dirty repo plus low score means "do not refactor yet" unless explicit baseline acceptance is recorded.
- Generated files are excluded unless proven hand-edited.
- Include a `Steelman Check` for low-score areas and massive-codebase coverage claims.
- Name unreviewed areas and mark inferred risks as inference.
- Include verification commands/results and a first safe slice.

Output:

Start with the map, sampling ledger, coverage statement, and heatmap. Do not write findings until you can justify the sampled hotspots.
```
