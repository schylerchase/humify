package handoff

import (
	"os"
	"testing"

	"github.com/schylerryan/humify/internal/layout"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	root := t.TempDir()
	want := Handoff{
		Stage: "audit", Action: "spawn", NextCommand: "humify consolidate",
		Prompts: []string{".humify/tmp/auditors/01-a.prompt.md"}, Note: "spawn then consolidate",
	}
	if err := Save(root, want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, found, err := Load(root)
	if err != nil || !found {
		t.Fatalf("Load: found=%v err=%v", found, err)
	}
	if got.Stage != want.Stage || got.NextCommand != want.NextCommand ||
		got.Action != want.Action || len(got.Prompts) != 1 || got.Prompts[0] != want.Prompts[0] {
		t.Fatalf("round-trip mismatch: got %+v want %+v", got, want)
	}
}

func TestLoadMissingIsNotAnError(t *testing.T) {
	root := t.TempDir()
	_, found, err := Load(root)
	if err != nil {
		t.Fatalf("missing file must not error, got %v", err)
	}
	if found {
		t.Fatal("missing file must report found=false")
	}
}

func TestConsumeDeletes(t *testing.T) {
	root := t.TempDir()
	if err := Save(root, Handoff{Stage: "plan", NextCommand: "humify execute"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, found, _ := Consume(root); !found {
		t.Fatal("first Consume must find the cursor")
	}
	if _, found, _ := Load(root); found {
		t.Fatal("cursor must be gone after Consume")
	}
}

func TestSaveLeavesNoTempArtifact(t *testing.T) {
	root := t.TempDir()
	if err := Save(root, Handoff{Stage: "heatmap", NextCommand: "humify audit"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(layout.HandoffFile(root) + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("atomic write left a .tmp artifact behind")
	}
}
