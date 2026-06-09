//go:build !windows

package spawn

import (
	"os/exec"
	"syscall"
	"time"
)

// setProcGroup makes the agent the leader of its own process group and rewires
// the context-timeout cancel to SIGKILL that WHOLE group, not just the direct
// `sh`. This is load-bearing for the barrier's invariant.
//
// exec.CommandContext's default cancel kills only the command's direct child.
// When --agent-cmd is anything that makes `sh -c` fork instead of exec-replace —
// a pipeline (`claude | tee`), an && chain, a wrapper script, a shell function —
// the real agent is a grandchild. Killing only `sh` orphans it: it reparents to
// init and keeps running, and an LLM agent's expected failure mode is to hang, so
// the timeout path is the designed path, not a corner case. An orphan that later
// writes its fragment/PLAN.md/SUMMARY.md into .humify/ AFTER spawn.Run's barrier
// would corrupt the disk-derived state the driver reconciles — resurrecting a
// Failed id, masking a stuck stage, or committing into a worktree the merge
// barrier already passed. Killing the process group makes the agent genuinely
// dead before Run returns. WaitDelay bounds Run so a stray descendant holding the
// stdin pipe cannot wedge it.
func setProcGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative pid targets the whole process group led by the agent.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 2 * time.Second
}
