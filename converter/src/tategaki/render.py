"""EPUB -> .jbk rendering with headless Chromium (plan §5)."""

import hashlib
import io
import json
import time
from dataclasses import asdict, dataclass, field
from importlib import resources
from pathlib import Path
from urllib.parse import unquote, urlparse

from PIL import Image, ImageDraw, ImageFont
from playwright.sync_api import Page, sync_playwright

from . import images
from .bundle import BundleWriter
from .epub import Epub
from .paginate import Cut, Illustration, paginate
from .sentences import sentence_spans

LAYOUT_JS = resources.files('tategaki').joinpath('layout.js').read_text(encoding='utf-8')


@dataclass
class Profile:
    name: str
    width: int
    height: int
    dpi: int
    margins: dict

    @classmethod
    def load(cls, name: str) -> 'Profile':
        data = json.loads(resources.files('tategaki').joinpath('profiles', f'{name}.json').read_text())
        data.pop('description', None)
        return cls(**data)

    @property
    def content_width(self) -> int:
        return self.width - self.margins['left'] - self.margins['right']

    @property
    def content_height(self) -> int:
        return self.height - self.margins['top'] - self.margins['bottom']


@dataclass
class RenderSettings:
    font: Path
    font_size_pt: float = 9
    line_height: float = 1.75
    furigana: str = 'rare'        # all | rare | none
    rare_threshold: int = 10000
    spreads: str = 'split'        # split | rotate | fit
    spread_ratio: float = 1.2
    gamma: float = 1.0
    quantize: bool = True

    def describe(self) -> dict:
        d = asdict(self)
        d['font'] = Path(self.font).name
        return d


@dataclass
class Stamp:
    """Proof-sheet label drawn in a page corner."""
    text: str


@dataclass
class _Chapter:
    text: str = ''
    pages: list = field(default_factory=list)  # layout page dicts
    ids: dict = field(default_factory=dict)


def _css(s: RenderSettings, p: Profile) -> str:
    font_px = s.font_size_pt * p.dpi / 72
    m = p.margins
    return f'''
@font-face {{ font-family: "TGReading"; src: url("{Path(s.font).resolve().as_uri()}"); }}
html {{
  writing-mode: vertical-rl !important; -epub-writing-mode: vertical-rl !important;
  line-break: strict !important; font-size: {font_px:.3f}px !important;
  line-height: {s.line_height} !important; margin: 0 !important;
  padding: {m["top"]}px {m["right"]}px {m["bottom"]}px {p.width}px !important;
  background: #fff !important; color: #000 !important; scrollbar-width: none;
}}
html, body, body * {{ font-family: "TGReading", serif !important; }}
body, body * {{ line-height: inherit !important; }}
body {{ margin: 0 !important; padding: 0 !important; height: {p.content_height}px !important;
  max-height: none !important; }}
img, svg {{ max-height: {p.content_height}px !important; max-width: {p.content_width}px !important; }}
::-webkit-scrollbar {{ display: none; }}
'''


def _load(page: Page, url: str, css: str) -> None:
    page.goto(url, wait_until='domcontentloaded')
    page.evaluate('''(css) => {
        const s = document.createElementNS('http://www.w3.org/1999/xhtml', 'style');
        s.textContent = css;
        (document.head || document.documentElement).appendChild(s);
    }''', css)
    page.evaluate(LAYOUT_JS)
    page.evaluate('''async () => {
        await Promise.all(Array.from(document.images).map(i => i.decode().catch(() => null)));
        await document.fonts.ready;
    }''')


def _url_to_path(url: str) -> Path:
    return Path(unquote(urlparse(url).path))


def _stamp(img: Image.Image, stamp: Stamp | None) -> Image.Image:
    if stamp:
        draw = ImageDraw.Draw(img)
        font = ImageFont.load_default(size=28)
        box = draw.textbbox((0, 0), stamp.text, font=font)
        w, h = box[2] - box[0], box[3] - box[1]
        draw.rectangle((8, 8, w + 24, h + 24), fill=255, outline=0)
        draw.text((16, 16 - box[1]), stamp.text, font=font, fill=0)
    return img


