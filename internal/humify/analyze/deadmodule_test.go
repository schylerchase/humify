package analyze

import "testing"

// deadSet runs a full analysis and returns the set of file paths flagged as
// dead_module candidates. Tests assert on this set rather than the raw findings
// so they read as "which files were nominated".
func deadSet(t *testing.T, root string, cfg Config) map[string]bool {
	t.Helper()
	a, err := Run(root, cfg)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	out := map[string]bool{}
	for _, f := range a.Findings {
		if f.Signal == "dead_module" {
			out[f.File] = true
		}
	}
	return out
}

// TestDeadModuleNominatesUnreferenced is the core case: a source file imported by
// nobody and not an entry point is a candidate; an imported file and a
// conventional entry are not.
func TestDeadModuleNominatesUnreferenced(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src/index.js", "import './used'\n") // conventional entry
	writeFile(t, root, "src/used.js", "export const x = 1\n")
	writeFile(t, root, "src/orphan.js", "export const y = 2\n")

	dead := deadSet(t, root, Defaults())
	if !dead["src/orphan.js"] {
		t.Errorf("orphan.js has no importer and is not an entry — must be nominated; got %v", dead)
	}
	if dead["src/used.js"] {
		t.Error("used.js is imported by index.js — must not be nominated")
	}
	if dead["src/index.js"] {
		t.Error("index.js is a conventional entry — must not be nominated")
	}
}

// TestDeadModuleSparesImportedAcrossDirs proves the importer can live anywhere:
// a relative import from a sibling directory still spares the target.
func TestDeadModuleSparesImportedAcrossDirs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a/foo.js", "export const f = 1\n")
	writeFile(t, root, "b/bar.js", "import { f } from '../a/foo'\n")

	dead := deadSet(t, root, Defaults())
	if dead["a/foo.js"] {
		t.Errorf("foo.js is imported from b/bar.js — must not be nominated; got %v", dead)
	}
}

