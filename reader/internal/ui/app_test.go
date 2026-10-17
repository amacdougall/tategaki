package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amacdougall/tategaki/reader/internal/config"
	"github.com/amacdougall/tategaki/reader/internal/display"
	"github.com/amacdougall/tategaki/reader/internal/text"
)

func testApp(t *testing.T) (*App, config.Paths) {
	home, _ := os.UserHomeDir()
	jbk := filepath.Join(home, "Tategaki", "books", "toradora-01", "build", "toradora-01.jbk")
	if _, err := os.Stat(jbk); err != nil {
		t.Skip("test book not built")
	}
	paths := config.Paths{Root: t.TempDir()}
	os.MkdirAll(paths.Books(), 0o755)
	os.Symlink(jbk, paths.Book("toradora-01"))
	fonts, err := text.Load("../../../data/fonts/BIZUDPGothic-Regular.ttf", "../../../data/fonts/BIZUDPGothic-Bold.ttf")
	if err != nil {
		t.Fatal(err)
	}
	return New(display.NewSim(1236, 1648), fonts, paths, t.TempDir()), paths
}

func TestStartShowsLibraryWithoutHistory(t *testing.T) {
	a, _ := testApp(t)
	a.Start()
	if _, ok := a.screen.(*Library); !ok {
		t.Fatalf("screen is %T, want *Library", a.screen)
	}
}

func TestStartResumesLastBook(t *testing.T) {
	a, paths := testApp(t)
	a.Start()
	if err := a.OpenBook("toradora-01"); err != nil {
		t.Fatal(err)
	}
	r := a.screen.(*Reader)
	if err := r.Goto(a, 122); err != nil {
		t.Fatal(err)
	}

	b, _ := testApp(t) // fresh app over the same state
	b.Paths, b.State = paths, config.OpenState(paths)
	b.Start()
	got, ok := b.screen.(*Reader)
	if !ok || got.page != 122 {
		t.Fatalf("resumed to %T page %v, want Reader page 122", b.screen, got)
	}
}

func TestStartFallsBackWhenBookRemoved(t *testing.T) {
	a, _ := testApp(t)
	a.State.Set("gone", config.Position{})
	a.Start()
	if _, ok := a.screen.(*Library); !ok {
		t.Fatalf("screen is %T, want *Library", a.screen)
	}
}
