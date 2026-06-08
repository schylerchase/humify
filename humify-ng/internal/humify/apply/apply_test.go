package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"humify-ng/internal/humify/analyze"
	"humify-ng/internal/humify/plan"
	"humify-ng/internal/humify/verify"
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
	dry, err := Apply(root, p, "HMF-001", true, false, time.Now())
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
	res, err := Apply(root, p, "HMF-001", false, true, time.Now())
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
	res, err := Apply(root, p, sw.ID, false, true, time.Now())
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
	if _, err := Apply(root, p, "HMF-999", false, true, time.Now()); err == nil {
		t.Error("apply on an unknown plan id must error")
	}
}

func TestRegressedDetectsNewlyFailing(t *testing.T) {
	baseline := verify.Validation{Commands: []verify.CmdResult{{Kind: "test", Ran: true, Passed: true}}}
	post := verify.Validation{Commands: []verify.CmdResult{{Kind: "test", Ran: true, Passed: false}}}
	if !regressed(baseline, post) {
		t.Error("a test that passed then failed is a regression")
	}
	if regressed(baseline, baseline) {
		t.Error("identical results are not a regression")
	}
	// A pre-existing failure (failing in both) is NOT a regression caused by apply.
	preExisting := verify.Validation{Commands: []verify.CmdResult{{Kind: "test", Ran: true, Passed: false}}}
	if regressed(preExisting, post) {
		t.Error("a pre-existing failure must not count as a regression")
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
