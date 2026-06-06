// Package consolidate is the fan-in engine — the stage humify never had. It
// reads the AUDIT_MANIFEST (the expected fragment set), loads every fragment
// fault-isolated, dedups findings, detects cross-reference cycles, and verifies
// completeness (covered vs pending). It fails closed: a missing manifest, a
// missing/invalid fragment, or any unconsolidated area is surfaced loudly as a
// blocker rather than silently producing a partial AUDIT.
package consolidate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"humify-ng/internal/fragment"
	"humify-ng/internal/manifest"
)

// Sentinel errors for the fail-closed manifest gate.
var (
	ErrNoManifest    = errors.New("no AUDIT_MANIFEST.json (run `humify heatmap` first)")
	ErrEmptyManifest = errors.New("AUDIT_MANIFEST.json lists no fragments")
)

var errDuplicateEntry = errors.New("duplicate manifest entry for this area")

// Conflict is one consolidation issue, bucketed by severity.
type Conflict struct {
	Bucket  string   `json:"bucket"` // blocker | warning | info
	Kind    string   `json:"kind"`
	Detail  string   `json:"detail"`
	Sources []string `json:"sources,omitempty"`
}

// Merged is a deduped finding plus the areas that reported it.
type Merged struct {
	fragment.Finding
	Sources []string `json:"sources"`
}

// Result is the full consolidation outcome.
type Result struct {
	Covered   []string   `json:"covered"`
	Pending   []string   `json:"pending"`
	Findings  []Merged   `json:"findings"`
	Conflicts []Conflict `json:"conflicts"`
	Blockers  int        `json:"blockers"`
	Warnings  int        `json:"warnings"`
	Infos     int        `json:"infos"`
	Status    string     `json:"status"` // BLOCKED | AWAITING | READY
}

type srcFinding struct {
	src string
	f   fragment.Finding
}

// Run executes the engine against the project at root.
func Run(root string) (Result, error) {
	m, err := manifest.Load(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{}, ErrNoManifest
		}
		return Result{}, err
	}
	if len(m.Fragments) == 0 {
		return Result{}, ErrEmptyManifest
	}

	var res Result
	covered := map[string]bool{}
	failReason := map[string]error{}           // why an expected fragment did not consolidate
	perArea := map[string][]fragment.Finding{} // findings buffered per covered area
	seen := map[string]bool{}                  // manifest area ids already encountered
	for _, e := range m.Fragments {
		if seen[e.AreaID] {
			// A second entry for the same area is a malformed manifest. Fail
			// closed: invalidate the area entirely rather than pick one entry.
			failReason[e.AreaID] = errDuplicateEntry
			delete(covered, e.AreaID)
			delete(perArea, e.AreaID)
			continue
		}
		seen[e.AreaID] = true
		frag, ferr := loadValid(root, e)
		if ferr != nil {
			failReason[e.AreaID] = ferr
			continue // area stays pending; one conflict emitted below
		}
		covered[e.AreaID] = true
		perArea[e.AreaID] = frag.Findings
	}

	// Consolidate findings only from finally-covered areas, deterministically.
	var items []srcFinding
	for _, id := range sortedKeys(covered) {
		for _, fd := range perArea[id] {
			items = append(items, srcFinding{src: id, f: fd})
		}
	}

	var dedupConf []Conflict
	res.Findings, dedupConf = dedup(items)
	res.Conflicts = append(res.Conflicts, dedupConf...)
	res.Conflicts = append(res.Conflicts, detectCycles(items)...) // from raw items, not post-dedup
	res.Covered = sortedKeys(covered)
	res.Pending = pendingAreas(m, covered)
	for _, p := range res.Pending {
		res.Conflicts = append(res.Conflicts, pendingConflict(p, failReason[p]))
	}
	res.tally()
	return res, nil
}

