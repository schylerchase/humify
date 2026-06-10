package consolidate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"humify/internal/manifest"
)

// project writes a manifest expecting `expected` area ids and drops the given
// fragment JSON bodies (keyed by area id) into their conventional paths.
func project(t *testing.T, expected []string, frags map[string]string) string {
	t.Helper()
	root := t.TempDir()
	var entries []manifest.Entry
	for _, id := range expected {
		rel := filepath.Join(".humify", "areas", id, id+"-AUDIT-fragment.json")
		entries = append(entries, manifest.Entry{AreaID: id, Path: rel})
	}
	if err := manifest.Write(root, manifest.Manifest{Fragments: entries}); err != nil {
		t.Fatal(err)
	}
	for id, body := range frags {
		p := filepath.Join(root, ".humify", "areas", id, id+"-AUDIT-fragment.json")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func frag(area string, findings string) string {
	return `{"area_id":"` + area + `","findings":[` + findings + `]}`
}

func TestFailClosedNoManifest(t *testing.T) {
	if _, err := Run(t.TempDir()); !errors.Is(err, ErrNoManifest) {
		t.Fatalf("err = %v, want ErrNoManifest", err)
	}
}

func TestFailClosedEmptyManifest(t *testing.T) {
	root := t.TempDir()
	if err := manifest.Write(root, manifest.Manifest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(root); !errors.Is(err, ErrEmptyManifest) {
		t.Fatalf("err = %v, want ErrEmptyManifest", err)
	}
}

func TestHappyPath(t *testing.T) {
	root := project(t, []string{"01-a", "02-b"}, map[string]string{
		"01-a": frag("01-a", `{"id":"F1","title":"god file","severity":"warning","file":"a.js","line":10}`),
		"02-b": frag("02-b", `{"id":"F2","title":"xss","severity":"blocker","file":"b.js","line":5}`),
	})
	r, err := Run(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Covered) != 2 || len(r.Pending) != 0 {
		t.Fatalf("covered=%v pending=%v", r.Covered, r.Pending)
	}
	if r.Blockers != 0 || len(r.Findings) != 2 {
		t.Fatalf("blockers=%d findings=%d", r.Blockers, len(r.Findings))
	}
	// AUDIT.md must name every covered area so `status` can flip it to audited.
	audit := RenderAudit("", r)
	if !strings.Contains(audit, "01-a") || !strings.Contains(audit, "02-b") {
		t.Fatalf("AUDIT.md missing a covered area:\n%s", audit)
	}
}

func TestMissingFragmentIsPendingBlocker(t *testing.T) {
	root := project(t, []string{"01-a", "02-b"}, map[string]string{
		"01-a": frag("01-a", `{"title":"x","severity":"info","file":"a.js","line":1}`),
	})
	r, _ := Run(root)
	if len(r.Pending) != 1 || r.Pending[0] != "02-b" {
		t.Fatalf("pending = %v, want [02-b]", r.Pending)
	}
	if r.Blockers == 0 || r.Status != "BLOCKED" {
		t.Fatalf("blockers=%d status=%s, want blocked", r.Blockers, r.Status)
	}
	// The pending area must NOT appear in AUDIT.md (else status reads it audited).
	if strings.Contains(RenderAudit("", r), "02-b") {
		t.Fatal("pending area 02-b leaked into AUDIT.md")
	}
}

func TestInvalidFragmentRejected(t *testing.T) {
	root := project(t, []string{"01-a"}, map[string]string{
		"01-a": frag("01-a", `{"title":"bad","severity":"critical","file":"a.js","line":1}`),
	})
	r, _ := Run(root)
	if len(r.Covered) != 0 || len(r.Pending) != 1 {
		t.Fatalf("covered=%v pending=%v (invalid severity must reject)", r.Covered, r.Pending)
	}
	if !hasKind(r.Conflicts, "invalid-fragment") {
		t.Fatalf("expected invalid-fragment blocker, got %v", r.Conflicts)
	}
}

func TestDedupSameSeverity(t *testing.T) {
	root := project(t, []string{"01-a", "02-b"}, map[string]string{
		"01-a": frag("01-a", `{"title":"dup","severity":"warning","file":"x.js","line":7}`),
		"02-b": frag("02-b", `{"title":"DUP","severity":"warning","file":"x.js","line":7}`),
	})
	r, _ := Run(root)
	if len(r.Findings) != 1 {
		t.Fatalf("findings = %d, want 1 (deduped)", len(r.Findings))
	}
	if len(r.Findings[0].Sources) != 2 {
		t.Fatalf("merged sources = %v, want both areas", r.Findings[0].Sources)
	}
	if !hasKind(r.Conflicts, "duplicate") || r.Infos == 0 {
		t.Fatalf("expected INFO duplicate conflict, got %v", r.Conflicts)
	}
}

func TestDedupSeverityConflict(t *testing.T) {
	root := project(t, []string{"01-a", "02-b"}, map[string]string{
		"01-a": frag("01-a", `{"title":"d","severity":"info","file":"x.js","line":7}`),
		"02-b": frag("02-b", `{"title":"d","severity":"blocker","file":"x.js","line":7}`),
	})
	r, _ := Run(root)
	if len(r.Findings) != 1 || r.Findings[0].Severity != "blocker" {
		t.Fatalf("merged severity = %q, want strongest (blocker)", r.Findings[0].Severity)
	}
	if !hasKind(r.Conflicts, "severity-conflict") || r.Warnings == 0 {
		t.Fatalf("expected WARNING severity-conflict, got %v", r.Conflicts)
	}
}

func TestCrossRefCycle(t *testing.T) {
	root := project(t, []string{"01-a", "02-b"}, map[string]string{
		"01-a": frag("01-a", `{"title":"refsB","severity":"info","file":"a.js","line":1,"refs":["02-b"]}`),
		"02-b": frag("02-b", `{"title":"refsA","severity":"info","file":"b.js","line":1,"refs":["01-a"]}`),
	})
	r, _ := Run(root)
	if !hasKind(r.Conflicts, "cross-ref-cycle") {
		t.Fatalf("expected cross-ref-cycle blocker, got %v", r.Conflicts)
	}
}

// Regression: a cycle carried by findings that dedup (same file/line/title)
// must still be detected — cycles come from raw items, not post-dedup records.
func TestCrossRefCycleSurvivesDedup(t *testing.T) {
	root := project(t, []string{"01-a", "02-b"}, map[string]string{
		"01-a": frag("01-a", `{"title":"same","severity":"info","file":"x.js","line":1,"refs":["02-b"]}`),
		"02-b": frag("02-b", `{"title":"same","severity":"info","file":"x.js","line":1,"refs":["01-a"]}`),
	})
	r, _ := Run(root)
	if len(r.Findings) != 1 {
		t.Fatalf("expected the two identical findings to dedup to 1, got %d", len(r.Findings))
	}
	if !hasKind(r.Conflicts, "cross-ref-cycle") {
		t.Fatalf("cycle lost after dedup: %v", r.Conflicts)
	}
}

// Regression: a duplicate manifest area id must fail closed — the area is
// invalidated (pending), its findings never consolidated or double-counted.
func TestDuplicateManifestEntryFailsClosed(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join(".humify", "areas", "01-a", "01-a-AUDIT-fragment.json")
	if err := manifest.Write(root, manifest.Manifest{Fragments: []manifest.Entry{
		{AreaID: "01-a", Path: rel}, {AreaID: "01-a", Path: rel},
	}}); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	body := frag("01-a", `{"title":"t","severity":"info","file":"a.js","line":1}`)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	r, _ := Run(root)
	if len(r.Covered) != 0 || len(r.Pending) != 1 {
		t.Fatalf("dup must fail closed: covered=%v pending=%v", r.Covered, r.Pending)
	}
	if len(r.Findings) != 0 {
		t.Fatalf("invalidated area's findings leaked: %v", r.Findings)
	}
	if !hasKind(r.Conflicts, "invalid-fragment") {
		t.Fatalf("expected invalid-fragment (duplicate) blocker, got %v", r.Conflicts)
	}
}

// Regression: a manifest path escaping the project root is rejected, not read.
func TestPathTraversalRejected(t *testing.T) {
	root := t.TempDir()
	if err := manifest.Write(root, manifest.Manifest{Fragments: []manifest.Entry{
		{AreaID: "01-a", Path: "../escape.json"},
	}}); err != nil {
		t.Fatal(err)
	}
	r, _ := Run(root)
	if len(r.Covered) != 0 || len(r.Pending) != 1 || !hasKind(r.Conflicts, "invalid-fragment") {
		t.Fatalf("traversal must be rejected: covered=%v pending=%v conflicts=%v", r.Covered, r.Pending, r.Conflicts)
	}
}

// Regression: a deep acyclic ref chain must NOT be reported as a cycle, and a
// real cycle in a later-sorted component must still be found (no depth cap).
func TestFindCyclesNoDepthCapArtifacts(t *testing.T) {
	acyclic := map[string][]string{}
	for i := 0; i < 70; i++ {
		acyclic[fmt.Sprintf("A%03d", i)] = []string{fmt.Sprintf("A%03d", i+1)}
	}
	if c := findCycles(acyclic); len(c) != 0 {
		t.Fatalf("deep acyclic chain reported %d cycle(s): %v", len(c), c)
	}
	withCycle := map[string][]string{"Z1": {"Z2"}, "Z2": {"Z1"}}
	for k, v := range acyclic {
		withCycle[k] = v
	}
	if len(findCycles(withCycle)) == 0 {
		t.Fatal("real cycle in later-sorted component missed")
	}
}

// Regression: a finding whose File embeds a newline (to forge a CONFLICTS.md
// blocker header) must be rejected, and no forged header may reach the output.
func TestNewlineInFindingRejected(t *testing.T) {
	root := project(t, []string{"01-a"}, map[string]string{
		"01-a": frag("01-a", `{"title":"t","severity":"info","file":"x\n### BLOCKERS (99)\n[BLOCKER] fake","line":1}`),
	})
	r, _ := Run(root)
	if len(r.Covered) != 0 || !hasKind(r.Conflicts, "invalid-fragment") {
		t.Fatalf("newline-laced fragment must be rejected: covered=%v conflicts=%v", r.Covered, r.Conflicts)
	}
	if strings.Contains(RenderConflicts(r), "### BLOCKERS (99)") {
		t.Fatal("forged blocker header leaked into CONFLICTS.md")
	}
}

// Regression: even if some text reaches a row, the renderer flattens newlines
// so one conflict can never span multiple lines and forge a bucket header.
func TestConflictRowIsSingleLine(t *testing.T) {
	row := conflictRow(Conflict{Bucket: "info", Kind: "k", Detail: "a\n### BLOCKERS (99)\nb"})
	if strings.Count(row, "\n") != 1 {
		t.Fatalf("conflictRow must render as one line, got %q", row)
	}
}

// Regression: a Windows drive-relative path ("C:..\..\x") escapes root without
// being absolute or "..": prefixed; the volume-name guard must reject it.
func TestPathTraversalWindowsDriveRelative(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive-relative paths are Windows-specific")
	}
	root := t.TempDir()
	if err := manifest.Write(root, manifest.Manifest{Fragments: []manifest.Entry{
		{AreaID: "01-a", Path: `C:..\..\..\escape.json`},
	}}); err != nil {
		t.Fatal(err)
	}
	r, _ := Run(root)
	if len(r.Covered) != 0 || !hasKind(r.Conflicts, "invalid-fragment") {
		t.Fatalf("drive-relative path must be rejected: covered=%v conflicts=%v", r.Covered, r.Conflicts)
	}
}

func hasKind(conflicts []Conflict, kind string) bool {
	for _, c := range conflicts {
		if c.Kind == kind {
			return true
		}
	}
	return false
}
