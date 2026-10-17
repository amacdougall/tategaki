# Tategaki Reader — Implementation Plan

A Japanese ebook reader for jailbroken Kindles with:
- vertical (縦書き, right-to-left) text,
- Hoshi/Yomitan-style tap lookup against an offline JMdict with JPDB frequency ranks,
- Anki mining using the Kaishi 1.5k note type.

It is installed and launched through KPM (Kindle Package Manager).

"Tategaki Reader" is a placeholder name. Rename freely.

---

## 0. Instructions for the implementing agent

- Read this whole document before writing code.
- Work **phase by phase** (§9). Each phase has acceptance criteria; don't start the next phase until they pass.
- Decisions marked **LOCKED** were settled in design discussion.
  - Don't change a LOCKED decision without asking the user first.
  - Items marked **VERIFY** are assumptions about external tools or the device that must be checked before relying on them.
- Keep a running `docs/DECISIONS.md` log for anything decided during implementation.
- You may run commands on the Kindle directly over SSH (§0.1). The user has accepted the risk of a factory reset.
  - Stay inside the guardrails in §0.1.
  - Still develop and test on the desktop first (simulator, §6.2). Use the device to verify, not to iterate.
- Much logic already exists in Python in the user's Calibre fork (§3). Treat it as the reference implementation.
  - Port that code; don't reinvent it.
  - Use golden-file tests generated from the Python code to prove parity.

### 0.1 Device access and guardrails

**Connection (VERIFY in Phase 0):**
- SSH to the Kindle as root. Two possible routes:
  - the USBNetwork hack, over USB or Wi-Fi,
  - KOReader's built-in SSH server (dropbear), over Wi-Fi.
- Check which one is installed and record the host, port and key setup in `docs/DEVICE.md`.
- Use key-based auth, and add a `kindle` host alias in `~/.ssh/config` so commands are short.
- macOS may not support the Kindle's USB-network mode. If USB networking fails, use Wi-Fi.
- **Deploying:** use `scp` or `rsync` to `/mnt/us/…`.
  - Do **not** rely on USB mass-storage mode while testing: `/mnt/us` is unmounted on the device while the Mac has it mounted, so the app and its data disappear.
- **Running:** start the app via `kpm launch <pkgname>` or the package's `launch.sh` over SSH, and capture `/mnt/us/tategaki/log.txt`.
- **Screenshots:** grab the screen with `fbgrab`, or with `fbink`'s dump option if `fbgrab` is missing, then copy the image back to the Mac and look at it. This is how you confirm what the screen shows.
- **Taps:** automated tap testing can inject input with `evemu` or by writing to `/dev/input`, if available. Otherwise ask the user to tap and report.

**Guardrails (LOCKED):**
- Never write to the root filesystem, and never run `mntroot rw`. All files go under `/mnt/us/`, plus `/var/local/` only where KPM itself puts things.
- Never run `dd`, `mkfs`, `fw_setenv`, or anything that flashes, partitions or touches the bootloader.
- Never place `update*.bin` files on the device, never trigger "Update your Kindle", and never re-enable OTA updates.
- Never delete anything outside `/mnt/us/tategaki/` and the package's own files.
- Before the first device session, back up the parts of `/mnt/us` the user cares about (documents, the `extensions` folder, `koreader`). Ask the user which parts first.
- Every launch path must restore the Kindle UI on exit, including after a crash (`trap` in `launch.sh`). If the device gets stuck, ask the user to hold the power button for about 40 seconds to force a reboot.
- Ask the user before any command whose effects you can't undo, and before changing system settings through `lipc-set-prop` beyond the screensaver and framework controls described in Phase 0.

**Why a factory reset is not entirely free:** a reset removes the jailbreak. If the Kindle then connects to Wi-Fi and auto-updates to a firmware version with no available jailbreak, it can't be re-jailbroken. If a reset ever happens, keep the device in airplane mode until it has been re-jailbroken.

---

## 1. Architecture overview (LOCKED)

**Layout happens on the desktop. The Kindle only displays pre-rendered pages and does lookup.**

