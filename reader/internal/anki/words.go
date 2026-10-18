package anki

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"strings"
)

// LoadWords reads anki-words.txt: the words already in Anki (the note type's
// first field, HTML stripped) when the desktop last deployed, one per line. A
// missing file is an empty set.
func LoadWords(path string) (map[string]bool, error) {
	words := map[string]bool{}
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return words, nil
	}
	if err != nil {
		return words, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if w := strings.TrimSpace(sc.Text()); w != "" {
			words[w] = true
		}
	}
	return words, sc.Err()
}
