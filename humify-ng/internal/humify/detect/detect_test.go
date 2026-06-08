package detect

import (
	"os"
	"path/filepath"
	"testing"

	"humify-ng/internal/humify/scan"
)

func TestDetectNodeProject(t *testing.T) {
	root := t.TempDir()
	write(t, root, "package.json", `{"main":"src/index.js","scripts":{"test":"jest","build":"tsc"}}`)
	write(t, root, "package-lock.json", "{}")
	write(t, root, "src/index.js", "console.log(1)\n")
	write(t, root, "src/util.ts", "export const a = 1\n")
	write(t, root, "src/util.test.ts", "test('a', () => {})\n")

	p := detectRoot(t, root)

	if p.PackageManager != "npm" {
		t.Errorf("package manager = %q, want npm", p.PackageManager)
	}
	if p.Scripts["test"] != "jest" || p.Scripts["build"] != "tsc" {
		t.Errorf("scripts not parsed: %v", p.Scripts)
	}
	if !contains(p.EntryPoints, "src/index.js") {
		t.Errorf("entry points missing src/index.js: %v", p.EntryPoints)
	}
	if len(p.Stack) == 0 || (p.Stack[0] != "ts" && p.Stack[0] != "js") {
		t.Errorf("stack should include js/ts: %v", p.Stack)
	}
	if p.Counts.Test != 1 {
		t.Errorf("test count = %d, want 1", p.Counts.Test)
	}
	if p.Counts.Config < 2 { // package.json + package-lock.json
		t.Errorf("config count = %d, want >=2", p.Counts.Config)
	}
}

func TestDetectGoProjectAndMakefile(t *testing.T) {
	root := t.TempDir()
	write(t, root, "go.mod", "module x\n\ngo 1.26\n")
	write(t, root, "main.go", "package main\n\nfunc main() {}\n")
	write(t, root, "Makefile", "build:\n\tgo build ./...\nlint:\n\tgolangci-lint run\n")

	p := detectRoot(t, root)

	if p.PackageManager != "go modules" {
		t.Errorf("package manager = %q, want go modules", p.PackageManager)
	}
	if !contains(p.EntryPoints, "main.go") {
		t.Errorf("entry points missing main.go: %v", p.EntryPoints)
	}
	if p.Scripts["build"] != "make build" || p.Scripts["lint"] != "make lint" {
		t.Errorf("makefile targets not detected: %v", p.Scripts)
	}
}

func detectRoot(t *testing.T, root string) Project {
	t.Helper()
	res, err := scan.Walk(root, nil)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return Detect(res, root)
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
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
