// Package intel is the persisted decomposition: the machine-readable record of
// what `humify heatmap` computed about a target codebase (areas, dependency
// edges, parallel waves, cycles, risk scores). It is written once by heatmap
// and read back by every later stage that needs to know an area's files, the
// target root, or the wave order — so the decomposition has a single on-disk
// definition rather than each stage re-scanning or re-deriving it.
package intel

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"humify/internal/area"
	"humify/internal/graph"
	"humify/internal/heatmap"
	"humify/internal/layout"
)

// ErrNotExist is returned by Load when no intel has been written yet.
var ErrNotExist = errors.New("no intel/areas.json (run `humify heatmap` first)")

// Data is the full decomposition record persisted to .humify/intel/areas.json.
type Data struct {
	Target string          `json:"target"`
	Files  int             `json:"source_files"`
	Areas  []area.Area     `json:"areas"`
	Edges  []graph.Edge    `json:"edges"`
	Waves  [][]string      `json:"waves"`
	Cycles [][]string      `json:"cycles"`
	Scores []heatmap.Score `json:"scores"`
}

// File returns the intel path under root.
func File(root string) string {
	return filepath.Join(layout.HumifyDir(root), "intel", "areas.json")
}

// Write persists the decomposition, creating .humify/intel if needed.
func Write(root string, d Data) error {
	if err := os.MkdirAll(filepath.Dir(File(root)), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(File(root), b, 0o644)
}

// Load reads the decomposition, mapping a missing file to ErrNotExist so callers
// can fail closed with a clear message.
func Load(root string) (Data, error) {
	var d Data
	b, err := os.ReadFile(File(root))
	if err != nil {
		if os.IsNotExist(err) {
			return d, ErrNotExist
		}
		return d, err
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return d, err
	}
	return d, nil
}

// AreasByID indexes the decomposition's areas by id for O(1) lookup.
func (d Data) AreasByID() map[string]area.Area {
	m := make(map[string]area.Area, len(d.Areas))
	for _, a := range d.Areas {
		m[a.ID] = a
	}
	return m
}

// WaveOf maps each area id to its topo wave level.
func (d Data) WaveOf() map[string]int {
	m := map[string]int{}
	for level, ids := range d.Waves {
		for _, id := range ids {
			m[id] = level
		}
	}
	return m
}
