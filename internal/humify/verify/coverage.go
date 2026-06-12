package verify

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/schylerryan/humify/internal/humify/state"
)

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

// parseGoProfile turns a `go test -coverprofile` body into per-file coverage,
// keyed by repo-relative path (modulePath stripped). A file is Covered iff any
// block executed (trailing count > 0).
func parseGoProfile(profile, modulePath string) map[string]FileCoverage {
	files := map[string]FileCoverage{}
	sc := bufio.NewScanner(strings.NewReader(profile))
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}
		// <path>:<sl>.<sc>,<el>.<ec> <numStmts> <count>
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			continue
		}
		colon := strings.LastIndex(fields[0], ":")
		if colon < 0 {
			continue
		}
		path := strings.TrimPrefix(fields[0][:colon], modulePath+"/")
		rng := fields[0][colon+1:] // "3.10,5.2"
		fc := files[path]
		if count > 0 {
			fc.Covered = true
			if startLine := leadingLine(rng); startLine > 0 {
				fc.Lines = append(fc.Lines, startLine)
			}
		}
		files[path] = fc
	}
	return files
}

// leadingLine returns the start line number from a coverprofile range "sl.sc,el.ec".
func leadingLine(rng string) int {
	dot := strings.IndexByte(rng, '.')
	if dot < 0 {
		return 0
	}
	n, _ := strconv.Atoi(rng[:dot])
	return n
}

// Provider runs a language's test suite under coverage and reports per-file
// coverage. A provider is used only when Detect is true for the repo.
type Provider interface {
	Name() string
	Detect(root string) bool
	Run(root string) (CoverageReport, error)
}

// providers is the ordered registry. The first whose Detect matches wins.
var providers = []Provider{goProvider{}}

// Coverage produces a coverage report for root by running the first matching
// provider's instrumented test suite. When no provider matches (or it errors), it
// returns an unmeasured report — Measured:false is the honest "couldn't measure"
// signal, never an error the caller must handle to stay truthful.
func Coverage(root string) CoverageReport {
	for _, p := range providers {
		if !p.Detect(root) {
			continue
		}
		rep, err := p.Run(root)
		if err != nil {
			return CoverageReport{Schema: state.Schema, Tool: p.Name(), Measured: false, Files: map[string]FileCoverage{}}
		}
		return rep
	}
	return CoverageReport{Schema: state.Schema, Measured: false, Files: map[string]FileCoverage{}}
}

type goProvider struct{}

func (goProvider) Name() string            { return "go" }
func (goProvider) Detect(root string) bool { return exists(filepath.Join(root, "go.mod")) }

func (goProvider) Run(root string) (CoverageReport, error) {
	prof := filepath.Join(root, ".humify-cover.out")
	defer os.Remove(prof)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-coverprofile="+prof, "./...")
	cmd.Dir = root
	_ = cmd.Run() // a failing/empty test suite still yields a (partial) profile
	data, err := os.ReadFile(prof)
	if err != nil {
		return CoverageReport{}, err
	}
	return CoverageReport{
		Schema:   state.Schema,
		Tool:     "go",
		Measured: true,
		Files:    parseGoProfile(string(data), goModulePath(root)),
	}, nil
}

// goModulePath reads the module path from go.mod, or "" if unreadable.
func goModulePath(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}
