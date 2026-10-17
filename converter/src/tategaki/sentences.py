"""Sentence spans for the .jbk layout (plan §5.4).

The reference algorithm (calibre `sentence_extraction_reference.py`, twin of
`anki_sentence.pyj`) answers "which sentence contains this offset?", scanning
outward from the offset with bracket-depth tracking. This module reuses that
scan to return spans instead of strings:

- `sentence_bounds(text, offset)` is the reference scan, returning
  `[start, end)` before the reference's `.strip()` and dangling-bracket trim.
- `sentence_spans(text, para_start, para_end)` partitions a paragraph by
  repeatedly taking the sentence that starts at the current position.

Because the reference scan depends on where the tap is (a tap inside a quoted
multi-sentence passage yields the inner sentence, a tap outside it the whole
outer sentence), a fixed partition can't reproduce every answer. The tests check
that each partition span, trimmed, equals the reference answer for its first
character.
"""

BRACKETS = {'「': '」', '『': '』', '（': '）', '(': ')', '【': '】',
            '〈': '〉', '《': '》', '〔': '〕', '｛': '｝', '{': '}', '［': '］', '[': ']'}
OPEN = set(BRACKETS)
CLOSE = set(BRACKETS.values())
DELIMS = set('。！？.!?\n\r')
TRAILING = set('。、！？…‥」』）)】〉》〕｝}］]')


def sentence_bounds(text: str, offset: int, lo: int = 0, hi: int | None = None) -> tuple[int, int]:
    """The reference backward/forward scan, limited to text[lo:hi]."""
    hi = len(text) if hi is None else hi
    depth = 0
    start = lo
    i = offset - 1
    while i >= lo:
        ch = text[i]
        if ch in CLOSE:
            depth += 1
        elif ch in OPEN:
            depth = max(0, depth - 1)
        elif depth == 0 and ch in DELIMS:
            start = i + 1
            break
        i -= 1

    depth = 0
    end = hi
    i = offset
    while i < hi:
        ch = text[i]
        if ch in OPEN:
            depth += 1
        elif ch in CLOSE:
            depth = max(0, depth - 1)
        elif depth == 0 and ch in DELIMS:
            end = i + 1
            while end < hi and text[end] in TRAILING:
                end += 1
            break
        i += 1
    return start, end


def sentence_spans(text: str, para_start: int, para_end: int) -> list[tuple[int, int]]:
    """Partition text[para_start:para_end] into sentence spans."""
    spans = []
    pos = para_start
    while pos < para_end:
        _, end = sentence_bounds(text, pos, para_start, para_end)
        end = max(end, pos + 1)
        spans.append((pos, end))
        pos = end
    return spans
