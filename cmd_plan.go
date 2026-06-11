package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/schylerryan/humify/internal/area"
	"github.com/schylerryan/humify/internal/consolidate"
	"github.com/schylerryan/humify/internal/handoff"
	"github.com/schylerryan/humify/internal/intel"
	"github.com/schylerryan/humify/internal/layout"
	"github.com/schylerryan/humify/internal/output"
	"github.com/schylerryan/humify/internal/plan"
	"github.com/schylerryan/humify/internal/plancheck"
	"github.com/schylerryan/humify/internal/spawn"
)

// errAuditIncomplete marks a plan round refused because the audit/consolidate
// stage has not finished. The manual command maps it to exit 2 (audit_incomplete);
// the autonomous driver never sees it (pipeline.Next routes to consolidate first).
var errAuditIncomplete = errors.New("audit incomplete")

// cmdPlan advances the plan stage's convergence loop by one round. It reads the
// consolidated findings (refusing to plan a half-audited project), derives each
// finding-bearing area's plan state from disk, and lets plan.Decide pick the
// round's actions: dispatch planners/checkers, or report converged/escalated.
// Like audit, the binary only writes prompts — the orchestrator spawns the
// dispatched agents and re-runs `humify plan` to take the next step.
func untanglePlan(opts options) int {
	root := opts.root
	if root == "" {
		if found, ok := layout.FindRoot(opts.path); ok {
			root = found
		} else {
			root = "."
		}
	}
	d, _, nothing, err := planRound(root, opts.maxReplans)
	if err != nil {
		return fail(opts, planReason(err), planCode(err), err.Error())
	}
	if nothing {
		return emitPlanNothing(opts, root)
	}
	return emitPlan(opts, root, d)
}

// planRound advances the plan convergence loop by one round: it derives the
// round's Decision from on-disk observations, writes the dispatched planner/
// checker prompts, persists the loop state, and returns the Decision plus the
// spawn jobs the round dispatched (each cwd = root, prompt delivered on stdin).
// It is the one round implementation both the manual `humify plan` command and
// the autonomous driver advance through, so the two can never diverge in how a
// round is computed or dispatched.
//
// nothing=true means no area has findings — the loop is vacuously converged. A
// still-incomplete audit is refused with errAuditIncomplete (the manual command
// surfaces it as exit 2; the driver is gated upstream by pipeline.Next, so it
// never reaches a plan round with an unfinished audit).
func planRound(root string, maxReplans int) (d plan.Decision, jobs []spawn.Job, nothing bool, err error) {
	res, err := consolidate.Run(root)
	if err != nil {
		return d, nil, false, err
	}
	if len(res.Pending) > 0 {
		return d, nil, false, fmt.Errorf("%w: %d area(s) not consolidated — finish `humify audit` + `humify consolidate` first",
			errAuditIncomplete, len(res.Pending))
	}
	in, err := intel.Load(root)
	if err != nil {
		return d, nil, false, err
	}
	targets := consolidate.FindingAreas(res)
	if len(targets) == 0 {
		return plan.Decision{Status: plan.StatusConverged}, nil, true, nil
	}

	st, err := plan.Load(root)
	if err != nil {
		return d, nil, false, fmt.Errorf("load loop state: %w", err)
	}
	if maxReplans > 0 {
		st.MaxReplans = maxReplans
	}
	st.Reconcile(targets)

	d = plan.Decide(plan.Observe(root, targets), &st)
	// Dispatch (which deletes stale verdicts for re-plans) BEFORE persisting the
	// bumped replan counters. A bumped counter must never reach disk while the
	// stale verdict it assumes-deleted is still there: that pairing would make
	// the next run false-stall (Replans>0 && Issues unchanged) and escalate an
	// area that never actually re-planned. With save last, a failed/crashed
	// dispatch simply leaves state un-bumped and the round replays cleanly.
	jobs, err = dispatchPlan(root, in.Target, in.AreasByID(), groupFindings(res.Findings), d)
	if err != nil {
		return d, nil, false, fmt.Errorf("dispatch failed: %w", err)
	}
	if err := st.Save(root); err != nil {
		return d, jobs, false, fmt.Errorf("save loop state: %w", err)
	}
	return d, jobs, false, nil
}