```
 DESKTOP                                      KINDLE
 ┌───────────────────────────┐   USB copy    ┌───────────────────────────────┐
 │ converter (Python)        │ ───────────▶  │ reader (Go, static binary)    │
 │  EPUB → Chromium layout   │   book.jbk    │  shows page PNGs (FBInk)      │
 │  vertical-rl, ruby, kinsoku│              │  tap → char offset → scan     │
 │  → page PNGs              │               │  JMdict lookup + deinflect    │
 │  → glyph boxes + text     │               │  popup, highlight, mine card  │
 │  → sentence spans         │               │  Anki queue (JSONL)           │
 └───────────────────────────┘               └──────────────┬────────────────┘
                                                            │ Wi-Fi (when reachable)
                                              ┌─────────────▼────────────────┐
                                              │ desktop Anki + AnkiConnect   │
                                              └──────────────────────────────┘
```

Why this split:
- No Kindle-side layout engine supports vertical Japanese text with ruby.
- Chromium handles vertical text, ruby, kinsoku and images very well.
- The cost is that font size and furigana mode are chosen at conversion time; changing them means reconverting.

---

## 2. Repository layout

```
/converter/            Python package (managed with uv). EPUB → .jbk
/reader/               Go module. The on-device app.
  cmd/tategaki/        main
  cmd/lookup/          desktop CLI for the lookup engine
  internal/book/       .jbk reader
  internal/dict/       deinflect, jmdict, scan, rank
  internal/anki/       Kaishi fields, queue, AnkiConnect client
  internal/sentence/   speaker-tag stripping (sentence spans come from the converter)
  internal/display/    Display interface; fbink and sim backends
  internal/input/      Input interface; evdev and sim backends
  internal/kindle/     framework pause/resume, powerd, lipc helpers
  internal/config/     reads config.json, sidecars, state.json (device side)
  internal/ui/         library, page view, popup, menu, book info
  internal/text/       glyph rasterising for the UI (popup etc.)
/packaging/kpm/        manifest.json, install.sh, launch.sh, uninstall.sh, build script
/data/                 jmdict.sqlite build, deinflect_rules.json, fonts
/tools/                flush_queue.py (USB fallback), bundle preview tool
/testdata/             golden fixtures generated from the Python reference
/docs/                 PLAN.md (this file), DECISIONS.md
```

License: **GPL-3.0**, because the deinflection rules come from Yomichan, which is GPL-3.

---

## 3. Existing code to reuse

Source repository: `github.com/amacdougall/calibre`, branch `customized`. Locate files with grep; the paths below are by package or module name.

The `anki_cards` package is stdlib-only and has no Calibre imports:

| Module | What it is | Used by |
|---|---|---|
| `deinflect.py` + `deinflect_rules.json` | Port of Yomichan deinflection, including `complete_partial_inflection` (擦ら → 擦る) | Port to Go |
| `build_jmdict_sqlite.py` | Builds `jmdict.sqlite`, including `--add-frequency` (JPDB v2 rank table) | Reuse as-is in `/data` |
| `jmdict.py` | Read-only SQLite accessor: `lookup_exact`, `lookup_prefix`, `min_rank` | Port to Go |
| `lookup.py` | `lookup_candidates`: ranking by tier → chain length → JPDB rank; rank by written form, reading only for kana-only entries | Port to Go (adapted to scanning, §7) |
| `kaishi.py` | `build_fields` (Kaishi 1.5k field mapping) | Port to Go |
| AnkiConnect client | Includes `build_note` with collection-wide `duplicate_scope` | Port to Go |
| `furigana.py` (`FuriganaRanker`) | JPDB rank per ruby word | Use **in the converter** directly |
| `sentence_extraction_reference.py` | Python twin of the bracket-depth sentence splitter, plus speaker-tag test cases | Use **in the converter** directly; port only speaker-tag stripping to Go |

Viewer-side `.pyj` code, used for behaviour reference only:
- `anki_sentence.pyj`: sentence extraction and speaker-tag stripping.
- `selection_text.pyj`: strips rt/rp/rtc from selections.
- `read_book/furigana.pyj`: "rare" mode. Joins runs of single-character rubies into one word, never hides katakana or emphasis rubies, and hides with `visibility: hidden` so layout doesn't change.

**Golden fixtures** (Phase 2): write `tools/gen_fixtures.py`, which imports the Python package and writes JSON fixtures to `/testdata`:
- lookup candidates for a word list,
- deinflection chains,
- Kaishi fields,
- speaker-tag cases.

