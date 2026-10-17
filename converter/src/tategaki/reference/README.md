# Reference modules (vendored)

Unmodified copies from the user's Calibre fork, `../calibre`, branch
`customized`, commit `f93440cfb5`, path `src/calibre/gui2/viewer/anki_cards/`.
These are the reference implementation (plan §3). Don't edit them here; port
or wrap them instead.

- `furigana.py`: `FuriganaRanker` (rare-mode furigana)
- `deinflect.py` and `deinflect_rules.json`: Yomichan deinflection, which
  `furigana.py` uses
- `jmdict.py`: SQLite accessor. Pass the repo's `data/jmdict.sqlite`
  explicitly, because its default path points next to this file.
- `lookup.py`: `lookup_candidates`. Ported to Go (`reader/internal/dict`).
- `kaishi.py`, `anki_connect.py`: Anki note fields and client (Phase 5).
- `sentence_extraction_reference.py`: the sentence scan and speaker-tag cases.
  It prints its demo cases on import.
- `build_jmdict_sqlite.py`: builds `jmdict.sqlite`. See `data/README.md`.

These modules are also the reference for the golden fixtures
(`tools/gen_fixtures.py` → `testdata/`).
