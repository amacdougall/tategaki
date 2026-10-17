package main

// The Phase 0 bring-up "hello" mode (-hello): a test pattern that marks and
// logs every tap, redraws on wake, and exits when EXIT is tapped.

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/amacdougall/tategaki/reader/internal/display"
	"github.com/amacdougall/tategaki/reader/internal/input"
)

const (
	corner = 80
	marker = 24
)

var exitBox = image.Rect(screenW/2-200, screenH/2-100, screenW/2+200, screenH/2+100)

func runHello(s *session) error {
	pattern := filepath.Join(s.tmp, "pattern.png")
	dot := filepath.Join(s.tmp, "marker.png")
	if err := writePNG(pattern, testPattern()); err != nil {
		return err
	}
	if err := writePNG(dot, square(marker)); err != nil {
		return err
	}

	redraw := func(reason string) {
		d, err := s.fb.Image(pattern, 0, 0, display.WaveformGC16, true)
		if err != nil {
			log.Printf("draw pattern: %v", err)
			return
		}
		log.Printf("LATENCY full-screen GC16+flash image: %v (%s)", d, reason)
		s.fb.Text(3, 3, "TATEGAKI PHASE 0")
		s.fb.Text(8, 2, "Tap anywhere: a dot should appear under your finger")
		s.fb.Text(26, 4, "EXIT")
	}
	redraw("startup")

	return s.loop(func(t input.Touch) bool {
		log.Printf("TAP down=(%d,%d) up=(%d,%d) raw=(%d,%d) held=%v %s",
			t.X0, t.Y0, t.X1, t.Y1, t.RawX0, t.RawY0, t.Duration.Round(time.Millisecond), describe(t.X0, t.Y0))
		if image.Pt(t.X0, t.Y0).In(exitBox) {
			log.Printf("EXIT tapped")
			return true
		}
		x := clamp(t.X0-marker/2, 0, screenW-marker)
		y := clamp(t.Y0-marker/2, 0, screenH-marker)
		if d, err := s.fb.Image(dot, x, y, display.WaveformDU, false); err != nil {
			log.Printf("draw marker: %v", err)
		} else {
			log.Printf("LATENCY region %dx%d DU image: %v", marker, marker, d)
		}
		return false
	}, func() { redraw("wake") })
}

func describe(x, y int) string {
	switch {
	case x < corner && y < corner:
		return "[top-left corner]"
	case x >= screenW-corner && y < corner:
		return "[top-right corner]"
	case x < corner && y >= screenH-corner:
		return "[bottom-left corner]"
	case x >= screenW-corner && y >= screenH-corner:
		return "[bottom-right corner]"
	}
	return ""
}

// testPattern: white page, 100px grid, 4px border, black corner squares and
// an outlined EXIT box. The grid makes coordinate offsets visible in
// screenshots.
func testPattern() *image.Gray {
	img := image.NewGray(image.Rect(0, 0, screenW, screenH))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	grid := color.Gray{Y: 0xC0}
	for x := 0; x < screenW; x += 100 {
		fill(img, image.Rect(x, 0, x+1, screenH), grid)
	}
	for y := 0; y < screenH; y += 100 {
		fill(img, image.Rect(0, y, screenW, y+1), grid)
	}
	black := color.Gray{}
	for _, r := range []image.Rectangle{
		image.Rect(0, 0, screenW, 4), image.Rect(0, screenH-4, screenW, screenH),
		image.Rect(0, 0, 4, screenH), image.Rect(screenW-4, 0, screenW, screenH),
		image.Rect(0, 0, corner, corner), image.Rect(screenW-corner, 0, screenW, corner),
		image.Rect(0, screenH-corner, corner, screenH), image.Rect(screenW-corner, screenH-corner, screenW, screenH),
	} {
		fill(img, r, black)
	}
	for i := 0; i < 6; i++ {
		b := exitBox.Inset(i)
		fill(img, image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+1), black)
		fill(img, image.Rect(b.Min.X, b.Max.Y-1, b.Max.X, b.Max.Y), black)
		fill(img, image.Rect(b.Min.X, b.Min.Y, b.Min.X+1, b.Max.Y), black)
		fill(img, image.Rect(b.Max.X-1, b.Min.Y, b.Max.X, b.Max.Y), black)
	}
	return img
}

func square(n int) *image.Gray {
	img := image.NewGray(image.Rect(0, 0, n, n))
	fill(img, img.Bounds(), color.Gray{})
	return img
}

func fill(img *image.Gray, r image.Rectangle, c color.Gray) {
	draw.Draw(img, r, &image.Uniform{C: c}, image.Point{}, draw.Src)
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func clamp(v, lo, hi int) int {
	return max(lo, min(v, hi))
}
