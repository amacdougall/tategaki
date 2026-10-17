package display

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"os"
	"sync"
)

// Sim is the simulator's framebuffer (plan §6.2). The sim HTTP server
// (internal/sim) serves it as a PNG.
type Sim struct {
	mu      sync.Mutex
	fb      *image.Gray
	version int
	changed chan struct{}
	// Log records every draw, for tests: "full 0,0 1236x1648".
	Log []string
}

var _ Display = (*Sim)(nil)

// NewSim returns a white framebuffer of the given size.
func NewSim(w, h int) *Sim {
	fb := image.NewGray(image.Rect(0, 0, w, h))
	for i := range fb.Pix {
		fb.Pix[i] = 0xff
	}
	return &Sim{fb: fb, changed: make(chan struct{})}
}

// Size is the screen size.
func (s *Sim) Size() image.Point { return s.fb.Bounds().Size() }

func (s *Sim) blit(img image.Image, at image.Point, mode Mode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := img.Bounds().Sub(img.Bounds().Min).Add(at)
	draw.Draw(s.fb, r, img, img.Bounds().Min, draw.Src)
	s.version++
	s.Log = append(s.Log, modeName(mode)+" "+r.String())
	close(s.changed)
	s.changed = make(chan struct{})
}

func modeName(m Mode) string {
	return [...]string{"full", "partial", "fast"}[m]
}

// Image draws img at a point.
func (s *Sim) Image(img *image.Gray, at image.Point, mode Mode) error {
	s.blit(img, at, mode)
	return nil
}

// PNGFile draws a PNG file at a point.
func (s *Sim) PNGFile(path string, at image.Point, mode Mode) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	s.blit(img, at, mode)
	return nil
}

// Snapshot returns the current framebuffer as PNG plus its version.
func (s *Sim) Snapshot() ([]byte, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var buf bytes.Buffer
	(&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, s.fb)
	return buf.Bytes(), s.version
}

// Version is the draw counter.
func (s *Sim) Version() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version
}

// Changed returns a channel closed at the next draw.
func (s *Sim) Changed() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.changed
}

// Frame returns a copy of the framebuffer.
func (s *Sim) Frame() *image.Gray {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := image.NewGray(s.fb.Rect)
	copy(cp.Pix, s.fb.Pix)
	return cp
}
