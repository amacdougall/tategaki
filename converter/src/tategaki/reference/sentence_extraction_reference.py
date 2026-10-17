# Reference implementation + regression tests for bracket-depth-aware Japanese
# sentence extraction. This is the Python twin of the algorithm in
# src/pyj/read_book/anki_sentence.pyj (extract_sentence / trim_dangling_brackets);
# pyj is Python-syntax so the two stay line-for-line equivalent. Runnable without
# a Calibre build:  python3 sentence_extraction_reference.py

import re

BRACKETS = {'「': '」', '『': '』', '（': '）', '(': ')', '【': '】',
            '〈': '〉', '《': '》', '〔': '〕', '｛': '｝', '{': '}', '［': '］', '[': ']'}
OPEN = set(BRACKETS.keys())
CLOSE = set(BRACKETS.values())
DELIMS = set('。！？.!?\n\r')
# chars allowed to be pulled in *after* a terminal delimiter (closing quotes, etc.)
TRAILING = set('。、！？…‥」』）)】〉》〕｝}］]')
WHITESPACE = set(' \t\u3000')
SPEAKER_TAG = re.compile('^[\u4e00-\u9fff々〆ヶ]{1,4}\u3000')


def speaker_tag_length(text, start):
    # Leading dialogue speaker tag (1-4 kanji + full-width space, e.g. "筆者　").
    m = SPEAKER_TAG.match(text[start:start + 5])
    return len(m.group()) if m else 0


def extract_sentence(text, offset, strip_speaker_tag=False):
    # --- scan backward to sentence start, ignoring delimiters inside brackets ---
    depth = 0
    start = 0
    i = offset - 1
    while i >= 0:
        ch = text[i]
        if ch in CLOSE:          # entering a quote from the back
            depth += 1
        elif ch in OPEN:         # leaving it
            depth = max(0, depth - 1)
        elif depth == 0 and ch in DELIMS:
            start = i + 1
            break
        i -= 1

    # --- scan forward to sentence end, ignoring delimiters inside brackets ---
    depth = 0
    end = len(text)
    i = offset
    while i < len(text):
        ch = text[i]
        if ch in OPEN:
            depth += 1
        elif ch in CLOSE:
            depth = max(0, depth - 1)
        elif depth == 0 and ch in DELIMS:
            end = i + 1
            while end < len(text) and text[end] in TRAILING:
                end += 1
            break
        i += 1

    # --- optionally drop a speaker tag, only at the start of a paragraph/line
    # and only when the selection is past it (so the tag itself can be mined) ---
    if strip_speaker_tag and (start == 0 or text[start - 1] in '\n\r'):
        while start < offset and text[start] in WHITESPACE:
            start += 1
        tag_len = speaker_tag_length(text, start)
        if tag_len and offset >= start + tag_len:
            start += tag_len

    return trim_dangling_brackets(text[start:end].strip())


def trim_dangling_brackets(s):
    # Depth-aware scanning can land between bracket pairs (multi-sentence quotes),
    # leaving a stray 「 at the front or 」 at the back. Strip any bracket char at
    # an edge that has no partner inside the slice.
    changed = True
    while changed and len(s) > 1:
        changed = False
        first = s[0]
        if first in CLOSE or (first in OPEN and BRACKETS[first] not in s[1:]):
            s = s[1:].strip(); changed = True; continue
        last = s[-1]
        if last in OPEN:
            s = s[:-1].strip(); changed = True; continue
        if last in CLOSE:
            opener = next(o for o, c in BRACKETS.items() if c == last)
            if opener not in s[:-1]:
                s = s[:-1].strip(); changed = True; continue
    return s


CASES = [
    # (paragraph, selected substring, label)
    ('田中さんは「ちょっと待って！」と言った。', '言った', 'word AFTER internal-quote'),
    ('田中さんは「ちょっと待って！」と言った。', '待って', 'word INSIDE quote'),
    ('田中さんは「ちょっと待って！」と言った。', '田中', 'word BEFORE quote'),
    ('猫が好きだ。犬も好きだ。鳥はどうかな。', '犬', 'plain middle sentence'),
    ('「でもはいらないよ。とにかく明日は宿を探そう。だから、そんなに気を張らないでいいよ」', '気を張', 'HoshiReader case A'),
    ('「でもはいらないよ。とにかく明日は宿を探そう。だから、そんなに気を張らないでいいよ」', 'でも', 'HoshiReader case B'),
    ('彼は『吾輩は猫である。名前はまだ無い。』を読んだ。', '読んだ', 'nested book-title quote'),
]

SPEAKER_CASES = [
    # (paragraph, selected substring, expected sentence with strip_speaker_tag=True)
    ('筆者　えーと……実は今、引っ越しを考えていまして。', '引っ越し', 'えーと……実は今、引っ越しを考えていまして。'),
    ('筆者　えーと……実は今、引っ越しを考えていまして。', '筆者', '筆者　えーと……実は今、引っ越しを考えていまして。'),
    ('栗原　やっぱり私は、殺された四人目がいると思うんですよ。これを見てください。', '見て', 'これを見てください。'),
    ('内側から鍵がかかっていたのではないですか？', '鍵', '内側から鍵がかかっていたのではないですか？'),
    ('　筆者は家に入った。', '家', '筆者は家に入った。'),
    ('猫が好きだ。\n片淵　どうでしょう。', 'どう', 'どうでしょう。'),
]

for para, sub, label in CASES:
    off = para.index(sub)
    print(f'{label:28} | sel={sub!r:8} -> {extract_sentence(para, off)!r}')

for para, sub, expected in SPEAKER_CASES:
    got = extract_sentence(para, para.index(sub), strip_speaker_tag=True)
    print(f'{"OK " if got == expected else "FAIL"} speaker | sel={sub!r:8} -> {got!r}')
