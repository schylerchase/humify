package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"humify/internal/audit"
	"humify/internal/consolidate"
	hexec "humify/internal/exec"
	"humify/internal/intel"
	"humify/internal/layout"
	"humify/internal/pipeline"
	"humify/internal/plan"
	"humify/internal/plancheck"
	"humify/internal/spawn"
	"humify/internal/worktree"
)

// advanceStage runs exactly one stage and returns its record. It dispatches on
// the stage pipeline.Next named — deterministic stages (consolidate, patchlog)
// just run; agent stages (audit, plan, execute) spawn through the shared
// primitive. It advances at most one step; the driver loop re-derives the next.
func advanceStage(root string, stage pipeline.Stage, opts options, cfg spawn.Config) (runStep, error) {
	switch stage {
	case pipeline.StageAudit:
		return advanceAudit(root, cfg)
	case pipeline.StageConsolidate:
		return advanceConsolidate(root)
	case pipeline.StagePlan:
		return advancePlan(root, opts, cfg)
	case pipeline.StageExecute:
		return advanceExecute(root, opts, cfg)
	case pipeline.StagePatchlog:
		return advancePatchlog(root)
	default:
		return runStep{}, fmt.Errorf("driver cannot advance stage %q", stage)
	}
}

// advanceAudit fans out one auditor per pending area through the spawn runner and
// reports how many produced a valid fragment. A deterministically-failing auditor
// leaves its area pending; the driver's no-progress guard (not this call) stops it.
func advanceAudit(root string, cfg spawn.Config) (runStep, error) {
	p, err := audit.BuildPlan(root)
	if err != nil {
		return runStep{}, err
	}
	out, err := audit.SpawnRunner{AgentCmd: cfg.AgentCmd, Jobs: cfg.Jobs, Timeout: cfg.Timeout}.Dispatch(p)
	if err != nil {
		return runStep{}, err
	}
	return runStep{Stage: "audit", Spawned: out.Spawned, Succeeded: out.Succeeded, Failed: out.Failed}, nil
}

// advanceConsolidate runs the deterministic fan-in (gather fragments → AUDIT.md).
// The target is the heatmap target from intel (cosmetic header only); an absent
// intel just yields an empty header, never a failure.
func advanceConsolidate(root string) (runStep, error) {
	res, err := consolidate.Run(root)
	if err != nil {
		return runStep{}, err
	}
	in, _ := intel.Load(root)
	if err := writeAudit(root, in.Target, res); err != nil {
		return runStep{}, err
	}
	return runStep{Stage: "consolidate", Note: fmt.Sprintf("%d area(s) gathered into AUDIT.md", len(res.Covered))}, nil
}

// advancePlan runs one round of the plan convergence loop and spawns whatever it
// dispatched (planners + checkers, each cwd = root). A converged/escalated round
// dispatches nothing; escalation surfaces as Blocked through nextActionable on the
// next iteration (its guard reads the same loop state Decide bumped here).
func advancePlan(root string, opts options, cfg spawn.Config) (runStep, error) {
	d, jobs, nothing, err := planRound(root, opts.maxReplans)
	if err != nil {
		return runStep{}, err
	}
	if nothing || d.Status != plan.StatusDispatch {
		return runStep{Stage: "plan", Note: d.Status}, nil
	}
	res := spawn.Run(jobs, cfg, planJobSucceeded(root, d))
	return runStep{Stage: "plan", Spawned: res.Spawned, Succeeded: res.Succeeded, Failed: res.Failed}, nil
}

// planJobSucceeded judges each dispatched plan job by the artifact it should have
// produced: a checker by a valid PLAN-CHECK.json, a planner by a PLAN.md on disk.
// Planners and checkers are disjoint within a round (Decide puts each area in at
// most one of PlanAreas/CheckAreas), so the id alone selects the right check.
//
// Note: for a RE-plan the PLAN.md already exists, so the planner check is trivially
// true — the trace's "succeeded" count is thus optimistic for re-plans. That is
// cosmetic only: Succeeded never gates the loop, and a re-planner that did nothing
// still terminates via Decide's stall/budget arms (issues fail to drop → escalated).
func planJobSucceeded(root string, d plan.Decision) func(string) bool {
	check := map[string]bool{}
	for _, id := range d.CheckAreas {
		check[id] = true
	}
	return func(id string) bool {
		if check[id] {
			c, err := plancheck.Load(layout.AreaPlanCheck(root, id))
			return err == nil && c.Validate() == nil
		}
		return fileExists(layout.AreaPlan(root, id))
	}
}

