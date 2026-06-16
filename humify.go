package main

// This file implements Humify's product command surface — analyze, plan, verify,
// apply, status, doctor — on top of the internal/humify packages. The handlers
// stay thin: parse intent, call the engine, render. JSON state under .humify/ is
// the control plane; terminal output and optional markdown are renderings of it.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/schylerryan/humify/internal/humify/analyze"
	"github.com/schylerryan/humify/internal/humify/apply"
	"github.com/schylerryan/humify/internal/humify/detect"
	hplan "github.com/schylerryan/humify/internal/humify/plan"
	"github.com/schylerryan/humify/internal/humify/scan"
	hstate "github.com/schylerryan/humify/internal/humify/state"
	"github.com/schylerryan/humify/internal/humify/verify"
)

// cmdAnalyze reviews the target read-only and writes .humify/analysis.json.
func cmdAnalyze(opts options) int {
	root := absTarget(opts)
	a, err := analyze.Run(root, loadConfig(root, opts))
	if err != nil {
		return fail(opts, "analyze_error", exitError, "analyze failed: "+err.Error())
	}
	if err := hstate.Save(root, hstate.AnalysisFile, a); err != nil {
		return fail(opts, "write_error", exitError, "could not write analysis: "+err.Error())
	}
	if opts.markdown {
		writeAnalysisMarkdown(root, a)
	}
	if opts.json {
		emitJSON(a)
		return exitOK
	}
	printAnalysis(a)
	return exitOK
}

// cmdPlan ranks findings into a refactor plan, analyzing first if needed.
func cmdPlan(opts options) int {
	root := absTarget(opts)
	a, err := loadOrAnalyze(root, opts)
	if err != nil {
		return fail(opts, "analyze_error", exitError, err.Error())
	}
	p := hplan.Build(a)
	stampVerification(root, opts, &p)
	if err := hstate.Save(root, hstate.PlanFile, p); err != nil {
		return fail(opts, "write_error", exitError, "could not write plan: "+err.Error())
	}
	if opts.markdown {
		writePlanMarkdown(root, p)
	}
	if opts.json {
		emitJSON(p)
		return exitOK
	}
	printPlan(p)
	return exitOK
}

// stampVerification stamps each applyable plan item with its coverage verdict.
// Coverage is produced here if it has not been already, so the verdict appears in
// the documented analyze->plan order and not only after a separate `verify`. The
// --no-coverage flag opts out (items then carry no verdict). A measured report with
// no execution yields "build-only"; an unmeasured report yields "unmeasured" — the
// verdict is never silently empty.
func stampVerification(root string, opts options, p *hplan.Plan) {
	if opts.noCoverage {
		return
	}
	var cov verify.CoverageReport
	if hstate.Load(root, hstate.CoverageFile, &cov) != nil {
		cov = verify.Coverage(root)
		_ = hstate.Save(root, hstate.CoverageFile, cov)
	}
	stampFromCoverage(p, cov)
}

// stampFromCoverage sets the Verification verdict on every applyable item from a
// coverage report. Pure (no I/O) so it is unit-testable.
func stampFromCoverage(p *hplan.Plan, cov verify.CoverageReport) {
	for i := range p.Items {
		if p.Items[i].Applyable {
			p.Items[i].Verification = string(cov.WorstVerdict(p.Items[i].Files))
		}
	}
}

