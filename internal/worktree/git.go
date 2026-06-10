package worktree

import (
	"os/exec"
	"strings"
)

// gitCLI is the production Git binding: it shells out to the git executable
// against a fixed repository directory. Worktree-scoped reads (IsClean) run in
// the worktree path instead, since that is where the slice's changes live.
type gitCLI struct {
	repoDir string
}

// NewGit returns a Git bound to the target repository directory.
func NewGit(repoDir string) Git { return gitCLI{repoDir: repoDir} }

// run executes git in dir and returns trimmed stdout, or an error carrying
// git's stderr so a failed gate reports why.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return "", &gitError{args: args, stderr: strings.TrimSpace(string(ee.Stderr))}
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

type gitError struct {
	args   []string
	stderr string
}

func (e *gitError) Error() string {
	return "git " + strings.Join(e.args, " ") + ": " + e.stderr
}

func (g gitCLI) Head() (string, error) {
	return run(g.repoDir, "rev-parse", "HEAD")
}

func (g gitCLI) AddWorktree(path, branch, base string) error {
	_, err := run(g.repoDir, "worktree", "add", "-b", branch, path, base)
	return err
}

func (g gitCLI) BranchExists(branch string) bool {
	_, err := run(g.repoDir, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

func (g gitCLI) MergeBase(a, b string) (string, error) {
	return run(g.repoDir, "merge-base", a, b)
}

func (g gitCLI) DeletedFiles(base, branch string) ([]string, error) {
	out, err := run(g.repoDir, "diff", "--diff-filter=D", "--name-only", base, branch)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func (g gitCLI) IsClean(worktreePath string) (bool, error) {
	out, err := run(worktreePath, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out == "", nil
}

func (g gitCLI) Merge(branch, message string) (string, error) {
	if _, err := run(g.repoDir, "merge", "--no-ff", "-m", message, branch); err != nil {
		return "", err
	}
	return run(g.repoDir, "rev-parse", "HEAD")
}

func (g gitCLI) AbortMerge() error {
	_, err := run(g.repoDir, "merge", "--abort")
	return err
}

func (g gitCLI) InMerge() bool {
	_, err := run(g.repoDir, "rev-parse", "--verify", "MERGE_HEAD")
	return err == nil
}

func (g gitCLI) RemoveWorktree(path string) error {
	_, err := run(g.repoDir, "worktree", "remove", path)
	return err
}

func (g gitCLI) DeleteBranch(branch string) error {
	_, err := run(g.repoDir, "branch", "-D", branch)
	return err
}

func (g gitCLI) RevertMerge(sha string) error {
	// -m 1 reverts relative to the first parent (the base the slice merged into),
	// which is the only sane mainline for a humify --no-ff slice merge.
	_, err := run(g.repoDir, "revert", "--no-edit", "-m", "1", sha)
	return err
}
