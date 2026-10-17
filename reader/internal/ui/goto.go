package ui

import (
	"fmt"
	"image"
	"strconv"

	"github.com/amacdougall/tategaki/reader/internal/display"
	"github.com/amacdougall/tategaki/reader/internal/input"
)

// Goto is a numeric keypad for jumping to a page.
type Goto struct {
	reader *Reader
	digits string
	keys   []gotoKey
	back   image.Rectangle
}

type gotoKey struct {
	label string
	rect  image.Rectangle
}

func (g *Goto) Draw(a *App) error {
	img := a.canvas()
	g.back = a.header(img, "Go to page", "Cancel")
	count := g.reader.book.Manifest.PageCount
	shown := g.digits
	if shown == "" {
		shown = "—"
	}
	a.label(img, image.Rect(0, headerHeight+40, a.W, headerHeight+200), shown, sizeTitle*2, true, true, black)
	a.label(img, image.Rect(0, headerHeight+200, a.W, headerHeight+270),
		fmt.Sprintf("1 – %d   (now %d)", count, g.reader.page+1), sizeBody, false, true, black)

	labels := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "Del", "0", "Go"}
	kw, kh, gap := 300, 220, 30
	x0 := (a.W - 3*kw - 2*gap) / 2
	y0 := headerHeight + 320
	g.keys = nil
	for i, l := range labels {
		r := image.Rect(x0+(i%3)*(kw+gap), y0+(i/3)*(kh+gap), x0+(i%3)*(kw+gap)+kw, y0+(i/3)*(kh+gap)+kh)
		g.keys = append(g.keys, gotoKey{l, r})
		a.button(img, r, l, l == "Go")
	}
	return a.Disp.Image(img, image.Point{}, display.Full)
}

func (g *Goto) Handle(a *App, ev input.Gesture) error {
	if ev.Kind != input.Tap {
		return nil
	}
	pt := image.Pt(ev.X, ev.Y)
	if pt.In(g.back) {
		a.Show(g.reader)
		return nil
	}
	for _, k := range g.keys {
		if !pt.In(k.rect) {
			continue
		}
		switch k.label {
		case "Del":
			if g.digits != "" {
				g.digits = g.digits[:len(g.digits)-1]
			}
		case "Go":
			n, err := strconv.Atoi(g.digits)
			if err != nil || n < 1 {
				return nil
			}
			return g.reader.Goto(a, n-1)
		default:
			if len(g.digits) < 5 {
				g.digits += k.label
			}
		}
		return g.redrawNumber(a)
	}
	return nil
}

// redrawNumber refreshes only the number field (fast refresh).
func (g *Goto) redrawNumber(a *App) error {
	r := image.Rect(0, headerHeight+40, a.W, headerHeight+200)
	img := image.NewGray(r)
	fill(img, r, white)
	shown := g.digits
	if shown == "" {
		shown = "—"
	}
	a.label(img, r, shown, sizeTitle*2, true, true, black)
	return a.Disp.Image(img, r.Min, display.Fast)
}
