package verify

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/schylerryan/humify/internal/humify/state"
)

// istanbulFile is the subset of an Istanbul coverage-final.json entry we read
// (c8/nyc --reporter=json output).
type istanbulFile struct {
	Path         string `json:"path"`
	StatementMap map[string]struct {
		Start struct {
			Line int `json:"line"`
		} `json:"start"`
	} `json:"statementMap"`
	S map[string]int `json:"s"` // statement id -> hit count
}

// jsProvider runs a node project's `npm test` under c8 and parses Istanbul JSON.
// Detect requires a declared test script AND an installed c8 binary; otherwise
// coverage stays unmeasured (honest) rather than guessed or network-fetched.
type jsProvider struct{}

func (jsProvider) Name() string { return "c8" }

func (jsProvider) Detect(root string) bool {
	if !exists(filepath.Join(root, "package.json")) {
		return false
	}
	if !exists(filepath.Join(root, "node_modules", ".bin", "c8")) {
		return false
	}
	return hasNodeTestScript(root)
}

func (jsProvider) Run(root string) (CoverageReport, error) {
	// c8 reports realpaths; resolve root's symlinks so they match (e.g. macOS
	// /var -> /private/var) and parseIstanbul can strip the prefix correctly.
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	covDir := filepath.Join(root, ".humify-cov")
	defer os.RemoveAll(covDir)
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	// --no-install: use the project's installed c8, never fetch from the network.
	cmd := exec.CommandContext(ctx, "npx", "--no-install", "c8",
		"--reporter=json", "--reports-dir="+covDir, "npm", "test")
	cmd.Dir = root
	_ = cmd.Run() // a failing test run still emits a coverage report
	data, err := os.ReadFile(filepath.Join(covDir, "coverage-final.json"))
	if err != nil {
		return CoverageReport{}, err
	}
	files, err := parseIstanbul(data, root)
	if err != nil {
		return CoverageReport{}, err
	}
	return CoverageReport{Schema: state.Schema, Tool: "c8", Measured: true, Files: files}, nil
}

// hasNodeTestScript reports whether package.json declares a "test" script.
func hasNodeTestScript(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return false
	}
	_, ok := pkg.Scripts["test"]
	return ok
}

// parseIstanbul turns a coverage-final.json body into per-file coverage keyed by
// repo-relative path (root stripped). A file is Covered iff any statement hit > 0.
// Entries outside root (dependencies) are ignored.
func parseIstanbul(data []byte, root string) (map[string]FileCoverage, error) {
	var raw map[string]istanbulFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	files := map[string]FileCoverage{}
	for _, f := range raw {
		rel := f.Path
		if r, err := filepath.Rel(root, f.Path); err == nil {
			rel = filepath.ToSlash(r)
		}
		if strings.HasPrefix(rel, "..") {
			continue // outside the repo (a dependency); ignore
		}
		var fc FileCoverage
		for id, hits := range f.S {
			if hits > 0 {
				fc.Covered = true
				if stmt, ok := f.StatementMap[id]; ok {
					fc.Lines = append(fc.Lines, stmt.Start.Line)
				}
			}
		}
		files[rel] = fc
	}
	return files, nil
}
