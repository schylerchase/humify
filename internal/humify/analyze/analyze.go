// Package analyze is Humify's read-only review engine. Given a target repository
// it scans the file tree, detects the project shape, measures per-file structural
// metrics, flags AI-slop / maintainability signals with file-and-line evidence,
// and scores five health categories. It never modifies the target — its only
// output is an Analysis value (persisted as .humify/analysis.json by the caller).
package analyze

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/schylerryan/humify/internal/humify/detect"
	"github.com/schylerryan/humify/internal/humify/ignore"
	"github.com/schylerryan/humify/internal/humify/scan"
	"github.com/schylerryan/humify/internal/humify/state"
)

// Tool and Version identify the producer in every Analysis.
const (
	Tool    = "humify"
	Version = "0.1.0"
)

// Config holds the tunable thresholds that turn metrics into findings. Defaults
// are sensible for general repos; humify.config.json may override them.
type Config struct {
	MaxFileLines     int `json:"maxFileLines"`
	MaxFunctionLines int `json:"maxFunctionLines"`
	MaxNestingDepth  int `json:"maxNestingDepth"`
}

// Defaults returns Humify's built-in thresholds.
func Defaults() Config {
	return Config{MaxFileLines: 400, MaxFunctionLines: 60, MaxNestingDepth: 5}
}

// withDefaults fills any unset (zero) threshold from Defaults so a partial config
// file is still usable.
func (c Config) withDefaults() Config {
	d := Defaults()
	if c.MaxFileLines <= 0 {
		c.MaxFileLines = d.MaxFileLines
	}
	if c.MaxFunctionLines <= 0 {
		c.MaxFunctionLines = d.MaxFunctionLines
	}
	if c.MaxNestingDepth <= 0 {
		c.MaxNestingDepth = d.MaxNestingDepth
	}
	return c
}

// Finding is one located maintainability/slop signal.
type Finding struct {
	ID       string `json:"id"`       // stable within a run, e.g. "F007"
	Category string `json:"category"` // readability|maintainability|correctness|testability|efficiency
	Signal   string `json:"signal"`   // e.g. "long_function", "vague_name"
	File     string `json:"file"`
	Line     int    `json:"line"`
	Severity string `json:"severity"` // info|warning|major
	Risk     string `json:"risk"`     // low|medium|high
	Evidence string `json:"evidence"`
	Detail   string `json:"detail"`
}

// FileScore is a per-file roll-up used to rank the worst files.
type FileScore struct {
	Path     string  `json:"path"`
	Lang     string  `json:"lang"`
	Metrics  Metrics `json:"metrics"`
	Findings int     `json:"findings"`
	Score    int     `json:"score"` // 0-100 health; lower is worse
}

// CategoryScores are the five health dimensions plus an overall, each 0-100 where
// higher is healthier (e.g. a high "correctness" score means low correctness risk).
type CategoryScores struct {
	Overall         int `json:"overall"`
	Readability     int `json:"readability"`
	Maintainability int `json:"maintainability"`
	Correctness     int `json:"correctness"`
	Testability     int `json:"testability"`
	Efficiency      int `json:"efficiency"`
}

// Summary holds headline tallies for quick terminal/status rendering.
type Summary struct {
	SourceFiles int            `json:"source_files"`
	Findings    int            `json:"findings"`
	BySeverity  map[string]int `json:"by_severity"`
	ByCategory  map[string]int `json:"by_category"`
	BySignal    map[string]int `json:"by_signal"`
}

// Analysis is the full read-only review of a repository.
type Analysis struct {
	Schema      int             `json:"schema"`
	Tool        string          `json:"tool"`
	Version     string          `json:"version"`
	Target      string          `json:"target"`
	GeneratedAt string          `json:"generated_at"`
	Project     detect.Project  `json:"project"`
	Scores      CategoryScores  `json:"scores"`
	Files       []FileScore     `json:"files"`
	Findings    []Finding       `json:"findings"`
	Summary     Summary         `json:"summary"`
}

