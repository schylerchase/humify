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

	// Regression: status used to ignore --root (it resolved from cwd only), so an
	// orchestrator driving the pipeline with --root uniformly hit a false "no
	// project" on the one read-only status call. With --root honored, status finds
	// the bootstrapped project regardless of cwd.
	t.Run("status honors --root", func(t *testing.T) {
		root, _ := fixture(t)
		code, out := runHumify(t, "untangle", "status", "--root", root, "--json")
		if code != exitOK {
			t.Fatalf("status --root should succeed, got %d:\n%s", code, out)
		}
		if strings.Contains(out, "no_humify_dir") {
			t.Fatalf("status ignored --root (looked in cwd, not the project):\n%s", out)
		}
	})

	// Regression: the driver advances disk state across stages but left no handoff
	// breadcrumb, so heatmap's pre-run "humify audit" cursor survived and every
	// completed run made the next resume cry stale_handoff on a healthy project.
	t.Run("after run, resume agrees — no false stale_handoff", func(t *testing.T) {
		root, _ := fixture(t)
		if code, out := runHumify(t, "untangle", "run", "--root", root, "--agent-cmd", agent, "--json"); code != exitOK {
			t.Fatalf("run failed (%d):\n%s", code, out)
		}
		code, out := runHumify(t, "untangle", "resume", "--root", root, "--json")
		if code != exitOK {
			t.Fatalf("resume after a clean run should be exit 0, got %d:\n%s", code, out)
		}
		if strings.Contains(out, "stale_handoff") {
			t.Fatalf("run left a stale cursor — resume falsely reports drift:\n%s", out)
		}
	})

	// Multi-area merge barrier — the regression lock for 8d21059, which until now
	// was only dogfooded by hand. A wave with two INDEPENDENT areas forks two
	// worktrees; conflictagent makes both executors write the same shared file with
	// different content, so the SECOND merge conflicts. The barrier must abort that
	// merge (no dangling MERGE_HEAD), keep the first slice it already landed, and
	// stop Blocked (exit 2) — never leave the repo mid-merge or spin.
	t.Run("multi-area conflicting merge aborts cleanly, blocks (8d21059)", func(t *testing.T) {
		cflt := buildConflictAgent(t)
		root, head := multiAreaFixture(t)
		code, out := runHumify(t, "untangle", "run", "--root", root,
			"--agent-cmd", cflt, "--execute", "--json")
		if code != exitDrift {
			t.Fatalf("a merge conflict must block (exit 2), got %d\n%s", code, out)
		}
		if !strings.Contains(out, "blocked") {
			t.Fatalf("want a merge-barrier blocked verdict, got:\n%s", out)
		}
		// The heart of 8d21059: a conflicted merge is ABORTED, never left in place.
		// A surviving MERGE_HEAD is the exact corruption AbortMerge exists to prevent
		// (and what would make the next merge/resume choke on stale mid-merge state).
		if fileExists(filepath.Join(root, ".git", "MERGE_HEAD")) {
			t.Error("MERGE_HEAD survived — the conflict was not aborted (AbortMerge regressed)")
		}
		// One slice merged before the other conflicted, so HEAD must have advanced.
		if now := gitHead(t, root); now == head {
			t.Error("HEAD did not move — the first slice should have merged before the conflict")
		}
		// No unmerged index entries strewn across the working tree (.humify churn aside).
		for _, line := range strings.Split(strings.TrimSpace(runGit(t, root, "status", "--porcelain")), "\n") {
			if len(line) >= 2 && (line[0] == 'U' || line[1] == 'U') {
				t.Errorf("unmerged entries left in the working tree:\n%s", line)
			}
		}
	})

	// Crashed executor — regression lock for the no-commit merge gate, found by the
	// real-OSS dogfood. An executor that dies before committing leaves its slice
	// branch sitting at the fork point. The barrier must BLOCK that empty slice
	// (gate "no-commit"), not merge it as a silent no-op and miscount it as a merged
	// slice (which is what happened before the gate existed — the wave reported
	// "merged N" while one slice contributed nothing, surviving only via the
	// separate no-progress guard).
	t.Run("crashed executor leaves an empty slice the barrier blocks (no-commit)", func(t *testing.T) {
		root, head := multiAreaFixture(t)
		code, out := runHumify(t, "untangle", "run", "--root", root,
			"--agent-cmd", agent+" --crash-exec", "--execute", "--json")
		if code != exitDrift {
			t.Fatalf("a crashed executor (empty slice) must block (exit 2), got %d\n%s", code, out)
		}
		if !strings.Contains(out, "no-commit") {
			t.Fatalf("the empty slice must block at the no-commit gate, got:\n%s", out)
		}
		// Nothing was committed, so nothing must merge — the empty slice is caught
		// before any no-op merge can advance HEAD.
		if now := gitHead(t, root); now != head {
			t.Errorf("HEAD moved (%s → %s) — an empty slice must not merge", head, now)
		}
	})
}

