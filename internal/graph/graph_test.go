package graph

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/schylerryan/humify/internal/area"
	"github.com/schylerryan/humify/internal/scan"
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

func TestBuildEdgesGoModulePaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.26\n")
	// Single-line import form.
	writeFile(t, filepath.Join(root, "main.go"),
		"package main\nimport \"example.com/app/util\"\nimport \"fmt\"\nfunc main() {}\n")
	// Grouped import form — the common Go style.
	writeFile(t, filepath.Join(root, "cmd", "run.go"),
		"package cmd\n\nimport (\n\t\"fmt\"\n\t\"example.com/app/util\"\n)\n\nvar _ = fmt.Sprint\n")
	writeFile(t, filepath.Join(root, "util", "util.go"), "package util\nfunc U() {}\n")

	files := []scan.File{
		{Rel: "main.go", LOC: 4}, {Rel: "cmd/run.go", LOC: 7}, {Rel: "util/util.go", LOC: 2},
	}
	areas := area.Decompose(files, 1000) // group by top-level dir: root, cmd, util
	edges := BuildEdges(root, areas)
	// Both main (root) and cmd import util; "fmt" is external. Expect two edges into util.
	intoUtil := 0
	for _, e := range edges {
		if strings.HasSuffix(e.To, "util") {
			intoUtil++
		}
	}
	if intoUtil != 2 {
		t.Fatalf("want 2 module-path edges into util (single-line + grouped), got %d of %+v", intoUtil, edges)
	}
}

func TestBuildEdgesDeepWaves(t *testing.T) {
	// a -> b -> c -> d: a four-link dependency chain across top-level dirs, the
	// deep multi-wave case real OSS dogfood targets (cobra/testify) never reached.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.26\n")
	writeFile(t, filepath.Join(root, "a", "a.go"), "package a\nimport \"example.com/app/b\"\nvar _ = 0\n")
	writeFile(t, filepath.Join(root, "b", "b.go"), "package b\nimport \"example.com/app/c\"\nvar _ = 0\n")
	writeFile(t, filepath.Join(root, "c", "c.go"), "package c\nimport \"example.com/app/d\"\nvar _ = 0\n")
	writeFile(t, filepath.Join(root, "d", "d.go"), "package d\nvar _ = 0\n")
	files := []scan.File{{Rel: "a/a.go"}, {Rel: "b/b.go"}, {Rel: "c/c.go"}, {Rel: "d/d.go"}}
	areas := area.Decompose(files, 1000)
	r := Compute(idsOf(areas), BuildEdges(root, areas))
	if len(r.Waves) != 4 {
		t.Fatalf("a→b→c→d should yield 4 ordered waves, got %d: %v", len(r.Waves), r.Waves)
	}
	if !contains(r.Waves[0], areaForDir(t, areas, "d")) {
		t.Fatalf("d (no deps) must lead in wave 0, got %v", r.Waves)
	}
	if !contains(r.Waves[3], areaForDir(t, areas, "a")) {
		t.Fatalf("a (transitive dependent) must be last, got %v", r.Waves)
	}
}

func TestBuildEdgesAreaCycle(t *testing.T) {
	// Two top-level dirs that import each other. Go forbids package-level cycles,
	// but AREA-level cycles arise in real repos when two dirs mutually reference
	// each other's (different) packages — the cycle-cluster path that never fired
	// on cobra/testify (cycles=0). The graph must surface it, not hang or mislabel.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.26\n")
	writeFile(t, filepath.Join(root, "x", "x.go"), "package x\nimport \"example.com/app/y\"\nvar _ = 0\n")
	writeFile(t, filepath.Join(root, "y", "y.go"), "package y\nimport \"example.com/app/x\"\nvar _ = 0\n")
	files := []scan.File{{Rel: "x/x.go"}, {Rel: "y/y.go"}}
	areas := area.Decompose(files, 1000)
	r := Compute(idsOf(areas), BuildEdges(root, areas))
	if len(r.Cycles) != 1 {
		t.Fatalf("x↔y is one cycle cluster, got %d: %v", len(r.Cycles), r.Cycles)
	}
	cyc := r.CycleSet()
	if !cyc[areaForDir(t, areas, "x")] || !cyc[areaForDir(t, areas, "y")] {
		t.Fatalf("both x and y must be in the cycle set: %v", r.Cycles)
	}
}

func idsOf(areas []area.Area) []string {
	ids := make([]string, len(areas))
	for i, a := range areas {
		ids[i] = a.ID
	}
	return ids
}

func areaForDir(t *testing.T, areas []area.Area, root string) string {
	t.Helper()
	for _, a := range areas {
		if a.Root == root {
			return a.ID
		}
	}
	t.Fatalf("no area owns dir %q", root)
	return ""
}

func contains(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
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