// pendingConflict emits exactly one blocker per unconsolidated area, separating
// a simply-absent fragment from one that exists but failed validation.
func pendingConflict(area string, err error) Conflict {
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return Conflict{
			Bucket: "blocker", Kind: "missing-fragment",
			Detail: "area " + area + " expected but no fragment present", Sources: []string{area},
		}
	}
	return Conflict{
		Bucket: "blocker", Kind: "invalid-fragment",
		Detail: area + ": " + err.Error(), Sources: []string{area},
	}
}

func loadValid(root string, e manifest.Entry) (fragment.Fragment, error) {
	var frag fragment.Fragment
	native := filepath.FromSlash(e.Path)
	// A legitimate fragment path is relative, root-local, and carries no volume.
	// The volume check stops Windows drive-relative escapes ("C:..\..\x") that
	// are neither absolute nor "..": prefixed yet still resolve above root.
	if e.Path == "" || filepath.IsAbs(native) || filepath.VolumeName(native) != "" {
		return frag, fmt.Errorf("manifest path must be relative and root-local, got %q", e.Path)
	}
	full := filepath.Join(root, filepath.Clean(native))
	rel, relErr := filepath.Rel(root, full)
	if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return frag, fmt.Errorf("manifest path escapes project root: %q", e.Path)
	}
	frag, err := fragment.Load(full)
	if err != nil {
		return frag, err
	}
	if err := frag.Validate(); err != nil {
		return frag, err
	}
	if frag.AreaID != e.AreaID {
		return frag, fmt.Errorf("fragment area_id %q != manifest %q", frag.AreaID, e.AreaID)
	}
	return frag, nil
}

// dedup merges findings sharing (file, line, normalized title). Same severity
// across reporters is an INFO merge; differing severity is a WARNING competing
// variant (we never silently pick — the strongest severity wins the merged
// record but the conflict is surfaced).
func dedup(items []srcFinding) ([]Merged, []Conflict) {
	type key struct {
		file  string
		line  int
		title string
	}
	idx := map[key]int{}
	var merged []Merged
	var conflicts []Conflict
	for _, it := range items {
		k := key{it.f.File, it.f.Line, normTitle(it.f.Title)}
		pos, ok := idx[k]
		if !ok {
			idx[k] = len(merged)
			merged = append(merged, Merged{Finding: it.f, Sources: []string{it.src}})
			continue
		}
		merged[pos].Sources = append(merged[pos].Sources, it.src)
		conflicts = append(conflicts, mergeConflict(&merged[pos], it.f))
	}
	return merged, conflicts
}

func mergeConflict(m *Merged, f fragment.Finding) Conflict {
	at := fmt.Sprintf("%s:%d %q", f.File, f.Line, f.Title)
	srcs := append([]string{}, m.Sources...)
	if m.Severity != f.Severity {
		c := Conflict{Bucket: "warning", Kind: "severity-conflict",
			Detail: fmt.Sprintf("%s reported as %s and %s", at, m.Severity, f.Severity), Sources: srcs}
		if fragment.SeverityRank(f.Severity) > fragment.SeverityRank(m.Severity) {
			m.Severity = f.Severity
		}
		return c
	}
	return Conflict{Bucket: "info", Kind: "duplicate",
		Detail: "merged duplicate " + at, Sources: srcs}
}

func pendingAreas(m manifest.Manifest, covered map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range m.Fragments {
		if !covered[e.AreaID] && !seen[e.AreaID] {
			seen[e.AreaID] = true
			out = append(out, e.AreaID)
		}
	}
	sort.Strings(out)
	return out
}

func (r *Result) tally() {
	for _, c := range r.Conflicts {
		switch c.Bucket {
		case "blocker":
			r.Blockers++
		case "warning":
			r.Warnings++
		case "info":
			r.Infos++
		}
	}
	switch {
	case r.Blockers > 0:
		r.Status = "BLOCKED"
	case r.Warnings > 0:
		r.Status = "AWAITING"
	default:
		r.Status = "READY"
	}
}

func normTitle(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
