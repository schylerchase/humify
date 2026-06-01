# Humify Concrete Examples

This file gives the model concrete situations to imitate. Use it with `HUMIFY-AI-INSTRUCTIONS.md` when the model needs more than abstract rules.

These examples are intentionally specific but synthetic. They should teach exact review behavior without naming private repos, absolute paths, remotes, or proprietary source maps.

Each example shows:

- What the model should notice.
- What it should avoid overclaiming.
- What a useful Humify finding sounds like.
- What the safe next move should be.

## Situation 1: Generic Processor Hides A Domain Contract

### Code Shape

```ts
export function processData(data: any[], options: any) {
  return data.map((item) => ({
    id: item.id ?? "",
    name: item.name ? String(item.name).trim() : "",
    status: item.status ?? "unknown",
  }));
}
```

### What To Notice

- The function name does not reveal the domain.
- `data`, `item`, and `options` make the reader infer the contract.
- Defaults may create plausible but invalid records.
- `any` avoids documenting the real input and output shapes.

### What Not To Say

Do not say "this is AI-generated." Say "this is machine-shaped" only if enough signals exist.

### Good Finding

```markdown
## H001. Generic processor hides the record contract (HIGH)
File: src/import/processData.ts
Lines: 1-12
Symptom: Callers can pass malformed records and still receive plausible output, making downstream behavior hard to trust.
Causal chain:
1. The function accepts `any[]` and `any`, so required fields are not documented or checked.
2. Missing fields are replaced with generic defaults.
3. Downstream code cannot distinguish a real empty value from a silently manufactured one.
Repro trigger: Import a row with a missing required `name` or unknown `status`.
Machine-shaped confidence: High
Signals: generic naming, weak types, repetitive field defaults, hidden domain contract
Fix: Identify the real record type, add characterization tests for missing fields, then replace `processData` with a domain-named normalizer.
```

### Safe Next Move

Do not start by renaming everything. First identify the actual record domain and add tests for required, optional, and invalid fields.

## Situation 2: Messy Human Workflow With Real Domain Terms

### Code Shape

```ts
export async function importCustomers(rows, db, logger) {
  for (const row of rows) {
    if (!row.email.includes("@")) {
      logger.warn("bad email");
      continue;
    }

    const existing = await db.customers.findByEmail(row.email);
    if (existing) {
      await db.customers.update(existing.id, row);
    } else {
      await db.customers.create(row);
    }
  }
}
```

### What To Notice

- It uses domain terms: customers, import, email.
- The issue is mixed responsibility: validation, normalization, persistence, and reporting.
- This is probably not primarily machine-shaped.

### What Not To Say

Do not assign high machine-shaped confidence just because the function is too broad.

### Good Finding

```markdown
## H001. Customer import mixes policy and persistence (MEDIUM)
File: src/customers/importCustomers.ts
Lines: 1-15
Symptom: Changing validation rules or persistence behavior requires editing the same workflow.
Causal chain:
1. The function validates email, decides create versus update, logs rejects, and writes to the database.
2. These responsibilities change for different reasons.
3. A small validation change can accidentally alter write behavior.
Repro trigger: Add a validation rule for inactive customers.
Machine-shaped confidence: Low
Signals: mixed responsibilities, broad dependency contract
Fix: Add import outcome tests, then extract validation and normalization before touching persistence.
```

### Safe Next Move

Characterize accepted, skipped, created, and updated rows before extraction.

## Situation 3: Risky Business Rule That Looks Simple

### Code Shape

```ts
export function applyDiscounts(items, discounts) {
  let total = sum(items);
  for (const discount of discounts) {
    total -= eligibleTotal(items, discount) * discount.percent;
  }
  return total;
}
```

### What To Notice

- The code may be readable but dangerous to refactor.
- Discount stacking, rounding, categories, and minimum-spend thresholds are hidden policy.
- The correct first action is tests, not cleanup.

### Good Finding

```markdown
## H001. Discount policy needs characterization before cleanup (HIGH)
File: src/cart/applyDiscounts.ts
Lines: 1-7
Symptom: A readability refactor could silently change discount stacking or eligibility behavior.
Causal chain:
1. The function combines subtotal, eligibility, discount stacking, and final total calculation.
2. Those rules interact across items and discounts.
3. Extracting helpers without tests can preserve the shape of the code while changing customer-facing totals.
Repro trigger: Apply multiple category discounts with minimum spend thresholds.
Machine-shaped confidence: Low
Signals: compact business rule with implicit policy
Fix: Add characterization tests for stacking, category eligibility, minimum spend, and rounding before extraction.
```

### Safe Next Move

Write tests for known current behavior, including odd cases that may look wrong. Refactor after behavior is pinned.

## Situation 4: Generated File That Looks Machine-Shaped

### Code Shape