// advanceExecute advances execution one step. If the current wave has un-forked
// slices it forks each into its own worktree and spawns an executor THERE (fork
// phase); otherwise it runs the merge barrier + gate (merge phase). The two-phase
// split is preserved from the manual flow — the driver re-enters until the wave,
// then the pipeline, is done.
func advanceExecute(root string, opts options, cfg spawn.Config) (runStep, error) {
	in, err := intel.Load(root)
	if err != nil {
		return runStep{}, err
	}
	if !isGitRepo(root) {
		return runStep{}, fmt.Errorf("execute requires a git repository at %s (it forks worktrees there)", root)
	}
	planned, executed := hexec.ScanPlanState(root)
	waveIdx, waveSlices, done := hexec.CurrentWave(in.Waves, planned, executed)
	if done {
		return runStep{Stage: "execute", Note: "nothing left to execute"}, nil
	}
	g := worktree.NewGit(root)
	manifest, err := hexec.LoadManifest(root)
	if err != nil {
		return runStep{}, err
	}
	if toFork := unforked(manifest, waveSlices); len(toFork) > 0 {
		return driveFork(root, g, waveIdx, toFork, manifest, cfg)
	}
	return driveMerge(root, g, waveIdx, manifest, opts.testCmd)
}

// unforked returns the wave's slices that have no worktree yet.
func unforked(manifest []worktree.Entry, waveSlices []string) []string {
	forked := map[string]bool{}
	for _, e := range manifest {
		forked[e.SliceID] = true
	}
	var out []string
	for _, id := range waveSlices {
		if !forked[id] {
			out = append(out, id)
		}
	}
	return out
}

// driveFork forks each un-built slice into its own worktree, writes its executor
// prompt, persists the wave manifest, then spawns one executor per slice IN ITS
// WORKTREE (per-job cwd) and barriers. Executor output lands inside the worktrees;
// the next step's merge phase is what brings it into the project and is the real
// success gate.
func driveFork(root string, g worktree.Git, waveIdx int, toFork []string, manifest []worktree.Entry, cfg spawn.Config) (runStep, error) {
	if err := os.MkdirAll(filepath.Join(layout.TmpDir(root), "executors"), 0o755); err != nil {
		return runStep{}, err
	}
	jobs := make([]spawn.Job, 0, len(toFork))
	for _, id := range toFork {
		entry, job, err := forkExecutor(root, g, id)
		if err != nil {
			return runStep{}, err
		}
		manifest = append(manifest, entry)
		jobs = append(jobs, job)
	}
	if err := hexec.SaveManifest(root, manifest); err != nil {
		return runStep{}, err
	}
	res := spawn.Run(jobs, cfg, executorProduced(root))
	return runStep{Stage: "execute", Spawned: res.Spawned, Succeeded: res.Succeeded, Failed: res.Failed,
		Note: fmt.Sprintf("forked + spawned wave %d (%s)", waveIdx, strings.Join(toFork, " "))}, nil
}

// forkExecutor forks one slice's worktree and renders its executor prompt as a
// spawn job (cwd = the worktree). The id is gated FIRST: WorktreeDir writes a
// sibling of the repo that ResolveInRoot cannot cover, so a ".."/separator id must
// be rejected before it is joined into a path.
func forkExecutor(root string, g worktree.Git, id string) (worktree.Entry, spawn.Job, error) {
	if err := guardAreaIDs(id); err != nil {
		return worktree.Entry{}, spawn.Job{}, err
	}
	wt := layout.WorktreeDir(root, id)
	entry, err := worktree.Fork(g, id, wt)
	if err != nil {
		return worktree.Entry{}, spawn.Job{}, fmt.Errorf("fork %s: %w", id, err)
	}
	prompt := hexec.RenderExecutorPrompt(hexec.ExecutorJob{
		AreaID: id, Worktree: wt, Branch: entry.Branch,
		PlanRel: layout.AreaPlanRel(id), SummaryRel: layout.AreaSummaryRel(id),
	})
	dest := filepath.Join(layout.TmpDir(root), "executors", id+".prompt.md")
	if err := os.WriteFile(dest, []byte(prompt), 0o644); err != nil {
		return worktree.Entry{}, spawn.Job{}, err
	}
	return entry, spawn.Job{ID: id, Dir: wt, Prompt: prompt}, nil
}