// Run performs a full read-only analysis of the repository at root.
func Run(root string, cfg Config) (Analysis, error) {
	cfg = cfg.withDefaults()
	matcher := ignore.New(root)
	res, err := scan.Walk(root, matcher)
	if err != nil {
		return Analysis{}, err
	}
	a := Analysis{
		Schema:      state.Schema,
		Tool:        Tool,
		Version:     Version,
		Target:      root,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Project:     detect.Detect(res, root),
	}
	a.Files, a.Findings = reviewFiles(res, cfg)
	a.Findings = append(a.Findings, staleFindings(res)...)
	assignIDs(a.Findings)
	a.Scores = score(a.Files, a.Findings, a.Project)
	a.Summary = summarize(a.Files, a.Findings)
	return a, nil
}

// reviewFiles measures and inspects every source file, returning per-file scores
// (worst first) and the flat list of findings.
func reviewFiles(res scan.Result, cfg Config) ([]FileScore, []Finding) {
	var files []FileScore
	var findings []Finding
	for _, f := range res.Files {
		if f.IsConfig || f.IsTest || f.Binary || f.Minified || f.Lang == "" {
			continue
		}
		content, err := os.ReadFile(f.Abs)
		if err != nil {
			continue
		}
		text := string(content)
		infos := scanLines(text, f.Lang)
		m := measureFrom(text, infos, f.Lang)
		fileFindings := inspect(f.Path, f.Lang, infos, splitLines(text), m, cfg)
		findings = append(findings, fileFindings...)
		files = append(files, FileScore{
			Path: f.Path, Lang: f.Lang, Metrics: m,
			Findings: len(fileFindings), Score: fileHealth(fileFindings),
		})
	}
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Score != files[j].Score {
			return files[i].Score < files[j].Score // worst (lowest) first
		}
		return files[i].Path < files[j].Path
	})
	return files, findings
}

// staleFindings flags throwaway or empty files across the whole scan (not just
// source) — the highest-confidence, safest cleanup, because quarantining them is
// reversible and apply re-runs validation to confirm nothing depended on them.
func staleFindings(res scan.Result) []Finding {
	var out []Finding
	for _, f := range res.Files {
		if reason := staleReason(f); reason != "" {
			out = append(out, Finding{
				Category: "maintainability", Signal: "stale_file", File: f.Path, Line: 1,
				Severity: "warning", Risk: "low", Evidence: reason,
				Detail:   "This file looks throwaway or empty; quarantine it (reversibly) and re-run validation to confirm nothing depends on it.",
			})
		}
	}
	return out
}

// staleReason returns why a file looks stale, or "" if it does not. It flags only
// unambiguously-throwaway names. Empty files are deliberately NOT flagged — an
// empty file is often significant (e.g. Python's __init__.py, .gitkeep), so
// quarantining it could silently break a package.
func staleReason(f scan.File) string {
	base := strings.ToLower(filepath.Base(f.Path))
	ext := strings.ToLower(filepath.Ext(f.Path))
	switch {
	case ext == ".bak" || ext == ".orig" || ext == ".tmp" || ext == ".old":
		return "throwaway extension " + ext
	case strings.HasSuffix(base, "~"):
		return "editor backup file"
	case strings.Contains(base, " copy") || strings.Contains(base, "-copy") || strings.HasPrefix(base, "untitled"):
		return "looks like an accidental copy"
	}
	return ""
}

// assignIDs gives findings stable, ordered identifiers after sorting them by
// severity then location, so report and plan reference the same F-numbers.
func assignIDs(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		if r := severityRank(findings[j].Severity) - severityRank(findings[i].Severity); r != 0 {
			return r < 0
		}
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		return findings[i].Line < findings[j].Line
	})
	for i := range findings {
		findings[i].ID = fmt.Sprintf("F%03d", i+1)
	}
}

// summarize tallies findings by severity, category, and signal.
func summarize(files []FileScore, findings []Finding) Summary {
	s := Summary{
		SourceFiles: len(files),
		Findings:    len(findings),
		BySeverity:  map[string]int{},
		ByCategory:  map[string]int{},
		BySignal:    map[string]int{},
	}
	for _, f := range findings {
		s.BySeverity[f.Severity]++
		s.ByCategory[f.Category]++
		s.BySignal[f.Signal]++
	}
	return s
}
