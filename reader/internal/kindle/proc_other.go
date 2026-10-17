//go:build !linux

package kindle

import "os/exec"

func dieWithParent(cmd *exec.Cmd) {}
