package analyze

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMeasureBraceLongestAndNesting(t *testing.T) {
	src := `package x

// header comment
func short() {}

func big(a int) int {
	if a > 0 {
		if a > 1 {
			if a > 2 {
				return a
			}
		}
	}
	return 0
}
`
	m := Measure(src, "go")
	if m.LongestFunc < 8 {
		t.Errorf("LongestFunc = %d, want >= 8 (the big function)", m.LongestFunc)
	}
	if m.LongestLine != 6 {
		t.Errorf("LongestLine = %d, want 6 (where func big starts)", m.LongestLine)
	}
	if m.MaxNesting < 4 {
		t.Errorf("MaxNesting = %d, want >= 4", m.MaxNesting)
	}
	if m.Comment < 1 {
		t.Error("header comment should be counted")
	}
}

func TestMeasureIgnoresBracesInStringsAndComments(t *testing.T) {
	// The braces in the string and comment must not be counted as a function body.
	src := "package x\n\nvar s = \"a { b { c\"\n// a stray } brace\nfunc f() { return }\n"
	m := Measure(src, "go")
	if m.MaxNesting > 1 {
		t.Errorf("MaxNesting = %d, want 1 (string/comment braces ignored)", m.MaxNesting)
	}
}

func TestMeasureIndentPython(t *testing.T) {
	src := "def f(x):\n    if x:\n        for i in range(x):\n            print(i)\n    return x\n"
	m := Measure(src, "py")
	if m.LongestFunc < 5 {
		t.Errorf("LongestFunc = %d, want >= 5", m.LongestFunc)
	}
	if m.MaxNesting < 4 {
		t.Errorf("MaxNesting = %d, want >= 4", m.MaxNesting)
	}
}

func TestInspectFindsSlopSignals(t *testing.T) {
	src := "package x\n\nfunc data() error {\n\t// TODO fix later\n\tif err != nil {\n\t}\n\treturn nil\n}\n"
	got := signalSet(inspectSrc("svc/data.go", "go", src))
	for _, want := range []string{"vague_name", "todo_marker", "swallowed_error"} {
		if !got[want] {
			t.Errorf("expected signal %q in %v", want, got)
		}
	}
}

func TestInspectPythonBroadAndSwallowed(t *testing.T) {
	src := "def f():\n    try:\n        g()\n    except Exception:\n        pass\n"
	got := signalSet(inspectSrc("f.py", "py", src))
	if !got["broad_catch"] {
		t.Errorf("expected broad_catch, got %v", got)
	}
	if !got["swallowed_error"] {
		t.Errorf("expected swallowed_error (except: pass), got %v", got)
	}
}

func TestInspectEmptyJSCatch(t *testing.T) {
	src := "function f() {\n  try { g(); } catch (e) {}\n}\n"
	got := signalSet(inspectSrc("f.js", "js", src))
	if !got["swallowed_error"] {
		t.Errorf("expected swallowed_error for empty catch, got %v", got)
	}
}

// --- regression tests for adversarial-review findings ---

func TestMeasureIgnoresMultilineRawString(t *testing.T) {
	// Braces inside a Go raw string must not corrupt nesting or the function span.
	src := "package x\n\nfunc f() {\n\tq := `\n}{{{`\n\tmore()\n}\n"
	m := Measure(src, "go")
	if m.MaxNesting != 1 {
		t.Errorf("MaxNesting = %d, want 1 (raw-string braces ignored)", m.MaxNesting)
	}
	if m.LongestFunc == 0 {
		t.Error("function span should close despite braces in the raw string")
	}
}

func TestMeasureIgnoresInlineBlockComment(t *testing.T) {
	src := "func g() {\n\tx := 1 /* } */\n\ty := 2\n\tz := 3\n}\n"
	m := Measure(src, "go")
	if m.LongestFunc < 5 {
		t.Errorf("LongestFunc = %d, want >= 5 (inline block-comment brace ignored)", m.LongestFunc)
	}
}

func TestMeasurePythonDocstringDoesNotInflateNesting(t *testing.T) {
	src := "def foo():\n    s = \"\"\"\n oneindent\n\"\"\"\n    if x:\n        if y:\n            z = 1\n"
	m := Measure(src, "py")
	if m.MaxNesting > 4 {
		t.Errorf("MaxNesting = %d, want <= 4 (docstring indentation ignored)", m.MaxNesting)
	}
}

