package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWalkClassifiesAndIgnores(t *testing.T) {
	root := t.TempDir()
	write(t, root, "src/app.go", "package app\n\nfunc Run() {}\n")
	write(t, root, "src/app_test.go", "package app\n")
	write(t, root, "package.json", `{"name":"x"}`)
	write(t, root, "node_modules/dep/index.js", "module.exports = 1\n") // ignored dir
	write(t, root, "dist/bundle.js", "console.log(1)\n")                // ignored dir
	write(t, root, "bin.dat", "ok\x00\x01binary")                       // binary file

	res, err := Walk(root, nil)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	files := index(res)

	if _, ok := files["node_modules/dep/index.js"]; ok {
		t.Error("node_modules must be pruned")
	}
	if _, ok := files["dist/bundle.js"]; ok {
		t.Error("dist must be pruned")
	}
	app, ok := files["src/app.go"]
	if !ok {
		t.Fatal("src/app.go must be scanned")
	}
	if app.Lang != "go" || app.LOC != 3 || app.IsTest || app.IsConfig {
		t.Errorf("app.go: got lang=%q loc=%d test=%v config=%v", app.Lang, app.LOC, app.IsTest, app.IsConfig)
	}
	if !files["src/app_test.go"].IsTest {
		t.Error("app_test.go should be a test file")
	}
	if !files["package.json"].IsConfig {
		t.Error("package.json should be config")
	}
	if !files["bin.dat"].Binary {
		t.Error("bin.dat should be detected as binary")
	}
	if files["bin.dat"].LOC != 0 {
		t.Error("binary files must not be line-counted")
	}
}

func TestWalkHonorsHumifyignore(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".humifyignore", "*.generated.ts\n")
	write(t, root, "src/real.ts", "export const a = 1\n")
	write(t, root, "src/api.generated.ts", "export const b = 2\n")

	res, err := Walk(root, nil)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	files := index(res)
	if _, ok := files["src/api.generated.ts"]; ok {
		t.Error(".humifyignore pattern should have excluded api.generated.ts")
	}
	if _, ok := files["src/real.ts"]; !ok {
		t.Error("real.ts should be scanned")
	}
}

func TestWalkFlagsMinified(t *testing.T) {
	root := t.TempDir()
	big := strings.Repeat("a;", 25000) // ~50 KB on one line
	write(t, root, "libs/huge.js", big+"\n"+big+"\n")                                  // ~50 KB/line -> ratio heuristic
	write(t, root, "libs/tiny.min.js", "var a=1;\n")                                   // tiny but *.min.js -> name heuristic
	write(t, root, "src/app.js", strings.Repeat("const x = computeValue();\n", 300))   // ~26 b/l -> NOT minified

	files := index(mustWalk(t, root))
	if !files["libs/huge.js"].Minified {
		t.Error("a high bytes-per-line file must be flagged minified")
	}
	if !files["libs/tiny.min.js"].Minified {
		t.Error("a *.min.js file must be flagged minified by name")
	}
	if files["src/app.js"].Minified {
		t.Error("normal short-line source must NOT be flagged minified")
	}
}

func mustWalk(t *testing.T, root string) Result {
	t.Helper()
	res, err := Walk(root, nil)
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	return res
}

func index(res Result) map[string]File {
	m := make(map[string]File, len(res.Files))
	for _, f := range res.Files {
		m[f.Path] = f
	}
	return m
}

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
