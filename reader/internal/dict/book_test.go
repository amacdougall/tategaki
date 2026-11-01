package dict

import "testing"

var testBook = BookDict{
	{Kanji: "五河士道", Reading: "いつかしどう", Definition: "protagonist"},
	{Kanji: "士道", Reading: "しどう", Definition: "Shidou, the protagonist"},
	{Kanji: "トウカ", Definition: "heroine"},
}

func TestBookScan(t *testing.T) {
	text := []rune("「五河士道」と呼んだ。トウカ")
	got := testBook.Scan(text, 1, 6)
	if len(got) != 1 || got[0].Word != "五河士道" || got[0].SourceLen != 4 || !got[0].Book || got[0].EntSeq != -1 {
		t.Fatalf("scan at 五: %+v", got)
	}
	if got[0].Reading != "いつかしどう" || got[0].Meaning != "protagonist" || got[0].Rank != Unranked {
		t.Fatalf("fields: %+v", got[0])
	}
	if got := testBook.Scan(text, 3, 6); len(got) != 1 || got[0].Word != "士道" || got[0].SourceLen != 2 {
		t.Fatalf("scan at 士: %+v", got)
	}
	if got := testBook.Scan(text, 2, 6); len(got) != 0 {
		t.Fatalf("scan at 河: %+v", got)
	}
	// The paragraph end bounds the match.
	if got := testBook.Scan(text, 1, 4); len(got) != 0 {
		t.Fatalf("scan past paragraph end: %+v", got)
	}
	if got := testBook.Scan(text, 11, len(text)); len(got) != 1 || got[0].Reading != "" || got[0].SourceLen != 3 {
		t.Fatalf("kana-only entry: %+v", got)
	}
}

func TestBookScanMatchesReadingAndSortsLongestFirst(t *testing.T) {
	text := []rune("いつかしどうです")
	got := testBook.Scan(text, 0, len(text))
	if len(got) != 1 || got[0].Word != "五河士道" || got[0].SourceLen != 6 {
		t.Fatalf("reading match: %+v", got)
	}
	both := BookDict{{Kanji: "士", Definition: "a"}, {Kanji: "士道", Definition: "b"}}
	got = both.Scan([]rune("士道"), 0, 2)
	if len(got) != 2 || got[0].Word != "士道" || got[1].Word != "士" {
		t.Fatalf("order: %+v", got)
	}
}

func TestBookExact(t *testing.T) {
	if got := testBook.Exact("しどう"); len(got) != 1 || got[0].Word != "士道" {
		t.Fatalf("exact reading: %+v", got)
	}
	if got := testBook.Exact("五河士"); len(got) != 0 {
		t.Fatalf("partial: %+v", got)
	}
}
