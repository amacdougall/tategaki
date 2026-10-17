package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/amacdougall/tategaki/reader/internal/book"
	"github.com/amacdougall/tategaki/reader/internal/dict"
	"github.com/amacdougall/tategaki/reader/internal/display"
	"github.com/amacdougall/tategaki/reader/internal/input"
	"github.com/amacdougall/tategaki/reader/internal/text"
)

// Tap-to-lookup (plan §7): hit testing, the scan, the highlight and the popup.

const (
	hitSlop    = 24 // px of slop around glyph boxes
	barW       = 7  // highlight side-line width
	barGap     = 4  // gap between glyph box and side-line
	maxSpanLen = 32
)

// lookupable reports whether a character starts a lookup; punctuation, spaces
// and symbols fall through to the page-turn zones (plan §7.1).
func lookupable(r rune) bool {
	return !(unicode.IsPunct(r) || unicode.IsSpace(r) || unicode.IsSymbol(r))
}

// hit returns the glyph nearest to (x, y) within hitSlop, if any.
func hit(glyphs []book.Glyph, x, y int) (book.Glyph, bool) {
	best, bestD := book.Glyph{}, -1
	for _, g := range glyphs {
		dx := max(g[1]-x, 0, x-(g[1]+g[3]))
		dy := max(g[2]-y, 0, y-(g[2]+g[4]))
		d := dx*dx + dy*dy
		if bestD < 0 || d < bestD {
			best, bestD = g, d
		}
	}
	return best, bestD >= 0 && bestD <= hitSlop*hitSlop
}

// pageGlyphs returns the current page's layout entry and chapter.
func (r *Reader) pageLayout() (*book.PageLayout, int, error) {
	ch := r.book.ChapterOf(r.page)
	l, err := r.book.Layout(ch)
	if err != nil {
		return nil, ch, err
	}
	for i := range l.Pages {
		if l.Pages[i].Page == r.page {
			return &l.Pages[i], ch, nil
		}
	}
	return nil, ch, fmt.Errorf("page %d not in chapter %d layout", r.page, ch)
}

func (r *Reader) chapterText(ch int) ([]rune, error) {
	if r.texts == nil {
		r.texts = map[int][]rune{}
	}
	if t, ok := r.texts[ch]; ok {
		return t, nil
	}
	t, err := r.book.Text(ch)
	if err == nil {
		r.texts[ch] = t
	}
	return t, err
}

// pageImage decodes the current page, for restoring regions under the popup
// and highlight. Decoding starts in the background when a page is drawn.
func (r *Reader) pageImage() *image.Gray {
	if r.img != nil && r.imgPage == r.page {
		return r.img
	}
	if r.imgReady != nil && r.imgPage == r.page {
		r.img = <-r.imgReady
		r.imgReady = nil
		return r.img
	}
	r.prefetch(r.lastPath)
	return r.pageImage()
}

func (r *Reader) prefetch(path string) {
	r.img, r.imgPage, r.lastPath = nil, r.page, path
	ch := make(chan *image.Gray, 1)
	r.imgReady = ch
	go func() {
		ch <- decodeGray(path)
	}()
}

func decodeGray(path string) *image.Gray {
	data, err := os.ReadFile(path)
	if err == nil {
		if img, err := png.Decode(bytes.NewReader(data)); err == nil {
			if g, ok := img.(*image.Gray); ok {
				return g
			}
			g := image.NewGray(img.Bounds())
			draw.Draw(g, g.Bounds(), img, image.Point{}, draw.Src)
			return g
		}
	}
	log.Printf("decode page %s: %v", path, err)
	return nil
}

// restore redraws a region of the page from the decoded page image.
func (r *Reader) restore(a *App, rect image.Rectangle, mode display.Mode) {
	img := r.pageImage()
	rect = rect.Intersect(image.Rect(0, 0, a.W, a.H))
	if img == nil || rect.Empty() {
		return
	}
	crop := image.NewGray(rect)
	draw.Draw(crop, rect, img, rect.Min, draw.Src)
	if err := a.Disp.Image(crop, rect.Min, mode); err != nil {
		log.Printf("restore: %v", err)
	}
}

// --- highlight ---------------------------------------------------------------