The Go tests must reproduce these exactly. Seed the word list with every example from the commit messages: 突き込, 擦ら, 食べ, 並々ならぬ, 悠々, 蜘蛛の巣, する/擦る, 筆者　えーと….

---

## 4. Target device

**LOCKED:** Kindle Paperwhite, 11th generation (2021), firmware **5.19.2**.

- **Device profile `pw11`:** 1236×1648 pixels, 300 ppi, portrait. Stored in `converter/profiles/pw11.json`.
  - **VERIFY** the resolution on the device (`fbink -e` or `/sys/class/graphics/fb0/virtual_size`).
  - Also check whether the framebuffer has a stride or padding wider than the visible area.
- **KPM platform:** `kindlehf`. Firmware 5.16.3 and later uses a hard-float userland.
  - Build with `CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7`.
  - Ship the `kindlehf` FBInk build.
  - Other platforms are out of scope unless the user asks.
- **Display controller:** this model uses an MTK e-ink controller, not the older i.MX one. FBInk handles that, so don't write refresh ioctls by hand. **VERIFY** that the FBInk version supports this model with partial and full refresh.

---

## 5. Converter (desktop, Python)

The converter is driven by config files in a desktop library folder (§5.8). Commands:

```
tategaki build  [BOOK_DIR ...]            # convert books whose inputs changed (default: all)
tategaki deploy [BOOK_DIR ...] [--settings-only] [--usb /Volumes/Kindle]
tategaki proof  BOOK.epub --pages 4 --font-size-pt 8,9,10 [...]   # Phase 1, see §9
tategaki convert BOOK.epub [--font-size-pt …] -o BOOK.jbk         # low-level, flags only; used by build
```

### 5.1 Pagination approach (LOCKED primary, with fallback)

The primary approach is **headless Chromium via Playwright, with our own "strip slicing" paginator.** Per spine item:

1. **Build the page.** Load the XHTML with injected CSS:
   - `writing-mode: vertical-rl` (forced, and also honour `-epub-writing-mode`)
   - `line-break: strict`
   - the chosen font and size
   - margins from the profile
   - `img { max-height: <content height>; max-width: <content width> }`
2. **Lay out as a strip.** Set the container height to the page content height and let it grow leftward (the block direction) as a single strip.
3. **Collect boxes.** Collect glyph boxes (§5.2) and image boxes.
4. **Group into columns.** Group glyph boxes into line boxes (vertical columns) by x-range, including ruby.
5. **Pack pages.** Cut pages only in the gaps *between* columns, packing columns greedily into the content width from right to left. A block that is wider than the remaining space starts a new page.
6. **Hard breaks.** Each spine item starts on a new page.
7. **Screenshot.** Screenshot each page region using a viewport at device resolution. Use `deviceScaleFactor` so that CSS pixels map to device pixels, and translate glyph boxes into page-local device pixels.

**Fallback:** if strip slicing produces unacceptable results, try Vivliostyle, a CSS typesetting engine for Japanese that runs in Chromium. Unacceptable means:
- clipped columns,
- broken ruby,
- bad image handling,
- or more than one in fifty pages visually wrong in the preview tool.

Record which approach was used in DECISIONS.md.

### 5.2 Text and glyph extraction

- **Walk text nodes** in document order, skipping `rt`, `rp` and `rtc` content. This is the furigana-free text stream, the same idea as `selection_text.pyj`.
- **Chapter text:** concatenate the text, inserting `\n` at paragraph and block boundaries.
- **Glyph boxes:** for each character, use a `Range` and `getClientRects()` to get its box, and record `[offset, x, y, w, h]`.
- **Offsets are Unicode code point indices** into the chapter text, not UTF-16 indices. JavaScript strings are UTF-16, so convert carefully: rare kanji are surrogate pairs.
- **Text cleanup:** normalise whitespace consistently, so that the offsets the device uses match the text the device reads.

### 5.3 Furigana

- **`rare` mode is baked in at conversion time.** Use `FuriganaRanker` with the same rules as `furigana.pyj`.
- **Hide with `visibility: hidden`**, never `display: none`. That way all modes produce **identical glyph boxes**.
- **Stretch goal:** emit two PNG variants per page (`all` and the chosen mode) and toggle between them on the device, sharing one layout.

