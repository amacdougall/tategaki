# Decisions log

Decisions made during implementation, newest last. LOCKED decisions live in
`plans/20261017-tategaki-reader.md`; this file records everything decided since.

## 2026-10-17

- **Jailbreak is Véra, with KPM and no KUAL.** Any plan step or third-party
  readme that assumes KUAL menus has to use `kpm launch …` over SSH or a
  documents-folder scriptlet instead. The plan's scriptlet launcher (§9 Phase 0)
  already fits this.
- **SSH route:** use the `dropbear-ssh` KPM package over Wi-Fi, on port 2022
  (see DEVICE.md). It is prebuilt and reported working on PW5, and it writes
  nothing to rootfs. The usbnetlite KPM build was rejected because it has no
  release and would need building.
- **Device facts confirmed** (DEVICE.md): firmware 5.19.2, `kindlehf`,
  1236×1648 at 8 bpp with a 1248-byte stride, and an MTK controller.
- **Deploying can't use rsync**, because the device doesn't have it. Use
  `scp`/`sftp` or `tar` over ssh. This changes how plan §5.8 is implemented, not
  what it does.
- **KPM calls in scripts always use `-y` and `</dev/null`**, after the
  interactive-prompt failure.
- **Phase 0 complete.** The KPM package (`packaging/kpm/`) installs, launches
  from the Library and over SSH, pauses and restores the Kindle UI, and
  survives sleep, wake and SIGKILL.
- **FBInk:** use the CLI, not cgo, and the vendored 1.25.0 libkh build, because
  it is the only one on hand with image support. Latencies are in DEVICE.md.
- **UI pause and restore live in `launch.sh`** (KOReader's FW ≥ 5.12.4 recipe:
  pillow off, SIGSTOP awesome, framebuffer dump and restore, home booklet), with
  `trap … EXIT`. No `wmctrl`.
- **No `preventScreenSaver`,** because it disables the power button. The reader
  calls `touchScreenSaverTimeout` on input, at most every 30 s.
- **Helper processes get `Pdeathsig=SIGKILL`,** so a crashed reader doesn't
  leave `lipc-wait-event` running.
- **Package id** is `tategaki`, and the Library entry is "Tategaki Reader". The
  package is versioned 0.0.x until the project is renamed (plan §11, open
  question 5).

## 2026-10-17 (Phase 1)

- **No LICENSE file.** At the user's request, ignore the plan's GPL-3.0 note.
- **Library location:** `~/Tategaki/` (plan default). The test book is
  `books/toradora-01/` (`toradora_01.epub`, copied from the Kindle). Fonts live
  in `~/Tategaki/fonts/`: Noto Serif JP (default), Shippori Mincho and
  BIZ UDPMincho, all OFL.
- **Desktop entry point:** `bin/tategaki` wraps `uv run tategaki` (build,
  deploy, preview, proof, convert).
- **Pagination: strip slicing worked**, so there's no Vivliostyle fallback. The
  first whole-book render took 31 s for 279 pages.
  - **Bands come from em boxes,** not client rects. Chromium's per-character
    rects span the font's ascent and descent (about 1.45 em for Noto CJK), and
    ruby boxes bridged the inter-column gap, merging up to four columns into
    one uncuttable band. Using em boxes (column centre ± font-size/2, plus the
    ruby characters' em boxes) gives one band per column, plus ruby. Glyph
    boxes for hit testing are still the real client rects.
  - **Each page image extends halfway into the gaps on either side** (at most
    12 px), so ink slightly outside an em box isn't clipped.
- **deviceScaleFactor is 1:** CSS px are device px, and the root font size is
  `pt * dpi / 72` px. The book CSS is em-based, so this is equivalent and
  avoids a fractional viewport.
- **Book JS is disabled** (`kobo.js` etc.). Pages load to `domcontentloaded`,
  because the `load` event never fired for this book. With JS disabled,
  Chromium can't call back into JS (for example, `TreeWalker` filter
  callbacks), so `layout.js` filters by hand.
- **Illustrations embedded in chapters** (a block containing only one
  `<img>`/`<svg><image>`, which is how this book does all of its mid-chapter
  plates) are cut out of the text strip and become full-screen illustration
  pages at that point. The plan only described whole-spine-item
  illustrations; both now work.
