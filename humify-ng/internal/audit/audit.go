// Package audit plans the fan-out half of the audit stage — the scatter that
// feeds the existing consolidate barrier. It derives which areas still need an
// auditor (resumable: an area whose fragment is already valid on disk is
// skipped), and turns each remaining area into a Job carrying the exact files
// that auditor owns and the fragment path it must write.
//
// The binary does not spawn auditors itself in v1. The default DispatchRunner
// materializes one prompt per pending area and hands the plan back to the
// orchestrator (the live agent host), which spawns the read-only auditors. The
// gather barrier and merge are the existing `humify consolidate` stage. Keeping
// spawn behind a Runner seam lets an autonomous `claude`/`codex` runner be
// added later without touching this deterministic planning code.
package audit

import (
	"errors"
	"os"
	"path/filepath"
	"sort"

	"humify-ng/internal/fragment"
	"humify-ng/internal/intel"
	"humify-ng/internal/layout"
	"humify-ng/internal/manifest"
)

// ErrNoManifest is the fail-closed gate for a project that was never bootstrapped.
var ErrNoManifest = errors.New("no AUDIT_MANIFEST.json (run `humify heatmap` first)")

// Job is one auditor assignment: read exactly Files, write one fragment.
type Job struct {
	AreaID       string   `json:"area_id"`
	Kind         string   `json:"kind"` // "dir" or "file"
	Root         string   `json:"root"` // path prefix the area owns, under Target
	Files        []string `json:"files"`
	LOC          int      `json:"loc"`
	Wave         int      `json:"wave"`
	FragmentPath string   `json:"fragment_path"` // relative to project root; where the auditor must write
	PromptPath   string   `json:"prompt_path"`   // relative to project root; where the prompt was/should be written
}

// Plan is the full audit dispatch plan: what to scatter and what is already done.
type Plan struct {
	Root    string   `json:"-"`
	Target  string   `json:"target"`
	Total   int      `json:"total_areas"`
	Pending []Job    `json:"pending"`
	Done    []string `json:"done"`
	Missing []string `json:"missing_from_intel,omitempty"` // manifest area absent from intel = drift
}

// BuildPlan derives the audit dispatch plan from disk. The manifest (written by
// heatmap) is the authoritative expected-fragment set; intel supplies each
// area's files/kind/wave. An area whose fragment already loads and validates is
// counted Done and never re-dispatched, so re-running after a partial scatter
// only picks up the stragglers.
func BuildPlan(root string) (Plan, error) {
	in, err := intel.Load(root)
	if err != nil {
		return Plan{}, err
	}
	m, err := manifest.Load(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Plan{}, ErrNoManifest
		}
		return Plan{}, err
	}

	byID := in.AreasByID()
	wave := in.WaveOf()

	plan := Plan{Root: root, Target: in.Target, Total: len(m.Fragments)}
	seen := map[string]bool{}
	for _, e := range m.Fragments {
		if seen[e.AreaID] {
			continue // duplicate manifest entry; consolidate handles the fail-closed case
		}
		seen[e.AreaID] = true

		a, ok := byID[e.AreaID]
		if !ok {
			plan.Missing = append(plan.Missing, e.AreaID)
			continue
		}
		if fragmentDone(root, e.Path, e.AreaID) {
			plan.Done = append(plan.Done, e.AreaID)
			continue
		}
		plan.Pending = append(plan.Pending, Job{
			AreaID:       a.ID,
			Kind:         a.Kind,
			Root:         a.Root,
			Files:        a.FilePaths,
			LOC:          a.LOC,
			Wave:         wave[a.ID],
			FragmentPath: e.Path,
			PromptPath:   promptPath(a.ID),
		})
	}

	sort.Strings(plan.Done)
	sort.Strings(plan.Missing)
	sort.SliceStable(plan.Pending, func(i, j int) bool {
		if plan.Pending[i].Wave != plan.Pending[j].Wave {
			return plan.Pending[i].Wave < plan.Pending[j].Wave
		}
		return plan.Pending[i].AreaID < plan.Pending[j].AreaID
	})
	return plan, nil
}

// promptPath is the conventional per-area prompt location, relative to root.
func promptPath(areaID string) string {
	return filepath.Join(layout.Dir, "tmp", "auditors", areaID+".prompt.md")
}

// fragmentDone reports whether an area's fragment already exists, parses, passes
// the mandatory-severity contract, and matches the manifest area id. Anything
// short of that leaves the area pending — never a false "done". The path is run
// through the same root-local gate consolidate uses, so an escape attempt in a
// hand-edited manifest is rejected here (treated as pending) instead of relying
// on the file happening not to exist; consolidate still rejects it loudly.
func fragmentDone(root, relPath, areaID string) bool {
	full, err := layout.ResolveInRoot(root, relPath)
	if err != nil {
		return false
	}
	f, err := fragment.Load(full)
	if err != nil {
		return false
	}
	if err := f.Validate(); err != nil {
		return false
	}
	return f.AreaID == areaID
}