// TestVerifyBaselineExitCodesE2E is the load-bearing guard for baseline-aware
// verify: the entire feature is the exit-code contract, and only an end-to-end run
// against a real toolchain proves it. A unit test mocking Validation cannot catch a
// regression where `len(newly) > 0` is flipped, the no-baseline fallthrough breaks,
// or an indeterminate kind leaks into "newly". Drives the real CLI on tiny Go
// modules so `go build` is the genuine discriminator.
func TestVerifyBaselineExitCodesE2E(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("verify shells out to the go toolchain POSIX-style; e2e is POSIX-only")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	const green = "package main\n\nfunc main() { _ = add(1, 2) }\nfunc add(a, b int) int { return a + b }\n"
	newRepo := func(t *testing.T, mainGo string) string {
		t.Helper()
		root := t.TempDir()
		mustWrite(t, filepath.Join(root, "go.mod"), "module smoke\n\ngo 1.26\n")
		mustWrite(t, filepath.Join(root, "main.go"), mainGo)
		for _, a := range [][]string{
			{"init"}, {"config", "user.email", "e@x.t"}, {"config", "user.name", "t"},
			{"config", "commit.gpgsign", "false"}, {"add", "-A"}, {"commit", "-m", "init"},
		} {
			runGit(t, root, a...)
		}
		return root
	}

	t.Run("regression: green baseline then break -> exit 2, newly failed", func(t *testing.T) {
		root := newRepo(t, green)
		if code, out := runHumify(t, "verify", "--save-baseline", "--no-coverage", root); code != exitOK {
			t.Fatalf("save-baseline on a green tree must be exit 0, got %d:\n%s", code, out)
		}
		mustWrite(t, filepath.Join(root, "main.go"),
			"package main\n\nfunc main() { _ = add(1, 2) }\nfunc add(a, b int) int { return a + b   \n") // missing brace
		code, out := runHumify(t, "verify", "--baseline", "--no-coverage", root)
		if code != exitDrift {
			t.Fatalf("a self-caused regression must exit 2 (drift), got %d:\n%s", code, out)
		}
		if !strings.Contains(out, "newly failed") {
			t.Fatalf("regression output must name newly-failed kinds:\n%s", out)
		}
	})

	t.Run("ambient: red baseline then cosmetic edit -> exit 0, already failing", func(t *testing.T) {
		root := newRepo(t, "package main\n\nfunc main() { _ = missing() }\n") // build already broken
		if code, out := runHumify(t, "verify", "--save-baseline", "--no-coverage", root); code != exitOK {
			t.Fatalf("save-baseline must exit 0 even on an ambient-red tree, got %d:\n%s", code, out)
		}
		mustWrite(t, filepath.Join(root, "main.go"), "package main\n\n// touch\nfunc main() { _ = missing() }\n")
		code, out := runHumify(t, "verify", "--baseline", "--no-coverage", root)
		if code != exitOK {
			t.Fatalf("an ambient failure that predates the change must NOT exit 2, got %d:\n%s", code, out)
		}
		if !strings.Contains(out, "already failing") {
			t.Fatalf("ambient output must mark the failure as pre-existing:\n%s", out)
		}
	})

	t.Run("no baseline -> loud degrade, plain exit", func(t *testing.T) {
		root := newRepo(t, green)
		code, out := runHumify(t, "verify", "--baseline", "--no-coverage", root)
		if code != exitOK {
			t.Fatalf("no-baseline on a green tree falls through to plain verify (exit 0), got %d:\n%s", code, out)
		}
		if !strings.Contains(out, "--save-baseline") {
			t.Fatalf("the no-baseline degrade must name --save-baseline:\n%s", out)
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

// buildConflictAgent compiles testdata/conflictagent — it behaves like fakeagent
// through audit/plan/check, but as an executor writes a shared .humify file with
// per-area content so the second merge in a wave conflicts (the AbortMerge path).
func buildConflictAgent(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "conflictagent")
	src, err := filepath.Abs(filepath.Join("testdata", "conflictagent", "main.go"))
	if err != nil {
		t.Fatalf("resolve conflictagent source: %v", err)
	}
	if out, err := exec.Command("go", "build", "-o", bin, src).CombinedOutput(); err != nil {
		t.Fatalf("build conflictagent: %v\n%s", err, out)
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

// multiAreaFixture is fixture's multi-area sibling: a flat "root" area plus an
// independent scripts/ subarea under src/ (no cross-import), so area.Decompose
// yields two areas that land in the same wave — the driver then forks two
// worktrees, the prerequisite for exercising the merge barrier. README.md exists
// only so conflictagent's canned finding references a real file. It fails fast if
// the bootstrap does not split into ≥2 areas, so the conflict test can never
// silently degrade to a single slice (and thus never reach a merge conflict).
func multiAreaFixture(t *testing.T) (root, head string) {
	t.Helper()
	root = t.TempDir()
	mustWrite(t, filepath.Join(root, "README.md"), "# fixture\n")
	mustWrite(t, filepath.Join(root, "src", "core.go"),
		"package src\n\n// Core does stuff.\nfunc Core() { var data interface{}; _ = data }\n")
	mustWrite(t, filepath.Join(root, "src", "scripts", "run.go"),
		"package scripts\n\nfunc Run() int { return 0 }\n")
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
	if n := countAreaDirs(t, root); n < 2 {
		t.Fatalf("multi-area fixture produced only %d area(s); the merge barrier needs ≥2", n)
	}
	return root, gitHead(t, root)
}

// countAreaDirs counts the per-area scaffold directories heatmap wrote under
// .humify/areas — one per decomposed area.
func countAreaDirs(t *testing.T, root string) int {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(root, ".humify", "areas"))
	if err != nil {
		t.Fatalf("read areas dir: %v", err)
	}
	n := 0
	for _, e := range ents {
		if e.IsDir() {
			n++
		}
	}
	return n
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
