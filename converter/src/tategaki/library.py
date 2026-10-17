"""`tategaki build` and `tategaki deploy` over the library folder (plan §5.8)."""

import hashlib
import json
import shlex
import shutil
import subprocess
import tempfile
import zipfile
from importlib import resources
from pathlib import Path

from .config import BookConfig, Library
from .convert import convert, sha256
from .render import Profile, RenderSettings

DEVICE_ROOT = '/mnt/us/tategaki'


def _code_hash() -> str:
    """Hash of the converter's own source, so code changes trigger rebuilds."""
    h = hashlib.sha256()
    pkg = resources.files('tategaki')
    for name in sorted(p.name for p in pkg.iterdir() if p.name.endswith(('.py', '.js'))):
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


def existing_hash(jbk: Path) -> str | None:
    try:
        with zipfile.ZipFile(jbk) as z:
            return json.loads(z.read('manifest.json')).get('build_hash')
    except (OSError, KeyError, zipfile.BadZipFile, json.JSONDecodeError):
        return None


def write_sidecar(book: BookConfig) -> None:
    book.build_dir.mkdir(parents=True, exist_ok=True)
    data = {'deck': book.anki['deck'], 'tags': book.anki['tags'],
            'strip_speaker_tags': book.anki['strip_speaker_tags']}
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

    def hashes(self) -> dict[str, str]:
        p = self.root / 'books' / '.deployed.json'
        return json.loads(p.read_text()) if p.exists() else {}

    def save_hashes(self, hashes: dict) -> None:
        (self.root / 'books' / '.deployed.json').write_text(json.dumps(hashes))


def deploy(lib: Library, ids: list[str], settings_only: bool = False, usb: Path | None = None,
           force: bool = False, log=print) -> None:
    target = UsbTarget(usb, log) if usb else SshTarget(lib.device['ssh_host'], log)
    target.mkdir(f'{DEVICE_ROOT}/books')

    with tempfile.TemporaryDirectory() as tmp:
        cfg = Path(tmp) / 'config.json'
        cfg.write_text(json.dumps(device_config(lib), ensure_ascii=False, indent=1), encoding='utf-8')
        target.put(cfg, f'{DEVICE_ROOT}/config.json')
    log('config.json deployed')

    deployed = target.hashes()
    for book_id in ids or lib.book_ids():
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
