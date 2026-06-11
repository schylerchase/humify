package main

import (
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	plan "github.com/schylerryan/humify/internal/humify/plan"
	hstate "github.com/schylerryan/humify/internal/humify/state"
)

// TestApplyUnsafeNonTTYDoesNotHang is the regression guard for the non-TTY confirm
// bypass. Before the fix, `apply --unsafe-permission --yes` printed "type yes" and
// blocked on fmt.Fscan(os.Stdin) whenever stdin was not a terminal — wedging every
// scripted, piped, CI, or agent-driven run. The fix prompts only when stdin is a
// TTY; a non-TTY proceeds on the two explicit flags already given.
//
// stdin here is a real pipe that is never written to: the fixed code must not read
// it (so it returns at once), while a regression would block on Fscan forever and
// trip the timeout. The agent (`cat >/dev/null`) is a no-op that exits 0, so the
// apply reaches its completion message.
func TestApplyUnsafeNonTTYDoesNotHang(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("apply spawns the agent via sh -c; POSIX-only")
	}
	root := t.TempDir()
	p := plan.Plan{
		Schema: hstate.Schema, Tool: "humify", Target: root,
		Items: []plan.Item{{
			ID: "HMF-001", Signal: "deep_nesting", Title: "Flatten deep nesting",
			AutomationSafety: "assisted", Applyable: false,
			AgentSpec: "test spec; the agent is a no-op",
		}},
	}
	if err := hstate.Save(root, hstate.PlanFile, p); err != nil {
		t.Fatalf("save plan: %v", err)
	}

	// An open pipe with no data: reading it blocks forever, so only a regressed
	// (non-TTY-blind) confirm path would hang here.
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	defer inR.Close()
	defer inW.Close()
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = inR, outW
	restore := func() { os.Stdin, os.Stdout = oldIn, oldOut }

	// run() in a goroutine so a hang trips the timeout instead of wedging the test.
	// Only run() and the channel send happen here — no t.* calls off the test goroutine.
	done := make(chan int, 1)
	go func() {
		done <- run([]string{"apply", "--target", "HMF-001",
			"--unsafe-permission", "--agent-cmd", "cat >/dev/null", "--yes", root})
	}()

	var code int
	select {
	case code = <-done:
	case <-time.After(20 * time.Second):
		restore()
		t.Fatal("apply hung on the non-TTY confirmation prompt — the TTY bypass regressed")
	}
	_ = outW.Close()
	restore()
	out, _ := io.ReadAll(outR)

	if code != exitOK {
		t.Fatalf("non-TTY unattended apply should exit 0, got %d:\n%s", code, out)
	}
	if !strings.Contains(string(out), "Agent completed") {
		t.Fatalf("the agent path did not run on a non-TTY stdin:\n%s", out)
	}
}
