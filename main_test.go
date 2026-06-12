package main

import (
	"testing"

	hplan "github.com/schylerryan/humify/internal/humify/plan"
	"github.com/schylerryan/humify/internal/humify/verify"
)

func TestStampFromCoverage(t *testing.T) {
	p := &hplan.Plan{Items: []hplan.Item{
		{ID: "HMF-001", Applyable: true, Files: []string{"a.go"}},  // covered
		{ID: "HMF-002", Applyable: true, Files: []string{"b.go"}},  // measured, uncovered
		{ID: "HMF-003", Applyable: false, Files: []string{"c.go"}}, // manual: untouched
	}}
	cov := verify.CoverageReport{
		Measured: true,
		Files: map[string]verify.FileCoverage{
			"a.go": {Covered: true, Lines: []int{1}},
			"b.go": {Covered: false},
		},
	}
	stampFromCoverage(p, cov)
	if p.Items[0].Verification != "behavior-verified" {
		t.Errorf("covered item -> behavior-verified, got %q", p.Items[0].Verification)
	}
	if p.Items[1].Verification != "build-only" {
		t.Errorf("uncovered item -> build-only, got %q", p.Items[1].Verification)
	}
	if p.Items[2].Verification != "" {
		t.Errorf("manual item must be left untouched, got %q", p.Items[2].Verification)
	}

	// Unmeasured report must yield an explicit "unmeasured", never empty.
	un := &hplan.Plan{Items: []hplan.Item{{ID: "HMF-001", Applyable: true, Files: []string{"a.go"}}}}
	stampFromCoverage(un, verify.CoverageReport{Measured: false})
	if un.Items[0].Verification != "unmeasured" {
		t.Errorf("unmeasured report -> unmeasured verdict, got %q", un.Items[0].Verification)
	}
}
