# Pure-Python Japanese deinflector (verb / i-adjective -> dictionary form).
#
# This is a port of the classic Yomichan deinflection algorithm. The rule table
# in deinflect_rules.json is taken verbatim from Yomichan
# (https://github.com/FooSoft/yomichan, ext/data/deinflect.json, GPL-3.0), the
# same open data used by 10ten-ja-reader and (in spirit) by HoshiReader. Each
# rule rewrites an inflected kana suffix into a less-inflected one, carrying
# part-of-speech "rule flags" so only grammatically valid transitions chain
# together.
#
# Stdlib only (json + os) so it stays importable from Calibre's bundled
# interpreter and unit-testable from the CLI with no Calibre build:
#   python3 deinflect.py
#
# The deinflector deliberately *over-generates* (e.g. 食べている also yields the
# bogus 食ぶ); spurious forms are filtered downstream by checking each candidate
# against JMdict (see lookup.py).

import json
import os

# Rule-type tags used by the table, mapped to bit flags (matching Yomichan).
# A deinflection result carries a bitmask of the part-of-speech classes its term
# is expected to belong to; 0 means "unknown" (only the original surface form).
RULE_TYPES = {
    'v1':    0b0000001,  # ichidan verb
    'v5':    0b0000010,  # godan verb
    'vs':    0b0000100,  # suru verb
    'vk':    0b0001000,  # kuru verb
    'vz':    0b0010000,  # zuru verb
    'adj-i': 0b0100000,  # i-adjective
    'iru':   0b1000000,  # intermediate -iru endings (progressive/perfect)
}

_RULES_PATH = os.path.join(os.path.dirname(__file__), 'deinflect_rules.json')


def _rules_to_mask(rules):
    mask = 0
    for r in rules:
        mask |= RULE_TYPES.get(r, 0)
    return mask


def _load_reasons(path=_RULES_PATH):
    '''Flatten the JSON rule table into (kana_in, kana_out, in_mask, out_mask,
    reason) tuples with the rule flags pre-compiled to bitmasks.'''
    with open(path, encoding='utf-8') as f:
        data = json.load(f)
    reasons = []
    for reason, variants in data.items():
        for v in variants:
            reasons.append((
                v['kanaIn'], v['kanaOut'],
                _rules_to_mask(v['rulesIn']), _rules_to_mask(v['rulesOut']),
                reason,
            ))
    return reasons


_REASONS = _load_reasons()


class Deinflection:
    '''One candidate dictionary form.

    ``term``    the (partly) deinflected surface string.
    ``rules``   bitmask of expected part-of-speech classes (0 == any/unknown).
    ``reasons`` the chain of inflection names applied, outermost first
                (e.g. ['-masu', '-te'] for a doubly-inflected form).
    '''

    __slots__ = ('term', 'rules', 'reasons')

    def __init__(self, term, rules, reasons):
        self.term = term
        self.rules = rules
        self.reasons = reasons

    def __repr__(self):
        return f'Deinflection({self.term!r}, {self.rules:#b}, {self.reasons!r})'


def deinflect(source):
    '''Return all candidate deinflections of ``source`` (Yomichan algorithm).

    The first result is always ``source`` itself (rules=0, reasons=[]), so the
    raw surface form is tried before any morphology. Breadth-first: each result
    is re-fed through every rule, so multi-step inflections unwind fully.
    '''
    results = [Deinflection(source, 0, [])]
    i = 0
    while i < len(results):
        d = results[i]
        i += 1
        term, rules, reasons = d.term, d.rules, d.reasons
        for kana_in, kana_out, in_mask, out_mask, reason in _REASONS:
            # A rule applies only if the current term's known rule flags (if any)
            # intersect the rule's required input flags, the suffix matches, and
            # the rewrite doesn't empty the string.
            if (rules != 0 and (rules & in_mask) == 0):
                continue
            if not term.endswith(kana_in):
                continue
            if len(term) - len(kana_in) + len(kana_out) <= 0:
                continue
            results.append(Deinflection(
                term[:len(term) - len(kana_in)] + kana_out,
                out_mask,
                [reason] + reasons,
            ))
    return results


