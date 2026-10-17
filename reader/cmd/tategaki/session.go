package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/amacdougall/tategaki/reader/internal/display"
	"github.com/amacdougall/tategaki/reader/internal/input"
	"github.com/amacdougall/tategaki/reader/internal/kindle"
)

const (
	screenW = 1236
	screenH = 1648
)

// session owns the device for one run: display, touch, power events and
// signals.
type session struct {
	fb        *display.FBInk
	tmp       string
	idle      time.Duration
	touch     *input.Device
	touches   <-chan input.Touch
	power     <-chan string
	stopPower func()
	sigs      chan os.Signal
}

func newSession(fb *display.FBInk, tmp string, idle time.Duration) (*session, error) {
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return nil, err
	}
	s := &session{fb: fb, tmp: tmp, idle: idle, stopPower: func() {}}
	touch, err := input.FindByName("pt_mt")
	if err != nil {
		return nil, err
	}
	s.touch = touch
	ax, _ := touch.Abs(input.AbsMTPosX)
	ay, _ := touch.Abs(input.AbsMTPosY)
	mapping := input.Mapping{MaxX: int(ax.Maximum), MaxY: int(ay.Maximum), Width: screenW, Height: screenH}
	log.Printf("touch device %s %q, mapping %+v", touch.Path, touch.Name, mapping)
	s.touches = input.Touches(touch, mapping)

	if power, stop, err := kindle.PowerEvents(); err != nil {
		log.Printf("power events unavailable: %v", err)
	} else {
		s.power, s.stopPower = power, stop
	}
	s.sigs = make(chan os.Signal, 1)
	signal.Notify(s.sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	return s, nil
}

func (s *session) close() {
	s.stopPower()
	s.touch.Close()
}

// loop dispatches events until onTouch returns true, a signal arrives, or the
// idle timeout passes. onWake runs when the device leaves the screensaver.
func (s *session) loop(onTouch func(input.Touch) bool, onWake func()) error {
	timer := time.NewTimer(s.idle)
	var lastIdleReset time.Time
	power := s.power
	for {
		select {
		case t, ok := <-s.touches:
			if !ok {
				return fmt.Errorf("touch device closed")
			}
			timer.Reset(s.idle)
			if time.Since(lastIdleReset) > 30*time.Second {
				lastIdleReset = time.Now()
				go kindle.ResetIdleTimer()
			}
			if onTouch(t) {
				return nil
			}
		case ev, ok := <-power:
			if !ok {
				log.Printf("power event stream ended")
				power = nil
				continue
			}
			log.Printf("POWER %s", ev)
			if strings.HasPrefix(ev, "outOfScreenSaver") {
				onWake()
			}
		case sig := <-s.sigs:
			log.Printf("signal %v", sig)
			return nil
		case <-timer.C:
			log.Printf("idle for %v, exiting", s.idle)
			return nil
		}
	}
}
