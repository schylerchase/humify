package verify

import (
	"os/exec"
	"strings"
	"time"

	"github.com/schylerryan/humify/internal/humify/state"
)

// BaselineSnapshot is a saved pre-edit verify run, persisted to
// .humify/verify-baseline.json. It is the read-only half of baseline-aware
// verify: `--save-baseline` writes one before the AI edits, `--baseline` loads
// it after and diffs via Delta. HeadSHA anchors staleness (a commit landed
// since), and Dirty flags the dangerous ordering mistake — saving a baseline
// after editing has already begun, so the baseline silently contains the
// breakage and every regression then reads as ambient.
type BaselineSnapshot struct {
	Schema  int        `json:"schema"`
	SavedAt string     `json:"saved_at"` // RFC3339, injected via now
	HeadSHA string     `json:"head_sha"` // "" when root is not a git repo
	Dirty   bool       `json:"dirty"`    // tree dirty outside .humify/ AT SAVE
	Result  Validation `json:"result"`   // the captured pre-edit validation
}

// SaveBaseline captures v as the pre-edit baseline. It writes only under
// .humify/ (humify's own state dir) — verify never touches target source — so a
// non-git or read-only target still works (HeadSHA just stays empty).
//
// dirty must be measured by the caller BEFORE running the validation commands:
// verify's own build can litter the tree (e.g. `go build ./...` drops a binary in
// cwd), so a dirtiness check taken after Run() would false-positive. The honest
// signal is "was the tree dirty when the user invoked --save-baseline".
func SaveBaseline(root string, v Validation, dirty bool, now time.Time) error {
	head, _ := GitHead(root)
	snap := BaselineSnapshot{
		Schema:  state.Schema,
		SavedAt: now.UTC().Format(time.RFC3339),
		HeadSHA: head,
		Dirty:   dirty,
		Result:  v,
	}
	return state.Save(root, state.BaselineFile, snap)
}

// LoadBaseline reads the saved baseline. ok is false when none exists or it is
// unreadable, so a caller can degrade loudly to a plain single run.
func LoadBaseline(root string) (BaselineSnapshot, bool) {
	var snap BaselineSnapshot
	if err := state.Load(root, state.BaselineFile, &snap); err != nil {
		return BaselineSnapshot{}, false
	}
	return snap, true
}

// BaselineStale reports whether HEAD moved since the baseline was saved — the
// snapshot then predates a commit and no longer reflects the pre-edit tree. With
// no git anchor (HeadSHA empty, or root not a repo now) staleness is unknowable,
// so it returns false rather than cry wolf.
func BaselineStale(snap BaselineSnapshot, root string) bool {
	if snap.HeadSHA == "" {
		return false
	}
	head, ok := GitHead(root)
	if !ok {
		return false
	}
	return head != snap.HeadSHA
}

// GitHead returns the current commit SHA, and false if root is not a git repo or
// has no commits. (Moved here from apply: it is a read-only repo inspector, which
// is verify's role.)
func GitHead(root string) (string, bool) {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// RepoDirtyExcludingHumify reports uncommitted changes OUTSIDE .humify/. humify's
// own state dir (analysis, plan, quarantine, baseline) is created by the tool
// itself and must not count as user dirt. A non-repo or git error returns false
// (nothing to warn about). (Moved here from apply.)
func RepoDirtyExcludingHumify(root string) bool {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:]) // porcelain: "XY <path>"
		if path == state.Dir || strings.HasPrefix(path, state.Dir+"/") {
			continue
		}
		return true
	}
	return false
}
