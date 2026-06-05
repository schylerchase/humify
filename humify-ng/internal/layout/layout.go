// Package layout defines the on-disk .humify/ contract and discovers areas.
//
// The .humify/ tree is the single source of truth. Status is never stored;
// every command re-derives it by scanning this layout (see package state).
//
//	.humify/
//	  AUDIT.md        consolidated audit (the synthesizer's output)
//	  PATCHLOG.md     roll-up of executed slices
//	  areas/
//	    NN-<slug>/    one directory per codebase area
//	      NN-MAP.md
//	      NN-AUDIT-fragment.json
//	      NN-MM-PLAN.md / NN-MM-SUMMARY.md
package layout

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Dir is the name of the humify state directory.
const Dir = ".humify"

// FindRoot walks up from start until it finds a directory containing .humify/.
// It returns the project root (the directory that holds .humify) and whether
// one was found.
func FindRoot(start string) (string, bool) {
	cur, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		if isDir(filepath.Join(cur, Dir)) {
			return cur, true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false
		}
		cur = parent
	}
}

// HumifyDir returns the path to the .humify directory under root.
func HumifyDir(root string) string { return filepath.Join(root, Dir) }

// AreasDir returns the path to .humify/areas under root.
func AreasDir(root string) string { return filepath.Join(root, Dir, "areas") }

// AuditFile returns the path to the consolidated .humify/AUDIT.md.
func AuditFile(root string) string { return filepath.Join(root, Dir, "AUDIT.md") }

// PatchlogFile returns the path to .humify/PATCHLOG.md.
func PatchlogFile(root string) string { return filepath.Join(root, Dir, "PATCHLOG.md") }

// DiscoverAreas returns the area directory names under .humify/areas, sorted.
// A missing areas directory is not an error; it yields no areas.
func DiscoverAreas(root string) ([]string, error) {
	entries, err := os.ReadDir(AreasDir(root))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var areas []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			areas = append(areas, e.Name())
		}
	}
	sort.Strings(areas)
	return areas, nil
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