// highlightBars returns one side-line per column for glyphs in [start, end).
// The line runs down the left of the column: in vertical text ruby sits on
// the right, where a side-line would collide with it.
func highlightBars(glyphs []book.Glyph, start, end int) []image.Rectangle {
	var bars []image.Rectangle
	for _, g := range glyphs {
		if g[0] < start || g[0] >= end {
			continue
		}
		bar := image.Rect(g[1]-barGap-barW, g[2], g[1]-barGap, g[2]+g[4])
		if n := len(bars); n > 0 && bars[n-1].Min.X == bar.Min.X && bar.Min.Y-bars[n-1].Max.Y < g[4] {
			bars[n-1].Max.Y = bar.Max.Y
			continue
		}
		bars = append(bars, bar)
	}
	return bars
}

func (r *Reader) drawBars(a *App, bars []image.Rectangle) {
	img := r.pageImage()
	for _, b := range bars {
		region := b.Inset(-2).Intersect(image.Rect(0, 0, a.W, a.H))
		crop := image.NewGray(region)
		if img != nil {
			draw.Draw(crop, region, img, region.Min, draw.Src)
		}
		fill(crop, b, black)
		a.Disp.Image(crop, region.Min, display.Partial)
	}
}

func (r *Reader) clearBars(a *App, bars []image.Rectangle) {
	for _, b := range bars {
		r.restore(a, b.Inset(-2), display.Partial)
	}
}

// --- popup -------------------------------------------------------------------

// Popup is the lookup sheet (plan §7.5).
type Popup struct {
	chapter   int
	offset    int // tapped code point offset in the chapter text
	paraStart int
	paraEnd   int
	spanLen   int  // selection length when span-adjusted
	adjusted  bool // span adjust in effect: candidates come from LookupCandidates
	cands     []dict.Candidate
	raw       bool // no match: a single raw row
	selected  int
	expanded  bool
	sheet     image.Rectangle
	bars      []image.Rectangle
	pages     [][]int // candidate indices per sheet page
	pageIdx   int
	rowRects  map[int]image.Rectangle
	ankiRects map[int]image.Rectangle
	shrink    image.Rectangle
	extend    image.Rectangle
	close     image.Rectangle
	startedAt time.Time
}

// highlightLen is how many characters the highlight covers.
func (p *Popup) highlightLen() int {
	if p.adjusted || p.raw || len(p.cands) == 0 {
		return p.spanLen
	}
	return max(1, p.cands[p.selected].SourceLen)
}

// lookupAt starts a lookup at a tapped glyph.
func (r *Reader) lookupAt(a *App, ch int, g book.Glyph) error {
	started := time.Now()
	db, err := a.Dict()
	if err != nil {
		return err
	}
	text, err := r.chapterText(ch)
	if err != nil {
		return err
	}
	offset := g[0]
	paraStart, paraEnd := 0, len(text)
	if l, err := r.book.Layout(ch); err == nil {
		for _, para := range l.Paragraphs {
			if para[0] <= offset && offset < para[1] {
				paraStart, paraEnd = para[0], para[1]
				break
			}
		}
	}
	cands, err := db.Scan(text, offset, paraEnd, a.Dev.ScanLen, a.Dev.RankingPolicy, dict.ScanMaxResults)
	if err != nil {
		return err
	}
	scanned := time.Since(started)

	// The sheet covers 3/4 of the screen, on the side away from the tapped
	// character, which stays visible when it is in the outer quarter.
	sheetH := a.H * 3 / 4
	sheet := image.Rect(0, a.H-sheetH, a.W, a.H)
	if g[2]+g[4]/2 >= a.H/2 {
		sheet = image.Rect(0, 0, a.W, sheetH)
	}
	p := &Popup{chapter: ch, offset: offset, paraStart: paraStart, paraEnd: paraEnd, cands: cands, sheet: sheet, spanLen: 1, startedAt: started}
	if len(cands) == 0 {
		p.raw = true
	} else {
		p.spanLen = max(1, cands[0].SourceLen)
	}
	// Restore the old sheet only if the new one lands elsewhere; otherwise it
	// is simply drawn over.
	r.closePopup(a, r.popup != nil && r.popup.sheet != sheet)
	r.popup = p
	r.showPopup(a)
	log.Printf("LOOKUP %q at %d: %d candidates, scan %v, tap-to-popup %v", string(text[offset:min(offset+p.highlightLen(), len(text))]),
		offset, len(cands), scanned.Round(time.Millisecond), time.Since(started).Round(time.Millisecond))
	return nil
}

