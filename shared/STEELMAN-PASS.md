# Humify Steelman Pass

The steelman pass strengthens Humify outputs before they are trusted.

It asks: "If a skeptical senior engineer, maintainer, operator, or product owner pushed back on this review, would the evidence and plan still hold up?"

Use this file when improving Humify documents, reviewing a codebase, or converting a low score into a refactor plan.

## Purpose

The steelman pass prevents Humify from becoming:

- a style guide with stronger branding,
- a vague AI-code detector,
- a rewrite generator,
- a pile of subjective taste,
- a report that cannot survive contact with a real repo.

Humify should produce claims that are constrained, evidenced, useful, and safe to act on.

## When To Run It

Run a steelman pass:

- before finalizing `HUMIFY-AUDIT.md`,
- before finalizing `HUMIFY-PLAN.md`,
- before accepting a low-score heatmap,
- after adding or changing framework rules,
- when a finding claims machine-shaped code,
- when the recommended plan touches risky behavior,
- when the target repo is large enough that coverage is partial.

## Steelman Questions

Ask these questions before final output.

### Evidence

- What exact code fact supports each claim?
- Is the finding anchored to file and line evidence?
- Does the causal chain explain why the code becomes harder to change or unsafe?
- Is any claim really an inference? If yes, is it labeled as one?
- Would the finding still make sense to a maintainer who disagrees with the style preference?

### Counterexamples

- Could this be normal framework boilerplate?
- Could this be generated or vendored code that should be excluded?
- Could this be rushed human code rather than machine-shaped code?
- Could the repo have a local convention that makes this pattern acceptable?
- Could the "bad" abstraction be a stable public API or compatibility layer?

### Risk

- What behavior could change if the recommended fix is applied?
- Are tests or characterization required before movement?
- Does the fix mix behavior change and structure cleanup?
- Is rollback possible?
- Does the plan touch money, permissions, deletion, infrastructure, imports, exports, concurrency, or operator-facing logs?

### Scale

- Did the review cover enough of the repo to justify the claim?
- Are unreviewed areas named?
- Are repeated issues clustered instead of duplicated?
- Is the sampling plan biased toward high-risk areas?
- Does the heatmap distinguish "observed" from "suspected" risk?

### Usefulness

- Can another engineer execute the next step without guessing?
- Does the finding name the human cost?
- Does the plan identify exact files?
- Does each unit have verification and rollback?
- Is the first step small, safe, and valuable?

## Evidence Hierarchy

Prefer stronger evidence.

| Strength | Evidence type | Example |
| --- | --- | --- |
| 5 | Direct repository proof | generated header, commit metadata, failing test, public API contract |
| 4 | Local convention proof | neighboring modules consistently keep domain rules outside routes |
| 3 | Code fact | function mixes validation, database writes, logging, and rendering |
| 2 | Behavioral inference | missing field defaults may hide invalid records |
| 1 | Weak signal | vague name, long function, repeated comments |

Rules:

- A High severity finding needs at least one strength 4 or 5 signal, or several strength 3 signals tied to a risky behavior.
- High machine-shaped confidence needs multiple signals and a local-convention mismatch, not one generic name.
- Weak signals can support a finding but should not be the whole finding.
- If evidence is strength 1 or 2 only, downgrade confidence or move the item to `Cleared Items` / `Open Questions`.

## Claim Tightening

Replace broad claims with constrained claims.

| Weak claim | Steelmanned claim |
| --- | --- |
| "This is AI-generated." | "This is machine-shaped: generic naming, repeated field blocks, and obvious narration make the domain contract hard to trust." |
| "This file is messy." | "This function mixes validation, persistence, and reporting, so changes to one policy can accidentally affect another." |
| "Refactor this module." | "Add characterization tests for current import outcomes, then extract row validation before touching persistence." |
| "Architecture is bad." | "The route duplicates pricing behavior that otherwise lives in `src/domain/pricing`, splitting one rule across two boundaries." |
| "Needs better tests." | "The current test only asserts a mock call and would pass if the saved customer payload were wrong." |

## Steelman Output Block

For normal audits, do not add a long steelman section unless the user asks. Apply the pass internally and revise the output.

For massive-codebase reviews or low-score plans, include a short block:

```markdown
## Steelman Check

- Strongest evidence: <best file/line or convention proof>
- Biggest uncertainty: <what was not reviewed or remains inferred>
- Main false-positive risk: <why this may be less severe>
- Safety guardrail: <test, rollback, or scope limit>
- Decision: <proceed | narrow scope | gather more evidence>
```

## Plan Acceptance Gates

A Humify plan passes the steelman check only if:

- every high-risk unit starts with behavior protection,
- every finding maps to at least one unit or is explicitly deferred,
- every unit names exact files,
- every unit has verification and rollback,
- generated files remain excluded,
- public API changes are called out,
- unreviewed areas are listed as unknowns,
- the plan does not rely on a whole-repo rewrite.

## Failure Modes To Watch

Reject or revise Humify output when it:

- overclaims origin,
- hides uncertainty,
- reports too many duplicate findings,
- confuses a style preference with maintainability risk,
- recommends extraction before tests,
- forces the stellar example's folder structure onto a repo with a different good convention,
- gives a massive-repo verdict without coverage accounting,
- fails to identify the first safe slice.

