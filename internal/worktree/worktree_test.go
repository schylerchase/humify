package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// --- mock Git --------------------------------------------------------------

type mockGit struct {
	head         string
	addErr       error
	exists       map[string]bool   // branch -> exists (default true)
	tipVal       map[string]string // branch -> tip sha (default "committed", != base)
	mergeBaseVal map[string]string // branch -> merge-base (default defaultBase)
	defaultBase  string
	deletedVal   map[string][]string // branch -> deleted files
	cleanVal     map[string]bool     // worktree path -> clean (default true)
	mergeErr     map[string]bool     // branch -> merge fails
	shaSeq       int
	mergeCalls   []string
	removeCalls  []string
	delBranches  []string
	reverted     []string
	aborted      int
	inMerge      bool
}

func newMock() *mockGit {
	return &mockGit{
		head: "base0", defaultBase: "base0",
		exists: map[string]bool{}, tipVal: map[string]string{}, mergeBaseVal: map[string]string{},
		deletedVal: map[string][]string{}, cleanVal: map[string]bool{}, mergeErr: map[string]bool{},
	}
}

func (m *mockGit) Head() (string, error) { return m.head, nil }
func (m *mockGit) AddWorktree(path, branch, base string) error {
	return m.addErr
}
func (m *mockGit) BranchExists(b string) bool {
	if v, ok := m.exists[b]; ok {
		return v
	}
	return true
}
func (m *mockGit) Tip(b string) (string, error) {
	if v, ok := m.tipVal[b]; ok {
		return v, nil
	}
	return "committed", nil // default: ahead of base0, so the no-commit gate passes
}
func (m *mockGit) MergeBase(a, b string) (string, error) {
	if v, ok := m.mergeBaseVal[b]; ok {
		return v, nil
	}
	return m.defaultBase, nil
}
func (m *mockGit) DeletedFiles(base, branch string) ([]string, error) {
	return m.deletedVal[branch], nil
}
func (m *mockGit) IsClean(path string) (bool, error) {
	if v, ok := m.cleanVal[path]; ok {
		return v, nil
	}
	return true, nil
}
func (m *mockGit) Merge(branch, msg string) (string, error) {
	m.mergeCalls = append(m.mergeCalls, branch)
	if m.mergeErr[branch] {
		return "", fmt.Errorf("merge conflict")
	}
	m.shaSeq++
	return fmt.Sprintf("sha%d", m.shaSeq), nil
}
func (m *mockGit) AbortMerge() error    { m.aborted++; return nil }
func (m *mockGit) InMerge() bool        { return m.inMerge }
func (m *mockGit) RemoveWorktree(path string) error {
	m.removeCalls = append(m.removeCalls, path)
	return nil
}
func (m *mockGit) DeleteBranch(b string) error  { m.delBranches = append(m.delBranches, b); return nil }
func (m *mockGit) RevertMerge(sha string) error { m.reverted = append(m.reverted, sha); return nil }

func ent(id string) Entry {
	return Entry{SliceID: id, WorktreePath: "/wt/" + id, Branch: BranchFor(id), ExpectedBase: "base0"}
}

// --- MergeWave: happy path + fail-closed -----------------------------------

func TestMergeWaveAllClean(t *testing.T) {
	m := newMock()
	r := MergeWave(m, []Entry{ent("01-a"), ent("02-b")}, "base0")
	if !r.OK() || len(r.Merged) != 2 || len(r.Pending) != 0 {
		t.Fatalf("want 2 merged, 0 pending, ok; got %+v", r)
	}
	if len(m.mergeCalls) != 2 || len(m.removeCalls) != 2 || len(m.delBranches) != 2 {
		t.Fatalf("each merged slice must merge+remove+delete; merges=%v removes=%v dels=%v",
			m.mergeCalls, m.removeCalls, m.delBranches)
	}
}

// The first failure stops the barrier: earlier slices stay merged, the failing
// slice is Blocked, and every later slice is Pending and never touched.
func TestMergeWaveFailClosedStopsAndPends(t *testing.T) {
	m := newMock()
	m.cleanVal["/wt/02-b"] = false // second slice has a dirty worktree
	r := MergeWave(m, []Entry{ent("01-a"), ent("02-b"), ent("03-c")}, "base0")
	if r.OK() {
		t.Fatal("wave must not be OK when a slice blocks")
	}
	if r.Blocked.SliceID != "02-b" || r.Blocked.Gate != "clean-worktree" {
		t.Fatalf("blocked = %+v, want 02-b at clean-worktree", r.Blocked)
	}
	if len(r.Merged) != 1 || r.Merged[0].SliceID != "01-a" {
		t.Fatalf("merged = %+v, want only 01-a", r.Merged)
	}
	if len(r.Pending) != 1 || r.Pending[0] != "03-c" {
		t.Fatalf("pending = %v, want [03-c]", r.Pending)
	}
	if len(m.mergeCalls) != 1 {
		t.Fatalf("only 01-a should have merged; merges=%v (03-c merged past a block?)", m.mergeCalls)
	}
}

