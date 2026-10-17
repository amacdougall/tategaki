"""Writer for the .jbk bundle (plan §6): an uncompressed zip."""

import io
import json
import zipfile
from pathlib import Path

from PIL import Image

FORMAT = 1


class BundleWriter:
    def __init__(self, path: Path):
        self.path = path
        self.tmp = path.with_suffix(path.suffix + '.tmp')
        self.zip = zipfile.ZipFile(self.tmp, 'w', compression=zipfile.ZIP_STORED)
        self.page_count = 0
        self.images: set[str] = set()

    def _png(self, name: str, img: Image.Image) -> None:
        buf = io.BytesIO()
        img.save(buf, 'PNG', optimize=False)
        self.zip.writestr(name, buf.getvalue())

    def add_page(self, img: Image.Image) -> int:
        index = self.page_count
        self._png(f'pages/{index:05d}.png', img)
        self.page_count += 1
        return index

    def add_image(self, key: str, img: Image.Image) -> str:
        name = f'images/{key}.png'
        if name not in self.images:
            self._png(name, img)
            self.images.add(name)
        return name

    def add_cover(self, img: Image.Image) -> None:
        self._png('cover.png', img)

    def add_text(self, chapter: int, text: str) -> str:
        name = f'text/{chapter:04d}.txt'
        self.zip.writestr(name, text.encode('utf-8'))
        return name

    def add_json(self, name: str, data) -> str:
        self.zip.writestr(name, json.dumps(data, ensure_ascii=False, separators=(',', ':')).encode('utf-8'))
        return name

    def finish(self, manifest: dict) -> None:
        manifest = {'format': FORMAT, **manifest, 'page_count': self.page_count}
        self.zip.writestr('manifest.json', json.dumps(manifest, ensure_ascii=False, indent=1).encode('utf-8'))
        self.zip.close()
        self.tmp.replace(self.path)

    def abort(self) -> None:
        self.zip.close()
        self.tmp.unlink(missing_ok=True)
