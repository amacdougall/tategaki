package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/amacdougall/tategaki/reader/internal/book"
	"github.com/amacdougall/tategaki/reader/internal/config"
	"github.com/amacdougall/tategaki/reader/internal/display"
	"github.com/amacdougall/tategaki/reader/internal/input"

	xdraw "golang.org/x/image/draw"
)

func itoa(n int) string { return strconv.Itoa(n) }

type libEntry struct {
	id       string
	manifest book.Manifest
	cover    *image.Gray
	pos      config.Position
	hasPos   bool
}

// Library lists the deployed books (plan §8.1).
type Library struct {
	entries []libEntry
	page    int
	rows    []image.Rectangle // on screen, parallel to visible entries
	exit    image.Rectangle
	prev    image.Rectangle
	next    image.Rectangle
	err     string
}

const (
	rowHeight = 300
	coverW    = 180
	coverH    = 240
)

// NewLibrary scans the books folder. Settings are re-read every time the
// library opens, so a deploy takes effect without restarting (plan §5.8).
func NewLibrary(a *App) *Library {
	a.reloadConfig()
	l := &Library{}
	paths, _ := filepath.Glob(filepath.Join(a.Paths.Books(), "*.jbk"))
	for _, p := range paths {
		id := strings.TrimSuffix(filepath.Base(p), ".jbk")
		b, err := book.Open(p)
		if err != nil {
			log.Printf("library: %s: %v", p, err)
			continue
		}
		e := libEntry{id: id, manifest: b.Manifest}
		if data, err := b.Cover(); err == nil {
			e.cover = thumbnail(data)
		}
		b.Close()
		e.pos, e.hasPos = a.State.Get(id)
		l.entries = append(l.entries, e)
	}
	// Most recently read first, then unread books by title.
	sort.SliceStable(l.entries, func(i, j int) bool {
		ei, ej := l.entries[i], l.entries[j]
		if ei.pos.Updated != ej.pos.Updated {
			return ei.pos.Updated > ej.pos.Updated
		}
		return ei.manifest.Title < ej.manifest.Title
	})
	if len(l.entries) == 0 {
		l.err = "No books yet. On the computer, run:  bin/tategaki build && bin/tategaki deploy"
		if _, err := os.Stat(a.Paths.Books()); err != nil {
			l.err = "No books folder at " + a.Paths.Books()
		}
	}
	return l
}

func thumbnail(data []byte) *image.Gray {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	b := src.Bounds()
	scale := min(float64(coverW)/float64(b.Dx()), float64(coverH)/float64(b.Dy()))
	w, h := int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)
	dst := image.NewGray(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

func (l *Library) perPage(a *App) int {
	return (a.H - headerHeight - 140) / rowHeight
}

// Draw renders the list.
func (l *Library) Draw(a *App) error {
	img := a.canvas()
	l.exit = a.header(img, fmt.Sprintf("Library (%d)", len(l.entries)), "Exit")
	if l.err != "" {
		face := a.Fonts.Face(sizeBody, false)
		y := headerHeight + 120
		for _, line := range wrap(face, l.err, a.W-96) {
			drawText(img, face, 48, y, line)
			y += sizeBody * 3 / 2
		}
		return a.Disp.Image(img, image.Point{}, display.Full)
	}
	n := l.perPage(a)
	pages := (len(l.entries) + n - 1) / n
	l.page = min(l.page, pages-1)
	l.rows = nil
	y := headerHeight
	for i := l.page * n; i < len(l.entries) && i < (l.page+1)*n; i++ {
		e := l.entries[i]
		r := image.Rect(0, y, a.W, y+rowHeight)
		l.rows = append(l.rows, r)
		cover := image.Rect(32, y+30, 32+coverW, y+30+coverH)
		if e.cover != nil {
			cb := e.cover.Bounds()
			at := image.Pt(cover.Min.X+(coverW-cb.Dx())/2, cover.Min.Y+(coverH-cb.Dy())/2)
			draw.Draw(img, cb.Add(at), e.cover, image.Point{}, draw.Src)
			frame(img, cb.Add(at), 2, gray)
		} else {
			frame(img, cover, 2, gray)
		}
		x := cover.Max.X + 32
		a.label(img, image.Rect(x, y+30, a.W-24, y+110), e.manifest.Title, sizeTitle, true, false, black)
		a.label(img, image.Rect(x, y+110, a.W-24, y+180), e.manifest.Author, sizeBody, false, false, black)
		status := "Not started · " + itoa(e.manifest.PageCount) + " pages"
		if e.hasPos {
			status = fmt.Sprintf("%d%% · %s", percent(e.pos.Page, e.manifest.PageCount), pageLabel(e.pos.Page, e.manifest.PageCount))
		}
		a.label(img, image.Rect(x, y+180, a.W-24, y+250), status, sizeSmall, false, false, gray)
		fill(img, image.Rect(24, y+rowHeight-2, a.W-24, y+rowHeight), light)
		y += rowHeight
	}
	l.prev, l.next = image.Rectangle{}, image.Rectangle{}
	if pages > 1 {
		foot := a.H - 120
		l.next = image.Rect(24, foot, 324, foot+96) // Japanese order: next on the left
		l.prev = image.Rect(a.W-324, foot, a.W-24, foot+96)
		a.button(img, l.next, "◀ Next", false)
		a.button(img, l.prev, "Prev ▶", false)
		a.label(img, image.Rect(324, foot, a.W-324, foot+96), fmt.Sprintf("%d / %d", l.page+1, pages), sizeBody, false, true, black)
	}
	return a.Disp.Image(img, image.Point{}, display.Full)
}

// Handle opens a book or pages the list.
func (l *Library) Handle(a *App, g input.Gesture) error {
	pt := image.Pt(g.X, g.Y)
	switch {
	case g.Kind == input.SwipeRight || (g.Kind == input.Tap && pt.In(l.next)):
		if l.next != (image.Rectangle{}) {
			l.page++
			return l.Draw(a)
		}
	case g.Kind == input.SwipeLeft || (g.Kind == input.Tap && pt.In(l.prev)):
		if l.page > 0 {
			l.page--
			return l.Draw(a)
		}
	case g.Kind == input.Tap && pt.In(l.exit):
		a.Quit()
	case g.Kind == input.Tap:
		for i, r := range l.rows {
			if pt.In(r) {
				return a.OpenBook(l.entries[l.page*l.perPage(a)+i].id)
			}
		}
	}
	return nil
}
