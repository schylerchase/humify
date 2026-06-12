package verify

// FileCoverage records whether a file was executed by the test suite and which
// of its lines were hit. Covered is the load-bearing field; Lines is captured for
// later line-level use (dead-function detection) and is not required by v1.
type FileCoverage struct {
	Covered bool  `json:"covered"`
	Lines   []int `json:"lines,omitempty"`
}

// CoverageReport is the per-file coverage of one test run. Measured is false when
// no coverage tooling could run — verdicts then become Unmeasured, never a silent
// pass.
type CoverageReport struct {
	Schema   int                     `json:"schema"`
	Tool     string                  `json:"tool"` // "go" | "c8" | "nyc" | ""
	Measured bool                    `json:"measured"`
	Files    map[string]FileCoverage `json:"files"`
}

// Verdict is the honest strength of verification for one file.
type Verdict string

const (
	VerdictBehaviorVerified Verdict = "behavior-verified"
	VerdictBuildOnly        Verdict = "build-only"
	VerdictUnmeasured       Verdict = "unmeasured"
)

// VerdictFor returns the verification verdict for a repo-relative file path. An
// unmeasured report yields Unmeasured; a measured report yields BehaviorVerified
// iff the file has covered lines, else BuildOnly (the suite did not execute it).
func (r CoverageReport) VerdictFor(file string) Verdict {
	if !r.Measured {
		return VerdictUnmeasured
	}
	if fc, ok := r.Files[file]; ok && fc.Covered {
		return VerdictBehaviorVerified
	}
	return VerdictBuildOnly
}
