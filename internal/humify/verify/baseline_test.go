package verify

import (
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func requireGit(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("git inspectors shell out POSIX-style; skip on windows")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func initRepo(t *testing.T, root string) {
	t.Helper()
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "t@e.x")
	runGit(t, root, "config", "user.name", "t")
	runGit(t, root, "config", "commit.gpgsign", "false")
}

func TestSaveLoadBaselineRoundTrip(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	if err := SaveBaseline(root, val(cfail("test"), cpass("build")), false, now); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}
	snap, ok := LoadBaseline(root)
	if !ok {
		t.Fatal("LoadBaseline: not found after save")
	}
	if snap.SavedAt != now.Format(time.RFC3339) {
		t.Errorf("SavedAt = %q, want %q", snap.SavedAt, now.Format(time.RFC3339))
	}
	if len(snap.Result.Commands) != 2 || snap.Result.Commands[0].Kind != "test" {
		t.Errorf("Result not round-tripped: %+v", snap.Result.Commands)
	}
}

func TestLoadBaselineAbsentIsNotFound(t *testing.T) {
	if _, ok := LoadBaseline(t.TempDir()); ok {
		t.Error("LoadBaseline should report not-found in a fresh dir")
	}
}

func TestBaselineStaleOnHeadChange(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	initRepo(t, root)
	writeFile(t, root, "a.txt", "one\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "first")
	if err := SaveBaseline(root, val(cpass("test")), false, time.Now()); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}
	snap, ok := LoadBaseline(root)
	if !ok {
		t.Fatal("LoadBaseline: not found")
	}
	if BaselineStale(snap, root) {
		t.Error("baseline must not be stale before any new commit")
	}
	writeFile(t, root, "a.txt", "two\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "second")
	if !BaselineStale(snap, root) {
		t.Error("baseline must be stale after a new commit landed")
	}
}

func TestSaveBaselineStoresDirtyFlag(t *testing.T) {
	root := t.TempDir()
	if err := SaveBaseline(root, val(cpass("test")), true, time.Now()); err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}
	snap, _ := LoadBaseline(root)
	if !snap.Dirty {
		t.Error("SaveBaseline must persist the dirty flag the caller passed")
	}
}

// TestRepoDirtyExcludingHumify covers the extracted inspector directly: a clean
// committed tree is not dirty, an uncommitted edit to a tracked file is, and dirt
// confined to .humify/ does not count (it is humify's own state).
func TestRepoDirtyExcludingHumify(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	initRepo(t, root)
	writeFile(t, root, "a.txt", "one\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "first")
	if RepoDirtyExcludingHumify(root) {
		t.Error("a freshly-committed tree must not be dirty")
	}
	writeFile(t, root, ".humify/analysis.json", "{}\n")
	if RepoDirtyExcludingHumify(root) {
		t.Error("dirt confined to .humify/ must not count as user dirt")
	}
	writeFile(t, root, "a.txt", "edited\n")
	if !RepoDirtyExcludingHumify(root) {
		t.Error("an uncommitted edit to a tracked file must be dirty")
	}
}
