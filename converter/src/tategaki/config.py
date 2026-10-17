"""Library configuration (plan §5.8).

Precedence: built-in defaults < tategaki.toml < books/<id>/book.toml.
Unknown keys are errors, since they are usually typos.
"""

import os
import tomllib
from dataclasses import dataclass
from pathlib import Path

# section -> key -> (type, default). A None default means "required to be
# resolvable later" (for example the book's EPUB).
SCHEMA = {
    'device': {
        'profile': (str, 'pw11'),
        'ssh_host': (str, 'kindle'),
        'ankiconnect_url': (str, 'http://127.0.0.1:8765'),
        'anki_api_key': (str, ''),
        'default_deck': (str, 'Default'),
        'note_type': (str, 'Kaishi 1.5k'),
        'ranking_policy': (str, 'longest'),
        'scan_len': (int, 16),
        'full_refresh_every': (int, 6),
        'edge_zone_pct': (int, 12),
    },
    'render': {
        'font': (str, 'fonts/NotoSerifJP-Regular.otf'),
        'font_size_pt': (float, 9.0),
        'line_height': (float, 1.75),
        'furigana': (str, 'rare'),
        'rare_threshold': (int, 10000),
        'spreads': (str, 'split'),
        'spread_ratio': (float, 1.2),
        'gamma': (float, 1.0),
        'quantize': (bool, True),
    },
    'anki': {
        'deck': (str, None),   # falls back to device.default_deck on the device
        'tags': (list, ['tategaki']),
        'strip_speaker_tags': (bool, False),
    },
}
CHOICES = {
    ('render', 'furigana'): {'all', 'rare', 'none'},
    ('render', 'spreads'): {'split', 'rotate', 'fit'},
}
BOOK_TOP = {'epub': str, 'title': str, 'author': str}


class ConfigError(Exception):
    pass


def home() -> Path:
    return Path(os.environ.get('TATEGAKI_HOME', Path.home() / 'Tategaki')).expanduser()


def _check(where: str, section: str, key: str, value):
    typ, _ = SCHEMA[section][key]
    if typ is float and isinstance(value, int) and not isinstance(value, bool):
        value = float(value)
    if typ is list:
        if not isinstance(value, list) or not all(isinstance(v, str) for v in value):
            raise ConfigError(f'{where}: [{section}] {key} must be a list of strings')
    elif not isinstance(value, typ) or (typ is int and isinstance(value, bool)):
        raise ConfigError(f'{where}: [{section}] {key} must be {typ.__name__}, got {value!r}')
    allowed = CHOICES.get((section, key))
    if allowed and value not in allowed:
        raise ConfigError(f'{where}: [{section}] {key} must be one of {sorted(allowed)}, got {value!r}')
    return value


def _load(path: Path, sections: set[str], top: dict) -> dict:
    try:
        data = tomllib.loads(path.read_text(encoding='utf-8'))
    except tomllib.TOMLDecodeError as e:
        raise ConfigError(f'{path}: {e}') from None
    out = {}
    for key, value in data.items():
        if isinstance(value, dict):
            if key not in sections:
                raise ConfigError(f'{path}: unknown section [{key}] (allowed: {sorted(sections)})')
            out[key] = {}
            for k, v in value.items():
                if k not in SCHEMA[key]:
                    raise ConfigError(f'{path}: unknown key "{k}" in [{key}] (allowed: {sorted(SCHEMA[key])})')
                out[key][k] = _check(str(path), key, k, v)
        elif key in top:
            if not isinstance(value, top[key]):
                raise ConfigError(f'{path}: {key} must be {top[key].__name__}')
            out[key] = value
        else:
            raise ConfigError(f'{path}: unknown key "{key}"')
    return out


def defaults() -> dict:
    return {s: {k: (list(d) if isinstance(d, list) else d) for k, (_, d) in keys.items()} for s, keys in SCHEMA.items()}


@dataclass
class BookConfig:
    id: str
    dir: Path
    epub: Path
    title: str | None
    author: str | None
    render: dict
    anki: dict

    @property
    def build_dir(self) -> Path:
        return self.dir / 'build'

    @property
    def jbk(self) -> Path:
        return self.build_dir / f'{self.id}.jbk'

    @property
    def sidecar(self) -> Path:
        return self.build_dir / f'{self.id}.json'


@dataclass
class Library:
    root: Path
    device: dict
    render: dict
    anki: dict

    @classmethod
    def load(cls, root: Path | None = None) -> 'Library':
        root = root or home()
        cfg = defaults()
        path = root / 'tategaki.toml'
        if path.exists():
            for section, values in _load(path, {'device', 'render', 'anki'}, {}).items():
                cfg[section].update(values)
        return cls(root=root, device=cfg['device'], render=cfg['render'], anki=cfg['anki'])

    def book_ids(self) -> list[str]:
        books = self.root / 'books'
        return sorted(p.name for p in books.iterdir() if p.is_dir()) if books.exists() else []

    def book(self, book_id: str) -> BookConfig:
        d = self.root / 'books' / book_id
        if not d.is_dir():
            raise ConfigError(f'no book folder {d}')
        if not book_id.isascii() or not all(c.isalnum() or c in '-_.' for c in book_id):
            raise ConfigError(f'book id "{book_id}" should be an ASCII slug (letters, digits, - _ .)')
        render, anki = dict(self.render), dict(self.anki)
        top = {}
        path = d / 'book.toml'
        if path.exists():
            data = _load(path, {'render', 'anki'}, BOOK_TOP)
            render.update(data.pop('render', {}))
            anki.update(data.pop('anki', {}))
            top = data
        if 'epub' in top:
            epub = d / top['epub']
            if not epub.exists():
                raise ConfigError(f'{path}: epub "{top["epub"]}" not found in {d}')
        else:
            epubs = sorted(d.glob('*.epub'))
            if len(epubs) != 1:
                raise ConfigError(f'{d}: expected exactly one .epub (found {len(epubs)}); set epub = "..." in book.toml')
            epub = epubs[0]
        font = Path(render['font']).expanduser()
        render['font'] = str(font if font.is_absolute() else self.root / font)
        return BookConfig(id=book_id, dir=d, epub=epub, title=top.get('title'), author=top.get('author'),
                          render=render, anki=anki)
