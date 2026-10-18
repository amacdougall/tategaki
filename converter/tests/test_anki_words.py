from tategaki import library


def note(**fields):
    return {'fields': {name: {'value': v, 'order': i} for i, (name, v) in enumerate(fields.items())}}


def test_first_field_strips_html():
    assert library.first_field(note(Word='<b>贔屓</b>&nbsp;', Sentence='x')) == '贔屓'
    assert library.first_field(note(Word='<ruby>面<rt>おも</rt></ruby><ruby>白<rt>しろ</rt></ruby>おかしい')) == '面白おかしい'
    assert library.first_field({}) == ''  # notesInfo returns {} for a deleted id


def test_anki_words_queries_note_type(monkeypatch):
    calls = []

    def invoke(device, action, **params):
        calls.append((action, params))
        if action == 'findNotes':
            return [1, 2, 3, 4]
        return [note(Word='蜘蛛'), note(Word='贔屓'), note(Word='蜘蛛'), note(Word='<br>')]

    monkeypatch.setattr(library, 'anki_invoke', invoke)
    words = library.anki_words({'note_type': 'Kaishi 1.5k'})
    assert words == ['蜘蛛', '贔屓']
    assert calls[0] == ('findNotes', {'query': '"note:Kaishi 1.5k"'})
