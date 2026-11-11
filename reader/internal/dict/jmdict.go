package dict

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // pure Go, no cgo
)

// Unranked is the rank of a form JPDB doesn't list (sorts last). Matches
// UNRANKED in jmdict.py.
const Unranked = 9_999_999

const unitSep = "\x1f"

// Sense is one numbered meaning.
type Sense struct {
	POS     []string `json:"pos"`
	Glosses []string `json:"glosses"`
}

// Entry is one JMdict entry.
type Entry struct {
	EntSeq   int64
	Kanji    []string
	Readings []string
	Senses   []Sense
}

// DB is a read-only JMdict accessor (port of jmdict.py), with per-process
// entry caching.
type DB struct {
	db      *sql.DB
	entries map[int64]*Entry
}

// Open opens jmdict.sqlite read-only.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro&immutable=1", path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("jmdict %s: %w", path, err)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name = 'kanji'").Scan(&n); err != nil || n == 0 {
		db.Close()
		return nil, fmt.Errorf("jmdict %s: not a JMdict database (%v)", path, err)
	}
	return &DB{db: db, entries: map[int64]*Entry{}}, nil
}

// Close closes the database.
func (d *DB) Close() error { return d.db.Close() }

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func queryInts(db *sql.DB, q string, args ...any) ([]int64, error) {
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// LookupExact returns ent_seqs whose kanji headword or reading equals surface,
// in the same order as jmdict.py's lookup_exact (same SQL).
func (d *DB) LookupExact(surface string) ([]int64, error) {
	if surface == "" {
		return nil, nil
	}
	return queryInts(d.db, "SELECT ent_seq FROM kanji WHERE text = ? UNION SELECT ent_seq FROM reading WHERE text = ?",
		surface, surface)
}

// LookupExactMany maps each term to its matching ent_seqs, ascending, in one
// query. Used by the scan.
func (d *DB) LookupExactMany(terms []string) (map[string][]int64, error) {
	out := map[string][]int64{}
	if len(terms) == 0 {
		return out, nil
	}
	args := make([]any, 0, 2*len(terms))
	for _, t := range terms {
		args = append(args, t)
	}
	args = append(args, args...)
	ph := placeholders(len(terms))
	rows, err := d.db.Query("SELECT text, ent_seq FROM kanji WHERE text IN ("+ph+") UNION "+
		"SELECT text, ent_seq FROM reading WHERE text IN ("+ph+") ORDER BY 1, 2", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t string
		var seq int64
		if err := rows.Scan(&t, &seq); err != nil {
			return nil, err
		}
		out[t] = append(out[t], seq)
	}
	return out, rows.Err()
}

// LookupPrefix returns ent_seqs whose headword or reading starts with surface
// (excluding exact equals), capped at limit. Same SQL as jmdict.py.
func (d *DB) LookupPrefix(surface string, limit int) ([]int64, error) {
	if surface == "" {
		return nil, nil
	}
	r := []rune(surface)
	hi := string(r[:len(r)-1]) + string(r[len(r)-1]+1)
	return queryInts(d.db, "SELECT ent_seq FROM ("+
		"  SELECT text, ent_seq FROM kanji WHERE text >= ? AND text < ? AND text != ? "+
		"  UNION "+
		"  SELECT text, ent_seq FROM reading WHERE text >= ? AND text < ? AND text != ? "+
		") LIMIT ?", surface, hi, surface, surface, hi, surface, limit)
}

// Entries loads entries by ent_seq (cached). Kanji and readings keep JMdict
// order (rowid), senses keep ord order, as jmdict.py's _entry does.
func (d *DB) Entries(seqs []int64) (map[int64]*Entry, error) {
	out := make(map[int64]*Entry, len(seqs))
	var missing []any
	for _, s := range seqs {
		if e, ok := d.entries[s]; ok {
			out[s] = e
		} else if _, dup := out[s]; !dup {
			out[s] = nil
			missing = append(missing, s)
		}
	}
	if len(missing) > 0 {
		ph := placeholders(len(missing))
		fresh := map[int64]*Entry{}
		for _, s := range missing {
			seq := s.(int64)
			fresh[seq] = &Entry{EntSeq: seq, Kanji: []string{}, Readings: []string{}, Senses: []Sense{}}
		}
		for _, tbl := range []string{"kanji", "reading"} {
			rows, err := d.db.Query("SELECT ent_seq, text FROM "+tbl+" WHERE ent_seq IN ("+ph+") ORDER BY ent_seq, rowid", missing...)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var seq int64
				var text string
				if err := rows.Scan(&seq, &text); err != nil {
					rows.Close()
					return nil, err
				}
				if tbl == "kanji" {
					fresh[seq].Kanji = append(fresh[seq].Kanji, text)
				} else {
					fresh[seq].Readings = append(fresh[seq].Readings, text)
				}
			}
			rows.Close()
		}
		rows, err := d.db.Query("SELECT ent_seq, pos, gloss FROM sense WHERE ent_seq IN ("+ph+") ORDER BY ent_seq, ord", missing...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var seq int64
			var pos, gloss sql.NullString
			if err := rows.Scan(&seq, &pos, &gloss); err != nil {
				rows.Close()
				return nil, err
			}
			fresh[seq].Senses = append(fresh[seq].Senses, Sense{POS: split(pos.String), Glosses: split(gloss.String)})
		}
		rows.Close()
		for seq, e := range fresh {
			d.entries[seq] = e
			out[seq] = e
		}
	}
	return out, nil
}

