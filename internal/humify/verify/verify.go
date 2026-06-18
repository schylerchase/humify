// Package verify detects a target repo's validation commands (test, build, lint,
// typecheck) and runs them, recording exit codes and output. Running a command
// executes project code, so verify only runs commands inferred from the project's
// own manifests — the same trust model as a build script. It never edits source.
package verify

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/schylerryan/humify/internal/humify/detect"
	"github.com/schylerryan/humify/internal/humify/scan"
	"github.com/schylerryan/humify/internal/humify/state"
)

// DefaultTimeout bounds a single validation command so a hung test run cannot
// freeze verify.
const DefaultTimeout = 5 * time.Minute

// CmdResult records one validation command's outcome.
type CmdResult struct {
	Kind     string `json:"kind"` // test|build|lint|typecheck
	Command  string `json:"command"`
	Ran      bool   `json:"ran"`
	Skipped  bool   `json:"skipped"`
	Reason   string `json:"reason,omitempty"` // why it was skipped, or how it ended
	ExitCode int    `json:"exit_code"`
	Passed   bool   `json:"passed"`
	Summary  string `json:"summary,omitempty"` // tail of combined output
}

// Validation is the full validation report. Validated and Passed together form an
// honest tri-state: Validated reports whether any command actually ran, letting a
// consumer tell a real pass (Validated && Passed) from the vacuous "nothing to run"
// pass (!Validated && Passed). The latter must never be presented as success.
type Validation struct {
	Schema      int         `json:"schema"`
	Tool        string      `json:"tool"`
	Target      string      `json:"target"`
	GeneratedAt string      `json:"generated_at"`
	Commands    []CmdResult `json:"commands"`
	Validated   bool        `json:"validated"` // at least one command actually ran
	Passed      bool        `json:"passed"`    // no command that ran failed (vacuously true if none ran — check Validated)
}

// command is a detected validation command before it runs.
type command struct {
	kind string
	line string
}

// Run detects and runs the target's validation commands. now is injected so the
// timestamp is testable; pass time.Now().
func Run(root string, now time.Time) (Validation, error) {
	res, err := scan.Walk(root, nil)
	if err != nil {
		return Validation{}, err
	}
	project := detect.Detect(res, root)
	v := Validation{
		Schema: state.Schema, Tool: "humify", Target: root,
		GeneratedAt: now.UTC().Format(time.RFC3339), Passed: true,
	}
	cmds := Detect(project, root)
	skipped := DetectSkipped(project)
	if len(cmds) == 0 && len(skipped) == 0 {
		v.Commands = []CmdResult{{Kind: "all", Skipped: true, Reason: "no validation commands detected for this project"}}
		return v, nil // Validated stays false: nothing ran, so this is not a real pass.
	}
	for _, c := range cmds {
		r := runCommand(root, c)
		v.Commands = append(v.Commands, r)
		if r.Ran {
			v.Validated = true
		}
		if r.Ran && !r.Passed {
			v.Passed = false
		}
	}
	// Surface declared <kind>:* scripts verify did not run, so a green verdict that
	// only covers `test` cannot be mistaken for one that also ran `test:unit`. These
	// are Ran=false, so they never flip Validated/Passed — they only widen honesty.
	for _, c := range skipped {
		v.Commands = append(v.Commands, CmdResult{
			Kind: c.kind, Command: c.line, Skipped: true,
			Reason: "declared validation script not run by verify — the verdict's scope excludes it; run it manually to widen coverage",
		})
	}
	return v, nil
}

// Detect infers validation commands from the project's manifests. It is exported
// so callers (and tests) can inspect what would run without running it.
func Detect(project detect.Project, root string) []command {
	var cmds []command
	if exists(filepath.Join(root, "go.mod")) {
		cmds = append(cmds,
			command{"build", "go build ./..."},
			command{"vet", "go vet ./..."},
			command{"test", "go test ./..."},
		)
	}
	cmds = append(cmds, nodeCommands(project)...)
	if exists(filepath.Join(root, "Cargo.toml")) {
		cmds = append(cmds, command{"build", "cargo build"}, command{"test", "cargo test"})
	}
	if isPython(project) && hasPytest(project, root) {
		cmds = append(cmds, command{"test", "python3 -m pytest -q"})
	}
	return cmds
}

