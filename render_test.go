package main

import (
	"io"
	"os"
	"strings"
	"testing"

	analyze "github.com/schylerryan/humify/internal/humify/analyze"
	hplan "github.com/schylerryan/humify/internal/humify/plan"
	verify "github.com/schylerryan/humify/internal/humify/verify"
)

// captureStatus runs printStatus with stdout redirected and returns what it printed.
func captureStatus(t *testing.T, a analyze.Analysis, haveA bool, p hplan.Plan, haveP bool, v verify.Validation, haveV bool) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	printStatus("/repo", a, haveA, p, haveP, v, haveV)
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}

// capture runs fn with stdout redirected and returns what it printed.
func capture(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out)
}

func vCmd(kind string, passed bool, exit int) verify.CmdResult {
	return verify.CmdResult{Kind: kind, Command: kind + " cmd", Ran: true, Passed: passed, ExitCode: exit}
}

func vReport(cmds ...verify.CmdResult) verify.Validation {
	v := verify.Validation{Commands: cmds, Validated: len(cmds) > 0, Passed: true}
	for _, c := range cmds {
		if c.Ran && !c.Passed {
			v.Passed = false
		}
	}
	return v
}

// TestPrintBaselineDelta_NamesRegression: a kind that passed in the baseline and
// cleanly fails after the edit is the AI's regression — it must be named as such,
// not waved through as ambient.
func TestPrintBaselineDelta_NamesRegression(t *testing.T) {
	snap := verify.BaselineSnapshot{Result: vReport(vCmd("test", true, 0))}
	post := vReport(vCmd("test", false, 1))
	out := capture(t, func() { printBaselineDelta(post, snap, false) })
	low := strings.ToLower(out)
	if !strings.Contains(low, "newly failed") {
		t.Errorf("a previously-passing kind that now fails must be named a regression:\n%s", out)
	}
	if !strings.Contains(low, "your change") && !strings.Contains(low, "caused by") {
		t.Errorf("regression must be attributed to the change, not ambient:\n%s", out)
	}
}

// TestPrintBaselineDelta_AmbientNotBlamed is the exact confusion the blind canary
// hit: a check failing in a deps-less checkout was indistinguishable from a
// regression. A pre-existing failure must read as ambient, never as the change.
func TestPrintBaselineDelta_AmbientNotBlamed(t *testing.T) {
	snap := verify.BaselineSnapshot{Result: vReport(vCmd("build", false, 1))}
	post := vReport(vCmd("build", false, 1))
	out := capture(t, func() { printBaselineDelta(post, snap, false) })
	low := strings.ToLower(out)
	if !strings.Contains(low, "already failing") && !strings.Contains(low, "ambient") {
		t.Errorf("a pre-existing failure must be marked ambient:\n%s", out)
	}
	if strings.Contains(low, "newly failed") {
		t.Errorf("an ambient failure must NOT be reported as newly failed:\n%s", out)
	}
}

// TestPrintBaselineDelta_IndeterminateShown: Delta drops ExitCode<0 kinds, so the
// renderer must surface them explicitly or a flaky/uninstalled command silently
// vanishes and the report reads as clean.
func TestPrintBaselineDelta_IndeterminateShown(t *testing.T) {
	snap := verify.BaselineSnapshot{Result: vReport(vCmd("test", true, 0))}
	post := vReport(vCmd("test", false, -1)) // timed out / failed to launch
	out := capture(t, func() { printBaselineDelta(post, snap, false) })
	if !strings.Contains(strings.ToLower(out), "could not compare") {
		t.Errorf("an indeterminate post kind must be surfaced, not dropped:\n%s", out)
	}
}

// TestPrintNoBaselineIsLoud: a quiet degrade is the original gap wearing a success
// message. The no-baseline path must tell the caller to capture one first.
func TestPrintNoBaselineIsLoud(t *testing.T) {
	out := capture(t, func() { printNoBaseline() })
	if !strings.Contains(out, "--save-baseline") {
		t.Errorf("the no-baseline degrade must name --save-baseline:\n%s", out)
	}
}

// TestPrintStatus_FlagsStalePlan covers ROADMAP #10: plan.AnalysisAt was written but
// read nowhere, so a plan built from an analysis that has since been re-run showed
// its scores next to a stale plan with no warning.
func TestPrintStatus_FlagsStalePlan(t *testing.T) {
	a := analyze.Analysis{GeneratedAt: "2026-06-14T00:00:00Z", Summary: analyze.Summary{Findings: 3}}
	p := hplan.Plan{AnalysisAt: "2026-06-13T00:00:00Z", GeneratedAt: "2026-06-13T00:00:01Z", Items: []hplan.Item{{ID: "HMF-001"}}}
	out := captureStatus(t, a, true, p, true, verify.Validation{}, false)
	if !strings.Contains(out, "stale") {
		t.Errorf("a plan built from an older analysis must be flagged stale:\n%s", out)
	}
}

func TestPrintStatus_NoStaleWhenMatched(t *testing.T) {
	a := analyze.Analysis{GeneratedAt: "2026-06-14T00:00:00Z"}
	p := hplan.Plan{AnalysisAt: "2026-06-14T00:00:00Z", GeneratedAt: "2026-06-14T00:00:01Z", Items: []hplan.Item{{ID: "HMF-001"}}}
	out := captureStatus(t, a, true, p, true, verify.Validation{}, false)
	if strings.Contains(out, "stale") {
		t.Errorf("a plan matching the current analysis must not be flagged stale:\n%s", out)
	}
}

// TestStatusView_PresenceFlags covers ROADMAP #14: empty state marshalled to a bare
// {}, so a consumer could not tell "absent" from "empty". The presence booleans must
// always be present.
func TestStatusView_PresenceFlags(t *testing.T) {
	empty := statusView(analyze.Analysis{}, false, hplan.Plan{}, false, verify.Validation{}, false)
	for _, k := range []string{"have_analysis", "have_plan", "have_validation"} {
		if v, ok := empty[k].(bool); !ok || v {
			t.Errorf("%s must be present and false on empty state, got %v (present=%v)", k, empty[k], ok)
		}
	}
	for _, k := range []string{"analysis", "plan", "validation"} {
		if _, ok := empty[k]; ok {
			t.Errorf("%s payload must be absent when not present", k)
		}
	}
	withA := statusView(analyze.Analysis{}, true, hplan.Plan{}, false, verify.Validation{}, false)
	if withA["have_analysis"] != true {
		t.Error("have_analysis must be true when analysis is present")
	}
	if _, ok := withA["analysis"]; !ok {
		t.Error("analysis payload must be present when haveA")
	}
}
