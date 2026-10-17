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
