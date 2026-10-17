package ui

import (
	"fmt"
	"image"
	"sort"
	"strings"

	"github.com/amacdougall/tategaki/reader/internal/anki"
	"github.com/amacdougall/tategaki/reader/internal/config"
	"github.com/amacdougall/tategaki/reader/internal/display"
	"github.com/amacdougall/tategaki/reader/internal/input"
	"github.com/amacdougall/tategaki/reader/internal/text"
)

// Menu is a sheet over the top of the page (plan §8.1). Tapping below it
// closes it.
type Menu struct {
	reader  *Reader
	sheet   image.Rectangle
	buttons []menuButton
	anki    string // Anki button label while/after sending
}

type menuButton struct {
	label string
	rect  image.Rectangle
	do    func(a *App) error
}

func (m *Menu) Draw(a *App) error {
	r := m.reader
	m.sheet = image.Rect(0, 0, a.W, 850)
	img := image.NewGray(m.sheet)
	fill(img, m.sheet, white)
	b := r.book.Manifest
	a.label(img, image.Rect(24, 20, a.W-24, 110), b.Title, sizeTitle, true, false, black)
	where := pageLabel(r.page, b.PageCount) + fmt.Sprintf("  (%d%%)", percent(r.page, b.PageCount))
	if t := r.book.TOCTitle(r.page); t != "" {
		where = t + " · " + where
	}
	a.label(img, image.Rect(24, 110, a.W-24, 180), where, sizeBody, false, false, black)

	m.buttons = []menuButton{
		{label: "Contents", do: func(a *App) error { a.Show(&TOC{reader: r}); return nil }},
		{label: "Go to page", do: func(a *App) error { a.Show(&Goto{reader: r}); return nil }},
		{label: "Book info", do: func(a *App) error { a.Show(&Info{reader: r}); return nil }},
		{label: "Library", do: func(a *App) error { r.Library(a); return nil }},
		{label: m.ankiLabel(a), do: func(a *App) error {
			m.anki = "Sending…"
			m.Draw(a)
			a.FlushAnki(func(res anki.FlushResult) {
				m.anki = res.String()
				if a.screen == m {
					m.Draw(a)
				}
			})
			return nil
		}},
		{label: "Close menu", do: func(a *App) error { a.Show(r); return nil }},
		{label: "Exit", do: func(a *App) error { a.Quit(); return nil }},
	}
	bw, bh, gap := (a.W-3*24)/2, 120, 24
	for i := range m.buttons {
		x := 24 + (i%2)*(bw+gap)
		y := 210 + (i/2)*(bh+gap)
		m.buttons[i].rect = image.Rect(x, y, x+bw, y+bh)
		a.button(img, m.buttons[i].rect, m.buttons[i].label, false)
	}
	fill(img, image.Rect(0, m.sheet.Max.Y-6, a.W, m.sheet.Max.Y), black)
	return a.Disp.Image(img, image.Point{}, display.Partial)
}

func (m *Menu) ankiLabel(a *App) string {
	if m.anki != "" {
		return m.anki
	}
	return fmt.Sprintf("Send to Anki (%d)", a.Queue.Pending())
}

func (m *Menu) Handle(a *App, g input.Gesture) error {
	if g.Kind != input.Tap {
		return nil
	}
	pt := image.Pt(g.X, g.Y)
	for _, b := range m.buttons {
		if pt.In(b.rect) {
			return b.do(a)
		}
	}
	if !pt.In(m.sheet) {
		a.Show(m.reader) // tap outside closes
	}
	return nil
}

// --- Table of contents -----------------------------------------------------

// TOC lists the book's table of contents.
type TOC struct {
	reader *Reader
	list   listView
}

func (t *TOC) Draw(a *App) error {
	b := t.reader.book
	if t.list.items == nil {
		current := -1
		for i, e := range b.Manifest.TOC {
			indent := strings.Repeat("　", max(e.Level-1, 0))
			t.list.items = append(t.list.items, listItem{label: indent + e.Title, detail: itoa(e.Page + 1), value: e.Page})
			if e.Page <= t.reader.page {
				current = i
			}
		}
		t.list.marked = current
		if current >= 0 {
			t.list.page = current / t.list.perPage(a)
		}
	}
	return t.list.draw(a, "Contents", "Back")
}

func (t *TOC) Handle(a *App, g input.Gesture) error {
	switch t.list.handle(a, g) {
	case listBack:
		a.Show(t.reader)
	case listChosen:
		return t.reader.Goto(a, t.list.chosen)
	case listRedraw:
		return t.Draw(a)
	}
	return nil
}

// --- Book info -------------------------------------------------------------

// Info shows what was deployed: render settings and the resolved Anki deck and
// tags, read-only (plan §8.1).
type Info struct{ reader *Reader }

func (i *Info) Draw(a *App) error {
	img := a.canvas()
	a.header(img, "Book info", "Back")
	b := i.reader.book.Manifest
	side, _ := config.LoadBook(a.Paths, i.reader.id)
	deck := a.Dev.DefaultDeck + " (default)"
	if side.Deck != nil && *side.Deck != "" {
		deck = *side.Deck
	}
	var render []string
	keys := make([]string, 0, len(b.Render))
	for k := range b.Render {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		render = append(render, fmt.Sprintf("  %s: %v", k, b.Render[k]))
	}
	lines := []string{
		b.Title, b.Author, "",
		fmt.Sprintf("Book id: %s", i.reader.id),
		fmt.Sprintf("Pages: %d   Chapters: %d", b.PageCount, len(b.Chapters)),
		"", "Anki",
		"  Deck: " + deck,
		"  Tags: " + strings.Join(side.Tags, ", "),
		fmt.Sprintf("  Strip speaker tags: %v", side.StripSpeakerTags),
		"  Note type: " + a.Dev.NoteType,
		"", "Rendering",
	}
	lines = append(lines, render...)
	face := a.Fonts.Face(sizeSmall, false)
	y := headerHeight + 70
	for _, line := range lines {
		for _, l := range text.Wrap(face, line, a.W-96, 0) {
			if y > a.H-40 {
				break
			}
			drawText(img, face, 48, y, l)
			y += sizeSmall * 3 / 2
		}
	}
	return a.Disp.Image(img, image.Point{}, display.Full)
}

func (i *Info) Handle(a *App, g input.Gesture) error {
	if g.Kind == input.Tap {
		a.Show(i.reader)
	}
	return nil
}
