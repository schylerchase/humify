package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	hexec "humify-ng/internal/exec"
	"humify-ng/internal/intel"
	"humify-ng/internal/layout"
	"humify-ng/internal/output"
	"humify-ng/internal/worktree"
)

// cmdExecute advances execution one wave at a time. A wave's planned-but-unbuilt
// slices are each forked into an isolated worktree and handed an executor prompt
// (fork phase); once forked, the next call runs the fail-closed merge barrier,
// the build/test gate, and dispatches verifiers (merge phase). Like every other
// stage the binary only orchestrates and writes prompts — the orchestrator
// spawns the dispatched agents and re-runs to advance.
func cmdExecute(opts options) int {
	root := opts.root
	if root == "" {
		if found, ok := layout.FindRoot(opts.path); ok {
			root = found
		} else {
			root = "."
		}
	}
	if !isGitRepo(root) {
		return fail(opts, "not_a_git_repo", exitError,
			"execute requires a git repository at the project root ("+root+"); it forks worktrees there")
	}
	in, err := intel.Load(root)
	if err != nil {
		reason := "intel_error"
		if err == intel.ErrNotExist {
			reason = "no_intel"
		}
		return fail(opts, reason, exitError, err.Error())
	}

	planned, executed := scanPlanState(root)
	waveIdx, waveSlices, done := hexec.CurrentWave(in.Waves, planned, executed)
	if done {
		return emitExecuteDone(opts, len(executed) > 0)
	}

	g := worktree.NewGit(root)
	manifest, err := hexec.LoadManifest(root)
	if err != nil {
		return fail(opts, "manifest_error", exitError, "load exec manifest: "+err.Error())
	}
	forked := map[string]bool{}
	for _, e := range manifest {
		forked[e.SliceID] = true
	}

	var toFork []string
	for _, id := range waveSlices {
		if !forked[id] {
			toFork = append(toFork, id)
		}
	}
	if len(toFork) > 0 {
		return forkPhase(opts, root, g, in.Target, waveIdx, toFork, manifest)
	}
	return mergePhase(opts, root, g, waveIdx, manifest)
}

// forkPhase creates an isolated worktree+branch per un-forked slice and writes
// its executor prompt, then exits for the orchestrator to spawn the executors.
func forkPhase(opts options, root string, g worktree.Git, target string, waveIdx int, toFork []string, manifest []worktree.Entry) int {
	if err := os.MkdirAll(filepath.Join(layout.TmpDir(root), "executors"), 0o755); err != nil {
		return fail(opts, "dispatch_error", exitError, err.Error())
	}
	for _, id := range toFork {
		wt := layout.WorktreeDir(root, id)
		entry, err := worktree.Fork(g, id, wt)
		if err != nil {
			return fail(opts, "fork_error", exitError, "fork "+id+": "+err.Error())
		}
		manifest = append(manifest, entry)
		job := hexec.ExecutorJob{
			AreaID: id, Worktree: wt, Branch: entry.Branch,
			PlanRel: layout.AreaPlanRel(id), SummaryRel: layout.AreaSummaryRel(id),
		}
		dest := filepath.Join(layout.TmpDir(root), "executors", id+".prompt.md")
		if err := os.WriteFile(dest, []byte(hexec.RenderExecutorPrompt(job)), 0o644); err != nil {
			return fail(opts, "dispatch_error", exitError, err.Error())
		}
	}
	if err := hexec.SaveManifest(root, manifest); err != nil {
		return fail(opts, "manifest_error", exitError, "save exec manifest: "+err.Error())
	}
	data := map[string]any{"status": "forked", "wave": waveIdx, "forked": toFork}
	if opts.json {
		output.EmitJSON(os.Stdout, output.Result{Ok: true, ReasonCode: "forked", Data: data})
		return exitOK
	}
	fmt.Printf("execute wave %d: forked %d worktree(s): %s\n", waveIdx, len(toFork), strings.Join(toFork, " "))
	fmt.Printf("wrote executor prompts under %s\n", filepath.Join(layout.Dir, "tmp", "executors"))
	fmt.Println("next: spawn one executor per prompt (each works in its worktree, commits on its slice branch),")
	fmt.Println("      then re-run `humify execute` to merge the wave")
	return exitOK
}

// mergePhase runs the fail-closed barrier over the wave's worktrees, records the
// merge commits for undo, runs the build/test gate, and dispatches verifiers.
func mergePhase(opts options, root string, g worktree.Git, waveIdx int, manifest []worktree.Entry) int {
	base, err := g.Head()
	if err != nil {
		return fail(opts, "git_error", exitError, "read HEAD: "+err.Error())
	}
	res := worktree.MergeWave(g, manifest, base)

	// Record whatever merged (even on a block) so undo can revert it.
	if recs := commitRecords(res.Merged, waveIdx); len(recs) > 0 {
		if err := hexec.AppendCommits(root, recs); err != nil {
			return fail(opts, "manifest_error", exitError, "record commits: "+err.Error())
		}
	}

	if res.Blocked != nil {
		// Keep only the blocked + pending entries so a re-run retries them.
		_ = hexec.SaveManifest(root, remainingEntries(manifest, res))
		return emitMergeBlocked(opts, waveIdx, res)
	}
	if err := hexec.ClearManifest(root); err != nil {
		return fail(opts, "manifest_error", exitError, "clear manifest: "+err.Error())
	}

	if opts.testCmd != "" {
		if code, err := runGate(root, opts.testCmd); err != nil || code != 0 {
			return emitGateFailed(opts, waveIdx, opts.testCmd, code, err)
		}
	}
	dispatchVerifiers(root, res.Merged)
	return emitWaveMerged(opts, waveIdx, res, opts.testCmd != "")
}

