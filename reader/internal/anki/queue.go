package anki

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"
	"time"
)

// Status of a queued card (plan §7.7).
const (
	Pending   = "pending"
	Sent      = "sent"
	Duplicate = "duplicate"
	Failed    = "error"
)

// Item is one line of anki-queue.jsonl.
type Item struct {
	ID        string            `json:"id"`
	CreatedAt string            `json:"created_at"`
	Book      string            `json:"book"`
	Deck      string            `json:"deck"`
	Model     string            `json:"model"`
	Fields    map[string]string `json:"fields"`
	Tags      []string          `json:"tags"`
	Status    string            `json:"status"`
	Error     string            `json:"error,omitempty"`
	NoteID    int64             `json:"note_id,omitempty"`
	// EntSeq is the JMdict entry that was mined (0 for a raw selection); used
	// to show each popup row's state.
	EntSeq int64 `json:"ent_seq,omitempty"`
}

// Note builds the AnkiConnect note for an item, collection-wide duplicate
// scope, as the Calibre viewer does.
func (it Item) Note() Note {
	tags := it.Tags
	if tags == nil {
		tags = []string{}
	}
	return Note{DeckName: it.Deck, ModelName: it.Model, Fields: it.Fields, Tags: tags}
}

// Queue is anki-queue.jsonl. Every change rewrites the file atomically.
type Queue struct {
	mu      sync.Mutex
	flushMu sync.Mutex // one flush at a time, so no item is sent twice
	path    string
	items   []Item
}

// OpenQueue loads the queue; a missing file is an empty queue. Unparseable
// lines are kept out of the queue but reported.
func OpenQueue(path string) (*Queue, error) {
	q := &Queue{path: path}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return q, nil
	}
	if err != nil {
		return q, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	var bad int
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var it Item
		if json.Unmarshal([]byte(line), &it) != nil {
			bad++
			continue
		}
		q.items = append(q.items, it)
	}
	if bad > 0 {
		return q, fmt.Errorf("%s: skipped %d unreadable lines", path, bad)
	}
	return q, sc.Err()
}

// Items returns a copy of the queue.
func (q *Queue) Items() []Item {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]Item(nil), q.items...)
}

// Pending counts items not yet delivered.
func (q *Queue) Pending() int {
	n := 0
	for _, it := range q.Items() {
		if it.Status == Pending {
			n++
		}
	}
	return n
}

// Append adds a pending item and saves.
func (q *Queue) Append(it Item) (Item, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	if it.ID == "" {
		it.ID = fmt.Sprintf("%d-%d", now.UnixNano(), len(q.items))
	}
	it.CreatedAt = now.UTC().Format(time.RFC3339)
	it.Status = Pending
	q.items = append(q.items, it)
	return it, q.save()
}

// Latest returns the newest item for (book, ent_seq, word), if any.
func (q *Queue) Latest(book string, entSeq int64, word string) (Item, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i := len(q.items) - 1; i >= 0; i-- {
		it := q.items[i]
		if it.Book == book && it.EntSeq == entSeq && it.Fields[Word] == word {
			return it, true
		}
	}
	return Item{}, false
}

// InAnki reports whether a card with this Word, from any book, was sent or
// found to be a duplicate: Anki has it, even if the deployed word list
// predates it.
func (q *Queue) InAnki(word string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for _, it := range q.items {
		if it.Fields[Word] == word && (it.Status == Sent || it.Status == Duplicate) {
			return true
		}
	}
	return false
}

func (q *Queue) save() error {
	tmp := q.path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	for _, it := range q.items {
		line, _ := json.Marshal(it)
		w.Write(line)
		w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
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
	return os.Rename(tmp, q.path)
}

// FlushResult summarises a flush.
type FlushResult struct {
	Sent, Duplicates, Errors, Pending int
	// Unavailable is set when AnkiConnect couldn't be reached; the remaining
	// items stay pending.
	Unavailable error
}

func (r FlushResult) String() string {
	if r.Unavailable != nil {
		return fmt.Sprintf("Anki not reachable; %d waiting", r.Pending)
	}
	parts := []string{fmt.Sprintf("%d sent", r.Sent)}
	if r.Duplicates > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", r.Duplicates)) // already in Anki
	}
	if r.Errors > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", r.Errors))
	}
	return strings.Join(parts, ", ")
}

// Flush sends every pending item: canAddNotes first (false means a
// duplicate, collection-wide), then addNote. It stops at the first
// connection failure, leaving the rest pending. The file is rewritten after
// each item, so an interrupted flush loses nothing.
func (q *Queue) Flush(c *Client) FlushResult {
	q.flushMu.Lock()
	defer q.flushMu.Unlock()
	var res FlushResult
	for i := range q.Items() {
		q.mu.Lock()
		it := q.items[i]
		q.mu.Unlock()
		if it.Status != Pending {
			continue
		}
		if res.Unavailable != nil {
			res.Pending++
			continue
		}
		status, errText, noteID := deliver(c, it)
		if status == Pending {
			res.Unavailable = errors.New(errText)
			res.Pending++
			continue
		}
		q.mu.Lock()
		q.items[i].Status, q.items[i].Error, q.items[i].NoteID = status, errText, noteID
		err := q.save()
		q.mu.Unlock()
		if err != nil {
			res.Errors++
			continue
		}
		switch status {
		case Sent:
			res.Sent++
		case Duplicate:
			res.Duplicates++
		default:
			res.Errors++
		}
	}
	return res
}

func deliver(c *Client, it Item) (status, errText string, noteID int64) {
	n := it.Note()
	ok, err := c.CanAddNote(n)
	if errors.Is(err, ErrUnavailable) {
		return Pending, err.Error(), 0
	}
	if err != nil {
		return Failed, err.Error(), 0
	}
	if !ok {
		return Duplicate, "", 0
	}
	id, err := c.AddNote(n)
	if errors.Is(err, ErrUnavailable) {
		return Pending, err.Error(), 0
	}
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			return Duplicate, "", 0
		}
		return Failed, err.Error(), 0
	}
	return Sent, "", id
}
