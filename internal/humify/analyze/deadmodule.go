package analyze

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/schylerryan/humify/internal/humify/scan"
)

// deadModuleFindings nominates source modules that no other module imports and
// that play no entry-point role. Each is a CANDIDATE, never an assertion: the
// dead_module item is applyable as a reversible quarantine, so apply re-runs the
// project's validation commands and rolls the move back if a previously-passing
// check regresses. Detection is limited to JS/TS/Python in v1.
//
// The design fails toward over-sparing (a false negative — missing a dead file)
// rather than over-nominating (a false positive — flagging a live file), because
// every approximation here (lowercased base names, a global import set, the loose
// entry-corpus sweep) only ever adds reasons to spare a file.
func deadModuleFindings(res scan.Result, cfg Config) []Finding {
	imported := importedBasenames(res)
	live := liveModuleSet(res, cfg)
	var out []Finding
	for _, f := range res.Files {
		if !isDeadCandidate(f) || isEntryCorpus(f) {
			continue
		}
		base := moduleBase(f.Path)
		if imported[base] || live[base] || live[f.Path] {
			continue
		}
		out = append(out, Finding{
			Category: "maintainability", Signal: "dead_module", File: f.Path, Line: 1,
			Severity: "warning", Risk: "low",
			Evidence: "no import-shaped reference from any other module",
			Detail: "No other module imports this file and it is not a known entry point, so it may be dead. " +
				"`humify apply` quarantines it reversibly and re-runs your validation commands; if a previously-passing check now fails, the move is rolled back. " +
				"A use that validation never exercises — a dynamic import, a runtime-only entry — cannot be caught this way, so confirm or pin such files in humify.config.json \"liveModules\" first.",
		})
	}
	return out
}

// deadCandidateLangs are the languages dead-module detection supports in v1. Go is
// excluded by design: files in one Go package do not import each other, so an
// unreferenced-by-import heuristic would false-positive on nearly every file.
var deadCandidateLangs = map[string]bool{"js": true, "ts": true, "py": true}

// isDeadCandidate reports whether a file is even eligible to be nominated: a
// supported-language source file that is not a test, config, binary, or minified.
func isDeadCandidate(f scan.File) bool {
	return deadCandidateLangs[f.Lang] && !f.IsTest && !f.IsConfig && !f.Binary && !f.Minified
}

// moduleBase is a path's import identity: its base name without extension,
// lowercased so matching tolerates case drift. Distinct files that share a base
// name collapse together, which can only spare a file (a safe false negative).
func moduleBase(path string) string {
	b := filepath.Base(path)
	b = strings.TrimSuffix(b, filepath.Ext(b))
	return strings.ToLower(b)
}

// Import-shaped patterns. JS/TS: the quoted specifier in `from '…'`, in a
// `require('…')` / `import('…')` call, and in a side-effect `import '…'`. Python:
// the dotted module in `from … import` and `import …`. Backticks sit literally
// inside the negated quote class.
var (
	jsFromRe   = regexp.MustCompile("from\\s*[\"'`]([^\"'`]+)[\"'`]")
	jsCallRe   = regexp.MustCompile("(?:require|import)\\s*\\(\\s*[\"'`]([^\"'`]+)[\"'`]")
	jsBareRe   = regexp.MustCompile("import\\s+[\"'`]([^\"'`]+)[\"'`]")
	pyFromRe   = regexp.MustCompile(`(?m)^\s*from\s+([.\w]+)\s+import\b`)
	pyImportRe = regexp.MustCompile(`(?m)^\s*import\s+([\w.]+(?:\s*,\s*[\w.]+)*)`)
)

// importedBasenames collects the base name of every module imported anywhere in
// the repo. Test files count as importers — a module used only by tests is test
// support, not dead. Comments are stripped first so a commented-out import (itself
// dead) does not spare its target.
func importedBasenames(res scan.Result) map[string]bool {
	imported := map[string]bool{}
	for _, f := range res.Files {
		if !deadCandidateLangs[f.Lang] || f.Binary || f.Minified {
			continue
		}
		data, err := os.ReadFile(f.Abs)
		if err != nil {
			continue
		}
		text := stripComments(string(data), f.Lang)
		if f.Lang == "py" {
			collectPythonImports(imported, text)
			continue
		}
		for _, re := range []*regexp.Regexp{jsFromRe, jsCallRe, jsBareRe} {
			for _, m := range re.FindAllStringSubmatch(text, -1) {
				imported[moduleBase(m[1])] = true
			}
		}
	}
	return imported
}

