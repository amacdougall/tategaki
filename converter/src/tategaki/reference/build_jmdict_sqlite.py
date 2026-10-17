#!/usr/bin/env python3
# Offline build step: parse JMdict_e into the compact jmdict.sqlite that the
# reader bundles for word lookup. Run once (and again whenever you want to
# refresh the dictionary); the resulting jmdict.sqlite is committed to the repo.
#
#   # download + build in one go (English-only edition):
#   python3 build_jmdict_sqlite.py --download
#
#   # or point at a file you already have (gz or plain XML):
#   python3 build_jmdict_sqlite.py --input ~/Downloads/JMdict_e.gz
#
# Stdlib only. JMdict is from the EDRDG project
# (https://www.edrdg.org/jmdict/j_jmdict.html), distributed under CC BY-SA 4.0.
#
# We keep only what lookup needs: headwords (kanji), readings (kana), and per
# sense the part-of-speech codes + English glosses. That keeps the DB small and
# the schema trivial to query with stdlib sqlite3 (see jmdict.py).

import argparse
import gzip
import hashlib
import io
import json
import os
import re
import sqlite3
import sys
import time
import urllib.request
import xml.etree.ElementTree as ET
import zipfile

JMDICT_E_URL = 'http://ftp.edrdg.org/pub/Nihongo/JMdict_e.gz'
DEFAULT_OUTPUT = os.path.join(os.path.dirname(__file__), 'jmdict.sqlite')

# Pinned JPDB v2 (Kana) frequency dictionary, distributed as a Yomitan zip.
# A rank-based corpus frequency list (lower = more common); the same list the
# Kaishi 1.5k deck was ordered with. Download-once, effectively static -- bump
# by hand if a newer edition is ever published. Kept in sync with the sibling
# anki-example-sentence-builder/lib/frequency_database.rb.
FREQUENCY_URL = ('https://github.com/Kuuuube/yomitan-dictionaries/raw/main/'
                 'dictionaries/JPDB_v2.2_Frequency_Kana_2024-10-13.zip')
# The source ships ~338k term-meta entries; guard against a truncated build.
FREQUENCY_MIN_FORMS = 100_000
# Rank used for a surface form JPDB has no entry for (out-of-dictionary word);
# matches the sentinel the Kaishi cards use. Also defined in jmdict.py.
FREQUENCY_UNRANKED = 9_999_999

# JMdict's pos/misc fields are XML entity refs (&v1;, &n;, ...) whose DTD
# replacement text is a long human description ("Ichidan verb"). We want the
# short code, so before parsing we rewrite every entity to expand to its own
# name: <!ENTITY v1 "Ichidan verb"> becomes <!ENTITY v1 "v1">, after which expat
# resolves &v1; to the literal "v1".
_ENTITY_DEF = re.compile(rb'<!ENTITY\s+(\S+)\s+"[^"]*"\s*>')

UNIT_SEP = '\x1f'  # field-internal list separator (matches kaishi.py convention)


def _read_bytes(path):
    opener = gzip.open if path.endswith('.gz') else open
    with opener(path, 'rb') as f:
        return f.read()


def _rewrite_entities(data):
    return _ENTITY_DEF.sub(rb'<!ENTITY \1 "\1">', data)


def _iter_entries(xml_bytes):
    '''Yield (ent_seq, kanji[], readings[], senses[]) from JMdict XML bytes.

    Each sense is (pos[], glosses[]); a sense with no <pos> inherits the
    previous sense's pos, as JMdict specifies.
    '''
    for _event, elem in ET.iterparse(io.BytesIO(xml_bytes), events=('end',)):
        if elem.tag != 'entry':
            continue
        ent_seq = int(elem.findtext('ent_seq'))
        kanji = [k.text for k in elem.findall('k_ele/keb') if k.text]
        readings = [r.text for r in elem.findall('r_ele/reb') if r.text]
        senses = []
        last_pos = []
        for sense in elem.findall('sense'):
            pos = [p.text for p in sense.findall('pos') if p.text]
            if pos:
                last_pos = pos
            else:
                pos = last_pos
            glosses = [g.text for g in sense.findall('gloss') if g.text]
            if glosses:
                senses.append((pos, glosses))
        yield ent_seq, kanji, readings, senses
        elem.clear()  # keep memory flat across the ~200k entries


