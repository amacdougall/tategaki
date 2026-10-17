"""Proof sheets (plan §9 Phase 1): the first N text pages of a book, rendered
for every combination of typography settings into one stamped .jbk."""

import itertools
import re
from pathlib import Path

from .bundle import BundleWriter
from .convert import make_ranker, sha256
from .epub import cleanup, open_epub
from .render import Profile, Renderer, RenderSettings, Stamp, public_chapters

MIN_TEXT_CHARS = 2000


def _first_text_item(book) -> str:
    """The first spine item with real prose (skips covers, TOC, front matter)."""
    for item in book.spine:
        raw = book.path(item).read_text(encoding='utf-8', errors='replace')
        body = re.sub(r'<rt>.*?</rt>|<[^>]+>|\s', '', raw)
        if len(body) >= MIN_TEXT_CHARS:
            return item
    return book.spine[0]


def proof(epub_path: Path, out: Path, profile: Profile, pages: int, fonts: list[Path],
          sizes: list[float], line_heights: list[float], furigana: str = 'rare', log=print) -> dict:
    book = open_epub(epub_path)
    writer = BundleWriter(out)
    try:
        item = _first_text_item(book)
        renderer = Renderer(profile, ranker=make_ranker(), log=log)
        chapters: list[dict] = []
        for font, size, lh in itertools.product(fonts, sizes, line_heights):
            label = f'{size:g}pt  {font.stem}  lh {lh:g}'
            log(f'{label}')
            settings = RenderSettings(font=font, font_size_pt=size, line_height=lh, furigana=furigana)
            before = len(chapters)
            renderer.render(book, writer, settings, stamp=Stamp(label), page_limit=writer.page_count + pages,
                            spine=[item], skip_illustrations=True, chapters_out=chapters)
            if len(chapters) > before:
                chapters[before]['_label'] = label
        manifest = {
            'title': f'Proof: {book.title}', 'author': book.author, 'source_sha256': sha256(epub_path),
            'build_hash': None,
            'profile': {'name': profile.name, 'width': profile.width, 'height': profile.height, 'dpi': profile.dpi},
            'render': {'proof': True, 'fonts': [f.name for f in fonts], 'font_size_pt': sizes,
                       'line_height': line_heights, 'furigana': furigana},
            'chapters': public_chapters(chapters),
            'toc': [{'title': c['_label'], 'page': c['first_page'], 'level': 1} for c in chapters if '_label' in c],
        }
        writer.finish(manifest)
        manifest['page_count'] = writer.page_count
        return manifest
    except BaseException:
        writer.abort()
        raise
    finally:
        cleanup(book)
