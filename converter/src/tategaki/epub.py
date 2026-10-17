"""Minimal EPUB 2/3 reader: metadata, spine, cover and table of contents."""

import posixpath
import shutil
import tempfile
import zipfile
from dataclasses import dataclass, field
from pathlib import Path
from urllib.parse import unquote, urldefrag
from xml.etree import ElementTree as ET

NS = {
    'c': 'urn:oasis:names:tc:opendocument:xmlns:container',
    'opf': 'http://www.idpf.org/2007/opf',
    'dc': 'http://purl.org/dc/elements/1.1/',
    'x': 'http://www.w3.org/1999/xhtml',
    'epub': 'http://www.idpf.org/2007/ops',
    'ncx': 'http://www.daisy.org/z3986/2005/ncx/',
}


@dataclass
class TocEntry:
    title: str
    href: str  # zip path, possibly with #fragment
    level: int


@dataclass
class Epub:
    root: Path               # extracted directory
    title: str = ''
    author: str = ''
    spine: list[str] = field(default_factory=list)  # zip paths in reading order
    cover: str | None = None  # zip path of the cover image
    toc: list[TocEntry] = field(default_factory=list)
    rtl: bool = True

    def path(self, zip_path: str) -> Path:
        return self.root / zip_path

    def zip_path(self, file: Path) -> str:
        return file.resolve().relative_to(self.root.resolve()).as_posix()


def _join(base_dir: str, href: str) -> str:
    path, frag = urldefrag(unquote(href))
    joined = posixpath.normpath(posixpath.join(base_dir, path)) if path else ''
    return joined + (f'#{frag}' if frag else '')


def open_epub(epub_file: Path, workdir: Path | None = None) -> Epub:
    """Extract the EPUB to a temporary directory and parse its package."""
    root = Path(workdir or tempfile.mkdtemp(prefix='tategaki-epub-'))
    with zipfile.ZipFile(epub_file) as z:
        for info in z.infolist():
            target = (root / info.filename).resolve()
            if not str(target).startswith(str(root.resolve())):
                raise ValueError(f'unsafe path in EPUB: {info.filename}')
        z.extractall(root)

    container = ET.parse(root / 'META-INF' / 'container.xml')
    opf_path = container.find('.//c:rootfile', NS).get('full-path')
    opf_dir = posixpath.dirname(opf_path)
    opf = ET.parse(root / opf_path).getroot()

    book = Epub(root=root)
    meta = opf.find('opf:metadata', NS)
    book.title = (meta.findtext('dc:title', '', NS) or '').strip()
    book.author = (meta.findtext('dc:creator', '', NS) or '').strip()

    manifest = {}
    cover_id = None
    for m in meta.findall('opf:meta', NS):
        if m.get('name') == 'cover':
            cover_id = m.get('content')
    nav_href = None
    for item in opf.find('opf:manifest', NS):
        href = _join(opf_dir, item.get('href'))
        manifest[item.get('id')] = href
        props = (item.get('properties') or '').split()
        if 'cover-image' in props:
            book.cover = href
        if 'nav' in props:
            nav_href = href
    if not book.cover and cover_id in manifest:
        book.cover = manifest[cover_id]

    spine = opf.find('opf:spine', NS)
    book.rtl = spine.get('page-progression-direction', 'rtl') != 'ltr'
    for ref in spine.findall('opf:itemref', NS):
        if ref.get('linear', 'yes') != 'no' and ref.get('idref') in manifest:
            book.spine.append(manifest[ref.get('idref')])

    if nav_href:
        book.toc = _nav_toc(root, nav_href)
    elif spine.get('toc') in manifest:
        book.toc = _ncx_toc(root, manifest[spine.get('toc')])
    return book


def _text(el) -> str:
    return ''.join(el.itertext()).strip()


def _nav_toc(root: Path, nav_href: str) -> list[TocEntry]:
    doc = ET.parse(root / nav_href).getroot()
    base = posixpath.dirname(nav_href)
    nav = None
    for n in doc.iter(f'{{{NS["x"]}}}nav'):
        if n.get(f'{{{NS["epub"]}}}type') == 'toc':
            nav = n
            break
    if nav is None:
        return []
    entries = []

    def walk(ol, level):
        for li in ol.findall('x:li', NS):
            a = li.find('x:a', NS)
            if a is not None and a.get('href'):
                entries.append(TocEntry(_text(a), _join(base, a.get('href')), level))
            sub = li.find('x:ol', NS)
            if sub is not None:
                walk(sub, level + 1)

    top = nav.find('x:ol', NS)
    if top is not None:
        walk(top, 1)
    return entries


def _ncx_toc(root: Path, ncx_href: str) -> list[TocEntry]:
    doc = ET.parse(root / ncx_href).getroot()
    base = posixpath.dirname(ncx_href)
    entries = []

    def walk(parent, level):
        for np in parent.findall('ncx:navPoint', NS):
            label = np.findtext('ncx:navLabel/ncx:text', '', NS).strip()
            content = np.find('ncx:content', NS)
            if content is not None:
                entries.append(TocEntry(label, _join(base, content.get('src')), level))
            walk(np, level + 1)

    nav_map = doc.find('ncx:navMap', NS)
    if nav_map is not None:
        walk(nav_map, 1)
    return entries


def cleanup(book: Epub) -> None:
    shutil.rmtree(book.root, ignore_errors=True)
