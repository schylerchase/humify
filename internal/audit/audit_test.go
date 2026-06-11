package audit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/schylerryan/humify/internal/area"
	"github.com/schylerryan/humify/internal/fragment"
	"github.com/schylerryan/humify/internal/intel"
	"github.com/schylerryan/humify/internal/layout"
	"github.com/schylerryan/humify/internal/manifest"
)

// --- helpers ---------------------------------------------------------------

func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(layout.HumifyDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func putIntel(t *testing.T, root string, in intel.Data) {
	t.Helper()
	if err := intel.Write(root, in); err != nil {
		t.Fatal(err)
	}
}

func putManifest(t *testing.T, root string, areaIDs ...string) {
	t.Helper()
	var fr []manifest.Entry
	for _, id := range areaIDs {
		fr = append(fr, manifest.Entry{AreaID: id, Path: fragRel(id)})
	}
	if err := manifest.Write(root, manifest.Manifest{Fragments: fr}); err != nil {
		t.Fatal(err)
	}
}

func fragRel(areaID string) string {
	return layout.AreaFragmentRel(areaID)
}

// putFragment writes a fragment for areaID. valid=false writes a fragment whose
// finding has a bad severity, which must fail the done check.
func putFragment(t *testing.T, root, areaID string, valid bool) {
	t.Helper()
	sev := "info"
	if !valid {
		sev = "bogus"
	}
	f := fragment.Fragment{AreaID: areaID, Findings: []fragment.Finding{
		{ID: areaID + "-1", Title: "hazard", Severity: sev, File: "x.go", Line: 1},
	}}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, filepath.FromSlash(fragRel(areaID)))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkArea(id, kind, root string, files ...string) area.Area {
	return area.Area{ID: id, Kind: kind, Root: root, FilePaths: files, LOC: 100}
}

func jobByID(p Plan, id string) (Job, bool) {
	for _, j := range p.Pending {
		if j.AreaID == id {
			return j, true
		}
	}
	return Job{}, false
}

// --- BuildPlan -------------------------------------------------------------

func TestBuildPlanAllPending(t *testing.T) {
	root := newProject(t)
	putIntel(t, root, intel.Data{
		Target: "target",
		Areas:  []area.Area{mkArea("01-a", "dir", "a", "a/x.go"), mkArea("02-b", "dir", "b", "b/y.go")},
		Waves:  [][]string{{"01-a", "02-b"}},
	})
	putManifest(t, root, "01-a", "02-b")

	p, err := BuildPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Pending) != 2 || len(p.Done) != 0 || len(p.Missing) != 0 {
		t.Fatalf("want 2 pending/0 done/0 missing, got pending=%d done=%d missing=%d", len(p.Pending), len(p.Done), len(p.Missing))
	}
	j, ok := jobByID(p, "01-a")
	if !ok {
		t.Fatal("01-a not in pending")
	}
	if j.FragmentPath != fragRel("01-a") {
		t.Fatalf("fragment path = %q, want %q", j.FragmentPath, fragRel("01-a"))
	}
	if want := filepath.Join(layout.Dir, "tmp", "auditors", "01-a.prompt.md"); j.PromptPath != want {
		t.Fatalf("prompt path = %q, want %q", j.PromptPath, want)
	}
	if len(j.Files) != 1 || j.Files[0] != "a/x.go" {
		t.Fatalf("01-a files = %v, want [a/x.go]", j.Files)
	}
}

func TestBuildPlanResumableSkipsValidFragment(t *testing.T) {
	root := newProject(t)
	putIntel(t, root, intel.Data{
		Target: "t",
		Areas:  []area.Area{mkArea("01-a", "dir", "a", "a/x.go"), mkArea("02-b", "dir", "b", "b/y.go")},
		Waves:  [][]string{{"01-a", "02-b"}},
	})
	putManifest(t, root, "01-a", "02-b")
	putFragment(t, root, "01-a", true) // already audited

	p, err := BuildPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Done) != 1 || p.Done[0] != "01-a" {
		t.Fatalf("done = %v, want [01-a]", p.Done)
	}
	if len(p.Pending) != 1 || p.Pending[0].AreaID != "02-b" {
		t.Fatalf("pending = %v, want [02-b]", p.Pending)
	}
}

func TestBuildPlanInvalidFragmentStaysPending(t *testing.T) {
	root := newProject(t)
	putIntel(t, root, intel.Data{
		Target: "t",
		Areas:  []area.Area{mkArea("01-a", "dir", "a", "a/x.go")},
		Waves:  [][]string{{"01-a"}},
	})
	putManifest(t, root, "01-a")
	putFragment(t, root, "01-a", false) // exists but invalid severity

	p, err := BuildPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Done) != 0 || len(p.Pending) != 1 {
		t.Fatalf("invalid fragment must stay pending: done=%v pending=%d", p.Done, len(p.Pending))
	}
}

