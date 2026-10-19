# Tategaki Reader

A Japanese ebook reader for a jailbroken Kindle Paperwhite. It offers:

- vertical (縦書き, right-to-left) text with furigana;
- tap-to-look-up against an offline JMdict, with JPDB frequency ranks;
- one-tap mining to Anki (Kaishi 1.5k note type), with an offline queue,
  and an offline "Exists" marker for words already in Anki;
- a menu toggle for the Kindle's own dark mode, and one for the frontlight;
- the book's cover, or お休みなさい, on screen while the Kindle sleeps.

It's a personal project, built for one device and one reader. "Tategaki
Reader" is still a placeholder name. If you want to run it yourself, possibly
on a different Kindle, see [Starting your own fork](#starting-your-own-fork).

## How it works

**Layout happens on the desktop. The Kindle only shows pre-rendered pages and
does lookups.**

```
 Mac                                            Kindle (Go binary, KPM package)
 converter (Python + headless Chromium)          reader
   EPUB → vertical-rl strip → page PNGs  ──.jbk─▶  shows page PNGs (FBInk CLI)
   + glyph boxes, chapter text, TOC                tap → glyph → JMdict scan → popup
 tategaki build / deploy (scp over SSH)            + Anki → queue → AnkiConnect on the Mac
```

- **No Kindle-side layout engine handles vertical Japanese with ruby**, so the
  converter lays each chapter out as one long vertical strip in Chromium. It
  cuts pages only in the gaps between columns, and records every character's
  box and code point offset.
- **The `.jbk` bundle** is a zip: page PNGs, per-chapter text and layout JSON,
  a cover thumbnail, an optional full-screen cover for sleep (`sleep.png`),
  and a manifest.
- **The reader** hit-tests taps against the glyph boxes and scans the chapter
  text with a Go port of the Calibre fork's Yomichan-style deinflector and
  ranking.
- **The cost:** font size and furigana mode are fixed at conversion time.
  Changing them means rebuilding the book (about 30 s for a novel).

## Starting your own fork

This repository was built with an AI coding agent, and its docs are written
for one: `README.md`, the plan, `docs/DECISIONS.md` and `docs/DEVICE.md`
together hold everything an agent needs to work on it. In all of them, **"the
user" means the original author**, and the device facts are for one specific
Kindle. When you fork, tell your agent so, and have it record your own device
and decisions in the same files.

### What you need

- **A jailbroken Kindle with KPM** (the Kindle Package Manager). This was
  built on a Paperwhite 11th gen with firmware 5.19.2, jailbroken with
  [Véra](https://kindlemodding.org/jailbreaking/Vera/), which installs KPM.
  Whether your model and firmware can be jailbroken is the first thing to
  check; [kindlemodding.org](https://kindlemodding.org/) is the place to start.
- **SSH to the Kindle.** We use the `dropbear-ssh` KPM package; setup details
  and pitfalls are in `docs/DEVICE.md`, "SSH access". Put a `kindle` alias in
  `~/.ssh/config`, or set `[device] ssh_host`.
- **A desktop with `go`, `uv` and `python3`.** Development has only been done
  on macOS. Nothing is deliberately Mac-only, but Linux is untested.
- **A Japanese font** for the pages, in `~/Tategaki/fonts/`. The default is
  `NotoSerifJP-Regular.otf` ([Noto Serif JP](https://fonts.google.com/noto/specimen/Noto+Serif+JP)).
- **Optional:** Anki with AnkiConnect and the Kaishi 1.5k deck, for mining
  (`docs/ANKI.md`).

### First run

1. **Build the dictionary.** The Setup section below copies it from the
   author's Calibre fork, which you won't have. Instead, build it from source
   with the vendored script (JMdict is CC BY-SA 4.0; the frequency list is
   JPDB's, via Yomitan):
   ```sh
   S=converter/src/tategaki/reference/build_jmdict_sqlite.py
   python3 $S --download --output data/jmdict.sqlite
   python3 $S --download-frequency --output data/jmdict.sqlite
   python3 tools/prepare_jmdict.py data/jmdict.sqlite data/jmdict.sqlite   # adds indexes
   ```
   A newer JMdict than the author's can make the golden lookup tests fail.
   If so, regenerate the fixtures (see [Tests](#tests)).
2. **Install the converter:**
   `(cd converter && uv sync && uv run playwright install chromium)`.
3. **Try it without a Kindle.** Put an EPUB in `~/Tategaki/books/<id>/`, run
   `bin/tategaki build`, then `tools/sim.sh` and open
   http://localhost:8800. The simulator runs the real reader UI, so you can
   check rendering, lookups and the menus first.
4. **Install on the Kindle:** `tools/dev-install.sh`. It builds the KPM
   package, copies it over SSH and installs it.
5. **Send books, settings and the dictionary:** `bin/tategaki deploy`.
6. **Launch** "Tategaki Reader" from the Kindle's Library.

If your Kindle isn't a Paperwhite 11th gen, do the porting work below
before step 4.

### Porting to another Kindle

The model-specific parts are few, and each is listed here. Work through them in
this order, recording what you find in `docs/DEVICE.md`.

1. **Learn the device.** Over SSH, find:
   - **The screen:** the size in pixels, the stride, and the rotation. Run
     `fbink -e` (from your jailbreak or KOReader; see `docs/DEVICE.md` for which
     builds work).
   - **The touchscreen:** its device name and axis ranges. Cross-compile and
     run the diagnostic:
     `(cd reader && GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o ../build/evinfo ./cmd/evinfo)`,
     copy it to the Kindle, and run `evinfo`, then `evinfo -watch 10` while
     tapping the corners.
   - **The platform:** hard-float (`/lib/ld-linux-armhf.so.3`, KPM platform
     `kindlehf`) or soft-float (`kindlepw2`). Recent firmware is hard-float.
   - **The frontlight:** `lipc-get-prop com.lab126.powerd flMaxIntensity`.
2. **Change the code** where those facts live:

   | What | Where |
   |---|---|
   | Screen size | `screenW` / `screenH` in `reader/cmd/tategaki/session.go` |
   | Touchscreen name (`pt_mt`) | `deviceEvents` in `reader/cmd/tategaki/session.go` |
   | Touch rotation or mirroring | the `input.Mapping` built there has `SwapXY`, `MirrorX` and `MirrorY`; nothing sets them yet |
   | Page size, dpi and margins | a new profile in `converter/src/tategaki/profiles/` (copy `pw11.json`), added to the file list in `converter/src/tategaki/library.py` that triggers rebuilds; then `[device] profile = "<name>"` |
   | Frontlight maximum (24) | `MaxFrontlight` in `reader/internal/kindle/power.go`, and `RANGES` in `converter/src/tategaki/config.py` |
   | FBInk binary | `third_party/fbink/<platform>/fbink`. It must be built with image support (`Image=Yes`); see `third_party/fbink/README.md` |
   | KPM platform | `supported_platforms` in `packaging/kpm/manifest.json`. The Go build in `packaging/kpm/build.py` targets ARMv7, which covers every recent Kindle |

   The screen size is the one hard-coded value that matters most. A cleaner
   fork would read it from FBInk at start-up instead.
3. **Bring it up.** Run the reader's `--hello` mode
   (`reader/cmd/tategaki/hello.go`). It draws a test pattern, marks and logs
   every tap, and exits when you tap EXIT. Tap all four corners and check that
   the marks land under your finger. Then check the latencies against
   `docs/DEVICE.md`: a full-screen page should draw well under 250 ms.
4. **Check the system integration.** These depend on the Kindle firmware
   more than the model, but verify each on a new device:
   - `packaging/kpm/launch.sh` pauses the Kindle UI (pillow and `awesome`)
     and restores it on exit.
   - Sleep and wake come from `com.lab126.powerd` events
     (`reader/internal/kindle/power.go`).
   - Dark mode uses `com.lab126.winmgr epdcMode`.
   - The frontlight uses `com.lab126.powerd flIntensity`.

   The [guardrails](#guardrails-from-the-plan-locked) below apply to any
   device: never write to the root filesystem.

**Colour screens (Colorsoft):** the pipeline is greyscale end to end, from
the 16-level quantised page images to the 8-bit framebuffer writes. Nobody
has tried it on a colour panel. Check what `fbink -e` reports there, and
expect showing colour to be new work.

## Read these first

| File | What it holds |
|---|---|
| `plans/20261017-tategaki-reader.md` | The original plan: architecture, phases, and **LOCKED** decisions. Don't change a LOCKED decision without asking the user. |
| `docs/DECISIONS.md` | Everything decided or changed since, phase by phase, with reasons. **Read it before changing behaviour**, and append to it when you decide something. |
| `docs/DEVICE.md` | Verified device facts: model, framebuffer quirks, touch mapping, LIPC events, SSH setup, latencies. |
| `docs/ANKI.md` | AnkiConnect setup, the mined fields, the queue, and the USB fallback. |

All phases of the plan (0–5) are complete and accepted by the user. Phase 6
("polish") is open-ended. Done so far: dark mode, the Anki word list, the sleep cover, and the
frontlight toggle.

## Repository layout

```
converter/          Python package (uv): EPUB → .jbk, plus the `tategaki` CLI
  src/tategaki/       render.py (Chromium driver), layout.js (in-page extraction),
                      paginate.py (strip slicing), epub.py, images.py, bundle.py,
                      config.py (TOML), library.py (build/deploy), proof.py, preview.py
  src/tategaki/reference/   UNMODIFIED copies of the Calibre fork's Python modules
                            (the reference implementation; see its README)
reader/             Go module: the on-device app
  cmd/tategaki        main: device or --sim
  cmd/lookup          desktop CLI for the lookup engine
  cmd/evinfo          input-device diagnostic
  internal/dict       deinflection, JMdict (pure-Go SQLite), selection lookup, tap scan
  internal/ui         screens: library, page view, lookup popup, menu, TOC, go-to, info
  internal/anki       Kaishi fields, AnkiConnect client, JSONL queue
  internal/sentence   Calibre's sentence extraction + speaker tags
  internal/book       .jbk reading, reading position ↔ page mapping
  internal/display    FBInk and simulator backends
  internal/input      evdev multitouch, gesture classification
  internal/kindle     powerd events, idle timer, dark mode (LIPC)
  internal/config     config.json, per-book sidecars, state.json
  internal/sim        simulator HTTP server
packaging/kpm/      KPM package: manifest, install/launch/uninstall hooks, build.py
testdata/           golden fixtures from the Python reference (Go must match exactly)
tools/              dev scripts (see below)
data/               UI fonts (committed); jmdict.sqlite (NOT committed; see data/README.md)
third_party/fbink/  vendored FBInk 1.25.0 kindlehf binary (provenance in its README)
bin/tategaki        wrapper for the converter CLI
```

## Setup (desktop)

You'll need `go` and `uv`, with Homebrew on macOS. The user's Calibre fork must
be checked out at `../calibre`. (Without it, build the dictionary from source:
see [First run](#first-run).)

```sh
python3 tools/prepare_jmdict.py            # copies ../calibre's jmdict.sqlite into data/, adds indexes
(cd converter && uv sync && uv run playwright install chromium)
```

The user's library lives in `~/Tategaki/` (override with `TATEGAKI_HOME`):

- `tategaki.toml`: global settings, mostly `[device]` and `[render]`.
- `fonts/`: reading fonts.
- `books/<id>/`: one folder per book, holding the `.epub`, an optional
  `book.toml` with per-book `[render]`, `[anki]` and `[reader]` overrides, and
  `build/` output.

Every setting is described in the [user's manual](#users-manual) below.

## Everyday commands

```sh
bin/tategaki build [ID…]          # convert books whose inputs changed (hash-checked)
bin/tategaki deploy [ID…]         # send books, settings and the dictionary to the Kindle
bin/tategaki deploy --settings-only   # also refreshes the Anki word list (Anki must be open)
bin/tategaki preview ID           # HTML preview with glyph overlays (click a glyph)
bin/tategaki proof EPUB --font-size-pt 12,14,16 --deploy   # typography proof sheets
tools/dev-install.sh              # build + (re)install the reader package on the Kindle
tools/sim.sh                      # run the reader in a desktop simulator: http://localhost:8800
build/lookup --scan "蜘蛛の巣が張って"   # after: (cd reader && go build -o ../build/lookup ./cmd/lookup)
```

## User's manual

### Adding a book

1. Make a folder `~/Tategaki/books/<id>/`. The id is an ASCII slug (letters,
   digits, `-`, `_`, `.`); it names the book on the Kindle and in
   `state.json`.
2. Put the `.epub` in it. Add a `book.toml` only if you want to change
   something (see below).
3. Run `bin/tategaki build <id> && bin/tategaki deploy <id>`.

To replace the EPUB with a revised one, keep the same folder and id. The
reading position survives, though it may drift a little if the text changed.

### On the Kindle

**Library.** Opens when the reader starts with no book to resume, or from
**Menu → Library**.

- Books are listed most recently read first, then unread books by title. Each
  row shows the cover, title, author, and progress (or "Not started").
- Tap a row to open the book at its saved position.
- With more books than fit, page with **◀ Next** / **Prev ▶** or by swiping.
  As in a Japanese book, *next* is on the left.
- **Exit** returns to the Kindle's home screen.
- Opening the library re-reads `config.json`, per-book settings and the Anki
  word list, so a deploy takes effect without restarting the reader.

**Reading.** The reader starts on the last book you read, at your place.

| Gesture | Effect |
|---|---|
| Tap a character (or hold it) | Look it up (see below) |
| Tap blank space, left half | Next page |
| Tap blank space, right half | Previous page |
| Swipe left → right | Next page (pulls it in from the left) |
| Swipe right → left | Previous page |
| Tap the top margin (top 10% of the screen, off any character) | Open the menu |

- Punctuation and spaces aren't looked up, so a tap on them turns the page.
- The position is saved on every page turn. It's stored as a place in the
  text, not a page number, so it survives re-rendering the book.
- Page turns use fast partial refreshes, with a full flashing refresh every
  `full_refresh_every` turns (6 by default) to clear ghosting.

**Lookup popup.** Covers three quarters of the screen, on the side away from
the tapped character. A black line beside the text marks the matched span.

- Each row shows the headword, reading, the deinflection chain (`← past ←
  negative`), part of speech, and the first two gloss lines.
- The badge is the JPDB frequency rank. It's solid black for common words
  (rank 10,000 or better) and outlined for rarer ones.
- **Tap a row** to select it (the highlight follows its length). Tap the
  selected row again to show or hide all its senses.
- **◀ ▶** shorten or lengthen the span by one character and look up exactly
  that text. Use them when the automatic match is too short or too long.
  When nothing matches, the popup shows a single raw row.
- **Swipe up / down** to page through more entries (a "1 / 2" indicator
  shows when there are more).
- **+ Anki** mines the entry; see [Anki](#anki) below.
- **×** or a tap on the page outside the popup closes it. A tap on another
  character looks that one up instead.

**Menu.** A sheet over the top of the page, showing the title, chapter, page
and percentage. A tap below it closes it.

| Button | Effect |
|---|---|
| Contents | The book's table of contents, with the current chapter marked. Tap an entry to jump there. |
| Go to page | A keypad: type a page number and tap **Go**. |
| Book info | What was deployed for this book: id, page and chapter counts, the Anki deck, tags and note type, and the render settings. Tap anywhere to go back. |
| Library | Close the book and show the library. |
| Send to Anki (n) | Retry the n cards waiting in the queue. The button then reads "x sent, y skipped". |
| Dark mode: On/Off | Toggle the Kindle's own (hardware) dark mode. |
| Light: On/Off | Toggle the frontlight between off and `light_level`. In dark mode it's greyed out and reads "Light: 11" (`dark_light_level`), since dark mode fixes the light. Leaving dark mode restores the light as it was. |
| Close menu | Back to the page. |
| Exit | Quit to the Kindle's home screen. |

**Sleep.** When the Kindle goes to sleep, the reader draws the current book's
cover full screen (in dark mode too: it's pre-inverted so it doesn't show as a
negative). With `sleep_screen = "goodnight"`, or for a book without a cover,
it draws お休みなさい instead. On wake, the page comes back.

### Anki

Full setup and troubleshooting are in [`docs/ANKI.md`](docs/ANKI.md). In
short:

- **One-time setup:** install AnkiConnect on the computer, set its
  `webBindAddress` to `0.0.0.0` and an `apiKey`, and put the URL and key in
  `tategaki.toml` (`ankiconnect_url`, `anki_api_key`).
- **Mining:** tap **+ Anki** on a popup row. The card is a Kaishi 1.5k note
  with the headword, reading, every gloss, and the sentence around the tapped
  word. It goes to the book's `[anki] deck` (or `default_deck`) with the
  book's `tags`.
- **Offline is fine.** Every card is saved to a queue on the Kindle first. The
  reader tries to send it at once, retries everything at each start, and
  **Menu → Send to Anki** retries on demand. Without Wi-Fi, the USB fallback
  (`tools/flush_queue.py`) sends the queue from the computer.
- **The button shows the card's state:** **Queued** (waiting for Anki),
  **Added ✓**, **Exists** (already in Anki, so nothing was added), or
  **Error** (tap to retry; the reason is in the log). Only **+ Anki** and
  **Error** respond to taps.
- **Words you already have** show a grey **Exists** before you tap anything,
  even offline. The list comes from Anki at each deploy: run
  `bin/tategaki deploy --settings-only` with Anki open to refresh it. A
  duplicate is a Kaishi note with the same `Word` in any deck.

### Settings files

Settings are TOML. **Precedence:** built-in defaults, then
`~/Tategaki/tategaki.toml`, then `books/<id>/book.toml`. Unknown sections or
keys are errors, since they're usually typos.

**When changes take effect:**

- **`[render]`** settings are baked into the page images, so changing them
  needs `bin/tategaki build` (about 30 s per novel) and then `deploy`. Build
  only re-renders books whose EPUB, render settings, font or converter code
  changed.
- **Everything else** (`[device]`, `[anki]`, `[reader]`) needs only
  `bin/tategaki deploy --settings-only`. The Kindle picks it up the next time
  the library opens.

#### `tategaki.toml` only: `[device]`

| Key | Default | Meaning |
|---|---|---|
| `ssh_host` | `"kindle"` | SSH host that `deploy` copies to. |
| `profile` | `"pw11"` | Device profile (screen size and resolution) the pages are rendered for. |
| `ankiconnect_url` | `"http://127.0.0.1:8765"` | AnkiConnect as seen from the Kindle: use the computer's LAN IP. |
| `anki_api_key` | `""` | Must match AnkiConnect's `apiKey`. |
| `default_deck` | `"Default"` | Deck for books that don't set `[anki] deck`. |
| `note_type` | `"Kaishi 1.5k"` | Anki note type for mined cards. |
| `ranking_policy` | `"longest"` | Lookup ordering. `"longest"`: longest match, then shortest deinflection, then most frequent. `"frequency"`: most frequent first. |
| `scan_len` | `16` | How many characters after the tap a lookup considers. |
| `full_refresh_every` | `6` | Page turns between full (flashing) refreshes. 1 flashes on every turn. |
| `light_level` | `9` | Frontlight step for **Light: On**, 0–24 (the Kindle's slider). |
| `dark_light_level` | `11` | Frontlight step throughout dark mode, 0–24. |
| `edge_zone_pct` | `12` | Accepted but has no effect: page-turn zones are the screen halves. |

#### Both files, or per book: `[render]`

| Key | Default | Meaning |
|---|---|---|
| `font` | `"fonts/NotoSerifJP-Regular.otf"` | Reading font, relative to `~/Tategaki/` (or absolute). |
| `font_size_pt` | `9.0` | Text size in points on the Kindle's screen. Try `bin/tategaki proof` to compare sizes. |
| `line_height` | `1.75` | Spacing between columns, as a multiple of the font size. |
| `furigana` | `"rare"` | `"all"`: keep the book's furigana. `"none"`: hide it all. `"rare"`: hide it on common words, as the Calibre viewer does. Katakana and emphasis ruby are never hidden. |
| `rare_threshold` | `10000` | In `"rare"` mode, words with a frequency rank this good or better lose their furigana. Lower it to keep more furigana. |
| `spreads` | `"split"` | Landscape illustrations (two-page spreads): `"split"` into two pages, right half first; `"rotate"` 90° to fill the screen; or `"fit"` as is, letterboxed. |
| `spread_ratio` | `1.2` | An image counts as a spread when its width exceeds its height times this. |
| `gamma` | `1.0` | Gamma curve for illustrations and the sleep cover. Above 1 darkens, below 1 lightens. |
| `quantize` | `true` | Reduce illustrations to the panel's 16 grey levels, with dithering. |

#### Both files, or per book: `[anki]`

| Key | Default | Meaning |
|---|---|---|
| `deck` | `default_deck` | Deck for cards mined from this book. It must already exist in Anki. |
| `tags` | `["tategaki"]` | Tags on cards mined from this book. |
| `strip_speaker_tags` | `false` | Drop 「筆者　」-style speaker tags from the start of mined sentences. |

#### Both files, or per book: `[reader]`

| Key | Default | Meaning |
|---|---|---|
| `sleep_screen` | `"cover"` | What shows while the Kindle sleeps: `"cover"` (the book's cover) or `"goodnight"` (お休みなさい). |

#### `book.toml` only: top-level keys

| Key | Meaning |
|---|---|
| `epub` | Which EPUB to use, when the folder holds more than one. |
| `title` | Override the title from the EPUB. Needs a rebuild. |
| `author` | Override the author from the EPUB. Needs a rebuild. |

A complete example:

```toml
# ~/Tategaki/books/henna-ie/book.toml
title = "変な家"

[render]
font_size_pt = 10
furigana = "all"

[anki]
deck = "日本語::変な家"
tags = ["変な家"]
strip_speaker_tags = true

[reader]
sleep_screen = "goodnight"
```

## Tests

```sh
(cd reader && go test ./...)       # includes golden parity tests against testdata/
(cd converter && uv run pytest)    # includes layout tests in headless Chromium
uv run --project converter python tools/gen_fixtures.py   # regenerate testdata/ from the reference
```

Parity rule: Go ports of the Calibre code must reproduce `testdata/` exactly.
If you change the reference behaviour, regenerate the fixtures. Never edit
fixtures by hand.

## Working on the device

- **Access:** `ssh kindle` (an alias in `~/.ssh/config`: root, port 2022, key
  auth) via the dropbear-ssh KPM package over Wi-Fi.
  - After a Kindle reboot, someone has to start the server on the device:
    Library → **Dropbear SSH** → Start Server.
  - A sleeping Kindle drops Wi-Fi, so SSH times out until it wakes.
- **Launch:** from the Kindle Library ("Tategaki Reader"), or over SSH:
  ```sh
  ssh kindle 'nohup /var/local/kmc/bin/kpm launch tategaki </dev/null >/dev/null 2>&1 &'
  ```
- **Stop:** `ssh kindle killall tategaki`. `launch.sh` restores the Kindle UI on
  any exit, including SIGKILL.
- **Log:** `/mnt/us/tategaki/log.txt` (draw timings, `LOOKUP` and `ANKI`
  lines).
- **Screenshot:** `uv run --project converter --with pillow tools/fbshot.py out.png`.
- **Data on the device** lives in `/mnt/us/tategaki/`: `books/`, `config.json`,
  `state.json`, `anki-queue.jsonl`, `anki-words.txt`, `jmdict.sqlite`. The KPM package itself sits
  in `/mnt/us/kmc/kpm/packages/tategaki/`.
- **Taps can't be injected remotely.** Test interaction in the simulator, then
  ask the user to try it on the device and read the log.

### Guardrails (from the plan; LOCKED)

- **Never** write to the root filesystem, run `mntroot rw`, `dd`, `mkfs` or
  `fw_setenv`, or put `update*.bin` files on the device.
- Only delete inside `/mnt/us/tategaki/` and the package's own files.
- Ask before system-setting changes through `lipc-set-prop`, beyond
  screensaver, framework, dark-mode and frontlight controls.
- A factory reset removes the jailbreak. If one ever happens, keep the device
  in airplane mode until it's re-jailbroken.

## Non-obvious things that will bite you

- **The Kindle UI is paused** while the reader runs: pillow is disabled and
  `awesome` gets `SIGSTOP`.
  - LIPC calls to `com.lab126.winmgr` hang in that state, so dark mode briefly
    resumes `awesome`.
  - The paused UI can still draw for a few seconds after launch, which is why
    the reader redraws its screen at 2 s and 5 s.
- **Sleep:** don't set `preventScreenSaver`, because it disables the power
  button. The reader resets powerd's idle timer on input instead.
- **KPM** won't reinstall the same version (so `dev-install.sh` uninstalls
  first), and it can wait at a hidden `[Y/n]` prompt (so always pass `-y`
  and `</dev/null`).
- **Pagination uses em-box bands, not client rects,** and ruby bands are
  extended into their base column. Otherwise columns merge into one uncuttable
  band, or furigana lands on the wrong page. See DECISIONS, Phase 1.
- **Positions:** reading position is a chapter code point offset, not a page,
  so it survives re-rendering.
- **The tap scan** has no partial-inflection completion. The span-adjust (◀ ▶)
  path uses `LookupCandidates`, which does complete.
- **Mining** extracts the sentence on the device from the paragraph text,
  using the exact Calibre algorithm. It doesn't use the bundle's stored sentence
  spans.
- **AnkiConnect** on the Mac must bind `0.0.0.0` and uses an API key
  (`anki_api_key` in `tategaki.toml`).

## License

GPL-3.0-or-later; see `LICENSE`. The vendored Calibre modules and their Go
ports are GPL-3.0 like Calibre, and FBInk is GPL-3.0-or-later (see
`third_party/fbink/README.md`). The UI fonts in `data/fonts/` are under the
SIL Open Font License (`data/fonts/OFL.txt`). The dictionary built from
JMdict is CC BY-SA 4.0, from the
[EDRDG](https://www.edrdg.org/jmdict/j_jmdict.html).
