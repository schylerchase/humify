package verify

import "testing"

func TestParseGoProfile(t *testing.T) {
	profile := "mode: set\n" +
		"github.com/me/proj/pkg/a.go:3.10,5.2 1 1\n" +
		"github.com/me/proj/pkg/a.go:6.2,6.20 1 0\n" +
		"github.com/me/proj/pkg/b.go:7.10,9.2 1 0\n"
	files := parseGoProfile(profile, "github.com/me/proj")
	if !files["pkg/a.go"].Covered {
		t.Errorf("a.go has a hit block (count 1) -> must be Covered; got %+v", files["pkg/a.go"])
	}
	if files["pkg/b.go"].Covered {
		t.Errorf("b.go has only a zero-count block -> must NOT be Covered; got %+v", files["pkg/b.go"])
	}
}

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
