import json
import zipfile

from tategaki import library
from tategaki.config import Library


def make_lib(tmp_path, ids):
    root = tmp_path / 'home'
    for i in ids:
        build = root / 'books' / i / 'build'
        build.mkdir(parents=True)
        (build.parent / f'{i}.epub').write_bytes(b'')
        with zipfile.ZipFile(build / f'{i}.jbk', 'w') as z:
            z.writestr('manifest.json', json.dumps({'title': i.upper(), 'build_hash': 'h-' + i}))
    return Library.load(root)


def make_volume(tmp_path, ids, state=None):
    vol = tmp_path / 'kindle'
    books = vol / 'tategaki' / 'books'
    books.mkdir(parents=True)
    for i in ids:
        (books / f'{i}.jbk').write_bytes(b'x' * 1000)
        (books / f'{i}.json').write_text('{}')
    (books / '.deployed.json').write_text(json.dumps({i: 'h-' + i for i in ids} | {'_jmdict': 'j'}))
    if state:
        (vol / 'tategaki' / 'state.json').write_text(json.dumps(state))
    return vol


def test_remove_deletes_book_and_settings(tmp_path):
    lib = make_lib(tmp_path, ['a', 'b'])
    vol = make_volume(tmp_path, ['a', 'b'])
    library.remove(lib, ['a', 'missing'], usb=vol, log=lambda s: None)
    books = vol / 'tategaki' / 'books'
    assert sorted(p.name for p in books.iterdir()) == ['.deployed.json', 'b.jbk', 'b.json']
    assert json.loads((books / '.deployed.json').read_text()) == {'b': 'h-b', '_jmdict': 'j', '_removed': ['a']}


def test_remove_rejects_paths(tmp_path):
    lib = make_lib(tmp_path, [])
    vol = make_volume(tmp_path, [])
    for bad in ['../state', '..', 'a/b']:
        try:
            library.remove(lib, [bad], usb=vol)
        except SystemExit:
            continue
        raise AssertionError(bad)


def test_list_shows_status_and_progress(tmp_path):
    lib = make_lib(tmp_path, ['a', 'b', 'c'])
    vol = make_volume(tmp_path, ['a', 'b', 'proof'],
                      state={'last_book': 'b', 'books': {'b': {'page': 9, 'page_count': 100}}})
    deployed = vol / 'tategaki' / 'books' / '.deployed.json'
    deployed.write_text(json.dumps({'a': 'h-a', 'b': 'old', '_removed': ['c']}))
    out = []
    library.list_books(lib, usb=vol, log=out.append)
    assert out[0].split() == ['a', 'A', '0.0', 'MB', 'not', 'started', 'up', 'to', 'date']
    assert out[1].split()[:3] == ['*', 'b', 'B'] and 'p. 10 / 100 (10%)' in out[1]
    assert out[1].endswith('local build is newer')
    assert out[2].endswith('not in the library')
    assert out[-1] == 'in the library, not on the device: c (removed)'


def test_deploy_all_skips_removed(tmp_path, monkeypatch):
    lib = make_lib(tmp_path, ['a', 'b'])
    vol = make_volume(tmp_path, [])
    deployed = vol / 'tategaki' / 'books' / '.deployed.json'
    deployed.write_text(json.dumps({'_removed': ['a']}))
    monkeypatch.setattr(library, 'deploy_anki_words', lambda *a: None)
    monkeypatch.setattr(library, 'deploy_dictionary', lambda *a: None)
    library.deploy(lib, [], usb=vol, log=lambda s: None)
    assert not (vol / 'tategaki' / 'books' / 'a.jbk').exists()
    assert (vol / 'tategaki' / 'books' / 'b.jbk').exists()
    library.deploy(lib, ['a'], usb=vol, log=lambda s: None)  # naming it restores it
    assert (vol / 'tategaki' / 'books' / 'a.jbk').exists()
    assert json.loads(deployed.read_text())['_removed'] == []
