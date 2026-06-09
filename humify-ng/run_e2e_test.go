package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestUntangleRunE2E is the autonomous driver's end-to-end proof — the analog of
// the apply-rollback e2e. It drives the WHOLE untangle chain in-process against a
// real git fixture, with a real --agent-cmd (the fakeagent binary) spawned through
// the production ShellExec path. Unit tests cover the pieces; only this proves the
// spawn → cwd → merge → termination interactions actually compose.
//
// Three behaviors, each a load-bearing guarantee:
//   - without --execute the driver stops at plan-converged and never touches source;
//   - with --execute it forks worktrees, spawns executors that rewrite + commit,
//     auto-merges, and reaches done;
//   - a deterministically-failing auditor makes the driver stop (stuck), not loop.
func TestUntangleRunE2E(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("driver spawns via sh and forks git worktrees; this e2e is POSIX-only")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	agent := buildFakeAgent(t)

	t.Run("stops at plan-converged without --execute (no source mutation)", func(t *testing.T) {
		root, head := fixture(t)
		code, out := runHumify(t, "untangle", "run", "--root", root, "--agent-cmd", agent, "--json")
		if code != exitOK {
			t.Fatalf("want exit 0, got %d\n%s", code, out)
		}
		if !strings.Contains(out, "plan_converged") {
			t.Fatalf("want plan_converged reason, got:\n%s", out)
		}
		// The source-touching stage never ran: no worktrees forked, no patchlog, and
		// the tracked source + HEAD are exactly as the fixture left them.
		if dirExists(filepath.Join(filepath.Dir(root), ".humify-worktrees")) {
			t.Error("worktrees were forked without --execute")
		}
		if fileExists(filepath.Join(root, ".humify", "PATCHLOG.md")) {
			t.Error("PATCHLOG.md written without --execute")
		}
		if s := gitPorcelain(t, root, "src"); s != "" {
			t.Errorf("source mutated without --execute:\n%s", s)
		}
		if now := gitHead(t, root); now != head {
			t.Errorf("HEAD moved without --execute: %s → %s", head, now)
		}
	})

	t.Run("full chain with --execute reaches done", func(t *testing.T) {
		root, head := fixture(t)
		code, out := runHumify(t, "untangle", "run", "--root", root, "--agent-cmd", agent,
			"--execute", "--test-cmd", "true", "--json")
		if code != exitOK {
			t.Fatalf("want exit 0, got %d\n%s", code, out)
		}
		if !strings.Contains(out, "complete") {
			t.Fatalf("want complete reason, got:\n%s", out)
		}
		if !fileExists(filepath.Join(root, ".humify", "PATCHLOG.md")) {
			t.Error("no PATCHLOG.md after a completed --execute run")
		}
		if !hasSummary(root) {
			t.Error("no executor SUMMARY merged into the project")
		}
		if now := gitHead(t, root); now == head {
			t.Error("HEAD did not move — nothing was actually merged")
		}
	})

	t.Run("--execute with a failing build/test gate blocks after merging (exit 2)", func(t *testing.T) {
		// Exercises the source-mutating Blocked limb: fork → executor commits → merge
		// (AppendCommits + ClearManifest run) → the gate (`false`) fails → driveMerge
		// returns a Blocked runStep → the driver stops at exit 2. The merge has already
		// landed, so HEAD must have moved — that's why the verdict says fix-forward/undo.
		root, head := fixture(t)
		code, out := runHumify(t, "untangle", "run", "--root", root, "--agent-cmd", agent,
			"--execute", "--test-cmd", "false", "--json")
		if code != exitDrift {
			t.Fatalf("want exit 2 (blocked), got %d\n%s", code, out)
		}
		if !strings.Contains(out, "blocked") && !strings.Contains(out, "gate") {
			t.Fatalf("want a blocked/gate-failed verdict, got:\n%s", out)
		}
		if now := gitHead(t, root); now == head {
			t.Error("HEAD did not move — the wave should have merged before the gate failed")
		}
	})

	t.Run("deterministically-failing auditor blocks, no infinite loop", func(t *testing.T) {
		root, _ := fixture(t)
		code, out := runHumify(t, "untangle", "run", "--root", root,
			"--agent-cmd", agent+" --fail-audit", "--json")
		if code != exitDrift {
			t.Fatalf("want exit 2 (stuck), got %d\n%s", code, out)
		}
		if !strings.Contains(out, "stuck") {
			t.Fatalf("want stuck reason, got:\n%s", out)
		}
	})

	// An auditor that WRITES a fragment every round but an INVALID one (no severity)
	// must still be caught as stuck — not spin to the iteration cap on file churn.
	// This is the regression lock for the validated-state progress guard.
	t.Run("invalid-fragment auditor is stuck, not iteration-cap", func(t *testing.T) {
		root, _ := fixture(t)
		code, out := runHumify(t, "untangle", "run", "--root", root,
			"--agent-cmd", agent+" --bad-audit", "--json")
		if code != exitDrift {
			t.Fatalf("want exit 2 (stuck), got %d\n%s", code, out)
		}
		if !strings.Contains(out, "stuck") {
			t.Fatalf("want stuck (not iteration_cap), got:\n%s", out)
		}
	})
}