func split(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, unitSep)
}

// rankKey is a frequency row: a written form and the reading it's for.
// Kana headwords have reading "". So does every row of a frequency table
// built before readings were stored (see Ranks).
type rankKey struct{ text, reading string }

// Ranks holds the JPDB ranks for a set of entries' forms.
type Ranks struct {
	byKey map[rankKey]int
	// formsOnly: the table has no reading column, so rows are keyed by form
	// alone and an entry is ranked by its forms, as before readings landed.
	formsOnly bool
}

// Ranks fetches the JPDB ranks of every form of entries.
func (d *DB) Ranks(entries map[int64]*Entry) (Ranks, error) {
	out := Ranks{byKey: map[rankKey]int{}}
	var args []any
	seen := map[string]bool{}
	for _, e := range entries {
		for _, k := range rankKeys(e) {
			if !seen[k.text] {
				seen[k.text] = true
				args = append(args, k.text)
			}
		}
	}
	if len(args) == 0 {
		return out, nil
	}
	in := " FROM frequency WHERE text IN (" + placeholders(len(args)) + ")"
	rows, err := d.db.Query("SELECT text, reading, rank"+in, args...)
	if err != nil {
		// An older jmdict.sqlite (deployed separately from the binary) has no
		// reading column: fall back to its form-only ranks.
		out.formsOnly = true
		rows, err = d.db.Query("SELECT text, '', rank"+in, args...)
		if err != nil {
			return out, nil // no frequency table: everything unranked, as jmdict.py
		}
	}
	defer rows.Close()
	for rows.Next() {
		var k rankKey
		var r int
		if err := rows.Scan(&k.text, &k.reading, &r); err != nil {
			return out, err
		}
		if old, ok := out.byKey[k]; !ok || r < old {
			out.byKey[k] = r
		}
	}
	return out, rows.Err()
}

// rankKeys is what an entry is ranked by (jmdict.py entry_rank): each pair of
// its written forms and readings, so 後方/しりえ doesn't inherit the rank of
// 後 (that's あと); or its readings as kana headwords for kana-only entries.
func rankKeys(e *Entry) []rankKey {
	var out []rankKey
	if len(e.Kanji) == 0 {
		for _, r := range e.Readings {
			out = append(out, rankKey{r, ""})
		}
		return out
	}
	for _, k := range e.Kanji {
		for _, r := range e.Readings {
			out = append(out, rankKey{k, r})
		}
	}
	return out
}

// Of returns e's rank: its most common (form, reading) pair, or Unranked.
func (rs Ranks) Of(e *Entry) int {
	best := Unranked
	for _, k := range rankKeys(e) {
		if rs.formsOnly {
			k.reading = ""
		}
		if r, ok := rs.byKey[k]; ok && r < best {
			best = r
		}
	}
	return best
}
