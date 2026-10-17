# Anki setup

The reader adds cards from the Kindle over Wi-Fi to Anki on your computer,
through the [AnkiConnect](https://ankiweb.net/shared/info/2055492159) add-on
(code `2055492159`). Cards use the **Kaishi 1.5k** note type. The reader fills
four fields:

| Field | Value |
|---|---|
| `Word` | the dictionary headword (or the raw text, if nothing matched) |
| `Word Reading` | the reading |
| `Word Meaning` | every gloss, joined with "; " |
| `Sentence` | the sentence around the word, extracted as the Calibre viewer does |

These are the same fields and the same duplicate rule as the Calibre fork:
a word is a duplicate if a Kaishi note with that `Word` exists in **any** deck.

## One-time setup on the computer

1. **Install AnkiConnect** (Tools → Add-ons → Get Add-ons → `2055492159`) and
   import the Kaishi 1.5k deck so the note type exists.
2. **Let the Kindle reach it.** In Tools → Add-ons → AnkiConnect → Config, set:
   ```json
   "webBindAddress": "0.0.0.0",
   "apiKey": "<a secret of your choice>",
   ```
   - By default, AnkiConnect only listens to the computer itself
     (`127.0.0.1`). `0.0.0.0` lets other devices on your network connect.
   - The API key stops other devices on the same network from adding cards or
     reading your collection.
   - Restart Anki afterwards. If macOS asks whether Anki may accept incoming
     connections, allow it.
3. **Tell the reader**, in `~/Tategaki/tategaki.toml`:
   ```toml
   [device]
   ankiconnect_url = "http://<computer's LAN IP>:8765"
   anki_api_key = "<the same secret>"
   default_deck = "Default"
   ```
   Give the computer a fixed IP address on your router, if you can.
4. **Choose decks per book** in `books/<id>/book.toml`:
   ```toml
   [anki]
   deck = "日本語::変な家"
   tags = ["変な家"]
   strip_speaker_tags = true   # drop 「筆者　」-style speaker tags from sentences
   ```
   The deck must already exist in Anki.
5. Run `bin/tategaki build && bin/tategaki deploy --settings-only`. Settings
   changes don't re-render books.

## On the Kindle

- **Adding cards:** in the lookup popup, tap **+ Anki** on an entry. The
  button then shows the card's state:
  - **Queued:** saved on the Kindle, waiting for Anki.
  - **Added ✓:** in Anki.
  - **Exists:** already in Anki (a duplicate), so nothing was added.
  - **Error:** Anki refused it, for example a missing deck. Check
    `/mnt/us/tategaki/log.txt`.
- **The queue:** every card is saved to `/mnt/us/tategaki/anki-queue.jsonl`
  before anything is sent, so nothing is lost if Anki is closed or the Kindle is
  offline.
  - The reader tries to send each card right away, waiting up to 2 seconds.
  - **Menu → Send to Anki (n)** retries everything still queued.
  - Waiting cards are also retried once each time the reader starts.
  - The result reads "x sent, y skipped". Skipped cards were already in Anki.

## Without Wi-Fi (USB fallback)

Connect the Kindle by USB, open Anki, and run:

```sh
uv run --project converter python tools/flush_queue.py --key <secret>
```

It reads `/Volumes/Kindle/tategaki/anki-queue.jsonl`, sends the pending cards to
the local AnkiConnect the same way the reader does, and updates their
statuses. Eject the Kindle afterwards.

## Troubleshooting

- **"Anki not reachable":** check the following.
  - Anki is running.
  - `webBindAddress` is `0.0.0.0` and Anki has been restarted since you changed it.
  - The URL holds the computer's current IP address.
  - Both devices are on the same Wi-Fi.
  - From the computer, this should print `{"result": 6, "error": null}`:
    `curl -s http://<LAN IP>:8765 -d '{"action":"version","version":6,"key":"<secret>"}'`
- **Error "valid api key must be provided":** the key in `tategaki.toml`
  doesn't match AnkiConnect's. Redeploy with `bin/tategaki deploy --settings-only`.
- **Error "deck was not found":** create the deck in Anki, or fix `deck` in
  `book.toml`.
