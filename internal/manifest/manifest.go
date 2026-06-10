// Package manifest is the fan-in source of truth: the set of audit fragments
// the consolidate stage expects. It is written at fan-out time (today by
// heatmap; later by the audit dispatcher) and read by consolidate, which
// fails closed if it is missing or empty rather than producing a partial AUDIT.
package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"

	"humify/internal/layout"
)

// Entry names one expected fragment.
type Entry struct {
	AreaID string `json:"area_id"`
	Path   string `json:"path"` // fragment path, relative to project root
}

// Manifest is the full expected-fragment set.
type Manifest struct {
	Fragments []Entry `json:"fragments"`
}

// File returns the manifest path under root.
func File(root string) string {
	return filepath.Join(layout.TmpDir(root), "AUDIT_MANIFEST.json")
}

// Load reads the manifest. A missing file returns os.ErrNotExist so callers can
// fail closed.
func Load(root string) (Manifest, error) {
	var m Manifest
	b, err := os.ReadFile(File(root))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(b, &m)
	return m, err
}

// Write persists the manifest, creating .humify/tmp if needed.
func Write(root string, m Manifest) error {
	if err := os.MkdirAll(layout.TmpDir(root), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(File(root), b, 0o644)
}
