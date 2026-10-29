"""`tategaki build` and `tategaki deploy` over the library folder (plan §5.8)."""

import hashlib
import html
import json
import re
import shlex
import shutil
import subprocess
import tempfile
import unicodedata
import urllib.request
import zipfile
from importlib import resources
from pathlib import Path

from .config import BookConfig, Library
from .convert import JMDICT, convert, sha256
from .reference.anki_connect import AnkiConnectError, AnkiConnectUnavailable
from .render import Profile, RenderSettings

DEVICE_ROOT = '/mnt/us/tategaki'
ANKI_WORDS = 'anki-words.txt'
# Key in books/.deployed.json listing books taken off the device with
# `tategaki remove`, which a plain `deploy` (all books) then skips.
REMOVED = '_removed'


# Source files whose changes alter a rendered bundle. CLI, config and deploy
# code are left out so editing them doesn't re-render every book.
RENDER_SOURCES = ['bundle.py', 'convert.py', 'epub.py', 'images.py', 'layout.js', 'paginate.py',
                  'render.py', 'sentences.py', 'reference/furigana.py', 'reference/deinflect.py',
                  'reference/deinflect_rules.json', 'reference/jmdict.py', 'profiles/pw11.json']


def _code_hash() -> str:
    """Hash of the rendering code, so converter changes trigger rebuilds."""
    h = hashlib.sha256()
    pkg = resources.files('tategaki')
    for name in RENDER_SOURCES:
        h.update(name.encode())
        h.update(pkg.joinpath(name).read_bytes())
    return h.hexdigest()[:16]


def settings_for(book: BookConfig) -> RenderSettings:
    r = dict(book.render)
    return RenderSettings(font=Path(r.pop('font')), **r)


def build_hash(book: BookConfig, profile_name: str) -> str:
    r = dict(book.render)
    font = Path(r['font'])
    r['font'] = sha256(font) if font.exists() else str(font)
    key = {'epub': sha256(book.epub), 'render': r, 'title': book.title, 'author': book.author,
           'profile': profile_name, 'code': _code_hash()}
    return hashlib.sha256(json.dumps(key, sort_keys=True).encode()).hexdigest()


def manifest(jbk: Path) -> dict | None:
    try:
        with zipfile.ZipFile(jbk) as z:
            return json.loads(z.read('manifest.json'))
    except (OSError, KeyError, zipfile.BadZipFile, json.JSONDecodeError):
        return None


def existing_hash(jbk: Path) -> str | None:
    m = manifest(jbk)
    return m.get('build_hash') if m else None


def write_sidecar(book: BookConfig) -> None:
    book.build_dir.mkdir(parents=True, exist_ok=True)
    data = {'deck': book.anki['deck'], 'tags': book.anki['tags'],
            'strip_speaker_tags': book.anki['strip_speaker_tags'],
            'sleep_screen': book.reader['sleep_screen']}
    tmp = book.sidecar.with_suffix('.json.tmp')
    tmp.write_text(json.dumps(data, ensure_ascii=False, indent=1), encoding='utf-8')
    tmp.replace(book.sidecar)


def build(lib: Library, ids: list[str], force: bool = False, log=print) -> list[str]:
    """Build the given books (all if empty). Returns ids that were rebuilt."""
    profile = Profile.load(lib.device['profile'])
    built = []
    for book_id in ids or lib.book_ids():
        book = lib.book(book_id)
        write_sidecar(book)
        h = build_hash(book, profile.name)
        if not force and existing_hash(book.jbk) == h:
            log(f'{book_id}: up to date')
            continue
        font = Path(book.render['font'])
        if not font.exists():
            raise SystemExit(f'{book_id}: font not found: {font}')
        log(f'{book_id}: building from {book.epub.name}')
        m = convert(book.epub, book.jbk, settings_for(book), profile,
                    title=book.title, author=book.author, build_hash=h, log=log)
        log(f'{book_id}: {m["page_count"]} pages -> {book.jbk}')
        built.append(book_id)
    return built


def device_config(lib: Library) -> dict:
    return dict(lib.device)