func TestGoErrElseIsNotSwallowed(t *testing.T) {
	src := "func h() {\n\tif err != nil {\n\t} else {\n\t\tdoThing()\n\t}\n}\n"
	if signalSet(inspectSrc("h.go", "go", src))["swallowed_error"] {
		t.Error("`if err != nil {} else {...}` handles the error and must not be flagged")
	}
}

func TestSlopIgnoresStringLiteralContent(t *testing.T) {
	// A vague "declaration" living inside a raw string must not be flagged.
	src := "package x\n\nvar doc = `\ntype data struct {}\n`\n"
	if signalSet(inspectSrc("x.go", "go", src))["vague_name"] {
		t.Error("a declaration inside a string literal must not be flagged as vague_name")
	}
}

func TestNoisyCommentNeedsIdentifierMatch(t *testing.T) {
	noisy := "// getName\nfunc getName() {}\n"
	if !signalSet(inspectSrc("a.go", "go", noisy))["noisy_comment"] {
		t.Error("a comment restating the next identifier should be noisy")
	}
	prose := "// total\nsubtotal := a + b\n"
	if signalSet(inspectSrc("b.go", "go", prose))["noisy_comment"] {
		t.Error("'// total' above 'subtotal' is unrelated prose, not noise")
	}
}

func TestTodoInStringIsNotFlagged(t *testing.T) {
	src := "package x\n\nvar s = \"TODO not a real marker\"\n"
	if signalSet(inspectSrc("x.go", "go", src))["todo_marker"] {
		t.Error("TODO inside a string literal must not be flagged")
	}
	comment := "package x\n\n// TODO real marker\nvar y = 1\n"
	if !signalSet(inspectSrc("y.go", "go", comment))["todo_marker"] {
		t.Error("TODO in a comment should be flagged")
	}
}

func TestEmptyFileNotFlaggedStale(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "pkg/__init__.py", "") // empty but significant — must NOT be stale
	writeFile(t, root, "old.bak", "x\n")       // throwaway — must be stale
	a, err := Run(root, Defaults())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	stale := map[string]bool{}
	for _, f := range a.Findings {
		if f.Signal == "stale_file" {
			stale[f.File] = true
		}
	}
	if stale["pkg/__init__.py"] {
		t.Error("an empty __init__.py is significant and must not be quarantined as stale")
	}
	if !stale["old.bak"] {
		t.Error("old.bak (throwaway extension) should be flagged stale")
	}
}

func inspectSrc(path, lang, src string) []Finding {
	infos := scanLines(src, lang)
	return inspect(path, lang, infos, splitLines(src), measureFrom(src, infos, lang), Defaults())
}

func TestRunScoresAndPersistsShape(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "good.go", "package x\n\n// Add sums two ints.\nfunc Add(a, b int) int { return a + b }\n")
	writeFile(t, root, "bad.go", giantFunc())

	a, err := Run(root, Defaults())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if a.Tool != "humify" || a.Schema == 0 || a.GeneratedAt == "" {
		t.Errorf("metadata not stamped: %+v", a)
	}
	if len(a.Findings) == 0 {
		t.Fatal("expected findings from bad.go")
	}
	if a.Findings[0].ID == "" {
		t.Error("findings must have stable IDs")
	}
	// The worst file should sort first and be bad.go.
	if a.Files[0].Path != "bad.go" {
		t.Errorf("worst file = %q, want bad.go", a.Files[0].Path)
	}
	if a.Scores.Overall < 0 || a.Scores.Overall > 100 {
		t.Errorf("overall score out of range: %d", a.Scores.Overall)
	}
	if a.Scores.Maintainability >= 100 {
		t.Error("maintainability should drop given a giant function")
	}
}

func TestScoreMonotonicWithFindings(t *testing.T) {
	clean := fileHealth(nil)
	dirty := fileHealth([]Finding{{Severity: "major"}, {Severity: "warning"}})
	if dirty >= clean {
		t.Errorf("more findings must lower health: clean=%d dirty=%d", clean, dirty)
	}
}

func giantFunc() string {
	src := "package x\n\nfunc Process() {\n"
	for i := 0; i < 80; i++ {
		src += "\tprintln(1)\n"
	}
	return src + "}\n"
}

func signalSet(findings []Finding) map[string]bool {
	m := map[string]bool{}
	for _, f := range findings {
		m[f.Signal] = true
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