// isPython reports whether the project is a Python project, by package manager
// or detected source language. It gates pytest detection so a Go or JS repo —
// whose test files also count toward the language-agnostic Counts.Test — never
// spuriously fires pytest.
func isPython(p detect.Project) bool {
	switch p.PackageManager {
	case "pip", "uv", "poetry":
		return true
	}
	return slices.Contains(p.Stack, "py")
}

// hasPytest reports whether a Python project looks runnable under pytest: either
// an explicit config file, or — the common bare layout — at least one test file
// the scan already classified. Reusing Counts.Test avoids re-globbing and tracks
// the same test-detection logic the rest of Humify uses.
func hasPytest(p detect.Project, root string) bool {
	if exists(filepath.Join(root, "pytest.ini")) ||
		exists(filepath.Join(root, "pyproject.toml")) ||
		exists(filepath.Join(root, "setup.cfg")) {
		return true
	}
	return p.Counts.Test > 0
}

// nodeCommands picks validation scripts a package.json actually declares, so
// verify never invents a script the project does not have.
func nodeCommands(project detect.Project) []command {
	runner := scriptRunner(project.PackageManager)
	if runner == "" {
		return nil
	}
	var cmds []command
	for _, kind := range []string{"build", "lint", "typecheck", "test"} {
		if _, ok := project.Scripts[kind]; ok {
			cmds = append(cmds, command{kind, runner + " run " + kind})
		}
	}
	return cmds
}

// DetectSkipped returns declared validation scripts that Detect deliberately does
// NOT run: npm <kind>:* siblings such as `test:unit`/`test:node` beside `test`.
// nodeCommands runs only the exact-named build/lint/typecheck/test, so without
// this a project whose real coverage lives in test:unit would show a green that
// only ran `test`. Reporting them as skipped keeps the verdict's scope honest.
// Colon is the npm namespacing idiom; a hyphenated `test-ci` is intentionally not
// matched (it is not a recognized sibling convention).
func DetectSkipped(project detect.Project) []command {
	runner := scriptRunner(project.PackageManager)
	if runner == "" {
		return nil
	}
	kinds := map[string]bool{"build": true, "lint": true, "typecheck": true, "test": true}
	var out []command
	for name := range project.Scripts {
		if k, _, ok := strings.Cut(name, ":"); ok && kinds[k] {
			out = append(out, command{k, runner + " run " + name})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].line < out[j].line })
	return out
}

// scriptRunner returns the npm-style runner for a JS package manager, or "".
func scriptRunner(pm string) string {
	switch pm {
	case "npm", "yarn", "pnpm":
		return pm
	default:
		return ""
	}
}

// runCommand executes one validation command in root with a timeout and captures
// a tail of its combined output.
func runCommand(root string, c command) CmdResult {
	r := CmdResult{Kind: c.kind, Command: c.line, Ran: true}
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shell(), shellArg(), c.line)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	r.Summary = tail(string(out), 1500)
	if ctx.Err() == context.DeadlineExceeded {
		r.Reason = "timed out after " + DefaultTimeout.String()
		r.ExitCode = -1
		return r
	}
	if err == nil {
		r.Passed = true
		return r
	}
	if exit, ok := err.(*exec.ExitError); ok {
		r.ExitCode = exit.ExitCode()
	} else {
		r.ExitCode = -1
		r.Reason = err.Error()
	}
	return r
}

func shell() string {
	if runtime.GOOS == "windows" {
		return "cmd"
	}
	return "sh"
}

func shellArg() string {
	if runtime.GOOS == "windows" {
		return "/c"
	}
	return "-c"
}

// tail returns the last max bytes of s, prefixed to show truncation.
func tail(s string, max int) string {
	s = strings.TrimRight(s, "\n")
	if len(s) <= max {
		return s
	}
	return "…" + s[len(s)-max:]
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
