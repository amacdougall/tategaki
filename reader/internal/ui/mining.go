package ui

import (
	"log"
	"time"

	"github.com/amacdougall/tategaki/reader/internal/anki"
	"github.com/amacdougall/tategaki/reader/internal/config"
	"github.com/amacdougall/tategaki/reader/internal/sentence"
)

// flushTimeout bounds each AnkiConnect request (plan §7.7: a 2-second try
// after each add).
const flushTimeout = 2 * time.Second

func (a *App) ankiClient() *anki.Client {
	return &anki.Client{URL: a.Dev.AnkiConnectURL, APIKey: a.Dev.AnkiAPIKey, Timeout: flushTimeout}
}

// FlushAnki sends pending cards in the background, then runs done (if any)
// on the event loop.
func (a *App) FlushAnki(done func(anki.FlushResult)) {
	q, client := a.Queue, a.ankiClient()
	go func() {
		start := time.Now()
		res := q.Flush(client)
		log.Printf("ANKI flush: %s (%v)", res, time.Since(start).Round(time.Millisecond))
		if res.Unavailable != nil {
			log.Printf("ANKI unavailable: %v", res.Unavailable)
		}
		if done != nil {
			a.Post(func() { done(res) })
		}
	}()
}

// ankiStatus is a popup row's button label; done is false for "+ Anki".
func ankiStatus(it anki.Item, ok bool) (label string, done bool) {
	if !ok {
		return "+ Anki", false
	}
	switch it.Status {
	case anki.Pending:
		return "Queued", true
	case anki.Sent:
		return "Added ✓", true
	case anki.Duplicate:
		return "Exists", true
	default:
		return "Error", true
	}
}

// mine queues a card for popup row i (plan §7.6) and tries to send it.
func (r *Reader) mine(a *App, i int) error {
	p := r.popup
	text, err := r.chapterText(p.chapter)
	if err != nil {
		return err
	}
	side, err := config.LoadBook(a.Paths, r.id)
	if err != nil {
		log.Printf("sidecar %s: %v", r.id, err)
	}
	// The sentence comes from the paragraph, as in the Calibre viewer, with
	// the book's speaker-tag setting.
	para := text[p.paraStart:p.paraEnd]
	sent := sentence.Extract(para, p.offset-p.paraStart, side.StripSpeakerTags)

	var word, reading, meaning string
	var entSeq int64
	if p.raw || len(p.cands) == 0 {
		word = r.popupText(p)
	} else {
		c := p.cands[i]
		word, reading, meaning, entSeq = c.Word, c.Reading, c.Meaning, c.EntSeq
	}
	deck := a.Dev.DefaultDeck
	if side.Deck != nil && *side.Deck != "" {
		deck = *side.Deck
	}
	tags := side.Tags
	if tags == nil {
		tags = []string{"tategaki"}
	}
	_, err = a.Queue.Append(anki.Item{Book: r.id, Deck: deck, Model: a.Dev.NoteType, Tags: tags, EntSeq: entSeq,
		Fields: anki.BuildFields(word, sent, reading, meaning, "")})
	if err != nil {
		return err
	}
	log.Printf("ANKI queued %q (deck %q) sentence %q", word, deck, sent)
	r.showPopup(a)
	a.FlushAnki(func(anki.FlushResult) {
		if r.popup == p && a.screen == r {
			r.showPopup(a) // update the row's status
		}
	})
	return nil
}
