// Command tategaki is the on-device reader. launch.sh pauses and restores the
// Kindle UI around it.
//
// Until the reader proper (plan Phase 3) exists, it runs either the Phase 0
// bring-up test (-hello) or a minimal page viewer for proof sheets.
package main

import (
	"flag"
	"log"
	"os"
	"time"

	"github.com/amacdougall/tategaki/reader/internal/display"
)

func main() {
	fbinkBin := flag.String("fbink", "fbink", "path to the FBInk CLI")
	tmp := flag.String("tmp", "/var/tmp/tategaki", "scratch directory")
	idle := flag.Duration("idle-exit", 30*time.Minute, "exit after this long without input")
	hello := flag.Bool("hello", false, "run the Phase 0 bring-up test instead of the viewer")
	books := flag.String("books", "/mnt/us/tategaki/books", "folder of .jbk bundles")
	bookPath := flag.String("book", "", "open this .jbk (default: the most recently deployed)")
	flag.Parse()
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Printf("tategaki starting, pid %d", os.Getpid())

	s, err := newSession(&display.FBInk{Bin: *fbinkBin}, *tmp, *idle)
	if err == nil {
		defer s.close()
		if *hello {
			err = runHello(s)
		} else {
			err = runViewer(s, *books, *bookPath)
		}
	}
	if err != nil {
		log.Printf("fatal: %v", err)
		os.Exit(1)
	}
	log.Printf("clean exit")
}
