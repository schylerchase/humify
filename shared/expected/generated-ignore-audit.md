# Expected Humify Audit: Generated Ignore Fixture

Fixture: `fixtures/generated-ignore/client.generated.ts`

Expected result: **EXCLUDED**

## Findings

No Humify findings expected.

## Why this should be excluded

- The file declares itself as generated.
- The filename includes `.generated`.
- The comment says not to edit it by hand.
- Humify should not score generated SDK/client/migration output unless the repository edits it manually.

## Required classification

| Category | Expected |
| --- | --- |
| Overall | Excluded generated file |
| Machine-shaped confidence | Not applicable |
| Refactor required | No |
| Characterization tests required before edits | No |

## Failure condition

Humify fails this fixture if it reports machine-shaped findings against this file without first recognizing that the file should be excluded.

