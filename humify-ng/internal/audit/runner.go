package audit

import (
	"os"
	"path/filepath"

	"humify-ng/internal/layout"
)

// Outcome reports what a Runner did with a Plan. In dispatch mode it lists the
// prompt files written; an autonomous runner would additionally report spawned
// and failed workers.
type Outcome struct {
	Runner  string   `json:"runner"`
	Prompts []string `json:"prompts,omitempty"` // prompt paths written, relative to project root
}

// Runner turns an audit Plan into produced (or to-be-produced) fragments. The
// seam keeps the binary's deterministic planning/barrier/merge fixed while the
// spawn strategy varies: the default DispatchRunner only materializes prompts
// for an external orchestrator; a future ClaudeRunner could shell out to
// `claude -p` and produce the fragments itself.
type Runner interface {
	Name() string
	Dispatch(p Plan) (Outcome, error)
}

// DispatchRunner writes one prompt per pending area and returns their paths. It
// produces no fragments itself — the orchestrator (the live agent host) spawns
// the read-only auditors, then runs `humify consolidate` to gather them.
type DispatchRunner struct{}

// Name identifies the runner in structured output.
func (DispatchRunner) Name() string { return "dispatch" }

// Dispatch materializes the per-area prompt files under .humify/tmp/auditors/.
func (DispatchRunner) Dispatch(p Plan) (Outcome, error) {
	out := Outcome{Runner: "dispatch"}
	if len(p.Pending) == 0 {
		return out, nil
	}
	dir := filepath.Join(layout.TmpDir(p.Root), "auditors")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return out, err
	}
	for _, j := range p.Pending {
		body := RenderPrompt(j, p.Target)
		dest := filepath.Join(p.Root, j.PromptPath)
		if err := os.WriteFile(dest, []byte(body), 0o644); err != nil {
			return out, err
		}
		out.Prompts = append(out.Prompts, j.PromptPath)
	}
	return out, nil
}