// TestDeadModuleSparesEntryFromPackageJSON: a file named only as package.json
// "main" (no import) is an entry and must be spared, while a true orphan beside
// it is still caught.
func TestDeadModuleSparesEntryFromPackageJSON(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{"name":"x","main":"lib/server.js"}`)
	writeFile(t, root, "lib/server.js", "export const serve = () => {}\n")
	writeFile(t, root, "lib/orphan.js", "export const dead = 1\n")

	dead := deadSet(t, root, Defaults())
	if dead["lib/server.js"] {
		t.Error("server.js is the package.json main entry — must not be nominated")
	}
	if !dead["lib/orphan.js"] {
		t.Errorf("orphan.js is unreferenced — must be nominated; got %v", dead)
	}
}

// TestDeadModuleSparesEntryFromHTML: a non-conventionally-named file referenced
// only by an HTML <script src> is live (the corpus sweep catches it).
func TestDeadModuleSparesEntryFromHTML(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "index.html", `<html><body><script src="app/boot.js"></script></body></html>`)
	writeFile(t, root, "app/boot.js", "console.log('boot')\n")

	dead := deadSet(t, root, Defaults())
	if dead["app/boot.js"] {
		t.Errorf("boot.js is referenced by index.html — must not be nominated; got %v", dead)
	}
}

// TestDeadModuleSparesBundlerEntry is the Azure-Mapper pattern: an esbuild-style
// entryPoints array in a build script names a module as a bare string. The corpus
// sweep must treat that string as a live reference even though no source imports it.
func TestDeadModuleSparesBundlerEntry(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "build.js", "require('esbuild').build({ entryPoints: ['src/widget.js'], bundle: true })\n")
	writeFile(t, root, "src/widget.js", "export const w = 1\n")

	dead := deadSet(t, root, Defaults())
	if dead["src/widget.js"] {
		t.Errorf("widget.js is an esbuild entry point in build.js — must not be nominated; got %v", dead)
	}
}

// TestDeadModuleRespectsLiveModulesConfig: the explicit escape hatch in
// humify.config.json keeps a file off the candidate list even when the detector
// can find no reference (the dynamic-import / plugin-registry case).
func TestDeadModuleRespectsLiveModulesConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src/plugin.js", "export const p = 1\n")

	bare := deadSet(t, root, Defaults())
	if !bare["src/plugin.js"] {
		t.Fatalf("precondition: plugin.js should be nominated without config; got %v", bare)
	}

	cfg := Defaults()
	cfg.LiveModules = []string{"src/plugin.js"}
	if deadSet(t, root, cfg)["src/plugin.js"] {
		t.Error("plugin.js is pinned live in config — must not be nominated")
	}
}

// TestDeadModuleSkipsTestsAndUnsupportedLangs: test files and Go (cut from v1 for
// false-positive reasons — same-package Go files never import each other) are
// never nominated, even when unreferenced.
func TestDeadModuleSkipsTestsAndUnsupportedLangs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "widget.test.js", "test('x', () => {})\n")
	writeFile(t, root, "lonely.go", "package main\n\nfunc helper() {}\n")
	writeFile(t, root, "real-orphan.js", "export const o = 1\n") // keeps the detector active

	dead := deadSet(t, root, Defaults())
	if dead["widget.test.js"] {
		t.Error("test files must never be nominated as dead modules")
	}
	if dead["lonely.go"] {
		t.Error("Go is out of scope in v1 — must never be nominated")
	}
	if !dead["real-orphan.js"] {
		t.Errorf("detector must still be active; real-orphan.js should be nominated; got %v", dead)
	}
}

// TestDeadModuleSkipsConfigAndBuildScripts: tooling configs (*.config.js, run by a
// tool) and build entry scripts (build.js, build-web.js, run directly) are never
// imported as leaf modules — nominating them as dead is a false positive. Caught on
// the real Azure-Mapper repo, where playwright.config.js and build-web.js were
// wrongly flagged.
func TestDeadModuleSkipsConfigAndBuildScripts(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "playwright.config.js", "module.exports = { testDir: './tests' }\n")
	writeFile(t, root, "build-web.js", "require('esbuild').build({ entryPoints: ['src/site.js'] })\n")
	writeFile(t, root, "vite.config.ts", "export default { root: '.' }\n")
	writeFile(t, root, "real-orphan.js", "export const o = 1\n") // keeps the detector active

	dead := deadSet(t, root, Defaults())
	for _, f := range []string{"playwright.config.js", "build-web.js", "vite.config.ts"} {
		if dead[f] {
			t.Errorf("%s is tooling/build, run directly — must not be nominated as a dead module", f)
		}
	}
	if !dead["real-orphan.js"] {
		t.Errorf("detector must still be active; real-orphan.js should be nominated; got %v", dead)
	}
	// The build script's own entry (src/site.js) must be spared by the corpus sweep.
	writeFile(t, root, "src/site.js", "export const s = 1\n")
	if deadSet(t, root, Defaults())["src/site.js"] {
		t.Error("src/site.js is build-web.js's entry point — must be spared")
	}
}

// TestDeadModuleCommentedImportStillNominates: a reference that exists only inside
// a comment is itself dead, so comment-stripping the import index means the
// referenced file is still correctly nominated.
func TestDeadModuleCommentedImportStillNominates(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "importer.js", "// import './ghost'\nexport const real = 1\n")
	writeFile(t, root, "ghost.js", "export const g = 1\n")

	dead := deadSet(t, root, Defaults())
	if !dead["ghost.js"] {
		t.Errorf("ghost.js is referenced only in a comment — must still be nominated; got %v", dead)
	}
}

// TestFalseNominationKeepsLiveFindings is the safety invariant the advisor flagged:
// nominating a file as dead must NEVER erase the file's other findings. analyze is
// add-only, so a file that is both an unreferenced candidate AND has a real
// swallowed_error keeps both findings. Removal is only ever confirmed empirically
// at apply time — never by deleting findings on an unproven heuristic.
func TestFalseNominationKeepsLiveFindings(t *testing.T) {
	root := t.TempDir()
	// Unreferenced (→ dead_module candidate) and contains a swallowed error.
	writeFile(t, root, "orphan.js", "function f() {\n  try { g(); } catch (e) {}\n}\n")

	a, err := Run(root, Defaults())
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	var sawDead, sawSwallowed bool
	for _, f := range a.Findings {
		if f.File != "orphan.js" {
			continue
		}
		switch f.Signal {
		case "dead_module":
			sawDead = true
		case "swallowed_error":
			sawSwallowed = true
		}
	}
	if !sawDead {
		t.Error("orphan.js should be nominated as a dead_module candidate")
	}
	if !sawSwallowed {
		t.Error("dead-module nomination must NOT erase orphan.js's swallowed_error finding")
	}
}
