# Read-only JMdict lookup over the bundled jmdict.sqlite (built by
# build_jmdict_sqlite.py). Stdlib only (sqlite3), no Calibre imports, so it can
# be exercised from the CLI with no GUI build:
#   python3 jmdict.py            # runs the self-test
#   python3 jmdict.py 食べる      # dump entries for a surface form

import os
import sqlite3
import sys

DEFAULT_DB_PATH = os.path.join(os.path.dirname(__file__), 'jmdict.sqlite')
UNIT_SEP = '\x1f'  # matches build_jmdict_sqlite.py
# Rank for a form JPDB has no entry for (lower = more common). Sorts unranked
# forms last. Matches FREQUENCY_UNRANKED in build_jmdict_sqlite.py.
UNRANKED = 9_999_999


class JMdictUnavailable(Exception):
    '''The bundled jmdict.sqlite is missing or unreadable.'''


class JMdict:
    '''Thin read-only accessor for the bundled dictionary.

    Open lazily and keep one connection; lookups are single indexed queries so
    this is cheap to call synchronously on the UI thread.
    '''

    def __init__(self, db_path=DEFAULT_DB_PATH):
        self.db_path = db_path
        self._conn = None

    @property
    def conn(self):
        if self._conn is None:
            if not os.path.exists(self.db_path):
                raise JMdictUnavailable(
                    f'JMdict database not found at {self.db_path}. '
                    'Build it with build_jmdict_sqlite.py.'
                )
            # Read-only; tolerate being called from the same (UI) thread only.
            self._conn = sqlite3.connect(
                f'file:{self.db_path}?mode=ro', uri=True)
        return self._conn

    def close(self):
        if self._conn is not None:
            self._conn.close()
            self._conn = None

    def lookup_exact(self, surface):
        '''Return all entries whose kanji headword OR reading equals ``surface``.

        Each entry is a dict:
            {'ent_seq', 'kanji': [...], 'readings': [...],
             'senses': [{'pos': [...], 'glosses': [...]}, ...]}
        Sense order is preserved from JMdict.
        '''
        if not surface:
            return []
        cur = self.conn.cursor()
        rows = cur.execute(
            'SELECT ent_seq FROM kanji WHERE text = ? '
            'UNION SELECT ent_seq FROM reading WHERE text = ?',
            (surface, surface)).fetchall()
        return [self._entry(ent_seq) for (ent_seq,) in rows]

    def lookup_prefix(self, surface, limit=50):
        '''Return entries whose kanji headword OR reading *starts with*
        ``surface`` (left-anchored prefix), for jisho-style forward completion
        of an under-selected stem (突き込 -> 突き込む).

        Exact equals are excluded so callers can tier completions strictly below
        exact/deinflected matches. Implemented as a half-open range scan
        (``text >= surface AND text < surface++``) so the existing BINARY
        ``text`` indexes are used; capped at ``limit`` to guard against floods
        (食べ has dozens of completions). Entry shape matches ``lookup_exact``.
        '''
        if not surface:
            return []
        # Upper bound of the prefix range: surface with its final code point
        # bumped by one, e.g. '突き込' -> '突き' + chr(ord('込')+1).
        hi = surface[:-1] + chr(ord(surface[-1]) + 1)
        cur = self.conn.cursor()
        rows = cur.execute(
            'SELECT ent_seq FROM ('
            '  SELECT text, ent_seq FROM kanji WHERE text >= ? AND text < ? AND text != ? '
            '  UNION '
            '  SELECT text, ent_seq FROM reading WHERE text >= ? AND text < ? AND text != ? '
            ') LIMIT ?',
            (surface, hi, surface, surface, hi, surface, limit)).fetchall()
        return [self._entry(ent_seq) for (ent_seq,) in rows]

    def _entry(self, ent_seq):
        cur = self.conn.cursor()
        kanji = [r[0] for r in cur.execute(
            'SELECT text FROM kanji WHERE ent_seq = ?', (ent_seq,))]
        readings = [r[0] for r in cur.execute(
            'SELECT text FROM reading WHERE ent_seq = ?', (ent_seq,))]
        senses = []
        for pos, gloss in cur.execute(
                'SELECT pos, gloss FROM sense WHERE ent_seq = ? ORDER BY ord',
                (ent_seq,)):
            senses.append({
                'pos': pos.split(UNIT_SEP) if pos else [],
                'glosses': gloss.split(UNIT_SEP) if gloss else [],
            })
        return {'ent_seq': ent_seq, 'kanji': kanji,
                'readings': readings, 'senses': senses}

    def min_rank(self, forms):
        '''Smallest JPDB frequency rank (lower = more common) among ``forms``
        (an entry's kanji + readings), or ``UNRANKED`` if none are ranked.

        Degrades to ``UNRANKED`` if the frequency table is absent (an old
        jmdict.sqlite built before frequency ranking landed), so lookups keep
        working -- they just fall back to insertion order within a tier.
        '''
        forms = [f for f in forms if f]
        if not forms:
            return UNRANKED
        placeholders = ','.join('?' * len(forms))
        try:
            row = self.conn.execute(
                f'SELECT MIN(rank) FROM frequency WHERE text IN ({placeholders})',
                forms).fetchone()
        except sqlite3.Error:
            return UNRANKED
        if row and row[0] is not None:
            return row[0]
        return UNRANKED

    def meta(self):
        '''Return the {key: value} build-provenance map (or {} if absent).'''
        try:
            cur = self.conn.cursor()
            return dict(cur.execute('SELECT key, value FROM meta'))
        except sqlite3.Error:
            return {}


