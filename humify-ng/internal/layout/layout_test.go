package layout

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveInRootAcceptsRootLocal(t *testing.T) {
	root := t.TempDir()
	rel := AreaFragmentRel("01-core")
	got, err := ResolveInRoot(root, rel)
	if err != nil {
		t.Fatalf("unexpected error for root-local path: %v", err)
	}
	if want := filepath.Join(root, rel); got != want {
		t.Fatalf("resolved %q, want %q", got, want)
	}
}

func TestResolveInRootRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name string
		path string
	}{
		{"empty", ""},
		{"parent-escape", "../x.json"},
		{"clean-then-escape", "a/../../x.json"},
		{"absolute", filepath.Join(os.TempDir(), "x.json")},
	}
	for _, c := range cases {
		if _, err := ResolveInRoot(root, c.path); err == nil {
			t.Errorf("%s: expected rejection, got nil error", c.name)
		}
	}
}

// Windows drive-relative paths ("C:..\..\x") escape root while being neither
// absolute nor ".."-prefixed; the VolumeName guard must reject them.
func TestResolveInRootRejectsDriveRelativeWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("drive-relative paths are Windows-specific")
	}
	root := t.TempDir()
	if _, err := ResolveInRoot(root, `C:..\..\x.json`); err == nil {
		t.Error("drive-relative escape accepted")
	}
}
