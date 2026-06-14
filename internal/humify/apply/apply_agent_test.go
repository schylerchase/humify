package apply

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/schylerryan/humify/internal/humify/plan"
	"github.com/schylerryan/humify/internal/humify/verify"
)

// These tests exercise the agent apply path (ROADMAP #2): it must refuse a dirty
// repo, hard-roll-back on crash and regression while preserving .humify/, signal
// failure with a non-zero exit, and write an audit manifest on success.

func requireGit(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("agent path spawns via sh -c; POSIX-only")
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

// initRepo makes root a git repo with one committed file and a clean tree.
func initRepo(t *testing.T, root string) {
	t.Helper()
	// Test-isolation guard: every test here runs git reset --hard / clean -fd, which
	// would devastate a real working tree. Refuse to operate anywhere but a TempDir.
	if !strings.Contains(root, "TempDir") && !strings.Contains(filepath.ToSlash(root), "/T/") && !strings.HasPrefix(root, os.TempDir()) {
		t.Fatalf("refusing to git-init a non-temp dir in a destructive test: %s", root)
	}
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "t@e.x")
	runGit(t, root, "config", "user.name", "t")
	runGit(t, root, "config", "commit.gpgsign", "false")
}

func agentPlan(spec string) plan.Plan {
	return plan.Plan{Items: []plan.Item{{
		ID: "HMF-001", Signal: "deep_nesting", Title: "Flatten deep nesting",
		AutomationSafety: "assisted", Applyable: false, AgentSpec: spec,
	}}}
}

// applyAgent drives Apply down the agent branch (non-applyable item, unsafe + yes).
func applyAgent(root string, p plan.Plan, agentCmd string) (Result, error) {
	return Apply(root, p, "HMF-001", false, true, agentCmd, true, time.Now())
}

// TestApplyAgentDryRunPreviewDoesNotExecute covers the preview branch (ROADMAP #11):
// with --unsafe-permission but without --yes, apply must describe the spawn and the
// spec, and must NOT run the agent. No git needed — preview returns before any work.
func TestApplyAgentDryRunPreviewDoesNotExecute(t *testing.T) {
	root := t.TempDir()
	res, err := Apply(root, agentPlan("SPEC-MARKER"), "HMF-001", true, false, "echo RAN > ran.txt", true, time.Now())
	if err != nil {
		t.Fatalf("dry-run preview should not error: %v", err)
	}
	if !res.DryRun || res.Applied || res.RolledBack {
		t.Errorf("preview must be a no-op dry run: %+v", res)
	}
	if !strings.Contains(res.Message, "would spawn agent") || !strings.Contains(res.Message, "SPEC-MARKER") {
		t.Errorf("preview must describe the spawn and include the spec: %q", res.Message)
	}
	if isFile(filepath.Join(root, "ran.txt")) {
		t.Error("the agent must never run on a dry-run preview")
	}
}

func TestAgentApplyRefusesDirtyRepo(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	initRepo(t, root)
	writeFile(t, root, "src.txt", "v1\n")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "init")
	// Dirty the tree with an uncommitted, non-.humify change.
	writeFile(t, root, "dirty.txt", "uncommitted\n")

	res, err := applyAgent(root, agentPlan("spec"), "echo RAN > ran.txt")
	if err == nil {
		t.Fatal("agent apply must refuse a dirty repo — there is no clean rollback point")
	}
	if res.Applied {
		t.Error("must not apply on a dirty repo")
	}
	if isFile(filepath.Join(root, "ran.txt")) {
		t.Error("the agent must not run when the repo is refused")
	}
}

func TestAgentApplyRefusesNonGitRepo(t *testing.T) {
	requireGit(t)
	root := t.TempDir() // not a git repo
	res, err := applyAgent(root, agentPlan("spec"), "echo RAN > ran.txt")
	if err == nil {
		t.Fatal("agent apply must refuse outside a git repo — git is its only rollback")
	}
	if res.Applied || isFile(filepath.Join(root, "ran.txt")) {
		t.Error("the agent must not run without a git repo")
	}
}

