package ui

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"log"
	"time"

	xdraw "golang.org/x/image/draw"

	"github.com/amacdougall/tategaki/reader/internal/book"
	"github.com/amacdougall/tategaki/reader/internal/config"
	"github.com/amacdougall/tategaki/reader/internal/display"
)

// Sleep draws the sleep screen as the Kindle goes to its screensaver: with
// the Kindle UI paused, the e-ink keeps whatever we drew last. Wake redraws
// the screen underneath.
//
// The screen is the book's cover, or お休みなさい when the book's
// sleep_screen is "goodnight", it has no cover, or there's no book. The book
// is the one opened this session, else the last one read.
func (a *App) Sleep() {
	start := time.Now()
	id := a.bookID
	if id == "" {
		id = a.State.State.LastBook
	}
	img, what := a.goodnight(), "goodnight"
	if id != "" {
		side, err := config.LoadBook(a.Paths, id)
		if err != nil {
			log.Printf("sidecar %s: %v", id, err)
		}
		if side.SleepScreen != "goodnight" {
			if cover, err := a.sleepCover(id); err != nil {
				log.Printf("sleep cover %s: %v", id, err)
			} else {
				img, what = cover, "cover "+id
				if a.Dark { // the hardware inversion would show it as a negative
					for i, v := range img.Pix {
						img.Pix[i] = 255 - v
					}
				}
			}
		}
	}
	if err := a.Disp.Image(img, image.Point{}, display.Full); err != nil {
		log.Printf("sleep screen: %v", err)
		return
	}
	log.Printf("SLEEP %s drawn in %v", what, time.Since(start).Round(time.Millisecond))
}

// goodnightSize is the お休みなさい glyph size in pixels (about 48 pt).
const goodnightSize = 200

// goodnight is お休みなさい in one vertical column, centred. Unlike the
// cover, it follows dark mode (white on black).
func (a *App) goodnight() *image.Gray {
	img := a.canvas()
	chars := []rune("お休みなさい")
	step := goodnightSize * 21 / 20
	y := (a.H - step*len(chars)) / 2
	for _, c := range chars {
		box := image.Rect(a.W/2-goodnightSize, y, a.W/2+goodnightSize, y+step)
		a.label(img, box, string(c), goodnightSize, false, true, black)
		y += step
	}
	return img
}

// sleepCover is the book's sleep.png (already screen-sized), or its cover
// thumbnail scaled up, centred on white.
func (a *App) sleepCover(id string) (*image.Gray, error) {
	b, err := book.Open(a.Paths.Book(id))
	if err != nil {
		return nil, err
	}
	defer b.Close()
	data, err := b.SleepCover()
	if err != nil {
		return nil, err
	}
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	img := a.canvas()
	sb := src.Bounds()
	if sb.Dx() == a.W && sb.Dy() == a.H {
		draw.Draw(img, img.Bounds(), src, sb.Min, draw.Src)
		return img, nil
	}
	scale := min(float64(a.W)/float64(sb.Dx()), float64(a.H)/float64(sb.Dy()))
	w, h := int(float64(sb.Dx())*scale), int(float64(sb.Dy())*scale)
	at := image.Pt((a.W-w)/2, (a.H-h)/2)
	xdraw.CatmullRom.Scale(img, image.Rectangle{at, at.Add(image.Pt(w, h))}, src, sb, draw.Src, nil)
	return img, nil
}
