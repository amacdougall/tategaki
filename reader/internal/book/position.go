package book

import "github.com/amacdougall/tategaki/reader/internal/config"

// pagePos is a page's position: the offset of its first glyph, or for a
// glyphless (illustration) page, the offset of the text that follows it.
type pagePos struct {
	page   int
	offset int
	image  bool
}

func (b *Book) chapterPositions(chapter int) ([]pagePos, error) {
	l, err := b.Layout(chapter)
	if err != nil {
		return nil, err
	}
	out := make([]pagePos, len(l.Pages))
	next := -1 // offset of the next text page, scanning backwards
	for i := len(l.Pages) - 1; i >= 0; i-- {
		p := l.Pages[i]
		if len(p.Glyphs) > 0 {
			next = p.Glyphs[0][0]
			out[i] = pagePos{page: p.Page, offset: next}
			continue
		}
		off := next
		if off < 0 { // no text after it: use the end of the text before it
			off = 0
			for j := i - 1; j >= 0; j-- {
				if g := l.Pages[j].Glyphs; len(g) > 0 {
					off = g[len(g)-1][0] + 1
					break
				}
			}
		}
		out[i] = pagePos{page: p.Page, offset: off, image: true}
	}
	return out, nil
}

// PositionOf returns the reading position for a global page.
func (b *Book) PositionOf(page int) (config.Position, error) {
	ch := b.ChapterOf(page)
	positions, err := b.chapterPositions(ch)
	if err != nil {
		return config.Position{}, err
	}
	for _, p := range positions {
		if p.page == page {
			return config.Position{Chapter: ch, Offset: p.offset, Image: p.image, Page: page,
				PageCount: b.Manifest.PageCount}, nil
		}
	}
	return config.Position{Chapter: ch, Page: page, PageCount: b.Manifest.PageCount}, nil
}

// PageOf maps a saved position back to a page: an exact match (same offset,
// same kind) if there is one, else the page whose text contains the offset.
// Positions from a different layout (another font size) land on the page
// containing the saved offset.
func (b *Book) PageOf(pos config.Position) int {
	chapters := b.Manifest.Chapters
	if len(chapters) == 0 {
		return 0
	}
	ch := min(max(pos.Chapter, 0), len(chapters)-1)
	positions, err := b.chapterPositions(ch)
	if err != nil || len(positions) == 0 {
		return chapters[ch].FirstPage
	}
	exact := -1
	for _, p := range positions {
		if p.offset == pos.Offset && p.image == pos.Image {
			if p.page == pos.Page {
				return p.page // several pages share a position (a split spread)
			}
			if exact < 0 {
				exact = p.page
			}
		}
	}
	if exact >= 0 {
		return exact
	}
	best := positions[0].page
	for _, p := range positions {
		if !p.image && p.offset <= pos.Offset {
			best = p.page
		}
	}
	return best
}
