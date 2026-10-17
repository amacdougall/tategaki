// Command evinfo lists input devices and their absolute axis ranges, then
// optionally streams raw events. A device-bring-up diagnostic.
//
//	evinfo            list devices
//	evinfo -watch 10  also print events for 10 seconds
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/amacdougall/tategaki/reader/internal/input"
)

func main() {
	watch := flag.Int("watch", 0, "seconds to stream events from all devices")
	flag.Parse()

	paths, _ := filepath.Glob("/dev/input/event*")
	var devs []*input.Device
	for _, p := range paths {
		d, err := input.Open(p)
		if err != nil {
			fmt.Println(p, "error:", err)
			continue
		}
		fmt.Printf("%s  %q\n", p, d.Name)
		for _, ax := range []struct {
			name string
			code uintptr
		}{{"ABS_X", input.AbsX}, {"ABS_Y", input.AbsY}, {"ABS_MT_SLOT", input.AbsMTSlot},
			{"ABS_MT_POSITION_X", input.AbsMTPosX}, {"ABS_MT_POSITION_Y", input.AbsMTPosY},
			{"ABS_MT_TRACKING_ID", input.AbsMTTrack}} {
			ai, err := d.Abs(ax.code)
			if err == nil && (ai.Minimum != 0 || ai.Maximum != 0) {
				fmt.Printf("    %-20s min=%d max=%d res=%d\n", ax.name, ai.Minimum, ai.Maximum, ai.Resolution)
			}
		}
		devs = append(devs, d)
	}

	if *watch <= 0 {
		return
	}
	for _, d := range devs {
		go func(d *input.Device) {
			for {
				ev, err := d.Read()
				if err != nil {
					return
				}
				fmt.Printf("%s %d.%06d type=%d code=%#x value=%d\n", filepath.Base(d.Path), ev.Sec, ev.Usec, ev.Type, ev.Code, ev.Value)
			}
		}(d)
	}
	time.Sleep(time.Duration(*watch) * time.Second)
	os.Exit(0)
}
