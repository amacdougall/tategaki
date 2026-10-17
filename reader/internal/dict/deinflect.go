// Package dict is the lookup engine: deinflection, the JMdict accessor, and
// candidate ranking. It ports the Calibre fork's anki_cards Python modules
// (deinflect.py, jmdict.py, lookup.py) and adds the tap scan (plan §7.2).
// testdata/ golden fixtures from tools/gen_fixtures.py prove parity.
package dict

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Rule-type bit flags, as in Yomichan and deinflect.py.
const (
	RuleV1   uint32 = 1 << 0 // ichidan verb
	RuleV5   uint32 = 1 << 1 // godan verb
	RuleVS   uint32 = 1 << 2 // suru verb
	RuleVK   uint32 = 1 << 3 // kuru verb
	RuleVZ   uint32 = 1 << 4 // zuru verb
	RuleAdjI uint32 = 1 << 5 // i-adjective
	RuleIru  uint32 = 1 << 6 // intermediate -iru endings
)

var ruleTypes = map[string]uint32{
	"v1": RuleV1, "v5": RuleV5, "vs": RuleVS, "vk": RuleVK, "vz": RuleVZ, "adj-i": RuleAdjI, "iru": RuleIru,
}

// The Yomichan rule table, verbatim (GPL-3.0); see deinflect.py.
//
//go:embed deinflect_rules.json
var rulesJSON []byte

type rule struct {
	kanaIn, kanaOut string
	inLen, outLen   int // in code points
	inMask, outMask uint32
	reason          string
}

// rules is the flattened table, in file order: deinflect.py iterates a JSON
// object, whose order Python preserves, and result order is part of parity.
var rules = mustLoadRules(rulesJSON)

func mustLoadRules(data []byte) []rule {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	expect := func(want json.Delim) {
		t, err := dec.Token()
		if err != nil || t != want {
			panic(fmt.Sprintf("deinflect rules: expected %v, got %v (%v)", want, t, err))
		}
	}
	mask := func(names []string) uint32 {
		var m uint32
		for _, n := range names {
			m |= ruleTypes[n]
		}
		return m
	}
	var out []rule
	expect('{')
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			panic(err)
		}
		reason := t.(string)
		var variants []struct {
			KanaIn   string   `json:"kanaIn"`
			KanaOut  string   `json:"kanaOut"`
			RulesIn  []string `json:"rulesIn"`
			RulesOut []string `json:"rulesOut"`
		}
		if err := dec.Decode(&variants); err != nil {
			panic(err)
		}
		for _, v := range variants {
			out = append(out, rule{
				kanaIn: v.KanaIn, kanaOut: v.KanaOut,
				inLen: utf8.RuneCountInString(v.KanaIn), outLen: utf8.RuneCountInString(v.KanaOut),
				inMask: mask(v.RulesIn), outMask: mask(v.RulesOut), reason: reason,
			})
		}
	}
	expect('}')
	return out
}

// Deinflection is one candidate dictionary form.
type Deinflection struct {
	Term    string
	Rules   uint32   // expected part-of-speech classes; 0 = unknown (surface form)
	Reasons []string // inflections applied, outermost first
}

// Deinflect returns every candidate deinflection of source, breadth first.
// The first result is always source itself. It over-generates on purpose;
// candidates are filtered against JMdict by RulesCompatible.
func Deinflect(source string) []Deinflection {
	results := []Deinflection{{Term: source, Reasons: []string{}}}
	for i := 0; i < len(results); i++ {
		d := results[i]
		termLen := utf8.RuneCountInString(d.Term)
		for _, r := range rules {
			if d.Rules != 0 && d.Rules&r.inMask == 0 {
				continue
			}
			if !strings.HasSuffix(d.Term, r.kanaIn) {
				continue
			}
			if termLen-r.inLen+r.outLen <= 0 {
				continue
			}
			reasons := make([]string, 0, len(d.Reasons)+1)
			reasons = append(append(reasons, r.reason), d.Reasons...)
			results = append(results, Deinflection{
				Term:    d.Term[:len(d.Term)-len(r.kanaIn)] + r.kanaOut,
				Rules:   r.outMask,
				Reasons: reasons,
			})
		}
	}
	return results
}

// CompletePartialInflection guesses dictionary forms for a selection that
// stops part-way through an inflection (擦ら -> 擦る): for every rule whose
// inflected suffix source ends with a proper prefix of, strip that partial
// suffix and append the rule's output.
func CompletePartialInflection(source string) []Deinflection {
	type key struct {
		base   string
		mask   uint32
		reason string
	}
	seen := map[key]bool{}
	var results []Deinflection
	for _, r := range rules {
		in := []rune(r.kanaIn)
		for plen := 1; plen < len(in); plen++ {
			partial := string(in[:plen])
			if !strings.HasSuffix(source, partial) {
				continue
			}
			stem := source[:len(source)-len(partial)]
			if stem == "" {
				continue
			}
			base := stem + r.kanaOut
			if base == source {
				continue
			}
			k := key{base, r.outMask, r.reason}
			if seen[k] {
				continue
			}
			seen[k] = true
			results = append(results, Deinflection{Term: base, Rules: r.outMask, Reasons: []string{r.reason}})
		}
	}
	return results
}

// posMask maps one JMdict part-of-speech tag to a rule bit (0 if none).
func posMask(pos string) uint32 {
	switch {
	case strings.HasPrefix(pos, "v1"):
		return RuleV1
	case strings.HasPrefix(pos, "v5"):
		return RuleV5
	case strings.HasPrefix(pos, "vs"):
		return RuleVS
	case pos == "vk":
		return RuleVK
	case pos == "vz":
		return RuleVZ
	case strings.HasPrefix(pos, "adj-i"):
		return RuleAdjI
	}
	return 0
}

// RulesCompatible reports whether an entry with these part-of-speech tags is
// a valid target for a deinflection with the given rule flags.
func RulesCompatible(candidateRules uint32, pos []string) bool {
	if candidateRules == 0 {
		return true
	}
	var m uint32
	for _, p := range pos {
		m |= posMask(p)
	}
	return candidateRules&m != 0
}