// A god file is split out of its directory's area; the dir auditor must not
// also own that file, or two auditors double-cover it.
func TestBuildPlanGodFileOwnershipDisjoint(t *testing.T) {
	root := newProject(t)
	putIntel(t, root, intel.Data{
		Target: "t",
		Areas: []area.Area{
			mkArea("01-app-core", "file", "src/app-core.js", "src/app-core.js"),
			mkArea("02-src", "dir", "src", "src/util.js", "src/view.js"),
		},
		Waves: [][]string{{"01-app-core", "02-src"}},
	})
	putManifest(t, root, "01-app-core", "02-src")

	p, err := BuildPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	owner := map[string]string{}
	for _, j := range p.Pending {
		for _, f := range j.Files {
			if prev, dup := owner[f]; dup {
				t.Fatalf("file %q owned by both %q and %q", f, prev, j.AreaID)
			}
			owner[f] = j.AreaID
		}
	}
	if owner["src/app-core.js"] != "01-app-core" {
		t.Fatalf("god file owned by %q, want 01-app-core", owner["src/app-core.js"])
	}
}

func TestBuildPlanDriftMissingFromIntel(t *testing.T) {
	root := newProject(t)
	putIntel(t, root, intel.Data{
		Target: "t",
		Areas:  []area.Area{mkArea("01-a", "dir", "a", "a/x.go")},
		Waves:  [][]string{{"01-a"}},
	})
	putManifest(t, root, "01-a", "99-ghost") // 99-ghost has no intel entry

	p, err := BuildPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Missing) != 1 || p.Missing[0] != "99-ghost" {
		t.Fatalf("missing = %v, want [99-ghost]", p.Missing)
	}
	if len(p.Pending) != 1 || p.Pending[0].AreaID != "01-a" {
		t.Fatalf("pending = %v, want [01-a]", p.Pending)
	}
}

func TestBuildPlanPendingSortedByWaveThenID(t *testing.T) {
	root := newProject(t)
	putIntel(t, root, intel.Data{
		Target: "t",
		Areas: []area.Area{
			mkArea("03-c", "dir", "c", "c/z.go"),
			mkArea("01-a", "dir", "a", "a/x.go"),
			mkArea("02-b", "dir", "b", "b/y.go"),
		},
		Waves: [][]string{{"02-b"}, {"01-a", "03-c"}}, // 02-b in wave 0; a,c in wave 1
	})
	putManifest(t, root, "01-a", "02-b", "03-c")

	p, err := BuildPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{p.Pending[0].AreaID, p.Pending[1].AreaID, p.Pending[2].AreaID}
	want := []string{"02-b", "01-a", "03-c"} // wave 0 first, then wave 1 sorted by id
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pending order = %v, want %v", got, want)
		}
	}
}

func TestBuildPlanNoIntel(t *testing.T) {
	root := newProject(t)
	putManifest(t, root, "01-a")
	if _, err := BuildPlan(root); !errors.Is(err, intel.ErrNotExist) {
		t.Fatalf("err = %v, want intel.ErrNotExist", err)
	}
}

func TestBuildPlanNoManifest(t *testing.T) {
	root := newProject(t)
	putIntel(t, root, intel.Data{Target: "t", Areas: []area.Area{mkArea("01-a", "dir", "a", "a/x.go")}})
	if _, err := BuildPlan(root); !errors.Is(err, ErrNoManifest) {
		t.Fatalf("err = %v, want ErrNoManifest", err)
	}
}

// --- DispatchRunner --------------------------------------------------------

func TestDispatchRunnerWritesOnePromptPerPending(t *testing.T) {
	root := newProject(t)
	putIntel(t, root, intel.Data{
		Target: "t",
		Areas:  []area.Area{mkArea("01-a", "dir", "a", "a/x.go"), mkArea("02-b", "dir", "b", "b/y.go")},
		Waves:  [][]string{{"01-a", "02-b"}},
	})
	putManifest(t, root, "01-a", "02-b")

	p, err := BuildPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	out, err := DispatchRunner{}.Dispatch(p)
	if err != nil {
		t.Fatal(err)
	}
	if out.Runner != "dispatch" || len(out.Prompts) != 2 {
		t.Fatalf("outcome = %+v, want runner=dispatch with 2 prompts", out)
	}
	for _, rel := range out.Prompts {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("prompt %q not written: %v", rel, err)
		}
		if len(b) == 0 {
			t.Fatalf("prompt %q is empty", rel)
		}
	}
}

func TestDispatchRunnerNoPendingWritesNothing(t *testing.T) {
	root := newProject(t)
	out, err := DispatchRunner{}.Dispatch(Plan{Root: root, Target: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Prompts) != 0 {
		t.Fatalf("prompts = %v, want none", out.Prompts)
	}
	if _, err := os.Stat(filepath.Join(layout.TmpDir(root), "auditors")); !os.IsNotExist(err) {
		t.Fatal("auditors dir created with no pending jobs")
	}
}

// --- RenderPrompt ----------------------------------------------------------

func TestRenderPromptCarriesContract(t *testing.T) {
	j := Job{
		AreaID: "01-core", Kind: "dir", Root: "src", Files: []string{"src/a.go"},
		FragmentPath: fragRel("01-core"),
	}
	out := RenderPrompt(j, "mytarget")
	for _, want := range []string{
		"01-core",           // area id
		fragRel("01-core"),  // exact output path
		"blocker",           // severity vocabulary
		"mytarget/src/a.go", // the file to read, joined to target
		"\"severity\"",      // schema field
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("prompt missing %q\n---\n%s", want, out)
		}
	}
}
