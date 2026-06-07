package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"humify-ng/internal/area"
	"humify-ng/internal/graph"
	"humify-ng/internal/heatmap"
	"humify-ng/internal/layout"
	"humify-ng/internal/manifest"
	"humify-ng/internal/output"
	"humify-ng/internal/scan"
)

// intel is the machine-readable decomposition written to .humify/intel/areas.json.
type intel struct {
	Target string          `json:"target"`
	Files  int             `json:"source_files"`
	Areas  []area.Area     `json:"areas"`
	Edges  []graph.Edge    `json:"edges"`
	Waves  [][]string      `json:"waves"`
	Cycles [][]string      `json:"cycles"`
	Scores []heatmap.Score `json:"scores"`
}

// cmdHeatmap scans a target codebase and bootstraps a .humify/ project:
// decomposition -> dependency DAG -> waves -> risk scores -> HEATMAP.md +
// area scaffold + intel. Pure deterministic code; no agents.
func cmdHeatmap(opts options) int {
	if opts.target == "" {
		return fail(opts, "missing_target", exitError, "heatmap requires --target=DIR")
	}
	files, err := scan.WalkSource(opts.target)
	if err != nil {
		return fail(opts, "scan_error", exitError, "scan failed: "+err.Error())
	}
	if len(files) == 0 {
		return fail(opts, "no_source", exitError, "no source files found under "+opts.target)
	}
	areas := area.Decompose(files, opts.godLOC)
	edges := graph.BuildEdges(opts.target, areas)
	g := graph.Compute(areaIDs(areas), edges)
	scores := heatmap.Rank(areas, g, heatmap.ChurnFromGit(opts.target, areas))

	root := opts.root
	if root == "" {
		root = "."
	}
	in := intel{
		Target: opts.target, Files: len(files), Areas: areas,
		Edges: edges, Waves: g.Waves, Cycles: g.Cycles, Scores: scores,
	}
	if err := writeProject(root, opts.target, scores, g, in); err != nil {
		return fail(opts, "write_error", exitError, "write failed: "+err.Error())
	}
	return emitHeatmap(opts, root, scores, g, len(files))
}

func areaIDs(areas []area.Area) []string {
	ids := make([]string, len(areas))
	for i, a := range areas {
		ids[i] = a.ID
	}
	return ids
}

func writeProject(root, target string, scores []heatmap.Score, g graph.Result, in intel) error {
	intelDir := filepath.Join(layout.HumifyDir(root), "intel")
	if err := os.MkdirAll(intelDir, 0o755); err != nil {
		return err
	}
	var expected []manifest.Entry
	for _, a := range in.Areas {
		if err := os.MkdirAll(filepath.Join(layout.AreasDir(root), a.ID), 0o755); err != nil {
			return err
		}
		expected = append(expected, manifest.Entry{AreaID: a.ID, Path: layout.AreaFragmentRel(a.ID)})
	}
	if err := manifest.Write(root, manifest.Manifest{Fragments: expected}); err != nil {
		return err
	}
	md := heatmap.RenderMarkdown(target, scores, g, in.Files)
	if err := os.WriteFile(filepath.Join(layout.HumifyDir(root), "HEATMAP.md"), []byte(md), 0o644); err != nil {
		return err
	}
	b, err := json.MarshalIndent(in, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(intelDir, "areas.json"), b, 0o644)
}

func emitHeatmap(opts options, root string, scores []heatmap.Score, g graph.Result, files int) int {
	top := scores
	if len(top) > 10 {
		top = top[:10]
	}
	if opts.json {
		data := map[string]any{
			"root": root, "areas": len(scores), "source_files": files,
			"waves": len(g.Waves), "cycle_clusters": len(g.Cycles), "top": top,
		}
		output.EmitJSON(os.Stdout, output.Result{Ok: true, ReasonCode: "ok", Data: data})
		return exitOK
	}
	fmt.Printf("scanned %d source files -> %d areas, %d waves, %d cycle cluster(s)\n",
		files, len(scores), len(g.Waves), len(g.Cycles))
	fmt.Printf("wrote %s\n\ntop risk areas:\n", filepath.Join(layout.HumifyDir(root), "HEATMAP.md"))
	for i, s := range top {
		fmt.Printf("  %2d. %-30s score=%-3d loc=%-6d maxfile=%-6d wave=%d\n",
			i+1, s.AreaID, s.Total, s.LOC, s.MaxFile, s.Wave)
	}
	return exitOK
}