```ts
// <auto-generated>
// Do not edit by hand.
export type ApiCustomer = {
  id?: string;
  name?: string;
};
```

### What To Notice

- The file is generated and should not be scored.
- Generated code may be verbose, repetitive, or weakly typed because it mirrors an external contract.

### Good Output

```markdown
Classification: Excluded generated file
Machine-shaped confidence: Not applicable
Refactor required: No

## Findings

No findings expected.

## Cleared Items

- The file has generated-code markers and should be excluded from Humify scoring.
```

### Safe Next Move

If the generated shape is causing pain, review the generator config or adapter layer, not the generated file itself.

## Situation 5: Local Convention Drift

### Code Shape

The codebase usually stores business rules in `src/domain`, but a new route handler contains validation, pricing, database writes, and email sending.

### What To Notice

- The problem is not just a long file.
- The code violates local architecture by putting domain rules and side effects in the entrypoint.
- The finding should cite the local convention if evidence exists.

### Good Finding

```markdown
## H001. Route handler bypasses established domain boundary (HIGH)
File: src/app/routes/createOrder.ts
Lines: 18-96
Symptom: Order pricing rules now live in an HTTP route instead of the domain layer, so future pricing changes are easy to miss.
Causal chain:
1. Existing order rules are implemented under `src/domain/orders`.
2. The new route calculates pricing and writes records directly.
3. Pricing behavior is now split across two ownership boundaries.
Repro trigger: Change the order discount rule in the domain layer and call this route.
Machine-shaped confidence: Medium
Signals: convention drift, mixed side effects, duplicate domain rule
Fix: Add route-level characterization tests, then move pricing decisions into the existing order domain service while keeping HTTP parsing in the route.
```

### Safe Next Move

Do a local convention pass before judging. A pattern is a defect when it conflicts with the repo's own architecture or increases change risk.

## Situation 6: Decorative Abstraction

### Code Shape

```ts
class CustomerManager {
  process(customer) {
    return CustomerProcessor.processCustomerData(customer);
  }
}
```

### What To Notice

- Classes and service layers are not automatically better.
- A wrapper with no domain decision can make code harder to navigate.
- The fix may be deletion or inlining, not more abstraction.

### Good Finding

```markdown
## H001. Wrapper abstraction adds navigation without ownership (LOW)
File: src/customers/CustomerManager.ts
Lines: 1-5
Symptom: Readers must jump through an extra class that does not own a decision or adapt a boundary.
Causal chain:
1. `CustomerManager.process` delegates directly to `CustomerProcessor.processCustomerData`.
2. The wrapper adds no validation, policy, side effect boundary, or stable interface.
3. Future readers must inspect both files to understand one operation.
Repro trigger: Trace customer import processing from the entrypoint.
Machine-shaped confidence: Medium
Signals: generic manager name, pass-through abstraction, no clear ownership
Fix: Remove the wrapper or rename it only if it becomes the actual owner of a domain workflow.
```

### Safe Next Move

Verify no external callers depend on the wrapper as a public API before deleting it.

## Situation 7: Tests That Only Prove Mocks Were Called

### Code Shape

```ts
it("saves customer", async () => {
  await service.saveCustomer(input);
  expect(repository.save).toHaveBeenCalled();
});
```

### What To Notice

- This test may not prove behavior.
- Humify should prefer tests that pin observable outcomes and important edge cases.

### Good Finding

```markdown
## H001. Test does not protect customer save behavior (MEDIUM)
File: src/customers/customerService.test.ts
Lines: 12-15
Symptom: The test can pass even if the service saves the wrong normalized customer shape.
Causal chain:
1. The test only asserts that `repository.save` was called.
2. It does not assert the saved customer fields or validation outcome.
3. A refactor can break normalization while keeping the mock call intact.
Repro trigger: Change email normalization inside `saveCustomer`.
Machine-shaped confidence: Low
Signals: mock-only assertion, missing behavior expectation
Fix: Assert the saved customer payload and add an invalid-input case before refactoring `saveCustomer`.
```

### Safe Next Move

Improve tests before using them as refactor protection.

## Situation 8: Massive File With Multiple Findings

### Code Shape

A 1,500-line module handles parsing, validation, database access, rendering, retries, and logging.

### What To Notice

- Do not write 30 findings from one file.
- Cluster the risk into a few high-value findings.
- Produce a refactor map, not a rewrite demand.

### Good Output Strategy

```markdown
Classification: High-risk refactor candidate
Machine-shaped confidence: Medium
Refactor required: Yes, after tests

## Findings

H001. Parser, validator, and writer are fused into one workflow
H002. Retry behavior is undocumented and untested
H003. Rendering code depends on persistence details

## Refactor Direction

1. Add golden-output tests around the current end-to-end workflow.
2. Extract parsing as pure logic.
3. Extract validation as named domain rules.
4. Move persistence behind an adapter.
5. Split rendering after output snapshots are stable.
```

