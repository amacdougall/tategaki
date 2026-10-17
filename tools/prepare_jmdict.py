#!/usr/bin/env python3
"""Prepare data/jmdict.sqlite for the reader.

Copies the Calibre-built dictionary (build_jmdict_sqlite.py output) and adds
the indexes the reader needs: the original has no index on kanji.ent_seq or
reading.ent_seq, so every entry fetch scans ~250k rows. Indexes don't change
row order (rows with equal ent_seq still come back in rowid order), so lookups
return identical results.

Usage: python3 tools/prepare_jmdict.py [SRC] [DST]
"""

import shutil
import sqlite3
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_SRC = ROOT.parent / 'calibre' / 'src' / 'calibre' / 'gui2' / 'viewer' / 'anki_cards' / 'jmdict.sqlite'
DEFAULT_DST = ROOT / 'data' / 'jmdict.sqlite'


def main() -> int:
    src = Path(sys.argv[1]) if len(sys.argv) > 1 else DEFAULT_SRC
    dst = Path(sys.argv[2]) if len(sys.argv) > 2 else DEFAULT_DST
    if src.resolve() != dst.resolve():
        shutil.copyfile(src, dst)
    con = sqlite3.connect(dst)
    con.executescript('''
        CREATE INDEX IF NOT EXISTS idx_kanji_ent ON kanji (ent_seq);
        CREATE INDEX IF NOT EXISTS idx_reading_ent ON reading (ent_seq);
        INSERT OR REPLACE INTO meta (key, value) VALUES ('tategaki_indexes', 'kanji.ent_seq,reading.ent_seq');
        ANALYZE;
    ''')
    con.commit()
    con.execute('VACUUM')
    con.close()
    print(f'{dst}: {dst.stat().st_size / 1e6:.1f} MB')
    return 0


if __name__ == '__main__':
    sys.exit(main())
