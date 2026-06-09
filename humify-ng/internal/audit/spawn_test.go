package audit

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"humify-ng/internal/layout"
)

// testPlan builds a Plan rooted in a fresh temp dir with one pending Job per id.
// Each Job carries the canonical fragment + prompt paths so the post-barrier
// fragmentDone reconciliation (and writePrompts) operate exactly as in production.
func testPlan(t *testing.T, ids ...string) Plan {
	t.Helper()
	p := Plan{Root: t.TempDir(), Target: "src", Total: len(ids)}
	for i, id := range ids {
		p.Pending = append(p.Pending, Job{
			AreaID:       id,
			Kind:         "dir",
			Root:         id,
			Files:        []string{id + "/main.go"},
			Wave:         i,
			FragmentPath: layout.AreaFragmentRel(id),
			PromptPath:   promptPath(id),
		})
	}
	return p
}

// fragmentWriter returns a run fake that writes a valid fragment for whichever
// pending job's prompt it received — matched by the unique FragmentPath embedded
// in the prompt (stdin), exactly the cue a real agent keys on. Ids in failIDs
// instead return an error and write nothing, simulating a crashed/no-op agent;
// the post-barrier check must then count them Failed.
func fragmentWriter(p Plan, failIDs ...string) func(string, string, string, time.Duration) error {
	fail := map[string]bool{}
	for _, id := range failIDs {
		fail[id] = true
	}
	return func(dir, _ /*agentCmd*/, stdin string, _ time.Duration) error {
		for _, j := range p.Pending {
			if !strings.Contains(stdin, j.FragmentPath) {
				continue
			}
			if fail[j.AreaID] {
				return errors.New("agent failed for " + j.AreaID)
			}
			full := filepath.Join(dir, j.FragmentPath)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return err
			}
			body := `{"area_id":"` + j.AreaID + `","findings":[]}`
			return os.WriteFile(full, []byte(body), 0o644)
		}
		return nil
	}
}

// TestWritePromptsRejectsEscape locks in the security fix: an area id with ".."
// segments (only reachable via a hand-edited intel/areas.json — slugify forbids
// it in generated ids) must be rejected by the root gate on the WRITE path, just
// as the read paths reject it, and must not truncate a file outside the project.
func TestWritePromptsRejectsEscape(t *testing.T) {
	root := t.TempDir()
	evilID := "../../../../tmp/humify-escape-evil"
	p := Plan{Root: root, Target: "src", Pending: []Job{{
		AreaID:       evilID,
		PromptPath:   promptPath(evilID),
		FragmentPath: layout.AreaFragmentRel("x"),
	}}}
	_, err := writePrompts(p)
	if err == nil || !strings.Contains(err.Error(), "escapes project root") {
		t.Fatalf("writePrompts err = %v; want a root-escape rejection (the gate must fire, not a stray ENOENT)", err)
	}
	outside := filepath.Join(filepath.Dir(root), "tmp", "humify-escape-evil.prompt.md")
	if _, statErr := os.Stat(outside); statErr == nil {
		os.Remove(outside)
		t.Fatalf("escaping prompt was written outside root at %s", outside)
	}
}

func TestSpawnAllSucceed(t *testing.T) {
	p := testPlan(t, "alpha", "beta", "gamma")
	r := SpawnRunner{AgentCmd: "x", Jobs: 2, run: fragmentWriter(p)}
	out, err := r.Dispatch(p)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if out.Spawned != 3 || out.Succeeded != 3 || len(out.Failed) != 0 {
		t.Fatalf("got Spawned=%d Succeeded=%d Failed=%v; want 3/3/none",
			out.Spawned, out.Succeeded, out.Failed)
	}
	if len(out.Prompts) != 3 {
		t.Fatalf("Prompts=%v; want 3 written", out.Prompts)
	}
}

