#!/usr/bin/env python3
"""Cross-platform Humify refactor-plan evaluator (macOS/Linux/Windows).

Equivalent to tools/humify-evaluate-plans.ps1. Reads tools/humify-plan-fixtures.json
and scores actual-plans/ against expected-plans/.

The ExpectedDir is load-bearing: each criterion is credited only when the actual plan
agrees with the matching expected baseline, so a missing or hollowed expected file
lowers the score instead of silently passing. Missing actual or expected files are
reported loudly. An identity self-test (actual dir == expected dir) is labeled as a
wiring check, not real-repo accuracy. See tools/humify_selftest.py for the negative
control proving the evaluator can fail.
"""

from __future__ import annotations

import argparse
import math
import sys
from datetime import datetime, timezone
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import humify_lib as h  # noqa: E402


def evaluate(actual_dir: Path, expected_dir: Path):
    rubric = h.load_json(h.repo_root() / "tools" / "humify-plan-fixtures.json")
    plans = rubric["plans"]
    shape_needles = rubric["shapeNeedles"]
    unit_header = rubric["unitHeaderPattern"]
    unit_fields = rubric["unitFieldNeedles"]
    tests_first = rubric["testsFirstNeedles"]
    max_per = rubric.get("maxPerPlan", 3)
    ready_ratio = rubric.get("readyRatio", 0.86)

    results = []
    warnings = []
    for pl in plans:
        actual_path = actual_dir / pl["file"]
        expected_path = expected_dir / pl["file"]
        actual_text = h.read_text(actual_path)
        expected_text = h.read_text(expected_path)

        if actual_text is None:
            warnings.append(f"MISSING actual: {actual_path}")
            results.append({"id": pl["id"], "score": 0, "status": "Missing actual",
                            "notes": f"Create {actual_path}"})
            continue

        notes = []
        if expected_text is None:
            warnings.append(f"MISSING expected (cannot calibrate): {expected_path}")
            notes.append("expected_file_missing=true")

        def plan_checks(text):
            return [
                all(h.contains_any(text, [n]) for n in shape_needles),
                bool(h.regex_search(text, unit_header)),
                all(h.contains_any(text, [n]) for n in unit_fields),
                h.contains_any(text, pl["requiredNeedles"]) and h.contains_any(text, pl["firstSliceNeedles"]),
                h.contains_any(text, tests_first),
            ]

        act = plan_checks(actual_text)
        # ExpectedDir is load-bearing: a criterion counts only when the actual plan
        # AND the expected baseline both establish it. A missing or hollowed expected
        # file makes its criteria unwinnable, so corrupting expected-plans/ changes the
        # score (mirroring the audit evaluator's actual-must-agree-with-expected rule).
        # See humify_selftest.py section 4 for the falsifiability proof.
        exp = plan_checks(expected_text) if expected_text is not None else [False] * len(act)
        checks = [a and e for a, e in zip(act, exp)]
        score = h.score_from_plan_checks(checks)
        status = "Pass" if score == 3 else ("Partial" if score >= 1 else "Fail")

        labels = ["shape", "unit_header", "unit_fields", "scenario_and_first_slice", "tests_first"]
        notes += [f"{lbl}={c}(exp={e},act={a})" for lbl, c, a, e in zip(labels, checks, act, exp)]
        results.append({"id": pl["id"], "score": score, "status": status, "notes": "; ".join(notes)})

    max_score = len(plans) * max_per
    total = sum(r["score"] for r in results)
    return results, total, max_score, ready_ratio, warnings


def write_report(path: Path, results, total, max_score, readiness, actual_dir, expected_dir):
    ts = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    out = [
        "# Humify Plan Score", "",
        f"Generated: {ts}", "",
        f"Actual plan dir: `{actual_dir}`",
        f"Expected plan dir: `{expected_dir}`", "",
        "## Summary", "",
        f"Total score: **{total} / {max_score}**", "",
        f"Readiness: **{readiness}**", "",
        "| Plan | Score | Status | Notes |",
        "| --- | ---: | --- | --- |",
    ]
    for r in results:
        notes = r["notes"].replace("|", "\\|")
        out.append(f"| {r['id']} | {r['score']} / 3 | {r['status']} | {notes} |")
    out += ["", "## Interpretation", "",
            "- 86-100%: Plan calibration ready.",
            "- 67-85%: Usable with human review.",
            "- 40-66%: Needs planning guidance tuning.",
            "- 0-39%: Not reliable yet.",
            "",
            "Note: an identity self-test (actual dir == expected dir) is forced to the "
            "maximum score and only proves evaluator wiring, not real-repo accuracy."]
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("\n".join(out) + "\n", encoding="utf-8")


def main(argv=None):
    root = h.repo_root()
    ap = argparse.ArgumentParser(description="Humify refactor-plan evaluator (cross-platform).")
    ap.add_argument("--actual-plan-dir", default=str(root / "actual-plans"))
    ap.add_argument("--expected-plan-dir", default=str(root / "expected-plans"))
    ap.add_argument("--output", default=str(root.parent / ".humify-runs" / "HUMIFY-PLAN-SCORE.md"))
    ap.add_argument("--ready-threshold", type=int, default=0)
    ap.add_argument("--fail-below-threshold", action="store_true")
    args = ap.parse_args(argv)

    actual_dir = Path(args.actual_plan_dir)
    expected_dir = Path(args.expected_plan_dir)
    results, total, max_score, ready_ratio, warnings = evaluate(actual_dir, expected_dir)

    identity = h.same_dir(actual_dir, expected_dir)
    readiness = h.readiness_label(total, max_score, kind="plan", identity_selftest=identity)
    threshold = args.ready_threshold if args.ready_threshold > 0 else math.ceil(max_score * ready_ratio)

    write_report(Path(args.output), results, total, max_score, readiness, actual_dir, expected_dir)

    for w in warnings:
        print(f"WARNING: {w}", file=sys.stderr)
    print(f"Humify plan score: {total} / {max_score} - {readiness}")
    print(f"Report: {args.output}")

    if args.fail_below_threshold and total < threshold:
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
