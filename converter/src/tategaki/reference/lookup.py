# Dictionary lookup orchestrator: deinflect a selection, match it against
# JMdict, and return the candidate dictionary entries to show the user. This is
# the Python analogue of HoshiReader's native lookup(text, maxResults, scanLength).
#
# Stdlib only, no Calibre imports, so it is exercisable from the CLI:
#   python3 lookup.py 食べている
#   python3 lookup.py            # runs the self-test

import sys

# Dual-mode imports: relative when loaded as a Calibre submodule
# (calibre.gui2.viewer.anki_cards.lookup), bare when run as a CLI script from
# inside this directory (python3 lookup.py ...).
try:
    from .deinflect import complete_partial_inflection, deinflect, rules_compatible
    from .jmdict import UNRANKED, JMdict
except ImportError:
    from deinflect import complete_partial_inflection, deinflect, rules_compatible
    from jmdict import UNRANKED, JMdict

MAX_RESULTS = 16
# When the whole selection finds nothing, retry on slightly shorter leading
# prefixes to absorb an over-selected trailing particle (が, を, から, ...).
# Kept small on purpose: trimming aggressively would let any leading-kana run
# match some short common word and flood the popup with noise.
TRAILING_TRIM = 3
# Cap on forward-completion (prefix) matches folded in per lookup. The panel may
# ask for more; the popup slices to MAX_RESULTS anyway. Keeps 食べ (~dozens of
# completions) from flooding.
PREFIX_LIMIT = 50

# Match tiers, most-certain first. The tier is the *primary* sort key: a match
# that explains the whole selection always outranks a guess at unseen
# characters, regardless of frequency (added as a tiebreaker in a later phase).
TIER_LITERAL = 0      # full selection is a headword/reading, no inflection
TIER_DEINFLECTED = 1  # full selection deinflects to a dictionary-form headword
TIER_PREFIX = 2       # headword/reading starts with the selection (completion)
TIER_TRIM = 3         # last resort: over-selection absorbed by trailing-trim


def _meaning_text(senses):
    '''Flatten an entry's senses into a single '; '-joined gloss string.'''
    glosses = []
    for s in senses:
        glosses.extend(s['glosses'])
    return '; '.join(glosses)


def _flat_pos(senses):
    '''Unique part-of-speech tags across senses, order preserved.'''
    seen = {}
    for s in senses:
        for p in s['pos']:
            seen[p] = True
    return list(seen.keys())


def _candidate_entry(entry, reasons, matched_term, completed_from=None, rank=None):
    '''Shape a JMdict entry (+ the deinflection chain that found it) into the
    JSON-able dict that crosses the bridge and drives the popup / panel.

    ``matched_term`` is the (deinflected) form that hit this entry; when it is
    one of the entry's readings we surface that reading as primary, so a lookup
    by kana shows the reading the user actually selected.

    ``completed_from`` is set to the selection prefix when this entry was reached
    by forward completion (突き込 -> 突き込む); the UI annotates it ("completed
    from 突き込"), mirroring the deinflection annotation built from ``reasons``.
    '''
    kanji = entry['kanji']
    readings = entry['readings']
    senses = entry['senses']
    glosses = [g for s in senses for g in s['glosses']]
    if matched_term in readings:
        primary_reading = matched_term
    else:
        primary_reading = readings[0] if readings else ''
    return {
        'ent_seq': entry['ent_seq'],
        'word': kanji[0] if kanji else (readings[0] if readings else ''),
        'reading': primary_reading,
        'readings': readings,
        'pos': _flat_pos(senses),
        'meaning': _meaning_text(senses),
        'glosses': glosses,
        # Per-sense structure preserved (unlike the flattened ``glosses``) so the
        # Lookup side panel can number senses jisho-style; the popup ignores it.
        'senses': [{'pos': s['pos'], 'glosses': s['glosses']} for s in senses],
        'reasons': reasons,
        'completed_from': completed_from,
        # JPDB corpus frequency rank (lower = more common); tiebreaker within a
        # match tier, chiefly ordering the forward-completion tier jisho-style.
        'rank': UNRANKED if rank is None else rank,
    }


