package dict

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Golden fixtures from tools/gen_fixtures.py (the Python reference).
const testdata = "../../../testdata"

func dbPath(t testing.TB) string {
	p := filepath.Join("..", "..", "..", "data", "jmdict.sqlite")
	if _, err := os.Stat(p); err != nil {
		t.Skipf("no %s (run tools/prepare_jmdict.py)", p)
	}
	return p
}

func openDB(t testing.TB) *DB {
	db, err := Open(dbPath(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func load(t *testing.T, name string, v any) {
	data, err := os.ReadFile(filepath.Join(testdata, name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

type fixtureDeinflection []json.RawMessage // [term, rules, reasons]

func decodeDeinflections(t *testing.T, raw []fixtureDeinflection) []Deinflection {
	out := []Deinflection{}
	for _, r := range raw {
		var d Deinflection
		if err := json.Unmarshal(r[0], &d.Term); err != nil {
			t.Fatal(err)
		}
		json.Unmarshal(r[1], &d.Rules)
		json.Unmarshal(r[2], &d.Reasons)
		out = append(out, d)
	}
	return out
}

func TestDeinflectGolden(t *testing.T) {
	var cases []struct {
		Source          string                `json:"source"`
		Deinflect       []fixtureDeinflection `json:"deinflect"`
		CompletePartial []fixtureDeinflection `json:"complete_partial"`
	}
	load(t, "deinflect.json", &cases)
	for _, c := range cases {
		if got, want := Deinflect(c.Source), decodeDeinflections(t, c.Deinflect); !reflect.DeepEqual(got, want) {
			t.Errorf("Deinflect(%q): got %d results, want %d\n got  %v\n want %v", c.Source, len(got), len(want), got, want)
		}
		got, want := CompletePartialInflection(c.Source), decodeDeinflections(t, c.CompletePartial)
		if len(got) == 0 && len(want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("CompletePartialInflection(%q):\n got  %v\n want %v", c.Source, got, want)
		}
	}
}

func TestLookupCandidatesGolden(t *testing.T) {
	db := openDB(t)
	var cases []struct {
		Selection  string      `json:"selection"`
		Candidates []Candidate `json:"candidates"`
	}
	load(t, "lookup.json", &cases)
	for _, c := range cases {
		got, err := db.LookupCandidates(c.Selection, MaxResults)
		if err != nil {
			t.Fatal(err)
		}
		diff(t, "LookupCandidates("+c.Selection+")", got, c.Candidates)
	}
}

func TestScanGolden(t *testing.T) {
	db := openDB(t)
	var cases []struct {
		Text       string      `json:"text"`
		Offset     int         `json:"offset"`
		ParaEnd    int         `json:"para_end"`
		Policy     string      `json:"policy"`
		Candidates []Candidate `json:"candidates"`
	}
	load(t, "scan.json", &cases)
	for _, c := range cases {
		got, err := db.Scan([]rune(c.Text), c.Offset, c.ParaEnd, ScanLen, c.Policy, ScanMaxResults)
		if err != nil {
			t.Fatal(err)
		}
		r := []rune(c.Text)
		diff(t, "Scan("+string(r[c.Offset:min(c.Offset+6, len(r))])+"…, "+c.Policy+")", got, c.Candidates)
	}
}

func diff(t *testing.T, name string, got, want []Candidate) {
	t.Helper()
	if reflect.DeepEqual(got, want) {
		return
	}
	words := func(cs []Candidate) []string {
		var w []string
		for _, c := range cs {
			w = append(w, c.Word)
		}
		return w
	}
	t.Errorf("%s:\n got  %v\n want %v", name, words(got), words(want))
	for i := range min(len(got), len(want)) {
		if !reflect.DeepEqual(got[i], want[i]) {
			g, _ := json.Marshal(got[i])
			w, _ := json.Marshal(want[i])
			t.Errorf("  first difference at %d:\n   got  %s\n   want %s", i, g, w)
			break
		}
	}
}

// Plan Phase 2 acceptance: a scan completes in under 100 ms on the desktop.
func TestScanSpeed(t *testing.T) {
	db := openDB(t)
	text := []rune("　苛立ちに任せ、曇る鏡を乱暴に手のひらで拭った。ボロい洗面所には目覚ましに浴びたシャワーの湿気がこもったままで")
	var worst time.Duration
	for off := range text {
		db.entries = map[int64]*Entry{} // cold cache: measure real queries
		start := time.Now()
		if _, err := db.Scan(text, off, len(text), ScanLen, PolicyLongest, ScanMaxResults); err != nil {
			t.Fatal(err)
		}
		worst = max(worst, time.Since(start))
	}
	t.Logf("worst cold scan over %d offsets: %v", len(text), worst)
	if worst > 100*time.Millisecond {
		t.Errorf("worst scan took %v, budget 100ms", worst)
	}
}
