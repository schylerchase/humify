// Package graph builds the area dependency graph and partitions areas into
// parallel "waves" via Kahn topological sort. Unlike GSD's phase.cjs (which
// hard-errors on a cycle because its plan DAG is acyclic by construction),
// real codebase import graphs contain cycles — so this engine tolerates them:
// the acyclic part is ordered into waves, and the leftover strongly-coupled
// areas are surfaced as flagged cycle clusters placed in a trailing wave.
//
// Edge direction is dependency: From depends on To (From imports To). Waves
// are ordered dependencies-first, so an area's dependencies land in an earlier
// (or equal) wave than the area itself.
package graph

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/schylerryan/humify/internal/area"
	"github.com/schylerryan/humify/internal/scan"
)

// Edge is a dependency edge: From depends on To.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// Result is the computed wave partition plus coupling metrics.
type Result struct {
	Waves  [][]string     `json:"waves"`
	Cycles [][]string     `json:"cycles"`
	FanIn  map[string]int `json:"fan_in"`
	FanOut map[string]int `json:"fan_out"`
}

// BuildEdges resolves intra-repo imports into area->area edges. Two resolution
// schemes run side by side: relative imports (`./x`, JS/TS/Python) resolve to a
// file, and Go module-path imports (`<module>/pkg`) resolve to a package
// directory. Truly external imports (stdlib, third-party) couple nothing and are
// dropped. Without the module-path scheme, Go's absolute intra-repo imports all
// looked external, so every area collapsed into a single wave.
func BuildEdges(root string, areas []area.Area) []Edge {
	byPath := map[string]string{}  // rel-without-ext -> area id (relative imports resolve to a file)
	dirArea := map[string]string{} // package dir -> area id (module-path imports resolve to a dir)
	fileArea := map[string]string{}
	for _, a := range areas {
		for _, f := range a.Files {
			byPath[stripExt(f.Rel)] = a.ID
			dirArea[path.Dir(f.Rel)] = a.ID
			fileArea[f.Rel] = a.ID
		}
	}
	modulePath := goModulePath(root)
	seen := map[Edge]bool{}
	var edges []Edge
	for _, a := range areas {
		for _, f := range a.Files {
			abs := filepath.Join(root, filepath.FromSlash(f.Rel))
			from := fileArea[f.Rel]
			for _, imp := range scan.ImportsOf(abs) {
				to := resolve(f.Rel, imp, byPath)
				if to == "" {
					to = resolveModule(imp, modulePath, dirArea)
				}
				if to == "" || to == from {
					continue
				}
				if e := (Edge{From: from, To: to}); !seen[e] {
					seen[e] = true
					edges = append(edges, e)
				}
			}
		}
	}
	return edges
}

// resolveModule maps a Go module-path import to the area owning the imported
// package directory. Intra-repo Go imports are absolute (`<module>` or
// `<module>/<pkgdir>`), never relative, so the relative resolver never sees them.
// Returns "" for stdlib/third-party imports or when no go.mod gives a module path.
func resolveModule(imp, modulePath string, dirArea map[string]string) string {
	if modulePath == "" || imp == "" || imp[0] == '.' {
		return ""
	}
	if imp == modulePath {
		return dirArea["."]
	}
	if rel, ok := strings.CutPrefix(imp, modulePath+"/"); ok {
		return dirArea[rel]
	}
	return ""
}