// cmdVerify runs the project's detected validation commands. With --save-baseline
// it snapshots the result for a later comparison; with --baseline it diffs the
// current run against that snapshot to separate a self-caused regression from an
// ambient failure that was already red.
func cmdVerify(opts options) int {
	root := absTarget(opts)
	// Measure dirtiness BEFORE running validation: verify's build can litter the
	// tree (e.g. `go build ./...` drops a binary in cwd), so a post-run check would
	// false-positive. This is the honest "was the tree dirty at invocation" signal.
	preDirty := opts.saveBaseline && verify.RepoDirtyExcludingHumify(root)
	v, err := verify.Run(root, time.Now())
	if err != nil {
		return fail(opts, "verify_error", exitError, "verify failed: "+err.Error())
	}
	if err := hstate.Save(root, hstate.ValidationFile, v); err != nil {
		return fail(opts, "write_error", exitError, "could not write validation: "+err.Error())
	}
	if !opts.noCoverage {
		cov := verify.Coverage(root)
		_ = hstate.Save(root, hstate.CoverageFile, cov)
	}
	switch {
	case opts.saveBaseline:
		return verifySaveBaseline(opts, root, v, preDirty)
	case opts.baseline:
		return verifyWithBaseline(opts, root, v)
	default:
		return verifyPlain(opts, v)
	}
}

// verifyPlain is the unchanged single-snapshot behavior: report pass/fail.
func verifyPlain(opts options, v verify.Validation) int {
	if opts.json {
		emitJSON(v)
	} else {
		printValidation(v)
	}
	if !v.Passed {
		return exitDrift
	}
	return exitOK
}

// verifySaveBaseline persists v as the pre-edit baseline. It returns exitOK on a
// successful capture regardless of pass/fail: an ambient-red baseline is expected
// and must not block the AI's `--save-baseline && edit` step.
func verifySaveBaseline(opts options, root string, v verify.Validation, dirty bool) int {
	if err := verify.SaveBaseline(root, v, dirty, time.Now()); err != nil {
		return fail(opts, "write_error", exitError, "could not write baseline: "+err.Error())
	}
	snap, _ := verify.LoadBaseline(root)
	if opts.json {
		emitJSON(snap)
	} else {
		printValidation(v)
		printBaselineSaved(snap)
	}
	return exitOK
}

// verifyWithBaseline diffs the current run against the saved baseline. A
// previously-passing kind that now cleanly fails is the change's regression
// (exitDrift); anything already failing is ambient. With no saved baseline it
// degrades loudly to a plain run.
func verifyWithBaseline(opts options, root string, post verify.Validation) int {
	snap, ok := verify.LoadBaseline(root)
	if !ok {
		if !opts.json {
			printNoBaseline()
		}
		return verifyPlain(opts, post)
	}
	stale := verify.BaselineStale(snap, root)
	_, newly, _ := verify.Delta(snap.Result, post)
	if opts.json {
		emitJSON(baselineResultOf(post, snap, stale))
	} else {
		printBaselineDelta(post, snap, stale)
	}
	if len(newly) > 0 {
		return exitDrift
	}
	return exitOK
}

// baselineResult is the machine-readable --baseline verdict for AI callers.
type baselineResult struct {
	Post           verify.Validation `json:"post"`
	AlreadyFailing []string          `json:"already_failing"`
	NewlyFailing   []string          `json:"newly_failing"`
	Fixed          []string          `json:"fixed"`
	Indeterminate  []string          `json:"indeterminate"`
	BaselineStale  bool              `json:"baseline_stale"`
	BaselineDirty  bool              `json:"baseline_dirty_at_save"`
}

func baselineResultOf(post verify.Validation, snap verify.BaselineSnapshot, stale bool) baselineResult {
	already, newly, fixed := verify.Delta(snap.Result, post)
	return baselineResult{
		Post: post, AlreadyFailing: already, NewlyFailing: newly, Fixed: fixed,
		Indeterminate: indeterminateKinds(post), BaselineStale: stale, BaselineDirty: snap.Dirty,
	}
}

