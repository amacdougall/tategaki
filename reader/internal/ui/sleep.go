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
	"github.com/amacdougall/tategaki/reader/internal/display"
)

// Sleep draws the book's cover full-screen as the Kindle goes to its
// screensaver: with the Kindle UI paused, the e-ink keeps whatever we drew
// last. The book is the one opened this session, else the last one read; with
// neither, the screen is left as it is. Wake redraws the screen underneath.
func (a *App) Sleep() {
	start := time.Now()
	id := a.bookID
	if id == "" {
		id = a.State.State.LastBook
	}
	if id == "" {
		return
	}
	img, err := a.sleepCover(id)
	if err != nil {
		log.Printf("sleep cover %s: %v", id, err)
		return
	}
	if a.Dark { // the hardware inversion would show the cover as a negative
		for i, v := range img.Pix {
			img.Pix[i] = 255 - v
		}
	}
	if err := a.Disp.Image(img, image.Point{}, display.Full); err != nil {
		log.Printf("sleep cover %s: %v", id, err)
		return
	}
	log.Printf("SLEEP cover %s drawn in %v", id, time.Since(start).Round(time.Millisecond))
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
