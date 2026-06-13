# Rank, then judge

Turn humify's verified plan into a single sequenced next move — using a prompt, not a command.

## What this is / isn't

Humify **ranks** cleanup work. It orders findings by signal and severity, groups
them into `HMF-###` plan items, and stamps each applyable item with a verification
verdict. What it does *not* do is **judge** them: it never tells you which one item to
do first, which single item is the prize, or where to stop for the day.

This doc is a **prompt pattern** that adds that judgment layer. It is not a new humify
command and it never will be. You paste a template into any LLM session, hand it
humify's JSON, and get back a sequenced recommendation.

The line to keep straight:

- **Facts** come from humify — findings, scores, `action`, `verification`. Trust them.
- **Judgment** comes from the LLM — `next_move`, the prize, the stop point. It is
  opinion. Treat it as opinion.

`rank → judge`. Humify ranks. The prompt sequences.

## Why a prompt, not a command

The user chose a prompt over a `humify advise` command deliberately:

- **Judgment belongs in the conversation.** Sequencing depends on what *you* are about
  to do next — the branch you're on, the deadline, the blast radius you can tolerate.
  A command would freeze one generic answer; a session can weigh your actual context.
- **Persisted judgment goes stale.** Humify's brand is verified, reversible, **current**
  facts. An `advice.json` would be wrong the moment the tree changed. So this advice is
  never written to disk. You re-run the prompt on demand against fresh JSON instead.

## Inputs

Produce the facts first. These commands write JSON under `.humify/` and never modify
your source:

```sh
humify analyze            # writes .humify/analysis.json
humify plan               # writes .humify/plan.json  (+ .humify/coverage.json)
```

Feed the model three things:

1. **Plan items** — `.humify/plan.json`. Each item carries `signal`, `action`
   (`"quarantine"` or `""`), `applyable` (bool), and a stamped `verification`
   verdict for applyable items. The items are already in humify's rank order
   (`HMF-001` first).
2. **File outliers** — `.humify/analysis.json`. Its `files` array is worst-first
   (lowest `score`); the worst entries plus each file's `metrics` are your
   size/structure outliers.
3. **Recent progress** — `git log --oneline -20` (or similar). Lets the model see
   what you've already cleaned and avoid re-recommending it.

Note: the per-item verdict lives on the **plan item** (`verification` field), written by
plan's coverage stamping — not in `coverage.json`, which holds the raw report. If you ran
`humify plan --no-coverage`, items carry no verdict (the field is omitted); tell the model
to treat such items as `unmeasured`.

## The prompt template

Paste this into an LLM session, then paste the JSON and `git log` after it.

```
You are sequencing a code-cleanup session. humify (a deterministic Go CLI) has
ALREADY ranked these items by signal and severity. Do NOT re-rank them and do NOT
re-score them — its ordering and verdicts are verified facts. Your job is to add the
judgment humify deliberately does not: sequence the work, pick the single prize, and
say where to stop.

I will give you three inputs:
- PLAN ITEMS: humify's .humify/plan.json items (each has id, signal, action,
  applyable, and a verification verdict). Already in rank order.
- FILE OUTLIERS: the worst entries of .humify/analysis.json `files` (lowest score
  first) with their metrics — the biggest / most tangled files.
- RECENT GIT LOG: what I have already changed.

Verdict meanings (weight them):
- behavior-verified: the test suite executed this file. Safest to touch.
- build-only: a measured run that did NOT execute this file. It compiles but its
  behavior was not exercised — verify carefully after touching it.
- unmeasured: no coverage tooling could run. No safety signal at all.
An item is only as trustworthy as its weakest file.

Return EXACTLY these four fields, nothing else:

next_move:        the one item to do first, by HMF-id, and the literal command to run.
                  `humify apply` is DRY RUN by default; it acts only with `--yes`. For an
                  applyable item, emit the command for the step you mean (preview without
                  `--yes`, or apply with `--yes`).
why:              one or two sentences. Favor safe + behavior-verified for the warm-up.
dominant_target:  the one highest-leverage item by HMF-id — the largest reduction in
                  size / complexity / risk — plus one sentence on why it is the prize and,
                  if it is not first, why not. Weigh the FILE OUTLIERS metrics (lines,
                  nesting), not just rank position; the prize is often a manual item with
                  no verdict that sits LOW in the rank. If the safe warm-up IS the
                  highest-leverage item, say so and let next_move and dominant_target be
                  the same id — do not invent a second-best to fill the field.
stop:             where to stop this session and what NOT to touch (e.g. any `manual`
                  item, or any build-only / unmeasured item you are not ready to verify).
```

