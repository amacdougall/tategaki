# data/

- `jmdict.sqlite` is **not committed** (54 MB). It holds JMdict plus JPDB
  frequency ranks.
  - **To create it,** run `python3 tools/prepare_jmdict.py`. This copies the
    Calibre fork's dictionary from
    `../calibre/src/calibre/gui2/viewer/anki_cards/jmdict.sqlite` and adds
    `ent_seq` indexes that the original lacks.
  - **To refresh the source data,** rebuild that file with the fork's
    `build_jmdict_sqlite.py --download`, then run the script again.
  - **Who uses it:** the converter, for rare-mode furigana, and the reader,
    which ships it for lookup.
