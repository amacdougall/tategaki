# Frequency ranking for furigana'd words, so the reader can show furigana only
# on rare words ("rare" furigana mode). The reader sends each ruby group's base
# text plus the text that follows it (okurigana and the rest of a compound,
# e.g. 食 + べたい, 自転 + 車に); we find the longest dictionary word starting
# there and return its JPDB rank.
#
# Stdlib only, no Calibre imports, so it is exercisable from the CLI:
#   python3 furigana.py 食 べたい     # rank for one ruby group
#   python3 furigana.py               # runs the self-test

import re
import sys

try:
    from .deinflect import deinflect
    from .jmdict import UNRANKED, JMdict
except ImportError:
    from deinflect import deinflect
    from jmdict import UNRANKED, JMdict

# Longest run of following text considered part of the ruby word. Enough for
# okurigana plus a common inflection (べさせられた) without scanning whole clauses.
MAX_TRAILING = 6
# Kanji (incl. 々 and ヶ) and hiragana: the characters that can continue a word
# after its ruby base. Katakana and punctuation end it.
_TRAILING_RE = re.compile(r'[々ヶぁ-ゟ一-鿿㐀-䶿豈-﫿]*')


class FuriganaRanker:
    '''Rank ruby groups by corpus frequency, caching per (base, trailing).

    One instance lives for the viewer session; the cache makes re-displaying a
    chapter (or the same word recurring through a book) free.
    '''

    def __init__(self, db=None):
        self.db = db or JMdict()
        self.cache = {}

    def _min_rank(self, terms):
        placeholders = ','.join('?' * len(terms))
        try:
            row = self.db.conn.execute(
                f'SELECT MIN(rank) FROM frequency WHERE text IN ({placeholders})',
                terms).fetchone()
        except Exception:
            return None
        return row[0] if row else None

    def rank(self, base, following=''):
        '''JPDB rank (lower = more common) of the longest dictionary word made of
        ``base`` plus a prefix of the kanji/hiragana that starts ``following``,
        trying longest first and deinflecting each candidate (食べた -> 食べる).
        ``UNRANKED`` if nothing matches, which is how proper names and rare
        words not in the corpus come out -- exactly the ones to keep furigana on.
        '''
        base = (base or '').strip()
        if not base:
            return UNRANKED
        trailing = _TRAILING_RE.match(following or '').group()[:MAX_TRAILING]
        key = (base, trailing)
        ans = self.cache.get(key)
        if ans is None:
            ans = UNRANKED
            for length in range(len(trailing), -1, -1):
                terms = list({d.term for d in deinflect(base + trailing[:length])})
                r = self._min_rank(terms)
                if r is not None:
                    ans = r
                    break
            self.cache[key] = ans
        return ans

    def ranks(self, items):
        '''Ranks for a list of ``[base, following]`` pairs, in order.'''
        return [self.rank(base, following) for base, following in items]


def _selftest():
    r = FuriganaRanker()
    common = [('日本', ''), ('食', 'べたい'), ('行', 'った'), ('自転', '車に'), ('私', 'は')]
    for base, following in common:
        rank = r.rank(base, following)
        assert rank < 20_000, f'{base}+{following} should be common, got {rank}'
        print(f'ok common {base}|{following} -> {rank}')
    for base, following in [('魑魅魍魎', 'が'), ('嘯', 'く'), ('贔屓', 'する')]:
        rank = r.rank(base, following)
        assert rank > 10_000, f'{base}+{following} should be rare, got {rank}'
        print(f'ok rare {base}|{following} -> {rank}')
    # a following particle must not be swallowed into the word: 私は falls back
    # to 私; a sentence break stops the scan.
    assert r.rank('私', 'は') == r.rank('私', ''), '私は should rank as 私'
    assert r.rank('雨', '。晴れ') == r.rank('雨', ''), 'punctuation ends the word'
    print('ok trailing particle / punctuation handling')
    assert r.rank('', 'x') == UNRANKED
    print('All furigana self-tests passed.')


if __name__ == '__main__':
    if len(sys.argv) > 1:
        print(FuriganaRanker().rank(sys.argv[1], sys.argv[2] if len(sys.argv) > 2 else ''))
    else:
        _selftest()
