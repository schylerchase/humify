#!/usr/bin/env python3
"""Humify evaluator self-test + NEGATIVE CONTROL (cross-platform).

The identity self-tests (expected vs expected) only prove wiring: by construction
they score the maximum. This script ADDS a negative control — it feeds deliberately
wrong audit/plan outputs and asserts the score drops below the readiness threshold
and the evaluator exits non-zero. That is the proof the evaluator can actually fail,
which an identity check can never demonstrate.

Run:  python3 ./tools/humify_selftest.py
Exit: 0 if all assertions hold, 1 otherwise.
"""

from __future__ import annotations

import math
import sys
import tempfile
import time
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import humify_lib as h  # noqa: E402
import humify_evaluate as ev  # noqa: E402
import humify_evaluate_plans as evp  # noqa: E402

ROOT = h.repo_root()
JUNK = "# Wrong output\n\nClassification: not a real classification\n\nNothing useful here.\n"
failures = []


def check(label, cond):
    print(f"  [{'PASS' if cond else 'FAIL'}] {label}")
    if not cond:
        failures.append(label)


def make_junk_dir(names):
    d = Path(tempfile.mkdtemp(prefix="humify-neg-"))
    for n in names:
        (d / n).write_text(JUNK, encoding="utf-8")
    return d


def main():
    print("Humify self-test")

    print("\n1) Identity self-tests (wiring only — must hit the maximum):")
    _, a_total, a_max, a_ratio, _ = ev.evaluate(ROOT / "expected", ROOT / "expected")
    check(f"audit identity == {a_max}/{a_max}", a_total == a_max)
    _, p_total, p_max, p_ratio, _ = evp.evaluate(ROOT / "expected-plans", ROOT / "expected-plans")
    check(f"plan identity == {p_max}/{p_max}", p_total == p_max)

    print("\n2) Negative control (deliberately wrong outputs — MUST score low & fail):")
    a_rubric = h.load_json(ROOT / "tools" / "humify-fixtures.json")
    a_junk = make_junk_dir([f["file"] for f in a_rubric["fixtures"]])
    _, na_total, na_max, _, _ = ev.evaluate(a_junk, ROOT / "expected")
    a_threshold = math.ceil(na_max * a_ratio)
    check(f"audit junk score {na_total} < threshold {a_threshold}", na_total < a_threshold)
    check("audit junk score < max (evaluator is falsifiable)", na_total < na_max)

    p_rubric = h.load_json(ROOT / "tools" / "humify-plan-fixtures.json")
    p_junk = make_junk_dir([p["file"] for p in p_rubric["plans"]])
    _, np_total, np_max, _, _ = evp.evaluate(p_junk, ROOT / "expected-plans")
    p_threshold = math.ceil(np_max * p_ratio)
    check(f"plan junk score {np_total} < threshold {p_threshold}", np_total < p_threshold)
    check("plan junk score < max (evaluator is falsifiable)", np_total < np_max)

    print("\n3) ExpectedDir is load-bearing (a single flipped classification is detected):")
    one = make_junk_dir([])
    # Copy the real expected audits, then flip ONE classification.
    for f in a_rubric["fixtures"]:
        src = h.read_text(ROOT / "expected" / f["file"])
        if src is not None:
            (one / f["file"]).write_text(src, encoding="utf-8")
    target = ROOT / "expected" / "machine-shaped-audit.md"
    flipped = h.read_text(target).replace("Machine-shaped readability risk", "Clean")
    (one / "machine-shaped-audit.md").write_text(flipped, encoding="utf-8")
    _, flip_total, flip_max, _, _ = ev.evaluate(one, ROOT / "expected")
    check(f"flipped-classification score {flip_total} < max {flip_max}", flip_total < flip_max)

    print("\n4) Plan ExpectedDir is load-bearing (hollowing one expected plan is detected):")
    p_gold = ROOT / "expected-plans"
    mod_exp = make_junk_dir([])
    for p in p_rubric["plans"]:
        src = h.read_text(p_gold / p["file"])
        if src is not None:
            (mod_exp / p["file"]).write_text(src, encoding="utf-8")
    # Hollow ONE expected plan; a perfect actual (the gold plans) must now lose that
    # plan's credit, proving the expected baseline content actually affects the score.
    (mod_exp / p_rubric["plans"][0]["file"]).write_text(JUNK, encoding="utf-8")
    _, lb_total, lb_max, _, _ = evp.evaluate(p_gold, mod_exp)
    check(f"hollowed-expected-plan score {lb_total} < max {lb_max}", lb_total < lb_max)

    print("\n5) Parser robustness (latent real-repo hazards the current baselines hide):")
    cls = a_rubric["classifications"]
    # A "candidates considered / ruled out" analysis table appearing BEFORE the
    # verdict row must not hijack classification detection (the verdict wins).
    hijack = ("## Candidates considered\n\n"
              "| Candidate | Ruled out |\n| --- | --- |\n"
              "| High-risk refactor candidate | yes (has tests) |\n\n"
              "## Required classification\n\n"
              "| Category | Expected |\n| --- | --- |\n| Overall | Clean |\n")
    check("ruled-out analysis table does not hijack the verdict row",
          h.detect_classification(hijack, cls) == "Clean")
    # In prose, an explicit verdict line beats a label that is merely mentioned.
    prose = "Considered `High-risk refactor candidate`; ruled it out.\n\nClassification: Clean\n"
    check("prose verdict line beats a ruled-out mention",
          h.detect_classification(prose, cls) == "Clean")
    # Evidence detection must not be satisfied by look-alike words that merely END
    # in a field label: profile:->File:, outlines:->Lines:, prefix:->Fix:.
    lookalike = ("Performance profile: nominal\nFunction outlines: three\n"
                 "Symptom: slow\nCausal chain: a -> b\nConfig prefix: app_\n")
    check("evidence check rejects substring look-alikes (profile:/outlines:/prefix:)",
          h.has_audit_evidence(lookalike) is False)
    # ...but a properly labeled finding still registers as evidence.
    real_finding = h.read_text(ROOT / "expected" / "machine-shaped-audit.md")
    check("a properly labeled finding still registers as evidence",
          h.has_audit_evidence(real_finding) is True)
    # Multi-section audits: when several verdicts are stated, the worst-of-present
    # governs regardless of which section is written first (rubric/severity order),
    # so the classification check does not silently flip on ordering.
    multi = ("### a.py\nClassification: Clean\n\n"
             "### b.py\nClassification: High-risk refactor candidate\n")
    check("multi-section verdict resolves to worst-of-present, not first-stated",
          h.detect_classification(multi, cls) == "High-risk refactor candidate")
    # The line-anchored parsers must stay (near-)linear: a long run of whitespace
    # must not trigger catastrophic regex backtracking (ReDoS). The fixed parsers
    # finish in well under a millisecond; a backtracking regex takes seconds.
    pad = " " * 1200
    t0 = time.perf_counter(); h.has_audit_evidence(pad + "x"); dt_ev = time.perf_counter() - t0
    t0 = time.perf_counter(); h.detect_classification("classification:" + pad, cls); dt_cl = time.perf_counter() - t0
    check(f"parsers stay linear on long whitespace (evidence {dt_ev*1000:.0f}ms, "
          f"classify {dt_cl*1000:.0f}ms; must be < 500ms each)",
          dt_ev < 0.5 and dt_cl < 0.5)

    print()
    if failures:
        print(f"SELF-TEST FAILED: {len(failures)} assertion(s) failed: {failures}")
        return 1
    print("SELF-TEST PASSED: identity wiring holds AND the evaluator demonstrably fails on bad input.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
