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
	"fmt"
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

// ConflictsFile returns the path to .humify/CONFLICTS.md.
func ConflictsFile(root string) string { return filepath.Join(root, Dir, "CONFLICTS.md") }

// TmpDir returns .humify/tmp, home of transient fan-in state (the manifest).
func TmpDir(root string) string { return filepath.Join(root, Dir, "tmp") }

// AreaFragmentRel returns the audit-fragment path for an area id, relative to
// the project root. This is the single definition of the fragment filename
// pattern; the manifest stores this relative form (so it is portable) and
// AreaFragment joins it to a root.
func AreaFragmentRel(areaID string) string {
	return filepath.Join(Dir, "areas", areaID, areaID+"-AUDIT-fragment.json")
}

// AreaFragment returns the absolute audit-fragment path for an area id.
func AreaFragment(root, areaID string) string {
	return filepath.Join(root, AreaFragmentRel(areaID))
}

// ResolveInRoot resolves a project-relative path against root, rejecting any
// path that is empty, absolute, carries a volume, or escapes the root via "..".
// The volume check stops Windows drive-relative escapes ("C:..\..\x") that are
// neither absolute nor ".."-prefixed yet still resolve above root. It is the one
// gate every stage uses before opening a path that came from a manifest.
func ResolveInRoot(root, relPath string) (string, error) {
	native := filepath.FromSlash(relPath)
	if relPath == "" || filepath.IsAbs(native) || filepath.VolumeName(native) != "" {
		return "", fmt.Errorf("path must be relative and root-local, got %q", relPath)
	}
	full := filepath.Join(root, filepath.Clean(native))
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes project root: %q", relPath)
	}
	return full, nil
}

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