def _collect_exact(db, surface, consider, force_tier=None):
    '''Deinflect ``surface``, look each candidate up, and hand matches to
    ``consider``. Untrimmed calls split into TIER_LITERAL (raw surface, no
    inflection) vs TIER_DEINFLECTED (deinflected); trailing-trim calls pass
    ``force_tier=TIER_TRIM`` to sink every match to the last-resort tier.'''
    for d in deinflect(surface):
        for entry in db.lookup_exact(d.term):
            if not rules_compatible(d.rules, _flat_pos(entry['senses'])):
                continue
            if force_tier is not None:
                tier = force_tier
            else:
                tier = TIER_LITERAL if not d.reasons else TIER_DEINFLECTED
            rank = db.min_rank(entry['kanji'] or entry['readings'])
            consider(tier, len(d.reasons), _candidate_entry(entry, d.reasons, d.term, rank=rank))


def _collect_prefix(db, surface, limit, consider):
    '''Fold jisho-style forward completions of ``surface`` in at TIER_PREFIX.

    Two completion mechanisms, both guesses at unseen trailing characters:
      * headword completion  — 突き込 -> 突き込む (a headword starts with surface)
      * inflection completion — 擦ら  -> 擦る   (surface is a partial inflection)
    '''
    for entry in db.lookup_prefix(surface, limit):
        rank = db.min_rank(entry['kanji'] or entry['readings'])
        consider(TIER_PREFIX, 0, _candidate_entry(entry, [], '', completed_from=surface, rank=rank))
    for comp in complete_partial_inflection(surface):
        for entry in db.lookup_exact(comp.term):
            if not rules_compatible(comp.rules, _flat_pos(entry['senses'])):
                continue
            rank = db.min_rank(entry['kanji'] or entry['readings'])
            consider(TIER_PREFIX, len(comp.reasons),
                     _candidate_entry(entry, comp.reasons, comp.term, completed_from=surface, rank=rank))


def lookup_candidates(selection, max_results=MAX_RESULTS, db=None, prefix_limit=PREFIX_LIMIT):
    '''Return candidate dictionary entries for a selected string.

    Builds one richly-tiered list (see the TIER_* constants): a literal or
    deinflected match on the *whole* selection outranks any forward-completion
    guess, which in turn outranks the trailing-trim last resort. Trailing-trim
    only runs when nothing else matched (over-selection like 食べていますが).
    Deduped by ent_seq keeping the best (tier, chain-length); capped at
    ``max_results``. Empty list if nothing matches.
    '''
    surface = (selection or '').strip()
    if not surface:
        return []
    db = db or JMdict()

    best = {}  # ent_seq -> (tier, chain_len, candidate)

    def consider(tier, chain_len, cand):
        ent_seq = cand['ent_seq']
        key = (tier, chain_len)
        if ent_seq not in best or key < best[ent_seq][:2]:
            best[ent_seq] = (tier, chain_len, cand)

    # Whole-selection matches: literal + deinflected, then forward completion.
    _collect_exact(db, surface, consider)
    _collect_prefix(db, surface, prefix_limit, consider)

    # Last resort: nothing explained the selection, so absorb an over-selected
    # trailing particle by retrying shorter leading prefixes.
    if not best:
        min_len = max(1, len(surface) - TRAILING_TRIM)
        for length in range(len(surface) - 1, min_len - 1, -1):
            _collect_exact(db, surface[:length], consider, force_tier=TIER_TRIM)
            if best:
                break

    # Sort: match tier (primary) -> deinflection-chain length -> frequency rank.
    # Frequency only ever reorders *within* a tier (chiefly the completion tier);
    # it never lifts a lower tier above a higher one.
    ordered = sorted(best.values(), key=lambda t: (t[0], t[1], t[2]['rank']))
    return [cand for _tier, _chain_len, cand in ordered][:max_results]


