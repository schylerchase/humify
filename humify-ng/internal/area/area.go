// Package area decomposes scanned files into "areas" — the unit a single
// auditor agent will later own. Two rules: any file at or above the god-file
// LOC threshold becomes its own area (so a 19k-line app-core.js is isolated as
// the hotspot it is), and everything else is grouped by top-level directory.
package area

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"humify-ng/internal/scan"
)

// Area is one decomposed unit of the target codebase.
type Area struct {
	ID         string      `json:"id"`
	Slug       string      `json:"slug"`
	Kind       string      `json:"kind"` // "dir" or "file"
	Root       string      `json:"root"` // path prefix this area owns
	Files      []scan.File `json:"-"`
	FilePaths  []string    `json:"files"` // exact files this area owns; persisted so the auditor reads precisely its slice, never a sibling area's god-file
	FileCount  int         `json:"file_count"`
	LOC        int         `json:"loc"`
	MaxFileLOC int         `json:"max_file_loc"`
	Branches   int         `json:"branches"`
	HasTests   bool        `json:"has_tests"`
}

// Decompose groups files into areas. godLOC is the single-file size at or
// above which a file is isolated into its own area.
func Decompose(files []scan.File, godLOC int) []Area {
	groups := map[string]*Area{}
	var order []string
	for _, f := range files {
		key, a := bucket(f, godLOC)
		cur := groups[key]
		if cur == nil {
			groups[key] = a
			order = append(order, key)
			cur = a
		}
		addFile(cur, f)
	}
	areas := make([]Area, 0, len(order))
	for _, k := range order {
		areas = append(areas, *groups[k])
	}
	return assignIDs(areas)
}

// bucket returns the group key and a fresh Area shell for a file.
func bucket(f scan.File, godLOC int) (string, *Area) {
	if f.LOC >= godLOC {
		return "file:" + f.Rel, &Area{Kind: "file", Root: f.Rel, Slug: slugify(stripExt(f.Rel))}
	}
	top := topDir(f.Rel)
	return "dir:" + top, &Area{Kind: "dir", Root: top, Slug: slugify(top)}
}

func addFile(a *Area, f scan.File) {
	a.Files = append(a.Files, f)
	a.FilePaths = append(a.FilePaths, f.Rel)
	a.FileCount++
	a.LOC += f.LOC
	a.Branches += f.Branches
	if f.LOC > a.MaxFileLOC {
		a.MaxFileLOC = f.LOC
	}
	if f.IsTest {
		a.HasTests = true
	}
}

func assignIDs(areas []Area) []Area {
	sort.SliceStable(areas, func(i, j int) bool {
		if areas[i].Slug != areas[j].Slug {
			return areas[i].Slug < areas[j].Slug
		}
		return areas[i].Root < areas[j].Root
	})
	width := 2
	if n := len(strconv.Itoa(len(areas))); n > width {
		width = n
	}
	seen := map[string]int{}
	for i := range areas {
		base := areas[i].Slug
		slug := base
		if c := seen[base]; c > 0 {
			slug = base + "-" + strconv.Itoa(c+1)
		}
		seen[base]++
		areas[i].Slug = slug
		areas[i].ID = fmt.Sprintf("%0*d-%s", width, i+1, slug)
	}
	return areas
}

func topDir(rel string) string {
	if idx := strings.Index(rel, "/"); idx >= 0 {
		return rel[:idx]
	}
	return "(root)"
}

func stripExt(p string) string { return strings.TrimSuffix(p, path.Ext(p)) }

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = nonSlug.ReplaceAllString(strings.ToLower(s), "-")
	if s = strings.Trim(s, "-"); s == "" {
		return "area"
	}
	return s
}
