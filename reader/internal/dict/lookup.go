package dict

import (
	"sort"
	"strings"
)

// Candidate is one dictionary entry offered for a lookup. Field names and JSON
// tags mirror lookup.py's _candidate_entry dict, so golden fixtures decode
// straight into it.
type Candidate struct {
	EntSeq        int64    `json:"ent_seq"`
	Word          string   `json:"word"`
	Reading       string   `json:"reading"`
	Readings      []string `json:"readings"`
	POS           []string `json:"pos"`
	Meaning       string   `json:"meaning"`
	Glosses       []string `json:"glosses"`
	Senses        []Sense  `json:"senses"`
	Reasons       []string `json:"reasons"`
	CompletedFrom *string  `json:"completed_from"`
	Rank          int      `json:"rank"`
	// SourceLen is how many characters of text the candidate explains (scan
	// only; 0 for selection lookups).
	SourceLen int `json:"source_len,omitempty"`
}

// Lookup tuning, as in lookup.py.
const (
	MaxResults   = 16
	TrailingTrim = 3
	PrefixLimit  = 50
)

// Match tiers, most certain first (lookup.py TIER_*).
const (
	TierLiteral = iota
	TierDeinflected
	TierPrefix
	TierTrim
)

func flatPOS(senses []Sense) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range senses {
		for _, p := range s.POS {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

func newCandidate(e *Entry, reasons []string, matchedTerm string, completedFrom *string, rank int) Candidate {
	primary := ""
	for _, r := range e.Readings {
		if r == matchedTerm {
			primary = matchedTerm
		}
	}
	if primary == "" && len(e.Readings) > 0 {
		primary = e.Readings[0]
	}
	word := ""
	if len(e.Kanji) > 0 {
		word = e.Kanji[0]
	} else if len(e.Readings) > 0 {
		word = e.Readings[0]
	}
	glosses := []string{}
	for _, s := range e.Senses {
		glosses = append(glosses, s.Glosses...)
	}
	senses := make([]Sense, len(e.Senses))
	copy(senses, e.Senses)
	if reasons == nil {
		reasons = []string{}
	}
	return Candidate{
		EntSeq: e.EntSeq, Word: word, Reading: primary, Readings: e.Readings, POS: flatPOS(e.Senses),
		Meaning: strings.Join(glosses, "; "), Glosses: glosses, Senses: senses, Reasons: reasons,
		CompletedFrom: completedFrom, Rank: rank,
	}
}

type scored struct {
	tier, chain int
	cand        Candidate
}

// collector keeps the best (tier, chain) per entry in first-insertion order,
// like lookup.py's best dict.
type collector struct {
	order []int64
	best  map[int64]scored
}

func newCollector() *collector { return &collector{best: map[int64]scored{}} }

func (c *collector) consider(tier, chain int, cand Candidate) {
	old, ok := c.best[cand.EntSeq]
	if !ok {
		c.order = append(c.order, cand.EntSeq)
	}
	if !ok || tier < old.tier || (tier == old.tier && chain < old.chain) {
		c.best[cand.EntSeq] = scored{tier, chain, cand}
	}
}

// entriesFor fetches entries plus their ranks.
func (d *DB) entriesFor(seqs []int64) (map[int64]*Entry, map[string]int, error) {
	entries, err := d.Entries(seqs)
	if err != nil {
		return nil, nil, err
	}
	var forms []string
	for _, e := range entries {
		forms = append(forms, rankForms(e)...)
	}
	ranks, err := d.MinRanks(forms)
	return entries, ranks, err
}

func (d *DB) collectExact(surface string, c *collector, forceTier int) error {
	for _, di := range Deinflect(surface) {
		seqs, err := d.LookupExact(di.Term)
		if err != nil {
			return err
		}
		entries, ranks, err := d.entriesFor(seqs)
		if err != nil {
			return err
		}
		for _, seq := range seqs {
			e := entries[seq]
			if !RulesCompatible(di.Rules, flatPOS(e.Senses)) {
				continue
			}
			tier := forceTier
			if tier < 0 {
				tier = TierLiteral
				if len(di.Reasons) > 0 {
					tier = TierDeinflected
				}
			}
			c.consider(tier, len(di.Reasons), newCandidate(e, di.Reasons, di.Term, nil, minRank(e, ranks)))
		}
	}
	return nil
}

func (d *DB) collectPrefix(surface string, limit int, c *collector) error {
	from := surface
	seqs, err := d.LookupPrefix(surface, limit)
	if err != nil {
		return err
	}
	entries, ranks, err := d.entriesFor(seqs)
	if err != nil {
		return err
	}
	for _, seq := range seqs {
		e := entries[seq]
		c.consider(TierPrefix, 0, newCandidate(e, nil, "", &from, minRank(e, ranks)))
	}
	for _, comp := range CompletePartialInflection(surface) {
		seqs, err := d.LookupExact(comp.Term)
		if err != nil {
			return err
		}
		entries, ranks, err := d.entriesFor(seqs)
		if err != nil {
			return err
		}
		for _, seq := range seqs {
			e := entries[seq]
			if !RulesCompatible(comp.Rules, flatPOS(e.Senses)) {
				continue
			}
			c.consider(TierPrefix, len(comp.Reasons), newCandidate(e, comp.Reasons, comp.Term, &from, minRank(e, ranks)))
		}
	}
	return nil
}

// LookupCandidates ports lookup.py's lookup_candidates: candidates for an
// exact selection, tiered literal > deinflected > forward completion >
// trailing-trim. The reader uses it for manual span adjustment (plan §7.5).
func (d *DB) LookupCandidates(selection string, maxResults int) ([]Candidate, error) {
	surface := strings.TrimSpace(selection)
	if surface == "" {
		return []Candidate{}, nil
	}
	c := newCollector()
	if err := d.collectExact(surface, c, -1); err != nil {
		return nil, err
	}
	if err := d.collectPrefix(surface, PrefixLimit, c); err != nil {
		return nil, err
	}
	if len(c.order) == 0 {
		r := []rune(surface)
		minLen := max(1, len(r)-TrailingTrim)
		for n := len(r) - 1; n >= minLen; n-- {
			if err := d.collectExact(string(r[:n]), c, TierTrim); err != nil {
				return nil, err
			}
			if len(c.order) > 0 {
				break
			}
		}
	}
	list := make([]scored, 0, len(c.order))
	for _, seq := range c.order {
		list = append(list, c.best[seq])
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		if a.tier != b.tier {
			return a.tier < b.tier
		}
		if a.chain != b.chain {
			return a.chain < b.chain
		}
		return a.cand.Rank < b.cand.Rank
	})
	out := []Candidate{}
	for i := 0; i < len(list) && i < maxResults; i++ {
		out = append(out, list[i].cand)
	}
	return out, nil
}
