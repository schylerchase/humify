package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	hexec "github.com/schylerryan/humify/internal/exec"
	"github.com/schylerryan/humify/internal/handoff"
	"github.com/schylerryan/humify/internal/layout"
	"github.com/schylerryan/humify/internal/output"
)

// cmdPatchlog rolls up every executed area into PATCHLOG.md — a deterministic,
// no-agent summary of what the run changed. The "## Patched areas" list names
// each area on its own line, which is what package state reads to flip an area
// to "patched". Per-area details carry the merge commit (from the commit log)
// and the SUMMARY's first line, so the log is traceable back to git history.
func untanglePatchlog(opts options) int {
	root := opts.root
	if root == "" {
		if found, ok := layout.FindRoot(opts.path); ok {
			root = found
		} else {
			root = "."
		}
	}
	ids, err := layout.DiscoverAreas(root)
	if err != nil {
		return fail(opts, "read_error", exitError, "cannot read areas: "+err.Error())
	}
	var executed []string
	for _, id := range ids {
		if fileExists(layout.AreaSummary(root, id)) {
			executed = append(executed, id)
		}
	}
	if len(executed) == 0 {
		return fail(opts, "nothing_executed", exitError,
			"no executed areas to roll up — run `humify execute` first")
	}
	sort.Strings(executed)

	commits, err := hexec.LoadCommits(root)
	if err != nil {
		return fail(opts, "manifest_error", exitError, "load commit log: "+err.Error())
	}
	shaByArea := map[string]string{}
	for _, c := range commits {
		shaByArea[c.SliceID] = c.CommitSHA
	}

	doc := renderPatchlog(root, executed, shaByArea)
	if err := os.WriteFile(layout.PatchlogFile(root), []byte(doc), 0o644); err != nil {
		return fail(opts, "write_error", exitError, "write PATCHLOG.md: "+err.Error())
	}
	saveHandoff(root, handoff.Handoff{Stage: "patchlog", Action: "proceed",
		NextCommand: "humify status", Note: "pipeline complete — patchlog rolled up"})
	if opts.json {
		output.EmitJSON(os.Stdout, output.Result{Ok: true, ReasonCode: "ok",
			Data: map[string]any{"patched": executed}})
		return exitOK
	}
	fmt.Printf("rolled up %d executed area(s) -> %s\n", len(executed), layout.PatchlogFile(root))
	fmt.Printf("patched: %s\n", strings.Join(executed, " "))
	return exitOK
}

func renderPatchlog(root string, executed []string, shaByArea map[string]string) string {
	var b strings.Builder
	b.WriteString("# Humify Patchlog\n\n## Patched areas\n\n")
	// Bare id-per-line coverage rows: package state matches these to flip status.
	for _, id := range executed {
		fmt.Fprintf(&b, "- %s\n", id)
	}
	b.WriteString("\n## Details\n")
	for _, id := range executed {
		fmt.Fprintf(&b, "\n### %s\n", id)
		if sha := shaByArea[id]; sha != "" {
			fmt.Fprintf(&b, "merge commit: %s\n", sha)
		}
		if line := firstSummaryLine(layout.AreaSummary(root, id)); line != "" {
			fmt.Fprintf(&b, "summary: %s\n", line)
		}
	}
	return b.String()
}

// firstSummaryLine returns the first non-empty, non-heading line of a SUMMARY,
// flattened to a single line for the roll-up.
func firstSummaryLine(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		return t
	}
	return ""
}
