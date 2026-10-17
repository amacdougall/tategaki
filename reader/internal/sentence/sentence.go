// Package sentence extracts the sentence around a mined word. It ports
// calibre's sentence_extraction_reference.py (the Python twin of
// anki_sentence.pyj) exactly, speaker-tag stripping included, and runs on the
// paragraph text at mining time rather than using the bundle's stored spans,
// because the reference answer depends on where in a quotation the word is
// (see docs/DECISIONS.md, Phase 1).
package sentence

import (
	"strings"
	"unicode"
)

var brackets = map[rune]rune{'「': '」', '『': '』', '（': '）', '(': ')', '【': '】',
	'〈': '〉', '《': '》', '〔': '〕', '｛': '｝', '{': '}', '［': '］', '[': ']'}

var closers = func() map[rune]rune {
	m := map[rune]rune{}
	for o, c := range brackets {
		m[c] = o
	}
	return m
}()

func isOpen(r rune) bool  { _, ok := brackets[r]; return ok }
func isClose(r rune) bool { _, ok := closers[r]; return ok }

const (
	delims   = "。！？.!?\n\r"
	trailing = "。、！？…‥」』）)】〉》〕｝}］]"
	spaces   = " \t　"
)

func isKanjiTag(r rune) bool {
	return (r >= 0x4e00 && r <= 0x9fff) || r == '々' || r == '〆' || r == 'ヶ'
}

// speakerTagLength: a leading dialogue speaker tag is 1–4 kanji followed by
// an ideographic space (筆者　).
func speakerTagLength(text []rune, start int) int {
	n := 0
	for start+n < len(text) && n < 4 && isKanjiTag(text[start+n]) {
		n++
	}
	if n >= 1 && start+n < len(text) && text[start+n] == '　' {
		return n + 1
	}
	return 0
}

// Extract returns the sentence containing offset, scanning outward with
// bracket-depth tracking. With stripSpeakerTag, a speaker tag at the start of
// a paragraph is dropped when the word lies past it.
func Extract(text []rune, offset int, stripSpeakerTag bool) string {
	depth, start := 0, 0
back:
	for i := offset - 1; i >= 0; i-- {
		ch := text[i]
		switch {
		case isClose(ch):
			depth++
		case isOpen(ch):
			depth = max(0, depth-1)
		case depth == 0 && strings.ContainsRune(delims, ch):
			start = i + 1
			break back
		}
	}

	depth = 0
	end := len(text)
	for i := offset; i < len(text); i++ {
		ch := text[i]
		if isOpen(ch) {
			depth++
		} else if isClose(ch) {
			depth = max(0, depth-1)
		} else if depth == 0 && strings.ContainsRune(delims, ch) {
			end = i + 1
			for end < len(text) && strings.ContainsRune(trailing, text[end]) {
				end++
			}
			break
		}
	}

	if stripSpeakerTag && (start == 0 || text[start-1] == '\n' || text[start-1] == '\r') {
		for start < offset && strings.ContainsRune(spaces, text[start]) {
			start++
		}
		if n := speakerTagLength(text, start); n > 0 && offset >= start+n {
			start += n
		}
	}
	return trimDanglingBrackets(pyStrip(string(text[start:end])))
}

// pyStrip is Python's str.strip(): Unicode whitespace on both ends.
func pyStrip(s string) string { return strings.TrimFunc(s, unicode.IsSpace) }

// trimDanglingBrackets strips edge brackets whose partner isn't in the slice.
func trimDanglingBrackets(s string) string {
	r := []rune(s)
	for changed := true; changed && len(r) > 1; {
		changed = false
		first := r[0]
		if isClose(first) || (isOpen(first) && !containsRune(r[1:], brackets[first])) {
			r = []rune(pyStrip(string(r[1:])))
			changed = true
			continue
		}
		last := r[len(r)-1]
		if isOpen(last) {
			r = []rune(pyStrip(string(r[:len(r)-1])))
			changed = true
			continue
		}
		if isClose(last) && !containsRune(r[:len(r)-1], closers[last]) {
			r = []rune(pyStrip(string(r[:len(r)-1])))
			changed = true
			continue
		}
	}
	return string(r)
}

func containsRune(r []rune, x rune) bool {
	for _, c := range r {
		if c == x {
			return true
		}
	}
	return false
}
