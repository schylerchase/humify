package plan

import (
	"fmt"
	"strings"
	"testing"

	"github.com/schylerryan/humify/internal/humify/analyze"
)

func TestBuildRanksQuarantineFirst(t *testing.T) {
	a := analyze.Analysis{
		Target: "/repo",
		Findings: []analyze.Finding{
			{ID: "F001", Category: "correctness", Signal: "swallowed_error", File: "a.go", Line: 5, Severity: "major"},
			{ID: "F002", Category: "maintainability", Signal: "stale_file", File: "old.bak", Line: 1, Severity: "warning", Evidence: "throwaway extension .bak"},
			{ID: "F003", Category: "correctness", Signal: "swallowed_error", File: "b.go", Line: 9, Severity: "major"},
		},
		Summary: analyze.Summary{SourceFiles: 2},
	}
	p := Build(a)
	if len(p.Items) < 2 {
		t.Fatalf("want >=2 items, got %d", len(p.Items))
	}
	first := p.Items[0]
	if first.ID != "HMF-001" || first.Signal != "stale_file" {
		t.Errorf("HMF-001 should be the stale_file quarantine, got %s/%s", first.ID, first.Signal)
	}
	if !first.Applyable || first.Action != "quarantine" {
		t.Errorf("quarantine item must be applyable with action=quarantine: %+v", first)
	}
	if len(first.Files) != 1 || first.Files[0] != "old.bak" {
		t.Errorf("quarantine files = %v, want [old.bak]", first.Files)
	}

	sw, ok := findSignal(p, "swallowed_error")
	if !ok {
		t.Fatal("expected a swallowed_error item")
	}
	if sw.Applyable {
		t.Error("a manual (high-risk) item must not be applyable")
	}
	if len(sw.FindingIDs) != 2 {
		t.Errorf("swallowed_error item should reference both findings, got %v", sw.FindingIDs)
	}
}

func TestBuildAssignsSequentialIDs(t *testing.T) {
	a := analyze.Analysis{Findings: []analyze.Finding{
		{Signal: "stale_file", Severity: "warning", File: "x.bak"},
		{Signal: "long_function", Severity: "major", File: "y.go"},
		{Signal: "todo_marker", Severity: "info", File: "z.go"},
	}}
	p := Build(a)
	for i, it := range p.Items {
		want := "HMF-00" + string(rune('1'+i))
		if it.ID != want {
			t.Errorf("item %d id = %s, want %s", i, it.ID, want)
		}
	}
}

// TestDeadCandidateExcludedFromRefactorSpecNonDestructively is the safety contract
// at the plan layer: a file flagged as a possibly-dead module is dropped from a
// refactor agent's "Files to modify" list (so the agent does not edit a file slated
// for removal) — but it is NEVER erased. It stays in the item's Files and is named
// in the excluded section, so a false nomination loses nothing.
func TestDeadCandidateExcludedFromRefactorSpecNonDestructively(t *testing.T) {
	a := analyze.Analysis{
		Target: "/repo",
		Findings: []analyze.Finding{
			{ID: "F001", Category: "maintainability", Signal: "long_function", File: "dead.ts", Line: 10, Severity: "major"},
			{ID: "F002", Category: "maintainability", Signal: "long_function", File: "live.ts", Line: 5, Severity: "major"},
			{ID: "F003", Category: "maintainability", Signal: "dead_module", File: "dead.ts", Line: 1, Severity: "warning"},
		},
	}
	p := Build(a)

	dm, ok := findSignal(p, "dead_module")
	if !ok || !dm.Applyable || dm.Action != "quarantine" {
		t.Fatalf("dead_module item must exist and be an applyable quarantine, got %+v", dm)
	}

	lf, ok := findSignal(p, "long_function")
	if !ok {
		t.Fatal("expected a long_function item")
	}
	// Non-destructive: both files remain on the item.
	if !contains(lf.Files, "dead.ts") || !contains(lf.Files, "live.ts") {
		t.Errorf("a dead nomination must not drop files from the item: %v", lf.Files)
	}
	// The spec must split them: live.ts is modifiable, dead.ts is excluded-as-dead.
	marker := "Files excluded (flagged as possibly-dead"
	idx := strings.Index(lf.AgentSpec, marker)
	if idx < 0 {
		t.Fatalf("agent spec must carry an excluded-dead section:\n%s", lf.AgentSpec)
	}
	modifiable, excluded := lf.AgentSpec[:idx], lf.AgentSpec[idx:]
	if strings.Contains(modifiable, "dead.ts") {
		t.Error("dead.ts must not appear under Files to modify — the agent would edit a file slated for removal")
	}
	if !strings.Contains(modifiable, "live.ts") {
		t.Error("live.ts must remain modifiable")
	}
	if !strings.Contains(excluded, "dead.ts") {
		t.Error("dead.ts must be named in the excluded section, not silently dropped")
	}
}