### Safe Next Move

Start with the smallest characterization test that proves the current behavior end-to-end.

## Situation 9: Bad Humify Finding vs Steelmanned Finding

### Bad Finding

```markdown
## H001. Bad architecture (HIGH)
File: src/orders/createOrder.ts
Lines: 1-120
Symptom: This file is messy and probably AI-generated.
Causal chain:
1. It is too long.
2. It does too much.
3. It should be refactored.
Fix: Rewrite using clean architecture.
```

### Why It Fails

- "Bad architecture" is not specific.
- "Probably AI-generated" is unsupported.
- The causal chain does not explain user or maintainer harm.
- The fix is a rewrite, not a safe slice.
- There is no test or rollback guidance.

### Steelmanned Finding

```markdown
## H001. Order route owns pricing decisions that belong to the order domain (HIGH)
File: src/orders/createOrder.ts
Lines: 44-91
Symptom: Pricing changes can be applied in the domain layer while this route continues using different embedded rules.
Causal chain:
1. Existing order pricing rules live in `src/domain/orders/pricing.ts`.
2. This route calculates discounts and tax directly before writing the order.
3. One pricing policy now has two editable locations, so future pricing changes can diverge.
Repro trigger: Change discount eligibility in the domain pricing module and create an order through this route.
Machine-shaped confidence: Medium
Signals: convention drift, mixed HTTP/persistence/pricing concerns, duplicated domain rule
Fix: Add route-level characterization tests for current pricing output, then move the pricing decision into the existing domain pricing module while keeping request parsing in the route.
```

### Lesson

Steelman the finding until it could survive disagreement from someone who wrote the code.

## Situation 10: Human-Shaped Plugin Code With High Refactor Risk

### Code Shape

A large plugin view class renders a dashboard, persists UI state, creates markdown notes, updates task lines, and suppresses self-generated vault modify events. The comments mention specific historical race conditions and platform behavior.

### What To Notice

- The code is not automatically machine-shaped just because it is large.
- Specific comments about prior race bugs are a positive signal.
- The main issue is refactor blast radius: UI state, vault writes, and event ordering are coupled.
- Dirty worktree state should block refactoring but not block read-only auditing.

### What Not To Say

Do not call it AI-generated. Do not recommend a broad rewrite. Do not remove comments that preserve historical context.

### Good Finding

```markdown
## H001. Dashboard view owns UI state and vault-write workflows (MEDIUM)
File: src/plugin/VaultDashboardView.ts
Lines: 410-452, 1280-1324, 1710-1742, 2040-2062
Symptom: A maintainer changing project note creation or dashboard state persistence must reason through the same large class that handles render refresh and self-modify suppression.
Causal chain:
1. The view class owns render callbacks, dashboard state, vault folder/file creation, project note creation, and event suppression.
2. Those behaviors change for different reasons and have different failure modes.
3. A focused UI change can accidentally alter vault writes or race handling.
Repro trigger: Change project note creation, category target creation, or dashboard scope persistence.
Machine-shaped confidence: Low
Signals: high responsibility density, side-effect coupling, real historical comments, domain-specific workflow
Fix: Preserve the current dirty worktree, add characterization tests for one workflow, then extract category/project note creation before broader UI cleanup.
```

### Safe Next Move

Treat it as a high-risk refactor candidate with low machine-shaped confidence. First freeze the active working state, then write tests around one workflow and extract that workflow only.

## Situation 11: Embedded Generated Artifact Duplicates Maintained Source

### Code Shape

A repo has a readable standalone PowerShell exporter under `scripts/export-data.ps1`, but the app also ships a downloadable exporter generated from a dense JavaScript string array.

### What To Notice

- The embedded artifact may be intentionally generated, but it still affects users.
- The risk is behavioral drift between the maintained script and the embedded downloadable script.
- Compact string arrays are hard to review and can deserve high machine-shaped confidence without claiming AI origin.
- The fix should be parity tests or template generation, not manual prettifying of both copies.

### Good Finding

```markdown
## H001. Embedded exporter can drift from the maintained script (HIGH)
File: src/exports/downloadableExporter.ts; scripts/export-data.ps1
Lines: src/exports/downloadableExporter.ts 180-318; scripts/export-data.ps1 1-92, 210-268
Symptom: Users can download a script whose profile, permission, or error behavior differs from the standalone script documented in the repo.
Causal chain:
1. The standalone script owns readable profile validation and access-denied messaging.
2. The embedded script duplicates that behavior as compact string literals.
3. Future fixes can land in one path while the other silently serves stale behavior.
Repro trigger: Change profile/region/preflight behavior in the standalone script and download the embedded script.
Machine-shaped confidence: High
Signals: compact generated string source, duplicated operator-facing behavior, no visible parity check
Fix: Generate both outputs from one template or add golden parity tests for the user-facing contract before editing either script.
```

