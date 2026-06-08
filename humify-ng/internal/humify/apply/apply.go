// Package apply is Humify's only state-changing command, and it is deliberately
// timid. It executes exactly one plan item, only if that item is marked applyable
// (today: the reversible "quarantine stale files" action), only when explicitly
// confirmed. When the project has detectable validation commands it re-runs them
// and rolls the change back if a previously-passing check regresses; when it has
// none it relies on the action being reversible and records honestly that nothing
// was validated rather than claiming a pass. Everything else it explains and refuses.
package apply

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"humify-ng/internal/humify/plan"
	"humify-ng/internal/humify/state"
	"humify-ng/internal/humify/verify"
)

// FileMove records one quarantined file for the manifest.
type FileMove struct {
	Original string `json:"original"`
	New      string `json:"new"`
	Reason   string `json:"reason"`
}

// Manifest is the reversible record written into a quarantine directory.
type Manifest struct {
	Schema     int        `json:"schema"`
	Tool       string     `json:"tool"`
	PlanItem   string     `json:"plan_item"`
	Timestamp  string     `json:"timestamp"`
	Validation ValSummary `json:"validation"`
	Files      []FileMove `json:"files"`
}

// ValSummary is the apply-time validation outcome stored with a quarantine.
type ValSummary struct {
	Ran    bool   `json:"ran"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

// Result is what apply did, for the command layer to render.
type Result struct {
	ItemID       string
	Action       string
	DryRun       bool
	Applied      bool
	Skipped      bool
	RolledBack   bool
	RepoDirty    bool
	Validated    bool // a validation command actually ran to confirm the change
	Moves        []FileMove
	Validation   *verify.Validation
	ManifestPath string
	Message      string
}

// Apply executes one plan item. dryRun (the default) only describes the change;
// yes performs it. now is injected for testable timestamps.
func Apply(root string, p plan.Plan, itemID string, dryRun, yes bool, now time.Time) (Result, error) {
	item, ok := p.Find(itemID)
	if !ok {
		return Result{}, fmt.Errorf("no plan item %q (run `humify plan` and pick an HMF-### id)", itemID)
	}
	res := Result{ItemID: item.ID, Action: item.Action, DryRun: dryRun}
	if !item.Applyable || item.Action != "quarantine" {
		res.Skipped = true
		res.Message = fmt.Sprintf("%s (%s) is not auto-applyable — automation safety is %q. Humify will not modify source for this item; address it by hand, then run `humify verify`.",
			item.ID, item.Title, item.AutomationSafety)
		return res, nil
	}

	moves := plannedMoves(root, item) // absolute paths for filesystem ops
	res.Moves = relManifest(root, moves)
	if len(moves) == 0 {
		res.Message = "Nothing to quarantine — the files named by this item no longer exist."
		return res, nil
	}
	if dryRun || !yes {
		res.Message = fmt.Sprintf("Dry run: would quarantine %d file(s) into %s. Re-run with `--target %s --yes` to apply.",
			len(moves), relQuarantine(item.ID), item.ID)
		return res, nil
	}
	return performQuarantine(root, item, moves, now)
}

// performQuarantine moves the files (absolute paths), verifies, and rolls back on
// regression. moves carry absolute paths; res.Moves is rewritten to root-relative
// for display and the manifest.
func performQuarantine(root string, item plan.Item, moves []FileMove, now time.Time) (Result, error) {
	res := Result{ItemID: item.ID, Action: item.Action, Moves: relManifest(root, moves)}
	res.RepoDirty = gitDirty(root)

	baseline, _ := verify.Run(root, now)
	done, err := move(moves)
	if err != nil {
		restore(done)
		return res, fmt.Errorf("quarantine aborted and rolled back: %w", err)
	}
	post, _ := verify.Run(root, now)
	res.Validation = &post
	res.Validated = post.Validated

	if regressed(baseline, post) {
		restore(moves)
		res.RolledBack = true
		res.Message = "Validation regressed after quarantine — change rolled back, no files moved. Investigate before retrying."
		return res, nil
	}

	manifestPath, err := writeManifest(root, item, moves, post)
	if err != nil {
		restore(moves)
		return res, fmt.Errorf("quarantine rolled back: could not write manifest: %w", err)
	}
	res.Applied = true
	res.ManifestPath = manifestPath
	res.Message = fmt.Sprintf("Quarantined %d file(s) into %s. %s Restore by moving files back from that directory.",
		len(moves), relQuarantine(item.ID), validationNote(post))
	return res, nil
}

// plannedMoves computes the source→quarantine moves (as absolute paths) for the
// item's files that still exist within the repo root.
func plannedMoves(root string, item plan.Item) []FileMove {
	var moves []FileMove
	for _, rel := range item.Files {
		src := filepath.Join(root, filepath.FromSlash(rel))
		if !withinRoot(root, src) || !isFile(src) {
			continue
		}
		dst := filepath.Join(state.QuarantineDir(root, item.ID), filepath.FromSlash(rel))
		moves = append(moves, FileMove{
			Original: src, New: dst,
			Reason: "stale file (quarantined by " + item.ID + ")",
		})
	}
	return moves
}

// move relocates each file into quarantine (paths are absolute), returning the
// moves it completed so a failure can be rolled back.
func move(moves []FileMove) ([]FileMove, error) {
	var done []FileMove
	for _, m := range moves {
		if err := os.MkdirAll(filepath.Dir(m.New), 0o755); err != nil {
			return done, err
		}
		if err := os.Rename(m.Original, m.New); err != nil {
			return done, err
		}
		done = append(done, m)
	}
	return done, nil
}

// restore moves quarantined files back to their original locations (best effort).
func restore(moves []FileMove) {
	for _, m := range moves {
		_ = os.MkdirAll(filepath.Dir(m.Original), 0o755)
		_ = os.Rename(m.New, m.Original)
	}
}

// writeManifest records the quarantine so it is auditable and reversible.
func writeManifest(root string, item plan.Item, moves []FileMove, post verify.Validation) (string, error) {
	man := Manifest{
		Schema: state.Schema, Tool: "humify", PlanItem: item.ID,
		Timestamp:  post.GeneratedAt,
		Validation: ValSummary{Ran: post.Validated, Passed: post.Validated && post.Passed, Detail: validationNote(post)},
		Files:      relManifest(root, moves),
	}
	data, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(state.QuarantineDir(root, item.ID), "manifest.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// regressed reports whether any validation kind that passed in baseline failed
// afterward — the signal that the change broke something.
func regressed(baseline, post verify.Validation) bool {
	passedBefore := map[string]bool{}
	for _, c := range baseline.Commands {
		if c.Ran && c.Passed {
			passedBefore[c.Kind] = true
		}
	}
	for _, c := range post.Commands {
		if c.Ran && !c.Passed && passedBefore[c.Kind] {
			return true
		}
	}
	return false
}

// gitDirty reports whether the repo has uncommitted changes. A non-repo or missing
// git returns false (nothing to warn about).
func gitDirty(root string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = root
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// validationNote summarizes a validation run for human output.
func validationNote(v verify.Validation) string {
	if !v.Validated {
		return "No validation commands detected, so the change could not be auto-verified — review the quarantine."
	}
	if v.Passed {
		return "Validation passed after the change."
	}
	return "Validation reported failures (see .humify/validation.json) — review before trusting the change."
}

// relManifest rewrites move paths to be root-relative for the on-disk manifest.
func relManifest(root string, moves []FileMove) []FileMove {
	out := make([]FileMove, len(moves))
	for i, m := range moves {
		out[i] = FileMove{Original: toRel(root, m.Original), New: toRel(root, m.New), Reason: m.Reason}
	}
	return out
}

func toRel(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(p)
}

func relQuarantine(id string) string {
	return filepath.ToSlash(filepath.Join(state.Dir, state.DeleteMeDir, id))
}

func withinRoot(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}
