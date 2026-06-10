package exec

import (
	"fmt"
	"strings"

	"humify/internal/worktree"
)

// ExecutorJob is one slice's execution assignment, all paths relative to the
// worktree root the executor works inside.
type ExecutorJob struct {
	AreaID     string
	Worktree   string // absolute path to the isolated checkout to work in
	Branch     string // the slice branch already checked out there
	PlanRel    string // PLAN.md path, relative to the worktree
	SummaryRel string // where to write the SUMMARY, relative to the worktree
}

// VerifierJob is one merged slice's verification assignment.
type VerifierJob struct {
	AreaID     string
	Repo       string // the merged repository root to inspect
	PlanRel    string
	SummaryRel string
	VerifyRel  string // where to write the verdict, relative to the repo
}

// RenderExecutorPrompt builds the executor prompt. The executor works inside an
// isolated worktree on a dedicated slice branch, applies the plan, writes a
// SUMMARY, and commits both — committing IS the proof of work, and the merge
// barrier later refuses anything off the slice branch or with a dirty tree.
func RenderExecutorPrompt(j ExecutorJob) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Humify executor — slice %s\n\n", j.AreaID)

	b.WriteString("## Where you work\n")
	fmt.Fprintf(&b, "- Worktree (an isolated git checkout — do ALL work here): `%s`\n", j.Worktree)
	fmt.Fprintf(&b, "- It already has the branch `%s` checked out. Stay on it; do not switch branches.\n", j.Branch)
	fmt.Fprintf(&b, "- The plan to execute: `%s` (relative to the worktree)\n\n", j.PlanRel)

	b.WriteString("## Task\n")
	b.WriteString("Apply the plan to the code in this worktree, one implementation unit at a time. The plan " +
		"is behaviour-preserving: write each unit's characterization test first (locking current behaviour), " +
		"then make the change, and keep the build/tests green as you go. Touch only what the plan calls for.\n\n")

	b.WriteString("## Finish — commit is your proof of work\n")
	fmt.Fprintf(&b, "1. Write a SUMMARY of what you actually did to `%s` (relative to the worktree): which "+
		"units you implemented, the tests you added, and anything you deviated from and why.\n", j.SummaryRel)
	b.WriteString("2. Stage your changes and the SUMMARY, then commit them on the current slice branch.\n")
	fmt.Fprintf(&b, "3. Before committing, confirm you are on `%s` — the merge barrier rejects commits made on "+
		"any other branch. Do NOT merge, rebase, or push; the binary gathers your branch.\n", j.Branch)
	b.WriteString("4. Leave the worktree clean (everything committed). A dirty worktree blocks the merge.\n")
	b.WriteString("5. Do NOT delete files unless the plan explicitly says to — the barrier blocks a slice that " +
		"deletes files, to catch a refactor gone wrong.\n")
	return b.String()
}

// RenderVerifierPrompt builds the adversarial, read-only verifier prompt for a
// merged slice. The verifier checks the SUMMARY's claims against the merged tree
// and judges whether behaviour was preserved, writing a structured verdict.
func RenderVerifierPrompt(j VerifierJob) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Humify verifier — slice %s\n\n", j.AreaID)

	b.WriteString("## Stance\n")
	b.WriteString("You are an adversarial, READ-ONLY verifier of an already-merged refactor. Do not trust the " +
		"SUMMARY — check its claims against the actual merged files. Your job is to catch a change that does " +
		"not do what it says, or that altered behaviour it was supposed to preserve.\n\n")

	fmt.Fprintf(&b, "- Merged repository root: `%s`\n", j.Repo)
	fmt.Fprintf(&b, "- The plan: `%s`\n- The executor's SUMMARY: `%s`\n\n", j.PlanRel, j.SummaryRel)

	b.WriteString("## Check\n")
	b.WriteString("- Each claim in the SUMMARY: is it actually true in the merged files?\n")
	b.WriteString("- Each plan unit: was it really applied, with its characterization test present?\n")
	b.WriteString("- Behaviour preservation: does anything suggest behaviour changed where it should not have?\n\n")

	b.WriteString("## Output — write exactly one file\n")
	fmt.Fprintf(&b, "Write your verdict to `%s` (relative to the repo) and nothing else. JSON of this shape:\n\n", j.VerifyRel)
	b.WriteString("```json\n")
	fmt.Fprintf(&b, `{
  "area_id": %q,
  "claims_checked": 0,
  "passed": true,
  "failed": []
}`, j.AreaID)
	b.WriteString("\n```\n\n")
	b.WriteString("Set `passed` to false and list each problem in `failed` if any claim is false or behaviour " +
		"changed unsafely. A passing verdict must have an empty `failed` list. Do not modify any file.\n")
	return b.String()
}

// BranchFor is re-exported for the command so it labels prompts/worktrees with
// the same branch name the barrier expects.
func BranchFor(sliceID string) string { return worktree.BranchFor(sliceID) }
