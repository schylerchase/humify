// Command fakeagent is a deterministic stand-in for the LLM agent the autonomous
// driver spawns. It reads a stage prompt on stdin (exactly as a real agent does),
// recovers its role from the prompt header and the artifact path from the
// "Write … to `<path>`" instruction, and writes one valid artifact at that path —
// a fragment, a plan, a plan-check verdict, or (for an executor) a SUMMARY it then
// commits on its slice branch. It lives under testdata/ so `go build ./...` and
// `go test ./...` ignore it; the e2e test builds it explicitly and points
// --agent-cmd at the binary. With --fail-audit it produces nothing for auditor
// prompts, simulating a deterministically-failing agent so the e2e can prove the
// driver's no-progress guard stops rather than loops.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	failAudit, badAudit := false, false
	for _, a := range os.Args[1:] {
		switch a {
		case "--fail-audit":
			failAudit = true
		case "--bad-audit":
			badAudit = true
		}
	}
	data, _ := io.ReadAll(os.Stdin)
	prompt := string(data)
	header := firstLine(prompt)
	id := lastField(afterDash(header))

	switch {
	case strings.Contains(header, "auditor"):
		if failAudit {
			return // produce no fragment → area stays pending → driver's no-progress guard fires
		}
		if badAudit {
			// A fragment that is written but INVALID (no severity) — fragment.Validate
			// rejects it, so BuildPlan.Done never moves. Proves the validated-state
			// progress guard trips 'stuck' instead of churning a byte fingerprint.
			writeFile(pathAfter(prompt, "Write your fragment to"),
				fmt.Sprintf(`{"area_id":%q,"findings":[{"id":%q,"title":"x","file":"src/util.go","line":1,"detail":"d","refs":[]}]}`, id, id+"-1"))
			return
		}
		writeFile(pathAfter(prompt, "Write your fragment to"),
			fmt.Sprintf(`{"area_id":%q,"findings":[{"id":%q,"title":"god function","severity":"warning","file":"src/util.go","line":1,"detail":"split it","refs":[]}]}`, id, id+"-1"))
	case strings.Contains(header, "checker"):
		writeFile(pathAfter(prompt, "Write your verdict to"),
			fmt.Sprintf(`{"area_id":%q,"issues":[]}`, id)) // empty issues accepts the plan
	case strings.Contains(header, "planner"):
		writeFile(pathAfter(prompt, "Write the plan to"),
			"# Plan for "+id+"\n\n1. **what & where**: src/util.go — extract helper.\n   **addresses**: finding 1.\n   **characterization test**: lock current output.\n   **risk**: none.\n")
	case strings.Contains(header, "executor"):
		writeFile(pathAfter(prompt, "Write a SUMMARY of what you actually did to"),
			"# Summary "+id+"\n\nExtracted helper; added characterization test.\n")
		git("add", "-A")
		git("commit", "-m", "exec "+id)
	default:
		fmt.Fprintf(os.Stderr, "fakeagent: unrecognized prompt header %q\n", header)
		os.Exit(1)
	}
}

// pathAfter returns the first backtick-delimited token following marker, the
// project- or worktree-relative path the prompt tells the agent to write.
func pathAfter(s, marker string) string {
	i := strings.Index(s, marker)
	if i < 0 {
		return ""
	}
	rest := s[i+len(marker):]
	a := strings.IndexByte(rest, '`')
	if a < 0 {
		return ""
	}
	rest = rest[a+1:]
	b := strings.IndexByte(rest, '`')
	if b < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:b])
}

func writeFile(rel, content string) {
	if rel == "" {
		fmt.Fprintln(os.Stderr, "fakeagent: no output path found in prompt")
		os.Exit(1)
	}
	if dir := filepath.Dir(rel); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	if err := os.WriteFile(rel, []byte(content), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "fakeagent:", err)
		os.Exit(1)
	}
}

// git runs a git subcommand in the current working directory (the worktree the
// executor was spawned in). Output is surfaced for debugging; an error is fatal
// so a failed commit shows up as a stuck/blocked driver rather than a silent pass.
func git(args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "fakeagent git", args, ":", err)
		os.Exit(1)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// afterDash returns the text after the em-dash in a header line, e.g.
// "# Humify auditor — area 01-src" → "area 01-src".
func afterDash(s string) string {
	if i := strings.Index(s, "— "); i >= 0 {
		return strings.TrimSpace(s[i+len("— "):])
	}
	return s
}

func lastField(s string) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}
