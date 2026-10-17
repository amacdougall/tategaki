// Package kindle wraps Kindle framework services reached through LIPC.
//
// Pausing and restoring the Kindle UI happens in launch.sh, so that the shell
// trap can restore it even if this binary crashes.
package kindle

import (
	"bufio"
	"os/exec"
	"strings"
)

// PowerEvents streams com.lab126.powerd event lines (for example
// "goingToScreenSaver 2" or "outOfScreenSaver 1") from lipc-wait-event.
// The channel closes if lipc-wait-event exits.
func PowerEvents() (<-chan string, func(), error) {
	cmd := exec.Command("lipc-wait-event", "-m", "-s", "0", "com.lab126.powerd", "*")
	dieWithParent(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	ch := make(chan string, 16)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(out)
		for sc.Scan() {
			if line := strings.TrimSpace(sc.Text()); line != "" {
				ch <- line
			}
		}
		cmd.Wait()
	}()
	stop := func() { cmd.Process.Kill() }
	return ch, stop, nil
}

// ResetIdleTimer restarts powerd's screensaver countdown, as the stock UI
// does on every touch. Our touches never reach the paused framework, so
// without this the Kindle would sleep mid-reading after screenSaverTimeout.
// (preventScreenSaver is not used: it also blocks the power button.)
func ResetIdleTimer() error {
	return exec.Command("lipc-set-prop", "com.lab126.powerd", "touchScreenSaverTimeout", "1").Run()
}
