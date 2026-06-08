package verify

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"humify-ng/internal/humify/detect"
	"humify-ng/internal/humify/scan"
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

func detectFor(t *testing.T, root string) []command {
	t.Helper()
	res, err := scan.Walk(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	return Detect(detect.Detect(res, root), root)
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
