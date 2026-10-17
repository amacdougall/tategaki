// Package anki mines cards: the Kaishi 1.5k field mapping (port of kaishi.py),
// an AnkiConnect client (port of anki_connect.py), and the on-device queue
// (plan §7.7: queue first, flush when Anki is reachable).
package anki

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Kaishi 1.5k field names (kaishi.py).
const (
	ModelName   = "Kaishi 1.5k"
	Word        = "Word"
	WordReading = "Word Reading"
	WordMeaning = "Word Meaning"
	Sentence    = "Sentence"
	Notes       = "Notes"
)

// BuildFields fills the fields the reader can supply; empty values are
// omitted so Anki leaves those fields blank (kaishi.build_fields).
func BuildFields(word, sentence, reading, meaning, note string) map[string]string {
	f := map[string]string{Word: word}
	if sentence != "" {
		f[Sentence] = sentence
	}
	if reading != "" {
		f[WordReading] = reading
	}
	if meaning != "" {
		f[WordMeaning] = meaning
	}
	if note != "" {
		f[Notes] = note
	}
	return f
}

// Note is an AnkiConnect note object (anki_connect.build_note). A nil
// DuplicateScope makes duplicate detection collection-wide.
type Note struct {
	DeckName  string            `json:"deckName"`
	ModelName string            `json:"modelName"`
	Fields    map[string]string `json:"fields"`
	Tags      []string          `json:"tags"`
	Options   NoteOptions       `json:"options"`
}

// NoteOptions are AnkiConnect's per-note options.
type NoteOptions struct {
	AllowDuplicate bool    `json:"allowDuplicate"`
	DuplicateScope *string `json:"duplicateScope"`
}

// ErrUnavailable means AnkiConnect couldn't be reached (Anki closed, Wi-Fi
// off, wrong address). Queue items stay pending.
var ErrUnavailable = errors.New("AnkiConnect unavailable")

// Client talks to AnkiConnect's HTTP API (version 6).
type Client struct {
	URL     string
	APIKey  string
	Timeout time.Duration
}

// Invoke calls an action and decodes its result into out (if non-nil).
func (c *Client) Invoke(action string, params map[string]any, out any) error {
	body := map[string]any{"action": action, "version": 6}
	if params != nil {
		body["params"] = params
	}
	if c.APIKey != "" {
		body["key"] = c.APIKey
	}
	payload, _ := json.Marshal(body)
	hc := http.Client{Timeout: c.Timeout}
	resp, err := hc.Post(c.URL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("%w at %s: %v", ErrUnavailable, c.URL, err)
	}
	defer resp.Body.Close()
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  *string         `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return fmt.Errorf("unexpected AnkiConnect response (HTTP %d): %v", resp.StatusCode, err)
	}
	if r.Error != nil {
		return errors.New(*r.Error)
	}
	if out != nil {
		return json.Unmarshal(r.Result, out)
	}
	return nil
}

// CanAddNote reports whether the note would be added (false for a duplicate).
func (c *Client) CanAddNote(n Note) (bool, error) {
	var ok []bool
	if err := c.Invoke("canAddNotes", map[string]any{"notes": []Note{n}}, &ok); err != nil {
		return false, err
	}
	return len(ok) == 1 && ok[0], nil
}

// AddNote adds a note and returns its id.
func (c *Client) AddNote(n Note) (int64, error) {
	var id int64
	err := c.Invoke("addNote", map[string]any{"note": n}, &id)
	return id, err
}
