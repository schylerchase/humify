package verify

import "testing"

func report(measured bool, files map[string]FileCoverage) CoverageReport {
	return CoverageReport{Schema: 1, Measured: measured, Files: files}
}

func TestVerdictFor(t *testing.T) {
	covered := map[string]FileCoverage{"a.go": {Covered: true, Lines: []int{3}}, "b.go": {Covered: false}}
	tests := []struct {
		name string
		rep  CoverageReport
		file string
		want Verdict
	}{
		{"measured+covered -> behavior-verified", report(true, covered), "a.go", VerdictBehaviorVerified},
		{"measured+uncovered -> build-only", report(true, covered), "b.go", VerdictBuildOnly},
		{"measured+absent -> build-only", report(true, covered), "z.go", VerdictBuildOnly},
		{"unmeasured -> unmeasured", report(false, nil), "a.go", VerdictUnmeasured},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rep.VerdictFor(tt.file); got != tt.want {
				t.Errorf("VerdictFor(%q) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}
