// Command humify-ng is a massive-codebase untangler that owns its
// orchestration loop in deterministic code (the agent is a worker it calls,
// not the orchestrator). Stage 1 ships `status`: it derives each area's
// pipeline stage from on-disk artifacts and exits non-zero on drift.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"humify-ng/internal/layout"
	"humify-ng/internal/output"
	"humify-ng/internal/state"
)

const (
	exitOK    = 0 // clean
	exitError = 1 // not a humify project / read failure
	exitDrift = 2 // at least one area is audit-incomplete
)

type options struct {
	path string
	json bool
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cmd, opts := parseArgs(args)
	switch cmd {
	case "status":
		return cmdStatus(opts)
	case "", "help", "-h", "--help":
		printUsage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		return exitError
	}
}

func parseArgs(args []string) (string, options) {
	opts := options{path: "."}
	var cmd string
	for _, a := range args {
		switch {
		case a == "--json":
			opts.json = true
		case strings.HasPrefix(a, "--path="):
			opts.path = strings.TrimPrefix(a, "--path=")
		case cmd == "" && !strings.HasPrefix(a, "-"):
			cmd = a
		}
	}
	return cmd, opts
}

func cmdStatus(opts options) int {
	root, found := layout.FindRoot(opts.path)
	if !found {
		return fail(opts, "no_humify_dir", exitError,
			"no .humify/ directory found from "+opts.path)
	}
	ids, err := layout.DiscoverAreas(root)
	if err != nil {
		return fail(opts, "read_error", exitError, "cannot read areas: "+err.Error())
	}

	auditDoc := readFile(layout.AuditFile(root))
	patchlogDoc := readFile(layout.PatchlogFile(root))
	data := buildStatus(root, ids, auditDoc, patchlogDoc)

	reason, code := "ok", exitOK
	if data.Drifted > 0 {
		reason, code = "audit_incomplete", exitDrift
	}
	if opts.json {
		output.EmitJSON(os.Stdout, output.Result{Ok: code == exitOK, ReasonCode: reason, Data: data})
	} else {
		output.EmitStatusTable(os.Stdout, data)
	}
	return code
}

func buildStatus(root string, ids []string, auditDoc, patchlogDoc string) output.StatusData {
	areas := make([]state.Area, 0, len(ids))
	patched, drifted := 0, 0
	for _, id := range ids {
		a := state.Derive(filepath.Join(layout.AreasDir(root), id), id, auditDoc, patchlogDoc)
		switch a.Status {
		case state.Patched:
			patched++
		case state.AuditIncomplete:
			drifted++
		}
		areas = append(areas, a)
	}
	return output.StatusData{
		Root: root, Total: len(areas), Patched: patched,
		Drifted: drifted, Progress: pct(patched, len(areas)), Areas: areas,
	}
}

func fail(opts options, reason string, code int, human string) int {
	if opts.json {
		output.EmitJSON(os.Stdout, output.Result{Ok: false, ReasonCode: reason})
	} else {
		fmt.Fprintln(os.Stderr, human)
	}
	return code
}

func readFile(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

func pct(n, total int) string {
	if total == 0 {
		return "0%"
	}
	return fmt.Sprintf("%d%%", n*100/total)
}

func printUsage() {
	fmt.Println(`humify-ng — massive-codebase untangler (stage 1: status)

usage:
  humify status [--path=DIR] [--json]

status derives each area's lifecycle stage from on-disk artifacts under
.humify/areas/ — nothing is stored, so a reset loses no progress. Exit code:
  0  clean
  1  not a humify project (no .humify/ found)
  2  drift: an area is audit-incomplete (fragments exist but AUDIT.md
     never gathered them) — the failure that stranded the azure run.`)
}
