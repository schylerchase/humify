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
