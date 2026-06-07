// Package handoff is the one-shot resume cursor a dispatching command leaves on
// disk so `humify resume` can name the next step and, when the cursor still
// agrees with the disk-derived truth, the exact prompts to spawn.
//
// It is deliberately a CONVENIENCE layer, never a source of truth. The .humify/
// topology is authoritative (see package pipeline); resume always derives the
// next step from disk and only reconciles this cursor against it. A cursor that
// is absent — or stale, because the agents a command dispatched have since
// advanced the disk — never makes resume wrong: disk wins. Stored under tmp/ so
// it is per-run scratch and never travels in version control.
package handoff

import (
	"encoding/json"
	"errors"
	"os"

	"humify-ng/internal/layout"
)

// Handoff records what a command just did and what comes next. It is structural,
// not prose: resume reconciles Stage/NextCommand against disk and attaches
// Prompts only when the cursor and disk still agree.
type Handoff struct {
	Stage       string   `json:"stage"`             // stage that just ran: heatmap|audit|consolidate|plan|execute|patchlog
	Action      string   `json:"action"`            // "spawn" | "proceed" | "blocked"
	NextCommand string   `json:"next_command"`      // e.g. "humify consolidate"
	Prompts     []string `json:"prompts,omitempty"` // root-relative prompt paths to spawn, when Action=="spawn"
	Note        string   `json:"note,omitempty"`
}

// Save atomically writes the cursor (write-temp-then-rename) so a crash mid-write
// never leaves a half-parsed HANDOFF.json. A best-effort convenience cursor must
// never become a corruption that breaks resume.
func Save(root string, h Handoff) error {
	if err := os.MkdirAll(layout.TmpDir(root), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	final := layout.HandoffFile(root)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// Load reads the cursor. A missing file is found=false with no error: its absence
// is the normal disk-derived path, not a failure.
func Load(root string) (h Handoff, found bool, err error) {
	b, err := os.ReadFile(layout.HandoffFile(root))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Handoff{}, false, nil
		}
		return Handoff{}, false, err
	}
	if err := json.Unmarshal(b, &h); err != nil {
		return Handoff{}, false, err
	}
	return h, true, nil
}

// Consume reads then deletes the cursor (one-shot). The delete is best-effort
// once the value is read: a failed remove must not lose the answer the caller
// already holds, and disk-first reconciliation makes a lingering stale cursor
// harmless on the next call.
func Consume(root string) (Handoff, bool, error) {
	h, found, err := Load(root)
	if err != nil || !found {
		return h, found, err
	}
	_ = os.Remove(layout.HandoffFile(root))
	return h, true, nil
}
