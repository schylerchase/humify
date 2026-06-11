package apply

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/schylerryan/humify/internal/humify/analyze"
	"github.com/schylerryan/humify/internal/humify/plan"
	"github.com/schylerryan/humify/internal/humify/verify"
)

// buildRepo creates a target with one stale file and one JS file containing a
// swallowed error, and returns its analysis-derived plan. No go.mod/package.json,
// so verify detects no commands and the test needs no external toolchain.
func buildRepo(t *testing.T) (string, plan.Plan) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "keep.txt", "hello\n")
	writeFile(t, root, "old.bak", "obsolete\n")
	writeFile(t, root, "svc.js", "function f() {\n  try { g(); } catch (e) {}\n}\n")
	a, err := analyze.Run(root, analyze.Defaults())
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	return root, plan.Build(a)
}

func TestApplyQuarantineEndToEnd(t *testing.T) {
	root, p := buildRepo(t)
	item, ok := p.Find("HMF-001")
	if !ok || item.Signal != "stale_file" {
		t.Fatalf("HMF-001 should be the stale_file quarantine, got %+v", item)
	}

	// Dry run (default) must not move anything.
	dry, err := Apply(root, p, "HMF-001", true, false, "", false, time.Now())
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if dry.Applied {
		t.Error("dry run must not apply")
	}
	if !isFile(filepath.Join(root, "old.bak")) {
		t.Error("dry run must not move the file")
	}

	// Confirmed apply quarantines the file and writes a manifest.
	res, err := Apply(root, p, "HMF-001", false, true, "", false, time.Now())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.Applied {
		t.Fatalf("apply should have succeeded: %s", res.Message)
	}
	if isFile(filepath.Join(root, "old.bak")) {
		t.Error("source file should have been moved out")
	}
	quarantined := filepath.Join(root, ".humify", "delete-me", "HMF-001", "old.bak")
	if !isFile(quarantined) {
		t.Errorf("file should be quarantined at %s", quarantined)
	}
	manPath := filepath.Join(root, ".humify", "delete-me", "HMF-001", "manifest.json")
	if !isFile(manPath) {
		t.Error("a manifest should be written")
	}

	// This repo has no validation commands, so apply validated nothing. The result
	// and the on-disk manifest must say so honestly — never Passed:true with
	// Ran:false (the vacuous "validated nothing" lie).
	if res.Validated {
		t.Error("res.Validated must be false when no validation command ran")
	}
	var man Manifest
	if data, err := os.ReadFile(manPath); err != nil {
		t.Fatalf("read manifest: %v", err)
	} else if err := json.Unmarshal(data, &man); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if man.Validation.Ran || man.Validation.Passed {
		t.Errorf("a quarantine that validated nothing must record Ran:false Passed:false, got %+v", man.Validation)
	}
}

func TestApplyRefusesManualItem(t *testing.T) {
	root, p := buildRepo(t)
	sw, ok := findSignal(p, "swallowed_error")
	if !ok {
		t.Fatal("expected a swallowed_error item in the plan")
	}
	res, err := Apply(root, p, sw.ID, false, true, "", false, time.Now())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.Applied || !res.Skipped {
		t.Errorf("a manual item must be skipped, not applied: %+v", res)
	}
	if !isFile(filepath.Join(root, "svc.js")) {
		t.Error("apply must never modify source for a manual item")
	}
}

func TestApplyUnknownItemErrors(t *testing.T) {
	root, p := buildRepo(t)
	if _, err := Apply(root, p, "HMF-999", false, true, "", false, time.Now()); err == nil {
		t.Error("apply on an unknown plan id must error")
	}
}

// TestApplyRollsBackOnRegression is the end-to-end proof of apply's safety
// contract: a quarantine that breaks the build is caught by re-running validation
// and fully reverted. The target is a real Go module whose untitled.go — flagged
// stale by name yet compiled — defines a symbol main.go calls, so quarantining it
// turns a passing build into a failing one. Requires the go toolchain.
func TestApplyRollsBackOnRegression(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a real Go module and runs the go toolchain")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module rollbackdemo\n\ngo 1.26\n")
	writeFile(t, root, "main.go", "package main\n\nfunc main() { println(Greeting()) }\n")
	// "untitled" is flagged stale by name, but the .go file is compiled and
	// load-bearing, so removing it breaks the build — the adversarial case.
	writeFile(t, root, "untitled.go", "package main\n\nfunc Greeting() string { return \"hi\" }\n")

	a, err := analyze.Run(root, analyze.Defaults())
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	item, ok := findSignal(plan.Build(a), "stale_file")
	if !ok {
		t.Fatal("expected a stale_file quarantine item for untitled.go")
	}

	// Precondition: the baseline genuinely passes, so a post-failure is a true
	// regression and not a pre-existing breakage.
	if base, _ := verify.Run(root, time.Now()); !base.Validated || !base.Passed {
		t.Fatalf("baseline must validate and pass; got validated=%v passed=%v", base.Validated, base.Passed)
	}

	res, err := Apply(root, plan.Build(a), item.ID, false, true, "", false, time.Now())
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.RolledBack {
		t.Fatalf("a build-breaking quarantine must roll back; got %+v", res)
	}
	if res.Applied {
		t.Error("a rolled-back apply must not report Applied")
	}
	if !res.Validated {
		t.Error("verify ran the go toolchain, so Validated must be true")
	}
	// A rollback leaves no trace: file restored, quarantine empty, no manifest.
	if !isFile(filepath.Join(root, "untitled.go")) {
		t.Error("untitled.go must be restored to its original location")
	}
	if isFile(filepath.Join(root, ".humify", "delete-me", item.ID, "untitled.go")) {
		t.Error("the quarantined copy must be removed on rollback")
	}
	if isFile(filepath.Join(root, ".humify", "delete-me", item.ID, "manifest.json")) {
		t.Error("no manifest must be written when the change is rolled back")
	}
}

