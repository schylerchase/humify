package main

import (
	"fmt"
	"os"

	hexec "humify-ng/internal/exec"
	"humify-ng/internal/layout"
	"humify-ng/internal/output"
	"humify-ng/internal/worktree"
)

// cmdUndo rolls back execute's merges by reverting each recorded merge commit,
// newest first, with `git revert` (never `git reset` — history is preserved and
// the undo is itself reversible). On full success it clears the commit log so a
// second undo does not double-revert. A revert that conflicts stops the run with
// the offending commit, leaving the repo for the user to resolve.
func cmdUndo(opts options) int {
	root := opts.root
	if root == "" {
		if found, ok := layout.FindRoot(opts.path); ok {
			root = found
		} else {
			root = "."
		}
	}
	if !isGitRepo(root) {
		return fail(opts, "not_a_git_repo", exitError, "undo requires a git repository at "+root)
	}
	commits, err := hexec.LoadCommits(root)
	if err != nil {
		return fail(opts, "manifest_error", exitError, "load commit log: "+err.Error())
	}
	if len(commits) == 0 {
		if opts.json {
			output.EmitJSON(os.Stdout, output.Result{Ok: true, ReasonCode: "nothing_to_undo"})
			return exitOK
		}
		fmt.Println("nothing to undo — no recorded execute merges")
		return exitOK
	}

	g := worktree.NewGit(root)
	var reverted []string
	for i := len(commits) - 1; i >= 0; i-- { // newest first
		c := commits[i]
		if err := g.RevertMerge(c.CommitSHA); err != nil {
			return emitUndoConflict(opts, c, reverted, err)
		}
		reverted = append(reverted, c.SliceID)
	}
	// All reverted cleanly: drop the log so the next undo is a no-op.
	if err := hexec.ClearCommits(root); err != nil {
		return fail(opts, "manifest_error", exitError, "clear commit log: "+err.Error())
	}
	if opts.json {
		output.EmitJSON(os.Stdout, output.Result{Ok: true, ReasonCode: "undone",
			Data: map[string]any{"reverted": reverted}})
		return exitOK
	}
	fmt.Printf("reverted %d merge commit(s) (newest first); commit log cleared\n", len(reverted))
	fmt.Println("the reverts are new commits — history is intact and the undo is itself reversible")
	return exitOK
}

func emitUndoConflict(opts options, c hexec.CommitRecord, reverted []string, err error) int {
	if opts.json {
		output.EmitJSON(os.Stdout, output.Result{Ok: false, ReasonCode: "revert_conflict",
			Data: map[string]any{"failed_slice": c.SliceID, "commit": c.CommitSHA, "reverted_before": reverted}})
		return exitDrift
	}
	fmt.Printf("undo stopped at slice %s (commit %s): %v\n", c.SliceID, c.CommitSHA, err)
	fmt.Printf("reverted before the conflict: %d slice(s). Resolve the conflict, then re-run `humify undo`.\n", len(reverted))
	return exitDrift
}
