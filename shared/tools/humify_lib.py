"""Shared helpers for the cross-platform (Python) Humify tooling.

These mirror the logic in the PowerShell scripts so that Windows users (pwsh)
and macOS/Linux users (python3) get identical behavior. The calibration rubric
itself lives in tools/humify-fixtures.json and tools/humify-plan-fixtures.json,
which BOTH the .ps1 and .py tools read, so there is a single source of truth.
"""

from __future__ import annotations

import json
import os
import re
from pathlib import Path


def repo_root() -> Path:
    """Repository root = parent of the tools/ directory holding this file."""
    return Path(__file__).resolve().parent.parent


def read_text(path: os.PathLike | str) -> str | None:
    """Read a UTF-8 file, returning None if it does not exist."""
    p = Path(path)
    if not p.is_file():
        return None
    return p.read_text(encoding="utf-8")


def load_json(path: os.PathLike | str) -> dict:
    return json.loads(Path(path).read_text(encoding="utf-8"))


def contains_any(text: str | None, needles) -> bool:
    """True if any needle appears in text (case-insensitive substring)."""
    if not text or not text.strip():
        return False
    low = text.lower()
    return any(n.lower() in low for n in needles)


def regex_search(text: str | None, pattern: str) -> bool:
    if not text or not text.strip():
        return False
    return re.search(pattern, text, re.IGNORECASE) is not None


# Label cells (a row's first cell) that mark a row as stating the overall verdict,
# e.g. ``| Overall | Clean |``. Matched as whole words so "Category" never counts.
_VERDICT_LABELS = ("overall", "classification", "verdict", "final", "result")


def _row_cells(line: str):
    """Normalized (markdown/quote-stripped, lower-cased) cells of a table row."""
    return [cell.strip().strip("`*_\"' ").strip().lower() for cell in line.split("|")]


def detect_classification(text: str | None, classifications) -> str | None:
    """Return the audit's stated classification verdict.

    Detection is ordered most- to least-specific so that prose or an analysis
    table that merely *mentions* a label in order to rule it out cannot hijack
    the result:

    1. A verdict table row — first cell is a verdict label (``Overall`` /
       ``Classification`` / …) — returns the classification named in that row.
       This wins even when a "candidates considered / ruled out" table appears
       earlier in the document.
    2. Prose verdict line(s), e.g. ``Classification: Clean`` / ``Overall - Clean``.
    3. Any table cell that equals a classification (legacy behavior).
    4. The first classification phrase appearing anywhere (rubric/specificity order).

    Steps 3–4 are the original fallbacks verbatim. The only way the result differs
    from the old logic is the intended one: a label that is merely *mentioned*
    (e.g. ruled out) but not *stated* as the verdict no longer wins. Where several
    verdicts are stated (a multi-section audit), steps 1–2 resolve by the same
    rubric/severity order the old prose fallback used, not by document order.
    """
    if not text:
        return None
    by_lower = {c.lower(): c for c in classifications}

    # 1. Verdict row: first non-empty cell is a verdict label, and a later cell
    #    on that row names a classification.
    for line in text.splitlines():
        if "|" not in line:
            continue
        cells = _row_cells(line)
        nonempty = [c for c in cells if c]
        if not nonempty:
            continue
        label_words = re.findall(r"[a-z]+", nonempty[0])
        if any(v in label_words for v in _VERDICT_LABELS):
            for cell in cells:
                if cell in by_lower:
                    return by_lower[cell]

    # 2. Prose verdict line(s): explicit "<verdict label> <sep> <classification>"
    #    lines. When more than one is stated (a multi-section audit), return the
    #    highest-priority classification in rubric order — matching the legacy
    #    step-4 ordering, so the result does not depend on which line comes first.
    #    The character classes are single, non-overlapping quantifiers (no
    #    "\\**[ \\t]*" adjacency) so a long whitespace run cannot trigger
    #    catastrophic backtracking.
    verdict_line = re.compile(
        r"(?im)^[ \t|*+-]*"
        r"(?:overall[ \t]+classification|final[ \t]+classification|"
        r"overall|final|classification|verdict|result)"
        r"[ \t*_`]*[:=|-]+[ \t*_`]*("
        + "|".join(re.escape(c) for c in classifications)
        + r")\b"
    )
    prose_hits = {m.group(1).lower() for m in verdict_line.finditer(text)}
    if prose_hits:
        for c in classifications:
            if c.lower() in prose_hits:
                return c

    # 3. Any table cell equal to a classification (legacy).
    for line in text.splitlines():
        if "|" not in line:
            continue
        for cell in _row_cells(line):
            if cell in by_lower:
                return by_lower[cell]

    # 4. First classification phrase appearing anywhere (legacy).
    low = text.lower()
    for c in classifications:
        if c.lower() in low:
            return c
    return None


