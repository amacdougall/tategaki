// Package kindle wraps Kindle framework services reached through LIPC.
//
// Pausing and restoring the Kindle UI happens in launch.sh, so that the shell
// trap can restore it even if this binary crashes.
package kindle

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
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

// Dark mode is the system's hardware inversion, com.lab126.winmgr epdcMode
// (Y8 normal, Y8INV dark). winmgr is awesome, which launch.sh SIGSTOPs while
// the reader runs, so the property is set with awesome briefly resumed.
const (
	EPDCNormal = "Y8"
	EPDCDark   = "Y8INV"
)

// SetDarkMode switches the system dark mode.
func SetDarkMode(dark bool) error {
	mode := EPDCNormal
	if dark {
		mode = EPDCDark
	}
	exec.Command("killall", "-CONT", "awesome").Run()
	defer exec.Command("killall", "-STOP", "awesome").Run()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "lipc-set-prop", "com.lab126.winmgr", "epdcMode", mode).CombinedOutput(); err != nil {
		return fmt.Errorf("lipc-set-prop epdcMode %s: %v: %s", mode, err, out)
	}
	return nil
}
