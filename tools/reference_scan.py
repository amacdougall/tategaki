"""Python reference for the reader's tap scan (plan §7.2, §7.3).

Calibre looks up a *selection*; the reader looks up from a *tap*, scanning
forward from the tapped character the way Yomitan and Hoshi Reader do. This
module defines that scan on top of the vendored Calibre modules, so the Go port
can be checked against golden fixtures generated here (tools/gen_fixtures.py).

    window = text[offset : min(offset + scan_len, para_end)]
    for n in len(window) .. 1:
        hits for window[:n]: exact headword/reading matches of every
        deinflection, filtered by part of speech (Calibre's rules_compatible)
    dedupe by ent_seq keeping the longest source_len, then the shortest chain

There is no partial-inflection completion (擦ら -> 擦る) in the scan. The text
after the tap holds the whole inflection, so some prefix always deinflects
fully, and completing the 16-character window itself never yields a word. A
lookup only ends mid-inflection when the user shortens the span by hand
(span adjust, plan §7.5). That path uses Calibre's lookup_candidates, which
includes forward and partial-inflection completion.

Ranking policies (config `ranking_policy`):
  longest    (source_len desc, chain asc, rank asc, ent_seq)   Yomitan-style
  frequency  (rank asc, source_len desc, chain asc, ent_seq)
"""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'converter' / 'src'))

from tategaki.reference.deinflect import deinflect, rules_compatible  # noqa: E402
from tategaki.reference.lookup import _candidate_entry, _flat_pos  # noqa: E402

SCAN_LEN = 16
MAX_RESULTS = 32

POLICIES = {
    'longest': lambda c: (-c['source_len'], len(c['reasons']), c['rank'], c['ent_seq']),
    'frequency': lambda c: (c['rank'], -c['source_len'], len(c['reasons']), c['ent_seq']),
}


def _entries(db, term):
    return sorted(db.lookup_exact(term), key=lambda e: e['ent_seq'])


def scan(db, text, offset, para_end=None, scan_len=SCAN_LEN, policy='longest', max_results=MAX_RESULTS):
    para_end = len(text) if para_end is None else para_end
    window = text[offset:min(offset + scan_len, para_end)]
    best = {}  # ent_seq -> candidate

    def consider(n, d, entry):
        if not rules_compatible(d.rules, _flat_pos(entry['senses'])):
            return
        rank = db.min_rank(entry['kanji'] or entry['readings'])
        cand = _candidate_entry(entry, d.reasons, d.term, rank=rank)
        cand['source_len'] = n
        old = best.get(cand['ent_seq'])
        if old is None or (-n, len(d.reasons)) < (-old['source_len'], len(old['reasons'])):
            best[cand['ent_seq']] = cand

    for n in range(len(window), 0, -1):
        for d in deinflect(window[:n]):
            for entry in _entries(db, d.term):
                consider(n, d, entry)

    return sorted(best.values(), key=POLICIES[policy])[:max_results]