def _selftest():
    db = JMdict()
    print('meta:', db.meta())

    taberu = db.lookup_exact('食べる')
    assert taberu, '食べる should be found'
    e = taberu[0]
    all_pos = {p for s in e['senses'] for p in s['pos']}
    all_read = set(e['readings'])
    assert 'たべる' in all_read, f'reading missing: {all_read}'
    assert 'v1' in all_pos, f'v1 missing: {all_pos}'
    assert any('eat' in g for s in e['senses'] for g in s['glosses']), 'gloss missing'
    print(f'ok 食べる -> {e["ent_seq"]} readings={e["readings"]} pos={sorted(all_pos)}')

    # kana-only surface resolves via the reading column
    assert db.lookup_exact('たべる'), 'kana たべる should resolve'
    print('ok kana たべる resolves')

    # homograph: はし has multiple distinct entries (橋/箸/端/...)
    hashi = db.lookup_exact('はし')
    assert len({x['ent_seq'] for x in hashi}) >= 2, 'はし should be ambiguous'
    print(f'ok はし -> {len(hashi)} entries')

    assert db.lookup_exact('そんなんあるわけない') == [], 'nonsense should be empty'
    print('ok nonsense -> []')

    # forward completion: 突き込 (partial stem) reaches 突き込む
    comp = db.lookup_prefix('突き込')
    comp_kanji = {k for e in comp for k in e['kanji']}
    assert '突き込む' in comp_kanji, f'突き込む should complete 突き込, got {comp_kanji}'
    assert '突き込' not in comp_kanji, 'exact equals excluded from completions'
    print(f'ok prefix 突き込 -> {sorted(comp_kanji)[:6]}')

    # flood guard: a short common prefix is capped
    flood = db.lookup_prefix('食べ', limit=10)
    assert len(flood) <= 10, 'prefix limit should cap results'
    print(f'ok prefix 食べ capped at {len(flood)}')
    print('All JMdict self-tests passed.')


if __name__ == '__main__':
    if len(sys.argv) > 1:
        db = JMdict()
        for entry in db.lookup_exact(sys.argv[1]):
            print(entry)
    else:
        _selftest()