// showPopup draws (or redraws) the highlight and the sheet.
func (r *Reader) showPopup(a *App) {
	p := r.popup
	pl, _, err := r.pageLayout()
	if err == nil {
		bars := highlightBars(pl.Glyphs, p.offset, p.offset+p.highlightLen())
		if !sameRects(bars, p.bars) {
			r.clearBars(a, p.bars)
			p.bars = bars
			r.drawBars(a, p.bars)
		}
	}
	img := r.renderPopup(a)
	if err := a.Disp.Image(img, p.sheet.Min, display.Partial); err != nil {
		log.Printf("popup: %v", err)
	}
}

func sameRects(a, b []image.Rectangle) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// closePopup removes the sheet and highlight, restoring the page beneath.
func (r *Reader) closePopup(a *App, redrawSheet bool) {
	p := r.popup
	if p == nil {
		return
	}
	r.popup = nil
	r.clearBars(a, p.bars)
	if redrawSheet {
		r.restore(a, p.sheet, display.Partial)
	}
}

// popupText is what's being looked up, for the span-adjust bar.
func (r *Reader) popupText(p *Popup) string {
	text, _ := r.chapterText(p.chapter)
	n := min(p.offset+p.highlightLen(), len(text))
	return string(text[p.offset:n])
}

const (
	popPad     = 28
	controlH   = 120
	sizeWord   = 54
	sizeRead   = 42
	sizeNote   = 34
	sizeSense  = 38
	senseLineH = sizeSense * 3 / 2
	collapsed  = 2   // sense lines shown for unselected rows
	rowHeader  = 126 // word line + note line
)

type rowLayout struct {
	idx    int
	height int
	lines  []string // sense lines, already wrapped
}

func formatRank(rank int) string {
	s := strconv.Itoa(rank)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return "#" + s
}

func (r *Reader) layoutRows(a *App, p *Popup) []rowLayout {
	face := a.Fonts.Face(sizeSense, false)
	width := a.W - 2*popPad - 40
	var rows []rowLayout
	for i, c := range p.cands {
		var lines []string
		for j, s := range c.Senses {
			label := strconv.Itoa(j+1) + ". " + strings.Join(s.Glosses, "; ")
			lines = append(lines, text.Wrap(face, label, width, 0)...)
		}
		if !(i == p.selected && p.expanded) && len(lines) > collapsed {
			lines = append(lines[:collapsed-1], text.Ellipsize(face, lines[collapsed-1]+"…", width))
		}
		rows = append(rows, rowLayout{idx: i, height: rowHeader + len(lines)*senseLineH + 16, lines: lines})
	}
	return rows
}

// paginate groups rows into sheet pages.
func paginate(rows []rowLayout, avail int) [][]int {
	var pages [][]int
	var cur []int
	used := 0
	for _, row := range rows {
		if len(cur) > 0 && used+row.height > avail {
			pages = append(pages, cur)
			cur, used = nil, 0
		}
		cur = append(cur, row.idx)
		used += row.height
	}
	if len(cur) > 0 {
		pages = append(pages, cur)
	}
	return pages
}

