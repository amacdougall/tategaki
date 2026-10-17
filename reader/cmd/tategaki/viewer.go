package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/amacdougall/tategaki/reader/internal/book"
	"github.com/amacdougall/tategaki/reader/internal/display"
	"github.com/amacdougall/tategaki/reader/internal/input"
)

// fullRefreshEvery: a flashing refresh clears e-ink ghosting (plan §8.3).
const fullRefreshEvery = 6

// newestBook returns the most recently modified .jbk in dir.
func newestBook(dir string) (string, error) {
	paths, _ := filepath.Glob(filepath.Join(dir, "*.jbk"))
	var best string
	var bestTime int64
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && st.ModTime().UnixNano() > bestTime {
			best, bestTime = p, st.ModTime().UnixNano()
		}
	}
	if best == "" {
		return "", fmt.Errorf("no .jbk files in %s", dir)
	}
	return best, nil
}

// runViewer is a minimal page viewer: tap the left half for the next page
// (Japanese order), the right half for the previous one, the top strip to exit.
func runViewer(s *session, dir, path string) error {
	if path == "" {
		var err error
		if path, err = newestBook(dir); err != nil {
			return err
		}
	}
	b, err := book.Open(path)
	if err != nil {
		return err
	}
	defer b.Close()
	log.Printf("opened %s: %q, %d pages", path, b.Manifest.Title, b.Manifest.PageCount)

	page, turns := 0, 0
	show := func(flash bool) {
		png, err := b.ExtractPage(page, s.tmp)
		if err != nil {
			log.Printf("page %d: %v", page, err)
			return
		}
		d, err := s.fb.Image(png, 0, 0, display.WaveformGC16, flash)
		if err != nil {
			log.Printf("draw page %d: %v", page, err)
			return
		}
		log.Printf("page %d drawn in %v (flash=%v)", page, d, flash)
	}
	show(true)

	topStrip := screenH / 10
	return s.loop(func(t input.Touch) bool {
		switch {
		case t.Y0 < topStrip:
			log.Printf("exit tapped")
			return true
		case t.X0 < screenW/2 && page < b.Manifest.PageCount-1:
			page++
		case t.X0 >= screenW/2 && page > 0:
			page--
		default:
			return false
		}
		turns++
		show(turns%fullRefreshEvery == 0)
		return false
	}, func() { show(true) })
}
