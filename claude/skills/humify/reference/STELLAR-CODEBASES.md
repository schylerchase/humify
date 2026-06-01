# Stellar Codebase Guidance

Humify should not only identify bad code. It should teach the model what good code looks like.

Use this file when the model needs positive examples to imitate.

## Stellar Codebase Traits

A stellar codebase is not necessarily fancy. It is easy to understand, safe to change, and honest about its boundaries.

Look for:

- Names that match the domain.
- Entry points that orchestrate instead of implementing everything.
- Domain logic that is isolated from infrastructure.
- Side effects behind clear ports, adapters, or boundary modules.
- Tests that prove user-visible or operator-visible behavior.
- Small modules with clear ownership.
- Explicit error behavior.
- Minimal global state.
- Few decorative wrappers.
- Generated code separated from hand-edited code.

## Stellar Does Not Mean Over-Architected

Do not mistake "stellar" for a specific architecture pattern.

A stellar codebase may be:

- a small script with three well-named functions,
- a Rails app following Rails conventions,
- a React app with clean component and state boundaries,
- a PowerShell admin package with explicit paths, logs, and no hidden reboot behavior,
- a service with domain/application/infrastructure folders.

The common trait is not folder shape. The common trait is that a maintainer can predict where behavior lives and how to change it safely.

## Positive Reference Example

See:

```text
examples/stellar-codebase/
```

That example demonstrates:

- `domain/` owns invoice rules.
- `application/` owns the use case.
- `infrastructure/` owns persistence details.
- tests verify behavior through a useful workflow boundary.

## How To Use Stellar Examples During Review

When a target codebase scores low, compare it to the stellar traits:

| Question | Strong answer | Weak answer |
| --- | --- | --- |
| Where is the domain rule? | In a named domain module. | Hidden in a route, UI component, or database adapter. |
| What does this function do? | Its name and signature make that clear. | The reader must inspect the body and callers. |
| Where are side effects? | At boundaries. | Mixed into calculation and validation logic. |
| What proves behavior? | Tests assert outputs and state changes. | Tests only assert mocks were called. |
| Can this be changed locally? | Yes, dependencies are explicit. | No, changes require tracing hidden globals or broad helpers. |

## Positive Example Finding

Use positive examples in `Cleared Items` when helpful:

```markdown
## Cleared Items

- `src/domain/invoice.ts` keeps subtotal and tax rules in pure functions, similar to the stellar invoice example. No Humify finding.
- `src/application/createInvoice.ts` is thin orchestration. Persistence is behind `InvoiceRepository`, so boundary hygiene is acceptable.
```

## When Not To Force The Example

Do not force every codebase into this exact folder structure.

The stellar example is a teaching aid, not a universal architecture mandate. If a repository already has a clear convention, Humify should improve that convention instead of replacing it.

## Stellar Codebase Rubric

Score positive examples as well as negative ones.

| Area | Strong signal | Weak signal |
| --- | --- | --- |
| Naming | Names describe domain decisions. | Names describe mechanics only. |
| Boundaries | Side effects are at edges. | Side effects are mixed into decisions. |
| Tests | Tests protect behavior. | Tests only assert mocks or snapshots without meaning. |
| Errors | Failure modes are explicit. | Errors are swallowed or logged without action. |
| Scale | Modules have clear ownership. | Helpers and managers collect unrelated logic. |
| Change safety | A small change has a predictable location. | A small change requires broad search and guessing. |

Use this rubric to write `Cleared Items` too. Humify should say when code is good.

## High-Quality Patterns To Prefer

### Thin Entrypoint

```ts
export async function route(request: Request) {
  const command = parseRequest(request);
  const result = await createInvoice(command, dependencies);
  return toHttpResponse(result);
}
```

Why it is good:

- Parsing, workflow, and response formatting are visible.
- Business decisions are elsewhere.
- The route can be tested lightly while domain behavior is tested deeply.

### Domain Decision

```ts
export function canApproveInvoice(invoice: Invoice, approver: User): boolean {
  return approver.limit.cents >= invoice.total.cents;
}
```

Why it is good:

- The name says the decision.
- Inputs are explicit.
- No side effects.
- Easy to test.

### Adapter Boundary

```ts
export type InvoiceRepository = {
  save(invoice: Invoice): Promise<void>;
};
```

Why it is good:

- Domain/application code does not need database details.
- Tests can use a simple implementation.
- Persistence can change without changing invoice rules.

### Behavior Test

```ts
it("rejects invoices without lines", async () => {
  await expect(createInvoice(emptyInvoiceRequest)).rejects.toThrow(
    "Invoice requires at least one line.",
  );
});
```

Why it is good:

- Tests observable behavior.
- Protects a domain rule.
- Does not depend on private implementation shape.