### 5.4 Sentences

Run the Python bracket-depth sentence splitter over each chapter's text and store the sentence spans `[start, end)` as code point offsets. Speaker-tag stripping is applied on the device, per book (§7.6).

### 5.5 Images (LOCKED)

Light novels have many illustrations, so images get their own path rather than going through text layout.

- **Full-page illustrations**
  - **Detection:** a spine item whose visible content is a single `<img>` or `<svg><image>` (the usual light-novel insert and colour-plate pattern). Ignore whitespace and empty wrappers.
  - **Rendering:** skip Chromium. Decode the original image with Pillow, scale it to fit the full screen with no margins (Lanczos, centred, letterboxed with white), and write it as that page's PNG.
  - **Layout:** the page gets an empty `glyphs` list and one image box covering the image.
- **Two-page spreads (見開き):** a landscape image (width > height × 1.2, the threshold is a flag), handled by `--spreads split|rotate|fit`.
  - `split` (default) cuts the image down the middle into two pages, right half first, since the reading order is right-to-left.
  - `rotate` turns it 90° to fill the portrait screen.
  - `fit` letterboxes it.
- **E-ink image processing**, applied to illustration pages and inline-image copies:
  - convert to grayscale,
  - apply a gamma or contrast curve (a flag),
  - quantise to 16 levels with Floyd–Steinberg dithering.
  - **Text pages are not dithered.** They are only quantised.
- **Inline images** are still drawn into the page PNG during layout. In addition, write a screen-fitted copy (same processing) to `images/<hash>.png`, and record it on the page's image entry so the reader can show it full-screen when tapped.
- **Cover:** a library thumbnail written to `cover.png`, from the EPUB's cover image, falling back to the first page.

### 5.6 Output post-processing

- Save PNGs as 8-bit grayscale.
- Optionally quantise to 16 levels, since the panel is 4-bit, and apply a gamma or contrast tweak. Make both converter flags.

### 5.7 Preview tool (required, Phase 1)

`tools/preview` generates a local HTML page for a `.jbk` with:
- page navigation,
- the page PNG with glyph-box overlays,
- click a glyph to show its offset, the surrounding text and its sentence span.

This is how the converter gets verified without the device.

### 5.8 Configuration (LOCKED)

**All configuration is written on the laptop, in TOML files.** The Kindle has no settings screens.
- Editing on the laptop makes Japanese input (deck names, tags) trivial.
- The device only *reads* configuration. The only state it writes is reading position and the Anki queue.

**Desktop library folder** (location set by `TATEGAKI_HOME`, default `~/Tategaki`):

```
~/Tategaki/
  tategaki.toml                 global: device settings + render defaults
  fonts/                        reading fonts referenced by render settings
  books/
    hen-na-ie/                  one folder per book; the folder name is the book id
      変な家.epub
      book.toml                 per-book overrides (optional; all keys optional)
      build/hen-na-ie.jbk       build output
      build/hen-na-ie.json      settings sidecar (generated)
```

`tategaki.toml`:

```toml
[device]
profile = "pw11"
ssh_host = "kindle"                       # ~/.ssh/config alias
ankiconnect_url = "http://192.168.1.20:8765"
anki_api_key = ""
default_deck = "Default"
note_type = "Kaishi 1.5k"
ranking_policy = "longest"                # §7.3
scan_len = 16
full_refresh_every = 6
edge_zone_pct = 12

[render]                                  # defaults for every book
font = "fonts/NotoSerifJP-Regular.otf"
font_size_pt = 9
line_height = 1.75
furigana = "rare"                         # all | rare | none
rare_threshold = 10000
spreads = "split"                         # split | rotate | fit

[anki]                                    # defaults for every book
tags = ["tategaki"]
strip_speaker_tags = false
```

`books/<id>/book.toml`, where every key is optional:

```toml
epub = "変な家.epub"        # default: the only .epub in the folder
title = "変な家"            # override the EPUB metadata
author = "雨穴"

[render]
font_size_pt = 10

[anki]
deck = "日本語::変な家"
tags = ["変な家"]
strip_speaker_tags = true
```

**Precedence:** built-in defaults < `tategaki.toml` < `book.toml`. Validate against a schema, and fail with a clear message on unknown keys, which are usually typos.

