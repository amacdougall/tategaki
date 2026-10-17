package dict

import (
	"fmt"
	"sort"
)

// Scan defaults (plan §7.2; config scan_len).
const (
	ScanLen         = 16
	ScanMaxResults  = 32
	PolicyLongest   = "longest"
	PolicyFrequency = "frequency"
)

// Policy orders scan candidates (config ranking_policy, plan §7.3). Ties are
// always broken by ent_seq so results are deterministic.
type Policy func(a, b *Candidate) bool

func cmp3(a, b [3]int) int {
	for i := range a {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// Policies by config name.
var Policies = map[string]Policy{
	// Yomitan-style: the longest match wins, then the shortest deinflection
	// chain, then the most frequent.
	PolicyLongest: func(a, b *Candidate) bool {
		if c := cmp3([3]int{-a.SourceLen, len(a.Reasons), a.Rank}, [3]int{-b.SourceLen, len(b.Reasons), b.Rank}); c != 0 {
			return c < 0
		}
		return a.EntSeq < b.EntSeq
	},
	// Frequency first, then longest.
	PolicyFrequency: func(a, b *Candidate) bool {
		if c := cmp3([3]int{a.Rank, -a.SourceLen, len(a.Reasons)}, [3]int{b.Rank, -b.SourceLen, len(b.Reasons)}); c != 0 {
			return c < 0
		}
		return a.EntSeq < b.EntSeq
	},
}

// Scan looks up the text following a tap (plan §7.2; Python twin in
// tools/reference_scan.py). It tries every prefix of the window
// text[offset : min(offset+scanLen, paraEnd)], longest first, matching each
// deinflection against headwords and readings, filtered by part of speech.
// Each entry keeps its longest match, then its shortest chain. All dictionary
// work is batched into a handful of queries.
//
// offset and paraEnd are code point indices into text.
func (d *DB) Scan(text []rune, offset, paraEnd, scanLen int, policy string, maxResults int) ([]Candidate, error) {
	less, ok := Policies[policy]
	if !ok {
		return nil, fmt.Errorf("unknown ranking policy %q", policy)
	}
	if offset < 0 || offset >= len(text) || paraEnd > len(text) || offset >= paraEnd {
		return []Candidate{}, nil
	}
	window := text[offset:min(offset+scanLen, paraEnd)]

	// Deinflect every prefix; remember the lookup order.
	type step struct {
		n int
		d Deinflection
	}
	var steps []step
	termSet := map[string]bool{}
	var terms []string
	for n := len(window); n >= 1; n-- {
		for _, di := range Deinflect(string(window[:n])) {
			steps = append(steps, step{n, di})
			if !termSet[di.Term] {
				termSet[di.Term] = true
				terms = append(terms, di.Term)
			}
		}
	}
	hits, err := d.LookupExactMany(terms)
	if err != nil {
		return nil, err
	}
	var seqs []int64
	for _, s := range hits {
		seqs = append(seqs, s...)
	}
	entries, ranks, err := d.entriesFor(seqs)
	if err != nil {
		return nil, err
	}

	best := map[int64]*Candidate{}
	for _, st := range steps {
		for _, seq := range hits[st.d.Term] {
			e := entries[seq]
			if !RulesCompatible(st.d.Rules, flatPOS(e.Senses)) {
				continue
			}
			if old, ok := best[seq]; ok {
				if st.n < old.SourceLen || (st.n == old.SourceLen && len(st.d.Reasons) >= len(old.Reasons)) {
					continue
				}
			}
			c := newCandidate(e, st.d.Reasons, st.d.Term, nil, minRank(e, ranks))
			c.SourceLen = st.n
			best[seq] = &c
		}
	}

	list := make([]*Candidate, 0, len(best))
	for _, c := range best {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return less(list[i], list[j]) })
	out := []Candidate{}
	for i := 0; i < len(list) && i < maxResults; i++ {
		out = append(out, *list[i])
	}
	return out, nil
}
