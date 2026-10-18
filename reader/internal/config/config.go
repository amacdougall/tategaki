// Package config reads the settings deployed from the desktop (config.json and
// per-book sidecars, plan §5.8) and owns state.json, the only settings-like file
// the device writes (plan §8.4).
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Device mirrors tategaki.toml [device].
type Device struct {
	Profile          string `json:"profile"`
	SSHHost          string `json:"ssh_host"`
	AnkiConnectURL   string `json:"ankiconnect_url"`
	AnkiAPIKey       string `json:"anki_api_key"`
	DefaultDeck      string `json:"default_deck"`
	NoteType         string `json:"note_type"`
	RankingPolicy    string `json:"ranking_policy"`
	ScanLen          int    `json:"scan_len"`
	FullRefreshEvery int    `json:"full_refresh_every"`
	EdgeZonePct      int    `json:"edge_zone_pct"`
}

// Defaults match the converter's built-in defaults (converter config.py).
func Defaults() Device {
	return Device{
		Profile: "pw11", AnkiConnectURL: "http://127.0.0.1:8765", DefaultDeck: "Default",
		NoteType: "Kaishi 1.5k", RankingPolicy: "longest", ScanLen: 16, FullRefreshEvery: 6, EdgeZonePct: 12,
	}
}

// Book mirrors a book's resolved [anki] and [reader] settings
// (books/<id>.json).
type Book struct {
	Deck             *string  `json:"deck"`
	Tags             []string `json:"tags"`
	StripSpeakerTags bool     `json:"strip_speaker_tags"`
	// SleepScreen is "cover" (the default) or "goodnight".
	SleepScreen string `json:"sleep_screen"`
}

// Paths under the reader's root (/mnt/us/tategaki on the device).
type Paths struct{ Root string }

func (p Paths) Config() string           { return filepath.Join(p.Root, "config.json") }
func (p Paths) State() string            { return filepath.Join(p.Root, "state.json") }
func (p Paths) AnkiWords() string        { return filepath.Join(p.Root, "anki-words.txt") }
func (p Paths) Books() string            { return filepath.Join(p.Root, "books") }
func (p Paths) Book(id string) string    { return filepath.Join(p.Root, "books", id+".jbk") }
func (p Paths) Sidecar(id string) string { return filepath.Join(p.Root, "books", id+".json") }

// LoadDevice reads config.json over the defaults. A missing file is not an
// error: the defaults apply.
func LoadDevice(p Paths) (Device, error) {
	d := Defaults()
	data, err := os.ReadFile(p.Config())
	if errors.Is(err, fs.ErrNotExist) {
		return d, nil
	}
	if err != nil {
		return d, err
	}
	if err := json.Unmarshal(data, &d); err != nil {
		return Defaults(), err
	}
	return d, nil
}

// LoadBook reads a book's sidecar; missing means "use the global defaults".
func LoadBook(p Paths, id string) (Book, error) {
	var b Book
	data, err := os.ReadFile(p.Sidecar(id))
	if errors.Is(err, fs.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return b, err
	}
	return b, json.Unmarshal(data, &b)
}

// Position is a reading position. It is stored as a chapter code point
// offset, not a page number, so it survives re-rendering at another font
// size (plan Phase 6). Image marks a glyphless illustration page whose offset
// is that of the following text.
type Position struct {
	Chapter int  `json:"chapter"`
	Offset  int  `json:"offset"`
	Image   bool `json:"image,omitempty"`
	// Page and PageCount are informational (library progress display).
	Page      int    `json:"page"`
	PageCount int    `json:"page_count"`
	Updated   string `json:"updated,omitempty"`
}

// State is state.json.
type State struct {
	LastBook string              `json:"last_book,omitempty"`
	Books    map[string]Position `json:"books"`
}

// StateFile loads and atomically saves state.json.
type StateFile struct {
	mu    sync.Mutex
	path  string
	State State
}

// OpenState loads state.json; a missing or corrupt file starts empty.
func OpenState(p Paths) *StateFile {
	s := &StateFile{path: p.State(), State: State{Books: map[string]Position{}}}
	if data, err := os.ReadFile(s.path); err == nil {
		var st State
		if json.Unmarshal(data, &st) == nil && st.Books != nil {
			s.State = st
		}
	}
	return s
}

// Set records a book's position and saves.
func (s *StateFile) Set(id string, pos Position) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos.Updated = time.Now().UTC().Format(time.RFC3339)
	s.State.Books[id] = pos
	s.State.LastBook = id
	return s.save()
}

// Get returns a book's saved position.
func (s *StateFile) Get(id string) (Position, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.State.Books[id]
	return p, ok
}

func (s *StateFile) save() error {
	return WriteAtomic(s.path, s.State)
}

// WriteAtomic writes v as JSON via a temporary file and rename, so a crash or
// power loss never leaves a half-written file.
func WriteAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
