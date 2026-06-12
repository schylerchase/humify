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

	"context"

	"github.com/schylerryan/humify/internal/humify/plan"
	"github.com/schylerryan/humify/internal/humify/state"
	"github.com/schylerryan/humify/internal/humify/verify"
)

// FileMove records one quarantined file for the manifest.
type FileMove struct {
	Original string `json:"original"`
	New      string `json:"new"`
	Reason   string `json:"reason"`
}

// Manifest is the reversible record written into a quarantine directory.
type Manifest struct {
	Schema       int        `json:"schema"`
	Tool         string     `json:"tool"`
	PlanItem     string     `json:"plan_item"`
	Timestamp    string     `json:"timestamp"`
	Verification string     `json:"verification,omitempty"`
	Validation   ValSummary `json:"validation"`
	Files        []FileMove `json:"files"`
}

// ValSummary is the apply-time validation outcome stored with a quarantine.
type ValSummary struct {
	Ran            bool     `json:"ran"`
	Passed         bool     `json:"passed"`
	AlreadyFailing []string `json:"already_failing,omitempty"` // kinds that failed before the change
	NewlyFailing   []string `json:"newly_failing,omitempty"`   // kinds that newly failed (regression)
	Fixed          []string `json:"fixed,omitempty"`           // kinds that newly passed
	Detail         string   `json:"detail"`
}

// Result is what apply did, for the command layer to render. JSON tags are
// snake_case to match the rest of the surface (plan.Item, verify.Validation).
type Result struct {
	ItemID       string             `json:"item_id"`
	Action       string             `json:"action"`
	DryRun       bool               `json:"dry_run"`
	Applied      bool               `json:"applied"`
	Skipped      bool               `json:"skipped"`
	RolledBack   bool               `json:"rolled_back"`
	RepoDirty    bool               `json:"repo_dirty"`
	Validated    bool               `json:"validated"` // a validation command actually ran to confirm the change
	Moves        []FileMove         `json:"moves"`
	Validation   *verify.Validation `json:"validation,omitempty"`
	ManifestPath string             `json:"manifest_path,omitempty"`
	Verification string             `json:"verification,omitempty"`
	Message      string             `json:"message"`
}

// Apply executes one plan item. dryRun (the default) only describes the change;
// yes performs it. agentCmd and unsafePermission together unlock autonomous agent
// execution for assisted/manual items — the caller is responsible for the
// double-confirmation gate before reaching here. now is injected for testable timestamps.
func Apply(root string, p plan.Plan, itemID string, dryRun, yes bool, agentCmd string, unsafePermission bool, now time.Time) (Result, error) {
	item, ok := p.Find(itemID)
	if !ok {
		return Result{}, fmt.Errorf("no plan item %q (run `humify plan` and pick an HMF-### id)", itemID)
	}
	res := Result{ItemID: item.ID, Action: item.Action, DryRun: dryRun}

	if unsafePermission && agentCmd != "" && !item.Applyable {
		if dryRun || !yes {
			res.Message = fmt.Sprintf("Dry run: would spawn agent for %s (%s, safety: %s). Re-run with --yes and confirm at the prompt to execute.\n\nAgent spec:\n%s",
				item.ID, item.Title, item.AutomationSafety, item.AgentSpec)
			return res, nil
		}
		return performAgentApply(root, item, agentCmd, now)
	}

	if !item.Applyable || item.Action != "quarantine" {
		res.Skipped = true
		res.Message = fmt.Sprintf("%s (%s) is not auto-applyable — automation safety is %q. Use --unsafe-permission --agent-cmd=CMD to execute autonomously, or address it by hand then run `humify verify`.",
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
	res2, err := performQuarantine(root, item, moves, now)
	res2.Verification = item.Verification
	return res2, err
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

	if outcome, kinds := gate(baseline, post); outcome != gateOK {
		restore(moves)
		res.RolledBack = true
		res.Message = "Rolled back the quarantine: " + rollbackReason(outcome, kinds) + ". No files were moved; investigate before retrying."
		return res, nil
	}

	manifestPath, err := writeManifest(root, item, moves, baseline, post)
	if err != nil {
		restore(moves)
		return res, fmt.Errorf("quarantine rolled back: could not write manifest: %w", err)
	}
	res.Applied = true
	res.ManifestPath = manifestPath
	msg := fmt.Sprintf("Quarantined %d file(s) into %s. %s Restore by moving files back from that directory.",
		len(moves), relQuarantine(item.ID), applyValidationNote(baseline, post))
	if item.Verification == "build-only" {
		msg += " (build-only: no test exercised this file)"
	}
	res.Message = msg
	return res, nil
}

// performAgentApply runs an external agent with the item's AgentSpec on stdin,
// then verifies and rolls back on regression. Unlike quarantine, there is no
// mechanical file-level undo — the agent's changes are in the working tree and
// must be reverted via git if the verification gate fails.
func performAgentApply(root string, item plan.Item, agentCmd string, now time.Time) (Result, error) {
	res := Result{ItemID: item.ID, Action: "agent", Applied: false}
	res.RepoDirty = gitDirty(root)

	baseline, _ := verify.Run(root, now)
	if err := runAgent(root, agentCmd, item.AgentSpec, 30*time.Minute); err != nil {
		res.Message = fmt.Sprintf("Agent exited with error: %v. Source may be partially modified — review with `git diff` and revert if needed.", err)
		return res, nil
	}

	post, _ := verify.Run(root, now)
	res.Validation = &post
	res.Validated = post.Validated

	if outcome, kinds := gate(baseline, post); outcome != gateOK {
		res.RolledBack = true
		res.Message = "Stopped after agent changes: " + rollbackReason(outcome, kinds) + ". The agent's edits are still in the working tree — run `git diff` to review and revert before committing."
		return res, nil
	}

	res.Applied = true
	res.Message = fmt.Sprintf("Agent completed %s (%s). %s Review changes with `git diff` before committing.", item.ID, item.Title, applyValidationNote(baseline, post))
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
			Reason: quarantineReason(item),
		})
	}
	return moves
}