def build(input_path, output_path):
    raw = _read_bytes(input_path)
    source_sha256 = hashlib.sha256(raw).hexdigest()
    xml_bytes = _rewrite_entities(raw)

    if os.path.exists(output_path):
        os.remove(output_path)
    conn = sqlite3.connect(output_path)
    try:
        cur = conn.cursor()
        cur.executescript('''
            CREATE TABLE entry (ent_seq INTEGER PRIMARY KEY);
            CREATE TABLE kanji (ent_seq INTEGER, text TEXT);
            CREATE TABLE reading (ent_seq INTEGER, text TEXT);
            CREATE TABLE sense (ent_seq INTEGER, ord INTEGER, pos TEXT, gloss TEXT);
            CREATE TABLE meta (key TEXT PRIMARY KEY, value TEXT);
        ''')

        n_entries = 0
        for ent_seq, kanji, readings, senses in _iter_entries(xml_bytes):
            n_entries += 1
            cur.execute('INSERT INTO entry (ent_seq) VALUES (?)', (ent_seq,))
            cur.executemany('INSERT INTO kanji (ent_seq, text) VALUES (?, ?)',
                            [(ent_seq, k) for k in kanji])
            cur.executemany('INSERT INTO reading (ent_seq, text) VALUES (?, ?)',
                            [(ent_seq, r) for r in readings])
            cur.executemany(
                'INSERT INTO sense (ent_seq, ord, pos, gloss) VALUES (?, ?, ?, ?)',
                [(ent_seq, i, UNIT_SEP.join(pos), UNIT_SEP.join(glosses))
                 for i, (pos, glosses) in enumerate(senses)])

        cur.executescript('''
            CREATE INDEX idx_kanji_text ON kanji (text);
            CREATE INDEX idx_reading_text ON reading (text);
            CREATE INDEX idx_sense_ent ON sense (ent_seq);
        ''')
        cur.executemany('INSERT INTO meta (key, value) VALUES (?, ?)', [
            ('source_sha256', source_sha256),
            ('source_url', JMDICT_E_URL),
            ('built_at', time.strftime('%Y-%m-%dT%H:%M:%S')),
            ('entry_count', str(n_entries)),
        ])
        conn.commit()
        cur.execute('VACUUM')
        conn.commit()
    finally:
        conn.close()

    size_mb = os.path.getsize(output_path) / (1024 * 1024)
    print(f'Built {output_path}')
    print(f'  entries: {n_entries}')
    print(f'  source sha256: {source_sha256}')
    print(f'  size: {size_mb:.1f} MiB')


def _http_get(url):
    '''GET bytes, following redirects (GitHub raw 302s to its CDN).'''
    req = urllib.request.Request(url, headers={'User-Agent': 'calibre-jmdict-build'})
    with urllib.request.urlopen(req, timeout=120) as r:
        return r.read()


def _frequency_value(data):
    '''Pull the numeric rank out of a Yomitan freq meta value, whose shape varies:
        11                                            (bare int / "11")
        {"value": 11, "displayValue": "11㈝"}     (kana headword)
        {"reading": "たべる", "frequency": {"value": 184}}  (kanji headword)
        {"reading": "...", "frequency": 184}          (nested bare int)
    Returns an int rank or None. Mirrors frequency_database.rb#frequency_value.
    '''
    if isinstance(data, bool):
        return None
    if isinstance(data, int):
        return data
    if isinstance(data, str):
        try:
            return int(data)
        except ValueError:
            return None
    if isinstance(data, dict):
        if 'frequency' in data:
            return _frequency_value(data['frequency'])
        if 'value' in data:
            return _frequency_value(data['value'])
    return None


def _iter_frequencies(zip_bytes):
    '''Yield (surface_form, rank) from every term_meta_bank_*.json in the zip.'''
    with zipfile.ZipFile(io.BytesIO(zip_bytes)) as z:
        names = sorted(n for n in z.namelist()
                       if re.search(r'term_meta_bank_\d+\.json$', n))
        if not names:
            raise SystemExit('No term_meta_bank_*.json in frequency zip')
        for name in names:
            for row in json.loads(z.read(name).decode('utf-8')):
                # Each row is [form, mode, data]; we only want frequency modes.
                if len(row) < 3 or row[1] != 'freq':
                    continue
                rank = _frequency_value(row[2])
                if rank is not None:
                    yield row[0], rank


