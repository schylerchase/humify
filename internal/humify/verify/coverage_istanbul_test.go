package verify

import "testing"

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
