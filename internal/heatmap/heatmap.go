// Package heatmap turns the deterministic decomposition into a ranked risk
// score per area. Every input is mechanical (LOC, god-file size, a branch
// density complexity proxy, coupling from the dependency graph, git churn, and
// a test-coverage gap) so the ranking is reproducible — the interpretation is
// left to the later auditor agents, the ranking is not.
package heatmap

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/schylerryan/humify/internal/area"
	"github.com/schylerryan/humify/internal/graph"
)

// Score is one area's risk breakdown.
type Score struct {
	AreaID   string `json:"area_id"`
	Total    int    `json:"score"`
	LOC      int    `json:"loc"`
	MaxFile  int    `json:"max_file_loc"`
	FanIn    int    `json:"fan_in"`
	FanOut   int    `json:"fan_out"`
	Churn    int    `json:"churn"`
	HasTests bool   `json:"has_tests"`
	Wave     int    `json:"wave"`
	InCycle  bool   `json:"in_cycle"`
}

// Rank scores areas by composite risk and returns them highest-first.
func Rank(areas []area.Area, g graph.Result, churn map[string]int) []Score {
	waveOf := g.WaveOf()
	cyc := g.CycleSet()
	out := make([]Score, 0, len(areas))
	for _, a := range areas {
		s := Score{
			AreaID: a.ID, LOC: a.LOC, MaxFile: a.MaxFileLOC,
			FanIn: g.FanIn[a.ID], FanOut: g.FanOut[a.ID], Churn: churn[a.ID],
			HasTests: a.HasTests, Wave: waveOf[a.ID], InCycle: cyc[a.ID],
		}
		s.Total = composite(a, s)
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Total != out[j].Total {
			return out[i].Total > out[j].Total
		}
		return out[i].AreaID < out[j].AreaID
	})
	return out
}

// composite weights each dimension; god-file size dominates because an
// oversized single file is the canonical untangling hotspot.
func composite(a area.Area, s Score) int {
	total := 2 * band(a.LOC, 200, 800, 2500)
	total += 3 * band(a.MaxFileLOC, 500, 1500, 4000)
	total += band(density(a), 20, 50, 100)
	total += band(s.FanIn+s.FanOut, 1, 3, 8)
	total += band(s.Churn, 1, 5, 20)
	total += 2 * testGap(a)
	if s.InCycle {
		total += 2
	}
	return total
}

func density(a area.Area) int {
	if a.LOC == 0 {
		return 0
	}
	return a.Branches * 1000 / a.LOC
}

func testGap(a area.Area) int {
	if a.HasTests {
		return 0
	}
	return band(a.LOC, 200, 800, 2500)
}

func band(v, t1, t2, t3 int) int {
	switch {
	case v >= t3:
		return 3
	case v >= t2:
		return 2
	case v >= t1:
		return 1
	default:
		return 0
	}
}

// ChurnFromGit counts recent file-changes per area from git history. Returns an
// empty map (not an error) if the target is not a git repository.
func ChurnFromGit(target string, areas []area.Area) map[string]int {
	owner := map[string]string{}
	for _, a := range areas {
		for _, f := range a.Files {
			owner[f.Rel] = a.ID
		}
	}
	out := map[string]int{}
	b, err := exec.Command("git", "-C", target, "log", "--name-only", "--pretty=format:", "-n", "2000").Output()
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		rel := filepath.ToSlash(strings.TrimSpace(line))
		if id, ok := owner[rel]; ok {
			out[id]++
		}
	}
	return out
}

// RenderMarkdown builds HEATMAP.md: a coverage statement, the ranked table,
// the wave plan, and any cycle clusters.
func RenderMarkdown(target string, scores []Score, g graph.Result, fileCount int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Humify Heatmap\n\nTarget: `%s`\n\n", target)
	fmt.Fprintf(&b, "Coverage: %d areas decomposed from %d source files (map coverage only — "+
		"scores are deterministic; no file was deeply read).\n\n", len(scores), fileCount)
	b.WriteString("## Ranked areas\n\n")
	b.WriteString("| Rank | Area | Score | LOC | MaxFile | FanIn | FanOut | Churn | Tests | Wave | Flags |\n")
	b.WriteString("| ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | :---: | ---: | --- |\n")
	for i, s := range scores {
		b.WriteString(scoreRow(i+1, s))
	}
	b.WriteString("\n## Waves (dependency-first parallelization)\n\n")
	for i, w := range g.Waves {
		fmt.Fprintf(&b, "- wave %d: %s\n", i, strings.Join(w, ", "))
	}
	if len(g.Cycles) > 0 {
		b.WriteString("\n## Cycle clusters (strongly coupled — process together)\n\n")
		for _, c := range g.Cycles {
			fmt.Fprintf(&b, "- %s\n", strings.Join(c, ", "))
		}
	}
	return b.String()
}

func scoreRow(rank int, s Score) string {
	tests := "no"
	if s.HasTests {
		tests = "yes"
	}
	flags := ""
	if s.InCycle {
		flags = "cycle"
	}
	return fmt.Sprintf("| %d | %s | %d | %d | %d | %d | %d | %d | %s | %d | %s |\n",
		rank, s.AreaID, s.Total, s.LOC, s.MaxFile, s.FanIn, s.FanOut, s.Churn, tests, s.Wave, flags)
}
