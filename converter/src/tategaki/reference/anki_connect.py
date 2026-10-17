# Minimal AnkiConnect client.
#
# AnkiConnect (https://foosoft.net/projects/anki-connect/) is an Anki add-on
# that exposes an HTTP API on http://127.0.0.1:8765 while Anki is *running*.
# This is the opposite constraint from direct collection.anki2 writes (which
# require Anki to be closed) and is the natural fit for an interactive reader
# that adds a card mid-session: Anki itself handles note IDs, GUIDs, checksums,
# scheduling and sync, so we never touch the SQLite file directly.
#
# Zero dependencies: only the Python standard library, which matters because
# this is intended to be importable from Calibre's bundled interpreter without
# installing pip packages.

import json
import urllib.error
import urllib.request

DEFAULT_URL = 'http://127.0.0.1:8765'
ANKICONNECT_VERSION = 6


class AnkiConnectError(Exception):
    '''An error reported by AnkiConnect itself (non-null "error" field).'''


class AnkiConnectUnavailable(Exception):
    '''AnkiConnect could not be reached (Anki not running / add-on missing).'''


class AnkiConnect:
    '''Thin wrapper over the AnkiConnect JSON-RPC-ish HTTP API.

    Every call POSTs ``{"action", "version", "params"}`` and returns the
    ``result`` field, raising :class:`AnkiConnectError` if the server returns a
    non-null ``error`` or :class:`AnkiConnectUnavailable` if the endpoint can't
    be reached at all.
    '''

    def __init__(self, url=DEFAULT_URL, timeout=5.0):
        self.url = url
        self.timeout = timeout

    def invoke(self, action, **params):
        payload = json.dumps({
            'action': action,
            'version': ANKICONNECT_VERSION,
            'params': params,
        }).encode('utf-8')
        request = urllib.request.Request(
            self.url, data=payload,
            headers={'Content-Type': 'application/json'},
        )
        try:
            with urllib.request.urlopen(request, timeout=self.timeout) as resp:
                body = json.loads(resp.read().decode('utf-8'))
        except urllib.error.URLError as e:
            raise AnkiConnectUnavailable(
                f'Could not reach AnkiConnect at {self.url}: {e}. '
                'Is Anki running with the AnkiConnect add-on installed?'
            ) from e

        # AnkiConnect always returns both keys; "error" is null on success.
        if not isinstance(body, dict) or 'error' not in body or 'result' not in body:
            raise AnkiConnectError(f'Unexpected AnkiConnect response: {body!r}')
        if body['error'] is not None:
            raise AnkiConnectError(body['error'])
        return body['result']

    # --- Connectivity / discovery -----------------------------------------

    def version(self):
        '''Return the AnkiConnect API version, or raise if unreachable.'''
        return self.invoke('version')

    def is_available(self):
        '''Return True if AnkiConnect responds, without raising.'''
        try:
            self.version()
            return True
        except (AnkiConnectUnavailable, AnkiConnectError):
            return False

    def deck_names(self):
        return self.invoke('deckNames')

    def model_names(self):
        return self.invoke('modelNames')

    def model_field_names(self, model_name):
        return self.invoke('modelFieldNames', modelName=model_name)

    # --- Adding notes ------------------------------------------------------

    def can_add_note(self, note):
        '''Return True if the single note can be added (e.g. not a duplicate).'''
        return self.invoke('canAddNotes', notes=[note])[0]

    def add_note(self, note):
        '''Add a single note dict; returns the new note id.

        ``note`` is an AnkiConnect note object, most easily built with
        :func:`build_note`.
        '''
        return self.invoke('addNote', note=note)

    def find_notes(self, query):
        '''Run an Anki search query, returning a list of note ids.'''
        return self.invoke('findNotes', query=query)


def build_note(deck_name, model_name, fields, tags=None,
               allow_duplicate=False, duplicate_scope=None):
    '''Construct an AnkiConnect note object.

    ``fields`` is a ``{field_name: value}`` mapping. ``duplicate_scope`` of
    ``None`` (the default) makes duplicate detection collection-wide: a word is
    reported as already in Anki if a note of this type exists in *any* deck, not
    just the target deck. This suits mining vocab across many source decks that
    share one note type. Pass ``'deck'`` to restrict dedup to the target deck.

    Note: with collection-wide scope AnkiConnect uses Anki's native
    ``dupeOrEmpty``, which only matches within the *same note type* (keyed on the
    first field). That is exactly what we want here, since all the vocab notes
    use one type.
    '''
    return {
        'deckName': deck_name,
        'modelName': model_name,
        'fields': dict(fields),
        'tags': list(tags or []),
        'options': {
            'allowDuplicate': allow_duplicate,
            'duplicateScope': duplicate_scope,
        },
    }
