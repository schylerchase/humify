package audit

import (
	"time"

	"github.com/schylerryan/humify/internal/spawn"
)

// SpawnRunner writes the auditor prompts and then runs an operator-supplied agent
// command once per pending area through the shared spawn primitive — capped
// concurrency, a per-agent timeout, and a barrier that waits for all of them.
// After the barrier the primitive re-derives which fragments actually appeared
// (the success predicate below), so a crashed, hung (timed-out), or no-op agent
// is surfaced in Failed rather than silently passing. The gather/merge is still
// the separate `humify consolidate` stage.
type SpawnRunner struct {
	AgentCmd string        // operator-supplied; the prompt is delivered on stdin
	Jobs     int           // max concurrent agents
	Timeout  time.Duration // per-agent wall-clock cap (0 → spawn.DefaultTimeout)
	run      spawn.RunFunc // injectable for tests; nil → spawn.ShellExec
}

// Name identifies the runner in structured output.
func (SpawnRunner) Name() string { return "spawn" }

// Dispatch writes the prompts, spawns one agent per pending area (≤ Jobs at
// once), waits for all, then reports how many produced a valid fragment. Every
// auditor runs in the project root — auditing is read-only and writes only its
// own fragment under .humify/ — so all jobs share Dir = p.Root.
func (r SpawnRunner) Dispatch(p Plan) (Outcome, error) {
	prompts, err := writePrompts(p)
	out := Outcome{Runner: "spawn", Prompts: prompts}
	if err != nil || len(p.Pending) == 0 {
		return out, err
	}

	jobs := make([]spawn.Job, len(p.Pending))
	frag := make(map[string]string, len(p.Pending)) // area id → fragment path, for the success check
	for i, j := range p.Pending {
		jobs[i] = spawn.Job{ID: j.AreaID, Dir: p.Root, Prompt: RenderPrompt(j, p.Target)}
		frag[j.AreaID] = j.FragmentPath
	}

	res := spawn.Run(jobs,
		spawn.Config{AgentCmd: r.AgentCmd, Jobs: r.Jobs, Timeout: r.Timeout, Run: r.run},
		func(id string) bool { return fragmentDone(p.Root, frag[id], id) })
	out.Spawned, out.Succeeded, out.Failed = res.Spawned, res.Succeeded, res.Failed
	return out, nil
}