class Renderer:
    def __init__(self, profile: Profile, ranker=None, log=print):
        self.profile = profile
        self.ranker = ranker
        self.log = log
        self.skip_illustrations = False

    def _furigana(self, page: Page, s: RenderSettings) -> None:
        groups = page.evaluate('() => TG.collectRubyGroups()')
        if s.furigana == 'all' or not groups:
            return
        hide = []
        for g in groups:
            if g['exempt']:
                hide.append(False)
            elif s.furigana == 'none':
                hide.append(True)
            else:
                hide.append(self.ranker.rank(g['base'], g['following']) <= s.rare_threshold)
        page.evaluate('(hide) => TG.applyFurigana(hide)', hide)

    def _illustration(self, writer: BundleWriter, src: Path, s: RenderSettings,
                      stamp: Stamp | None) -> list[dict]:
        p = self.profile
        with Image.open(src) as img:
            out = images.illustration_pages(img, p.width, p.height, s.spreads, s.spread_ratio, s.gamma)
        entries = []
        for im in out:
            idx = writer.add_page(_stamp(im, stamp))
            entries.append({'page': idx, 'glyphs': [], 'images': [{'box': [0, 0, p.width, p.height]}]})
        return entries

    def _inline_image(self, writer: BundleWriter, src: Path, s: RenderSettings) -> str:
        key = hashlib.sha1(f'{src}|{s.gamma}'.encode()).hexdigest()[:12]
        p = self.profile
        with Image.open(src) as img:
            full = images.process_illustration(images.fit(img.convert('L'), p.width, p.height), s.gamma)
        return writer.add_image(key, full)

    def _text_page(self, page: Page, writer: BundleWriter, cut: Cut, data: dict,
                   s: RenderSettings, stamp: Stamp | None) -> dict:
        p = self.profile
        target = cut.right - (p.width - p.margins['right'])
        page.evaluate('(x) => window.scrollTo(x, 0)', target)
        actual = page.evaluate('() => window.scrollX')
        dx = -actual
        shot = Image.open(io.BytesIO(page.screenshot(type='png'))).convert('L')
        left, right = round(cut.clip_left + dx), round(cut.clip_right + dx) + 1
        draw = ImageDraw.Draw(shot)
        if left > 0:
            draw.rectangle((0, 0, left - 1, p.height), fill=255)
        if right < p.width:
            draw.rectangle((right, 0, p.width, p.height), fill=255)
        shot = images.gamma(shot, s.gamma)
        if s.quantize:
            shot = images.quantize(shot, dither=False)
        idx = writer.add_page(_stamp(shot, stamp))

        glyphs = []
        for off, x, y, w, h in data['glyphs']:
            if cut.clip_left <= x + w / 2 <= cut.clip_right:
                glyphs.append([off, round(x + dx), round(y), round(w), round(h)])
        imgs = []
        for im in data['inlineImages']:
            x, y, w, h = im['box']
            if cut.clip_left <= x + w / 2 <= cut.clip_right and im['src']:
                full = self._inline_image(writer, _url_to_path(im['src']), s)
                imgs.append({'box': [round(x + dx), round(y), round(w), round(h)], 'full': full})
        return {'page': idx, 'glyphs': glyphs, 'images': imgs}

    def _chapter(self, page: Page, book: Epub, item: str, writer: BundleWriter,
                 s: RenderSettings, stamp: Stamp | None, page_limit: int | None) -> _Chapter:
        _load(page, book.path(item).resolve().as_uri(), _css(s, self.profile))
        ch = _Chapter()
        illus = page.evaluate('() => TG.extractIllustrations()')
        sources = [_url_to_path(u) for u in illus['sources']]
        if illus['onlyIllustrations']:
            if self.skip_illustrations:
                return ch
            for src in sources:
                ch.pages += self._illustration(writer, src, s, stamp)
            return ch

        self._furigana(page, s)
        data = page.evaluate('() => TG.collect()')
        ch.text, ch.ids = data['text'], data['ids']
        markers = [(m[0], m[1]) for m in data['markers']]
        for entry in paginate([tuple(b) for b in data['bands']], markers, self.profile.content_width):
            if page_limit is not None and writer.page_count >= page_limit:
                break
            if isinstance(entry, Illustration):
                if not self.skip_illustrations:
                    ch.pages += self._illustration(writer, sources[entry.index], s, stamp)
            else:
                ch.pages.append(self._text_page(page, writer, entry, data, s, stamp))
        return ch

    def render(self, book: Epub, writer: BundleWriter, s: RenderSettings, *,
               stamp: Stamp | None = None, page_limit: int | None = None,
               spine: list[str] | None = None, skip_illustrations: bool = False,
               chapters_out: list | None = None) -> list[dict]:
        """Render the book's spine (or a subset) into writer, appending to
        chapters_out if given. Returns the manifest chapter entries."""
        chapters = [] if chapters_out is None else chapters_out
        self.skip_illustrations = skip_illustrations
        p = self.profile
        with sync_playwright() as pw:
            browser = pw.chromium.launch()
            ctx = browser.new_context(java_script_enabled=False, device_scale_factor=1,
                                      viewport={'width': p.width, 'height': p.height})
            page = ctx.new_page()
            try:
                for item in (book.spine if spine is None else spine):
                    if page_limit is not None and writer.page_count >= page_limit:
                        break
                    t0 = time.time()
                    first = writer.page_count
                    ch = self._chapter(page, book, item, writer, s, stamp, page_limit)
                    n = len(chapters)
                    text_name = writer.add_text(n, ch.text)
                    paragraphs = _paragraphs(ch.text)
                    sentences = [list(sp) for a, b in paragraphs for sp in sentence_spans(ch.text, a, b)]
                    layout_name = writer.add_json(f'layout/{n:04d}.json', {
                        'pages': ch.pages,
                        'paragraphs': [list(pg) for pg in paragraphs],
                        'sentences': sentences,
                    })
                    chapters.append({'id': n, 'source': item, 'text': text_name, 'layout': layout_name,
                                     'first_page': first, 'page_count': writer.page_count - first,
                                     '_ids': ch.ids, '_pages': ch.pages})
                    self.log(f'  {item}: {writer.page_count - first} pages, {len(ch.text)} chars '
                             f'({time.time() - t0:.1f}s)')
            finally:
                browser.close()
        return chapters