func TestAgentApplyCrashRollsBackAndFails(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	initRepo(t, root)
	writeFile(t, root, "tracked.txt", "original\n")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "init")

	// The agent corrupts a tracked file, creates an untracked file, then crashes.
	res, err := applyAgent(root, agentPlan("spec"), "echo corrupted > tracked.txt; echo junk > untracked.txt; exit 1")
	if err == nil {
		t.Fatal("an agent crash must surface as an error (non-zero exit), not exit 0")
	}
	if got, _ := os.ReadFile(filepath.Join(root, "tracked.txt")); string(got) != "original\n" {
		t.Errorf("tracked file must be restored to its pre-apply state, got %q", got)
	}
	if isFile(filepath.Join(root, "untracked.txt")) {
		t.Error("agent-created untracked file must be removed by git clean on rollback")
	}
	if !res.RolledBack {
		t.Error("a crash that rolled back should report RolledBack")
	}
}

func TestAgentApplyWritesManifestOnSuccess(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	initRepo(t, root)
	writeFile(t, root, "tracked.txt", "original\n")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "init")

	// No go.mod/package.json → no validation; a benign change passes the gate.
	res, err := applyAgent(root, agentPlan("spec"), "echo touched >> tracked.txt")
	if err != nil {
		t.Fatalf("a benign agent change should succeed: %v", err)
	}
	if !res.Applied {
		t.Fatalf("agent change should be applied: %s", res.Message)
	}
	if res.ManifestPath == "" || !isFile(res.ManifestPath) {
		t.Fatalf("agent success must write an audit manifest; ManifestPath=%q", res.ManifestPath)
	}
	var man Manifest
	if data, err := os.ReadFile(res.ManifestPath); err != nil {
		t.Fatalf("read manifest: %v", err)
	} else if err := json.Unmarshal(data, &man); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if man.PlanItem != "HMF-001" {
		t.Errorf("manifest must record the plan item, got %q", man.PlanItem)
	}
	if man.BaseSHA == "" {
		t.Error("agent manifest must record the pre-apply base SHA for auditability")
	}
}

// TestAgentApplyRegressionRollsBackHard proves a build-breaking agent change is
// caught by re-validation and fully reverted via git (not left in the tree), and
// reported as drift (RolledBack, err==nil). Requires the go toolchain.
func TestAgentApplyRegressionRollsBackHard(t *testing.T) {
	requireGit(t)
	if testing.Short() {
		t.Skip("builds a real Go module and runs the go toolchain")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	root := t.TempDir()
	initRepo(t, root)
	writeFile(t, root, "go.mod", "module agentrollback\n\ngo 1.26\n")
	writeFile(t, root, "main.go", "package main\n\nfunc main() { println(Helper()) }\n")
	writeFile(t, root, "helper.go", "package main\n\nfunc Helper() string { return \"hi\" }\n")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-m", "init")

	// Baseline must genuinely pass so a post-failure is a true regression.
	if base, _ := verify.Run(root, time.Now()); !base.Validated || !base.Passed {
		t.Fatalf("baseline must validate and pass; got validated=%v passed=%v", base.Validated, base.Passed)
	}
	// `go build ./...` leaves an untracked binary; clear it so Apply's refuse-dirty
	// check sees the clean tree it would in the real analyze→plan→apply flow.
	runGit(t, root, "clean", "-fd")

	// The agent deletes the file defining Helper(), breaking the build.
	res, err := applyAgent(root, agentPlan("spec"), "rm helper.go")
	if err != nil {
		t.Fatalf("a regression rollback is drift, not an error: %v", err)
	}
	if !res.RolledBack {
		t.Fatalf("a build-breaking agent change must roll back; got %+v", res)
	}
	if res.Applied {
		t.Error("a rolled-back agent apply must not report Applied")
	}
	if !isFile(filepath.Join(root, "helper.go")) {
		t.Error("git reset --hard must restore the file the agent deleted")
	}
	if base, _ := verify.Run(root, time.Now()); !base.Passed {
		t.Error("after rollback the build must pass again")
	}
}
