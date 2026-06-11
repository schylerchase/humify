// Command humify reviews a target codebase for AI-generated / AI-degraded code
// smells, scores its human maintainability, and produces a prioritized, evidence-
// backed refactor plan — analyzing and planning before it ever changes source.
//
// The primary commands are analyze, plan, verify, apply, status, and doctor, with
// JSON state under .humify/ as the control plane. The original massive-codebase
// "untangler" workflow (heatmap/audit/consolidate/...) is preserved under the
// `humify untangle <stage>` namespace so its commands do not collide with the
// product surface.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"humify/internal/handoff"
	"humify/internal/layout"
	"humify/internal/output"
	"humify/internal/state"
)

// Version is set at build time via -ldflags "-X main.Version=vX.Y.Z".
var Version = "dev"

const (
	exitOK    = 0 // clean
	exitError = 1 // not a humify project / read failure
	exitDrift = 2 // at least one area is audit-incomplete
)

type options struct {
	path       string
	root       string
	target     string // heatmap target dir, OR the HMF-### plan item for apply
	runner     string
	testCmd    string
	agentCmd   string // spawn runner: agent command (prompt piped on stdin)
	stage      string // untangle verify <stage>
	configPath string // explicit humify.config.json path (product commands)
	args       []string
	godLOC     int
	maxReplans int
	jobs       int           // spawn runner: max concurrent agents
	timeout    time.Duration // spawn runner: per-agent wall-clock cap (0 → runner default)
	json       bool
	yes             bool // apply: confirm a source-changing action
	dryRun          bool // apply: describe without changing
	markdown        bool // product commands: also write the optional markdown report
	execute         bool // untangle run: opt in to the source-modifying execute stage
	unsafePermission bool // apply: unlock autonomous agent execution for manual/assisted items
}

const defaultGodLOC = 1500

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	cmd, opts := parseArgs(args)
	switch cmd {
	case "analyze":
		return cmdAnalyze(opts)
	case "plan":
		return cmdPlan(opts)
	case "verify":
		return cmdVerify(opts)
	case "apply":
		return cmdApply(opts)
	case "status":
		return cmdStatus(opts)
	case "doctor":
		return cmdDoctor(opts)
	case "untangle":
		return runUntangle(opts)
	case "", "help", "-h", "--help":
		printUsage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		printUsage()
		return exitError
	}
}

// runUntangle dispatches the preserved massive-codebase workflow: the stage name
// is the first positional after `untangle` (e.g. `humify untangle heatmap`).
func runUntangle(opts options) int {
	stage := ""
	if len(opts.args) > 0 {
		stage = opts.args[0]
	}
	switch stage {
	case "status":
		if len(opts.args) > 1 && opts.path == "." {
			opts.path = opts.args[1]
		}
		return untangleStatus(opts)
	case "heatmap":
		return untangleHeatmap(opts)
	case "audit":
		return untangleAudit(opts)
	case "consolidate":
		return untangleConsolidate(opts)
	case "plan":
		return untanglePlan(opts)
	case "execute":
		return untangleExecute(opts)
	case "patchlog":
		return untanglePatchlog(opts)
	case "undo":
		return untangleUndo(opts)
	case "resume":
		return untangleResume(opts)
	case "run":
		return untangleRun(opts)
	case "verify":
		if len(opts.args) > 1 {
			opts.stage = opts.args[1]
		}
		return untangleVerify(opts)
	case "", "help", "-h", "--help":
		printUntangleUsage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "unknown untangle stage: %s\n\n", stage)
		printUntangleUsage()
		return exitError
	}
}