**Two kinds of per-book setting.** Only the first kind needs a rebuild.

1. **Render settings** (`[render]`, `title`, `author`) are baked into the `.jbk`.
   - `build` stores a hash of the EPUB plus the resolved render settings in the manifest (`build_hash`), and skips books whose hash is unchanged.
2. **Behaviour settings** (`[anki]`) go into a small sidecar, `build/<id>.json`, which is regenerated on every build (and is cheap).
   - Changing a deck name means editing `book.toml`, then running `tategaki build && tategaki deploy --settings-only`. Nothing is re-rendered.

**Deploy:**
- **Default:** `tategaki deploy` uses `rsync` over SSH. It sends `.jbk` files and sidecars to `/mnt/us/tategaki/books/`, and writes `/mnt/us/tategaki/config.json` from `[device]`.
- **USB fallback:** `--usb` copies to a mounted Kindle volume instead.
- **Book ids** (the folder names) should be ASCII slugs, to keep device filenames simple. Japanese belongs in `title` and in the Anki values.

**On the device:**
- `config.json` and the sidecars are re-read every time the library screen opens, so a deploy takes effect without restarting the app.
- If a sidecar is missing, use the global defaults.
---

## 6. Bundle format `.jbk` (LOCKED v1)

A zip archive. Store entries uncompressed (STORE), because the PNGs are already compressed.

```
manifest.json
text/0000.txt          chapter (spine item) text, UTF-8, '\n' between paragraphs
layout/0000.json       per-chapter layout
pages/00000.png        global page index, grayscale, full screen at profile resolution
images/<hash>.png      screen-fitted copies of inline images (for full-screen view)
cover.png              library thumbnail
```

`manifest.json`:

```json
{
  "format": 1,
  "title": "…", "author": "…", "source_sha256": "…",
  "profile": {"name": "pw11", "width": 1236, "height": 1648, "dpi": 300},
  "render": {"font": "…", "font_size_pt": 9, "furigana": "rare", "rare_threshold": 10000},
  "page_count": 312,
  "chapters": [{"id": 0, "text": "text/0000.txt", "layout": "layout/0000.json",
                "first_page": 0, "page_count": 14}],
  "toc": [{"title": "第一章", "page": 3, "level": 1}]
}
```

`layout/NNNN.json`:

```json
{
  "pages": [
    {"page": 0,
     "glyphs": [[0, 1100, 120, 40, 40], [1, 1100, 160, 40, 40]],
     "images": [{"box": [x, y, w, h], "full": "images/ab12cd.png"}]}
  ],
  "paragraphs": [[0, 85], [86, 240]],
  "sentences": [[0, 31], [31, 85]]
}
```

- Glyph tuples are `[codepoint_offset, x, y, w, h]` in page-local device pixels.
- All spans are half-open intervals in code points.
- Bump `format` for any breaking change.

### 6.1 Storage on the device

| Path | Contents |
|---|---|
| `/mnt/us/tategaki/books/<id>.jbk` | Books (deployed, §5.8) |
| `/mnt/us/tategaki/books/<id>.json` | Per-book behaviour settings sidecar (deployed) |
| `/mnt/us/tategaki/config.json` | Global device settings (deployed from `tategaki.toml` `[device]`) |
| `/mnt/us/tategaki/state.json` | **Device-owned:** reading position per book id, stored as a chapter code point offset |
| `/mnt/us/tategaki/anki-queue.jsonl` | Anki queue |
| `/mnt/us/tategaki/log.txt` | Log |

**User data must live outside the KPM package folder.** KPM deletes package files on upgrade.

Static data ships inside the package: the binary, `jmdict.sqlite`, the UI font and the FBInk binaries.

### 6.2 Simulator

The device-specific parts sit behind two interfaces:
- `Display`: full and partial refresh of a region.
- `Input`: tap, swipe and hold events in screen coordinates.

The `sim` backend is a small local HTTP server that:
- serves the current framebuffer as a PNG on a page that auto-refreshes,
- forwards clicks and drags as input events.

Run it with `tategaki --sim --root ./simroot`. The whole reader (library, pages, lookup, popup, Anki queue) must be usable in the simulator before any device testing.

---

## 7. Lookup design (LOCKED)

