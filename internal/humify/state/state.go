// Package state owns the on-disk contract for Humify's machine-readable output:
// the .humify/ directory and atomic JSON load/save. JSON is Humify's control
// plane — every command reads and writes through here, so the schema lives in one
// place. The markdown reports are a separate, optional rendering of this data.
package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	// Dir is the per-target directory holding Humify's JSON state and quarantine.
	Dir = ".humify"
	// Schema is bumped when a JSON shape changes incompatibly, so a stale file can
	// be detected rather than misread.
	Schema = 1

	AnalysisFile   = "analysis.json"
	PlanFile       = "plan.json"
	ValidationFile = "validation.json"
	// DeleteMeDir holds files apply has quarantined, namespaced per plan item.
	DeleteMeDir = "delete-me"
)

// dirPath returns <root>/.humify.
func dirPath(root string) string { return filepath.Join(root, Dir) }

// Path returns the absolute path of a Humify state file under root.
func Path(root, name string) string { return filepath.Join(dirPath(root), name) }

// QuarantineDir returns the quarantine directory for a plan item under root.
func QuarantineDir(root, planID string) string {
	return filepath.Join(dirPath(root), DeleteMeDir, planID)
}

// Save writes v as indented JSON to <root>/.humify/<name>, creating .humify if
// needed. The write is atomic (temp file + rename) so a crash never leaves a
// half-written state file that a later command would misparse.
func Save(root, name string, v any) error {
	if err := os.MkdirAll(dirPath(root), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	final := Path(root, name)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, final)
}

// Load reads <root>/.humify/<name> into v. It reports os.ErrNotExist (wrapped)
// when the file is absent so callers can distinguish "never analyzed" from a real
// read/parse error.
func Load(root, name string, v any) error {
	data, err := os.ReadFile(Path(root, name))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// Exists reports whether a Humify state file is present.
func Exists(root, name string) bool {
	_, err := os.Stat(Path(root, name))
	return err == nil
}