// quarantineReason names why a file was quarantined, for the reversible manifest.
func quarantineReason(item plan.Item) string {
	what := "file"
	switch item.Signal {
	case "stale_file":
		what = "stale file"
	case "dead_module":
		what = "unreferenced module"
	}
	return fmt.Sprintf("%s (quarantined by %s)", what, item.ID)
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
func writeManifest(root string, item plan.Item, moves []FileMove, baseline, post verify.Validation) (string, error) {
	already, newly, fixed := computeDelta(baseline, post)
	man := Manifest{
		Schema: state.Schema, Tool: "humify", PlanItem: item.ID,
		Timestamp:    post.GeneratedAt,
		Verification: item.Verification,
		Validation: ValSummary{
			Ran: post.Validated, Passed: post.Validated && post.Passed,
			AlreadyFailing: already, NewlyFailing: newly, Fixed: fixed,
			Detail: applyValidationNote(baseline, post),
		},
		Files: relManifest(root, moves),
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

// gateOutcome is the verdict of comparing post-change validation against the
// baseline. It drives both the keep/rollback decision and the message it carries,
// so the two can never disagree about why a change was reverted.
type gateOutcome int

const (
	gateOK           gateOutcome = iota // safe to keep
	gateRegressed                       // a kind that was not already broken now cleanly fails
	gateUnverifiable                    // a kind's safety could not be confirmed (timed out / could not run)
)

// gate classifies a change at kind granularity (build/test/lint), returning the
// verdict and the kinds responsible. A command is a "clean fail" when it ran and
// exited with a real status (ExitCode >= 0); "indeterminate" when it ran but
// produced no clean status (ExitCode < 0: timed out, signalled, or failed to
// launch) — in which case we genuinely do not know whether that kind passes.
//
// Per kind: a post pass is fine. A post clean-fail is a regression UNLESS the
// baseline also cleanly failed it (then it is pre-existing). A post indeterminate
// is unverifiable UNLESS the baseline cleanly failed it — a kind that was already
// broken has no passing behavior left to protect, so being unable to verify it is
// not a reason to roll back. A regressed kind outranks an unverifiable one.
//
// This is the fix for the silent-disable hole: previously an indeterminate baseline
// counted as "failed", so a real post-failure on that kind was waved through as
// "already failing — no regression".
func gate(baseline, post verify.Validation) (gateOutcome, []string) {
	baseCleanFail := map[string]bool{}
	for _, c := range baseline.Commands {
		if c.Ran && !c.Passed && c.ExitCode >= 0 {
			baseCleanFail[c.Kind] = true
		}
	}
	var regressed, unverifiable []string
	for _, c := range post.Commands {
		switch {
		case !c.Ran || c.Passed:
			// nothing to protect against
		case c.ExitCode < 0: // indeterminate
			if !baseCleanFail[c.Kind] {
				unverifiable = append(unverifiable, c.Kind)
			}
		default: // clean fail
			if !baseCleanFail[c.Kind] {
				regressed = append(regressed, c.Kind)
			}
		}
	}
	if len(regressed) > 0 {
		return gateRegressed, regressed
	}
	if len(unverifiable) > 0 {
		return gateUnverifiable, unverifiable
	}
	return gateOK, nil
}

// rollbackReason renders why a change was reverted so the quarantine and agent
// paths describe the same gate verdict identically.
func rollbackReason(outcome gateOutcome, kinds []string) string {
	switch outcome {
	case gateRegressed:
		return fmt.Sprintf("validation regressed — %s newly failed", strings.Join(kinds, ", "))
	case gateUnverifiable:
		return fmt.Sprintf("could not verify the change — %s did not complete (timed out or failed to run); raise --timeout or stabilize the command", strings.Join(kinds, ", "))
	}
	return ""
}

// runAgent executes agentCmd through the shell with spec on stdin. Unlike
// spawn.ShellExec it does NOT set Setpgid, so the agent runs in the same
// process group as humify and can access the terminal normally — required for
// interactive CLI agents (claude, aider, etc.) that need TTY control.
func runAgent(dir, agentCmd, spec string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", agentCmd)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(spec)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// gitDirty reports whether the repo has uncommitted changes. A non-repo or missing
// git returns false (nothing to warn about).
func gitDirty(root string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = root
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// computeDelta classifies kinds into already-failing (cleanly failed before and
// after), newly-failing (regression — passed before, cleanly failed after), and
// fixed (cleanly failed before, passed after). "Failed" means a clean fail
// (ExitCode >= 0); an indeterminate result (timeout/launch error, ExitCode < 0) is
// deliberately NOT counted as a failure here, so a baseline that merely timed out
// is never reported as "already failing". The gate rolls indeterminate post kinds
// back, so this success-path summary never has to describe one.
func computeDelta(baseline, post verify.Validation) (alreadyFailing, newlyFailing, fixed []string) {
	basePassed := map[string]bool{}
	baseCleanFail := map[string]bool{}
	for _, c := range baseline.Commands {
		if !c.Ran {
			continue
		}
		switch {
		case c.Passed:
			basePassed[c.Kind] = true
		case c.ExitCode >= 0:
			baseCleanFail[c.Kind] = true
		}
	}
	for _, c := range post.Commands {
		if !c.Ran {
			continue
		}
		switch {
		case c.Passed:
			if baseCleanFail[c.Kind] {
				fixed = append(fixed, c.Kind)
			}
		case c.ExitCode >= 0: // clean fail
			if basePassed[c.Kind] {
				newlyFailing = append(newlyFailing, c.Kind)
			} else if baseCleanFail[c.Kind] {
				alreadyFailing = append(alreadyFailing, c.Kind)
			}
		}
	}
	return alreadyFailing, newlyFailing, fixed
}

// applyValidationNote produces a delta-aware summary for human output. It
// distinguishes pre-existing failures from new regressions so users are not
// misled by failures that existed before the change.
func applyValidationNote(baseline, post verify.Validation) string {
	if !post.Validated {
		return "No validation commands detected — the change could not be auto-verified."
	}
	already, newly, fixed := computeDelta(baseline, post)
	if len(newly) > 0 {
		// Should not reach here (gate() returns gateRegressed and the change is
		// rolled back before this note is rendered), but be honest if it does.
		return fmt.Sprintf("Regression detected: %s newly failed after this change.", strings.Join(newly, ", "))
	}
	if len(already) > 0 {
		note := fmt.Sprintf("The %s check(s) were already failing before this change — no previously-passing check regressed.", strings.Join(already, ", "))
		if len(fixed) > 0 {
			note += fmt.Sprintf(" This change fixed: %s.", strings.Join(fixed, ", "))
		}
		note += " A pre-existing failure can mask new breakage within the same kind; review `git diff` and test output before committing."
		return note
	}
	if len(fixed) > 0 {
		return fmt.Sprintf("Validation passed. This change fixed: %s.", strings.Join(fixed, ", "))
	}
	return "Validation passed after the change."
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
