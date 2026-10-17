// Command tategaki is the on-device reader. launch.sh pauses and restores the
// Kindle UI around it.
//
//	tategaki                      run on the Kindle
//	tategaki --sim --root ./simroot   desktop simulator at http://localhost:8800
//	tategaki --hello              Phase 0 bring-up test (device only)
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/amacdougall/tategaki/reader/internal/config"
	"github.com/amacdougall/tategaki/reader/internal/display"
	"github.com/amacdougall/tategaki/reader/internal/input"
	"github.com/amacdougall/tategaki/reader/internal/sim"
	"github.com/amacdougall/tategaki/reader/internal/text"
	"github.com/amacdougall/tategaki/reader/internal/ui"
)

// findFonts locates the UI fonts: next to the binary in the package
// (bin/../fonts), or data/fonts in the repo for the simulator.
func findFonts(flagDir string) string {
	candidates := []string{flagDir}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "..", "fonts"))
	}
	candidates = append(candidates, "data/fonts", "../data/fonts")
	for _, d := range candidates {
		if d == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(d, "BIZUDPGothic-Regular.ttf")); err == nil {
			return d
		}
	}
	return "fonts"
}

func main() {
	fbinkBin := flag.String("fbink", "fbink", "path to the FBInk CLI")
	tmp := flag.String("tmp", "/var/tmp/tategaki", "scratch directory")
	root := flag.String("root", "/mnt/us/tategaki", "reader data folder (books/, config.json, state.json)")
	fontDir := flag.String("fonts", "", "UI font folder")
	idle := flag.Duration("idle-exit", 0, "exit after this long without input (0: never)")
	simMode := flag.Bool("sim", false, "run the desktop simulator instead of driving a Kindle")
	listen := flag.String("listen", "localhost:8800", "simulator address")
	hello := flag.Bool("hello", false, "run the Phase 0 bring-up test")
	dictPath := flag.String("dict", "", "jmdict.sqlite (default: <root>/jmdict.sqlite, then the repo's data/)")
	flag.Parse()
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("tategaki starting, pid %d", os.Getpid())
	if *idle == 0 {
		*idle = 1<<63 - 1
	}

	if err := run(*fbinkBin, *tmp, *root, findFonts(*fontDir), *dictPath, *idle, *simMode, *listen, *hello); err != nil {
		log.Printf("fatal: %v", err)
		os.Exit(1)
	}
	log.Printf("clean exit")
}

func run(fbinkBin, tmp, root, fontDir, dictPath string, idle time.Duration, simMode bool, listen string, hello bool) error {
	if simMode && tmp == "/var/tmp/tategaki" {
		tmp = filepath.Join(os.TempDir(), "tategaki-sim")
	}
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}

	var disp display.Display
	var ev *events
	if simMode {
		sd := display.NewSim(screenW, screenH)
		srv := sim.New(sd)
		go func() {
			log.Printf("simulator: http://%s/", listen)
			if err := http.ListenAndServe(listen, srv.Handler()); err != nil {
				log.Fatal(err)
			}
		}()
		disp = sd
		ev = &events{gestures: srv.Gestures, wake: srv.Wake, sigs: newSignals(), idle: idle}
	} else {
		fb := &display.FBInk{Bin: fbinkBin, Tmp: tmp, Width: screenW, Height: screenH}
		dev, cleanup, err := deviceEvents(idle)
		if err != nil {
			return err
		}
		defer cleanup()
		if hello {
			return runHello(fb, dev)
		}
		disp, ev = fb, dev
	}

	fonts, err := text.Load(filepath.Join(fontDir, "BIZUDPGothic-Regular.ttf"), filepath.Join(fontDir, "BIZUDPGothic-Bold.ttf"))
	if err != nil {
		return err
	}
	app := ui.New(disp, fonts, config.Paths{Root: root}, tmp)
	app.DictPaths = []string{dictPath, "data/jmdict.sqlite", "../data/jmdict.sqlite"}
	app.Start()
	return ev.loop(func(g input.Gesture) bool {
		app.Gesture(g)
		return app.Done()
	}, app.Wake, func() {
		log.Printf("post-launch redraw")
		app.Refresh()
	})
}
