// Package detect derives project facts from a scan: the language stack, the
// package manager, declared scripts, likely entry points, configuration files,
// the largest source files, and headline counts. Everything is inferred from
// files already on disk — detect never runs anything, so it is safe on any repo.
package detect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"humify-ng/internal/humify/scan"
)

// FileRef names a file with its line count, for "largest files" lists.
type FileRef struct {
	Path string `json:"path"`
	LOC  int    `json:"loc"`
}

// Counts are the headline file tallies.
type Counts struct {
	Files  int `json:"files"`
	Source int `json:"source"`
	Test   int `json:"test"`
	Config int `json:"config"`
}

// Project is the detected shape of a repository.
type Project struct {
	Stack          []string          `json:"stack"`
	PackageManager string            `json:"package_manager"`
	Scripts        map[string]string `json:"scripts"`
	EntryPoints    []string          `json:"entry_points"`
	Configs        []string          `json:"configs"`
	LargestFiles   []FileRef         `json:"largest_files"`
	Counts         Counts            `json:"counts"`
}

// maxLargest bounds the largest-files list so a report stays readable.
const maxLargest = 10

// Detect inspects a scanned repo rooted at root.
func Detect(res scan.Result, root string) Project {
	p := Project{
		Stack:          stackOf(res.Files),
		PackageManager: packageManager(root),
		Scripts:        scriptsOf(root),
		Configs:        configsOf(res.Files),
		LargestFiles:   largestOf(res.Files),
		Counts:         countsOf(res.Files),
	}
	p.EntryPoints = entryPoints(res.Files, root)
	return p
}

// stackOf lists languages present in source files, most common first.
func stackOf(files []scan.File) []string {
	count := map[string]int{}
	for _, f := range files {
		if isSource(f) {
			count[f.Lang]++
		}
	}
	langs := make([]string, 0, len(count))
	for l := range count {
		langs = append(langs, l)
	}
	sort.Slice(langs, func(i, j int) bool {
		if count[langs[i]] != count[langs[j]] {
			return count[langs[i]] > count[langs[j]]
		}
		return langs[i] < langs[j]
	})
	return langs
}

// pmSignal maps a manifest/lockfile to the package manager it implies, in
// priority order (the first present wins).
var pmSignals = []struct{ file, manager string }{
	{"pnpm-lock.yaml", "pnpm"},
	{"yarn.lock", "yarn"},
	{"package-lock.json", "npm"},
	{"go.mod", "go modules"},
	{"Cargo.toml", "cargo"},
	{"poetry.lock", "poetry"},
	{"requirements.txt", "pip"},
	{"Gemfile", "bundler"},
	{"pom.xml", "maven"},
	{"build.gradle", "gradle"},
	{"package.json", "npm"}, // package.json without a lockfile → npm by default
}

// packageManager returns the strongest package-manager signal at the repo root.
func packageManager(root string) string {
	for _, s := range pmSignals {
		if exists(filepath.Join(root, s.file)) {
			return s.manager
		}
	}
	return "unknown"
}

// scriptsOf collects declared scripts from package.json and Makefile targets.
func scriptsOf(root string) map[string]string {
	scripts := map[string]string{}
	if pkg, ok := readPackageJSON(root); ok {
		for name, cmd := range pkg.Scripts {
			scripts[name] = cmd
		}
	}
	for _, target := range makefileTargets(root) {
		if _, taken := scripts[target]; !taken {
			scripts[target] = "make " + target
		}
	}
	if len(scripts) == 0 {
		return nil
	}
	return scripts
}

// packageJSON is the subset of package.json detect reads.
type packageJSON struct {
	Main    string            `json:"main"`
	Module  string            `json:"module"`
	Bin     json.RawMessage   `json:"bin"`
	Scripts map[string]string `json:"scripts"`
}

// readPackageJSON parses root/package.json, reporting false if absent or invalid.
func readPackageJSON(root string) (packageJSON, bool) {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return packageJSON{}, false
	}
	var pkg packageJSON
	if json.Unmarshal(data, &pkg) != nil {
		return packageJSON{}, false
	}
	return pkg, true
}

// makefileTargets returns the names of phony-style targets in a root Makefile.
func makefileTargets(root string) []string {
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		return nil
	}
	var targets []string
	for _, line := range strings.Split(string(data), "\n") {
		name, rest, found := strings.Cut(line, ":")
		if !found || strings.HasPrefix(line, "\t") || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, " =/") || strings.HasPrefix(rest, "=") {
			continue
		}
		targets = append(targets, name)
	}
	return targets
}

// entryPoints returns likely program entry points that exist in the scan.
func entryPoints(files []scan.File, root string) []string {
	present := map[string]bool{}
	for _, f := range files {
		present[f.Path] = true
	}
	var points []string
	add := func(rel string) {
		if present[rel] {
			points = append(points, rel)
		}
	}
	for _, f := range files {
		base := filepath.Base(f.Path)
		if base == "main.go" || base == "__main__.py" || base == "manage.py" {
			points = append(points, f.Path)
		}
	}
	for _, rel := range []string{"index.js", "index.ts", "src/index.js", "src/index.ts", "src/main.ts", "src/main.js", "app.py", "main.py"} {
		add(rel)
	}
	if pkg, ok := readPackageJSON(root); ok {
		for _, m := range []string{pkg.Main, pkg.Module} {
			if m != "" && present[filepath.ToSlash(filepath.Clean(m))] {
				points = append(points, filepath.ToSlash(filepath.Clean(m)))
			}
		}
	}
	return dedupe(points)
}

// configsOf lists configuration file paths, sorted.
func configsOf(files []scan.File) []string {
	var cfgs []string
	for _, f := range files {
		if f.IsConfig {
			cfgs = append(cfgs, f.Path)
		}
	}
	sort.Strings(cfgs)
	return cfgs
}

// largestOf returns the largest source files by line count.
func largestOf(files []scan.File) []FileRef {
	var refs []FileRef
	for _, f := range files {
		if isSource(f) {
			refs = append(refs, FileRef{Path: f.Path, LOC: f.LOC})
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].LOC != refs[j].LOC {
			return refs[i].LOC > refs[j].LOC
		}
		return refs[i].Path < refs[j].Path
	})
	if len(refs) > maxLargest {
		refs = refs[:maxLargest]
	}
	return refs
}

// countsOf tallies files by role.
func countsOf(files []scan.File) Counts {
	c := Counts{Files: len(files)}
	for _, f := range files {
		switch {
		case f.IsConfig:
			c.Config++
		case f.IsTest:
			c.Test++
		case isSource(f):
			c.Source++
		}
	}
	return c
}

// isSource reports whether a file is human-authored program source (not a test,
// config, or binary, and in a recognized language).
func isSource(f scan.File) bool {
	return !f.IsConfig && !f.IsTest && !f.Binary && f.Lang != ""
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
