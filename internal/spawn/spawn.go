// Package spawn is the one place an autonomous stage turns a set of agent jobs
// into produced artifacts. It runs an operator-supplied agent command once per
// job — each in the job's own working directory, with the job's prompt on stdin
// — under a concurrency cap and a per-agent wall-clock timeout, barriers on all
// of them, then reconciles success from the artifacts that actually appeared
// rather than from the agents' exit codes.
//
// It is deliberately stage-agnostic. The audit stage runs every auditor in the
// project root and counts a fragment that validates; the execute stage runs
// every executor in its own worktree and counts a commit on its branch. The
// shared part is exactly "run N (dir, prompt) pairs, capped + timed + barriered";
// the per-stage part is the job list and the success predicate, both supplied by
// the caller. Keeping spawn behind this seam is what lets the deterministic
// planning/barrier/merge code stay fixed while only the spawn strategy varies.
package spawn

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

// Job is one agent invocation: run the agent command in Dir with Prompt on
// stdin. ID names the unit (area/slice) for reporting and for the success check.
type Job struct {
	ID     string
	Dir    string
	Prompt string
}

// Result is the post-barrier reconciliation. Succeeded + len(Failed) always
// equals Spawned: every spawned job is judged by the success predicate, never by
// the agent's exit status.
type Result struct {
	Spawned   int      `json:"spawned"`
	Succeeded int      `json:"succeeded"`
	Failed    []string `json:"failed,omitempty"` // ids whose predicate was false after the barrier, sorted
}

// RunFunc executes one agent: command agentCmd in dir, prompt on stdin, bounded
// by timeout. The default is ShellExec; tests inject a fake.
type RunFunc func(dir, agentCmd, stdin string, timeout time.Duration) error

// DefaultTimeout bounds a single agent run. An LLM agent's failure mode is to
// hang, not to terminate, so an unbounded wait could freeze the whole stage.
const DefaultTimeout = 10 * time.Minute

// Config bounds a Run. Jobs<1 clamps to 1; Timeout<=0 clamps to DefaultTimeout;
// a nil Run uses ShellExec.
type Config struct {
	AgentCmd string
	Jobs     int
	Timeout  time.Duration
	Run      RunFunc
}

// Run spawns every job (≤ cfg.Jobs at once), waits for all of them (the
// barrier), then calls succeeded(id) per job to reconcile. A run error is
// intentionally ignored: succeeded is the single source of truth, so an agent
// that "exited 0" but produced no artifact still lands in Failed, and a crashed
// or timed-out agent that nonetheless left a valid artifact still counts. Run
// never returns an error — a bad agent is data (a Failed id), not a stage abort.
func Run(jobs []Job, cfg Config, succeeded func(id string) bool) Result {
	if len(jobs) == 0 {
		return Result{}
	}
	runFn := cfg.Run
	if runFn == nil {
		runFn = ShellExec
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	limit := cfg.Jobs
	if limit < 1 {
		limit = 1
	}

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	for _, j := range jobs {
		wg.Add(1)
		go func(j Job) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// The error is deliberately dropped: the post-barrier success check
			// below is the single source of truth, so a "success" that wrote no
			// valid artifact still lands in Failed and a crash that wrote one counts.
			_ = runFn(j.Dir, cfg.AgentCmd, j.Prompt, timeout)
		}(j)
	}
	wg.Wait() // barrier: every agent has finished or timed out

	res := Result{Spawned: len(jobs)}
	for _, j := range jobs {
		if succeeded(j.ID) {
			res.Succeeded++
		} else {
			res.Failed = append(res.Failed, j.ID)
		}
	}
	sort.Strings(res.Failed)
	return res
}

// ShellExec runs agentCmd through the platform shell with prompt on stdin,
// bounded by timeout. The prompt is NEVER interpolated into the command line —
// only piped on stdin — so a crafted prompt cannot inject shell. agentCmd itself
// is operator-supplied and trusted (the same model as execute's --test-cmd).
//
// SECURITY: once an autonomous driver runs this through source-modifying stages,
// that trust covers an agent with write + commit power over the repo. A single
// operator --agent-cmd serves every role, so a read-only auditor inherits the
// same powers as an executor. Never wire agentCmd to a value read out of the
// target repository without sandboxing — that would be a command-injection path
// into an agent that can rewrite the codebase.
func ShellExec(dir, agentCmd, stdin string, timeout time.Duration) error {
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
	// Run the agent in its own process group and kill the WHOLE group on timeout —
	// otherwise a hung agent reached through a forking `sh -c` (pipeline, wrapper,
	// chain) is only orphaned, and its late write would corrupt the post-barrier
	// state. Platform-specific; a no-op group kill on Windows (documented there).
	setProcGroup(cmd)
	return cmd.Run()
}
