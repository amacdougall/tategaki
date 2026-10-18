"""One EPUB -> one .jbk, given explicit settings."""

import hashlib
from pathlib import Path

from .bundle import BundleWriter
from .epub import cleanup, open_epub
from .reference.furigana import FuriganaRanker
from .reference.jmdict import JMdict
from .render import Profile, Renderer, RenderSettings, cover_image, public_chapters, sleep_image, toc_pages

REPO = Path(__file__).resolve().parents[3]
JMDICT = REPO / 'data' / 'jmdict.sqlite'


def make_ranker():
    return FuriganaRanker(JMdict(str(JMDICT)))


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, 'rb') as f:
        for chunk in iter(lambda: f.read(1 << 20), b''):
            h.update(chunk)
    return h.hexdigest()


def convert(epub_path: Path, out: Path, settings: RenderSettings, profile: Profile, *,
            title: str | None = None, author: str | None = None, build_hash: str | None = None,
            page_limit: int | None = None, log=print) -> dict:
    book = open_epub(epub_path)
    writer = BundleWriter(out)
    try:
        renderer = Renderer(profile, ranker=make_ranker() if settings.furigana == 'rare' else None, log=log)
        chapters = renderer.render(book, writer, settings, page_limit=page_limit)
        cover = cover_image(book, None)
        if cover:
            writer.add_cover(cover)
        sleep = sleep_image(book, profile, settings.gamma)
        if sleep:
            writer.add_sleep(sleep)
        manifest = {
            'title': title or book.title,
            'author': author or book.author,
            'source_sha256': sha256(epub_path),
            'build_hash': build_hash,
            'profile': {'name': profile.name, 'width': profile.width, 'height': profile.height, 'dpi': profile.dpi},
            'render': settings.describe(),
            'chapters': public_chapters(chapters),
            'toc': toc_pages(book, chapters),
        }
        writer.finish(manifest)
        manifest['page_count'] = writer.page_count
        return manifest
    except BaseException:
        writer.abort()
        raise
    finally:
        cleanup(book)