// parseArgs parses flags and positionals. Value flags accept both `--flag=value`
// and `--flag value`; the first bare token is the command and the rest are
// positionals (opts.args), interpreted per command. Unknown flags are ignored.
func parseArgs(args []string) (string, options) {
	opts := options{path: ".", godLOC: defaultGodLOC, jobs: 4}
	var cmd string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, eqVal, hasEq := strings.Cut(a, "=")
		value := func() string {
			if hasEq {
				return eqVal
			}
			if i+1 < len(args) {
				i++
				return args[i]
			}
			return ""
		}
		switch {
		case a == "--json":
			opts.json = true
		case a == "--yes" || a == "-y":
			opts.yes = true
		case a == "--dry-run":
			opts.dryRun = true
		case a == "--markdown":
			opts.markdown = true
		case a == "--execute":
			opts.execute = true
		case a == "--unsafe-permission":
			opts.unsafePermission = true
		case name == "--path":
			opts.path = value()
		case name == "--root":
			opts.root = value()
		case name == "--target":
			opts.target = value()
		case name == "--runner":
			opts.runner = value()
		case name == "--test-cmd":
			opts.testCmd = value()
		case name == "--agent-cmd":
			opts.agentCmd = value()
		case name == "--config":
			opts.configPath = value()
		case name == "--god-loc":
			if n, err := strconv.Atoi(value()); err == nil && n > 0 {
				opts.godLOC = n
			}
		case name == "--max-replans":
			if n, err := strconv.Atoi(value()); err == nil && n > 0 {
				opts.maxReplans = n
			}
		case name == "--jobs":
			if n, err := strconv.Atoi(value()); err == nil && n > 0 {
				opts.jobs = n
			}
		case name == "--timeout":
			if d, err := time.ParseDuration(value()); err == nil && d > 0 {
				opts.timeout = d
			}
		case strings.HasPrefix(a, "-"):
			// Unknown flag — ignore so a typo never silently becomes a positional.
		case cmd == "":
			cmd = a
		default:
			opts.args = append(opts.args, a)
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

func untangleStatus(opts options) int {
	// resolveRoot, like every other untangle subcommand, honors --root first and
	// only then walks up from --path. (status used to call FindRoot(opts.path)
	// directly, silently ignoring --root.) resolveRoot can fall back to a path that
	// is not a project, so confirm .humify/ actually exists at the resolved root.
	root := resolveRoot(opts)
	if fi, err := os.Stat(layout.HumifyDir(root)); err != nil || !fi.IsDir() {
		return fail(opts, "no_humify_dir", exitError,
			"no .humify/ directory found from "+root)
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
	fmt.Println(`humify — review a codebase for AI-slop and plan safe refactors

usage:
  humify analyze [PATH] [--config=FILE] [--markdown] [--json]
  humify plan    [PATH] [--markdown] [--json]
  humify verify  [PATH] [--json]
  humify status  [PATH] [--json]
  humify doctor  [PATH] [--json]
  humify apply   --target HMF-### [--dry-run | --yes] [PATH]
  humify apply   --target HMF-### --unsafe-permission --agent-cmd=CMD [--yes] [PATH]
  humify untangle <stage> ...        (the massive-codebase workflow; see: humify untangle help)

PATH defaults to the current directory. Output JSON state is written under .humify/.

analyze  scan the repo (honoring .gitignore/.humifyignore and skipping generated/
         vendor/build dirs), detect stack/scripts/entry points, measure per-file
         metrics, flag AI-slop signals with file+line evidence, and score five
         health categories. Writes .humify/analysis.json. Read-only.
plan     rank the findings into prioritized HMF-### refactor items with evidence,
         risk, benefit, validation, and automation safety. Writes .humify/plan.json.
         Runs analyze first if no analysis exists. Read-only.
verify   detect and run the project's safe validation commands (test/build/lint/
         typecheck) and record results in .humify/validation.json.
status   print the current analysis/plan/validation state from .humify/ JSON.
doctor   check Humify's wiring, the target path, git state, and tool availability.
apply    the only command that changes source — and only conservatively. Defaults
         to a dry run. Requires --target HMF-### and --yes to act, performs only
         items marked safe (today: reversible file quarantine into
         .humify/delete-me/<id>/ with a manifest), re-runs validation, and rolls
         back on regression. Refuses broad/manual rewrites.

         --unsafe-permission unlocks autonomous agent execution for assisted and
         manual items. Requires --agent-cmd=CMD (prompt piped on stdin), --yes,
         and an explicit "yes" confirmation at the terminal — three gates, not one.
         The agent receives a precise action spec built by humify plan: which files
         to change, what transformation to apply, and what evidence supports it.
         humify verify runs after and rolls back the entire change on any regression.
         Use when you understand the risk and want the agent to handle a refactor
         that humify normally refuses to automate.

         example:
           humify apply --target HMF-002 --unsafe-permission --agent-cmd="claude --print" --yes

safety: analyze, plan, verify, status, and doctor never modify target source.
        apply quarantines (never deletes) and is reversible.
        apply --unsafe-permission mutates source via an agent — requires three
        explicit confirmations and rolls back on regression, but is not reversible
        in the same mechanical sense as a quarantine.
exit codes: 0 ok · 1 error · 2 verify failed or apply rolled back`)
}

func printUntangleUsage() {
	fmt.Println(`humify untangle — massive-codebase untangler (agent-orchestrated workflow)

usage:
  humify untangle status      [PATH]
  humify untangle heatmap     --target=DIR [--root=DIR] [--god-loc=N] [--json]
  humify untangle audit       [--root=DIR] [--runner=dispatch|spawn] [--agent-cmd=CMD] [--jobs=N] [--timeout=DUR] [--json]
  humify untangle consolidate [--root=DIR] [--json]
  humify untangle plan        [--root=DIR] [--max-replans=N] [--json]
  humify untangle execute     [--root=DIR] [--test-cmd=CMD] [--json]
  humify untangle patchlog    [--root=DIR] [--json]
  humify untangle undo        [--root=DIR] [--json]
  humify untangle resume      [--path=DIR] [--root=DIR] [--json]
  humify untangle verify      [STAGE] [--path=DIR] [--root=DIR] [--json]
  humify untangle run         --agent-cmd=CMD [--execute] [--test-cmd=CMD] [--jobs=N] [--timeout=DUR] [--max-replans=N] [--root=DIR] [--json]

This is the original LLM-auditor pipeline: it derives each area's stage from
on-disk artifacts under .humify/areas/ and dispatches agents to audit, plan, and
execute refactors wave by wave. Exit 2 signals drift/pending/blocked.

run is the autonomous driver: it walks the same next-step decision 'resume'
prints and ACTS on it, spawning --agent-cmd (prompt piped on stdin) at each
agent stage until the pipeline is done or blocked. By default it stops at
plan-converged — audit→consolidate→plan only ever write under .humify/, never
your source. Pass --execute to continue through execute (which forks worktrees,
spawns executors that rewrite source, and auto-merges behind the merge barrier +
optional --test-cmd gate) and patchlog. Autonomy over source is opt-in, like
apply's --yes; undo reverts a wave's merges.`)
}
