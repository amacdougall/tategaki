#!/usr/bin/env python3
"""Send the Kindle's queued Anki cards from a USB-mounted Kindle (plan §7.7).

The fallback when the Kindle can't reach Anki over Wi-Fi: mount the Kindle,
run this, eject. Pending cards go to the local AnkiConnect exactly as the
reader would send them (canAddNotes first, collection-wide duplicates), and
the queue file is rewritten atomically with the new statuses.

Usage: uv run --project converter python tools/flush_queue.py [QUEUE] [--url URL] [--key KEY]
       (default QUEUE: /Volumes/Kindle/tategaki/anki-queue.jsonl)
"""

import argparse
import json
import os
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'converter' / 'src'))

from tategaki.reference.anki_connect import (  # noqa: E402
    AnkiConnect,
    AnkiConnectError,
    AnkiConnectUnavailable,
    build_note,
)


def write_atomic(path: Path, items: list[dict]) -> None:
    tmp = path.with_name(path.name + '.tmp')
    with open(tmp, 'w', encoding='utf-8') as f:
        for it in items:
            f.write(json.dumps(it, ensure_ascii=False, separators=(',', ':')) + '\n')
        f.flush()
        os.fsync(f.fileno())
    tmp.replace(path)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument('queue', nargs='?', type=Path, default=Path('/Volumes/Kindle/tategaki/anki-queue.jsonl'))
    ap.add_argument('--url', default='http://127.0.0.1:8765')
    ap.add_argument('--key', default='', help='AnkiConnect API key, if one is set')
    args = ap.parse_args()

    if not args.queue.exists():
        print(f'No queue at {args.queue}. Is the Kindle mounted?', file=sys.stderr)
        return 1
    items = [json.loads(line) for line in args.queue.read_text(encoding='utf-8').splitlines() if line.strip()]
    pending = [it for it in items if it.get('status') == 'pending']
    print(f'{len(pending)} pending of {len(items)} queued')

    client = AnkiConnect(args.url)
    if args.key:  # the vendored client has no API key support
        client.invoke = lambda action, **params: _keyed(client, action, args.key, params)

    counts = {'sent': 0, 'duplicate': 0, 'error': 0}
    for it in pending:
        note = build_note(it['deck'], it['model'], it['fields'], it.get('tags') or [])
        try:
            if not client.can_add_note(note):
                it['status'] = 'duplicate'
            else:
                it['note_id'] = client.add_note(note)
                it['status'] = 'sent'
            it.pop('error', None)
        except AnkiConnectUnavailable as e:
            print(f'Anki not reachable: {e}', file=sys.stderr)
            break
        except AnkiConnectError as e:
            it['status'], it['error'] = 'error', str(e)
        counts[it['status']] += 1
        print(f"  {it['status']:9} {it['fields'].get('Word', '')}  → {it['deck']}")
        write_atomic(args.queue, items)
    print(f"done: {counts['sent']} sent, {counts['duplicate']} already in Anki, {counts['error']} failed")
    return 0


def _keyed(client, action, key, params):
    import urllib.request
    payload = json.dumps({'action': action, 'version': 6, 'params': params, 'key': key}).encode('utf-8')
    req = urllib.request.Request(client.url, data=payload, headers={'Content-Type': 'application/json'})
    try:
        with urllib.request.urlopen(req, timeout=client.timeout) as resp:
            body = json.loads(resp.read().decode('utf-8'))
    except OSError as e:
        raise AnkiConnectUnavailable(str(e)) from e
    if body.get('error') is not None:
        raise AnkiConnectError(body['error'])
    return body['result']


if __name__ == '__main__':
    sys.exit(main())
