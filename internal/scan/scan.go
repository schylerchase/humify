// Package scan walks a target codebase and produces per-file facts used by
// decomposition and risk scoring: LOC, a cheap complexity proxy (branch
// keyword count), test-file detection, and best-effort import extraction.
// Generated/vendored/build output is excluded so it never receives findings.
package scan

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// File is one scanned source file, with path relative to the scan root.
type File struct {
	Rel      string `json:"path"`
	LOC      int    `json:"loc"`
	Branches int    `json:"branches"`
	IsTest   bool   `json:"is_test"`
}

var skipDirs = map[string]bool{
	"node_modules": true, ".git": true, "dist": true, "build": true,
	"vendor": true, ".humify": true, "testdata": true, "coverage": true,
	".next": true, "out": true, "target": true, "bin": true, "obj": true,
	".venv": true, "__pycache__": true, ".idea": true, ".vscode": true,
}

var sourceExt = map[string]bool{
	".go": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true,
	".py": true, ".rs": true, ".java": true, ".c": true, ".cc": true,
	".cpp": true, ".h": true, ".hpp": true, ".cs": true, ".rb": true,
	".php": true, ".swift": true, ".kt": true, ".scala": true, ".sh": true,
	".ps1": true, ".lua": true, ".vue": true, ".svelte": true, ".mjs": true,
	".cjs": true,
}

var (
	branchRe = regexp.MustCompile(`\b(if|for|while|case|catch|switch)\b|&&|\|\||\?`)
	importRe = regexp.MustCompile(`(?m)(?:import\s+[^'"]*['"]([^'"]+)['"])|(?:require\(\s*['"]([^'"]+)['"]\s*\))|(?:from\s+([^\s]+)\s+import)|(?:#include\s+["<]([^">]+)[">])|(?:import\s+"([^"]+)")`)
)

// WalkSource walks root and returns facts for every source file, skipping
// generated/vendored directories, minified bundles, and non-source files.
func WalkSource(root string) ([]File, error) {
	var files []File
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // tolerate unreadable entries
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSource(d.Name()) {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		files = append(files, fileFacts(path, filepath.ToSlash(rel)))
		return nil
	})
	return files, err
}

func isSource(name string) bool {
	if strings.Contains(name, ".min.") {
		return false
	}
	return sourceExt[strings.ToLower(filepath.Ext(name))]
}

func fileFacts(abs, rel string) File {
	f := File{Rel: rel, IsTest: isTestPath(rel)}
	b, err := os.ReadFile(abs)
	if err != nil {
		return f
	}
	f.LOC = strings.Count(string(b), "\n") + 1
	f.Branches = len(branchRe.FindAllIndex(b, -1))
	return f
}

func isTestPath(rel string) bool {
	lower := strings.ToLower(rel)
	if strings.Contains(lower, "_test.") || strings.Contains(lower, ".test.") ||
		strings.Contains(lower, ".spec.") {
		return true
	}
	for _, seg := range strings.Split(lower, "/") {
		if seg == "test" || seg == "tests" || seg == "__tests__" {
			return true
		}
	}
	return false
}

// ImportsOf returns the raw import target strings referenced by a file.
// Best-effort across JS/TS, Python, Go, and C-style includes.
func ImportsOf(abs string) []string {
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil
	}
	var out []string
	for _, m := range importRe.FindAllStringSubmatch(string(b), -1) {
		for _, g := range m[1:] {
			if g != "" {
				out = append(out, g)
			}
		}
	}
	return out
}
