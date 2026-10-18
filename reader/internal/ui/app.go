// Package ui is the reader's screens: library, page view, menu, table of
// contents, go-to-page and book info (plan §8).
package ui

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/amacdougall/tategaki/reader/internal/anki"
	"github.com/amacdougall/tategaki/reader/internal/book"
	"github.com/amacdougall/tategaki/reader/internal/config"
	"github.com/amacdougall/tategaki/reader/internal/dict"
	"github.com/amacdougall/tategaki/reader/internal/display"
	"github.com/amacdougall/tategaki/reader/internal/input"
	"github.com/amacdougall/tategaki/reader/internal/kindle"
	"github.com/amacdougall/tategaki/reader/internal/text"
)

// Screen is one full-screen state of the app.
type Screen interface {
	// Draw renders the whole screen.
	Draw(a *App) error
	// Handle reacts to a gesture.
	Handle(a *App, g input.Gesture) error
}

// App holds the reader's shared state.
type App struct {
	Disp   display.Display
	Fonts  *text.Fonts
	Paths  config.Paths
	Dev    config.Device
	State  *config.StateFile
	Tmp    string
	W, H   int
	screen Screen
	quit   bool
	// pageTurns counts partial page refreshes since the last full one.
	pageTurns int
	// fullNext forces the next page draw to flash (after a UI screen).
	fullNext bool
	// DictPaths are tried in order when the dictionary is first needed.
	DictPaths []string
	dictDB    *dict.DB
	dictErr   error
	// Dark is the system dark mode; SetDark changes it (nil in the simulator,
	// where the flag is only remembered).
	Dark    bool
	SetDark func(bool) error
	// Light is the frontlight step; SetLight changes it (nil in the
	// simulator). Dark mode pins it to Dev.DarkLightLevel.
	Light    int
	SetLight func(int) error
	// Queue is the Anki queue (plan §7.7).
	Queue *anki.Queue
	// AnkiWords are the words already in Anki at the last deploy.
	AnkiWords map[string]bool
	// bookID is the book opened most recently this session (for the sleep
	// cover).
	bookID  string
	updates chan func()
}

// Updates delivers work posted from background goroutines; the event loop
// runs it, so screens are only touched from one goroutine.
func (a *App) Updates() <-chan func() { return a.updates }

// Post schedules f on the event loop.
func (a *App) Post(f func()) { a.updates <- f }

// Dict opens the dictionary on first use.
func (a *App) Dict() (*dict.DB, error) {
	if a.dictDB != nil || a.dictErr != nil {
		return a.dictDB, a.dictErr
	}
	paths := append([]string{filepath.Join(a.Paths.Root, "jmdict.sqlite")}, a.DictPaths...)
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			start := time.Now()
			a.dictDB, a.dictErr = dict.Open(p)
			log.Printf("dictionary %s opened in %v (err %v)", p, time.Since(start).Round(time.Millisecond), a.dictErr)
			return a.dictDB, a.dictErr
		}
	}
	a.dictErr = fmt.Errorf("no jmdict.sqlite (tried %v); run `tategaki deploy`", paths)
	return nil, a.dictErr
}

// New creates the app; Start shows the library.
func New(disp display.Display, fonts *text.Fonts, paths config.Paths, tmp string) *App {
	size := disp.Size()
	a := &App{Disp: disp, Fonts: fonts, Paths: paths, Tmp: tmp, W: size.X, H: size.Y, State: config.OpenState(paths),
		updates: make(chan func(), 16)}
	a.reloadConfig()
	a.Light = a.Dev.LightLevel
	q, err := anki.OpenQueue(filepath.Join(paths.Root, "anki-queue.jsonl"))
	if err != nil {
		log.Printf("anki queue: %v", err)
	}
	a.Queue = q
	return a
}

