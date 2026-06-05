// Command humify-ng is a massive-codebase untangler that owns its
// orchestration loop in deterministic code (the agent is a worker it calls,
// not the orchestrator). Stage 1 ships `status`: it derives each area's
// pipeline stage from on-disk artifacts and exits non-zero on drift.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
	path   string
	root   string
	target string
	godLOC int
	json   bool
}

const defaultGodLOC = 1500

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cmd, opts := parseArgs(args)
	switch cmd {
	case "status":
		return cmdStatus(opts)
	case "heatmap":
		return cmdHeatmap(opts)
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
	opts := options{path: ".", godLOC: defaultGodLOC}
	var cmd string
	for _, a := range args {
		switch {
		case a == "--json":
			opts.json = true
		case strings.HasPrefix(a, "--path="):
			opts.path = strings.TrimPrefix(a, "--path=")
		case strings.HasPrefix(a, "--root="):
			opts.root = strings.TrimPrefix(a, "--root=")
		case strings.HasPrefix(a, "--target="):
			opts.target = strings.TrimPrefix(a, "--target=")
		case strings.HasPrefix(a, "--god-loc="):
			if n, err := strconv.Atoi(strings.TrimPrefix(a, "--god-loc=")); err == nil && n > 0 {
				opts.godLOC = n
			}
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
	fmt.Println(`humify-ng — massive-codebase untangler (stages 1-2)

usage:
  humify status  [--path=DIR] [--json]
  humify heatmap --target=DIR [--root=DIR] [--god-loc=N] [--json]

status   derive each area's lifecycle stage from on-disk artifacts under
         .humify/areas/. Nothing is stored, so a reset loses no progress.
heatmap  scan a target codebase, decompose into areas, build the dependency
         DAG, compute parallel waves, score risk, and bootstrap .humify/
         (HEATMAP.md, area scaffold, intel/areas.json) under --root (cwd).

status exit codes:
  0  clean   1  not a humify project   2  drift (an area is audit-incomplete)`)
}
