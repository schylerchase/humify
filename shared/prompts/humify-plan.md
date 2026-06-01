# Humify Plan Prompt

Use this prompt after a Humify audit exists and the next step is refactor planning.

```markdown
You are converting a Humify audit into a Deep Plan-style refactoring plan.

Read and follow:

- `MODEL-CONTEXT-PACKET.md`
- `HUMIFY.md`
- `HUMIFY-AI-INSTRUCTIONS.md`
- `EXAMPLES.md`
- `STELLAR-CODEBASES.md`
- `REFACTOR-PLAN-PROTOCOL.md`
- `STEELMAN-PASS.md`
- `HUMIFY-AUDIT.md`
- `templates/HUMIFY-PLAN.template.md`

Objective:

Turn findings into safe implementation units that improve readability while preserving behavior.

Planning rules:

- Refactor mode requires explicit opt-in. If the audit request did not authorize source edits, produce the plan only.
- Include a refactor readiness gate for dirty repo state, explicit opt-in, no-commit status, generated-artifact exclusions, and privacy.
- If the repo is dirty, block implementation unless the user explicitly accepts the current tree as the baseline or a clean worktree is used.
- No-commit trial is the default execution stance. Do not commit unless the user explicitly asks.
- Tests or characterization come before refactor movement.
- Do not mix behavior changes with structure cleanup unless explicitly approved.
- Keep slices small enough to review and roll back.
- Use existing project structure and conventions.
- Address generated/vendor files only when the audit says they are hand-edited or incorrectly checked in.
- Apply `STEELMAN-PASS.md` before final output.
- Reject any unit that starts with broad movement before behavior is protected.

Each implementation unit must include:

- Name
- Goal
- Findings addressed
- Dependencies
- Files
- Approach
- Tests
- Verification
- Rollback

Default ordering:

1. Characterization tests or golden-output checks
2. Domain naming cleanup
3. Pure logic extraction
4. Boundary/adapters extraction
5. Duplicate abstraction collapse
6. Dead code deletion
7. Error/result contract tightening
8. Final readability and behavior verification

Output:

Write `HUMIFY-PLAN.md`.

Use `templates/HUMIFY-PLAN.template.md` as the output shape.

If implementation is not currently safe, write "do not refactor yet" in the readiness gate and still identify the first safe slice that should happen after the gate opens.
```