- **Ruby from the book:** per-character ruby (`苛<rt>いら</rt>立<rt>だ</rt>`) is
  grouped as in `furigana.pyj`. The furigana JS is a direct port, and the
  ranker is the vendored `FuriganaRanker` (`converter/src/tategaki/reference/`).
- **Sentence spans: the plan needs a decision here (§5.4, §7.6).** The
  reference algorithm is offset-dependent, not a splitter. Tapping inside a
  multi-sentence quotation yields the inner sentence; tapping outside it yields
  the whole enclosing sentence. A fixed partition of spans can't express that.
  - **What we did:** the `.jbk` stores a partition (each span is "the sentence
    that starts here"), and tests check it against the reference at every span
    start. `sentence_bounds` reproduces the reference at every offset.
  - **Recommendation for Phase 5:** port the ~40-line scan to Go and run it on
    the device over the paragraph text, instead of using stored spans. Then
    mining matches Calibre exactly.
- **Deploy uses `scp` with temp-file-then-`mv`,** not rsync. The device keeps
  `books/.deployed.json` (book id → build_hash) to skip unchanged books.
- **Rebuild detection:** `build_hash` covers the EPUB, the resolved render
  settings, the font file contents, the profile, and a hash of the converter's
  own source, so converter changes rebuild books automatically.
- **Phase 3 preview slice:** the device binary now opens the newest `.jbk` in a
  minimal page viewer (tap left half = next, right half = previous, top strip =
  exit), so proof sheets can be judged in hand. `-hello` keeps the Phase 0
  test.
- **Default typography: Noto Serif JP at 14 pt, line height 1.75,** chosen by
  the user from on-device proofs (8–16 pt) so it reads without glasses. That
  gives about 10 columns per page; the book comes to 557 pages.
- **Bug fix: furigana could be paginated away from its base word.**
  - **Symptom:** at 16 pt, the ruby of 高須竜児 appeared at the left edge of
    the previous page.
  - **Cause:** a ruby em box can sit a fraction of a pixel away from its base
    column, so the ruby formed its own band.
  - **Fix:** ruby bands are extended one ruby em leftward into the base
    column, so they always merge. Checked for ruby-only bands from 8 to
    18 pt in two chapters, and a regression test was added.
- **Phase 1 accepted by the user (2026-10-17).** Pages were reviewed on the
  device and in the preview, and typography was chosen from proofs. Next:
  Phase 2 (lookup engine in Go).

## 2026-10-17 (Phase 2)

- **Lookup engine:** `reader/internal/dict`.
  - It's a port of the vendored Calibre `deinflect.py`, `jmdict.py` and
    `lookup.py`. The Yomichan rules are embedded verbatim, and are read in file
    order, because Python dict order is part of result order.
  - SQLite is `modernc.org/sqlite` (pure Go). The static ARM binary works on
    the Kindle.
- **Golden fixtures:** `tools/gen_fixtures.py` writes them to `testdata/`. They
  cover deinflection, partial-inflection completion, `lookup_candidates`, the
  tap scan (both policies), Kaishi fields and sentence extraction. Go
  reproduces deinflection, lookup and scan exactly, and the tests were checked
  to fail on corrupted fixtures.
- **The tap scan's reference is in Python,** `tools/reference_scan.py`, built on
  the Calibre modules, because Calibre has no tap scan. The Go scan batches all
  queries (exact matches, entries, ranks); the reference does per-term queries.
  Both sort completely (ent_seq as the last key), so they agree.
- **No partial-inflection completion in the scan.** The text after a tap holds
  the whole inflection, and completing the 16-character window never yields a
  word. Completion matters only for a hand-shortened span (span adjust, plan
  §7.5), which uses `LookupCandidates`, the Calibre port, completions included.
  This changes how plan §7.2's "partial inflections" bullet is implemented.
- **Ranking policies:**
  - `longest` (default) sorts by (source_len desc, chain asc, rank asc).
  - `frequency` sorts by (rank asc, source_len desc, chain asc).
  - For 蜘蛛の巣, `longest` puts 蜘蛛の巣 first and `frequency` puts 蜘蛛 first.
    Which should be the default is still open (plan §7.3), to be decided in
    Phase 4 testing.
- **jmdict.sqlite gets `ent_seq` indexes** (`tools/prepare_jmdict.py`). The
  Calibre file lacks them, so every entry fetch was a full table scan. Row
  order is unchanged.
- **Timings:**
  - Desktop: the worst cold scan over a 55-character sentence takes 1.2 ms
    (Python `lookup_candidates` took up to 460 ms).
  - Kindle: scans take 20–47 ms (139 ms when nothing matches), and
    `LookupCandidates` takes 180 ms. That's well inside the 700 ms
    tap-to-popup budget.

## 2026-10-17 (Phase 3)

- **Startup resumes** the most recently read book at its saved place,
  requested by the user. The library opens instead if there's no history or
  that book has been removed. This replaces plan §8.1's "library first".
- **Positions:**
  - `state.json` stores `{chapter, offset, image, page, page_count}`. Offset
    is the first glyph on the page; for an illustration page it's the offset of
    the text after it, with `image: true`. `page` breaks ties between pages that
    share a position (the halves of a split spread).
  - The mapping round-trips for every page of the test book, and an offset
    from another layout lands on the page containing it (ready for Phase 6
    size variants).
- **UI font:** BIZ UDPGothic Regular/Bold (TrueType, OFL), shipped in the
  package's `fonts/`. `x/image` may not handle CID-keyed CJK OpenType fonts.
  The font has no ⌫ glyph, so the keypad says "Del".
- **UI images reach FBInk as raw PGM files on tmpfs.** FBInk's stb_image reads
  PGM, so the device never has to compress PNGs.
- **The paused Kindle UI can still draw for a few seconds after launch,** for
  example the home screen finishing a redraw. The reader redraws its screen
  2 s and 5 s after starting (`settle`).
- **Sleep shows no screensaver:** with `awesome` stopped, the e-ink keeps our
  page when the device sleeps. The user said this is fine for now. A "sleeping"
  cover screen could be drawn on `goingToScreenSaver` later.
- **Simulator:** run `tools/sim.sh` and open http://localhost:8800. It links
  the built books into `./simroot` (git-ignored), so its state is separate from
  the Kindle's.
- **Phase 3 accepted by the user (2026-10-17)** after testing on the device.

## 2026-10-17 (Phase 4)

- **Ranking default stays `longest`.** The user confirmed it after testing
  (plan §7.3, open question 3): "Greedily finding the longest dictionary entry
  starting from the cursor is fine, given that we have the frequency ranks."
- **Taps:**
  - A tap on a lookupable character always looks it up. That includes the
    first characters of every column, which sit inside the menu's top strip,
    and the outer columns, which sit inside the plan's edge zones.
  - The menu opens from the margin or gaps in the top strip.
  - Pages turn by swiping, or by tapping blank space: left half next, right
    half previous.
  - `edge_zone_pct` therefore has no separate effect.
- **Highlight:** a side-line on the *left* of each column. Plan §7.4 said
  right, but in vertical text the ruby is on the right.
- **Popup** (user's choices after trying it):
  - It covers 3/4 of the screen, on the side away from the tapped character.
    Four collapsed entries fit per sheet page.
  - Swipe up/down to page (a "1 / 2" indicator shows when there's more).
  - A × close button sits at the upper right. There are no ▲▼ buttons.
  - Tap a row to select it (the highlight follows) and tap again to expand all
    senses.
  - ◀ ▶ shorten or extend the span end and re-run `LookupCandidates`.
  - The raw row shows when nothing matches.
- **The dictionary is deployed by `tategaki deploy`** to
  `/mnt/us/tategaki/jmdict.sqlite`, sent only when its sha256 changes. It isn't
  in the KPM package, so app reinstalls stay small. Plan §6.1 placed it in the
  package.
- **Timings:**
  - Kindle: tap-to-popup 218–369 ms over 11 real lookups (scan 4–114 ms). The
    budget is 700 ms.
  - Simulator: about 20 ms.
- **Test book:** `tools/make_test_epub.py` writes `books/lookup-test`
  (辞書テスト) with the plan's example sentences.
- **Converter rebuild hash** now covers only rendering sources
  (`RENDER_SOURCES` in `library.py`), so CLI or deploy edits don't re-render
  every book.
- **Phase 4 accepted (2026-10-17).**
