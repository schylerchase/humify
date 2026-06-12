package verify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJSProviderDetect(t *testing.T) {
	mk := func(t *testing.T, withPkg, withTestScript, withC8 bool) string {
		root := t.TempDir()
		if withPkg {
			scripts := ""
			if withTestScript {
				scripts = `"scripts":{"test":"node test.js"}`
			}
			os.WriteFile(filepath.Join(root, "package.json"), []byte("{"+scripts+"}"), 0o644)
		}
		if withC8 {
			bin := filepath.Join(root, "node_modules", ".bin")
			os.MkdirAll(bin, 0o755)
			os.WriteFile(filepath.Join(bin, "c8"), []byte("#!/bin/sh\n"), 0o755)
		}
		return root
	}
	p := jsProvider{}
	if !p.Detect(mk(t, true, true, true)) {
		t.Error("package.json + test script + installed c8 -> Detect true")
	}
	if p.Detect(mk(t, true, true, false)) {
		t.Error("no installed c8 -> Detect false (stay unmeasured, never guess)")
	}
	if p.Detect(mk(t, true, false, true)) {
		t.Error("no test script -> Detect false")
	}
	if p.Detect(mk(t, false, false, false)) {
		t.Error("no package.json -> Detect false")
	}
}

func TestParseIstanbul(t *testing.T) {
	// Shape captured from a real `c8 --reporter=json` run.
	data := `{
      "/abs/repo/src/a.js": {"path":"/abs/repo/src/a.js","statementMap":{"0":{"start":{"line":3}},"1":{"start":{"line":4}}},"s":{"0":2,"1":0}},
      "/abs/repo/src/b.js": {"path":"/abs/repo/src/b.js","statementMap":{"0":{"start":{"line":5}}},"s":{"0":0}}
    }`
	files, err := parseIstanbul([]byte(data), "/abs/repo")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !files["src/a.js"].Covered {
		t.Errorf("a.js has a hit statement (s.0=2) -> Covered; got %+v", files["src/a.js"])
	}
	if files["src/b.js"].Covered {
		t.Errorf("b.js has only a zero-hit statement -> not Covered; got %+v", files["src/b.js"])
	}
}