Lookup follows Hoshi Reader and Yomitan: **a tap starts a scan that yields a ranked list of candidates.** It is not a single answer.

### 7.1 Tap to offset

1. Hit-test the tap against the current page's glyph boxes. Allow a few pixels of slop, and pick the nearest box.
2. If the tapped character is punctuation, whitespace or an image, there is no lookup. The tap falls through to the zone actions (§8.2).

### 7.2 Scan

```
window = chapter_text[offset : min(offset + SCAN_LEN, paragraph_end)]   # SCAN_LEN = 16
for n in len(window) … 1:
    prefix = window[:n]
    hits  = exact(prefix) ∪ deinflected(prefix)     # Yomichan deinflection, validated against JMdict POS
    record each hit with source_len = n
```

- **Paragraph-bounded:** the scan crosses column and page boundaries freely, because it works on chapter text, not page text.
- **Dedupe** by JMdict entry id, keeping the longest `source_len`.
- **Batch queries:** query all prefix and deinflection forms in one `IN (…)` query per scan.
- **Partial inflections:** include `complete_partial_inflection` for the shortest prefixes, so a tap that ends in the middle of an inflection still resolves.
- **No match:** show a single "raw" row containing the tapped character, with a mine-raw option, matching the Calibre fallback.

### 7.3 Ranking

- The default comparator is Yomitan-style: `(source_len desc, deinflection chain length asc, JPDB rank asc)`. Ties keep the existing rule of ranking by written form, using the reading only for kana-only entries.
- The comparator is pluggable (`ranking_policy` in config), so the user can try a frequency-first variant.
- **Open question for the user:** which policy should be the default after real-world testing? In the 蜘蛛の巣 example, the user expects 蜘蛛 to be the highlighted word, with 蜘蛛の巣 listed below it.

### 7.4 Highlight

- The span of the **currently selected candidate** (`offset … offset + source_len`) gets a side-line: a vertical bar to the right of each glyph box. This is the vertical-text equivalent of an underline.
- Use a partial refresh of that region only.
- Choosing a different candidate in the popup moves the highlight to that candidate's span.

### 7.5 Popup

- **Placement:** a sheet over the half of the screen away from the tapped column.
- **Each row shows:**
  - word【reading】
  - the JPDB rank badge `#1,234`. Common words (rank ≤ 10,000) get an inverted (white on black) badge, since there's no colour.
  - the deinflection or completion note (`← 擦ら`)
  - part of speech and numbered senses. A tap expands a collapsed row.
- **Per-row "+ Anki" button.** Its state shows queued, sent, duplicate or error.
- **Span adjust ◀ ▶:** shrink or extend the span end and re-run an exact-span lookup.
  - This **replaces** the Calibre editable word field, because an on-device Japanese IME is out of scope.
  - It covers the same purpose: words split by punctuation, and names.
- **Tapping another glyph** while the popup is open restarts the scan there.
- Tapping outside the popup closes it.
- **Popup text** is rasterised in Go:
  - with `golang.org/x/image/font/opentype` and a bundled Japanese UI font (for example Noto Sans JP, subset if size matters),
  - laid out horizontally,
  - scrolled by swipe, with paged scrolling suited to e-ink.

### 7.6 Mining

1. Find the sentence span containing the tap offset.
2. If `strip_speaker_tags` is set in the book's sidecar, apply speaker-tag stripping: 1–4 kanji followed by U+3000, only at the start of a paragraph, and only when the selected word lies past the tag. Port this from the Python twin together with its test cases.
3. Build the Kaishi fields with the port of `build_fields`.
4. Choose the deck and tags from the book's sidecar, falling back to `config.json` (`default_deck`, which defaults to Anki's own "Default" deck).
5. Append to the queue (§7.7).

### 7.7 Anki queue (LOCKED: queue first)

- **Queue file:** each line in `anki-queue.jsonl` is
  `{id, created_at, book, deck, model, fields, tags, status: pending|sent|duplicate|error, error?}`.
