package analyze

import "github.com/schylerryan/humify/internal/humify/detect"

// Penalty weights per severity. Scores are "health" out of 100: a clean repo
// stays near 100, and each finding subtracts from its category.
const (
	penaltyMajor   = 10
	penaltyWarning = 5
	penaltyInfo    = 2
)

// severityRank orders severities (higher is worse) for sorting findings.
func severityRank(s string) int {
	switch s {
	case "major":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

// penalty is the score cost of one finding by severity.
func penalty(severity string) int {
	switch severity {
	case "major":
		return penaltyMajor
	case "warning":
		return penaltyWarning
	default:
		return penaltyInfo
	}
}

// sev classifies a metric that already exceeded its threshold: "major" once it is
// majorMult× over, otherwise "warning".
func sev(value, threshold, majorMult int) string {
	if value >= threshold*majorMult {
		return "major"
	}
	return "warning"
}

// score turns findings and project shape into the five health categories plus an
// overall. Readability/maintainability/correctness come from their findings;
// testability comes from test coverage and complexity; efficiency is a deliberately
// conservative complexity proxy (Humify does not guess at runtime performance).
func score(files []FileScore, findings []Finding, project detect.Project) CategoryScores {
	byCat := map[string]int{}
	for _, f := range findings {
		byCat[f.Category] += penalty(f.Severity)
	}
	s := CategoryScores{
		Readability:     clamp(100 - byCat["readability"]),
		Maintainability: clamp(100 - byCat["maintainability"]),
		Correctness:     clamp(100 - byCat["correctness"]),
		Testability:     testability(project, countSignal(findings, SignalLongFunction)),
		Efficiency:      efficiency(findings),
	}
	s.Overall = clamp(weighted(s))
	return s
}

// testability rewards a healthy test-to-source ratio and lightly penalizes long
// functions, which are hard to test in isolation.
func testability(project detect.Project, longFuncs int) int {
	src := project.Counts.Source
	if src == 0 {
		return 100
	}
	ratio := float64(project.Counts.Test) / float64(src)
	if ratio > 1 {
		ratio = 1
	}
	return clamp(20 + int(ratio*80) - capPenalty(longFuncs*2, 20))
}

// efficiency starts healthy and subtracts a small amount for complexity signals
// (long functions, deep nesting). It never claims a runtime cost it cannot prove.
func efficiency(findings []Finding) int {
	complexity := countSignal(findings, SignalLongFunction) + countSignal(findings, SignalDeepNesting)
	return clamp(100 - capPenalty(complexity*2, 30))
}

// weighted blends the five categories into an overall score, weighting the
// human-facing dimensions (readability, maintainability) most.
func weighted(s CategoryScores) int {
	total := 0.25*float64(s.Readability) +
		0.25*float64(s.Maintainability) +
		0.20*float64(s.Correctness) +
		0.15*float64(s.Testability) +
		0.15*float64(s.Efficiency)
	return int(total + 0.5)
}

// fileHealth scores a single file from its own findings.
func fileHealth(findings []Finding) int {
	total := 0
	for _, f := range findings {
		total += penalty(f.Severity)
	}
	return clamp(100 - total)
}

// countSignal counts findings carrying a given signal.
func countSignal(findings []Finding, signal string) int {
	n := 0
	for _, f := range findings {
		if f.Signal == signal {
			n++
		}
	}
	return n
}

// capPenalty caps a penalty at max.
func capPenalty(p, max int) int {
	if p > max {
		return max
	}
	return p
}

// clamp bounds a score to [0, 100].
func clamp(n int) int {
	if n < 0 {
		return 0
	}
	if n > 100 {
		return 100
	}
	return n
}
