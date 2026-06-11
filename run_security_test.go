package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/schylerryan/humify/internal/area"
	"github.com/schylerryan/humify/internal/plan"
)

// TestDispatchPlanRejectsEscapingID locks the CRITICAL fix at its actual call site
// (not just the SafeAreaID predicate): an area id with ".." segments — reachable
// via a hand-edited .humify or a malicious target repo, since the manifest/fragment
// read path does not charset-validate ids — must be rejected by dispatchPlan before
// any prompt is written, and must not write a file outside the project root.
func TestDispatchPlanRejectsEscapingID(t *testing.T) {
	root := t.TempDir()
	evil := "../../../../tmp/humify-plan-escape"
	d := plan.Decision{PlanAreas: []string{evil}, Status: plan.StatusDispatch}

	_, err := dispatchPlan(root, "src", map[string]area.Area{}, map[string][]plan.Finding{}, d)
	if err == nil || !strings.Contains(err.Error(), "unsafe area id") {
		t.Fatalf("dispatchPlan err = %v; want an unsafe-area-id rejection (the gate must fire)", err)
	}
	outside := filepath.Join(filepath.Dir(root), "tmp", "humify-plan-escape.prompt.md")
	if _, statErr := os.Stat(outside); statErr == nil {
		os.Remove(outside)
		t.Fatalf("escaping planner prompt was written outside root at %s", outside)
	}
}