// goModulePath reads the module path from root/go.mod, or "" when there is none
// (a non-Go target). Only the `module` directive is needed to recognize which
// imports point back into this repo.
func goModulePath(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

func resolve(fromRel, imp string, byPath map[string]string) string {
	if imp == "" || imp[0] != '.' {
		return "" // bare/external import
	}
	joined := stripExt(path.Clean(path.Join(path.Dir(fromRel), imp)))
	if id, ok := byPath[joined]; ok {
		return id
	}
	if id, ok := byPath[joined+"/index"]; ok {
		return id
	}
	return ""
}

// Compute partitions ids into dependency-first waves and surfaces cycles.
// It condenses strongly-connected components (true cycles) via Tarjan, then
// topo-sorts the condensation so that a node merely downstream of a cycle is
// ordered into a later wave rather than mislabeled as part of the cycle.
func Compute(ids []string, edges []Edge) Result {
	res := Result{FanIn: map[string]int{}, FanOut: map[string]int{}}
	valid := map[string]bool{}
	for _, id := range ids {
		valid[id] = true
	}
	deps := map[string][]string{} // From -> dependencies (To), deduped
	seen := map[Edge]bool{}
	for _, e := range edges {
		if !valid[e.From] || !valid[e.To] || e.From == e.To || seen[e] {
			continue
		}
		seen[e] = true
		res.FanOut[e.From]++
		res.FanIn[e.To]++
		deps[e.From] = append(deps[e.From], e.To)
	}
	sccOf, sccs := tarjan(ids, deps)
	res.Cycles = cycleClusters(sccs)
	res.Waves = condensationWaves(deps, sccOf, sccs)
	return res
}

// tarjan returns each node's SCC index and the list of SCCs (members sorted).
func tarjan(ids []string, deps map[string][]string) (map[string]int, [][]string) {
	index := map[string]int{}
	low := map[string]int{}
	onStack := map[string]bool{}
	sccOf := map[string]int{}
	var stack []string
	var sccs [][]string
	counter := 0

	var visit func(v string)
	visit = func(v string) {
		index[v], low[v] = counter, counter
		counter++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range deps[v] {
			if _, ok := index[w]; !ok {
				visit(w)
				low[v] = min(low[v], low[w])
			} else if onStack[w] {
				low[v] = min(low[v], index[w])
			}
		}
		if low[v] == index[v] {
			sccs = append(sccs, popSCC(&stack, onStack, sccOf, len(sccs), v))
		}
	}
	for _, v := range ids {
		if _, ok := index[v]; !ok {
			visit(v)
		}
	}
	return sccOf, sccs
}

func popSCC(stack *[]string, onStack map[string]bool, sccOf map[string]int, idx int, root string) []string {
	var comp []string
	for {
		w := (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
		onStack[w] = false
		sccOf[w] = idx
		comp = append(comp, w)
		if w == root {
			break
		}
	}
	sort.Strings(comp)
	return comp
}

func cycleClusters(sccs [][]string) [][]string {
	var out [][]string
	for _, s := range sccs {
		if len(s) > 1 {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

// condensationWaves topo-sorts the SCC DAG dependencies-first; SCCs at the
// same level are merged into one parallel wave.
func condensationWaves(deps map[string][]string, sccOf map[string]int, sccs [][]string) [][]string {
	n := len(sccs)
	dependents := make([][]int, n) // dependency-scc -> dependent-sccs (for Kahn)
	indeg := make([]int, n)        // count of dependency SCCs
	seen := map[[2]int]bool{}
	for from, tos := range deps {
		fs := sccOf[from]
		for _, to := range tos {
			ts := sccOf[to]
			key := [2]int{fs, ts}
			if fs == ts || seen[key] {
				continue
			}
			seen[key] = true
			dependents[ts] = append(dependents[ts], fs)
			indeg[fs]++
		}
	}
	placed := make([]bool, n)
	var waves [][]string
	for {
		var wave []string
		var level []int
		for s := 0; s < n; s++ {
			if !placed[s] && indeg[s] == 0 {
				level = append(level, s)
			}
		}
		if len(level) == 0 {
			return waves
		}
		for _, s := range level {
			placed[s] = true
			wave = append(wave, sccs[s]...)
			for _, dep := range dependents[s] {
				indeg[dep]--
			}
		}
		sort.Strings(wave)
		waves = append(waves, wave)
	}
}

// WaveOf maps each area id to its wave index.
func (r Result) WaveOf() map[string]int {
	m := map[string]int{}
	for w, ids := range r.Waves {
		for _, id := range ids {
			m[id] = w
		}
	}
	return m
}

// CycleSet returns the set of area ids that participate in a cycle.
func (r Result) CycleSet() map[string]bool {
	m := map[string]bool{}
	for _, c := range r.Cycles {
		for _, id := range c {
			m[id] = true
		}
	}
	return m
}

func stripExt(p string) string { return p[:len(p)-len(path.Ext(p))] }
