// Package worktree owns the execute stage's git orchestration: forking one
// isolated worktree+branch per slice off a fixed base, and the fail-closed merge
// barrier that gathers those branches back. The barrier is the execute-side
// equivalent of the consolidate stage — N parallel refactors become one merged,
// verifiable tree, or the wave blocks loudly with nothing silently dropped.
//
// All git access goes through the Git interface so the barrier's safety logic
// (its gate sequence and fail-closed behaviour) is unit-tested against a scripted
// mock, independent of a real repository. The gitCLI implementation in git.go is
// the production binding.
package worktree

import (
	"fmt"
	"strings"
)

// BranchPrefix names every slice branch. The barrier refuses to merge anything
// not under it: an executor that committed onto the wrong branch is a blocker,
// not something to merge blindly.
const BranchPrefix = "humify-slice-"

// BranchFor returns the slice branch name for an area/slice id.
func BranchFor(sliceID string) string { return BranchPrefix + sliceID }

// Entry is one slice's worktree, recorded at fork time and consumed at the
// barrier. ExpectedBase pins the commit the slice forked from so the barrier can
// detect a branch that was rebased or built on the wrong base.
type Entry struct {
	SliceID      string `json:"slice_id"`
	WorktreePath string `json:"worktree_path"`
	Branch       string `json:"branch"`
	ExpectedBase string `json:"expected_base"`
}

// Git is the minimal git surface the barrier and fork need. Every method that
// can fail returns an error; the barrier treats any error as a failed gate.
type Git interface {
	Head() (string, error)
	AddWorktree(path, branch, base string) error
	BranchExists(branch string) bool
	MergeBase(a, b string) (string, error)
	DeletedFiles(base, branch string) ([]string, error)
	IsClean(worktreePath string) (bool, error)
	Merge(branch, message string) (sha string, err error)
	RemoveWorktree(path string) error
	DeleteBranch(branch string) error
}

// Merged records one slice that merged cleanly, with the resulting merge commit.
type Merged struct {
	SliceID   string `json:"slice_id"`
	CommitSHA string `json:"commit_sha"`
}

// Blocked records the first slice that failed a gate, and why.
type Blocked struct {
	SliceID string `json:"slice_id"`
	Gate    string `json:"gate"`
	Reason  string `json:"reason"`
}

// WaveResult is the outcome of merging one wave's worktrees.
type WaveResult struct {
	Merged  []Merged `json:"merged"`
	Blocked *Blocked `json:"blocked,omitempty"`
	Pending []string `json:"pending,omitempty"` // slice ids after the block, never processed
}

// OK reports whether the whole wave merged with no block.
func (r WaveResult) OK() bool { return r.Blocked == nil }

// Fork creates an isolated worktree+branch for a slice off the current HEAD and
// returns the Entry the barrier will later consume. The recorded ExpectedBase is
// HEAD at fork time, so the barrier can later verify the branch was not moved
// onto a different base.
func Fork(g Git, sliceID, worktreePath string) (Entry, error) {
	base, err := g.Head()
	if err != nil {
		return Entry{}, fmt.Errorf("read HEAD: %w", err)
	}
	branch := BranchFor(sliceID)
	if err := g.AddWorktree(worktreePath, branch, base); err != nil {
		return Entry{}, fmt.Errorf("add worktree for %s: %w", sliceID, err)
	}
	return Entry{SliceID: sliceID, WorktreePath: worktreePath, Branch: branch, ExpectedBase: base}, nil
}

// MergeWave runs the fail-closed merge barrier over one wave's entries, in order.
// Each entry passes a fixed gate sequence before its branch is merged --no-ff
// into baseRef (the wave's accumulating tree) and its worktree/branch cleaned up.
// The FIRST gate failure stops the barrier: that entry is recorded as Blocked and
// every entry after it is returned in Pending, unprocessed. Nothing is merged
// past a failure and nothing is silently dropped — the wave must be repaired and
// re-run.
func MergeWave(g Git, entries []Entry, baseRef string) WaveResult {
	var res WaveResult
	for i, e := range entries {
		sha, gate, reason, ok := mergeOne(g, e, baseRef)
		if !ok {
			res.Blocked = &Blocked{SliceID: e.SliceID, Gate: gate, Reason: reason}
			for _, rest := range entries[i+1:] {
				res.Pending = append(res.Pending, rest.SliceID)
			}
			return res
		}
		res.Merged = append(res.Merged, Merged{SliceID: e.SliceID, CommitSHA: sha})
		// Cleanup is best-effort: the merge already succeeded, so a failed
		// worktree-remove/branch-delete is housekeeping noise, not a lost change.
		_ = g.RemoveWorktree(e.WorktreePath)
		_ = g.DeleteBranch(e.Branch)
	}
	return res
}

// mergeOne runs the read-only safety gates and, if all pass, performs the merge
// exactly once. It returns the merge commit sha and (gate, reason, ok); ok=false
// names the first failed gate and sha is empty.
func mergeOne(g Git, e Entry, baseRef string) (sha, gate, reason string, ok bool) {
	if !strings.HasPrefix(e.Branch, BranchPrefix) {
		return "", "branch-name", fmt.Sprintf("branch %q is not a %s* slice branch", e.Branch, BranchPrefix), false
	}
	if !g.BranchExists(e.Branch) {
		return "", "branch-exists", fmt.Sprintf("branch %q does not exist (executor never committed?)", e.Branch), false
	}
	mb, err := g.MergeBase(baseRef, e.Branch)
	if err != nil {
		return "", "merge-base", "cannot compute merge-base: " + err.Error(), false
	}
	if mb != e.ExpectedBase {
		return "", "merge-base", fmt.Sprintf("merge-base %s != expected base %s (branch moved off its fork point)", short(mb), short(e.ExpectedBase)), false
	}
	dels, err := g.DeletedFiles(e.ExpectedBase, e.Branch)
	if err != nil {
		return "", "no-deletions", "cannot diff for deletions: " + err.Error(), false
	}
	if len(dels) > 0 {
		return "", "no-deletions", fmt.Sprintf("slice deletes %d file(s) (%s) — refactors should transform, not delete; resolve by hand", len(dels), strings.Join(dels, ", ")), false
	}
	clean, err := g.IsClean(e.WorktreePath)
	if err != nil {
		return "", "clean-worktree", "cannot read worktree status: " + err.Error(), false
	}
	if !clean {
		return "", "clean-worktree", "worktree has uncommitted changes (executor did not commit everything)", false
	}
	sha, err = g.Merge(e.Branch, "humify: merge slice "+e.SliceID)
	if err != nil {
		return "", "merge", "merge --no-ff failed (conflict?): " + err.Error(), false
	}
	return sha, "", "", true
}

func short(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
