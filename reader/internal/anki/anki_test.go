package anki

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuildFieldsGolden(t *testing.T) {
	data, err := os.ReadFile("../../../testdata/kaishi.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Args   map[string]string `json:"args"`
		Fields map[string]string `json:"fields"`
	}
	json.Unmarshal(data, &cases)
	for _, c := range cases {
		a := c.Args
		got := BuildFields(a["word"], a["sentence"], a["reading"], a["meaning"], a["note"])
		if !reflect.DeepEqual(got, c.Fields) {
			t.Errorf("BuildFields(%v) = %v, want %v", a, got, c.Fields)
		}
	}
}

// fakeAnki is a minimal AnkiConnect: notes are duplicates when their Word
// field (the note type's first field) is already present.
type fakeAnki struct {
	mu    sync.Mutex
	words map[string]bool
	calls []map[string]any
}

func (f *fakeAnki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req map[string]any
	json.Unmarshal(body, &req)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	params, _ := req["params"].(map[string]any)
	switch req["action"] {
	case "canAddNotes":
		notes := params["notes"].([]any)
		word := notes[0].(map[string]any)["fields"].(map[string]any)["Word"].(string)
		json.NewEncoder(w).Encode(map[string]any{"result": []bool{!f.words[word]}, "error": nil})
	case "addNote":
		note := params["note"].(map[string]any)
		word := note["fields"].(map[string]any)["Word"].(string)
		f.words[word] = true
		json.NewEncoder(w).Encode(map[string]any{"result": 1234, "error": nil})
	default:
		json.NewEncoder(w).Encode(map[string]any{"result": nil, "error": "unsupported action"})
	}
}

func TestQueueFlushSentAndDuplicate(t *testing.T) {
	fake := &fakeAnki{words: map[string]bool{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "anki-queue.jsonl")
	q, _ := OpenQueue(path)
	item := Item{Book: "b", Deck: "日本語::変な家", Model: ModelName, Tags: []string{"変な家"},
		Fields: BuildFields("蜘蛛", "蜘蛛の巣が張っている。", "くも", "spider", "")}
	q.Append(item)
	q.Append(item) // mined twice

	res := q.Flush(&Client{URL: srv.URL, APIKey: "k", Timeout: time.Second})
	if res.Sent != 1 || res.Duplicates != 1 || res.Unavailable != nil {
		t.Fatalf("flush = %+v", res)
	}
	// the request carried the API key, the deck and a null duplicateScope
	first := fake.calls[0]
	if first["key"] != "k" {
		t.Errorf("api key missing: %v", first)
	}
	raw, _ := json.Marshal(first["params"])
	if !strings.Contains(string(raw), `"duplicateScope":null`) || !strings.Contains(string(raw), "日本語::変な家") {
		t.Errorf("unexpected note: %s", raw)
	}

	// statuses persisted
	q2, err := OpenQueue(path)
	if err != nil {
		t.Fatal(err)
	}
	items := q2.Items()
	if items[0].Status != Sent || items[0].NoteID != 1234 || items[1].Status != Duplicate {
		t.Fatalf("reloaded statuses: %+v", items)
	}
	if q2.Pending() != 0 {
		t.Fatal("nothing should be pending")
	}
}

func TestQueueStaysPendingWhenUnreachable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "anki-queue.jsonl")
	q, _ := OpenQueue(path)
	q.Append(Item{Book: "b", Deck: "Default", Model: ModelName, Fields: BuildFields("猫", "", "", "", "")})
	q.Append(Item{Book: "b", Deck: "Default", Model: ModelName, Fields: BuildFields("犬", "", "", "", "")})
	res := q.Flush(&Client{URL: "http://127.0.0.1:1", Timeout: 500 * time.Millisecond})
	if res.Unavailable == nil || res.Pending != 2 {
		t.Fatalf("flush = %+v", res)
	}
	if q.Pending() != 2 {
		t.Fatal("items should stay pending")
	}
}

func TestQueueAnkiErrorMarksItem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"result": null, "error": "model was not found: Kaishi 1.5k"}`)
	}))
	defer srv.Close()
	q, _ := OpenQueue(filepath.Join(t.TempDir(), "q.jsonl"))
	q.Append(Item{Book: "b", Deck: "Default", Model: ModelName, Fields: BuildFields("猫", "", "", "", "")})
	res := q.Flush(&Client{URL: srv.URL, Timeout: time.Second})
	if res.Errors != 1 || q.Items()[0].Status != Failed || !strings.Contains(q.Items()[0].Error, "model was not found") {
		t.Fatalf("flush = %+v, items %+v", res, q.Items())
	}
}
