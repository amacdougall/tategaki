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


FRONTMATTER = Path(__file__).parent / 'fixtures' / 'frontmatter.xhtml'


class _Writer:
    """Just enough BundleWriter for Renderer._text_page."""
    def __init__(self):
        self.pages = []
        self.page_count = 0

    def add_page(self, img):
        self.pages.append(img)
        self.page_count += 1
        return self.page_count - 1

    def add_image(self, key, img):
        return f'images/{key}.png'


@pytest.fixture(scope='module')
def frontmatter():
    """A chapter opening like Date A Live's: a 1x1 spacer image, a 1.3em
    heading whose ruby is flush with the right margin, and a small logo alone
    in a block."""
    from tategaki.paginate import Cut, paginate
    from tategaki.render import Renderer
    profile = Profile.load('pw11')
    s = RenderSettings(font=FONT, font_size_pt=14)
    logs = []
    with sync_playwright() as pw:
        browser = pw.chromium.launch()
        ctx = browser.new_context(java_script_enabled=False, device_scale_factor=1,
                                  viewport={'width': profile.width, 'height': profile.height})
        page = ctx.new_page()
        _load(page, FRONTMATTER.resolve().as_uri(), _css(s, profile))
        illus = page.evaluate('() => TG.extractIllustrations()')
        data = page.evaluate('() => TG.collect()')
        cuts = paginate([tuple(b) for b in data['bands']], [(m[0], m[1]) for m in data['markers']],
                        profile.content_width)
        writer = _Writer()
        renderer = Renderer(profile, log=logs.append)
        text_cuts = [c for c in cuts if isinstance(c, Cut)]
        pages = [renderer._text_page(page, writer, c, data, s, None) for c in text_cuts]
        browser.close()
    return profile, illus, data, cuts, text_cuts, writer, pages, logs


def test_spacer_images_are_dropped_and_small_images_stay_inline(frontmatter):
    _, illus, data, cuts, *_ = frontmatter
    assert illus['sources'] == [] and not illus['onlyIllustrations']
    assert data['markers'] == []
    assert [im['src'].rsplit('/', 1)[-1] for im in data['inlineImages']] == ['logo.png']
    assert len(cuts) == 1  # no forced breaks: heading, text, logo and label share a page


def test_page_flush_with_right_margin_is_not_blank(frontmatter):
    profile, _, _, _, cuts, writer, pages, logs = frontmatter
    # precondition: the heading's column reaches the margin, so the naive
    # scroll target is 0, where headless Chromium paints nothing
    assert cuts[0].right > profile.width - profile.margins['right'] - 0.5
    assert pages[0]['glyphs']
    assert writer.pages[0].getextrema()[0] < 128
    assert logs == []
