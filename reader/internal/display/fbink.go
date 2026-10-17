// Package display draws to the screen.
//
// The FBInk backend shells out to the FBInk CLI. Plan §9 Phase 0 measured it
// at 64–77 ms for a full-screen page and ~13 ms for a small region, well under
// the 250 ms budget, so there is no cgo build.
package display

import (
	"bufio"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

// FBInk drives the screen through the fbink CLI binary.
type FBInk struct {
	Bin    string
	Tmp    string // scratch directory (tmpfs) for UI images
	Width  int
	Height int
	seq    int
}

var _ Display = (*FBInk)(nil)

func (f *FBInk) run(args ...string) (time.Duration, error) {
	start := time.Now()
	out, err := exec.Command(f.Bin, append([]string{"-q"}, args...)...).CombinedOutput()
	d := time.Since(start)
	if err != nil {
		return d, fmt.Errorf("fbink %v: %w: %s", args, err, out)
	}
	return d, nil
}

func waveform(mode Mode) []string {
	switch mode {
	case Full:
		return []string{"-W", "GC16", "-f"}
	case Fast:
		return []string{"-W", "DU"}
	default:
		return []string{"-W", "GC16"}
	}
}

// File draws an image file FBInk can decode (PNG, PGM, ...) and reports how
// long FBInk took.
func (f *FBInk) File(path string, x, y int, mode Mode) (time.Duration, error) {
	return f.run(append([]string{"-g", fmt.Sprintf("file=%s,x=%d,y=%d", path, x, y)}, waveform(mode)...)...)
}

// Size is the screen size.
func (f *FBInk) Size() image.Point { return image.Pt(f.Width, f.Height) }

// PNGFile draws a PNG file.
func (f *FBInk) PNGFile(path string, at image.Point, mode Mode) error {
	_, err := f.File(path, at.X, at.Y, mode)
	return err
}

// Image writes img as a binary PGM (no compression to pay for on the
// device) and draws it.
func (f *FBInk) Image(img *image.Gray, at image.Point, mode Mode) error {
	f.seq = (f.seq + 1) % 4 // a few rotating names: FBInk reads the file synchronously
	path := filepath.Join(f.Tmp, "ui"+strconv.Itoa(f.seq)+".pgm")
	if err := writePGM(path, img); err != nil {
		return err
	}
	_, err := f.File(path, at.X, at.Y, mode)
	return err
}

func writePGM(path string, img *image.Gray) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(file, 1<<16)
	b := img.Bounds()
	fmt.Fprintf(w, "P5 %d %d 255\n", b.Dx(), b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		i := img.PixOffset(b.Min.X, y)
		w.Write(img.Pix[i : i+b.Dx()])
	}
	if err := w.Flush(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// Text prints a line of FBInk's built-in bitmap font at text row `row`,
// centred horizontally. Only for diagnostics (the hello mode).
func (f *FBInk) Text(row int, scale int, s string) (time.Duration, error) {
	return f.run("-m", "-y", strconv.Itoa(row), "-S", strconv.Itoa(scale), s)
}
