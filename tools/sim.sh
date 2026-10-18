#!/bin/sh
# Run the reader in the desktop simulator against the books built in
# $TATEGAKI_HOME (default ~/Tategaki): open http://localhost:8800/
# Reading state is kept in ./simroot, separate from the Kindle's.
set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
HOME_DIR="${TATEGAKI_HOME:-$HOME/Tategaki}"
SIM="$ROOT/simroot"
mkdir -p "$SIM/books"
for f in "$HOME_DIR"/books/*/build/*.jbk "$HOME_DIR"/books/*/build/*.json; do
    [ -e "$f" ] && ln -sf "$f" "$SIM/books/$(basename "$f")"
done
# the Anki word list written by the last `tategaki deploy`
[ -e "$HOME_DIR/anki-words.txt" ] && ln -sf "$HOME_DIR/anki-words.txt" "$SIM/anki-words.txt"
cd "$ROOT/reader"
exec go run ./cmd/tategaki --sim --root "$SIM" --fonts "$ROOT/data/fonts" "$@"
