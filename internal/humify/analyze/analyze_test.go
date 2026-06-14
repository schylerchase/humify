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

func TestMeasureTemplateLiteralResumesCode(t *testing.T) {
	// A JS template literal with ${} interpolation (braces inside) and an escaped
	// backtick must NOT swallow the real function that follows it.
	src := "const t = `a ${fn({k: 1})} b \\` c`\n\nfunction big() {\n\tif (a) {\n\t\tif (b) {\n\t\t\treturn 1\n\t\t}\n\t}\n}\n"
	m := Measure(src, "js")
	if m.LongestFunc < 6 {
		t.Errorf("LongestFunc = %d; the function after the template must be measured", m.LongestFunc)
	}
	if m.MaxNesting < 3 {
		t.Errorf("MaxNesting = %d; nesting inside the post-template function must be seen", m.MaxNesting)
	}
}

func TestGoRawStringEndingInBackslashStillCloses(t *testing.T) {
	// Go raw strings have no escapes: a backslash before the closing backtick is
	// literal content, so the string must still close and the next func be measured.
	src := "package x\n\nfunc a() { p := `C:\\` ; _ = p }\n\nfunc big() {\n\tif x {\n\t\tif y {\n\t\t\treturn\n\t\t}\n\t}\n}\n"
	m := Measure(src, "go")
	if m.MaxNesting < 3 {
		t.Errorf("MaxNesting = %d; a Go raw string ending in backslash must not swallow the rest", m.MaxNesting)
	}
}

