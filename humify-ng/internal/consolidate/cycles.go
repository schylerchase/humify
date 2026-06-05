package consolidate

import (
	"sort"
	"strings"
)

// detectCycles builds a cross-reference graph from the RAW per-source findings
// (each area -> the areas its own findings reference) and reports each cycle as
// a blocker. It must use raw items, not post-dedup Merged records: dedup keeps
// only one finding's refs and would both drop a merged-away finding's real
// edges and replicate the survivor's edges onto every source — corrupting a
// fail-closed detector in both directions.
func detectCycles(items []srcFinding) []Conflict {
	adj := map[string][]string{}
	for _, it := range items {
		adj[it.src] = append(adj[it.src], it.f.Refs...)
	}
	var out []Conflict
	for _, c := range findCycles(adj) {
		out = append(out, Conflict{
			Bucket: "blocker", Kind: "cross-ref-cycle",
			Detail: "cross-reference cycle: " + strings.Join(c, " -> "), Sources: c,
		})
	}
	return out
}

// findCycles runs a three-color DFS and returns each distinct cycle (deduped
// across rotations). It needs no recursion-depth cap: a node is grayed on entry
// and recursion descends only into white nodes, so each node is entered at most
// once and the traversal always terminates. A path-length cap would only
// produce false cycles on deep-but-acyclic chains, so there is none.
func findCycles(adj map[string][]string) [][]string {
	const white, gray, black = 0, 1, 2
	color := map[string]int{}
	var path []string
	var cycles [][]string

	var dfs func(n string)
	dfs = func(n string) {
		color[n] = gray
		path = append(path, n)
		for _, w := range adj[n] {
			switch color[w] {
			case gray:
				cycles = append(cycles, extractCycle(path, w))
			case white:
				dfs(w)
			}
		}
		path = path[:len(path)-1]
		color[n] = black
	}

	for _, n := range sortedAdjKeys(adj) {
		if color[n] == white {
			dfs(n)
		}
	}
	return dedupCycles(cycles)
}

func extractCycle(path []string, start string) []string {
	for i, n := range path {
		if n == start {
			return append(append([]string{}, path[i:]...), start)
		}
	}
	return append([]string{}, path...)
}

func sortedAdjKeys(adj map[string][]string) []string {
	keys := make([]string, 0, len(adj))
	for k := range adj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// dedupCycles collapses rotations/reversals of the same cycle by hashing its
// node set.
func dedupCycles(cycles [][]string) [][]string {
	seen := map[string]bool{}
	var out [][]string
	for _, c := range cycles {
		sig := cycleSig(c)
		if seen[sig] {
			continue
		}
		seen[sig] = true
		out = append(out, c)
	}
	return out
}

func cycleSig(c []string) string {
	set := map[string]bool{}
	for _, n := range c {
		set[n] = true
	}
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, "|")
}
