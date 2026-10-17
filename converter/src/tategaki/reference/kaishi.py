# Kaishi 1.5k note-type mapping for the reader -> Anki feature.
#
# Unlike the reference project's direct-SQLite writer (which addresses fields by
# positional index into notes.flds), AnkiConnect addresses fields by *name*, so
# this mapping is just the field-name constants plus a helper that fills the two
# fields the reader can supply: the highlighted word and its surrounding
# sentence. Enrichment (reading, meaning, furigana, audio, pitch) is left to the
# existing anki-example-sentence-builder pipeline, which already backfills cards
# created elsewhere with only Word + Sentence populated.

MODEL_NAME = 'Kaishi 1.5k'

# Field names as they appear in the Kaishi 1.5k note type.
WORD = 'Word'
WORD_READING = 'Word Reading'
WORD_MEANING = 'Word Meaning'
WORD_FURIGANA = 'Word Furigana'
WORD_AUDIO = 'Word Audio'
SENTENCE = 'Sentence'
SENTENCE_MEANING = 'Sentence Meaning'
SENTENCE_FURIGANA = 'Sentence Furigana'
SENTENCE_AUDIO = 'Sentence Audio'
NOTES = 'Notes'
PITCH_ACCENT = 'Pitch Accent'


def build_fields(word, sentence='', reading='', meaning='', note=''):
    '''Build the ``{field_name: value}`` mapping for a reader-captured card.

    Only the fields the reader actually has are set; everything else is left for
    the post-add enrichment pipeline. Empty values are omitted so AnkiConnect
    leaves those fields blank.
    '''
    fields = {WORD: word}
    if sentence:
        fields[SENTENCE] = sentence
    if reading:
        fields[WORD_READING] = reading
    if meaning:
        fields[WORD_MEANING] = meaning
    if note:
        fields[NOTES] = note
    return fields
