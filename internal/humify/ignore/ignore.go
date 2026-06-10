// Package ignore decides which repository paths Humify skips while scanning.
//
// A Matcher combines three sources, in increasing specificity:
//
//  1. a built-in set of generated/vendor/build directory names that are never
//     worth analyzing (node_modules, dist, .git, ...),
//  2. patterns from the target repo's .gitignore, and
//  3. patterns from a .humifyignore file (Humify-specific overrides).
//
// The .gitignore support is a pragmatic subset — directory names, anchored and
// unanchored globs, and negation (!) — not the full Git spec. That is enough to
// keep generated and dependency code out of an analysis without pulling in a
// dependency, which is the goal: "respect .gitignore and .humifyignore where
// practical."
package ignore

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// DefaultDirs are directory names skipped regardless of any ignore file. They are
// dependency, build, cache, or VCS output — never the human-authored source a
// maintainability review is about. Humify's own output dirs are included so a
// re-run never analyzes its own reports.
var DefaultDirs = []string{
	".git", "node_modules", "dist", "build", ".next", ".nuxt", "coverage",
	"vendor", ".venv", "venv", "target", ".turbo", ".cache", ".parcel-cache",
	"out", ".tmp", "temp", "__pycache__",
	".humify", ".humify-dev", ".humify-runs", ".humify-worktrees",
}

// rule is one parsed ignore pattern.
type rule struct {
	pattern  string // normalized, slash-separated, no leading/trailing markers
	dirOnly  bool   // pattern ended in "/" — matches directories only
	anchored bool   // pattern began with "/" — matches from the repo root only
	negate   bool   // pattern began with "!" — re-includes a previously ignored path
}

// Matcher reports whether a repo-relative path should be skipped.
type Matcher struct {
	dirs  map[string]bool
	rules []rule
}

// New builds a Matcher for root: the default dirs plus any patterns found in
// root/.gitignore and root/.humifyignore. Missing ignore files are not an error.
func New(root string) *Matcher {
	m := &Matcher{dirs: map[string]bool{}}
	for _, d := range DefaultDirs {
		m.dirs[d] = true
	}
	for _, name := range []string{".gitignore", ".humifyignore"} {
		m.rules = append(m.rules, parseFile(filepath.Join(root, name))...)
	}
	return m
}

// parseFile reads ignore patterns from path, skipping blanks and comments. A
// missing file yields no rules.
func parseFile(path string) []rule {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var rules []rule
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if r, ok := parseLine(sc.Text()); ok {
			rules = append(rules, r)
		}
	}
	return rules
}

// parseLine turns one ignore-file line into a rule, reporting false for blanks
// and comments.
func parseLine(line string) (rule, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return rule{}, false
	}
	var r rule
	if strings.HasPrefix(line, "!") {
		r.negate = true
		line = line[1:]
	}
	if strings.HasPrefix(line, "/") {
		r.anchored = true
		line = strings.TrimPrefix(line, "/")
	}
	if strings.HasSuffix(line, "/") {
		r.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}
	if line == "" {
		return rule{}, false
	}
	r.pattern = filepath.ToSlash(line)
	return r, true
}

// Match reports whether the repo-relative path (slash-separated, e.g.
// "src/api/handler.go") should be skipped. isDir lets directory-only patterns and
// the default-dir set apply correctly. Matching is self-contained: a file under an
// ignored directory is reported ignored even if the caller did not prune the dir.
func (m *Matcher) Match(rel string, isDir bool) bool {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "" || rel == "." {
		return false
	}
	if m.defaultDirCovers(rel, isDir) {
		return true
	}
	// Later rules win, so negation can re-include a path a broader rule ignored.
	ignored := false
	for _, r := range m.rules {
		if r.covers(rel, isDir) {
			ignored = !r.negate
		}
	}
	return ignored
}

// defaultDirCovers reports whether rel is, or lives under, a default-ignored
// directory. A directory matches by its own base name; a file matches when any of
// its ancestor segments is a default-ignored dir.
func (m *Matcher) defaultDirCovers(rel string, isDir bool) bool {
	if isDir && m.dirs[path.Base(rel)] {
		return true
	}
	segs := strings.Split(rel, "/")
	for _, anc := range segs[:len(segs)-1] {
		if m.dirs[anc] {
			return true
		}
	}
	return false
}

// covers reports whether rel is, or lives under, a path this rule matches. A
// directory-only rule still covers files beneath the directory it matches.
func (r rule) covers(rel string, isDir bool) bool {
	if (!r.dirOnly || isDir) && r.matchPath(rel) {
		return true
	}
	// Any proper ancestor directory matching the pattern means rel is inside an
	// ignored directory. Ancestors are always directories, so dirOnly is satisfied.
	segs := strings.Split(rel, "/")
	for i := 1; i < len(segs); i++ {
		if r.matchPath(strings.Join(segs[:i], "/")) {
			return true
		}
	}
	return false
}

// matchPath reports whether the pattern matches path p exactly. An anchored
// pattern matches from the root; an unanchored pattern also matches p's base name
// and, for multi-segment patterns, any trailing sub-path.
func (r rule) matchPath(p string) bool {
	if r.anchored {
		return globMatch(r.pattern, p)
	}
	if globMatch(r.pattern, p) || globMatch(r.pattern, path.Base(p)) {
		return true
	}
	if strings.Contains(r.pattern, "/") {
		segs := strings.Split(p, "/")
		for i := range segs {
			if globMatch(r.pattern, strings.Join(segs[i:], "/")) {
				return true
			}
		}
	}
	return false
}

// globMatch applies shell-style matching, treating a pattern with no wildcard as
// an exact match. path.Match errors only on malformed patterns; those never match.
func globMatch(pattern, name string) bool {
	if !strings.ContainsAny(pattern, "*?[") {
		return pattern == name
	}
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}