func (a *App) reloadConfig() {
	dev, err := config.LoadDevice(a.Paths)
	if err != nil {
		log.Printf("config.json: %v (using defaults)", err)
	}
	if dev.FullRefreshEvery < 1 {
		dev.FullRefreshEvery = 1
	}
	dev.LightLevel = clampLight(dev.LightLevel)
	dev.DarkLightLevel = clampLight(dev.DarkLightLevel)
	a.Dev = dev
	words, err := anki.LoadWords(a.Paths.AnkiWords())
	if err != nil {
		log.Printf("anki words: %v", err)
	}
	a.AnkiWords = words
}

// Show switches to a screen and draws it.
func (a *App) Show(s Screen) {
	a.screen = s
	a.redraw()
}

func (a *App) redraw() {
	if err := a.screen.Draw(a); err != nil {
		log.Printf("draw %T: %v", a.screen, err)
	}
}

// Start resumes the most recently read book at its saved place, or opens the
// library if there is none (or it has been removed). Cards mined while Anki
// was unreachable are sent in the background.
func (a *App) Start() {
	if a.Dark {
		a.applyLight(a.Dev.DarkLightLevel)
	}
	if a.Queue.Pending() > 0 {
		a.FlushAnki(nil)
	}
	if id := a.State.State.LastBook; id != "" {
		if _, err := os.Stat(a.Paths.Book(id)); err == nil {
			if err := a.OpenBook(id); err == nil {
				return
			} else {
				log.Printf("resume %s: %v", id, err)
			}
		}
	}
	a.Show(NewLibrary(a))
}

// Quit ends the event loop.
func (a *App) Quit() { a.quit = true }

// Done reports whether the app asked to exit.
func (a *App) Done() bool { return a.quit }

// Gesture dispatches a gesture to the current screen.
func (a *App) Gesture(g input.Gesture) {
	start, screen := time.Now(), a.screen
	if err := screen.Handle(a, g); err != nil {
		log.Printf("%T %v: %v", screen, g.Kind, err)
	}
	log.Printf("%v at (%d,%d) on %T handled in %v", g.Kind, g.X, g.Y, screen, time.Since(start).Round(time.Millisecond))
}

// ToggleDark flips the system dark mode and redraws with a full refresh.
func (a *App) ToggleDark() error {
	if a.SetDark != nil {
		if err := a.SetDark(!a.Dark); err != nil {
			return err
		}
	}
	a.Dark = !a.Dark
	log.Printf("dark mode %v", a.Dark)
	if a.Dark {
		// Remember the light-mode step to restore on the way back.
		if err := a.State.SetLightLevel(a.Light); err != nil {
			log.Printf("state: %v", err)
		}
		a.applyLight(a.Dev.DarkLightLevel)
	} else {
		n := a.Dev.LightLevel
		if p := a.State.State.LightLevel; p != nil {
			n = *p
		}
		a.applyLight(n)
	}
	return nil
}

// ToggleLight switches the frontlight between off and Dev.LightLevel. It does
// nothing in dark mode, where the level is fixed.
func (a *App) ToggleLight() error {
	if a.Dark {
		return nil
	}
	n := a.Dev.LightLevel
	if a.Light > 0 {
		n = 0
	}
	if err := a.setLight(n); err != nil {
		return err
	}
	return a.State.SetLightLevel(n)
}

// applyLight sets the frontlight if it differs, logging failures: the light
// follows dark mode, and a failure there shouldn't undo the mode change.
func (a *App) applyLight(n int) {
	if n == a.Light {
		return
	}
	if err := a.setLight(n); err != nil {
		log.Printf("frontlight: %v", err)
	}
}

func (a *App) setLight(n int) error {
	if a.SetLight != nil {
		if err := a.SetLight(n); err != nil {
			return err
		}
	}
	a.Light = n
	log.Printf("frontlight %d", n)
	return nil
}

func clampLight(n int) int { return min(max(n, 0), kindle.MaxFrontlight) }

// Refresh redraws the current screen without forcing a flash. Used shortly
// after launch: the Kindle UI can still be finishing a redraw when it is
// paused (the home screen after a previous exit, or the Library launcher's
// "Opening…" handling), and that late drawing lands on top of ours.
func (a *App) Refresh() { a.redraw() }

