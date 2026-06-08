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
	"strings"
	"time"

	"humify-ng/internal/humify/detect"
	"humify-ng/internal/humify/scan"
	"humify-ng/internal/humify/state"
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
	if len(cmds) == 0 {
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
	if exists(filepath.Join(root, "pytest.ini")) || exists(filepath.Join(root, "pyproject.toml")) {
		cmds = append(cmds, command{"test", "python3 -m pytest -q"})
	}
	return cmds
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