class SshTarget:
    def __init__(self, host: str, log=print):
        self.host, self.log = host, log

    def run(self, cmd: str) -> str:
        r = subprocess.run(['ssh', self.host, cmd], capture_output=True, text=True)
        if r.returncode != 0:
            raise SystemExit(f'ssh {self.host} failed ({r.returncode}): {r.stderr.strip()}\n'
                             'Is the Kindle on Wi-Fi with Dropbear SSH started? See docs/DEVICE.md.')
        return r.stdout

    def put(self, local: Path, remote: str) -> None:
        # copy to a temporary name, then rename, so the reader never sees a partial file
        tmp = remote + '.part'
        r = subprocess.run(['scp', '-q', str(local), f'{self.host}:{tmp}'], capture_output=True, text=True)
        if r.returncode != 0:
            raise SystemExit(f'scp to {self.host} failed: {r.stderr.strip()}')
        self.run(f'mv -f {shlex.quote(tmp)} {shlex.quote(remote)}')

    def mkdir(self, path: str) -> None:
        self.run(f'mkdir -p {shlex.quote(path)}')

    def read(self, remote: str) -> str:
        """A text file's contents, or '' when it's missing."""
        return self.run(f'cat {shlex.quote(remote)} 2>/dev/null || true')

    def books(self) -> dict[str, int]:
        """book id -> .jbk size in bytes, for every book on the device."""
        out = self.run(f'cd {DEVICE_ROOT}/books 2>/dev/null || exit 0; '
                       'for f in *.jbk; do [ -f "$f" ] && echo "$(wc -c < "$f") $f"; done; true')
        sizes = {}
        for line in out.splitlines():
            size, name = line.strip().split(' ', 1)
            sizes[name.removesuffix('.jbk')] = int(size)
        return sizes

    def remove(self, remote: str) -> None:
        self.run(f'rm -f {shlex.quote(remote)}')

    def hashes(self) -> dict[str, str]:
        """book id -> build_hash recorded on the device at last deploy."""
        out = self.run(f'cat {DEVICE_ROOT}/books/.deployed.json 2>/dev/null || true').strip()
        try:
            return json.loads(out) if out else {}
        except json.JSONDecodeError:
            return {}

    def save_hashes(self, hashes: dict) -> None:
        with tempfile.NamedTemporaryFile('w', suffix='.json', delete=False) as f:
            json.dump(hashes, f)
        self.put(Path(f.name), f'{DEVICE_ROOT}/books/.deployed.json')
        Path(f.name).unlink()


class UsbTarget:
    def __init__(self, volume: Path, log=print):
        self.root = volume / 'tategaki'
        if not volume.exists():
            raise SystemExit(f'{volume} is not mounted')
        self.log = log

    def put(self, local: Path, remote: str) -> None:
        dest = self.root / Path(remote).relative_to(DEVICE_ROOT)
        dest.parent.mkdir(parents=True, exist_ok=True)
        tmp = dest.with_name(dest.name + '.part')
        shutil.copyfile(local, tmp)
        tmp.replace(dest)

    def mkdir(self, path: str) -> None:
        (self.root / Path(path).relative_to(DEVICE_ROOT)).mkdir(parents=True, exist_ok=True)

    def read(self, remote: str) -> str:
        p = self.root / Path(remote).relative_to(DEVICE_ROOT)
        return p.read_text(encoding='utf-8') if p.exists() else ''

    def books(self) -> dict[str, int]:
        return {p.stem: p.stat().st_size for p in sorted((self.root / 'books').glob('*.jbk'))}

    def remove(self, remote: str) -> None:
        (self.root / Path(remote).relative_to(DEVICE_ROOT)).unlink(missing_ok=True)

    def hashes(self) -> dict[str, str]:
        p = self.root / 'books' / '.deployed.json'
        return json.loads(p.read_text()) if p.exists() else {}

    def save_hashes(self, hashes: dict) -> None:
        (self.root / 'books' / '.deployed.json').write_text(json.dumps(hashes))


