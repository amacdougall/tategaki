// Package book reads .jbk bundles (plan §6).
package book

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Manifest is manifest.json.
type Manifest struct {
	Format    int    `json:"format"`
	Title     string `json:"title"`
	Author    string `json:"author"`
	PageCount int    `json:"page_count"`
	Profile   struct {
		Name          string `json:"name"`
		Width, Height int
		DPI           int `json:"dpi"`
	} `json:"profile"`
	Render   map[string]any `json:"render"`
	Chapters []Chapter      `json:"chapters"`
	TOC      []TOCEntry     `json:"toc"`
}

// Chapter is one spine item.
type Chapter struct {
	ID        int    `json:"id"`
	Text      string `json:"text"`
	Layout    string `json:"layout"`
	FirstPage int    `json:"first_page"`
	PageCount int    `json:"page_count"`
}

// TOCEntry points at a global page.
type TOCEntry struct {
	Title string `json:"title"`
	Page  int    `json:"page"`
	Level int    `json:"level"`
}

// Glyph is [codepoint offset, x, y, w, h] in page pixels.
type Glyph [5]int

// PageLayout is one page of layout/NNNN.json.
type PageLayout struct {
	Page   int     `json:"page"`
	Glyphs []Glyph `json:"glyphs"`
	Images []struct {
		Box  [4]int `json:"box"`
		Full string `json:"full"`
	} `json:"images"`
}

// Layout is layout/NNNN.json.
type Layout struct {
	Pages      []PageLayout `json:"pages"`
	Paragraphs [][2]int     `json:"paragraphs"`
	Sentences  [][2]int     `json:"sentences"`
}

// Book is an open .jbk.
type Book struct {
	Path     string
	Manifest Manifest
	zr       *zip.ReadCloser
	files    map[string]*zip.File
	layouts  map[int]*Layout
}

// Open opens a bundle and parses its manifest.
func Open(path string) (*Book, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	b := &Book{Path: path, zr: zr, files: map[string]*zip.File{}, layouts: map[int]*Layout{}}
	for _, f := range zr.File {
		b.files[f.Name] = f
	}
	data, err := b.read("manifest.json")
	if err != nil {
		zr.Close()
		return nil, err
	}
	if err := json.Unmarshal(data, &b.Manifest); err != nil {
		zr.Close()
		return nil, fmt.Errorf("%s: manifest: %w", path, err)
	}
	if b.Manifest.Format != 1 {
		zr.Close()
		return nil, fmt.Errorf("%s: unsupported format %d", path, b.Manifest.Format)
	}
	return b, nil
}

func (b *Book) read(name string) ([]byte, error) {
	f, ok := b.files[name]
	if !ok {
		return nil, fmt.Errorf("%s: missing %s", b.Path, name)
	}
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// ReadManifest reads only a bundle's manifest (for the library list).
func ReadManifest(path string) (Manifest, error) {
	b, err := Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer b.Close()
	return b.Manifest, nil
}

// Cover returns cover.png's bytes, if the bundle has one.
func (b *Book) Cover() ([]byte, error) { return b.read("cover.png") }

// Layout returns a chapter's layout, loading it on first use.
func (b *Book) Layout(chapter int) (*Layout, error) {
	if l, ok := b.layouts[chapter]; ok {
		return l, nil
	}
	if chapter < 0 || chapter >= len(b.Manifest.Chapters) {
		return nil, fmt.Errorf("chapter %d out of range", chapter)
	}
	data, err := b.read(b.Manifest.Chapters[chapter].Layout)
	if err != nil {
		return nil, err
	}
	var l Layout
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("%s: %w", b.Manifest.Chapters[chapter].Layout, err)
	}
	b.layouts[chapter] = &l
	return &l, nil
}

// Text returns a chapter's text as code points.
func (b *Book) Text(chapter int) ([]rune, error) {
	data, err := b.read(b.Manifest.Chapters[chapter].Text)
	if err != nil {
		return nil, err
	}
	return []rune(string(data)), nil
}

// ChapterOf returns the chapter containing a global page.
func (b *Book) ChapterOf(page int) int {
	for i, c := range b.Manifest.Chapters {
		if page >= c.FirstPage && page < c.FirstPage+c.PageCount {
			return i
		}
	}
	return max(0, len(b.Manifest.Chapters)-1)
}

// TOCTitle is the title of the TOC entry a page falls under ("" if none).
func (b *Book) TOCTitle(page int) string {
	title := ""
	for _, e := range b.Manifest.TOC {
		if e.Page <= page {
			title = e.Title
		}
	}
	return title
}

// ExtractPage writes page n's PNG to dir and returns its path. FBInk needs a
// file, and /var/tmp is tmpfs, so this is cheap.
func (b *Book) ExtractPage(n int, dir string) (string, error) {
	data, err := b.read(fmt.Sprintf("pages/%05d.png", n))
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "page.png")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", err
	}
	return path, os.Rename(tmp, path)
}

// Close closes the bundle.
func (b *Book) Close() error { return b.zr.Close() }
