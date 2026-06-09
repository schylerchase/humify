// Package scan walks a target repository into a flat list of files Humify can
// reason about. It applies the ignore rules (pruning ignored directories so they
// are never descended), classifies each file by language and role (source, test,
// config), and records line counts. It deliberately does NOT score anything — that
// is the analyze package's job. Keeping the walk separate keeps it cheap to test.
package scan

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"humify-ng/internal/humify/ignore"
)

// maxReadBytes caps how much of a file scan reads to count lines and sniff for
// binary content. Source files are far smaller; the cap only guards against a
// pathological generated blob slipping past the ignore rules.
const maxReadBytes = 5 << 20 // 5 MiB

// File is one scanned file with the facts later stages need.
type File struct {
	Path     string `json:"path"`     // repo-relative, slash-separated
	Abs      string `json:"-"`        // absolute path, for re-reading content
	Lang     string `json:"lang"`     // "go", "ts", "py", ... or "" if unknown
	LOC      int    `json:"loc"`      // newline count (0 for binary)
	Size     int64  `json:"size"`     // bytes on disk
	IsTest   bool   `json:"is_test"`  // looks like a test file
	IsConfig bool   `json:"is_config"`// looks like configuration
	Binary   bool   `json:"binary"`   // non-text content; excluded from source metrics
	Minified bool   `json:"minified"` // machine-minified/bundled; excluded from review
}

// Result is the outcome of a walk.
type Result struct {
	Root  string
	Files []File
}

// Walk scans root, skipping anything the matcher ignores. A nil matcher uses the
// default ignore set for root.
func Walk(root string, m *ignore.Matcher) (Result, error) {
	if m == nil {
		m = ignore.New(root)
	}
	res := Result{Root: root}
	err := filepath.WalkDir(root, func(abs string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, abs)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if m.Match(rel, true) {
				return filepath.SkipDir // prune the whole subtree
			}
			return nil
		}
		if m.Match(rel, false) || !d.Type().IsRegular() {
			return nil
		}
		res.Files = append(res.Files, classify(root, rel))
		return nil
	})
	return res, err
}

// classify reads a file's size, line count, and binary flag, and derives its
// language and role from its name and content.
func classify(root, rel string) File {
	abs := filepath.Join(root, rel)
	f := File{Path: rel, Abs: abs, Lang: langOf(rel)}
	if info, err := os.Stat(abs); err == nil {
		f.Size = info.Size()
	}
	if data, err := readCapped(abs); err == nil {
		f.Binary = isBinary(data)
		if !f.Binary {
			f.LOC = countLines(data)
		}
	}
	f.IsTest = isTestPath(rel)
	f.IsConfig = isConfigPath(rel)
	f.Minified = !f.Binary && looksMinified(rel, f.Size, f.LOC)
	return f
}

// minBytesPerLine is the average line length above which a file is treated as
// machine-minified — hand-written code essentially never averages this many bytes
// per line, but a minified bundle (one statement per megaline) far exceeds it.
const minBytesPerLine = 400

// looksMinified reports whether a file is machine-minified or bundled third-party
// code: by a conventional name (*.min.*, *.bundle.js, *-min.js) or an average
// bytes-per-line far beyond hand-written source. Such files are generated noise and
// must not be reviewed or scored.
func looksMinified(rel string, size int64, loc int) bool {
	base := strings.ToLower(filepath.Base(rel))
	if strings.Contains(base, ".min.") || strings.HasSuffix(base, ".bundle.js") || strings.HasSuffix(base, "-min.js") {
		return true
	}
	return loc > 0 && size/int64(loc) >= minBytesPerLine
}

// readCapped reads up to maxReadBytes of a file.
func readCapped(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	buf := make([]byte, maxReadBytes)
	n, err := file.Read(buf)
	if err != nil && n == 0 {
		return nil, err
	}
	return buf[:n], nil
}

// isBinary reports whether data looks non-textual. A NUL byte is the cheap,
// reliable signal used by Git and grep alike.
func isBinary(data []byte) bool {
	return bytes.IndexByte(data, 0) >= 0
}

// countLines counts lines, counting a final unterminated line.
func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		n++
	}
	return n
}

// langExt maps a file extension to a short language id.
var langExt = map[string]string{
	".go": "go", ".js": "js", ".jsx": "js", ".mjs": "js", ".cjs": "js",
	".ts": "ts", ".tsx": "ts", ".py": "py", ".rb": "rb", ".java": "java",
	".rs": "rs", ".c": "c", ".h": "c", ".cc": "cpp", ".cpp": "cpp", ".hpp": "cpp",
	".cs": "cs", ".php": "php", ".sh": "sh", ".bash": "sh", ".ps1": "ps1",
	".kt": "kotlin", ".swift": "swift", ".scala": "scala", ".m": "objc",
}

// langOf returns the language id for a path, or "" if unknown.
func langOf(rel string) string {
	return langExt[strings.ToLower(filepath.Ext(rel))]
}

// isTestPath reports whether a path looks like a test, by directory or name.
func isTestPath(rel string) bool {
	lower := strings.ToLower(rel)
	base := filepath.Base(lower)
	for _, dir := range []string{"/test/", "/tests/", "/__tests__/", "/spec/"} {
		if strings.Contains("/"+lower, dir) {
			return true
		}
	}
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return true
	case strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"):
		return true
	case strings.HasSuffix(base, "_test.py"):
		return true
	}
	for _, infix := range []string{".test.", ".spec."} {
		if strings.Contains(base, infix) {
			return true
		}
	}
	return false
}

// configNames are common configuration files recognized regardless of extension.
var configNames = map[string]bool{
	"package.json": true, "tsconfig.json": true, "go.mod": true, "go.sum": true,
	"cargo.toml": true, "pyproject.toml": true, "requirements.txt": true,
	"dockerfile": true, "makefile": true, ".eslintrc": true, ".prettierrc": true,
	"gemfile": true, "pom.xml": true, "build.gradle": true,
}

// configExts are extensions that are almost always configuration, not source.
var configExts = map[string]bool{
	".json": true, ".yml": true, ".yaml": true, ".toml": true, ".ini": true,
	".cfg": true, ".conf": true, ".env": true, ".lock": true,
}

// isConfigPath reports whether a path looks like configuration rather than source.
func isConfigPath(rel string) bool {
	base := strings.ToLower(filepath.Base(rel))
	if configNames[base] {
		return true
	}
	return configExts[strings.ToLower(filepath.Ext(rel))]
}