def detect_confidence(text: str | None, confidences) -> str | None:
    """Extract the stated machine-shaped confidence value.

    Tolerant of how the value is written: a connector may be ``:``/``|``/``=``/
    ``is``/``-`` (or absent in a table), and the value may be wrapped in markdown
    (backticks, asterisks, quotes). This is a superset of the strict
    ``confidence: <X>`` form, so it still detects every form the strict regex did.
    """
    if not text:
        return None
    m = re.search(
        r"machine[- ]?shaped\s+confidence\s*(?:is|:|=|\||-|–|—)?\s*"
        r"[`*_\"']*\s*(Not applicable|None|Low|Medium|High)\b",
        text,
        re.IGNORECASE,
    )
    if not m:
        return None
    value = m.group(1).strip().lower()
    for c in confidences:
        if c.lower() == value:
            return c
    return None


def has_finding(text: str | None) -> bool:
    """True if the audit contains at least one finding header like '## H001'."""
    return regex_search(text, r"##\s+H\d+")


def _field_at_line_start(text: str | None, label: str) -> bool:
    """True if ``label`` (e.g. ``"File:"``) begins a line.

    Allows leading whitespace, table-cell pipes, list bullets (``-``/``*``/``+``)
    and markdown emphasis before the label — all folded into one non-overlapping
    character class so a long run of those characters cannot trigger catastrophic
    backtracking. Anchoring at the line start prevents substring false positives
    where a longer word merely *ends* in the label — ``profile:`` → ``File:``,
    ``outlines:`` → ``Lines:``, ``prefix:`` → ``Fix:``.
    """
    if not text or not text.strip():
        return False
    return re.search(
        r"(?im)^[ \t|*+-]*" + re.escape(label),
        text,
    ) is not None


def has_audit_evidence(text: str | None) -> bool:
    """True if a finding carries the required evidence fields (each as a line label)."""
    if not text:
        return False
    has_line = _field_at_line_start(text, "Line:") or _field_at_line_start(text, "Lines:")
    return (
        _field_at_line_start(text, "File:")
        and has_line
        and _field_at_line_start(text, "Symptom:")
        and _field_at_line_start(text, "Causal chain:")
        and _field_at_line_start(text, "Fix:")
    )


def unsafe_generated_claim(text: str | None) -> bool:
    """True if the audit makes an unsupported AI/machine-generated ORIGIN claim.

    Counts only a *bare positive* claim. A disclaimer like "No claim is made that
    this file is AI-generated" is negated and is NOT unsafe — the methodology
    actively asks audits to write such disclaimers. We therefore flag a sentence
    only when the origin phrase has no negation cue before it in that sentence.
    """
    if not text:
        return False
    claim = re.compile(
        r"\b(is|was|appears to be|likely)\s+(ai|machine)[ -]?generated\b", re.IGNORECASE
    )
    negation = re.compile(
        r"\b(no|not|n't|never|without|cannot|rather than|ruled?\s+out|no\s+evidence)\b",
        re.IGNORECASE,
    )
    for sentence in re.split(r"(?<=[.!?])\s+|\n+", text):
        m = claim.search(sentence)
        if m and not negation.search(sentence[: m.start()]):
            return True
    return False


def score_from_checks(checks, *, unsafe: bool = False) -> int:
    """Map passed-check count to a 0-3 score. Any unsafe failure forces 0.

    4 checks -> 3, 3 -> 2, 2 -> 1, else 0. (Audit grid is 4 checks.)
    """
    if unsafe:
        return 0
    passed = sum(1 for c in checks if c)
    if passed >= 4:
        return 3
    if passed == 3:
        return 2
    if passed == 2:
        return 1
    return 0


def score_from_plan_checks(checks) -> int:
    """Plan grid is 5 checks: 5 -> 3, 4 -> 2, >=2 -> 1, else 0."""
    passed = sum(1 for c in checks if c)
    if passed >= 5:
        return 3
    if passed == 4:
        return 2
    if passed >= 2:
        return 1
    return 0


def readiness_label(total: int, max_score: int, *, kind: str, identity_selftest: bool) -> str:
    """Human-readable readiness band.

    When the run is an identity self-test (actual dir == expected dir) the score
    is mathematically forced to the maximum, so it must NOT be reported as
    real-repo accuracy. See HUMIFY-TESTING.md "Self-test honesty".
    """
    if identity_selftest:
        return "Evaluator wiring OK (identity self-test — not a measure of real-repo accuracy)"
    ratio = (total / max_score) if max_score > 0 else 0.0
    if kind == "plan":
        if ratio >= 0.86:
            return "Plan calibration ready"
        if ratio >= 0.67:
            return "Usable with human review"
        if ratio >= 0.40:
            return "Needs planning guidance tuning"
        return "Not reliable yet"
    if ratio >= 0.86:
        return "Ready to use on a real repo"
    if ratio >= 0.67:
        return "Usable with human review"
    if ratio >= 0.40:
        return "Needs framework tuning"
    return "Not reliable yet"


def same_dir(a: os.PathLike | str, b: os.PathLike | str) -> bool:
    try:
        return Path(a).resolve() == Path(b).resolve()
    except OSError:
        return False