func TestDeepNestingCountsBlocksNotIndent(t *testing.T) {
	// Wrapped call args indented far past their block must not inflate nesting —
	// only real block openers (lines ending in ':') count.
	wrapped := "def f():\n    if a:\n        do(\n                    x,\n                    y,\n        )\n"
	if m := Measure(wrapped, "py"); m.MaxNesting > 3 {
		t.Errorf("MaxNesting = %d; wrapped args must not nest past def+if (~2-3)", m.MaxNesting)
	}
	// Genuine nesting is still measured.
	nested := "def f():\n    if a:\n        for x in y:\n            if z:\n                w = 1\n"
	if m := Measure(nested, "py"); m.MaxNesting < 4 {
		t.Errorf("MaxNesting = %d; def>if>for>if is real depth, want >=4", m.MaxNesting)
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

func TestInspectPythonBroadVsSwallowedAreExclusive(t *testing.T) {
	// `except Exception: pass` is an empty broad catch — it must be swallowed_error
	// ONLY, never both (the AdminMigrationTool double-count bug).
	empty := signalSet(inspectSrc("f.py", "py", "def f():\n    try:\n        g()\n    except Exception:\n        pass\n"))
	if !empty["swallowed_error"] {
		t.Errorf("empty broad catch should be swallowed_error, got %v", empty)
	}
	if empty["broad_catch"] {
		t.Errorf("an empty broad catch must NOT also be broad_catch (double-count), got %v", empty)
	}
	// A broad catch with a real body is broad_catch, not swallowed.
	handled := signalSet(inspectSrc("g.py", "py", "def f():\n    try:\n        g()\n    except Exception:\n        log(e)\n"))
	if !handled["broad_catch"] {
		t.Errorf("broad catch with a body should be broad_catch, got %v", handled)
	}
	if handled["swallowed_error"] {
		t.Errorf("a handled broad catch must not be swallowed_error, got %v", handled)
	}
}

// TestInspectBraceBroadCatch covers ROADMAP #7: broad_catch was Python/Ruby-only;
// brace languages emitted only swallowed_error. A catch-all with a body in JS/TS,
// Java/C# (Exception/Throwable/...), or C++ (catch(...)) is now broad_catch; a narrow
// typed catch is not; and an EMPTY broad catch stays swallowed_error only.
func TestInspectBraceBroadCatch(t *testing.T) {
	for _, c := range []struct{ name, path, lang, src string }{
		{"java", "A.java", "java", "class A { void f(){ try { g(); } catch (Exception e) { log(e); } } }\n"},
		{"csharp", "A.cs", "cs", "class A { void F(){ try { G(); } catch (Exception e) { Log(e); } } }\n"},
		{"cpp", "a.cpp", "cpp", "void f(){ try { g(); } catch (...) { handle(); } }\n"},
		{"js", "a.js", "js", "function f(){ try { g(); } catch (e) { log(e); } }\n"},
	} {
		got := signalSet(inspectSrc(c.path, c.lang, c.src))
		if !got["broad_catch"] {
			t.Errorf("%s: expected broad_catch, got %v", c.name, got)
		}
		if got["swallowed_error"] {
			t.Errorf("%s: a bodied broad catch must not be swallowed_error, got %v", c.name, got)
		}
	}
	if signalSet(inspectSrc("A.java", "java", "class A { void f(){ try { g(); } catch (IOException e) { log(e); } } }\n"))["broad_catch"] {
		t.Error("a narrow typed catch must not be broad_catch")
	}
	empty := signalSet(inspectSrc("A.java", "java", "class A { void f(){ try { g(); } catch (Exception e) {} } }\n"))
	if !empty["swallowed_error"] {
		t.Errorf("an empty broad catch should be swallowed_error, got %v", empty)
	}
	if empty["broad_catch"] {
		t.Errorf("an empty broad catch must NOT also be broad_catch (double-count), got %v", empty)
	}
}

func TestSwallowedRespectsIntent(t *testing.T) {
	// A bare empty catch with no explanation is still a real swallow.
	if !signalSet(inspectSrc("a.js", "js", "function f(){ try{g()}catch(e){} }\n"))["swallowed_error"] {
		t.Error("an undocumented empty catch must still be flagged")
	}
	// Documented (inline comment) or suppressed catches are intentional — skip them.
	cases := []struct{ path, lang, src string }{
		{"b.js", "js", "function f(){ try{g()}catch(e){ /* localStorage may be disabled */ } }\n"},
		{"c.py", "py", "def f():\n    try:\n        g()\n    except Exception:  # noqa: S110\n        pass\n"},
	}
	for _, c := range cases {
		if signalSet(inspectSrc(c.path, c.lang, c.src))["swallowed_error"] {
			t.Errorf("a documented/suppressed catch must not be flagged: %q", c.src)
		}
	}
	// A NARROW typed except + pass is deliberate narrow handling, not a swallow.
	narrow := "def f():\n    try:\n        g()\n    except json.JSONDecodeError:\n        pass\n"
	if signalSet(inspectSrc("d.py", "py", narrow))["swallowed_error"] {
		t.Error("a narrow typed `except SomeError: pass` must not be flagged swallowed")
	}
}

// TestSwallowedGoBodyCommentAndAssignment covers ROADMAP #3: the common multi-line
// Go err-block documented by a BODY comment must be honored (not a false positive),
// the assignment idiom `if err := g(); err != nil {}` must be caught, and a
// single-line empty catch must NOT be excused by an unrelated comment on the
// following line (the discriminator a naive catchLines=[i,i+1] fix would regress).
func TestSwallowedGoBodyCommentAndAssignment(t *testing.T) {
	bodyDoc := "package x\nfunc f() {\n\tif err != nil {\n\t\t// ignore: best effort\n\t}\n}\n"
	if signalSet(inspectSrc("a.go", "go", bodyDoc))["swallowed_error"] {
		t.Error("a multi-line err block documented by a body comment must not be swallowed_error")
	}
	assign := "package x\nfunc f() {\n\tif err := g(); err != nil {\n\t}\n}\n"
	if !signalSet(inspectSrc("b.go", "go", assign))["swallowed_error"] {
		t.Error("`if err := g(); err != nil {}` (empty) must be flagged swallowed_error")
	}
	jsTrail := "function f(){\n\ttry{g()}catch(e){}\n\t// unrelated\n}\n"
	if !signalSet(inspectSrc("c.js", "js", jsTrail))["swallowed_error"] {
		t.Error("a single-line empty catch must stay flagged despite a following-line comment")
	}
}

func TestNoisyCommentSkipsSectionLabels(t *testing.T) {
	// Real section-header / divider comments from the validated repos — all FPs.
	for _, c := range []string{
		"package x\n\n// State\nstate := 1\n",
		"package x\n\n// SQL Servers\nsqlServers := list()\n",
		"package x\n\n// ── Group card ──\ngroupCard := 1\n",
		"package x\n\n// --- Common flags ---\ncommonFlags := 1\n",
		"package x\n\n// Tags\ntags := 1\n",
	} {
		if signalSet(inspectSrc("x.go", "go", c))["noisy_comment"] {
			t.Errorf("section label/divider must not be noisy_comment: %q", c)
		}
	}
	// A bare camelCase restatement is still noise.
	if !signalSet(inspectSrc("x.go", "go", "package x\n\n// getName\nfunc getName() {}\n"))["noisy_comment"] {
		t.Error("a bare camelCase comment restating the next identifier is still noise")
	}
}

func TestVagueNameDoesNotFlagFilenames(t *testing.T) {
	// utils/helper/misc are idiomatic module names — flagging the filename is a FP.
	for _, p := range []string{"src/utils.js", "crates/agent/src/helper.rs", "routes/misc.js"} {
		lang := "js"
		if filepathExt(p) == ".rs" {
			lang = "rs"
		}
		if signalSet(inspectSrc(p, lang, "package x\n\nfunc ok() {}\n"))["vague_name"] {
			t.Errorf("an idiomatic filename must not be flagged vague: %q", p)
		}
	}
	// A vague *declaration* inside a file is still flagged.
	if !signalSet(inspectSrc("svc.go", "go", "package x\n\ntype Manager struct{}\n"))["vague_name"] {
		t.Error("a vague declaration is still flagged")
	}
}

func filepathExt(p string) string {
	for i := len(p) - 1; i >= 0 && p[i] != '/'; i-- {
		if p[i] == '.' {
			return p[i:]
		}
	}
	return ""
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

func TestTodoProseMentionNotFlagged(t *testing.T) {
	// A comment that describes TODO markers in prose is not itself a marker —
	// this is the self-referential FP Humify hit on its own slop.go.
	prose := "package x\n\n// scans for leftover TODO markers in comments\nvar y = 1\n"
	if signalSet(inspectSrc("p.go", "go", prose))["todo_marker"] {
		t.Error("a mid-sentence prose mention of TODO must not be flagged as a marker")
	}
	// Documentation that names the markers as a slash-list (the detector's own
	// doc style) is not a marker — keyword followed by "/" or ")", not ":"/"(".
	selfdoc := "package x\n\n// the markers (TODO/FIXME/XXX/HACK) we detect\nvar y = 1\n"
	if signalSet(inspectSrc("p.go", "go", selfdoc))["todo_marker"] {
		t.Error("a slash-separated list naming the markers is documentation, not a marker")
	}
	// Conventional markers — leading the comment, or tag-punctuated — still fire.
	for _, src := range []string{
		"package x\n\n// TODO fix this\nvar y = 1\n",
		"package x\n\n// FIXME: broken\nvar y = 1\n",
		"package x\n\n// HACK(bob) workaround\nvar y = 1\n",
		"package x\n\n// see the TODO: above\nvar y = 1\n",
	} {
		if !signalSet(inspectSrc("p.go", "go", src))["todo_marker"] {
			t.Errorf("a real marker must still be flagged: %q", src)
		}
	}
}

func TestVagueNameSkipsIdiomaticResultAndItem(t *testing.T) {
	// Result/Item are idiomatic, package-qualified Go names (scan.Result), not slop.
	for _, src := range []string{
		"package scan\n\ntype Result struct{ N int }\n",
		"package plan\n\ntype Item struct{ ID string }\n",
	} {
		if signalSet(inspectSrc("scan.go", "go", src))["vague_name"] {
			t.Errorf("Result/Item must not be flagged vague: %q", src)
		}
	}
	// Genuinely vague declarations are still flagged.
	if !signalSet(inspectSrc("svc.go", "go", "package x\n\ntype Manager struct{}\n"))["vague_name"] {
		t.Error("Manager is still a vague name and must be flagged")
	}
}

func inspectSrc(path, lang, src string) []Finding {
	infos := scanLines(src, lang)
	return inspect(path, lang, infos, splitLines(src), measureFrom(infos, lang), Defaults())
}

func TestMinifiedFileNotReviewed(t *testing.T) {
	root := t.TempDir()
	// Same code in a minified lib and in real source; only the real one is reviewed.
	body := "function f(){try{g()}catch(e){}}\n"
	writeFile(t, root, "libs/x.min.js", body)
	writeFile(t, root, "app.js", body)
	a, err := Run(root, Defaults())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, f := range a.Findings {
		if f.File == "libs/x.min.js" {
			t.Errorf("a minified file must not be reviewed; got %s on it", f.Signal)
		}
	}
	reviewedApp := false
	for _, f := range a.Findings {
		if f.File == "app.js" {
			reviewedApp = true
		}
	}
	if !reviewedApp {
		t.Error("real source app.js should still be reviewed (sanity)")
	}
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