def _paragraphs(text: str) -> list[tuple[int, int]]:
    out, start = [], 0
    for i, ch in enumerate(text):
        if ch == '\n':
            if i > start:
                out.append((start, i))
            start = i + 1
    if len(text) > start:
        out.append((start, len(text)))
    return out


def toc_pages(book: Epub, chapters: list[dict]) -> list[dict]:
    """Map TOC hrefs to global page indices via element-id text offsets."""
    by_source = {c['source']: c for c in chapters}
    toc = []
    for entry in book.toc:
        path, _, frag = entry.href.partition('#')
        ch = by_source.get(path)
        if not ch or not ch['page_count']:
            continue
        page = ch['first_page']
        offset = ch['_ids'].get(frag) if frag else None
        if offset is not None:
            for pg in ch['_pages']:
                if pg['glyphs'] and pg['glyphs'][-1][0] >= offset:
                    page = pg['page']
                    break
        toc.append({'title': entry.title, 'page': page, 'level': entry.level})
    return toc


def cover_image(book: Epub, writer_pages_first: Image.Image | None) -> Image.Image | None:
    src = book.path(book.cover) if book.cover else None
    if src and src.exists():
        with Image.open(src) as img:
            return images.fit(img.convert('L'), 300, 400)
    if writer_pages_first is not None:
        return images.fit(writer_pages_first, 300, 400)
    return None


def public_chapters(chapters: list[dict]) -> list[dict]:
    return [{k: v for k, v in c.items() if not k.startswith('_')} for c in chapters]
