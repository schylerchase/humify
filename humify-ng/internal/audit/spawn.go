package audit

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// SpawnRunner writes the auditor prompts and then runs an operator-supplied agent
// command once per pending area — capped concurrency, a per-agent timeout, and a
// barrier that waits for all of them before returning. After the barrier it
// re-derives which fragments actually appeared, so a crashed, hung (timed-out), or
// no-op agent is surfaced in Failed rather than silently passing. The gather/merge
// is still the separate `humify consolidate` stage.
type SpawnRunner struct {
	AgentCmd string        // operator-supplied; the prompt is delivered on stdin
	Jobs     int           // max concurrent agents (clamped to >=1)
	Timeout  time.Duration // per-agent wall-clock cap (clamped to a default if <=0)
	run      func(dir, agentCmd, stdin string, timeout time.Duration) error // injectable for tests
}

// DefaultAgentTimeout bounds a single agent run. An LLM agent's failure mode is to
// hang, not to terminate, so an unbounded wait could freeze the whole stage.
const DefaultAgentTimeout = 10 * time.Minute

// Name identifies the runner in structured output.
func (SpawnRunner) Name() string { return "spawn" }

// Dispatch writes the prompts, spawns one agent per pending area (≤ Jobs at once),
// waits for all, then reports how many produced a valid fragment.
func (r SpawnRunner) Dispatch(p Plan) (Outcome, error) {
	prompts, err := writePrompts(p)
	out := Outcome{Runner: "spawn", Prompts: prompts}
	if err != nil || len(p.Pending) == 0 {
		return out, err
	}

	runFn := r.run
	if runFn == nil {
		runFn = shellExec
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultAgentTimeout
	}
	jobs := r.Jobs
	if jobs < 1 {
		jobs = 1
	}

	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup
	for _, j := range p.Pending {
		wg.Add(1)
		go func(j Job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// A failed/timed-out run is not handled here: the post-barrier fragment
			// check below is the single source of truth for success, so a "success"
			// that wrote no valid fragment still lands in Failed.
			_ = runFn(p.Root, r.AgentCmd, RenderPrompt(j, p.Target), timeout)
		}(j)
	}
	wg.Wait() // barrier: every agent has finished (or timed out)

	out.Spawned = len(p.Pending)
	for _, j := range p.Pending {
		if fragmentDone(p.Root, j.FragmentPath, j.AreaID) {
			out.Succeeded++
		} else {
			out.Failed = append(out.Failed, j.AreaID)
		}
	}
	sort.Strings(out.Failed)
	return out, nil
}

// shellExec runs the agent command through the platform shell with the prompt on
// stdin, bounded by timeout. The prompt is NEVER interpolated into the command
// line — only piped on stdin — so a crafted prompt cannot inject shell. agentCmd
// itself is operator-supplied and trusted (the same model as execute's --test-cmd);
// do not wire it to a value read out of the target repository without sandboxing.
func shellExec(dir, agentCmd, stdin string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/c", agentCmd)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", agentCmd)
	}
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr // surface agent output to the operator
	return cmd.Run()
}