def complete_partial_inflection(source):
    '''Deinflection guesses for a selection that stops PART-WAY through an
    inflection.

    A reader who selects 擦ら (the first two chars of 擦られる) has given us only
    a *prefix* of the passive suffix. Ordinary ``deinflect`` needs the whole
    suffix (``endswith(kanaIn)``) so it finds nothing, and headword-prefix
    completion can't help because 擦る does not start with 擦ら. Here we treat the
    missing tail as unseen: for every rule whose inflected suffix ``source`` ends
    with a *proper prefix* of, strip that partial suffix and append the rule's
    deinflected suffix (kanaOut), yielding a one-step dictionary-form guess
    (擦ら + passive-rule 'られる'→'る'  =>  擦る).

    Deliberately over-generates; callers look each guess up in JMdict and check
    ``rules_compatible``, so bogus completions vanish. A non-empty stem is
    required so a bare particle (ら) is never completed into a verb.
    '''
    seen = set()
    results = []
    for kana_in, kana_out, in_mask, out_mask, reason in _REASONS:
        # Proper prefixes only (1 .. len-1); a full-suffix match is what ordinary
        # deinflect already handles.
        for plen in range(1, len(kana_in)):
            if not source.endswith(kana_in[:plen]):
                continue
            stem = source[:len(source) - plen]
            if not stem:  # would complete a bare inflection fragment; skip
                continue
            base = stem + kana_out
            if base == source:
                continue
            key = (base, out_mask, reason)
            if key in seen:
                continue
            seen.add(key)
            results.append(Deinflection(base, out_mask, [reason]))
    return results


def pos_to_mask(pos):
    '''Map a single JMdict part-of-speech tag to a RULE_TYPES bit (0 if none).

    JMdict subdivides the godan/suru/adjective classes (v5u, v5k, vs-i, adj-ix,
    ...); we collapse each family onto the single rule-flag the deinflector uses.
    '''
    if pos.startswith('v1'):
        return RULE_TYPES['v1']
    if pos.startswith('v5'):
        return RULE_TYPES['v5']
    if pos.startswith('vs') or pos == 'vs':
        return RULE_TYPES['vs']
    if pos == 'vk':
        return RULE_TYPES['vk']
    if pos == 'vz':
        return RULE_TYPES['vz']
    if pos.startswith('adj-i'):
        return RULE_TYPES['adj-i']
    return 0


def pos_list_mask(pos_list):
    '''OR together pos_to_mask over a JMdict entry's part-of-speech tags.'''
    mask = 0
    for p in pos_list:
        mask |= pos_to_mask(p)
    return mask


def rules_compatible(candidate_rules, entry_pos_list):
    '''Is a JMdict entry a valid target for a deinflection candidate?

    A surface form (candidate_rules == 0) matches anything; otherwise the
    entry must have at least one part-of-speech in a compatible class.
    '''
    if candidate_rules == 0:
        return True
    return (candidate_rules & pos_list_mask(entry_pos_list)) != 0


def _selftest():
    def forms(w):
        return {d.term for d in deinflect(w)}

    cases = {
        '食べている': '食べる',
        '美味しかった': '美味しい',
        '読まない': '読む',
        'した': 'する',
        '来た': '来る',
        '行かなかった': '行く',
        '食べました': '食べる',
        '飲みたい': '飲む',
    }
    failures = 0
    for inflected, expected in cases.items():
        got = forms(inflected)
        ok = expected in got
        failures += not ok
        print(f'{"ok " if ok else "FAIL"} {inflected} -> {expected}  ({"" if ok else sorted(got)})')

    # POS compatibility checks
    assert rules_compatible(RULE_TYPES['v1'], ['v1']), 'v1/v1'
    assert rules_compatible(RULE_TYPES['v5'], ['v5u']), 'v5/v5u'
    assert rules_compatible(RULE_TYPES['vs'], ['vs-i']), 'vs/vs-i'
    assert rules_compatible(RULE_TYPES['adj-i'], ['adj-ix']), 'adj-i/adj-ix'
    assert not rules_compatible(RULE_TYPES['v1'], ['n']), 'v1 should not match noun'
    assert rules_compatible(0, ['n']), 'surface form matches noun'
    print('POS compatibility checks ok')

    if failures:
        raise SystemExit(f'{failures} deinflection case(s) failed')
    print('All deinflection self-tests passed.')


if __name__ == '__main__':
    _selftest()