def _selftest():
    db = JMdict()

    def words(sel):
        return [c['word'] for c in lookup_candidates(sel, db=db)]

    checks = [
        ('食べている', '食べる'),
        ('美味しかった', '美味しい'),
        ('読まない', '読む'),
        ('行かなかった', '行く'),
        ('飲みたい', '飲む'),
        ('食べる', '食べる'),       # already dictionary form
    ]
    failures = 0
    for sel, expected in checks:
        got = words(sel)
        ok = expected in got
        failures += not ok
        print(f'{"ok " if ok else "FAIL"} {sel} -> {expected}  got={got[:6]}')

    # over-generation is filtered: 食ぶ (a bogus deinflection) must not appear
    assert '食ぶ' not in words('食べている'), '食ぶ should be filtered by JMdict'
    print('ok over-generated 食ぶ filtered out')

    # over-selected trailing particle is absorbed by the small prefix trim
    assert '食べる' in words('食べていますが'), 'trailing が should be trimmed'
    print('ok trailing-particle 食べていますが -> 食べる')

    # forward completion: under-selected stems reach the full headword, and are
    # annotated via completed_from
    for partial, expected in (('突き込', '突き込む'), ('漬け込', '漬け込む')):
        cands = lookup_candidates(partial, db=db)
        match = next((c for c in cands if c['word'] == expected), None)
        assert match is not None, f'{partial} should complete to {expected}, got {[c["word"] for c in cands]}'
        assert match['completed_from'] == partial, 'completion must record completed_from'
        print(f'ok completion {partial} -> {expected}')

    # inflection completion: a selection cut off mid-inflection still resolves
    # (擦ら is the passive/negative stem of 擦る; neither headword completion nor
    # plain deinflection reaches it)
    for partial, expected in (('擦ら', '擦る'), ('食べら', '食べる'), ('読ま', '読む')):
        cands = lookup_candidates(partial, db=db)
        match = next((c for c in cands if c['word'] == expected), None)
        assert match is not None, f'{partial} should complete to {expected}, got {[c["word"] for c in cands]}'
        assert match['completed_from'] == partial, 'inflection completion must record completed_from'
        print(f'ok inflection-completion {partial} -> {expected} (reasons={match["reasons"]})')

    # deinflected/literal match on the whole selection outranks any completion:
    # 食べ deinflects to 食べる (masu-stem) which must beat completions like 食べ物
    tabe = lookup_candidates('食べ', db=db)
    assert tabe, '食べ should yield candidates'
    taberu_idx = next((i for i, c in enumerate(tabe) if c['word'] == '食べる'), None)
    completion_idxs = [i for i, c in enumerate(tabe) if c['completed_from']]
    assert taberu_idx is not None, f'食べ should surface 食べる, got {[c["word"] for c in tabe]}'
    if completion_idxs:
        assert taberu_idx < min(completion_idxs), '食べる must rank above completions'
    print(f'ok 食べ -> 食べる ranks above completions ({[c["word"] for c in tabe][:6]})')

    # frequency ranking: within the completion tier, common words come first
    # (jisho shows 食べ物 high among 食べ's completions, not rare compounds)
    tabe_completions = [c['word'] for c in lookup_candidates('食べ', db=db) if c['completed_from']]
    if '食べ物' in tabe_completions:
        rarer = ('食べ痩せ', '食べ掛け', '食べ初め')
        present_rarer = [w for w in rarer if w in tabe_completions]
        for w in present_rarer:
            assert tabe_completions.index('食べ物') < tabe_completions.index(w), \
                f'食べ物 should rank before {w}'
        print(f'ok frequency orders completions: {tabe_completions[:5]}')

    # flood guard: a short common prefix stays capped at max_results
    flood = lookup_candidates('食べ', db=db, max_results=16)
    assert len(flood) <= 16, 'completion flood should be capped'
    print(f'ok 食べ flood capped at {len(flood)}')

    # ambiguous: はし returns several distinct dictionary entries
    hashi = lookup_candidates('はし', db=db)
    assert len({c['ent_seq'] for c in hashi}) >= 2, 'はし should be ambiguous'
    print(f'ok はし -> {len(hashi)} candidates: {[c["word"] for c in hashi][:6]}')

    # non-Japanese input matches nothing
    assert lookup_candidates('xyzzy', db=db) == [], 'latin nonsense -> []'
    print('ok latin nonsense -> []')

    if failures:
        raise SystemExit(f'{failures} lookup case(s) failed')
    print('All lookup self-tests passed.')


if __name__ == '__main__':
    if len(sys.argv) > 1:
        for c in lookup_candidates(sys.argv[1]):
            print(f"{c['word']}  [{c['reading']}]  ({', '.join(c['pos'])})  "
                  f"{c['meaning'][:80]}  reasons={c['reasons']}")
    else:
        _selftest()
