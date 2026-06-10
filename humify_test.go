package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestProductCLIFlow drives the product commands through run() exactly as the
// binary would, asserting exit codes and that the JSON state + quarantine appear.
// The target has no go.mod/package.json, so verify detects no commands and the
// flow needs no external toolchain.
func TestProductCLIFlow(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "old.bak", "obsolete\n")
	writeRepoFile(t, root, "svc.js", "function f() {\n  try { g(); } catch (e) {}\n}\n")

	if code := runSilenced(t, "analyze", root); code != exitOK {
		t.Fatalf("analyze exit = %d, want %d", code, exitOK)
	}
	if !pathThere(filepath.Join(root, ".humify", "analysis.json")) {
		t.Fatal("analyze should write .humify/analysis.json")
	}

	if code := runSilenced(t, "plan", root); code != exitOK {
		t.Fatalf("plan exit = %d", code)
	}
	if !pathThere(filepath.Join(root, ".humify", "plan.json")) {
		t.Fatal("plan should write .humify/plan.json")
	}

	// Dry run must not move the stale file.
	if code := runSilenced(t, "apply", "--target", "HMF-001", "--dry-run", root); code != exitOK {
		t.Fatalf("apply dry-run exit = %d", code)
	}
	if !pathThere(filepath.Join(root, "old.bak")) {
		t.Fatal("dry-run apply must not move files")
	}

	// Confirmed apply quarantines it.
	if code := runSilenced(t, "apply", "--target", "HMF-001", "--yes", root); code != exitOK {
		t.Fatalf("apply --yes exit = %d", code)
	}
	if pathThere(filepath.Join(root, "old.bak")) {
		t.Fatal("apply --yes should have moved old.bak out of the tree")
	}
	if !pathThere(filepath.Join(root, ".humify", "delete-me", "HMF-001", "old.bak")) {
		t.Fatal("old.bak should be quarantined")
	}

	if code := runSilenced(t, "status", root); code != exitOK {
		t.Fatalf("status exit = %d", code)
	}
	if code := runSilenced(t, "doctor", root); code != exitOK {
		t.Fatalf("doctor exit = %d", code)
	}
}

// TestApplyExit2OnRollback proves the exit code end-to-end through the real CLI:
// when a quarantine breaks the build, apply rolls back and the binary exits 2
// (drift) — the only apply path that returns exitDrift. Requires the go toolchain
// because verify runs `go build/vet/test`.
func TestApplyExit2OnRollback(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the go toolchain via verify")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	root := t.TempDir()
	writeRepoFile(t, root, "go.mod", "module rollbackcli\n\ngo 1.26\n")
	writeRepoFile(t, root, "main.go", "package main\n\nfunc main() { println(Greeting()) }\n")
	// Flagged stale by name, but compiled and referenced by main.go.
	writeRepoFile(t, root, "untitled.go", "package main\n\nfunc Greeting() string { return \"hi\" }\n")

	if code := runSilenced(t, "analyze", root); code != exitOK {
		t.Fatalf("analyze exit = %d", code)
	}
	if code := runSilenced(t, "plan", root); code != exitOK {
		t.Fatalf("plan exit = %d", code)
	}
	// HMF-001 is the stale_file quarantine; applying it breaks the build → exit 2.
	if code := runSilenced(t, "apply", "--target", "HMF-001", "--yes", root); code != exitDrift {
		t.Fatalf("apply --yes exit = %d, want exitDrift(%d)", code, exitDrift)
	}
	if !pathThere(filepath.Join(root, "untitled.go")) {
		t.Error("untitled.go must be restored after the rollback")
	}
}

func TestUnknownCommandErrors(t *testing.T) {
	if code := runSilenced(t, "frobnicate"); code != exitError {
		t.Errorf("unknown command exit = %d, want %d", code, exitError)
	}
}

// runSilenced runs the CLI with stdout suppressed and returns the exit code.
func runSilenced(t *testing.T, args ...string) int {
	t.Helper()
	var code int
	withSilencedStdout(t, func() { code = run(args) })
	return code
}

func pathThere(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeRepoFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
