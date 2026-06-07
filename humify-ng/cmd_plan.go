package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"humify-ng/internal/area"
	"humify-ng/internal/consolidate"
	"humify-ng/internal/intel"
	"humify-ng/internal/layout"
	"humify-ng/internal/output"
	"humify-ng/internal/plan"
	"humify-ng/internal/plancheck"
)

// cmdPlan advances the plan stage's convergence loop by one round. It reads the
// consolidated findings (refusing to plan a half-audited project), derives each
// finding-bearing area's plan state from disk, and lets plan.Decide pick the
// round's actions: dispatch planners/checkers, or report converged/escalated.
// Like audit, the binary only writes prompts — the orchestrator spawns the
// dispatched agents and re-runs `humify plan` to take the next step.
func cmdPlan(opts options) int {
	root := opts.root
	if root == "" {
		if found, ok := layout.FindRoot(opts.path); ok {
			root = found
		} else {
			root = "."
		}
	}
	res, err := consolidate.Run(root)
	if err != nil {
		return fail(opts, consolidateReason(err), exitError, err.Error())
	}
	if len(res.Pending) > 0 {
		return fail(opts, "audit_incomplete", exitDrift, fmt.Sprintf(
			"audit incomplete: %d area(s) not consolidated — finish `humify audit` + `humify consolidate` first",
			len(res.Pending)))
	}
	in, err := intel.Load(root)
	if err != nil {
		reason := "intel_error"
		if err == intel.ErrNotExist {
			reason = "no_intel"
		}
		return fail(opts, reason, exitError, err.Error())
	}

	byArea := groupFindings(res.Findings)
	targets := findingTargets(byArea)
	if len(targets) == 0 {
		return emitPlanNothing(opts)
	}

	st, err := plan.Load(root)
	if err != nil {
		return fail(opts, "state_error", exitError, "load loop state: "+err.Error())
	}
	if opts.maxReplans > 0 {
		st.MaxReplans = opts.maxReplans
	}
	st.Reconcile(targets)

	d := plan.Decide(observe(root, targets), &st)
	// Dispatch (which deletes stale verdicts for re-plans) BEFORE persisting the
	// bumped replan counters. A bumped counter must never reach disk while the
	// stale verdict it assumes-deleted is still there: that pairing would make
	// the next run false-stall (Replans>0 && Issues unchanged) and escalate an
	// area that never actually re-planned. With save last, a failed/crashed
	// dispatch simply leaves state un-bumped and the round replays cleanly.
	if err := dispatchPlan(root, in.Target, in.AreasByID(), byArea, d); err != nil {
		return fail(opts, "dispatch_error", exitError, "dispatch failed: "+err.Error())
	}
	if err := st.Save(root); err != nil {
		return fail(opts, "state_error", exitError, "save loop state: "+err.Error())
	}
	return emitPlan(opts, d)
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

func findingTargets(byArea map[string][]plan.Finding) []string {
	var ids []string
	for id := range byArea {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// observe derives each target's plan state from disk: PLAN.md presence, a valid
// PLAN-CHECK.json verdict, and its blocking-issue count. An unreadable or
// invalid check counts as "no verdict" so the checker is simply re-dispatched.
func observe(root string, targets []string) []plan.Obs {
	obs := make([]plan.Obs, 0, len(targets))
	for _, id := range targets {
		o := plan.Obs{AreaID: id, HasPlan: fileExists(layout.AreaPlan(root, id))}
		if c, err := plancheck.Load(layout.AreaPlanCheck(root, id)); err == nil && c.Validate() == nil {
			o.HasCheck = true
			o.Issues = c.BlockingCount()
		}
		obs = append(obs, o)
	}
	return obs
}

// dispatchPlan writes the prompts the round calls for. Re-plans read the prior
// plan and verdict for feedback, then delete the stale verdict so the next round
// re-checks the fresh plan rather than re-judging the old one.
func dispatchPlan(root, target string, areas map[string]area.Area, byArea map[string][]plan.Finding, d plan.Decision) error {
	replan := map[string]bool{}
	for _, id := range d.Replans {
		replan[id] = true
	}
	if len(d.PlanAreas) > 0 {
		if err := os.MkdirAll(filepath.Join(layout.TmpDir(root), "planners"), 0o755); err != nil {
			return err
		}
	}
	if len(d.CheckAreas) > 0 {
		if err := os.MkdirAll(filepath.Join(layout.TmpDir(root), "plan-checkers"), 0o755); err != nil {
			return err
		}
	}
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
		dest := filepath.Join(layout.TmpDir(root), "planners", id+".prompt.md")
		if err := os.WriteFile(dest, []byte(plan.RenderPlannerPrompt(job)), 0o644); err != nil {
			return err
		}
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
		dest := filepath.Join(layout.TmpDir(root), "plan-checkers", id+".prompt.md")
		if err := os.WriteFile(dest, []byte(plan.RenderCheckerPrompt(job)), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func emitPlan(opts options, d plan.Decision) int {
	ok := d.Status != plan.StatusEscalated
	code := exitOK
	if !ok {
		code = exitDrift
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

func emitPlanNothing(opts options) int {
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
