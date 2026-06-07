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

	"humify-ng/internal/handoff"
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
	path       string
	root       string
	target     string
	runner     string
	testCmd    string
	stage      string // second positional, e.g. `verify <stage>`
	godLOC     int
	maxReplans int
	json       bool
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
	case "audit":
		return cmdAudit(opts)
	case "consolidate":
		return cmdConsolidate(opts)
	case "plan":
		return cmdPlan(opts)
	case "execute":
		return cmdExecute(opts)
	case "patchlog":
		return cmdPatchlog(opts)
	case "undo":
		return cmdUndo(opts)
	case "resume":
		return cmdResume(opts)
	case "verify":
		return cmdVerify(opts)
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
		case strings.HasPrefix(a, "--runner="):
			opts.runner = strings.TrimPrefix(a, "--runner=")
		case strings.HasPrefix(a, "--test-cmd="):
			opts.testCmd = strings.TrimPrefix(a, "--test-cmd=")
		case strings.HasPrefix(a, "--god-loc="):
			if n, err := strconv.Atoi(strings.TrimPrefix(a, "--god-loc=")); err == nil && n > 0 {
				opts.godLOC = n
			}
		case strings.HasPrefix(a, "--max-replans="):
			if n, err := strconv.Atoi(strings.TrimPrefix(a, "--max-replans=")); err == nil && n > 0 {
				opts.maxReplans = n
			}
		case !strings.HasPrefix(a, "-"):
			// First bare token is the command; a second (e.g. `verify <stage>`)
			// is captured as the stage argument.
			if cmd == "" {
				cmd = a
			} else if opts.stage == "" {
				opts.stage = a
			}
		}
	}
	return cmd, opts
}

// saveHandoff best-effort writes the resume cursor after a command acts. It is a
// convenience only — resume derives the next step from disk regardless — so a
// write failure must not fail the command; it just means resume falls back to
// pure disk derivation.
func saveHandoff(root string, h handoff.Handoff) {
	if root == "" {
		return
	}
	_ = handoff.Save(root, h)
}

// promptPaths builds the root-relative prompt paths a spawn cursor advertises,
// matching where each stage writes them under .humify/tmp/<sub>/.
func promptPaths(sub string, ids []string) []string {
	ps := make([]string, len(ids))
	for i, id := range ids {
		ps[i] = filepath.Join(layout.Dir, "tmp", sub, id+".prompt.md")
	}
	return ps
}

// resolveRoot finds the project root for a command: an explicit --root wins, else
// walk up from --path to the nearest .humify/, else fall back to --path (or ".").
// The fallback lets resume/verify still answer "needs_bootstrap" outside a project.
func resolveRoot(opts options) string {
	if opts.root != "" {
		return opts.root
	}
	if found, ok := layout.FindRoot(opts.path); ok {
		return found
	}
	if opts.path != "" {
		return opts.path
	}
	return "."
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
	fmt.Println(`humify-ng — massive-codebase untangler (stages 1-6)

usage:
  humify status      [--path=DIR] [--json]
  humify heatmap     --target=DIR [--root=DIR] [--god-loc=N] [--json]
  humify audit       [--root=DIR] [--runner=dispatch] [--json]
  humify consolidate [--root=DIR] [--json]
  humify plan        [--root=DIR] [--max-replans=N] [--json]
  humify execute     [--root=DIR] [--test-cmd=CMD] [--json]
  humify patchlog    [--root=DIR] [--json]
  humify undo        [--root=DIR] [--json]
  humify resume      [--path=DIR] [--root=DIR] [--json]
  humify verify      [STAGE] [--path=DIR] [--root=DIR] [--json]

status       derive each area's lifecycle stage from on-disk artifacts under
             .humify/areas/. Nothing is stored, so a reset loses no progress.
heatmap      scan a target codebase, decompose into areas, build the dependency
             DAG, compute parallel waves, score risk, and bootstrap .humify/
             (HEATMAP.md, area scaffold, intel, AUDIT_MANIFEST) under --root.
audit        plan the audit fan-out: derive which areas still need an auditor
             (resumable from disk), then dispatch. --runner=dispatch (default)
             writes one prompt per pending area under .humify/tmp/auditors/ for
             the orchestrator to spawn; the gather is the consolidate stage.
consolidate  gather all audit fragments named in the manifest into one AUDIT.md
             (dedup, cycle-detect, bucket conflicts), fail-closed on any pending
             or invalid fragment. Writes AUDIT.md + CONFLICTS.md.
plan         advance the per-area plan convergence loop one round: dispatch
             planners then adversarial plan-checkers, re-planning with feedback
             until each finding-bearing area has an accepted PLAN.md (bounded by
             --max-replans, default 3, with stall detection). Resumable; the
             orchestrator spawns the dispatched agents and re-runs.
execute      advance execution one dependency wave at a time: fork an isolated
             git worktree+branch per planned slice and dispatch executors, then
             on re-run run the fail-closed merge barrier, the --test-cmd gate,
             and dispatch verifiers. Requires a git repo at --root.
patchlog     deterministic roll-up of every executed area into PATCHLOG.md
             (flips each to "patched"), with its merge commit and summary line.
undo         revert execute's merge commits (newest first, via git revert, never
             reset) and clear the commit log. Requires a git repo at --root.
resume       name the next step in the pipeline (advisory — prints the command to
             run, never runs it). Disk is authoritative; a HANDOFF.json cursor, if
             present and still in agreement, adds the exact prompts to spawn.
verify       re-run a stage's deterministic gate read-only without doing its work.
             STAGE is one of: heatmap audit consolidate plan execute patchlog;
             omit STAGE to check the whole pipeline. Exit 2 on any incomplete gate.

exit codes (status, audit, consolidate, plan, execute, resume, verify):
  0  clean / dispatched / converged / merged   1  not a humify project / error
  2  drift, pending, escalated, blocked, or gate-failed`)
}