// buildFakeAgent compiles testdata/fakeagent into a binary that outlives the
// subtests (built in the parent test's temp dir).
func buildFakeAgent(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fakeagent")
	src, err := filepath.Abs(filepath.Join("testdata", "fakeagent", "main.go"))
	if err != nil {
		t.Fatalf("resolve fakeagent source: %v", err)
	}
	if out, err := exec.Command("go", "build", "-o", bin, src).CombinedOutput(); err != nil {
		t.Fatalf("build fakeagent: %v\n%s", err, out)
	}
	return bin
}

// fixture creates a committed git repo with a small slop-y source area under src/
// and bootstraps a .humify/ project on it (heatmap). It returns the repo root and
// its initial HEAD so a test can assert whether the driver moved it.
func fixture(t *testing.T) (root, head string) {
	t.Helper()
	root = t.TempDir()
	mustWrite(t, filepath.Join(root, "src", "util.go"),
		"package src\n\n// Util does stuff.\nfunc Util() { var data interface{}; _ = data }\n")
	mustWrite(t, filepath.Join(root, "src", "helper.go"),
		"package src\n\nfunc Helper() int { return 0 }\n")
	for _, args := range [][]string{
		{"init"}, {"config", "user.email", "e2e@humify.test"}, {"config", "user.name", "Humify E2E"},
		{"add", "-A"}, {"commit", "-m", "init"},
	} {
		runGit(t, root, args...)
	}
	if code, out := runHumify(t, "untangle", "heatmap", "--target", filepath.Join(root, "src"),
		"--root", root, "--json"); code != exitOK {
		t.Fatalf("heatmap bootstrap failed (%d):\n%s", code, out)
	}
	return root, gitHead(t, root)
}

// runHumify invokes the CLI entrypoint in-process, capturing stdout so tests can
// assert the reason code without a built binary.
func runHumify(t *testing.T, args ...string) (int, string) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	code := run(args)
	_ = w.Close()
	os.Stdout = old
	b, _ := io.ReadAll(r)
	return code, string(b)
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func gitHead(t *testing.T, dir string) string {
	return strings.TrimSpace(runGit(t, dir, "rev-parse", "HEAD"))
}

func gitPorcelain(t *testing.T, dir, path string) string {
	return strings.TrimSpace(runGit(t, dir, "status", "--porcelain", "--", path))
}

// hasSummary reports whether any area gained a merged executor SUMMARY.
func hasSummary(root string) bool {
	found := false
	_ = filepath.WalkDir(filepath.Join(root, ".humify", "areas"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, "-SUMMARY.md") {
			found = true
		}
		return nil
	})
	return found
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