// collectPythonImports adds each dotted-path segment of a file's imports to the
// set, so `from pkg.foo import x` spares both pkg and foo.
func collectPythonImports(imported map[string]bool, text string) {
	for _, m := range pyFromRe.FindAllStringSubmatch(text, -1) {
		addDottedSegments(imported, m[1])
	}
	for _, m := range pyImportRe.FindAllStringSubmatch(text, -1) {
		for _, part := range strings.Split(m[1], ",") {
			addDottedSegments(imported, part)
		}
	}
}

// addDottedSegments splits a dotted module path and records each lowercased,
// non-empty segment.
func addDottedSegments(set map[string]bool, dotted string) {
	for _, seg := range strings.Split(strings.TrimSpace(dotted), ".") {
		if seg = strings.TrimSpace(seg); seg != "" {
			set[strings.ToLower(seg)] = true
		}
	}
}

// conventionalEntries are base names that are almost always an entry point or
// package marker, not an importable leaf. Listing a name here can only spare a
// file, never cause a wrong nomination, so the list errs generous.
var conventionalEntries = []string{
	"index", "main", "app", "__main__", "__init__", "conftest", "setup", "manage",
}

// quotedStringRe captures the contents of any single/double/backtick-quoted
// string — used to harvest module references from manifests and build scripts.
var quotedStringRe = regexp.MustCompile("[\"'`]([^\"'`]+)[\"'`]")

// liveModuleSet collects every module known to be live by construction: pinned in
// config, a conventional entry, or named as a quoted path in an entry-corpus file
// (package.json, an HTML page, or a bundler/build script). This is how a module
// that no source imports — an esbuild entryPoint, an HTML <script src> — is spared.
func liveModuleSet(res scan.Result, cfg Config) map[string]bool {
	live := map[string]bool{}
	for _, name := range conventionalEntries {
		live[name] = true
	}
	for _, lm := range cfg.LiveModules {
		if lm = strings.TrimSpace(lm); lm == "" {
			continue
		}
		live[filepath.ToSlash(lm)] = true // exact repo-relative path
		live[moduleBase(lm)] = true       // bare base-name form
	}
	for _, f := range res.Files {
		if !isEntryCorpus(f) {
			continue
		}
		data, err := os.ReadFile(f.Abs)
		if err != nil {
			continue
		}
		for _, m := range quotedStringRe.FindAllStringSubmatch(string(data), -1) {
			live[moduleBase(m[1])] = true
		}
	}
	return live
}

// isEntryCorpus reports whether a file declares entry points or is itself run
// directly rather than imported: package.json, an HTML page, a tooling config
// (*.config.js — playwright, vite, jest, …), or a build script (build.js,
// build-web.js). Such files name the modules a project runs (so a module they
// mention is live with no source import), and are never themselves dead-module
// candidates.
func isEntryCorpus(f scan.File) bool {
	base := strings.ToLower(filepath.Base(f.Path))
	ext := strings.ToLower(filepath.Ext(f.Path))
	if base == "package.json" || ext == ".html" || ext == ".htm" {
		return true
	}
	if ext != ".js" && ext != ".mjs" && ext != ".cjs" && ext != ".ts" {
		return false
	}
	name := strings.TrimSuffix(base, ext) // base name without its extension
	switch {
	case strings.HasSuffix(name, ".config"): // playwright.config, vite.config, …
		return true
	case name == "build" || strings.HasPrefix(name, "build-") || strings.HasPrefix(name, "build."):
		return true
	}
	return false
}

// Comment patterns: a /* … */ block (across lines) and // or # line comments.
var (
	blockCommentRe  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	jsLineCommentRe = regexp.MustCompile(`//[^\n]*`)
	pyLineCommentRe = regexp.MustCompile(`#[^\n]*`)
)

// stripComments removes comments so commented-out imports do not register as real
// references. It is deliberately naive (no string-literal awareness); the residual
// risk is covered by apply's validation re-run.
func stripComments(text, lang string) string {
	if lang == "py" {
		return pyLineCommentRe.ReplaceAllString(text, "")
	}
	text = blockCommentRe.ReplaceAllString(text, "")
	return jsLineCommentRe.ReplaceAllString(text, "")
}
