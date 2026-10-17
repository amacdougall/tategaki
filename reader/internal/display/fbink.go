// Package display draws to the screen.
//
// The FBInk backend shells out to the FBInk CLI. Plan §9 Phase 0: if the CLI
// is too slow (over ~250 ms per page turn), switch to libfbink via cgo.
package display

import (
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// Waveform modes understood by FBInk's -W flag.
const (
	WaveformAuto = "AUTO"
	WaveformGC16 = "GC16" // full-quality grayscale
	WaveformGL16 = "GL16" // grayscale, less flashing
	WaveformDU   = "DU"   // fast, black/white only
)

// FBInk drives the screen through the fbink CLI binary.
type FBInk struct {
	Bin string
}

func (f *FBInk) run(args ...string) (time.Duration, error) {
	start := time.Now()
	out, err := exec.Command(f.Bin, append([]string{"-q"}, args...)...).CombinedOutput()
	d := time.Since(start)
	if err != nil {
		return d, fmt.Errorf("fbink %v: %w: %s", args, err, out)
	}
	return d, nil
}

// Image draws a PNG with its top-left corner at (x, y). flash requests a
// full black/white flash to clear ghosting.
func (f *FBInk) Image(path string, x, y int, waveform string, flash bool) (time.Duration, error) {
	args := []string{"-g", fmt.Sprintf("file=%s,x=%d,y=%d", path, x, y), "-W", waveform}
	if flash {
		args = append(args, "-f")
	}
	return f.run(args...)
}

// Text prints a line of FBInk's built-in bitmap font at text row `row`,
// centred horizontally. Only meant for diagnostics.
func (f *FBInk) Text(row int, scale int, s string) (time.Duration, error) {
	return f.run("-m", "-y", strconv.Itoa(row), "-S", strconv.Itoa(scale), s)
}

// Clear blanks the whole screen to white with a flashing refresh.
func (f *FBInk) Clear() (time.Duration, error) {
	return f.run("-k", "-f")
}
