package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"humify/internal/area"
	"humify/internal/intel"
	"humify/internal/layout"
	"humify/internal/manifest"
)

// --- fixture builders (advance a .humify/ project through the lifecycle) ---

func writeIntel(t *testing.T, root string, ids []string, waves [][]string) {
	t.Helper()
	areas := make([]area.Area, len(ids))
	for i, id := range ids {
		areas[i] = area.Area{ID: id}
		if err := os.MkdirAll(filepath.Join(layout.AreasDir(root), id), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := intel.Write(root, intel.Data{Target: "/tgt", Areas: areas, Waves: waves}); err != nil {
		t.Fatal(err)
	}
}

func writeManifest(t *testing.T, root string, ids []string) {
	t.Helper()
	var es []manifest.Entry
	for _, id := range ids {
		es = append(es, manifest.Entry{AreaID: id, Path: layout.AreaFragmentRel(id)})
	}
	if err := manifest.Write(root, manifest.Manifest{Fragments: es}); err != nil {
		t.Fatal(err)
	}
}

func writeAreaFile(t *testing.T, root, id, name, body string) {
	t.Helper()
	p := filepath.Join(layout.AreasDir(root), id, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFragment(t *testing.T, root, id string, withFinding bool) {
	t.Helper()
	finding := ""
	if withFinding {
		finding = `{"id":"` + id + `-1","title":"god file","severity":"warning","file":"a.go","line":1}`
	}
	writeAreaFile(t, root, id, id+"-AUDIT-fragment.json", `{"area_id":"`+id+`","findings":[`+finding+`]}`)
}

func writeAudit(t *testing.T, root string, ids []string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("# AUDIT\n\n## Areas consolidated\n")
	for _, id := range ids {
		fmt.Fprintf(&b, "- %s\n", id)
	}
	if err := os.WriteFile(layout.AuditFile(root), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePlanCheck(t *testing.T, root, id string, accepted bool) {
	t.Helper()
	issues := `{"severity":"blocker","title":"x","detail":"y"}`
	if accepted {
		issues = ""
	}
	writeAreaFile(t, root, id, id+"-PLAN-CHECK.json", `{"area_id":"`+id+`","issues":[`+issues+`]}`)
}

func writePatchlog(t *testing.T, root string, ids []string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("# Patchlog\n\n## Patched areas\n")
	for _, id := range ids {
		fmt.Fprintf(&b, "- %s\n", id)
	}
	if err := os.WriteFile(layout.PatchlogFile(root), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustNext(t *testing.T, root string, wantStage Stage, wantReason string) {
	t.Helper()
	got := Next(root)
	if got.Stage != wantStage || got.Reason != wantReason {
		t.Fatalf("Next = {%s, %s}, want {%s, %s} (detail: %s)", got.Stage, got.Reason, wantStage, wantReason, got.Detail)
	}
}

// TestNextWalksLifecycle drives one area through every stage and asserts the
// reducer names the right next step at each point.
func TestNextWalksLifecycle(t *testing.T) {
	root := t.TempDir()
	id := "01-a"
	waves := [][]string{{id}}

	// 0. nothing on disk → bootstrap.
	mustNext(t, root, StageHeatmap, "needs_bootstrap")

	// 1. bootstrapped, no fragment → audit.
	writeIntel(t, root, []string{id}, waves)
	writeManifest(t, root, []string{id})
	mustNext(t, root, StageAudit, "audit_pending")

	// 2. fragment on disk but no AUDIT.md → consolidate (the drift state).
	writeFragment(t, root, id, true)
	mustNext(t, root, StageConsolidate, "audit_incomplete")

	// 3. AUDIT.md covers it; finding has no accepted plan → plan.
	writeAudit(t, root, []string{id})
	mustNext(t, root, StagePlan, "plan_pending")

	// 4. PLAN.md present but check still has a blocker → still plan.
	writeAreaFile(t, root, id, id+"-PLAN.md", "# plan")
	writePlanCheck(t, root, id, false)
	mustNext(t, root, StagePlan, "plan_pending")

	// 5. check accepted (0 blocking), not executed → execute.
	writePlanCheck(t, root, id, true)
	mustNext(t, root, StageExecute, "execute_pending")

	// 6. SUMMARY.md present (executed) but not patched → patchlog.
	writeAreaFile(t, root, id, id+"-SUMMARY.md", "done")
	mustNext(t, root, StagePatchlog, "patchlog_pending")

	// 7. PATCHLOG.md covers it → done.
	writePatchlog(t, root, []string{id})
	mustNext(t, root, StageDone, "complete")
}

// TestNextIntelDrift: a manifest area absent from intel is corruption → heatmap, blocked.
func TestNextIntelDrift(t *testing.T) {
	root := t.TempDir()
	writeIntel(t, root, []string{"01-a"}, [][]string{{"01-a"}})
	writeManifest(t, root, []string{"01-a", "02-ghost"}) // 02-ghost not in intel
	step := Next(root)
	if step.Stage != StageHeatmap || step.Reason != "intel_drift" || !step.Blocked {
		t.Fatalf("Next = %+v, want heatmap/intel_drift/blocked", step)
	}
}

// TestNextCleanAuditIsDone: a fragment with no findings means nothing to plan or
// execute — the pipeline is complete once audited.
func TestNextCleanAuditIsDone(t *testing.T) {
	root := t.TempDir()
	id := "01-a"
	writeIntel(t, root, []string{id}, [][]string{{id}})
	writeManifest(t, root, []string{id})
	writeFragment(t, root, id, false) // no findings
	writeAudit(t, root, []string{id})
	mustNext(t, root, StageDone, "complete")
}

// TestCheckNotBootstrapped: no stage may report complete on a fresh project —
// the later stages once passed vacuously because their gate counters were zero.
func TestCheckNotBootstrapped(t *testing.T) {
	root := t.TempDir()
	for _, st := range Order {
		if Check(root, st).Pass {
			t.Fatalf("Check(%s) must fail on a non-bootstrapped project", st)
		}
	}
}

// TestNextEmptyManifestBlocks: a valid-but-empty manifest makes consolidate.Run
// fail; that error must surface as blocked, not be swallowed into "done".
func TestNextEmptyManifestBlocks(t *testing.T) {
	root := t.TempDir()
	writeIntel(t, root, []string{"01-a"}, [][]string{{"01-a"}})
	if err := manifest.Write(root, manifest.Manifest{}); err != nil { // zero fragments
		t.Fatal(err)
	}
	step := Next(root)
	if step.Stage != StageConsolidate || !step.Blocked {
		t.Fatalf("empty manifest must block at consolidate, got %+v", step)
	}
	if Check(root, StageConsolidate).Pass {
		t.Fatal("verify consolidate must fail on an empty manifest")
	}
}

// TestCheckPerStage spot-checks that Check agrees with where Next stops.
func TestCheckPerStage(t *testing.T) {
	root := t.TempDir()
	id := "01-a"
	writeIntel(t, root, []string{id}, [][]string{{id}})
	writeManifest(t, root, []string{id})
	writeFragment(t, root, id, true)
	writeAudit(t, root, []string{id})
	// audited but unplanned: audit+consolidate pass, plan fails.
	if r := Check(root, StageAudit); !r.Pass {
		t.Fatalf("audit should pass: %+v", r)
	}
	if r := Check(root, StageConsolidate); !r.Pass {
		t.Fatalf("consolidate should pass: %+v", r)
	}
	if r := Check(root, StagePlan); r.Pass {
		t.Fatalf("plan should fail (no accepted plan): %+v", r)
	}
	if r := Check(root, Stage("bogus")); r.Pass || r.Reason != "unknown_stage" {
		t.Fatalf("unknown stage must fail with unknown_stage: %+v", r)
	}
}
