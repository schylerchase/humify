// Package exec holds the execute stage's deterministic helpers: which wave to
// run next, the worktree manifest (fan-out source of truth for the merge
// barrier), the commit log (what each merge committed, so `humify undo` can
// revert it), the verifier verdict schema, and the executor/verifier prompts.
//
// The orchestration itself lives in the command; this package keeps the
// decidable parts pure and testable, away from real git I/O.
package exec

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"

	"humify-ng/internal/layout"
	"humify-ng/internal/worktree"
)

// manifestPath is the fan-out source of truth: the worktree entries forked for
// the wave currently in flight. The merge barrier fails closed if it is missing.
func manifestPath(root string) string {
	return filepath.Join(layout.TmpDir(root), "EXEC_MANIFEST.json")
}

// SaveManifest persists the current wave's forked worktree entries.
func SaveManifest(root string, entries []worktree.Entry) error {
	if err := os.MkdirAll(layout.TmpDir(root), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath(root), b, 0o644)
}

// LoadManifest reads the current wave's forked entries (nil if none forked yet).
func LoadManifest(root string) ([]worktree.Entry, error) {
	b, err := os.ReadFile(manifestPath(root))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var entries []worktree.Entry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// ClearManifest removes the wave manifest once its entries have all merged.
func ClearManifest(root string) error {
	err := os.Remove(manifestPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// CommitRecord is one merged slice and the merge commit it produced. The log is
// append-only and ordered, so undo reverts in reverse (newest first).
type CommitRecord struct {
	SliceID   string `json:"slice_id"`
	Wave      int    `json:"wave"`
	CommitSHA string `json:"commit_sha"`
}

// commitLogPath is the durable rollback record: every merge commit execute made.
func commitLogPath(root string) string {
	return filepath.Join(layout.HumifyDir(root), "manifest.json")
}

// AppendCommits adds merge records to the durable commit log.
func AppendCommits(root string, records []CommitRecord) error {
	existing, err := LoadCommits(root)
	if err != nil {
		return err
	}
	all := append(existing, records...)
	b, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(layout.HumifyDir(root), 0o755); err != nil {
		return err
	}
	return os.WriteFile(commitLogPath(root), b, 0o644)
}

// ClearCommits removes the durable commit log (a no-op if absent). Called after
// a fully successful undo so a second undo does not double-revert.
func ClearCommits(root string) error {
	err := os.Remove(commitLogPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// LoadCommits reads the durable commit log (nil if none yet).
func LoadCommits(root string) ([]CommitRecord, error) {
	b, err := os.ReadFile(commitLogPath(root))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var recs []CommitRecord
	if err := json.Unmarshal(b, &recs); err != nil {
		return nil, err
	}
	return recs, nil
}

// CurrentWave returns the lowest-index wave that still has planned-but-not-yet-
// executed slices, that wave's remaining slice ids (sorted), and done=true when
// no wave has remaining work. Returning only one wave's slices enforces the
// barrier: a wave must fully execute before the next forks.
func CurrentWave(waves [][]string, planned, executed map[string]bool) (idx int, slices []string, done bool) {
	for i, w := range waves {
		var rem []string
		for _, id := range w {
			if planned[id] && !executed[id] {
				rem = append(rem, id)
			}
		}
		if len(rem) > 0 {
			sort.Strings(rem)
			return i, rem, false
		}
	}
	return 0, nil, true
}
