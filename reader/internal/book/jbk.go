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
	Chapters []struct {
		ID        int    `json:"id"`
		Text      string `json:"text"`
		Layout    string `json:"layout"`
		FirstPage int    `json:"first_page"`
		PageCount int    `json:"page_count"`
	} `json:"chapters"`
	TOC []struct {
		Title string `json:"title"`
		Page  int    `json:"page"`
		Level int    `json:"level"`
	} `json:"toc"`
}

// Book is an open .jbk.
type Book struct {
	Path     string
	Manifest Manifest
	zr       *zip.ReadCloser
	files    map[string]*zip.File
}

// Open opens a bundle and parses its manifest.
func Open(path string) (*Book, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	b := &Book{Path: path, zr: zr, files: map[string]*zip.File{}}
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
