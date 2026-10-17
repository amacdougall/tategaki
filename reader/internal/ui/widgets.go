package ui

import (
	"image"
	"image/color"

	"golang.org/x/image/font"

	"github.com/amacdougall/tategaki/reader/internal/display"
	"github.com/amacdougall/tategaki/reader/internal/input"
	"github.com/amacdougall/tategaki/reader/internal/text"
)

func wrap(face font.Face, s string, width int) []string { return text.Wrap(face, s, width, 0) }

func drawText(img *image.Gray, face font.Face, x, y int, s string) {
	text.Draw(img, face, x, y, s, color.Gray{0})
}

// listView is a paged, tappable list (TOC, and later other pickers).
type listView struct {
	items  []listItem
	marked int // highlighted item (the current chapter), -1 for none
	page   int
	rows   []image.Rectangle
	back   image.Rectangle
	prev   image.Rectangle
	next   image.Rectangle
	chosen int
}

type listItem struct {
	label  string
	detail string
	value  int
}

type listResult int

const (
	listNone listResult = iota
	listBack
	listChosen
	listRedraw
)

const listRow = 120

func (l *listView) perPage(a *App) int { return (a.H - headerHeight - 160) / listRow }

func (l *listView) pages(a *App) int {
	n := l.perPage(a)
	return max(1, (len(l.items)+n-1)/n)
}

func (l *listView) draw(a *App, title, back string) error {
	img := a.canvas()
	l.back = a.header(img, title, back)
	n := l.perPage(a)
	l.page = min(l.page, l.pages(a)-1)
	l.rows = nil
	y := headerHeight
	for i := l.page * n; i < len(l.items) && i < (l.page+1)*n; i++ {
		r := image.Rect(0, y, a.W, y+listRow)
		l.rows = append(l.rows, r)
		fg := color.Gray{0}
		if i == l.marked {
			fill(img, r, black)
			fg = white
		}
		face := a.Fonts.Face(sizeSmall, false)
		dw := text.Width(face, l.items[i].detail) + 48
		a.label(img, image.Rect(24, y, a.W-dw, y+listRow), l.items[i].label, sizeBody, false, false, fg)
		a.label(img, image.Rect(a.W-dw, y, a.W-8, y+listRow), l.items[i].detail, sizeSmall, false, false, fg)
		fill(img, image.Rect(24, y+listRow-2, a.W-24, y+listRow), light)
		y += listRow
	}
	l.prev, l.next = image.Rectangle{}, image.Rectangle{}
	if pages := l.pages(a); pages > 1 {
		foot := a.H - 130
		l.next = image.Rect(24, foot, 324, foot+100)
		l.prev = image.Rect(a.W-324, foot, a.W-24, foot+100)
		a.button(img, l.next, "◀ Next", false)
		a.button(img, l.prev, "Prev ▶", false)
		a.label(img, image.Rect(324, foot, a.W-324, foot+100), itoa(l.page+1)+" / "+itoa(pages), sizeBody, false, true, black)
	}
	return a.Disp.Image(img, image.Point{}, display.Full)
}

func (l *listView) handle(a *App, g input.Gesture) listResult {
	pt := image.Pt(g.X, g.Y)
	switch {
	case g.Kind == input.SwipeRight || (g.Kind == input.Tap && pt.In(l.next)):
		if l.page < l.pages(a)-1 {
			l.page++
			return listRedraw
		}
	case g.Kind == input.SwipeLeft || (g.Kind == input.Tap && pt.In(l.prev)):
		if l.page > 0 {
			l.page--
			return listRedraw
		}
	case g.Kind == input.Tap && pt.In(l.back):
		return listBack
	case g.Kind == input.Tap:
		for i, r := range l.rows {
			if pt.In(r) {
				l.chosen = l.items[l.page*l.perPage(a)+i].value
				return listChosen
			}
		}
	}
	return listNone
}
