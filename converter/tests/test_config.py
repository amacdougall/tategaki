import json

import pytest

from tategaki.config import ConfigError, Library
from tategaki.library import write_sidecar


def make_lib(tmp_path, global_toml='', book_toml=None, epubs=('book.epub',)):
    tmp_path.mkdir(parents=True, exist_ok=True)
    (tmp_path / 'tategaki.toml').write_text(global_toml, encoding='utf-8')
    d = tmp_path / 'books' / 'hen-na-ie'
    d.mkdir(parents=True)
    for e in epubs:
        (d / e).write_bytes(b'')
    if book_toml is not None:
        (d / 'book.toml').write_text(book_toml, encoding='utf-8')
    return Library.load(tmp_path)


def test_precedence_defaults_global_book(tmp_path):
    lib = make_lib(tmp_path, '[render]\nfont_size_pt = 10\nline_height = 1.6\n',
                   '[render]\nfont_size_pt = 11\n')
    book = lib.book('hen-na-ie')
    assert book.render['font_size_pt'] == 11      # book.toml wins
    assert book.render['line_height'] == 1.6      # from tategaki.toml
    assert book.render['furigana'] == 'rare'      # built-in default
    assert book.render['font'] == str(tmp_path / 'fonts' / 'NotoSerifJP-Regular.otf')


def test_unknown_keys_are_errors(tmp_path):
    with pytest.raises(ConfigError, match='font_szie_pt'):
        make_lib(tmp_path, '[render]\nfont_szie_pt = 9\n')
    lib = make_lib(tmp_path / 'b', '', '[anki]\ndek = "x"\n')
    with pytest.raises(ConfigError, match='dek'):
        lib.book('hen-na-ie')


def test_bad_choice_and_type(tmp_path):
    with pytest.raises(ConfigError, match='furigana'):
        make_lib(tmp_path, '[render]\nfurigana = "some"\n')
    with pytest.raises(ConfigError, match='scan_len'):
        make_lib(tmp_path / 'b', '[device]\nscan_len = "16"\n')


def test_epub_selection(tmp_path):
    lib = make_lib(tmp_path, epubs=('a.epub', 'b.epub'))
    with pytest.raises(ConfigError, match='exactly one'):
        lib.book('hen-na-ie')


def test_japanese_values_round_trip_to_sidecar(tmp_path):
    lib = make_lib(tmp_path, '[anki]\ntags = ["tategaki"]\n',
                   'title = "変な家"\nauthor = "雨穴"\n[anki]\ndeck = "日本語::変な家"\ntags = ["変な家"]\nstrip_speaker_tags = true\n')
    book = lib.book('hen-na-ie')
    assert book.title == '変な家' and book.author == '雨穴'
    write_sidecar(book)
    data = json.loads(book.sidecar.read_text(encoding='utf-8'))
    assert data == {'deck': '日本語::変な家', 'tags': ['変な家'], 'strip_speaker_tags': True,
                    'sleep_screen': 'cover'}
    assert '日本語::変な家' in book.sidecar.read_text(encoding='utf-8')  # stored as UTF-8, not \u escapes


def test_sleep_screen(tmp_path):
    lib = make_lib(tmp_path, '[reader]\nsleep_screen = "goodnight"\n', '[reader]\nsleep_screen = "cover"\n')
    assert lib.reader['sleep_screen'] == 'goodnight'          # tategaki.toml default
    assert lib.book('hen-na-ie').reader['sleep_screen'] == 'cover'  # book.toml wins
    with pytest.raises(ConfigError, match='sleep_screen'):
        make_lib(tmp_path / 'bad', '', '[reader]\nsleep_screen = "blank"\n').book('hen-na-ie')
