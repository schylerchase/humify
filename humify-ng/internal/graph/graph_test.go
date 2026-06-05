package graph

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"humify-ng/internal/area"
	"humify-ng/internal/scan"
)

func TestComputeAcyclicChain(t *testing.T) {
	// A depends on B depends on C -> dependencies first: C, B, A.
	r := Compute([]string{"A", "B", "C"}, []Edge{{"A", "B"}, {"B", "C"}})
	want := [][]string{{"C"}, {"B"}, {"A"}}
	if !reflect.DeepEqual(r.Waves, want) {
		t.Fatalf("waves = %v, want %v", r.Waves, want)
	}
	if len(r.Cycles) != 0 {
		t.Fatalf("unexpected cycles: %v", r.Cycles)
	}
	if r.FanOut["A"] != 1 || r.FanIn["C"] != 1 {
		t.Fatalf("fan counts wrong: in=%v out=%v", r.FanIn, r.FanOut)
	}
}

func TestComputeParallelAndIsolated(t *testing.T) {
	// B and C both depend on A; D is isolated.
	// Wave 0: A, D (no deps). Wave 1: B, C.
	r := Compute([]string{"A", "B", "C", "D"}, []Edge{{"B", "A"}, {"C", "A"}})
	want := [][]string{{"A", "D"}, {"B", "C"}}
	if !reflect.DeepEqual(r.Waves, want) {
		t.Fatalf("waves = %v, want %v", r.Waves, want)
	}
}

func TestComputeCycleSurfaced(t *testing.T) {
	// E<->F is a true cycle; G merely depends on E (downstream, NOT in the cycle).
	r := Compute([]string{"E", "F", "G"}, []Edge{{"E", "F"}, {"F", "E"}, {"G", "E"}})
	if len(r.Cycles) != 1 || !reflect.DeepEqual(r.Cycles[0], []string{"E", "F"}) {
		t.Fatalf("cycle cluster = %v, want [[E F]]", r.Cycles)
	}
	cyc := r.CycleSet()
	if !cyc["E"] || !cyc["F"] || cyc["G"] {
		t.Fatalf("cycle set wrong (G must not be in cycle): %v", cyc)
	}
	// The cycle has no external deps -> wave 0; G depends on it -> wave 1.
	want := [][]string{{"E", "F"}, {"G"}}
	if !reflect.DeepEqual(r.Waves, want) {
		t.Fatalf("waves = %v, want %v", r.Waves, want)
	}
}

func TestComputeTwoSeparateCycles(t *testing.T) {
	r := Compute([]string{"A", "B", "C", "D"}, []Edge{{"A", "B"}, {"B", "A"}, {"C", "D"}, {"D", "C"}})
	if len(r.Cycles) != 2 {
		t.Fatalf("want 2 cycle clusters, got %v", r.Cycles)
	}
}

func TestBuildEdgesRelativeImports(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "src", "main.js"), `import { x } from "./util"; import "react";`)
	writeFile(t, filepath.Join(root, "src", "util.js"), `export const x = 1;`)
	writeFile(t, filepath.Join(root, "lib", "big.js"), `require("../src/util");`)

	files := []scan.File{
		{Rel: "src/main.js", LOC: 1}, {Rel: "src/util.js", LOC: 1}, {Rel: "lib/big.js", LOC: 1},
	}
	// Force each file into its own area by setting god threshold to 1.
	areas := area.Decompose(files, 1)
	edges := BuildEdges(root, areas)
	if len(edges) == 0 {
		t.Fatal("expected resolved relative-import edges, got none")
	}
	// "react" (bare import) must not produce an edge.
	for _, e := range edges {
		if e.To == "" {
			t.Fatalf("empty edge target: %+v", e)
		}
	}
}

func writeFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