- **After each add**, try a flush with a 2-second timeout if Wi-Fi is up.
- **Manual flush** is also available from the menu.
- **Flushing** sends each item to AnkiConnect at `config.ankiconnect_url`, with optional `apiKey`. Port the existing client's behaviour, including the collection-wide duplicate scope.
- **Desktop prerequisite:** AnkiConnect's `webBindAddress` must be `0.0.0.0`. Document this, and recommend setting an API key.
- **USB fallback:** `tools/flush_queue.py` reads the queue from the mounted Kindle, sends it to local AnkiConnect, and rewrites the statuses.
- **Rewrites are atomic:** write a temp file, then rename.

---

## 8. Reader UX

### 8.1 Screens

- **Library:** a list of `.jbk` files with title, author and progress percentage. Tap to open.
- **Page view:** the full-screen page PNG with a minimal overlay (page number) shown only on request.
- **Menu**, opened from the top strip:
  - table of contents
  - go to page
  - flush Anki queue (shows pending count)
  - book info: the resolved deck, tags and render settings, read-only, for checking what was deployed
  - back to library
  - exit

### 8.2 Gestures

Japanese reading order means the **next page is to the left**.

| Gesture | Action |
|---|---|
| Tap on a lookupable glyph | Lookup |
| Tap in the left edge zone (default 12% width), or on a non-glyph area in the left half | Next page |
| The same on the right | Previous page |
| Swipe right | Next page (pulls the page in from the left) |
| Swipe left | Previous page |
| Tap in the top strip | Menu |

Zone sizes and directions are configurable.

### 8.3 Refresh

- Use a partial (fast) refresh on page turns, with a full flashing refresh every N turns (`full_refresh_every`, default 6).
- Popup open and close and highlights use region refreshes.

### 8.4 Settings

The device has no settings UI. All settings come from the files deployed in §5.8.
- `config.json` mirrors `tategaki.toml` `[device]`.
- Each book's sidecar mirrors its resolved `[anki]` values.

`state.json` is the only settings-like file the device writes. It holds the reading position per book, as `{chapter, offset}`. Save it atomically on every page turn.

---

## 9. Phases

### Phase 0 — Device bring-up spike (agent runs on device via SSH)

First step: establish SSH access, write `docs/DEVICE.md` (§0.1), and confirm the device facts in §4 (`uname -a`, `/proc/cpuinfo`, the firmware version from `/etc/prettyversion.txt` or equivalent, the framebuffer geometry). Record them in `docs/DEVICE.md`.

Goal: a "hello" Go app, installed via KPM and launched from a library scriptlet, that:
- pauses the Kindle UI,
- shows a test PNG,
- logs touch events with screen coordinates,
- survives sleep and wake,
- exits cleanly back to the Kindle UI.

**VERIFY items:**
- **KPM package format.** Read `https://github.com/KindleModding/example_kpm_package` and `kpm-helper.py` for `manifest.json` fields and packing.
  - `install.sh` creates `/mnt/us/tategaki/` and writes a scriptlet to `/mnt/us/documents/` that runs `/var/local/kmc/bin/kpm launch <pkgname>`. Read the kindlemodding.org scriptlets page for the scriptlet format.
  - `uninstall.sh` removes the scriptlet, never deletes user data, and receives `upgrade` as an argument during upgrades.
  - **No rootfs writes.**
- **Framework pause and resume.** Base this on KOReader's Kindle launcher (`platform/kindle/koreader.sh` in the KOReader repo): how it stops or pauses the Kindle UI, prevents the screensaver (`lipc-set-prop com.lab126.powerd preventScreenSaver 1`), and restores everything on exit, including after a crash. Use a `trap` in `launch.sh`.
- **Display.** Bundle FBInk CLI binaries per platform; `launch.sh` picks the right one.
  - Measure the latency of a full-screen PNG draw and of a region draw.
  - If the CLI is too slow (over about 250 ms per page turn), switch to FBInk through cgo with one build per platform, and record the decision.
- **Input.** Read evdev from the touchscreen device, which must be discovered at runtime. Handle multitouch protocol B. Work out the rotation and scaling from raw touch coordinates to screen coordinates for this model; KOReader's device definitions are the reference.
- **Power.** Handle the power button and sleep/wake events via `lipc-wait-event` on `com.lab126.powerd`, and redraw on wake.
- **Static binary.** Confirm a `GOARM=7` static binary runs on the device.

**Acceptance:**
- Draw latency is measured on the device and logged.
- Tap coordinates are correct at all four corners. The user taps; the agent reads the log.
- Exit and wake behave cleanly, confirmed by screenshot.

