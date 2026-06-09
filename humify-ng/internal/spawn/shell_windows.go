//go:build windows

package spawn

import (
	"os/exec"
	"time"
)

// setProcGroup is a partial measure on Windows: a WaitDelay so Run cannot wedge
// if a descendant inherits a pipe, but no process-group/job-object teardown.
// Reliable whole-tree kill on Windows needs a Job Object, which is more involved;
// the autonomous driver's supported path is POSIX, where setProcGroup kills the
// agent's whole process group on timeout. On Windows a timed-out agent's
// descendants may survive — a documented limitation, not a silent one.
func setProcGroup(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
}
