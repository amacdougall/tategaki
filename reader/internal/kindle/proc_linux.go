package kindle

import (
	"os/exec"
	"syscall"
)

// dieWithParent makes the kernel kill cmd if this process dies, so helpers
// like lipc-wait-event don't outlive a crashed reader.
func dieWithParent(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}
