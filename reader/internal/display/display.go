package display

import (
	"image"
)

// Mode is how a drawn region is refreshed on e-ink.
type Mode int

const (
	// Full flashes black to clear ghosting (GC16 + flash).
	Full Mode = iota
	// Partial is a grayscale update without the flash (GC16).
	Partial
	// Fast is a quick black-and-white update for small UI changes (DU).
	Fast
)

// Display is the screen, behind an interface so the reader can run against the
// desktop simulator (plan §6.2).
type Display interface {
	Size() image.Point
	// Image draws img with its top-left corner at the given point.
	Image(img *image.Gray, at image.Point, mode Mode) error
	// PNGFile draws a PNG file (a book page) at the given point.
	PNGFile(path string, at image.Point, mode Mode) error
}
