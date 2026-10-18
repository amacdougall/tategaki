package ui

import (
	"bytes"
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

func TestSleepDrawsCover(t *testing.T) {
	a, _ := testApp(t)
	sim := a.Disp.(*display.Sim)
	a.Start() // library: no book opened, none read
	a.Sleep()
	if !bytes.Equal(sim.Frame().Pix, a.goodnight().Pix) {
		t.Fatal("no book: screen isn't お休みなさい")
	}

	if err := a.OpenBook("toradora-01"); err != nil {
		t.Fatal(err)
	}
	want, err := a.sleepCover("toradora-01")
	if err != nil {
		t.Fatal(err)
	}
	a.Sleep()
	if !bytes.Equal(sim.Frame().Pix, want.Pix) {
		t.Fatal("screen isn't the sleep cover")
	}
	a.Dark = true
	a.Sleep()
	if got := sim.Frame().Pix; got[0] != 255-want.Pix[0] || got[len(got)/2] != 255-want.Pix[len(got)/2] {
		t.Fatal("dark mode cover isn't pre-inverted")
	}
}

func TestSleepGoodnightSetting(t *testing.T) {
	a, paths := testApp(t)
	sim := a.Disp.(*display.Sim)
	if err := a.OpenBook("toradora-01"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(paths.Sidecar("toradora-01"), []byte(`{"sleep_screen":"goodnight"}`), 0o644)
	a.Dark = true // goodnight isn't pre-inverted
	a.Sleep()
	if !bytes.Equal(sim.Frame().Pix, a.goodnight().Pix) {
		t.Fatal("sleep_screen goodnight: screen isn't お休みなさい")
	}
}
