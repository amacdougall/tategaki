package sentence

import (
	"encoding/json"
	"os"
	"testing"
)

// Golden cases from tools/gen_fixtures.py (calibre's reference, both with
// and without speaker-tag stripping).
func TestExtractGolden(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/sentences.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Text     string `json:"text"`
		Offset   int    `json:"offset"`
		Strip    bool   `json:"strip_speaker_tag"`
		Sentence string `json:"sentence"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if got := Extract([]rune(c.Text), c.Offset, c.Strip); got != c.Sentence {
			t.Errorf("Extract(%q, %d, %v) = %q, want %q", c.Text, c.Offset, c.Strip, got, c.Sentence)
		}
	}
	if len(cases) < 20 {
		t.Fatalf("only %d cases", len(cases))
	}
}
