package ui

import (
	"image"
	"log"
	"os"
	"time"

	"github.com/amacdougall/tategaki/reader/internal/book"
	"github.com/amacdougall/tategaki/reader/internal/display"
	"github.com/amacdougall/tategaki/reader/internal/input"
)

// Reader shows one page at a time (plan §8.1, §8.2).
type Reader struct {
	id   string
	book *book.Book
	page int

	texts    map[int][]rune   // chapter text cache
	popup    *Popup           // open lookup, if any
	img      *image.Gray      // decoded current page (for restoring regions)
	imgPage  int              // page img / imgReady belong to
	imgReady chan *image.Gray // background decode in progress
	lastPath string           // extracted PNG of the current page
}

// topStrip is the share of the screen height that opens the menu.
const topStripPct = 10

// Draw shows the current page: a partial refresh on page turns, with a full
// flashing refresh every full_refresh_every turns (plan §8.3).
func (r *Reader) Draw(a *App) error {
	path, err := r.book.ExtractPage(r.page, a.Tmp)
	if err != nil {
		return err
	}
	mode := display.Partial
	a.pageTurns++
	if a.fullNext || a.pageTurns >= a.Dev.FullRefreshEvery {
		mode, a.pageTurns, a.fullNext = display.Full, 0, false
	}
	r.popup = nil // a full page draw replaces any popup
	start := time.Now()
	if err := a.Disp.PNGFile(path, image.Point{}, mode); err != nil {
		return err
	}
	if r.lastPath != "" && r.lastPath != path {
		os.Remove(r.lastPath)
	}
	r.prefetch(path)
	log.Printf("page %d/%d drawn in %v (mode %d)", r.page+1, r.book.Manifest.PageCount, time.Since(start).Round(time.Millisecond), mode)
	return nil
}

func (r *Reader) turn(a *App, delta int) error {
	next := r.page + delta
	if next < 0 || next >= r.book.Manifest.PageCount {
		return nil
	}
	r.page = next
	if err := r.Draw(a); err != nil {
		return err
	}
	return r.save(a)
}

// save stores the position atomically on every page turn (plan §8.4).
func (r *Reader) save(a *App) error {
	pos, err := r.book.PositionOf(r.page)
	if err != nil {
		return err
	}
	return a.State.Set(r.id, pos)
}

// Goto jumps to a page.
func (r *Reader) Goto(a *App, page int) error {
	r.page = min(max(page, 0), r.book.Manifest.PageCount-1)
	a.fullNext = true
	a.Show(r)
	return r.save(a)
}

// Handle maps gestures to lookups, page turns and the menu (plan §8.2). A tap
// on a lookupable character looks it up; other taps turn the page, the next
// page being to the left in Japanese reading order.
func (r *Reader) Handle(a *App, g input.Gesture) error {
	if r.popup != nil {
		return r.handlePopup(a, g)
	}
	// Characters win over zones: the top strip overlaps the first characters
	// of every column, so the menu opens from the margin or gaps above them.
	if g.Kind == input.Tap || g.Kind == input.Hold {
		if pl, ch, err := r.pageLayout(); err == nil {
			if gl, ok := hit(pl.Glyphs, g.X, g.Y); ok {
				if text, err := r.chapterText(ch); err == nil && gl[0] < len(text) && lookupable(text[gl[0]]) {
					return r.lookupAt(a, ch, gl)
				}
			}
		}
	}
	switch g.Kind {
	case input.SwipeRight: // pulls the next page in from the left
		return r.turn(a, 1)
	case input.SwipeLeft:
		return r.turn(a, -1)
	case input.Tap, input.Hold:
		if g.Y < a.H*topStripPct/100 {
			a.Show(&Menu{reader: r})
			return nil
		}
		if g.X < a.W/2 {
			return r.turn(a, 1)
		}
		return r.turn(a, -1)
	}
	return nil
}

// Close releases the book.
func (r *Reader) Close() { r.book.Close() }

// Library closes the book and returns to the library.
func (r *Reader) Library(a *App) {
	r.Close()
	a.Show(NewLibrary(a))
}