## Worked example

Suppose `.humify/plan.json` (rank order) and the stamped verdicts look like this, and
`.humify/analysis.json` reports `payments/charge.go` at ~1,800 lines as the worst file:

| id | signal | action | applyable | verification |
|----|--------|--------|-----------|--------------|
| HMF-001 | `stale_file` | `quarantine` | true | `behavior-verified` |
| HMF-002 | `dead_module` | `quarantine` | true | `build-only` |
| HMF-003 | `swallowed_error` | `""` | false | *(none — manual)* |
| HMF-004 | `broad_catch` | `""` | false | *(none — manual)* |
| HMF-005 | `giant_file` | `""` | false | *(none — manual)* |

Why the order is what it is: humify's `order()` pins `stale_file` first and `dead_module`
second; every other signal shares the bottom tier and is ranked by severity weight. So the
two quarantine items lead, and `giant_file` lands at HMF-005 — last — even though it is the
single biggest file in the tree. That is the discriminating case: a reader walking the
ranked list top-down would never nominate HMF-005 as the prize, because rank order alone
buries it.

A good sequenced answer:

```
next_move:        HMF-001 — humify apply --target=HMF-001 --yes
why:              Quarantine stale files is the safe warm-up: action=quarantine,
                  applyable, and behavior-verified — the suite exercised these files,
                  so a passing-check regression would be caught and rolled back. (Drop
                  --yes first if you want to preview the move as a dry run.)
dominant_target:  HMF-005 (the `giant_file` item), even though it is LAST in the rank.
                  The FILE OUTLIERS show payments/charge.go at ~1,800 lines — the single
                  largest reduction in size and complexity available, far more leverage
                  than either quarantine. It is manual (no verdict), so it is the prize
                  to plan, not the move to make blind.
stop:             Apply HMF-001, then stop. Do NOT quarantine HMF-002 yet — it is
                  build-only, so verify carefully before trusting it. Do NOT touch the
                  manual items HMF-003/004/005 this session; humify refuses to touch them,
                  and HMF-005 (the prize) needs a characterization test and a deliberate
                  split, not a one-shot apply.
```

Note what the model did **not** do: it left humify's rank order untouched (HMF-001 still
goes first), but its judgment diverges from a top-down read in a way the ranked list alone
could not produce — it reached *past* four higher-ranked items to name HMF-005 as the prize
on the strength of the FILE OUTLIERS metrics, and it weighed `build-only` against
`behavior-verified` to decide which applyable item is actually safe to apply now. That is
the judgment layer earning its keep: the prize sits at a rank position you could not have
reached by reading the list top-down.

## Guardrails

- **It's judgment, not a humify fact.** The four fields are an LLM's opinion. Humify's
  verdicts and ranks are the verified part; the sequencing is not.
- **Never persist it.** Don't save the answer to a file. The moment you apply one item
  the tree changes and the advice is stale. Re-run the prompt against fresh JSON.
- **Re-run after the tree changes.** Run `humify analyze` and `humify plan` again, then
  re-prompt. Yesterday's prize may already be quarantined.
- **Weight the verdicts.** Quote them to yourself before acting:
  - `behavior-verified` — the file has covered lines; the test suite executed it.
  - `build-only` — a measured report where the suite did **not** execute the file;
    it compiles but its behavior was not exercised.
  - `unmeasured` — no coverage tooling could run; verdicts become unmeasured rather
    than a silent pass.
  - And: **an item is only as trustworthy as its weakest file** — a multi-file item is
    `build-only` if even one of its files was not executed.
