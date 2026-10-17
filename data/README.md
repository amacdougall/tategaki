# data/

- `jmdict.sqlite` is **not committed** (48 MB). It is JMdict plus JPDB
  frequency ranks, built by the Calibre fork's `build_jmdict_sqlite.py`. Copy it
  from `../calibre/src/calibre/gui2/viewer/anki_cards/jmdict.sqlite`, or rebuild
  it with that script's `--download` option.
  - The converter uses it for rare-mode furigana.
  - The reader ships it for lookup (Phase 2).
