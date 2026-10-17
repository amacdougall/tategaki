"""Text/glyph extraction in Chromium (plan §5.2): code point offsets, ruby
exclusion, paragraph boundaries, illustration markers."""

from pathlib import Path

import pytest
from playwright.sync_api import sync_playwright

from tategaki.render import Profile, RenderSettings, _css, _load, _paragraphs

FIXTURE = Path(__file__).parent / 'fixtures' / 'mini.xhtml'
FONT = Path.home() / 'Tategaki' / 'fonts' / 'NotoSerifJP-Regular.otf'


@pytest.fixture(scope='module')
def collected():
    profile = Profile.load('pw11')
    with sync_playwright() as pw:
        browser = pw.chromium.launch()
        ctx = browser.new_context(java_script_enabled=False, viewport={'width': profile.width, 'height': profile.height})
        page = ctx.new_page()
        _load(page, FIXTURE.resolve().as_uri(), _css(RenderSettings(font=FONT), profile))
        illus = page.evaluate('() => TG.extractIllustrations()')
        groups = page.evaluate('() => TG.collectRubyGroups()')
        data = page.evaluate('() => TG.collect()')
        browser.close()
    return illus, groups, data


def test_text_excludes_ruby_and_separates_blocks(collected):
    _, _, data = collected
    assert data['text'] == '　悠々と𠮷野家へ行った。\n「待って！」と言った。猫が好きだ。\nspan across lines\n最後の文。'


def test_offsets_are_code_points(collected):
    _, _, data = collected
    text = data['text']
    by_offset = {g[0]: g for g in data['glyphs']}
    # 𠮷 (U+20BB7) is a surrogate pair in JS but one code point here
    assert text[4] == '𠮷' and 4 in by_offset and 5 in by_offset
    assert text[5] == '野'
    # every glyph offset points at a non-space character of the text
    for off, *_ in data['glyphs']:
        assert text[off] not in ' \n'
    # every visible character has exactly one glyph
    visible = [i for i, c in enumerate(text) if c not in ' \n']
    assert sorted(by_offset) == visible


def test_ids_map_to_offsets(collected):
    _, _, data = collected
    assert data['ids']['p1'] == 0
    assert data['text'][data['ids']['last']] == '最'


def test_illustration_block_becomes_marker(collected):
    illus, _, data = collected
    assert len(illus['sources']) == 1 and illus['sources'][0].endswith('pixel.png')
    assert not illus['onlyIllustrations']
    assert len(data['markers']) == 1
    assert data['text'][data['markers'][0][2]] == '最'  # marker sits before the last paragraph


def test_ruby_groups(collected):
    _, groups, _ = collected
    assert [(g['base'], g['exempt']) for g in groups] == [('悠', False), ('𠮷', False)]
    assert groups[0]['following'].startswith('々と')


def test_paragraph_spans(collected):
    _, _, data = collected
    text = data['text']
    spans = _paragraphs(text)
    assert [text[a:b] for a, b in spans] == text.split('\n')


def test_ruby_bands_merge_with_their_base_column(collected):
    from tategaki.paginate import merge_bands
    _, _, data = collected
    merged = merge_bands([tuple(b) for b in data['bands']])
    glyph_cols = merge_bands([(g[1] + g[3] / 2 - 1, g[1] + g[3] / 2 + 1) for g in data['glyphs']])
    # every merged band contains at least one glyph column: no ruby-only bands
    for a, b in merged:
        assert any(a <= x0 and x1 <= b for x0, x1 in glyph_cols), (a, b)