func commitRecords(merged []worktree.Merged, wave int) []hexec.CommitRecord {
	recs := make([]hexec.CommitRecord, 0, len(merged))
	for _, m := range merged {
		recs = append(recs, hexec.CommitRecord{SliceID: m.SliceID, Wave: wave, CommitSHA: m.CommitSHA})
	}
	return recs
}

// remainingEntries returns the manifest entries that did NOT merge (the blocked
// slice plus everything pending), so the next run retries exactly those.
func remainingEntries(manifest []worktree.Entry, res worktree.WaveResult) []worktree.Entry {
	merged := map[string]bool{}
	for _, m := range res.Merged {
		merged[m.SliceID] = true
	}
	var rem []worktree.Entry
	for _, e := range manifest {
		if !merged[e.SliceID] {
			rem = append(rem, e)
		}
	}
	return rem
}

func dispatchVerifiers(root string, merged []worktree.Merged) {
	if len(merged) == 0 {
		return
	}
	dir := filepath.Join(layout.TmpDir(root), "verifiers")
	if os.MkdirAll(dir, 0o755) != nil {
		return
	}
	for _, m := range merged {
		job := hexec.VerifierJob{
			AreaID: m.SliceID, Repo: root,
			PlanRel: layout.AreaPlanRel(m.SliceID), SummaryRel: layout.AreaSummaryRel(m.SliceID),
			VerifyRel: layout.AreaVerifyRel(m.SliceID),
		}
		_ = os.WriteFile(filepath.Join(dir, m.SliceID+".prompt.md"), []byte(hexec.RenderVerifierPrompt(job)), 0o644)
	}
}

// runGate runs the build/test command in root through the platform shell,
// returning its exit code. testCmd is intentionally shell-interpreted: it is an
// operator-supplied --test-cmd (e.g. "go build ./... && go test ./...") from the
// trusted invoker, NOT sourced from repo content. Do not wire this to a value
// read out of the target repository without sandboxing — that would be a command
// injection vector.
func runGate(root, testCmd string) (int, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", testCmd)
	} else {
		cmd = exec.Command("sh", "-c", testCmd)
	}
	cmd.Dir = root
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr // surface gate output to the user
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), nil
	}
	return -1, err
}

func isGitRepo(root string) bool {
	_, err := os.Stat(filepath.Join(root, ".git"))
	return err == nil
}

// scanPlanState derives, per area, whether it has a PLAN.md (planned) and a
// SUMMARY.md (executed) on disk.
func scanPlanState(root string) (planned, executed map[string]bool) {
	planned, executed = map[string]bool{}, map[string]bool{}
	ids, _ := layout.DiscoverAreas(root)
	for _, id := range ids {
		if fileExists(layout.AreaPlan(root, id)) {
			planned[id] = true
		}
		if fileExists(layout.AreaSummary(root, id)) {
			executed[id] = true
		}
	}
	return planned, executed
}

func emitExecuteDone(opts options, anyExecuted bool) int {
	reason, msg := "nothing_to_execute", "nothing to execute — run `humify plan` first"
	if anyExecuted {
		reason, msg = "execution_complete", "execution complete — run `humify patchlog` to roll up the changes"
	}
	if opts.json {
		output.EmitJSON(os.Stdout, output.Result{Ok: true, ReasonCode: reason})
		return exitOK
	}
	fmt.Println(msg)
	return exitOK
}

func emitMergeBlocked(opts options, waveIdx int, res worktree.WaveResult) int {
	if opts.json {
		output.EmitJSON(os.Stdout, output.Result{Ok: false, ReasonCode: "merge_blocked", Data: res})
		return exitDrift
	}
	b := res.Blocked
	fmt.Printf("execute wave %d BLOCKED at slice %s (gate: %s)\n  %s\n", waveIdx, b.SliceID, b.Gate, b.Reason)
	if len(res.Merged) > 0 {
		fmt.Printf("merged before the block: %d slice(s) — recorded for `humify undo`\n", len(res.Merged))
	}
	if len(res.Pending) > 0 {
		fmt.Printf("pending (not attempted): %s\n", strings.Join(res.Pending, " "))
	}
	fmt.Println("fix the blocked slice's worktree/branch, then re-run `humify execute`")
	return exitDrift
}

func emitGateFailed(opts options, waveIdx int, testCmd string, code int, err error) int {
	if opts.json {
		output.EmitJSON(os.Stdout, output.Result{Ok: false, ReasonCode: "gate_failed",
			Data: map[string]any{"wave": waveIdx, "test_cmd": testCmd, "exit": code}})
		return exitDrift
	}
	if err != nil {
		fmt.Printf("execute wave %d merged, but the build/test gate could not run (%q): %v\n", waveIdx, testCmd, err)
	} else {
		fmt.Printf("execute wave %d merged, but the build/test gate FAILED (%q exited %d)\n", waveIdx, testCmd, code)
	}
	fmt.Println("the merge is in place — fix forward, or `humify undo` to revert this wave's commits")
	return exitDrift
}

func emitWaveMerged(opts options, waveIdx int, res worktree.WaveResult, gated bool) int {
	if opts.json {
		output.EmitJSON(os.Stdout, output.Result{Ok: true, ReasonCode: "wave_merged", Data: res})
		return exitOK
	}
	gate := "no test-cmd gate"
	if gated {
		gate = "build/test gate passed"
	}
	fmt.Printf("execute wave %d: merged %d slice(s), %s\n", waveIdx, len(res.Merged), gate)
	fmt.Printf("wrote verifier prompts under %s (advisory)\n", filepath.Join(layout.Dir, "tmp", "verifiers"))
	fmt.Println("next: optionally spawn verifiers, then re-run `humify execute` for the next wave")
	return exitOK
}
