package spawn

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// producer returns a run fake that "succeeds" by recording each job's id in a
// concurrent set, except ids in failIDs (which error and record nothing). The
// id is recovered from the prompt — exactly the cue a real agent keys on — so
// the fake exercises the same stdin path production uses. The companion made()
// predicate is what Run reconciles against, mirroring "did the artifact appear".
func producer(failIDs ...string) (RunFunc, func(id string) bool) {
	fail := map[string]bool{}
	for _, id := range failIDs {
		fail[id] = true
	}
	var done sync.Map
	run := func(_, _, stdin string, _ time.Duration) error {
		id := stdin // tests pass the bare id as the prompt
		if fail[id] {
			return os.ErrInvalid // a crashed/no-op agent
		}
		done.Store(id, true)
		return nil
	}
	made := func(id string) bool { _, ok := done.Load(id); return ok }
	return run, made
}

func jobs(ids ...string) []Job {
	js := make([]Job, len(ids))
	for i, id := range ids {
		js[i] = Job{ID: id, Dir: ".", Prompt: id}
	}
	return js
}

func TestRunAllSucceed(t *testing.T) {
	run, made := producer()
	got := Run(jobs("alpha", "beta", "gamma"), Config{AgentCmd: "x", Jobs: 2, Run: run}, made)
	if got.Spawned != 3 || got.Succeeded != 3 || len(got.Failed) != 0 {
		t.Fatalf("got %+v; want Spawned=3 Succeeded=3 Failed=none", got)
	}
}

func TestRunNoneSucceed(t *testing.T) {
	// Agent "succeeds" (no error) but produces nothing: the predicate is the
	// single source of truth, so every job must land in Failed, sorted.
	run := func(_, _, _ string, _ time.Duration) error { return nil }
	got := Run(jobs("beta", "alpha"), Config{AgentCmd: "x", Jobs: 4, Run: run}, func(string) bool { return false })
	if got.Succeeded != 0 || !reflect.DeepEqual(got.Failed, []string{"alpha", "beta"}) {
		t.Fatalf("got %+v; want Succeeded=0 Failed=[alpha beta]", got)
	}
}

func TestRunMixed(t *testing.T) {
	run, made := producer("beta")
	got := Run(jobs("alpha", "beta", "gamma"), Config{AgentCmd: "x", Jobs: 3, Run: run}, made)
	if got.Succeeded != 2 || !reflect.DeepEqual(got.Failed, []string{"beta"}) {
		t.Fatalf("got %+v; want Succeeded=2 Failed=[beta] — one bad agent must not abort the barrier", got)
	}
}

func TestRunEmpty(t *testing.T) {
	var calls int32
	run := func(_, _, _ string, _ time.Duration) error { atomic.AddInt32(&calls, 1); return nil }
	got := Run(nil, Config{AgentCmd: "x", Jobs: 4, Run: run}, func(string) bool { return true })
	if got.Spawned != 0 || got.Succeeded != 0 || len(got.Failed) != 0 {
		t.Fatalf("empty job set must spawn nothing; got %+v", got)
	}
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("run called %d times for no jobs; want 0", n)
	}
}

// TestPerJobDir proves the new capability over audit's single-root runner: each
// job runs in ITS OWN directory (execute's executors live in separate worktrees).
// The fake records the dir it was handed per id; we assert the mapping is exact.
func TestPerJobDir(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]string{}
	run := func(dir, _, stdin string, _ time.Duration) error {
		mu.Lock()
		seen[stdin] = dir
		mu.Unlock()
		return nil
	}
	js := []Job{{ID: "a", Dir: "/wt/a", Prompt: "a"}, {ID: "b", Dir: "/wt/b", Prompt: "b"}}
	Run(js, Config{AgentCmd: "x", Jobs: 2, Run: run}, func(string) bool { return true })
	if seen["a"] != "/wt/a" || seen["b"] != "/wt/b" {
		t.Fatalf("per-job dir not honored: %v", seen)
	}
}

// TestConcurrencyCap forces real overlap: every fake blocks until released, so
// in-flight piles up. A correct semaphore admits exactly Jobs at once, and the
// reader proves the cap was reached, not just respected. Run executes in a
// goroutine so a select+timeout turns a barrier hang into a failed assertion.
func TestConcurrencyCap(t *testing.T) {
	const limit = 2
	js := jobs("a1", "a2", "a3", "a4", "a5") // 5 > limit
	arrived := make(chan struct{}, len(js))
	release := make(chan struct{})
	var mu sync.Mutex
	inflight, maxSeen := 0, 0
	run := func(_, _, _ string, _ time.Duration) error {
		mu.Lock()
		inflight++
		if inflight > maxSeen {
			maxSeen = inflight
		}
		mu.Unlock()
		arrived <- struct{}{}
		<-release
		mu.Lock()
		inflight--
		mu.Unlock()
		return nil
	}
	done := make(chan Result, 1)
	go func() {
		done <- Run(js, Config{AgentCmd: "x", Jobs: limit, Run: run}, func(string) bool { return true })
	}()

	for i := 0; i < limit; i++ {
		<-arrived // limit goroutines now concurrently blocked → cap reached
	}
	time.Sleep(50 * time.Millisecond) // let any (buggy) un-capped extra arrive
	mu.Lock()
	peak := maxSeen
	mu.Unlock()
	close(release)

	select {
	case got := <-done:
		if got.Succeeded != len(js) {
			t.Fatalf("Succeeded=%d; want %d", got.Succeeded, len(js))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not finish after release — barrier hang")
	}
	if peak != limit {
		t.Fatalf("max in-flight = %d; want exactly %d (cap reached, not exceeded)", peak, limit)
	}
}

// TestShellExecTimeout proves the real (non-injected) path enforces the per-agent
// wall-clock cap: a hung agent is killed at the deadline and returns an error.
func TestShellExecTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh/sleep")
	}
	start := time.Now()
	if err := ShellExec(t.TempDir(), "sleep 5", "", 80*time.Millisecond); err == nil {
		t.Fatal("expected a deadline error from a timed-out agent")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("ShellExec waited %v; the timeout was not enforced", elapsed)
	}
}

// TestShellExecKillsProcessTree proves the timeout SIGKILLs the agent's whole
// process group, not just the direct `sh`. The command backgrounds a child that
// would write a sentinel after the deadline, then `wait`s — so `sh` forks rather
// than exec-replaces and the child is a separate process (the realistic
// pipeline/wrapper case). With a correct group kill the child dies before it can
// write; without it the orphan survives and the sentinel appears.
func TestShellExecKillsProcessTree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group kill is POSIX-only")
	}
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "late.txt")
	cmd := "(sleep 1; echo late > " + sentinel + ") & wait"
	_ = ShellExec(dir, cmd, "", 150*time.Millisecond)
	time.Sleep(1300 * time.Millisecond) // past when a surviving orphan would have written
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("a backgrounded agent child survived the timeout — the process group was not killed")
	}
}

// TestShellExecStdinAndDir proves the prompt is delivered on stdin (never the
// command line — injection-safe) and the agent runs in the job's directory.
func TestShellExecStdinAndDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	dir := t.TempDir()
	const msg = "the-prompt-on-stdin"
	if err := ShellExec(dir, "cat > out.txt", msg, 5*time.Second); err != nil {
		t.Fatalf("ShellExec: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil {
		t.Fatalf("read out.txt (cwd not set to dir?): %v", err)
	}
	if string(got) != msg {
		t.Fatalf("stdin not piped: got %q want %q", got, msg)
	}
}
