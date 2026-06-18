package verify

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/schylerryan/humify/internal/humify/detect"
	"github.com/schylerryan/humify/internal/humify/scan"
)

func TestDetectGoCommands(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module x\n\ngo 1.26\n")
	writeFile(t, root, "main.go", "package main\nfunc main() {}\n")

	cmds := detectFor(t, root)
	kinds := kindSet(cmds)
	for _, want := range []string{"build", "vet", "test"} {
		if !kinds[want] {
			t.Errorf("go project should detect %q command; got %v", want, kinds)
		}
	}
}

func TestDetectNodeUsesDeclaredScriptsOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{"scripts":{"test":"jest","lint":"eslint ."}}`)
	writeFile(t, root, "package-lock.json", "{}")

	cmds := detectFor(t, root)
	kinds := kindSet(cmds)
	if !kinds["test"] || !kinds["lint"] {
		t.Errorf("should detect declared test+lint scripts: %v", kinds)
	}
	if kinds["typecheck"] {
		t.Error("must not invent a typecheck script that package.json does not declare")
	}
	for _, c := range cmds {
		if c.kind == "test" && c.line != "npm run test" {
			t.Errorf("test command = %q, want 'npm run test'", c.line)
		}
	}
}

// Regression for the Azure-Mapper dogfood gap: a project with `test` (run) plus
// `test:unit`/`test:node` (NOT run) must surface the siblings as skipped, so a
// green that only ran `test` is not mistaken for full coverage.
func TestDetectSkippedReportsNamespacedSiblings(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "package.json", `{"scripts":{`+
		`"test":"playwright test","test:unit":"jest","test:node":"node --test",`+
		`"lint":"eslint .","lint:ci":"eslint . --max-warnings 0","foo:bar":"echo hi"}}`)
	writeFile(t, root, "package-lock.json", "{}")

	lines := map[string]bool{}
	for _, c := range skippedFor(t, root) {
		lines[c.line] = true
	}
	for _, want := range []string{"npm run test:unit", "npm run test:node", "npm run lint:ci"} {
		if !lines[want] {
			t.Errorf("expected skipped sibling %q, got %v", want, lines)
		}
	}
	// Canonical (run) scripts and non-validation namespaces must NOT be reported as skipped.
	for _, bad := range []string{"npm run test", "npm run lint", "npm run foo:bar"} {
		if lines[bad] {
			t.Errorf("%q must not be reported as a skipped sibling: %v", bad, lines)
		}
	}
	if !kindSet(detectFor(t, root))["test"] {
		t.Error("canonical test script should still be detected to run")
	}
}

func TestRunNoCommandsIsSkippedNotFailed(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "notes.txt", "just text\n")

	v, err := Run(root, time.Now())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !v.Passed {
		t.Error("a repo with no validation commands must not be reported as failed")
	}
	if v.Validated {
		t.Error("nothing ran, so Validated must be false — Passed alone is vacuously true")
	}
	if len(v.Commands) != 1 || !v.Commands[0].Skipped {
		t.Errorf("expected a single skipped sentinel, got %+v", v.Commands)
	}
}

func TestDetectPytestFromBareTestsLayout(t *testing.T) {
	// The most common Python layout: a tests/ dir with test_*.py plus a
	// requirements.txt, but no pytest.ini or pyproject.toml. verify must still
	// detect pytest by reusing the scan's test-file classification rather than
	// requiring a config file.
	root := t.TempDir()
	writeFile(t, root, "requirements.txt", "requests\n")
	writeFile(t, root, "app.py", "def main():\n    return 1\n")
	writeFile(t, root, "tests/test_app.py", "def test_main():\n    assert True\n")

	var testLine string
	for _, c := range detectFor(t, root) {
		if c.kind == "test" {
			testLine = c.line
		}
	}
	if testLine != "python3 -m pytest -q" {
		t.Errorf("bare pytest layout should detect 'python3 -m pytest -q'; got %q", testLine)
	}
}

func TestDetectGoTestsDoNotTriggerPytest(t *testing.T) {
	// Counts.Test is language-agnostic, so the Python gate must keep a Go repo
	// with _test.go files from spuriously firing pytest.
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module x\n\ngo 1.26\n")
	writeFile(t, root, "main.go", "package main\nfunc main() {}\n")
	writeFile(t, root, "main_test.go", "package main\nimport \"testing\"\nfunc TestX(t *testing.T) {}\n")

	for _, c := range detectFor(t, root) {
		if c.line == "python3 -m pytest -q" {
			t.Errorf("go repo must not detect pytest; got %+v", c)
		}
	}
}

func detectFor(t *testing.T, root string) []command {
	t.Helper()
	res, err := scan.Walk(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	return Detect(detect.Detect(res, root), root)
}

func skippedFor(t *testing.T, root string) []command {
	t.Helper()
	res, err := scan.Walk(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	return DetectSkipped(detect.Detect(res, root))
}

func kindSet(cmds []command) map[string]bool {
	m := map[string]bool{}
	for _, c := range cmds {
		m[c.kind] = true
	}
	return m
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
