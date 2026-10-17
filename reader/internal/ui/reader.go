package ui

import (
	"image"
	"log"
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
	start := time.Now()
	if err := a.Disp.PNGFile(path, image.Point{}, mode); err != nil {
		return err
	}
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

// Handle maps gestures to page turns and the menu. The next page is to the
// left in Japanese reading order. Taps on text will start a lookup in Phase 4;
// for now every tap below the top strip turns the page.
func (r *Reader) Handle(a *App, g input.Gesture) error {
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