// planReason maps a planRound error to the command's machine reason code,
// preserving the distinctions JSON consumers branch on.
func planReason(err error) string {
	switch {
	case errors.Is(err, errAuditIncomplete):
		return "audit_incomplete"
	case errors.Is(err, intel.ErrNotExist):
		return "no_intel"
	case errors.Is(err, consolidate.ErrNoManifest), errors.Is(err, consolidate.ErrEmptyManifest):
		return consolidateReason(err)
	default:
		return "plan_error"
	}
}

// planCode maps a planRound error to its exit code: an incomplete audit is drift
// (exit 2, cleared by finishing audit); everything else is a hard error.
func planCode(err error) int {
	if errors.Is(err, errAuditIncomplete) {
		return exitDrift
	}
	return exitError
}

// groupFindings flattens consolidated findings to each area that reported them.
func groupFindings(merged []consolidate.Merged) map[string][]plan.Finding {
	byArea := map[string][]plan.Finding{}
	for _, m := range merged {
		f := plan.Finding{Severity: m.Severity, File: m.File, Line: m.Line, Title: m.Title, Detail: m.Detail}
		for _, src := range m.Sources {
			byArea[src] = append(byArea[src], f)
		}
	}
	return byArea
}

// dispatchPlan writes the prompts the round calls for and returns them as spawn
// jobs (each cwd = root, the rendered prompt on stdin) so the autonomous driver
// runs the exact prompts it wrote without re-rendering. Re-plans read the prior
// plan and verdict for feedback, then delete the stale verdict so the next round
// re-checks the fresh plan rather than re-judging the old one.
//
// Planners and checkers never collide within a round (Decide puts each area in
// at most one of PlanAreas/CheckAreas), so the returned job ids are unique and
// the driver's per-id success check is unambiguous.
func dispatchPlan(root, target string, areas map[string]area.Area, byArea map[string][]plan.Finding, d plan.Decision) ([]spawn.Job, error) {
	// Area ids flow from the manifest/fragments, which are NOT charset-validated on
	// the read path; an id with ".." or a separator would let the prompt write
	// below escape the project root (the same hand-edited-.humify threat the audit
	// stage guards via ResolveInRoot — see internal/audit/runner.go). Fail closed.
	if err := guardAreaIDs(d.PlanAreas...); err != nil {
		return nil, err
	}
	if err := guardAreaIDs(d.CheckAreas...); err != nil {
		return nil, err
	}
	replan := map[string]bool{}
	for _, id := range d.Replans {
		replan[id] = true
	}
	if len(d.PlanAreas) > 0 {
		if err := os.MkdirAll(filepath.Join(layout.TmpDir(root), "planners"), 0o755); err != nil {
			return nil, err
		}
	}
	if len(d.CheckAreas) > 0 {
		if err := os.MkdirAll(filepath.Join(layout.TmpDir(root), "plan-checkers"), 0o755); err != nil {
			return nil, err
		}
	}
	var jobs []spawn.Job
	for _, id := range d.PlanAreas {
		job := plan.PlannerJob{
			AreaID: id, Target: target, Files: areas[id].FilePaths,
			Findings: byArea[id], PlanPath: layout.AreaPlanRel(id),
		}
		if replan[id] {
			fb := &plan.Feedback{PriorPlan: readFile(layout.AreaPlan(root, id))}
			// Re-validate before trusting the verdict as feedback (observe already
			// validated it this round, but keep the read self-guarding so a corrupt
			// verdict yields no feedback issues rather than malformed ones).
			if c, err := plancheck.Load(layout.AreaPlanCheck(root, id)); err == nil && c.Validate() == nil {
				fb.Issues = c.Issues
			}
			job.Feedback = fb
		}
		prompt := plan.RenderPlannerPrompt(job)
		dest := filepath.Join(layout.TmpDir(root), "planners", id+".prompt.md")
		if err := os.WriteFile(dest, []byte(prompt), 0o644); err != nil {
			return nil, err
		}
		jobs = append(jobs, spawn.Job{ID: id, Dir: root, Prompt: prompt})
		if replan[id] {
			// Stale verdict already read for feedback; remove it so next round the
			// fresh plan is re-checked instead of re-judged against the old verdict.
			_ = os.Remove(layout.AreaPlanCheck(root, id))
		}
	}
	for _, id := range d.CheckAreas {
		job := plan.CheckerJob{
			AreaID: id, Target: target,
			PlanPath: layout.AreaPlanRel(id), CheckPath: layout.AreaPlanCheckRel(id),
		}
		prompt := plan.RenderCheckerPrompt(job)
		dest := filepath.Join(layout.TmpDir(root), "plan-checkers", id+".prompt.md")
		if err := os.WriteFile(dest, []byte(prompt), 0o644); err != nil {
			return nil, err
		}
		jobs = append(jobs, spawn.Job{ID: id, Dir: root, Prompt: prompt})
	}
	return jobs, nil
}

