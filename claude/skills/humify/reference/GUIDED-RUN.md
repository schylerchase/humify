# Humify Guided Run

Humify is worth using because it shows and explains what it finds. The audit and plan files are a byproduct. The product is the running explanation in the chat: what is wrong, where, why it matters, and what to do next.

Run every audit as a guided session. Narrate briefly, present findings clearly, and check in at the gates below.

## Principles

- Lead with the point. Say what you found before how you found it.
- Show evidence. Every finding cites a file and line. No evidence, no finding.
- Explain why it matters. Tie each finding to a real cost: a likely bug, a maintenance trap, a safety risk, or a change that could break behavior.
- Show your judgment. Name the things that looked suspicious but are fine, and why. Cleared items build trust.
- Do not dump files. Writing `HUMIFY-AUDIT.md` is not the same as explaining the audit. Present in the chat, then save the artifact.
- Stay read-only until the user opts in. Check in before planning, and again before any edit.

## Narration

Before each stage, say in one line what you are about to do and why. Keep it short.

- "Mapping the repo first so we know what is real code versus generated output."
- "Scoring the three hottest areas now. I will explain what drove each score."

## Stage presentation

### After the map

Give the lay of the land in a few lines:

- What the project is: languages, frameworks, entry points, rough size.
- What you excluded and why: generated, vendored, build output, lockfiles.
- Where you will look hardest, and why.

### After the heatmap

Present the scored hotspots as a short list. For each one:

- Area or file.
- Score and confidence.
- One line on what drove the score.

Do not give a high-confidence score from a filename alone. Say when you only sampled.

### After the audit

This is the core of the run. Present findings grouped by area, worst first. Use this shape for each:

```
[H001] <short title>   SEVERITY: HIGH | MEDIUM | LOW | COSMETIC
File: path/to/file.ext:120-148
What you see: the symptom a maintainer or user notices
Why it happens: the cause, trigger to effect, no hand-waving
Machine-shaped confidence: None | Low | Medium | High
Why it matters: the concrete cost or risk
Safe next step: the minimal change; tests first if the behavior is risky
```

Then a short cleared section, so the user sees the judgment calls:

```
Cleared (considered, not flagged):
- <area>: looked like <X>, but it is fine because <Y>.
```

Then the Refactor Readiness Verdict in plain words:

- Can we safely change this code yet? Gate open or closed, and why.
- The one thing you would do first.

### Checkpoint

Stop and ask. Offer clear options, for example:

- "Want a tests-first refactor plan for the top findings?"
- "Want me to go deeper on any finding?"
- "Or stop here with the audit?"

Do not start planning or editing without an answer.

### After the plan

Present the slices in order. For the first slice, explain why it is safe to do first, what behavior it protects, and how to roll it back. Name the gates that are open and any that are not. Check in before executing any slice.

### During a refactor slice

Only with explicit opt-in and an open gate. Narrate the slice, keep it no-commit, show before and after, and verify with tests or a concrete check. Stop and report after each slice.

## Tone

Direct and concrete. Short sentences. Explain the reasoning, not just the verdict. Do not pad. The user should finish each stage knowing what was found, why it matters, and what choices they have next.