// cmdApply executes one plan item — conservatively, defaulting to a dry run.
func cmdApply(opts options) int {
	root := absTarget(opts)
	if opts.target == "" {
		return fail(opts, "missing_target", exitError, "apply requires --target HMF-### (run `humify plan` to see ids)")
	}
	var p hplan.Plan
	if err := hstate.Load(root, hstate.PlanFile, &p); err != nil {
		return fail(opts, "no_plan", exitError, "no plan found — run `humify plan` first")
	}
	if opts.unsafePermission && opts.agentCmd == "" {
		return fail(opts, "missing_agent_cmd", exitError, "--unsafe-permission requires --agent-cmd=CMD (the command that receives the refactor spec on stdin)")
	}
	if opts.unsafePermission && opts.yes && !opts.dryRun {
		fmt.Fprintln(os.Stderr, "⚠  WARNING: --unsafe-permission will spawn an agent to autonomously rewrite source code.")
		fmt.Fprintln(os.Stderr, "   The change is validated and reverted on regression, but it is not mechanically reversible like a quarantine.")
		fmt.Fprintln(os.Stderr, "   Ensure --agent-cmd can write files unattended (e.g. claude needs --dangerously-skip-permissions).")
		// The interactive "yes" is a speed bump for a human at a terminal. When stdin
		// is not a TTY (scripted, piped, background, agent-driven) there is nobody to
		// prompt and fmt.Fscan would block forever — the two explicit flags above are
		// the deliberate signal, so proceed without prompting.
		if isInteractive(os.Stdin) {
			fmt.Fprintln(os.Stderr, "   Type \"yes\" to proceed, anything else to abort:")
			var confirm string
			fmt.Fscan(os.Stdin, &confirm)
			if confirm != "yes" {
				fmt.Fprintln(os.Stderr, "Aborted.")
				return exitError
			}
		}
	}
	res, err := apply.Apply(root, p, opts.target, opts.dryRun, opts.yes, opts.agentCmd, opts.unsafePermission, time.Now())
	if err != nil {
		return fail(opts, "apply_error", exitError, err.Error())
	}
	if opts.json {
		emitJSON(res)
	} else {
		printApply(res)
	}
	// A rollback means the action could not be applied safely — surface it as
	// drift so a script wrapper can distinguish it from a real apply.
	if res.RolledBack {
		return exitDrift
	}
	return exitOK
}

// isInteractive reports whether f is a terminal (character device) rather than a
// pipe, file, or closed stream. Used to decide whether prompting a human makes
// sense; a non-TTY would block on a read that no one can answer.
func isInteractive(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// cmdStatus prints the current Humify state from .humify/ JSON.
func cmdStatus(opts options) int {
	root := absTarget(opts)
	var a analyze.Analysis
	var p hplan.Plan
	var v verify.Validation
	haveA := hstate.Load(root, hstate.AnalysisFile, &a) == nil
	haveP := hstate.Load(root, hstate.PlanFile, &p) == nil
	haveV := hstate.Load(root, hstate.ValidationFile, &v) == nil
	if opts.json {
		emitJSON(statusView(a, haveA, p, haveP, v, haveV))
		return exitOK
	}
	printStatus(root, a, haveA, p, haveP, v, haveV)
	return exitOK
}

// cmdDoctor checks Humify's wiring and the target repo's readiness.
func cmdDoctor(opts options) int {
	root := absTarget(opts)
	checks := doctorChecks(root)
	if opts.json {
		emitJSON(checks)
	} else {
		printDoctor(root, checks)
	}
	for _, c := range checks {
		if c.Status == "fail" {
			return exitError
		}
	}
	return exitOK
}

// absTarget resolves the target repo: an explicit positional path wins, else
// --path, else the current directory; returned absolute for clean output.
func absTarget(opts options) string {
	target := opts.path
	if len(opts.args) > 0 {
		target = opts.args[0]
	}
	if abs, err := filepath.Abs(target); err == nil {
		return abs
	}
	return target
}

// loadOrAnalyze returns a fresh-enough analysis: the persisted one if its schema
// matches, otherwise a new run (which is also saved).
func loadOrAnalyze(root string, opts options) (analyze.Analysis, error) {
	var a analyze.Analysis
	if hstate.Load(root, hstate.AnalysisFile, &a) == nil && a.Schema == hstate.Schema {
		return a, nil
	}
	a, err := analyze.Run(root, loadConfig(root, opts))
	if err != nil {
		return a, err
	}
	_ = hstate.Save(root, hstate.AnalysisFile, a)
	return a, nil
}

// fileConfig is the subset of humify.config.json Humify reads today.
type fileConfig struct {
	MaxFileLines     int      `json:"maxFileLines"`
	MaxFunctionLines int      `json:"maxFunctionLines"`
	MaxNestingDepth  int      `json:"maxNestingDepth"`
	LiveModules      []string `json:"liveModules"`
}

// loadConfig reads thresholds from humify.config.json (explicit --config or the
// repo root), falling back to defaults when absent or invalid.
func loadConfig(root string, opts options) analyze.Config {
	path := opts.configPath
	if path == "" {
		path = filepath.Join(root, "humify.config.json")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return analyze.Defaults()
	}
	var fc fileConfig
	if json.Unmarshal(data, &fc) != nil {
		return analyze.Defaults()
	}
	return analyze.Config{
		MaxFileLines:     fc.MaxFileLines,
		MaxFunctionLines: fc.MaxFunctionLines,
		MaxNestingDepth:  fc.MaxNestingDepth,
		LiveModules:      fc.LiveModules,
	}
}

// emitJSON prints a value as indented JSON, matching the on-disk state shape.
func emitJSON(v any) {
	if b, err := json.MarshalIndent(v, "", "  "); err == nil {
		fmt.Println(string(b))
	}
}

// check is one doctor result line.
type check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok|warn|fail
	Detail string `json:"detail"`
}