func emitPlan(opts options, root string, d plan.Decision) int {
	ok := d.Status != plan.StatusEscalated
	code := exitOK
	if !ok {
		code = exitDrift
	}
	switch d.Status {
	case plan.StatusConverged:
		saveHandoff(root, handoff.Handoff{Stage: "plan", Action: "proceed",
			NextCommand: "humify execute", Note: "plans accepted — execute the waves"})
	case plan.StatusEscalated:
		saveHandoff(root, handoff.Handoff{Stage: "plan", Action: "blocked",
			NextCommand: "humify plan --max-replans=N", Note: "replan budget exhausted — inspect PLAN-CHECK.json"})
	default:
		prompts := append(promptPaths("planners", d.PlanAreas), promptPaths("plan-checkers", d.CheckAreas)...)
		saveHandoff(root, handoff.Handoff{Stage: "plan", Action: "spawn",
			NextCommand: "humify plan", Prompts: prompts,
			Note: "spawn the dispatched planners/checkers, then re-run plan"})
	}
	if opts.json {
		output.EmitJSON(os.Stdout, output.Result{Ok: ok, ReasonCode: d.Status, Data: planData(d)})
		return code
	}
	switch d.Status {
	case plan.StatusConverged:
		fmt.Printf("plan converged: %d area(s) have accepted plans — run `humify execute`\n", len(d.Accepted))
	case plan.StatusEscalated:
		fmt.Printf("plan ESCALATED: %d area(s) unresolved after max replans:\n", len(d.Escalated))
		for _, v := range d.Escalated {
			fmt.Printf("  %s — %s (inspect %s)\n", v.AreaID, v.Reason, layout.AreaPlanCheckRel(v.AreaID))
		}
	default: // dispatch
		fmt.Printf("plan round: %d to (re)plan, %d to check (%d accepted so far)\n",
			len(d.PlanAreas), len(d.CheckAreas), len(d.Accepted))
		if len(d.PlanAreas) > 0 {
			fmt.Printf("  plan:  %s\n", strings.Join(d.PlanAreas, " "))
		}
		if len(d.CheckAreas) > 0 {
			fmt.Printf("  check: %s\n", strings.Join(d.CheckAreas, " "))
		}
		fmt.Printf("wrote prompts under %s\n", filepath.Join(layout.Dir, "tmp"))
		fmt.Println("next: spawn the dispatched planners/checkers, then re-run `humify plan`")
	}
	return code
}

func emitPlanNothing(opts options, root string) int {
	saveHandoff(root, handoff.Handoff{Stage: "plan", Action: "proceed",
		NextCommand: "humify status", Note: "no findings to plan — pipeline complete"})
	if opts.json {
		output.EmitJSON(os.Stdout, output.Result{Ok: true, ReasonCode: "nothing_to_plan"})
		return exitOK
	}
	fmt.Println("nothing to plan: no area has findings (run `humify consolidate` first, or the audit is clean)")
	return exitOK
}

type escalatedArea struct {
	AreaID string `json:"area_id"`
	Reason string `json:"reason"`
}

func planData(d plan.Decision) map[string]any {
	esc := make([]escalatedArea, 0, len(d.Escalated))
	for _, v := range d.Escalated {
		esc = append(esc, escalatedArea{AreaID: v.AreaID, Reason: v.Reason})
	}
	return map[string]any{
		"status":      d.Status,
		"plan_areas":  d.PlanAreas,
		"check_areas": d.CheckAreas,
		"replans":     d.Replans,
		"accepted":    d.Accepted,
		"escalated":   esc,
	}
}

// fileExists is a presence check (it drives the derived HasPlan observation).
// It is deliberately distinct from readFile's best-effort content read: a plan's
// existence decides the loop's next action, while its content is only ever
// needed as best-effort re-plan feedback.
func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
