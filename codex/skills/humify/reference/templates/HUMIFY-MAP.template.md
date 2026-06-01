# Humify Map

Repository: `<repo name or path>`
Generated: `<date/time>`
Reviewer: `<model/person>`
Scope: `<whole repo | selected paths>`

## Run Safety

- Mode: `<read-only audit | explicit refactor trial>`
- Refactor permission: `<not requested | explicitly requested by user>`
- Commit permission: `<not granted | explicitly granted by user>`
- Dirty repo gate: `<clean | dirty-audit-only | dirty-baseline-explicitly-accepted>`
- Private output location: `<ignored local folder or public-safe path>`

## Privacy Mode

- Publication mode: `<private local run | public-safe synthetic summary>`
- Private identifiers present: `<none | local paths | private repo names | remotes | account/customer identifiers>`
- Redaction action: `<not needed | move to ignored run folder | convert to synthetic example>`

## Coverage Statement

- Inventory coverage: `<what was discovered>`
- Sample coverage: `<what was read as representative sample>`
- Deep-dive coverage: `<what received file/line findings>`
- Unknowns: `<areas not inspected deeply enough to score confidently>`

## Repository Summary

Purpose:
`<one or two sentences about what the repo appears to do>`

Primary workflows:

- `<workflow>`
- `<workflow>`

Primary languages/frameworks:

- `<language/framework>`

Primary entrypoints:

- `<path>` — `<role>`

## Exclusion Map

Files or directories excluded before scoring:

| Path/pattern | Reason | Example |
| --- | --- | --- |
| `<path>` | `<generated/vendor/build/lockfile/etc>` | `<example file>` |

## Inventory

| Area | Paths | Role | Generated/excluded? | Tests found? | Notes |
| --- | --- | --- | --- | --- | --- |
| `<area>` | `<paths>` | `<workflow/boundary>` | `<yes/no>` | `<paths or none>` | `<risk/context>` |

## Boundary Map

| Boundary | Owner paths | Depends on | Side effects | Notes |
| --- | --- | --- | --- | --- |
| `<domain/application/infrastructure/ui/etc>` | `<paths>` | `<dependencies>` | `<none/db/network/fs/etc>` | `<notes>` |

## Sampling Ledger

Sampled:

- `<path>` — `<why this file was sampled>`

Not deeply sampled:

- `<path or area>` — `<why it remains unknown or lower priority>`

## Initial Hotspot Candidates

| Candidate | Reason | Evidence | Suggested next step |
| --- | --- | --- | --- |
| `<area>` | `<score/churn/risk/size/convention drift>` | `<file or observed fact>` | `<sample/deep dive/defer>` |

## Resume State

Reviewed:

- `<area>`

Open hotspots:

- `<area>`

Do not re-review:

- `<area already cleared>`

Next pass should inspect:

- `<specific path and why>`
