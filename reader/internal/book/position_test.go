package book

import (
	"os"
	"path/filepath"
	"testing"
)

func testBook(t *testing.T) *Book {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, "Tategaki", "books", "toradora-01", "build", "toradora-01.jbk")
	if _, err := os.Stat(path); err != nil {
		t.Skip("test book not built")
	}
	b, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

// Every page survives the position round trip, including illustration pages
// and the two halves of a split spread.
func TestPositionRoundTrip(t *testing.T) {
	b := testBook(t)
	images := 0
	for page := 0; page < b.Manifest.PageCount; page++ {
		pos, err := b.PositionOf(page)
		if err != nil {
			t.Fatal(err)
		}
		if pos.Image {
			images++
		}
		if got := b.PageOf(pos); got != page {
			t.Errorf("page %d -> %+v -> page %d", page, pos, got)
		}
	}
	if images == 0 {
		t.Error("expected illustration pages")
	}
}

// A position from a different layout (other font size) lands on the page
// containing its offset.
func TestPageOfOffsetInsidePage(t *testing.T) {
	b := testBook(t)
	ch := b.ChapterOf(20)
	l, _ := b.Layout(ch)
	var mid int
	for _, p := range l.Pages {
		if p.Page == 20 {
			mid = p.Glyphs[len(p.Glyphs)/2][0]
		}
	}
	pos, _ := b.PositionOf(20)
	pos.Offset, pos.Page = mid, 0
	if got := b.PageOf(pos); got != 20 {
		t.Errorf("offset %d in page 20 mapped to page %d", mid, got)
	}
}