func (r *Reader) renderPopup(a *App) *image.Gray {
	p := r.popup
	img := image.NewGray(p.sheet)
	fill(img, p.sheet, white)
	top := p.sheet.Min.Y
	// border on the side facing the page
	if top == 0 {
		fill(img, image.Rect(0, p.sheet.Max.Y-8, a.W, p.sheet.Max.Y), black)
	} else {
		fill(img, image.Rect(0, top, a.W, top+8), black)
	}
	y0 := top + 8

	// Control bar: shrink ◀ [span] ▶ extend, and sheet paging.
	p.shrink = image.Rect(popPad, y0+12, popPad+130, y0+controlH-12)
	p.extend = image.Rect(popPad+140, y0+12, popPad+270, y0+controlH-12)
	a.button(img, p.shrink, "◀", false)
	a.button(img, p.extend, "▶", false)
	a.label(img, image.Rect(popPad+290, y0, a.W-310, y0+controlH), r.popupText(p), sizeWord, true, false, black)

	avail := p.sheet.Dy() - 8 - controlH - 16
	var rows []rowLayout
	if p.raw {
		p.pages = [][]int{{0}}
	} else {
		rows = r.layoutRows(a, p)
		p.pages = paginate(rows, avail)
		p.pageIdx = min(p.pageIdx, len(p.pages)-1)
	}
	p.close = image.Rect(a.W-150, y0+12, a.W-popPad, y0+controlH-12)
	a.button(img, p.close, "×", false)
	if len(p.pages) > 1 { // swipe up/down to page
		a.label(img, image.Rect(a.W-300, y0, a.W-160, y0+controlH),
			strconv.Itoa(p.pageIdx+1)+" / "+strconv.Itoa(len(p.pages)), sizeNote, false, true, gray)
	}
	fill(img, image.Rect(popPad, y0+controlH, a.W-popPad, y0+controlH+2), gray)

	y := y0 + controlH + 16
	p.rowRects = map[int]image.Rectangle{}
	p.ankiRects = map[int]image.Rectangle{}
	if p.raw {
		p.ankiRects[0] = r.ankiButton(a, img, p, 0, "", image.Rect(a.W-popPad-250, y+14, a.W-popPad, y+72))
		a.label(img, image.Rect(popPad, y, a.W-popPad-300, y+90), r.popupText(p), sizeWord, true, false, black)
		a.label(img, image.Rect(popPad, y+90, a.W-popPad, y+160), "No dictionary match. Use ◀ ▶ to change the span.", sizeNote, false, false, gray)
		return img
	}
	byIdx := map[int]rowLayout{}
	for _, row := range rows {
		byIdx[row.idx] = row
	}
	for _, i := range p.pages[p.pageIdx] {
		row := byIdx[i]
		rect := image.Rect(0, y, a.W, min(y+row.height, p.sheet.Max.Y-8))
		p.rowRects[i] = rect
		r.drawRow(a, img, p, i, row, rect)
		y += row.height
	}
	return img
}

func (r *Reader) drawRow(a *App, img *image.Gray, p *Popup, i int, row rowLayout, rect image.Rectangle) {
	c := p.cands[i]
	x := popPad + 24
	if i == p.selected {
		fill(img, image.Rect(popPad, rect.Min.Y+8, popPad+10, rect.Max.Y-8), black)
	}
	y := rect.Min.Y
	wordFace := a.Fonts.Face(sizeWord, true)
	readFace := a.Fonts.Face(sizeRead, false)
	noteFace := a.Fonts.Face(sizeNote, false)
	// Top line, right to left: [+ Anki] [rank badge] … word【reading】.
	button := image.Rect(a.W-popPad-250, y+14, a.W-popPad, y+72)
	p.ankiRects[i] = r.ankiButton(a, img, p, c.EntSeq, c.Word, button)
	right := button.Min.X - 16
	if c.Rank < dict.Unranked { // inverted badge for common words (rank <= 10,000)
		label := formatRank(c.Rank)
		w := text.Width(noteFace, label) + 2*sizeNote // label() pads by half the size on each side
		badge := image.Rect(right-w, y+14, right, y+72)
		if c.Rank <= 10000 {
			fill(img, badge, black)
			a.label(img, badge, label, sizeNote, true, true, white)
		} else {
			frame(img, badge, 3, black)
			a.label(img, badge, label, sizeNote, false, true, black)
		}
		right = badge.Min.X - 16
	}
	base := y + 64
	word := text.Ellipsize(wordFace, c.Word, right-x)
	text.Draw(img, wordFace, x, base, word, black)
	xr := x + text.Width(wordFace, word) + 8
	if c.Reading != "" && c.Reading != c.Word && xr < right {
		text.Draw(img, readFace, xr, base, text.Ellipsize(readFace, "【"+c.Reading+"】", right-xr), black)
	}
	var note []string
	if len(c.Reasons) > 0 {
		note = append(note, "← "+strings.Join(c.Reasons, " ← "))
	}
	if c.CompletedFrom != nil {
		note = append(note, "completed from "+*c.CompletedFrom)
	}
	if len(c.POS) > 0 {
		note = append(note, strings.Join(c.POS, ", "))
	}
	a.label(img, image.Rect(x-sizeNote/2, y+74, a.W-popPad, y+122), strings.Join(note, "   "), sizeNote, false, false, gray)
	sy := y + rowHeader + sizeSense - 8
	senseFace := a.Fonts.Face(sizeSense, false)
	for _, line := range row.lines {
		if sy > rect.Max.Y {
			break
		}
		text.Draw(img, senseFace, x, sy, line, black)
		sy += senseLineH
	}
	fill(img, image.Rect(popPad, rect.Max.Y-2, a.W-popPad, rect.Max.Y), light)
}

