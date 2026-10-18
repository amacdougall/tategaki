# Tategaki Reader

A Japanese ebook reader for a jailbroken Kindle Paperwhite. It offers:

- vertical (縦書き, right-to-left) text with furigana;
- tap-to-look-up against an offline JMdict, with JPDB frequency ranks;
- one-tap mining to Anki (Kaishi 1.5k note type), with an offline queue,
  and an offline "Exists" marker for words already in Anki;
- a menu toggle for the Kindle's own dark mode.

It's a personal project, built for one device and one reader. "Tategaki
Reader" is still a placeholder name.

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
  and a manifest.
- **The reader** hit-tests taps against the glyph boxes and scans the chapter
  text with a Go port of the Calibre fork's Yomichan-style deinflector and
  ranking.
- **The cost:** font size and furigana mode are fixed at conversion time.
  Changing them means rebuilding the book (about 30 s for a novel).

## Read these first

| File | What it holds |
|---|---|
| `plans/20261017-tategaki-reader.md` | The original plan: architecture, phases, and **LOCKED** decisions. Don't change a LOCKED decision without asking the user. |
| `docs/DECISIONS.md` | Everything decided or changed since, phase by phase, with reasons. **Read it before changing behaviour**, and append to it when you decide something. |
| `docs/DEVICE.md` | Verified device facts: model, framebuffer quirks, touch mapping, LIPC events, SSH setup, latencies. |
| `docs/ANKI.md` | AnkiConnect setup, the mined fields, the queue, and the USB fallback. |

All phases of the plan (0–5) are complete and accepted by the user. Phase 6
("polish") is open-ended. Done so far: dark mode, and the Anki word list.

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
be checked out at `../calibre`.

```sh
python3 tools/prepare_jmdict.py            # copies ../calibre's jmdict.sqlite into data/, adds indexes
(cd converter && uv sync && uv run playwright install chromium)
```

The user's library lives in `~/Tategaki/` (override with `TATEGAKI_HOME`):

- `tategaki.toml`: global settings, mostly `[device]` and `[render]`.
- `fonts/`: reading fonts.
- `books/<id>/`: one folder per book, holding the `.epub`, an optional
  `book.toml` with per-book `[render]` and `[anki]` overrides, and `build/`
  output.

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
  screensaver, framework and dark-mode controls.
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