// executorProduced is the spawn success check: an executor "succeeds" by writing
// its SUMMARY.md in its worktree (which it then commits). Advisory only — the
// merge barrier and the no-progress guard, not this, decide termination.
func executorProduced(root string) func(string) bool {
	return func(id string) bool {
		return fileExists(filepath.Join(layout.WorktreeDir(root, id), layout.AreaSummaryRel(id)))
	}
}

// driveMerge runs the fail-closed merge barrier over the wave's worktrees, records
// the merge commits for undo, runs the optional build/test gate, and writes (but
// does not spawn) the advisory verifier prompts. A merge-barrier block or a failed
// gate returns a Blocked runStep — terminal and human-owned, since the source has
// already changed (fix forward or `humify undo`).
func driveMerge(root string, g worktree.Git, waveIdx int, manifest []worktree.Entry, testCmd string) (runStep, error) {
	base, err := g.Head()
	if err != nil {
		return runStep{}, fmt.Errorf("read HEAD: %w", err)
	}
	res := worktree.MergeWave(g, manifest, base)
	if recs := commitRecords(res.Merged, waveIdx); len(recs) > 0 {
		if err := hexec.AppendCommits(root, recs); err != nil {
			return runStep{}, fmt.Errorf("record commits: %w", err)
		}
	}
	if res.Blocked != nil {
		_ = hexec.SaveManifest(root, remainingEntries(manifest, res)) // retry the blocked + pending slices
		return runStep{Stage: "execute", Blocked: true,
			Note: fmt.Sprintf("merge barrier blocked at %s (%s): %s — fix its worktree/branch, then `humify resume`",
				res.Blocked.SliceID, res.Blocked.Gate, res.Blocked.Reason)}, nil
	}
	if err := hexec.ClearManifest(root); err != nil {
		return runStep{}, fmt.Errorf("clear manifest: %w", err)
	}
	if testCmd != "" {
		if code, gerr := runGate(root, testCmd); gerr != nil || code != 0 {
			return runStep{Stage: "execute", Blocked: true,
				Note: fmt.Sprintf("build/test gate failed after merging wave %d (exit %d) — fix forward or `humify undo`", waveIdx, code)}, nil
		}
	}
	dispatchVerifiers(root, res.Merged) // advisory prompts only; verdicts do not gate the chain
	return runStep{Stage: "execute", Note: fmt.Sprintf("merged wave %d (%d slice(s))", waveIdx, len(res.Merged))}, nil
}

// advancePatchlog rolls every executed area up into PATCHLOG.md — the same
// deterministic, no-agent roll-up the manual command writes, sharing renderPatchlog
// so the autonomous and manual outputs are byte-identical.
func advancePatchlog(root string) (runStep, error) {
	ids, err := layout.DiscoverAreas(root)
	if err != nil {
		return runStep{}, err
	}
	var executed []string
	for _, id := range ids {
		if fileExists(layout.AreaSummary(root, id)) {
			executed = append(executed, id)
		}
	}
	if len(executed) == 0 {
		return runStep{Stage: "patchlog", Note: "nothing executed to roll up"}, nil
	}
	commits, err := hexec.LoadCommits(root)
	if err != nil {
		return runStep{}, err
	}
	shaByArea := map[string]string{}
	for _, c := range commits {
		shaByArea[c.SliceID] = c.CommitSHA
	}
	if err := os.WriteFile(layout.PatchlogFile(root), []byte(renderPatchlog(root, executed, shaByArea)), 0o644); err != nil {
		return runStep{}, err
	}
	return runStep{Stage: "patchlog", Note: fmt.Sprintf("rolled up %d executed area(s)", len(executed))}, nil
}
