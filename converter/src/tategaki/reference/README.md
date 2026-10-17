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