func TestSpawnNoFragments(t *testing.T) {
	p := testPlan(t, "alpha", "beta")
	// Agent "succeeds" (no error) but writes nothing: the fragment check is the
	// single source of truth, so both areas must land in Failed.
	r := SpawnRunner{AgentCmd: "x", Jobs: 4, run: func(_, _, _ string, _ time.Duration) error { return nil }}
	out, _ := r.Dispatch(p)
	if out.Spawned != 2 || out.Succeeded != 0 {
		t.Fatalf("got Spawned=%d Succeeded=%d; want 2/0", out.Spawned, out.Succeeded)
	}
	if want := []string{"alpha", "beta"}; !reflect.DeepEqual(out.Failed, want) {
		t.Fatalf("Failed=%v; want %v (sorted)", out.Failed, want)
	}
}

func TestSpawnOneErrorsRestSucceed(t *testing.T) {
	p := testPlan(t, "alpha", "beta", "gamma")
	r := SpawnRunner{AgentCmd: "x", Jobs: 3, run: fragmentWriter(p, "beta")}
	out, _ := r.Dispatch(p)
	if out.Succeeded != 2 {
		t.Fatalf("Succeeded=%d; want 2", out.Succeeded)
	}
	if !reflect.DeepEqual(out.Failed, []string{"beta"}) {
		t.Fatalf("Failed=%v; want [beta] — one bad agent must not abort the barrier", out.Failed)
	}
}

func TestSpawnEmptyPlan(t *testing.T) {
	p := testPlan(t) // nothing pending
	var calls int32
	r := SpawnRunner{AgentCmd: "x", Jobs: 4, run: func(_, _, _ string, _ time.Duration) error {
		atomic.AddInt32(&calls, 1)
		return nil
	}}
	out, err := r.Dispatch(p)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if out.Spawned != 0 || out.Succeeded != 0 || len(out.Failed) != 0 {
		t.Fatalf("empty plan must spawn nothing; got %+v", out)
	}
	if n := atomic.LoadInt32(&calls); n != 0 {
		t.Fatalf("run called %d times for an empty plan; want 0", n)
	}
}

// TestSpawnConcurrencyCap forces real overlap: every fake blocks until released,
// so the in-flight count actually piles up. With a correct semaphore exactly Jobs
// run at once — never more — and the reader proves the cap was *reached*, not just
// respected. Dispatch runs in a goroutine and a select+timeout turns a barrier
// hang into a failed assertion instead of a frozen test suite.
func TestSpawnConcurrencyCap(t *testing.T) {
	const jobs = 2
	p := testPlan(t, "a1", "a2", "a3", "a4", "a5") // pending (5) > jobs (2)
	arrived := make(chan struct{}, len(p.Pending)) // buffered: sends never block
	release := make(chan struct{})
	var mu sync.Mutex
	inflight, maxSeen := 0, 0
	write := fragmentWriter(p)
	fake := func(dir, agentCmd, stdin string, to time.Duration) error {
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
		return write(dir, agentCmd, stdin, to)
	}
	r := SpawnRunner{AgentCmd: "x", Jobs: jobs, run: fake}
	done := make(chan Outcome, 1)
	go func() { o, _ := r.Dispatch(p); done <- o }()

	for i := 0; i < jobs; i++ {
		<-arrived // jobs goroutines are now concurrently blocked → cap reached
	}
	// Let any (incorrectly) un-capped extra goroutine arrive before snapshotting.
	// A correct semaphore admits none; this turns a cap bug into a clean failure.
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	peak := maxSeen
	mu.Unlock()
	close(release)

	select {
	case out := <-done:
		if out.Succeeded != len(p.Pending) {
			t.Fatalf("Succeeded=%d; want %d", out.Succeeded, len(p.Pending))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Dispatch did not finish after release — barrier hang")
	}
	if peak != jobs {
		t.Fatalf("max in-flight = %d; want exactly %d (semaphore cap reached, not exceeded)", peak, jobs)
	}
}

// The low-level shell behaviors (per-agent timeout kill, stdin delivery, cwd) now
// live in internal/spawn's TestShellExec* — SpawnRunner delegates to spawn.Run,
// so those are tested once at the primitive rather than re-proven through every
// adapter. The cap test below stays here to prove the adapter wires concurrency
// through correctly.
