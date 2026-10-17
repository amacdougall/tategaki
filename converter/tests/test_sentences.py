"""Sentence spans must agree with the Python reference splitter (plan §5.4)."""

import contextlib
import importlib.util
import io
from pathlib import Path

import pytest

from tategaki.sentences import sentence_bounds, sentence_spans

REF = (Path(__file__).resolve().parents[3] / 'calibre' / 'src' / 'calibre' / 'gui2' / 'viewer'
       / 'anki_cards' / 'sentence_extraction_reference.py')


@pytest.fixture(scope='module')
def ref():
    if not REF.exists():
        pytest.skip(f'reference not found at {REF}')
    spec = importlib.util.spec_from_file_location('sentence_ref', REF)
    mod = importlib.util.module_from_spec(spec)
    with contextlib.redirect_stdout(io.StringIO()):  # the module prints its demo cases
        spec.loader.exec_module(mod)
    return mod


EXTRA = [
    '朝、七時三十分。',
    '「もうやめだ。どうにもならん」',
    '苛立ちに任せ、曇る鏡を乱暴に手のひらで拭った。ボロい洗面所には目覚ましに浴びたシャワーの湿気がこもったままで、拭ったそばからまた曇る。',
    '「……そしたら、やっちゃんのお昼ごはん……おべんとうは……？　おべんとうのにおいがしない……作ってくれなかったの？」',
    '「え？　なに？　なんで！？」「知らん」と彼は言った。',
    '「閉じない括弧。そのまま。',
]


def paragraphs(ref):
    return [p for p, _, _ in ref.CASES] + [p for p, _, _ in ref.SPEAKER_CASES] + EXTRA


def test_bounds_match_reference_at_every_offset(ref):
    for para in paragraphs(ref):
        for off in range(len(para)):
            a, b = sentence_bounds(para, off)
            assert ref.trim_dangling_brackets(para[a:b].strip()) == ref.extract_sentence(para, off), (para, off)


def test_partition_spans_match_reference_at_span_start(ref):
    for para in paragraphs(ref):
        spans = sentence_spans(para, 0, len(para))
        assert spans[0][0] == 0 and spans[-1][1] == len(para)
        assert all(spans[i][1] == spans[i + 1][0] for i in range(len(spans) - 1))
        for a, b in spans:
            assert ref.trim_dangling_brackets(para[a:b].strip()) == ref.extract_sentence(para, a), (para, a)


def test_spans_respect_paragraph_bounds():
    text = '一つ目。二つ目。\n三つ目'
    assert sentence_spans(text, 0, 8) == [(0, 4), (4, 8)]
    assert sentence_spans(text, 9, 12) == [(9, 12)]