// TestAgentSpecBansHumifyDir covers ROADMAP #13: the agent runs with cwd at the
// repo root and could wander into .humify/, corrupting the analysis/plan/quarantine
// state this very run depends on. The constraint block must forbid it (3169aff
// banned generated dirs but not .humify/).
func TestAgentSpecBansHumifyDir(t *testing.T) {
	a := analyze.Analysis{
		Target: "/repo",
		Findings: []analyze.Finding{
			{ID: "F001", Category: "maintainability", Signal: "long_function", File: "svc.go", Line: 10, Severity: "major"},
		},
	}
	lf, ok := findSignal(Build(a), "long_function")
	if !ok {
		t.Fatal("expected a long_function item")
	}
	if !strings.Contains(lf.AgentSpec, ".humify") {
		t.Errorf("agent spec must forbid touching humify's state dir .humify/:\n%s", lf.AgentSpec)
	}
	// The existing generated-output ban must remain (regression guard for 3169aff).
	if !strings.Contains(lf.AgentSpec, "node_modules") {
		t.Error("the generated-output ban must still be present")
	}
}

// TestAgentSpecEvidenceMatchesWorklist covers ROADMAP #8: buildAgentSpec listed
// every modifiable file but capped evidence at 5, so a signal with many findings
// commanded the agent to edit files it gave no evidence for (scope oversell). Every
// file under "Files to modify" must also have evidence.
func TestAgentSpecEvidenceMatchesWorklist(t *testing.T) {
	var findings []analyze.Finding
	for i := 0; i < 8; i++ {
		findings = append(findings, analyze.Finding{
			ID: fmt.Sprintf("F%03d", i), Category: "maintainability", Signal: "long_function",
			File: fmt.Sprintf("f%d.go", i), Line: 10 + i, Severity: "major",
			Evidence: fmt.Sprintf("function spans ~%d lines", 100+i),
		})
	}
	lf, ok := findSignal(Build(analyze.Analysis{Target: "/repo", Findings: findings}), "long_function")
	if !ok {
		t.Fatal("expected a long_function item")
	}
	spec := lf.AgentSpec
	mi, ei := strings.Index(spec, "Files to modify:"), strings.Index(spec, "Evidence (file:line finding):")
	if mi < 0 || ei < 0 {
		t.Fatalf("spec must have both a modify and an evidence section:\n%s", spec)
	}
	modify, evidence := spec[mi:ei], spec[ei:]
	for i := 0; i < 8; i++ {
		f := fmt.Sprintf("f%d.go", i)
		if strings.Contains(modify, f) && !strings.Contains(evidence, f) {
			t.Errorf("%s is commanded for modification but carries no evidence (scope oversell)", f)
		}
	}
}

