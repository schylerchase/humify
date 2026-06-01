# Expected Humify Audit: High-Trust Stale Data Fixture

Fixture: `fixtures/high-trust-stale-data/medicationPrices.ts`

Classification: High-risk refactor candidate
Machine-shaped confidence: Low
Refactor required: Yes, after tests

## Findings

```markdown
## H001. Stale medication price is presented as verified (HIGH)
File: fixtures/high-trust-stale-data/medicationPrices.ts
Lines: 8-23
Symptom: Users can treat old medication price data as current because the UI says "Verified price" without showing source freshness.
Causal chain:
1. The curated price includes `lastUpdated`.
2. The rendered card omits `lastUpdated`.
3. The badge says "Verified price" without date or stale-state context.
4. In a high-trust medication pricing flow, confidence wording can outlive the source data.
Repro trigger: Render the card for a curated price whose `lastUpdated` value is old.
Machine-shaped confidence: Low
Signals: stale source metadata, confidence wording mismatch, high-trust domain, missing freshness display
Fix: Add display tests for current, stale, and unknown source dates before changing price logic or trust wording.
```

## Cleared Items

- The code is short and domain-named. The issue is high-trust source freshness and confidence wording, not machine-shaped structure.
- Humify does not need to verify the real medication price to flag that the app hides its own freshness metadata.

## Required classification

| Category | Expected |
| --- | --- |
| Overall | High-risk refactor candidate |
| Machine-shaped confidence | Low |
| Refactor required | Yes, after tests |