// doctorChecks inspects the target and Humify's ability to operate on it.
func doctorChecks(root string) []check {
	var checks []check
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return append(checks, check{"target", "fail", root + " is not a readable directory"})
	}
	checks = append(checks, check{"target", "ok", root})
	checks = append(checks, writableCheck(root))
	checks = append(checks, gitCheck(root))
	checks = append(checks, projectCheck(root))
	checks = append(checks, stateCheck(root))
	return checks
}

// writableCheck confirms Humify can create its .humify/ state directory.
func writableCheck(root string) check {
	if err := os.MkdirAll(filepath.Join(root, hstate.Dir), 0o755); err != nil {
		return check{".humify writable", "fail", err.Error()}
	}
	return check{".humify writable", "ok", "can write Humify state"}
}

// gitCheck reports whether the target is a git repo (apply warns on a dirty repo).
func gitCheck(root string) check {
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		return check{"git", "ok", "git repository (apply changes are easy to review)"}
	}
	return check{"git", "warn", "not a git repository — apply changes are harder to review/revert"}
}

// projectCheck reports the detected stack and package manager.
func projectCheck(root string) check {
	res, err := scan.Walk(root, nil)
	if err != nil {
		return check{"project", "warn", "could not scan: " + err.Error()}
	}
	p := detect.Detect(res, root)
	if len(p.Stack) == 0 {
		return check{"project", "warn", "no recognized source languages found"}
	}
	return check{"project", "ok", fmt.Sprintf("stack: %s · package manager: %s", join(p.Stack), p.PackageManager)}
}

// stateCheck reports which Humify state files already exist.
func stateCheck(root string) check {
	var have []string
	for _, f := range []string{hstate.AnalysisFile, hstate.PlanFile, hstate.ValidationFile} {
		if hstate.Exists(root, f) {
			have = append(have, f)
		}
	}
	if len(have) == 0 {
		return check{"state", "warn", "no analysis yet — run `humify analyze`"}
	}
	return check{"state", "ok", "present: " + join(have)}
}

// statusView is the combined JSON shape `status --json` emits.
func statusView(a analyze.Analysis, haveA bool, p hplan.Plan, haveP bool, v verify.Validation, haveV bool) map[string]any {
	// Presence flags are always emitted so a consumer can tell "absent" from "empty"
	// — an empty state would otherwise marshal to a bare {} with no signal.
	view := map[string]any{
		"have_analysis":   haveA,
		"have_plan":       haveP,
		"have_validation": haveV,
	}
	if haveA {
		view["analysis"] = a
	}
	if haveP {
		view["plan"] = p
	}
	if haveV {
		view["validation"] = v
	}
	return view
}