def deploy_dictionary(target, deployed: dict, force: bool, log=print) -> None:
    """Send data/jmdict.sqlite when it changed. It lives in the reader's data
    folder rather than the KPM package, so app reinstalls don't resend 54 MB."""
    if not JMDICT.exists():
        log(f'dictionary: {JMDICT} missing (run tools/prepare_jmdict.py); skipping')
        return
    h = sha256(JMDICT)
    if not force and deployed.get('_jmdict') == h:
        log('dictionary: already on device')
        return
    log(f'dictionary: sending {JMDICT.stat().st_size / 1e6:.0f} MB')
    target.put(JMDICT, f'{DEVICE_ROOT}/jmdict.sqlite')
    deployed['_jmdict'] = h
    target.save_hashes(deployed)


def anki_invoke(device: dict, action: str, **params):
    """One AnkiConnect call with the configured URL and API key (the vendored
    client has no key support)."""
    body = {'action': action, 'version': 6, 'params': params}
    if device['anki_api_key']:
        body['key'] = device['anki_api_key']
    req = urllib.request.Request(device['ankiconnect_url'], data=json.dumps(body).encode('utf-8'),
                                 headers={'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            r = json.loads(resp.read().decode('utf-8'))
    except OSError as e:
        raise AnkiConnectUnavailable(f'Anki not reachable at {device["ankiconnect_url"]}: {e}') from e
    if r.get('error') is not None:
        raise AnkiConnectError(r['error'])
    return r['result']


def first_field(note: dict) -> str:
    """A note's first field (the one Anki's duplicate check compares) as plain
    text. Furigana is dropped, not just its tags: some notes hold
    <ruby>島<rt>しま</rt></ruby>, which should match the reader's 島."""
    fields = note.get('fields') or {}
    if not fields:
        return ''
    value = min(fields.values(), key=lambda f: f['order'])['value']
    value = re.sub(r'<(rt|rp)\b[^>]*>.*?</\1>', '', value, flags=re.S | re.I)
    return html.unescape(re.sub(r'<[^>]*>', '', value)).strip()


def anki_words(device: dict) -> list[str]:
    """Every word already in Anki for the reader's note type, collection-wide,
    as the reader's duplicate check sees them."""
    ids = anki_invoke(device, 'findNotes', query=f'"note:{device["note_type"]}"')
    words = set()
    for i in range(0, len(ids), 500):
        for note in anki_invoke(device, 'notesInfo', notes=ids[i:i + 500]):
            if w := first_field(note):
                words.add(w)
    return sorted(words)


def deploy_anki_words(lib: Library, target, log=print) -> None:
    """Send the words already in Anki, so the lookup popup can show Exists
    offline. If Anki can't be reached, the Kindle keeps its previous list."""
    try:
        words = anki_words(lib.device)
    except (AnkiConnectUnavailable, AnkiConnectError) as e:
        log(f'anki words: {e}; the Kindle keeps its previous list')
        return
    path = lib.root / ANKI_WORDS
    path.write_text(''.join(w + '\n' for w in words), encoding='utf-8')
    target.put(path, f'{DEVICE_ROOT}/{ANKI_WORDS}')
    log(f'anki words: {len(words)} deployed')


def target_for(lib: Library, usb: Path | None, log=print):
    return UsbTarget(usb, log) if usb else SshTarget(lib.device['ssh_host'], log)


def deploy(lib: Library, ids: list[str], settings_only: bool = False, usb: Path | None = None,
           force: bool = False, log=print) -> None:
    target = target_for(lib, usb, log)
    target.mkdir(f'{DEVICE_ROOT}/books')

    with tempfile.TemporaryDirectory() as tmp:
        cfg = Path(tmp) / 'config.json'
        cfg.write_text(json.dumps(device_config(lib), ensure_ascii=False, indent=1), encoding='utf-8')
        target.put(cfg, f'{DEVICE_ROOT}/config.json')
    log('config.json deployed')
    deploy_anki_words(lib, target, log)

    deployed = target.hashes()
    if not settings_only:
        deploy_dictionary(target, deployed, force, log)
    removed = set(deployed.get(REMOVED, []))
    if ids and removed & set(ids):
        # naming a removed book brings it back
        deployed[REMOVED] = sorted(removed - set(ids))
        target.save_hashes(deployed)
    for book_id in ids or lib.book_ids():
        if not ids and book_id in removed:
            log(f'{book_id}: removed from the device, skipping (`tategaki deploy {book_id}` restores it)')
            continue
        book = lib.book(book_id)
        write_sidecar(book)
        target.put(book.sidecar, f'{DEVICE_ROOT}/books/{book_id}.json')
        if settings_only:
            log(f'{book_id}: settings deployed')
            continue
        h = existing_hash(book.jbk)
        if h is None:
            log(f'{book_id}: not built yet, skipping (run `tategaki build {book_id}`)')
            continue
        if not force and deployed.get(book_id) == h:
            log(f'{book_id}: already on device')
            continue
        log(f'{book_id}: sending {book.jbk.stat().st_size / 1e6:.1f} MB')
        target.put(book.jbk, f'{DEVICE_ROOT}/books/{book_id}.jbk')
        deployed[book_id] = h
        target.save_hashes(deployed)
    log('deploy done')


def _cells(s: str) -> int:
    """Terminal width: full-width (Japanese) characters take two cells."""
    return sum(2 if unicodedata.east_asian_width(c) in 'WF' else 1 for c in s)


def _progress(pos: dict | None) -> str:
    if not pos or not pos.get('page_count'):
        return 'not started'
    return f'p. {pos["page"] + 1} / {pos["page_count"]} ({100 * (pos["page"] + 1) // pos["page_count"]}%)'


def list_books(lib: Library, usb: Path | None = None, log=print) -> None:
    """Print the books on the device, then local books that aren't on it."""
    target = target_for(lib, usb, log)
    on_device = target.books()
    deployed = target.hashes()
    try:
        state = json.loads(target.read(f'{DEVICE_ROOT}/state.json') or '{}')
    except json.JSONDecodeError:
        state = {}
    positions, last = state.get('books') or {}, state.get('last_book')
    local = set(lib.book_ids())

    rows = []
    for book_id, size in sorted(on_device.items()):
        title, status = '', 'not in the library'
        if book_id in local:
            m = manifest(lib.root / 'books' / book_id / 'build' / f'{book_id}.jbk')
            if m is None:
                status = 'not built locally'
            else:
                title = m.get('title') or ''
                status = 'up to date' if deployed.get(book_id) == m.get('build_hash') else 'local build is newer'
        mark = '*' if book_id == last else ' '
        rows.append((f'{mark} {book_id}', title, f'{size / 1e6:.1f} MB', _progress(positions.get(book_id)), status))
    if rows:
        widths = [max(_cells(r[i]) for r in rows) for i in range(4)]
        for r in rows:
            log(''.join(c + ' ' * (w - _cells(c) + 2) for c, w in zip(r, widths)) + r[4])
        log(f'{len(rows)} on the device' + ('; * = last read' if last in on_device else ''))
    else:
        log('no books on the device')

    removed = set(deployed.get(REMOVED, []))
    missing = sorted(local - set(on_device))
    if missing:
        log('in the library, not on the device: ' + ', '.join(
            b + (' (removed)' if b in removed else '') for b in missing))


def remove(lib: Library, ids: list[str], usb: Path | None = None, log=print) -> None:
    """Delete books (and their settings) from the device. Reading positions
    stay in state.json, so deploying a book again resumes where it was."""
    for book_id in ids:
        if not book_id.isascii() or not all(c.isalnum() or c in '-_.' for c in book_id) \
                or book_id.strip('.') == '':
            raise SystemExit(f'"{book_id}" is not a book id')
    target = target_for(lib, usb, log)
    on_device = target.books()
    deployed = target.hashes()
    removed = set(deployed.get(REMOVED, []))
    for book_id in ids:
        if book_id not in on_device:
            log(f'{book_id}: not on the device')
            continue
        for name in (f'{book_id}.jbk', f'{book_id}.json', f'{book_id}.jbk.part', f'{book_id}.json.part'):
            target.remove(f'{DEVICE_ROOT}/books/{name}')
        deployed.pop(book_id, None)
        if book_id in lib.book_ids():
            removed.add(book_id)
        log(f'{book_id}: removed')
    deployed[REMOVED] = sorted(removed)
    target.save_hashes(deployed)