### Phase 1 — Converter MVP plus preview

- **Library config (§5.8):** TOML loading with precedence and schema validation, `tategaki build` (skipping books whose `build_hash` is unchanged, always writing the sidecar), and `tategaki deploy` (SSH and `--usb`). Include tests for the precedence rules and for Japanese values surviving the round trip into the sidecar.
- `tategaki convert` produces a valid `.jbk` from one real vertical-text EPUB (the user will supply test books) and one horizontal-text EPUB that has been forced vertical.
- The preview tool shows correct glyph overlays.
- Unit tests: code point offsets (including a surrogate-pair kanji), ruby exclusion, paragraph boundaries, and sentence spans that match the Python splitter.

- **Proof sheets:** `tategaki proof BOOK.epub --pages 4 --font-size-pt 8,9,10 [--font A,B] [--line-height 1.6,1.8]` renders the first N pages for every combination into one `.jbk`. Each page is stamped with its settings in a corner. The agent deploys it over SSH and opens it on the device, so the user can judge type size in hand. Font size can't be judged on the Mac screen.

**Acceptance:**
- The user reviews about 30 pages in the preview with no clipped columns or misaligned boxes.
- The user picks default typography from a proof sheet on the device.

### Phase 2 — Lookup engine in Go

- `internal/dict`: deinflect, JMdict accessor using `modernc.org/sqlite` (pure Go, no cgo), scan, rank.
- `cmd/lookup`: `lookup --scan "蜘蛛の巣が張って"` prints candidates with `source_len`, rank and reasons.
- Golden parity tests against the Python fixtures (§3).

**Acceptance:** all fixtures pass, and a scan completes in under 100 ms on the desktop. Device timing is checked in Phase 4.

### Phase 3 — Reader core (simulator first, then device)

- Library, page view, page turns, refresh policy, progress persistence, menu and table of contents.

**Acceptance:** a full book is readable in the simulator and on the device, and reopening a book resumes on the correct page.

### Phase 4 — Tap lookup UX

- Hit testing, scan, popup, highlight, candidate selection, span adjust, and the raw fallback.

**Acceptance:**
- In the simulator, tapping 蜘 in 蜘蛛の巣 lists both entries with rank badges.
- On the device, the time from tap to popup is under 700 ms.

### Phase 5 — Anki

- Port of the Kaishi fields with golden tests, the queue, AnkiConnect flush, duplicate handling, per-row status, `flush_queue.py`, and a short AnkiConnect setup guide in `docs/`.

**Acceptance:**
- A word mined on the Kindle appears in desktop Anki with the correct deck, reading, meaning and sentence.
- Mining the same word twice is reported as a duplicate.

### Phase 6 — Polish and stretch goals

- Furigana variant toggle (§5.3).
- **Multiple sizes per bundle:** `--font-size-pt 8,9,10` renders several complete layouts into one `.jbk`, as `pages/<variant>/…` and `layout/<variant>/…`. The manifest lists the variants. Switching size on the device keeps the reading position, because position is stored as a chapter code point offset and mapped to the page containing that offset in the new layout. Design `state.json` so position is stored as an offset from the start (Phase 3), even before this exists.
- Full-screen view for inline images: tap the image, then tap again to close. The images are already in the bundle from Phase 1.
- Converter batch mode.
- The `kindlehf` package published to a static KPM repository on GitHub Pages (`kindlemodding.org/kindle-dev/kpm/creating-a-repository.html`).

---

## 10. Out of scope (for now)

- Reflowing or re-rendering on the device. Font size is chosen at conversion time, with optional pre-rendered size variants (Phase 6).
- On-device Japanese text input.
- Audio, pitch-accent and Yomitan-format dictionaries. **hoshidicts** (C++, used by Hoshi Reader) is the candidate if these are wanted later; it would require cgo.
- DRM'd books, and formats other than EPUB.
- Sync with other readers.

---

## 11. Open questions for the user

1. Which SSH route is installed on the Kindle (USBNetwork or KOReader's SSH server)?
2. The reading font for the converter, typically a Mincho family.
3. The default ranking policy (§7.3). Decide after trying both in Phase 4.
4. Preferred page-turn gestures, if different from §8.2.
5. The project name.
