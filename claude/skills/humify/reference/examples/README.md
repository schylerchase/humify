# Stellar Codebase Example

This is a small reference codebase for Humify. It is intentionally simple, but it demonstrates what "humane to maintain" looks like.

The example models invoice creation.

## Why This Is Stellar

- Domain concepts have names: `Invoice`, `InvoiceLine`, `Money`, `TaxPolicy`.
- Business rules live in `src/domain`.
- The application workflow lives in `src/application`.
- Persistence is represented by a small port/interface instead of leaking database details into domain code.
- Side effects are at the boundary.
- Tests describe behavior, not implementation mechanics.
- The code is small without being vague.

## Shape

```text
src/
  application/
    createInvoice.ts
  domain/
    invoice.ts
    money.ts
    taxPolicy.ts
  infrastructure/
    inMemoryInvoiceRepository.ts
tests/
  createInvoice.test.ts
```

## What The Model Should Learn

When reviewing a real codebase, this example should guide the model toward:

- Naming domain decisions directly.
- Keeping workflows thin.
- Separating calculation from persistence.
- Testing behavior at the useful boundary.
- Avoiding decorative abstractions.

## What This Example Is Not

This is not a mandatory architecture for every project. It is a compact reference showing clear ownership and readable boundaries.

