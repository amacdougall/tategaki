package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDeviceDefaultsAndOverride(t *testing.T) {
	p := Paths{Root: t.TempDir()}
	d, err := LoadDevice(p)
	if err != nil || d.FullRefreshEvery != 6 || d.RankingPolicy != "longest" {
		t.Fatalf("defaults: %+v %v", d, err)
	}
	os.WriteFile(p.Config(), []byte(`{"full_refresh_every": 4, "default_deck": "日本語"}`), 0o644)
	d, err = LoadDevice(p)
	if err != nil || d.FullRefreshEvery != 4 || d.DefaultDeck != "日本語" || d.ScanLen != 16 {
		t.Fatalf("override: %+v %v", d, err)
	}
}

func TestSidecarMissingAndPresent(t *testing.T) {
	p := Paths{Root: t.TempDir()}
	os.MkdirAll(p.Books(), 0o755)
	b, err := LoadBook(p, "x")
	if err != nil || b.Deck != nil {
		t.Fatalf("missing sidecar: %+v %v", b, err)
	}
	os.WriteFile(p.Sidecar("x"), []byte(`{"deck":"日本語::変な家","tags":["変な家"],"strip_speaker_tags":true}`), 0o644)
	b, err = LoadBook(p, "x")
	if err != nil || *b.Deck != "日本語::変な家" || !b.StripSpeakerTags || b.Tags[0] != "変な家" {
		t.Fatalf("sidecar: %+v %v", b, err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	p := Paths{Root: t.TempDir()}
	s := OpenState(p)
	if err := s.Set("toradora-01", Position{Chapter: 12, Offset: 413, Page: 20, PageCount: 557}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p.State() + ".tmp"); !os.IsNotExist(err) {
		t.Error("temp file left behind")
	}
	s2 := OpenState(p)
	pos, ok := s2.Get("toradora-01")
	if !ok || pos.Chapter != 12 || pos.Offset != 413 || s2.State.LastBook != "toradora-01" {
		t.Fatalf("reloaded: %+v %v", s2.State, ok)
	}
}

func TestCorruptStateStartsEmpty(t *testing.T) {
	p := Paths{Root: t.TempDir()}
	os.WriteFile(filepath.Join(p.Root, "state.json"), []byte("{not json"), 0o644)
	if s := OpenState(p); len(s.State.Books) != 0 {
		t.Fatal("expected empty state")
	}
}