// handlePopup routes a gesture while the popup is open.
func (r *Reader) handlePopup(a *App, g input.Gesture) error {
	p := r.popup
	pt := image.Pt(g.X, g.Y)
	if pt.In(p.sheet) {
		switch {
		case g.Kind == input.Tap && pt.In(p.close):
			r.closePopup(a, true)
		case g.Kind == input.SwipeUp:
			if p.pageIdx < len(p.pages)-1 {
				p.pageIdx++
				r.showPopup(a)
			}
		case g.Kind == input.SwipeDown:
			if p.pageIdx > 0 {
				p.pageIdx--
				r.showPopup(a)
			}
		case g.Kind == input.Tap && pt.In(p.shrink):
			return r.adjustSpan(a, -1)
		case g.Kind == input.Tap && pt.In(p.extend):
			return r.adjustSpan(a, 1)
		case g.Kind == input.Tap && r.hitAnki(p, pt) >= 0:
			return r.mine(a, r.hitAnki(p, pt))
		case g.Kind == input.Tap:
			for i, rect := range p.rowRects {
				if pt.In(rect) {
					if i == p.selected {
						p.expanded = !p.expanded
					} else {
						p.selected, p.expanded = i, true
					}
					r.showPopup(a)
					break
				}
			}
		}
		return nil
	}
	// Outside the sheet: another glyph restarts the lookup there; anything
	// else closes the popup.
	if g.Kind == input.Tap || g.Kind == input.Hold {
		if pl, ch, err := r.pageLayout(); err == nil {
			if gl, ok := hit(pl.Glyphs, g.X, g.Y); ok {
				text, _ := r.chapterText(ch)
				if gl[0] < len(text) && lookupable(text[gl[0]]) {
					return r.lookupAt(a, ch, gl)
				}
			}
		}
	}
	r.closePopup(a, true)
	return nil
}

// adjustSpan shrinks or extends the looked-up span by one character and
// re-runs an exact-span lookup (plan §7.5, replacing Calibre's editable word
// field).
func (r *Reader) adjustSpan(a *App, delta int) error {
	p := r.popup
	n := min(max(p.highlightLen()+delta, 1), p.paraEnd-p.offset, maxSpanLen)
	db, err := a.Dict()
	if err != nil {
		return err
	}
	text, err := r.chapterText(p.chapter)
	if err != nil {
		return err
	}
	cands, err := db.LookupCandidates(string(text[p.offset:p.offset+n]), dict.MaxResults)
	if err != nil {
		return err
	}
	p.adjusted, p.spanLen, p.cands, p.selected, p.expanded, p.pageIdx = true, n, cands, 0, false, 0
	p.raw = len(cands) == 0
	r.showPopup(a)
	return nil
}

// ankiButton draws a row's "+ Anki" button, labelled with the row's queue
// status (queued, sent, duplicate, error).
func (r *Reader) ankiButton(a *App, img *image.Gray, p *Popup, entSeq int64, word string, rect image.Rectangle) image.Rectangle {
	if word == "" {
		word = r.popupText(p)
	}
	label, done := ankiStatus(a.Queue.Latest(r.id, entSeq, word))
	if done { // a status, not a button: gray, so it doesn't read as a rank badge
		fill(img, rect, light)
	} else {
		frame(img, rect, 3, black)
	}
	a.label(img, rect, label, sizeNote, true, true, black)
	return rect
}

func (r *Reader) hitAnki(p *Popup, pt image.Point) int {
	for i, rect := range p.ankiRects {
		if pt.In(rect) {
			return i
		}
	}
	return -1
}