func TestMergeWaveGates(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*mockGit, *Entry)
		gate  string
	}{
		{"branch-name", func(m *mockGit, e *Entry) { e.Branch = "rogue-branch" }, "branch-name"},
		{"branch-exists", func(m *mockGit, e *Entry) { m.exists[e.Branch] = false }, "branch-exists"},
		{"merge-base", func(m *mockGit, e *Entry) { m.mergeBaseVal[e.Branch] = "different" }, "merge-base"},
		{"no-commit", func(m *mockGit, e *Entry) { m.tipVal[e.Branch] = e.ExpectedBase }, "no-commit"},
		{"no-deletions", func(m *mockGit, e *Entry) { m.deletedVal[e.Branch] = []string{"gone.go"} }, "no-deletions"},
		{"clean-worktree", func(m *mockGit, e *Entry) { m.cleanVal[e.WorktreePath] = false }, "clean-worktree"},
		{"merge", func(m *mockGit, e *Entry) { m.mergeErr[e.Branch] = true }, "merge"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := newMock()
			e := ent("01-a")
			c.setup(m, &e)
			r := MergeWave(m, []Entry{e}, "base0")
			if r.OK() || r.Blocked == nil {
				t.Fatalf("%s: expected a block, got %+v", c.name, r)
			}
			if r.Blocked.Gate != c.gate {
				t.Fatalf("%s: blocked at gate %q, want %q (reason: %s)", c.name, r.Blocked.Gate, c.gate, r.Blocked.Reason)
			}
			if len(r.Merged) != 0 {
				t.Fatalf("%s: nothing should merge when the only slice blocks", c.name)
			}
		})
	}
}

func TestMergeWaveConflictAbortsAndBlocks(t *testing.T) {
	m := newMock()
	m.mergeErr[BranchFor("01-a")] = true
	r := MergeWave(m, []Entry{ent("01-a"), ent("02-b")}, "base0")
	if r.OK() || r.Blocked == nil {
		t.Fatal("want blocked, got ok")
	}
	if r.Blocked.Gate != "merge" {
		t.Fatalf("gate = %q, want merge", r.Blocked.Gate)
	}
	if m.aborted != 1 {
		t.Fatalf("AbortMerge must be called once after conflict; got %d", m.aborted)
	}
}

func TestMergeWaveBlocksOnStaleMergeHead(t *testing.T) {
	m := newMock()
	m.inMerge = true
	r := MergeWave(m, []Entry{ent("01-a"), ent("02-b")}, "base0")
	if r.OK() || r.Blocked == nil {
		t.Fatal("want blocked when MERGE_HEAD present")
	}
	if r.Blocked.Gate != "mid-merge" {
		t.Fatalf("gate = %q, want mid-merge", r.Blocked.Gate)
	}
	if len(r.Pending) != 2 {
		t.Fatalf("all entries should be pending; got %v", r.Pending)
	}
	if len(m.mergeCalls) != 0 {
		t.Fatal("no merges should be attempted when already mid-merge")
	}
}

// --- Fork ------------------------------------------------------------------

func TestForkRecordsBaseAndBranch(t *testing.T) {
	m := newMock()
	m.head = "abc123"
	e, err := Fork(m, "01-a", "/wt/01-a")
	if err != nil {
		t.Fatal(err)
	}
	if e.ExpectedBase != "abc123" || e.Branch != "humify-slice-01-a" || e.WorktreePath != "/wt/01-a" {
		t.Fatalf("fork entry = %+v", e)
	}
}

func TestForkPropagatesAddError(t *testing.T) {
	m := newMock()
	m.addErr = fmt.Errorf("path exists")
	if _, err := Fork(m, "01-a", "/wt/01-a"); err == nil {
		t.Fatal("Fork must propagate AddWorktree error")
	}
}

// --- real git integration --------------------------------------------------

func TestMergeWaveRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	repo := t.TempDir()
	gitT(t, repo, "init", "-q")
	gitT(t, repo, "config", "user.email", "t@example.com")
	gitT(t, repo, "config", "user.name", "t")
	gitT(t, repo, "config", "commit.gpgsign", "false")
	writeFile(t, filepath.Join(repo, "base.txt"), "base\n")
	gitT(t, repo, "add", "-A")
	gitT(t, repo, "commit", "-q", "-m", "base")

	g := NewGit(repo)
	base, err := g.Head()
	if err != nil {
		t.Fatal(err)
	}

	wt := filepath.Join(t.TempDir(), "wt-01") // must not exist yet
	e, err := Fork(g, "01-a", wt)
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	// Do real work in the slice worktree and commit it on the slice branch.
	writeFile(t, filepath.Join(wt, "added.txt"), "from slice\n")
	gitT(t, wt, "add", "-A")
	gitT(t, wt, "commit", "-q", "-m", "slice work")

	r := MergeWave(g, []Entry{e}, base)
	if !r.OK() {
		t.Fatalf("real merge blocked: %+v", r.Blocked)
	}
	// The slice's file is now in the repo's working tree, the branch is gone,
	// and the worktree was removed.
	if _, err := os.Stat(filepath.Join(repo, "added.txt")); err != nil {
		t.Fatalf("merged file missing from repo: %v", err)
	}
	if g.BranchExists(e.Branch) {
		t.Fatal("slice branch should be deleted after merge")
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree dir should be removed, stat err=%v", err)
	}
}

func gitT(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
