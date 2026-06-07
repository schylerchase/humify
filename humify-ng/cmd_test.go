package main

import (
	"testing"

	"humify-ng/internal/pipeline"
)

// nextVerb/sameNextCommand decide whether a HANDOFF cursor agrees with the
// disk-derived step — the gate that keeps a stale cursor from overriding disk.
func TestNextVerb(t *testing.T) {
	cases := map[string]string{
		"humify consolidate":            "consolidate",
		"humify heatmap --target=<dir>": "heatmap",
		"humify":                        "", // no verb
		"":                              "",
		"consolidate":                   "", // missing the humify prefix
	}
	for in, want := range cases {
		if got := nextVerb(in); got != want {
			t.Fatalf("nextVerb(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSameNextCommand(t *testing.T) {
	if !sameNextCommand("humify plan", "humify plan --max-replans=3") {
		t.Fatal("same verb with differing args must agree")
	}
	if sameNextCommand("humify audit", "humify consolidate") {
		t.Fatal("different verbs must not agree")
	}
	if sameNextCommand("", "humify audit") {
		t.Fatal("an empty/verbless command must never agree (no verb to match)")
	}
	if sameNextCommand("not humify", "not humify") {
		t.Fatal("non-humify strings have no verb and must not agree")
	}
}

// stagePos maps a stage to its place in the lifecycle; "done" maps past the end
// so a completed pipeline leaves no stage marked unreached.
func TestStagePos(t *testing.T) {
	if stagePos(pipeline.StageHeatmap) != 0 {
		t.Fatal("heatmap must be index 0")
	}
	if stagePos(pipeline.StageDone) != len(pipeline.Order) {
		t.Fatal("done must map to len(Order)")
	}
}
