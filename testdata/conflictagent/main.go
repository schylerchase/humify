// Command conflictagent is a test agent that behaves like fakeagent for
// audit/plan/check stages but, as an executor, writes a real mutation to a
// shared file so that the second area's merge produces a git conflict.
// This exercises the AbortMerge path in the merge barrier.
//
// The shared file it touches is .humify/CONFLICT_PROBE — a file that every
// executor writes with different content, guaranteed to conflict on merge 2+.
// Using a .humify/ file (not source) keeps the conflict contained and avoids
// the no-deletions gate.
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
	data, _ := io.ReadAll(os.Stdin)
	prompt := string(data)
	header := firstLine(prompt)
	id := lastField(afterDash(header))

	switch {
	case strings.Contains(header, "auditor"):
		writeFile(pathAfter(prompt, "Write your fragment to"),
			fmt.Sprintf(`{"area_id":%q,"findings":[{"id":%q,"title":"stub finding","severity":"info","file":"README.md","line":1,"detail":"stub","refs":[]}]}`, id, id+"-1"))
	case strings.Contains(header, "checker"):
		writeFile(pathAfter(prompt, "Write your verdict to"),
			fmt.Sprintf(`{"area_id":%q,"issues":[]}`, id))
	case strings.Contains(header, "planner"):
		writeFile(pathAfter(prompt, "Write the plan to"),
			"# Plan for "+id+"\n\n1. **what & where**: README.md line 1 — add comment.\n   **addresses**: finding 1.\n   **characterization test**: none.\n   **risk**: none.\n")
	case strings.Contains(header, "executor"):
		// Write a conflicting mutation: every area writes a different line to the
		// same shared file. Area 01 writes "area: 01-root", area 02 writes
		// "area: 02-scripts", etc. The second merge WILL conflict.
		probeRel := filepath.Join(".humify", "CONFLICT_PROBE")
		writeFile(probeRel, "area: "+id+"\n")
		writeFile(pathAfter(prompt, "Write a SUMMARY of what you actually did to"),
			"# Summary "+id+"\n\nWrote conflict probe.\n")
		git("add", "-A")
		git("commit", "-m", "exec "+id)
	default:
		fmt.Fprintf(os.Stderr, "conflictagent: unrecognized prompt header %q\n", header)
		os.Exit(1)
	}
}

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
		fmt.Fprintln(os.Stderr, "conflictagent: no output path found in prompt")
		os.Exit(1)
	}
	if dir := filepath.Dir(rel); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	if err := os.WriteFile(rel, []byte(content), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "conflictagent:", err)
		os.Exit(1)
	}
}

func git(args ...string) {
	cmd := exec.Command("git", args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "conflictagent git", args, ":", err)
		os.Exit(1)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

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
