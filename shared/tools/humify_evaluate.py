#!/usr/bin/env python3
"""Cross-platform Humify audit evaluator (macOS/Linux/Windows).

Equivalent to tools/humify-evaluate.ps1. Reads the shared rubric in
tools/humify-fixtures.json and scores actual/ audit outputs against expected/.

Unlike the original, the ExpectedDir is load-bearing: a fixture's classification,
machine-shaped confidence, and finding-vs-no-finding expectation are derived from
the matching expected file and the actual must AGREE with them. A missing expected
file is reported loudly instead of silently passing. See tools/humify_selftest.py
for the negative control proving the evaluator can fail.
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
    rubric = h.load_json(h.repo_root() / "tools" / "humify-fixtures.json")
    classifications = rubric["classifications"]
    confidences = rubric["confidences"]
    fixtures = rubric["fixtures"]
    max_per = rubric.get("maxPerFixture", 3)
    ready_ratio = rubric.get("readyRatio", 0.86)

    results = []
    warnings = []
    for fx in fixtures:
        actual_path = actual_dir / fx["file"]
        expected_path = expected_dir / fx["file"]
        actual_text = h.read_text(actual_path)
        expected_text = h.read_text(expected_path)

        if actual_text is None:
            warnings.append(f"MISSING actual: {actual_path}")
            results.append({
                "id": fx["id"], "score": 0, "status": "Missing actual",
                "notes": f"Create {actual_path}", "actual": str(actual_path),
                "expected": str(expected_path),
            })
            continue

        notes = []
        if expected_text is None:
            warnings.append(f"MISSING expected (cannot calibrate): {expected_path}")
            notes.append("expected_file_missing=true")

        exp_class = h.detect_classification(expected_text, classifications)
        exp_conf = h.detect_confidence(expected_text, confidences)
        exp_has_finding = h.has_finding(expected_text)

        act_class = h.detect_classification(actual_text, classifications)
        act_conf = h.detect_confidence(actual_text, confidences)
        act_has_finding = h.has_finding(actual_text)

        classification_ok = exp_class is not None and exp_class == act_class
        confidence_ok = exp_conf is not None and exp_conf == act_conf
        if exp_has_finding:
            finding_ok = act_has_finding and h.has_audit_evidence(actual_text)
        else:
            finding_ok = (expected_text is not None) and (not act_has_finding)
        required_ok = h.contains_any(actual_text, fx["requiredNeedles"])
        safe_ok = h.contains_any(actual_text, fx["safeActionNeedles"])

        unsafe_claim = h.unsafe_generated_claim(actual_text)
        unsafe_generated = bool(fx.get("generatedExclusion")) and act_has_finding
        unsafe = unsafe_claim or unsafe_generated

        checks = [classification_ok, confidence_ok, finding_ok, required_ok and safe_ok]
        score = h.score_from_checks(checks, unsafe=unsafe)

        notes += [
            f"classification={classification_ok}(exp={exp_class},act={act_class})",
            f"confidence={confidence_ok}(exp={exp_conf},act={act_conf})",
            f"finding_or_exclusion={finding_ok}",
            f"safe_next_action={required_ok and safe_ok}",
        ]
        if unsafe_claim:
            notes.append("unsafe_generated_origin_claim=true")
        if unsafe_generated:
            notes.append("generated_file_was_scored=true")

        status = "Pass" if score == 3 else ("Partial" if score >= 1 else "Fail")
        results.append({
            "id": fx["id"], "score": score, "status": status,
            "expected_class": exp_class or "(none)",
            "notes": "; ".join(notes), "actual": str(actual_path),
            "expected": str(expected_path),
        })

    max_score = len(fixtures) * max_per
    total = sum(r["score"] for r in results)
    return results, total, max_score, ready_ratio, warnings


def write_report(path: Path, results, total, max_score, readiness, actual_dir, expected_dir):
    ts = datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    out = [
        "# Humify Score", "",
        f"Generated: {ts}", "",
        f"Actual dir: `{actual_dir}`",
        f"Expected dir: `{expected_dir}`", "",
        "## Summary", "",
        f"Total score: **{total} / {max_score}**", "",
        f"Readiness: **{readiness}**", "",
        "| Fixture | Score | Status | Notes |",
        "| --- | ---: | --- | --- |",
    ]
    for r in results:
        notes = r["notes"].replace("|", "\\|")
        out.append(f"| {r['id']} | {r['score']} / 3 | {r['status']} | {notes} |")
    out += ["", "## Interpretation", "",
            "- 86-100%: Ready to use on a real repo.",
            "- 67-85%: Usable with human review.",
            "- 40-66%: Needs framework tuning.",
            "- 0-39%: Not reliable yet.",
            "",
            "Note: an identity self-test (actual dir == expected dir) is forced to the "
            "maximum score and only proves evaluator wiring, not real-repo accuracy."]
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text("\n".join(out) + "\n", encoding="utf-8")


def main(argv=None):
    root = h.repo_root()
    ap = argparse.ArgumentParser(description="Humify audit evaluator (cross-platform).")
    ap.add_argument("--actual-dir", default=str(root / "actual"))
    ap.add_argument("--expected-dir", default=str(root / "expected"))
    ap.add_argument("--output", default=str(root.parent / ".humify-runs" / "HUMIFY-SCORE.md"))
    ap.add_argument("--ready-threshold", type=int, default=0)
    ap.add_argument("--fail-below-threshold", action="store_true")
    args = ap.parse_args(argv)

    actual_dir = Path(args.actual_dir)
    expected_dir = Path(args.expected_dir)
    results, total, max_score, ready_ratio, warnings = evaluate(actual_dir, expected_dir)

    identity = h.same_dir(actual_dir, expected_dir)
    readiness = h.readiness_label(total, max_score, kind="audit", identity_selftest=identity)
    threshold = args.ready_threshold if args.ready_threshold > 0 else math.ceil(max_score * ready_ratio)

    write_report(Path(args.output), results, total, max_score, readiness, actual_dir, expected_dir)

    for w in warnings:
        print(f"WARNING: {w}", file=sys.stderr)
    print(f"Humify score: {total} / {max_score} - {readiness}")
    print(f"Report: {args.output}")

    if args.fail_below_threshold and total < threshold:
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