### Safe Next Move

Pick contract points first: profile alias, region fallback, permission-denied message, output log shape, and multi-account behavior. Test those against both script paths before refactoring.

## Situation 12: Historical Audit Drift

### Code Shape

A repo contains `BUG_AUDIT.md` from months ago. It references files that no longer exist and issues that are already fixed in the current checkout.

### What To Notice

- Audit documents are evidence only after they are checked against current code.
- Historical findings can still be useful, but they should not drive current planning blindly.
- A stale audit is a planning hazard, not proof that the current code is broken.

### Good Finding

```markdown
## H001. Historical audit references deleted architecture (MEDIUM)
File: BUG_AUDIT.md
Lines: 3-5, 12-31, 118-119
Symptom: Future reviewers can spend time fixing files or defects that are not present in the current checkout.
Causal chain:
1. The audit says it covers all source files as of a past date.
2. Several findings cite paths that no longer exist.
3. A model or maintainer using the audit as live truth can plan the wrong work.
Repro trigger: Search the current checkout for each cited path.
Machine-shaped confidence: Not applicable
Signals: stale planning artifact, current-code mismatch
Fix: Archive the audit with its original commit/date or rewrite it as a current active/resolved/not-present table.
```

### Safe Next Move

Run a current checkout verification pass before using old audit content in a refactor plan.

## Situation 13: High-Trust Data Is Stale But Wording Sounds Certain

### Code Shape

A healthcare or pricing app ships curated data with a `lastUpdated` field, but the UI labels displayed values as "verified" without showing the date.

### What To Notice

- The defect is not simply stale data. The defect is stale data plus confidence wording.
- High-trust domains need source freshness, provenance, and failure behavior checks.
- Do not browse or validate the real-world value unless the task requires it; first check whether the app represents its own source metadata honestly.

### Good Finding

```markdown
## H001. Stale curated prices are presented as verified (HIGH)
File: src/data/pricing.json; src/components/PriceCard.tsx
Lines: src/data/pricing.json 120-128; src/components/PriceCard.tsx 42-47
Symptom: Users can treat old price data as current because the UI labels it as verified without displaying source freshness.
Causal chain:
1. The data file has `lastUpdated`, but the UI does not show it.
2. The card labels curated values as "Verified price."
3. In a high-trust pricing flow, confidence wording can outlive the source data.
Repro trigger: Search a curated medication and inspect the price card source label.
Machine-shaped confidence: Low
Signals: stale source metadata, confidence wording mismatch, high-trust domain
Fix: Add a display test for freshness metadata, pass the date through the comparison model, and make the label date-aware or stale-aware.
```

### Safe Next Move

Add tests for current, stale, and unknown source dates before changing pricing logic.

## Situation 14: Registry Drift Silently Drops A Resource Type

### Code Shape

A cloud topology tool has a sidebar input, folder-upload mapper, snapshot diff engine, report builder, and export script. A new resource type is added to two of those surfaces, but not all of them.

```js
// UI
{ id: "in_private_links", label: "Private Links" }

// Snapshot reconstruction
privateEndpoints: parseTextarea("in_private_endpoints")

// Export script
run "Private Links" "private-links.json" cloud private-link list
```

### What To Notice

- This is not a naming-style nit. It is a contract split across UI, import, export, snapshot, and report surfaces.
- The failure mode is silent omission: the app can render or export one resource path while snapshots, reports, or diffs ignore it.
- The safest fix is a registry or fixture test before renaming code.
- Machine-shaped confidence depends on the broader signals. The finding can be high severity even when machine-shaped confidence is only medium.

### Good Finding

```markdown
## H001. Snapshot context uses stale resource input IDs (HIGH)
File: src/modules/snapshotDiff.ts
Lines: 42-58
Symptom: Snapshot comparisons can omit private-link resources even after users upload or paste them successfully.
Causal chain:
1. The active UI stores private-link JSON under `in_private_links`.
2. Snapshot reconstruction reads `in_private_endpoints`.
3. Missing textarea IDs are treated as empty arrays.
4. The diff engine reports no change for resources it never loaded.
Repro trigger: Upload a fixture containing one private-link resource, save two snapshots, and compare them.
Machine-shaped confidence: Medium
Signals: parallel resource naming schemes, stale migration aliases, silent empty fallback, missing registry test
Fix: Add a fixture test proving every active input ID reaches snapshot context, then replace hardcoded IDs with the canonical input registry.
```

### Safe Next Move

Create one table of canonical resource IDs and assert that the UI, upload mapper, export filenames, snapshot/diff context, and report builder all use it. Do not start by changing names across the app without those tests.
