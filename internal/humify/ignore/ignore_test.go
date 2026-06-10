package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultDirsIgnored(t *testing.T) {
	m := New(t.TempDir())
	for _, d := range []string{"node_modules", "dist", ".git", ".humify"} {
		if !m.Match(d, true) {
			t.Errorf("default dir %q should be ignored", d)
		}
		if !m.Match(filepath.Join("pkg", d), true) {
			t.Errorf("nested default dir %q should be ignored at any depth", d)
		}
	}
	// A default dir name as a FILE is not a directory skip; it falls through to rules.
	if m.Match("src/app.go", false) {
		t.Error("ordinary source must not be ignored by default")
	}
}

func TestGitignoreGlob(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".gitignore"), "*.log\n/secret.txt\ngenerated/\n")
	m := New(root)
	cases := []struct {
		rel   string
		isDir bool
		want  bool
	}{
		{"app.log", false, true},          // unanchored glob, any depth
		{"deep/nested/app.log", false, true},
		{"secret.txt", false, true},       // anchored, at root
		{"sub/secret.txt", false, false},  // anchored: not at root → kept
		{"generated", true, true},         // dir-only pattern
		{"generated/x.go", false, true},   // inside an ignored dir
		{"src/main.go", false, false},     // ordinary source kept
	}
	for _, c := range cases {
		if got := m.Match(c.rel, c.isDir); got != c.want {
			t.Errorf("Match(%q, dir=%v) = %v, want %v", c.rel, c.isDir, got, c.want)
		}
	}
}

func TestHumifyignoreAndNegation(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".gitignore"), "build/\n")
	// .humifyignore ignores all markdown, then re-includes README.md via negation.
	writeFile(t, filepath.Join(root, ".humifyignore"), "*.md\n!README.md\n")
	m := New(root)
	if !m.Match("docs/guide.md", false) {
		t.Error(".humifyignore *.md should ignore guide.md")
	}
	if m.Match("README.md", false) {
		t.Error("negation !README.md should re-include it")
	}
	if !m.Match("build/out.js", false) {
		t.Error(".gitignore build/ should still apply alongside .humifyignore")
	}
}

func TestDirOnlyPatternDoesNotMatchFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".gitignore"), "cache/\n")
	m := New(root)
	if m.Match("cache", false) {
		t.Error("a dir-only pattern must not match a file of the same name")
	}
	if !m.Match("cache", true) {
		t.Error("a dir-only pattern must match the directory")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
