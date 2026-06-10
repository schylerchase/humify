package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeriveCascade(t *testing.T) {
	cases := []struct {
		name        string
		files       []string
		auditDoc    string
		patchlogDoc string
		areaID      string
		want        Status
	}{
		{"empty dir", nil, "", "", "01-core", Empty},
		{"mapped", []string{"01-MAP.md"}, "", "", "01-core", Mapped},
		{
			"audit-incomplete: fragment but AUDIT.md does not cover it",
			[]string{"01-MAP.md", "01-AUDIT-fragment.json"}, "", "", "01-core", AuditIncomplete,
		},
		{
			"audited: fragment + AUDIT covers id",
			[]string{"01-AUDIT-fragment.json"}, "## 01-core\nfindings...", "", "01-core", Audited,
		},
		{"planned", []string{"01-01-PLAN.md"}, "", "", "01-core", Planned},
		{
			"executed: summaries match plans",
			[]string{"01-01-PLAN.md", "01-02-PLAN.md", "01-01-SUMMARY.md", "01-02-SUMMARY.md"},
			"", "", "01-core", Executed,
		},
		{
			"planned: summaries fewer than plans",
			[]string{"01-01-PLAN.md", "01-02-PLAN.md", "01-01-SUMMARY.md"}, "", "", "01-core", Planned,
		},
		{
			"patched: PATCHLOG covers id",
			[]string{"01-01-PLAN.md", "01-01-SUMMARY.md"}, "", "- 01-core", "01-core", Patched,
		},
		{
			"word boundary: 01-core-utils in AUDIT must not satisfy 01-core",
			[]string{"01-AUDIT-fragment.json"}, "## 01-core-utils mapped", "", "01-core", AuditIncomplete,
		},
		{
			"fail-open guard: a covered area's finding naming a pending id in its file path must not cover the pending area",
			[]string{"01-AUDIT-fragment.json"},
			"## Areas consolidated\n- 09-other\n\n## Blocker findings (1)\n- [09-other] 01-core/handler.go:1 — bug (source: 09-other)",
			"", "01-core", AuditIncomplete,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.files {
				writeFile(t, filepath.Join(dir, f))
			}
			if got := Derive(dir, tc.areaID, tc.auditDoc, tc.patchlogDoc).Status; got != tc.want {
				t.Fatalf("status = %q, want %q", got, tc.want)
			}
		})
	}
}

// Regression: covers() must match a real coverage row but not a structural
// heading the renderer itself emits (e.g. "## Areas consolidated").
func TestCoversIgnoresHeadings(t *testing.T) {
	doc := "## Areas consolidated\n- 01-core\n\n## Blocker findings (1)\n"
	if covers(doc, "Areas consolidated") || covers(doc, "Blocker findings (1)") {
		t.Fatal("covers matched a structural heading")
	}
	if !covers(doc, "01-core") {
		t.Fatal("covers failed to match a real coverage row")
	}
}

func TestDeriveNoDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if got := Derive(missing, "01-core", "", "").Status; got != NoDirectory {
		t.Fatalf("status = %q, want %q", got, NoDirectory)
	}
}

func TestDeriveCounts(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"01-MAP.md", "01-AUDIT-fragment.json", "01-01-PLAN.md", "01-02-PLAN.md", "01-01-SUMMARY.md"} {
		writeFile(t, filepath.Join(dir, f))
	}
	a := Derive(dir, "01-core", "", "")
	if a.Plans != 2 || a.Summaries != 1 || a.Fragments != 1 || !a.HasMap {
		t.Fatalf("counts wrong: %+v", a)
	}
	if a.Status != Planned {
		t.Fatalf("status = %q, want planned", a.Status)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}
