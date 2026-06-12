package plan

import (
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

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func findSignal(p Plan, signal string) (Item, bool) {
	for _, it := range p.Items {
		if it.Signal == signal {
			return it, true
		}
	}
	return Item{}, false
}
