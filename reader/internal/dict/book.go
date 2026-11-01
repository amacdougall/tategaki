package dict

import (
	"sort"
	"strings"
)

// BookWord is one book-specific dictionary entry from book.toml's
// [[dictionary]] (usually a proper name JMdict lacks). Reading may be empty.
type BookWord struct {
	Kanji      string `json:"kanji"`
	Reading    string `json:"reading"`
	Definition string `json:"definition"`
}

// BookDict holds a book's own entries. They are matched literally (names
// don't inflect) against the kanji or the reading, and are always offered
// ahead of JMdict: the reader added them on purpose for this book.
type BookDict []BookWord

// candidate is entry i as a lookup row. Book entries get negative EntSeqs so
// they never collide with JMdict's, and no rank, so no badge.
func (b BookDict) candidate(i int, sourceLen int) Candidate {
	w := b[i]
	readings := []string{}
	if w.Reading != "" {
		readings = []string{w.Reading}
	}
	glosses := []string{w.Definition}
	return Candidate{
		EntSeq: -int64(i + 1), Word: w.Kanji, Reading: w.Reading, Readings: readings, POS: []string{},
		Meaning: w.Definition, Glosses: glosses, Senses: []Sense{{POS: []string{}, Glosses: glosses}},
		Reasons: []string{}, Rank: Unranked, SourceLen: sourceLen, Book: true,
	}
}

func (w BookWord) forms() []string {
	if w.Reading == "" || w.Reading == w.Kanji {
		return []string{w.Kanji}
	}
	return []string{w.Kanji, w.Reading}
}

// Scan returns the entries that text[offset:paraEnd] starts with, longest
// first (then in book.toml order). Unlike the JMdict scan, there's no
// scan_len window: a long name still matches.
func (b BookDict) Scan(text []rune, offset, paraEnd int) []Candidate {
	if offset < 0 || offset >= len(text) || paraEnd > len(text) || offset >= paraEnd {
		return nil
	}
	rest := string(text[offset:paraEnd])
	var out []Candidate
	for i, w := range b {
		best := ""
		for _, f := range w.forms() {
			if f != "" && strings.HasPrefix(rest, f) && len([]rune(f)) > len([]rune(best)) {
				best = f
			}
		}
		if best != "" {
			out = append(out, b.candidate(i, len([]rune(best))))
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].SourceLen > out[j].SourceLen })
	return out
}

// Exact returns the entries whose kanji or reading equals the selection (the
// span-adjust path), in book.toml order.
func (b BookDict) Exact(selection string) []Candidate {
	s := strings.TrimSpace(selection)
	var out []Candidate
	for i, w := range b {
		for _, f := range w.forms() {
			if f == s {
				out = append(out, b.candidate(i, 0))
				break
			}
		}
	}
	return out
}
