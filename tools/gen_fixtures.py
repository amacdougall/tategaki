#!/usr/bin/env python3
"""Generate golden fixtures in testdata/ from the Python reference (plan §3).

The Go ports in reader/internal/... must reproduce these exactly.

Usage: uv run --project converter python tools/gen_fixtures.py
"""

import contextlib
import io
import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'converter' / 'src'))
sys.path.insert(0, str(ROOT / 'tools'))

from reference_scan import scan  # noqa: E402
from tategaki.reference import kaishi  # noqa: E402
from tategaki.reference.deinflect import complete_partial_inflection, deinflect  # noqa: E402
from tategaki.reference.jmdict import JMdict  # noqa: E402
from tategaki.reference.lookup import lookup_candidates  # noqa: E402

with contextlib.redirect_stdout(io.StringIO()):  # the module prints its demo cases
    from tategaki.reference import sentence_extraction_reference as sent  # noqa: E402

OUT = ROOT / 'testdata'
DB = ROOT / 'data' / 'jmdict.sqlite'

# Seeded from the Calibre commit messages (plan §3), the reference self-tests,
# and words from the test book.
WORDS = [
    '突き込', '擦ら', '食べ', '並々ならぬ', '悠々', '蜘蛛の巣', 'する', '擦る', '筆者　えーと…',
    '食べている', '美味しかった', '読まない', '行かなかった', '飲みたい', '食べる', '食べていますが',
    '漬け込', '食べら', '読ま', 'はし', 'xyzzy', '蜘蛛', '蜘蛛の', '来た', 'した', '食べました',
    '苛立ち', '曇る', '拭った', '跪き', '掴み', '乱暴', '高須', 'ふんわり', 'うそっぱち',
    '変われる', '勝手', 'ぶちまけ', '悠', 'ゆう', '々', 'え', 'ー',
]

# (text, tapped substring or offset, paragraph end or None)
SCANS = [
    ('蜘蛛の巣が張っている', '蜘蛛'),
    ('蜘蛛の巣が張っている', '巣'),
    ('蜘蛛の巣が張っている', '張'),
    ('蜘蛛の巣が張っている', 'の'),
    ('　苛立ちに任せ、曇る鏡を乱暴に手のひらで拭った。', '苛立'),
    ('　苛立ちに任せ、曇る鏡を乱暴に手のひらで拭った。', '曇'),
    ('　苛立ちに任せ、曇る鏡を乱暴に手のひらで拭った。', '拭'),
    ('　苛立ちに任せ、曇る鏡を乱暴に手のひらで拭った。', '乱'),
    ('　苛立ちに任せ、曇る鏡を乱暴に手のひらで拭った。', '手'),
    ('そして床に跪き、こぼれたゴミを拾い集め、', '跪'),
    ('そして床に跪き、こぼれたゴミを拾い集め、', '拾'),
    ('彼は悠々と歩いていった。', '悠'),
    ('並々ならぬ決意で臨んだ。', '並'),
    ('筆者　えーと……実は今、引っ越しを考えていまして。', '引'),
    ('筆者　えーと……実は今、引っ越しを考えていまして。', '考'),
    ('壁を擦られた跡がある。', '擦'),
    ('食べていますが、まだ足りない。', '食'),
    ('そんなことするわけない', 'する'),
    # longer than scan_len
    ('これは非常に長い文章でありまして説明をさせていただきたく思います', '説明'),
    # paragraph end inside the scan window
    ('猫が好きだ。\n犬も好きだ。', '好'),
    ('ふんわりバングスでソフトな表情', 'ふ'),
]

SPEAKER = [(p, s, e) for p, s, e in sent.SPEAKER_CASES]
SENTENCE_CASES = [(p, s) for p, s, _ in sent.CASES]


def dump(name, data):
    path = OUT / name
    path.write_text(json.dumps(data, ensure_ascii=False, indent=1) + '\n', encoding='utf-8')
    print(f'{path.relative_to(ROOT)}: {len(data)} cases')


def main() -> int:
    OUT.mkdir(exist_ok=True)
    db = JMdict(str(DB))

    dump('deinflect.json', [
        {'source': w, 'deinflect': [[d.term, d.rules, d.reasons] for d in deinflect(w)],
         'complete_partial': [[d.term, d.rules, d.reasons] for d in complete_partial_inflection(w)]}
        for w in WORDS
    ])
    dump('lookup.json', [{'selection': w, 'candidates': lookup_candidates(w, db=db)} for w in WORDS])

    scans = []
    for text, tap in SCANS:
        offset = text.index(tap)
        para_end = text.find('\n', offset)
        para_end = len(text) if para_end < 0 else para_end
        for policy in ('longest', 'frequency'):
            scans.append({'text': text, 'offset': offset, 'para_end': para_end, 'policy': policy,
                          'candidates': scan(db, text, offset, para_end, policy=policy)})
    dump('scan.json', scans)

    dump('kaishi.json', [
        {'args': args, 'fields': kaishi.build_fields(**args)}
        for args in [
            {'word': '擦る', 'sentence': '壁を擦られた跡がある。', 'reading': 'こする', 'meaning': 'to rub; to scrub'},
            {'word': '蜘蛛', 'sentence': '', 'reading': 'くも', 'meaning': 'spider'},
            {'word': '筆者'},
            {'word': '悠々', 'sentence': '彼は悠々と歩いていった。', 'note': 'from とらドラ！'},
        ]
    ])
    dump('sentences.json', [
        {'text': p, 'offset': p.index(s), 'strip_speaker_tag': strip, 'sentence': sent.extract_sentence(p, p.index(s), strip)}
        for p, s in SENTENCE_CASES + [(p, s) for p, s, _ in SPEAKER] for strip in (False, True)
    ])
    return 0


if __name__ == '__main__':
    sys.exit(main())