def build_frequency_table(conn, zip_bytes):
    '''(Re)build the frequency(text, rank) table from a JPDB Yomitan zip.

    A surface form recurs (multiple readings + a kana variant); keep the
    smallest rank, i.e. the most common word sharing that form. ``text`` is the
    PRIMARY KEY (already indexed), so jmdict.py can look ranks up directly.
    '''
    best = {}
    for form, rank in _iter_frequencies(zip_bytes):
        cur_best = best.get(form)
        if cur_best is None or rank < cur_best:
            best[form] = rank
    if len(best) < FREQUENCY_MIN_FORMS:
        raise SystemExit(f'Frequency data looks incomplete ({len(best)} forms)')
    cur = conn.cursor()
    cur.execute('DROP TABLE IF EXISTS frequency')
    # WITHOUT ROWID: store rows directly in the text btree (no duplicate text in
    # a separate rowid table + unique index), keeping the committed binary small.
    cur.execute('CREATE TABLE frequency (text TEXT PRIMARY KEY, rank INTEGER) WITHOUT ROWID')
    cur.executemany('INSERT OR IGNORE INTO frequency (text, rank) VALUES (?, ?)',
                    best.items())
    conn.commit()
    return len(best)


def add_frequency(db_path, zip_path=None, download=False):
    '''Add/refresh the frequency table on an existing jmdict.sqlite in place.

    Kept separate from build() so the (slow, network-heavy) JMdict XML rebuild
    isn't needed just to (re)ingest frequency data.
    '''
    if download:
        print(f'Downloading {FREQUENCY_URL} ...')
        zip_bytes = _http_get(FREQUENCY_URL)
        print(f'  Downloaded {len(zip_bytes)} bytes')
    elif zip_path:
        zip_bytes = _read_bytes(zip_path) if not zip_path.endswith('.zip') else open(zip_path, 'rb').read()
    else:
        raise SystemExit('provide --frequency-zip PATH or --download-frequency')
    if not os.path.exists(db_path):
        raise SystemExit(f'{db_path} does not exist; build it from JMdict first')
    conn = sqlite3.connect(db_path)
    try:
        n = build_frequency_table(conn, zip_bytes)
        cur = conn.cursor()
        cur.executemany('INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)', [
            ('frequency_source_url', FREQUENCY_URL),
            ('frequency_forms', str(n)),
            ('frequency_built_at', time.strftime('%Y-%m-%dT%H:%M:%S')),
        ])
        conn.commit()
        cur.execute('VACUUM')
        conn.commit()
    finally:
        conn.close()
    size_mb = os.path.getsize(db_path) / (1024 * 1024)
    print(f'Added frequency table: {n} forms; db now {size_mb:.1f} MiB')


def main():
    p = argparse.ArgumentParser(description='Build jmdict.sqlite from JMdict_e.')
    p.add_argument('--input', help='Path to JMdict_e (.gz or plain XML).')
    p.add_argument('--download', action='store_true',
                   help=f'Download JMdict_e from {JMDICT_E_URL} into a temp file first.')
    p.add_argument('--output', default=DEFAULT_OUTPUT,
                   help='Output sqlite path (defaults next to this script).')
    p.add_argument('--add-frequency', action='store_true',
                   help='Add/refresh the frequency table on the existing DB (no XML rebuild).')
    p.add_argument('--download-frequency', action='store_true',
                   help=f'Download the pinned JPDB frequency zip from {FREQUENCY_URL}.')
    p.add_argument('--frequency-zip', help='Path to a JPDB Yomitan frequency zip.')
    args = p.parse_args()

    if args.add_frequency or args.download_frequency or args.frequency_zip:
        add_frequency(args.output, zip_path=args.frequency_zip,
                      download=args.download_frequency)
        return 0

    input_path = args.input
    if args.download:
        input_path = os.path.join(os.path.dirname(args.output) or '.', 'JMdict_e.gz')
        print(f'Downloading {JMDICT_E_URL} -> {input_path} ...')
        urllib.request.urlretrieve(JMDICT_E_URL, input_path)
    if not input_path:
        p.error('provide --input PATH or --download')

    build(input_path, args.output)
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