// TestBuildAgentSpecSizeCapAndSafeShortCircuit covers ROADMAP #12: a file over
// agentFileSizeLimit must be excluded from the modify list (named in the too-large
// section with its LOC, not silently dropped), and a safe signal must produce no
// agent spec (it uses the quarantine path).
func TestBuildAgentSpecSizeCapAndSafeShortCircuit(t *testing.T) {
	a := analyze.Analysis{
		Target: "/repo",
		Files: []analyze.FileScore{
			{Path: "huge.go", Metrics: analyze.Metrics{LOC: agentFileSizeLimit + 1000}},
			{Path: "small.go", Metrics: analyze.Metrics{LOC: 50}},
		},
		Findings: []analyze.Finding{
			{ID: "F001", Category: "maintainability", Signal: "long_function", File: "huge.go", Line: 10, Severity: "major", Evidence: "huge"},
			{ID: "F002", Category: "maintainability", Signal: "long_function", File: "small.go", Line: 5, Severity: "major", Evidence: "small"},
			{ID: "F003", Category: "maintainability", Signal: "stale_file", File: "old.bak", Line: 1, Severity: "warning", Evidence: "stale"},
		},
	}
	p := Build(a)

	lf, ok := findSignal(p, "long_function")
	if !ok {
		t.Fatal("expected a long_function item")
	}
	idx := strings.Index(lf.AgentSpec, "Files excluded (too large")
	if idx < 0 {
		t.Fatalf("spec must carry a too-large excluded section:\n%s", lf.AgentSpec)
	}
	modify, excluded := lf.AgentSpec[:idx], lf.AgentSpec[idx:]
	if strings.Contains(modify, "huge.go") {
		t.Error("an over-limit file must not be listed as modifiable")
	}
	if !strings.Contains(excluded, "huge.go") || !strings.Contains(excluded, fmt.Sprintf("%d", agentFileSizeLimit+1000)) {
		t.Errorf("the over-limit file must be named in the excluded section with its line count:\n%s", excluded)
	}
	if !strings.Contains(modify, "small.go") {
		t.Error("the under-limit file must remain modifiable")
	}

	sf, ok := findSignal(p, "stale_file")
	if !ok {
		t.Fatal("expected a stale_file item")
	}
	if sf.AgentSpec != "" {
		t.Errorf("a safe item must produce no agent spec, got:\n%s", sf.AgentSpec)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func TestItemCarriesVerification(t *testing.T) {
	it := Item{ID: "HMF-001", Signal: "dead_module", Verification: "build-only"}
	if it.Verification != "build-only" {
		t.Fatalf("Item must carry a Verification verdict; got %q", it.Verification)
	}
}

func findSignal(p Plan, signal string) (Item, bool) {
	for _, it := range p.Items {
		if it.Signal == signal {
			return it, true
		}
	}
	return Item{}, false
}

// TestSignalDescriptorRegistryIsComplete guards ROADMAP #9 + #16: every analyze
// signal must have a descriptor (else Build silently drops it), every non-safe
// signal must carry an agent instruction (else buildAgentSpec falls back silently),
// no safe signal may carry a dead instruction buildAgentSpec never reads, and every
// descriptor must have an in-range order tier. It iterates analyze.Signals() — the
// canonical registry, NOT a list re-hardcoded here — so a future detector that
// forgets a remediation fails this test instead of vanishing from the plan.
func TestSignalDescriptorRegistryIsComplete(t *testing.T) {
	for _, sig := range analyze.Signals() {
		d, ok := descriptors[sig]
		if !ok {
			t.Errorf("signal %q has no descriptor — Build would silently drop it", sig)
			continue
		}
		if d.tier < 0 || d.tier > 2 {
			t.Errorf("signal %q has an out-of-range order tier %d", sig, d.tier)
		}
		if d.safety == "safe" {
			if d.instruction != "" {
				t.Errorf("safe signal %q carries a dead agent instruction (buildAgentSpec short-circuits safe items)", sig)
			}
		} else if d.instruction == "" {
			t.Errorf("non-safe signal %q has no agent instruction — buildAgentSpec would fall back silently", sig)
		}
	}
}