// cpass/cfail/cindet build the three CmdResult states the gate distinguishes:
// a clean pass, a clean fail (real non-zero exit), and an indeterminate result
// (ExitCode -1 — timed out or could not run). val wraps them into a Validation.
func cpass(kind string) verify.CmdResult {
	return verify.CmdResult{Kind: kind, Ran: true, Passed: true}
}
func cfail(kind string) verify.CmdResult {
	return verify.CmdResult{Kind: kind, Ran: true, Passed: false, ExitCode: 1}
}
func cindet(kind string) verify.CmdResult {
	return verify.CmdResult{Kind: kind, Ran: true, Passed: false, ExitCode: -1}
}

func val(cmds ...verify.CmdResult) verify.Validation {
	v := verify.Validation{Commands: cmds, Validated: len(cmds) > 0, Passed: true}
	for _, c := range cmds {
		if c.Ran && !c.Passed {
			v.Passed = false
		}
	}
	return v
}

func sameKinds(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestGate is the full baseline→post truth table for apply's safety gate. The
// load-bearing row is indeterminate→clean-fail: it used to be silently waved
// through as "already failing" because an indeterminate baseline counted as a
// failure, disabling the only regression check.
func TestGate(t *testing.T) {
	tests := []struct {
		name     string
		baseline verify.Validation
		post     verify.Validation
		want     gateOutcome
		kinds    []string
	}{
		{"pass→pass: ok", val(cpass("test")), val(cpass("test")), gateOK, nil},
		{"pass→cleanfail: regressed", val(cpass("test")), val(cfail("test")), gateRegressed, []string{"test"}},
		{"cleanfail→cleanfail: pre-existing, ok", val(cfail("test")), val(cfail("test")), gateOK, nil},
		{"indeterminate→cleanfail: regressed (THE HOLE)", val(cindet("test")), val(cfail("test")), gateRegressed, []string{"test"}},
		{"pass→indeterminate: unverifiable", val(cpass("test")), val(cindet("test")), gateUnverifiable, []string{"test"}},
		{"indeterminate→indeterminate: unverifiable", val(cindet("test")), val(cindet("test")), gateUnverifiable, []string{"test"}},
		{"cleanfail→indeterminate: ok (nothing to protect)", val(cfail("test")), val(cindet("test")), gateOK, nil},
		{"indeterminate→pass: ok", val(cindet("test")), val(cpass("test")), gateOK, nil},
		{"regressed outranks unverifiable", val(cpass("build"), cpass("test")), val(cfail("build"), cindet("test")), gateRegressed, []string{"build"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, kinds := gate(tt.baseline, tt.post)
			if got != tt.want || !sameKinds(kinds, tt.kinds) {
				t.Errorf("gate = (%d, %v), want (%d, %v)", got, kinds, tt.want, tt.kinds)
			}
		})
	}
}

func TestComputeDelta(t *testing.T) {
	if a, n, f := computeDelta(val(cfail("test")), val(cpass("test"))); !sameKinds(f, []string{"test"}) || len(a)+len(n) != 0 {
		t.Errorf("cleanfail→pass should be fixed only; got already=%v newly=%v fixed=%v", a, n, f)
	}
	if a, n, f := computeDelta(val(cpass("test")), val(cfail("test"))); !sameKinds(n, []string{"test"}) || len(a)+len(f) != 0 {
		t.Errorf("pass→cleanfail should be newly-failing only; got already=%v newly=%v fixed=%v", a, n, f)
	}
	if a, n, f := computeDelta(val(cfail("test")), val(cfail("test"))); !sameKinds(a, []string{"test"}) || len(n)+len(f) != 0 {
		t.Errorf("cleanfail→cleanfail should be already-failing only; got already=%v newly=%v fixed=%v", a, n, f)
	}
	// The honesty fix: an indeterminate baseline that then passes is NOT "fixed"
	// (it was never known to be failing).
	if a, n, f := computeDelta(val(cindet("test")), val(cpass("test"))); len(a)+len(n)+len(f) != 0 {
		t.Errorf("indeterminate→pass must classify as nothing; got already=%v newly=%v fixed=%v", a, n, f)
	}
}

func TestApplyValidationNote(t *testing.T) {
	// An indeterminate baseline must never be described as "already failing".
	if note := applyValidationNote(val(cindet("test")), val(cpass("test"))); strings.Contains(note, "already failing") {
		t.Errorf("indeterminate baseline must not read as already-failing: %q", note)
	}
	if note := applyValidationNote(val(cfail("test")), val(cfail("test"))); !strings.Contains(note, "already failing") {
		t.Errorf("a genuine pre-existing failure should say so: %q", note)
	}
}

func findSignal(p plan.Plan, signal string) (plan.Item, bool) {
	for _, it := range p.Items {
		if it.Signal == signal {
			return it, true
		}
	}
	return plan.Item{}, false
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
