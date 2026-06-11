package exec

import (
	"testing"

	"github.com/schylerryan/humify/internal/worktree"
)

func TestCurrentWavePicksLowestWithWork(t *testing.T) {
	waves := [][]string{{"01-a", "02-b"}, {"03-c"}}
	planned := map[string]bool{"01-a": true, "02-b": true, "03-c": true}

	// Nothing executed → wave 0, both slices.
	idx, slices, done := CurrentWave(waves, planned, map[string]bool{})
	if done || idx != 0 || len(slices) != 2 || slices[0] != "01-a" || slices[1] != "02-b" {
		t.Fatalf("got idx=%d slices=%v done=%v, want wave 0 [01-a 02-b]", idx, slices, done)
	}

	// Wave 0 fully executed → advance to wave 1.
	idx, slices, done = CurrentWave(waves, planned, map[string]bool{"01-a": true, "02-b": true})
	if done || idx != 1 || len(slices) != 1 || slices[0] != "03-c" {
		t.Fatalf("got idx=%d slices=%v done=%v, want wave 1 [03-c]", idx, slices, done)
	}

	// All executed → done.
	_, _, done = CurrentWave(waves, planned, map[string]bool{"01-a": true, "02-b": true, "03-c": true})
	if !done {
		t.Fatal("want done when every planned slice is executed")
	}
}

func TestCurrentWaveIgnoresUnplanned(t *testing.T) {
	waves := [][]string{{"01-a", "02-b"}}
	// 02-b has no plan (e.g. no findings) → not a target; wave 0 work is just 01-a.
	idx, slices, done := CurrentWave(waves, map[string]bool{"01-a": true}, map[string]bool{})
	if done || idx != 0 || len(slices) != 1 || slices[0] != "01-a" {
		t.Fatalf("got idx=%d slices=%v done=%v, want wave 0 [01-a]", idx, slices, done)
	}
}

func TestManifestRoundTripAndClear(t *testing.T) {
	root := t.TempDir()
	if got, err := LoadManifest(root); err != nil || got != nil {
		t.Fatalf("empty load = %v, %v", got, err)
	}
	entries := []worktree.Entry{
		{SliceID: "01-a", WorktreePath: "/wt/01-a", Branch: "humify-slice-01-a", ExpectedBase: "base0"},
	}
	if err := SaveManifest(root, entries); err != nil {
		t.Fatal(err)
	}
	got, err := LoadManifest(root)
	if err != nil || len(got) != 1 || got[0].SliceID != "01-a" || got[0].ExpectedBase != "base0" {
		t.Fatalf("round-trip = %v, %v", got, err)
	}
	if err := ClearManifest(root); err != nil {
		t.Fatal(err)
	}
	if got, _ := LoadManifest(root); got != nil {
		t.Fatal("manifest should be gone after clear")
	}
	if err := ClearManifest(root); err != nil {
		t.Fatalf("clear of absent manifest must be a no-op, got %v", err)
	}
}

func TestCommitLogAppends(t *testing.T) {
	root := t.TempDir()
	if err := AppendCommits(root, []CommitRecord{{SliceID: "01-a", Wave: 0, CommitSHA: "sha1"}}); err != nil {
		t.Fatal(err)
	}
	if err := AppendCommits(root, []CommitRecord{{SliceID: "02-b", Wave: 1, CommitSHA: "sha2"}}); err != nil {
		t.Fatal(err)
	}
	recs, err := LoadCommits(root)
	if err != nil || len(recs) != 2 || recs[0].SliceID != "01-a" || recs[1].CommitSHA != "sha2" {
		t.Fatalf("commit log = %v, %v (want ordered 01-a then 02-b)", recs, err)
	}
}

func TestVerifyValidate(t *testing.T) {
	cases := []struct {
		name string
		v    Verify
		ok   bool
	}{
		{"pass clean", Verify{AreaID: "01-a", Passed: true}, true},
		{"fail with reasons", Verify{AreaID: "01-a", Passed: false, Failed: []string{"claim 2 false"}}, true},
		{"empty area", Verify{Passed: true}, false},
		{"fail no reasons", Verify{AreaID: "01-a", Passed: false}, false},
		{"pass with reasons", Verify{AreaID: "01-a", Passed: true, Failed: []string{"x"}}, false},
	}
	for _, c := range cases {
		if (c.v.Validate() == nil) != c.ok {
			t.Errorf("%s: Validate ok=%v, want %v", c.name, c.v.Validate() == nil, c.ok)
		}
	}
}