// Wake redraws after the device leaves the screensaver.
func (a *App) Wake() {
	a.fullNext = true
	a.redraw()
}

// --- drawing helpers -------------------------------------------------------

var (
	black = color.Gray{0}
	white = color.Gray{255}
	gray  = color.Gray{0x88}
	light = color.Gray{0xdd}
)

// Text sizes in device pixels (300 ppi): roughly 14, 11 and 9 pt.
const (
	sizeTitle = 58
	sizeBody  = 46
	sizeSmall = 38
)

func (a *App) canvas() *image.Gray {
	img := image.NewGray(image.Rect(0, 0, a.W, a.H))
	fill(img, img.Bounds(), white)
	return img
}

func fill(img *image.Gray, r image.Rectangle, c color.Gray) {
	draw.Draw(img, r, image.NewUniform(c), image.Point{}, draw.Src)
}

func frame(img *image.Gray, r image.Rectangle, w int, c color.Gray) {
	fill(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+w), c)
	fill(img, image.Rect(r.Min.X, r.Max.Y-w, r.Max.X, r.Max.Y), c)
	fill(img, image.Rect(r.Min.X, r.Min.Y, r.Min.X+w, r.Max.Y), c)
	fill(img, image.Rect(r.Max.X-w, r.Min.Y, r.Max.X, r.Max.Y), c)
}

// label draws one line of text vertically centred in r, left-aligned with
// padding, or centred when center is set. Text that doesn't fit is
// ellipsised.
func (a *App) label(img *image.Gray, r image.Rectangle, s string, size float64, bold, center bool, c color.Gray) {
	face := a.Fonts.Face(size, bold)
	pad := int(size / 2)
	s = text.Ellipsize(face, s, r.Dx()-2*pad)
	m := face.Metrics()
	y := r.Min.Y + (r.Dy()+m.Ascent.Ceil()-m.Descent.Ceil())/2
	x := r.Min.X + pad
	if center {
		x = r.Min.X + (r.Dx()-text.Width(face, s))/2
	}
	text.Draw(img, face, x, y, s, c)
}

// button draws a framed, labelled rectangle.
func (a *App) button(img *image.Gray, r image.Rectangle, s string, inverted bool) {
	if inverted {
		fill(img, r, black)
		a.label(img, r, s, sizeBody, true, true, white)
		return
	}
	frame(img, r, 4, black)
	a.label(img, r, s, sizeBody, false, true, black)
}

// header draws a title bar with an optional right-hand action, returning the
// action's rectangle.
func (a *App) header(img *image.Gray, title, action string) image.Rectangle {
	h := 140
	fill(img, image.Rect(0, h-4, a.W, h), black)
	var act image.Rectangle
	right := a.W
	if action != "" {
		face := a.Fonts.Face(sizeBody, false)
		w := text.Width(face, action) + 2*sizeBody
		act = image.Rect(a.W-w-24, 24, a.W-24, h-24)
		a.button(img, act, action, false)
		right = act.Min.X - 16
	}
	a.label(img, image.Rect(16, 0, right, h-4), title, sizeTitle, true, false, black)
	return act
}

const headerHeight = 140

func percent(page, count int) int {
	if count <= 1 {
		return 100
	}
	return page * 100 / (count - 1)
}

func pageLabel(page, count int) string {
	return strings.Join([]string{"p. ", itoa(page + 1), " / ", itoa(count)}, "")
}

// --- opening books ------------------------------------------------------

// OpenBook switches to the reader at the saved position.
func (a *App) OpenBook(id string) error {
	b, err := book.Open(a.Paths.Book(id))
	if err != nil {
		return err
	}
	page := 0
	if pos, ok := a.State.Get(id); ok {
		page = b.PageOf(pos)
	}
	a.fullNext, a.bookID = true, id
	a.Show(&Reader{id: id, book: b, page: page})
	return nil
}
