package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/amacdougall/tategaki/reader/internal/input"
	"github.com/amacdougall/tategaki/reader/internal/kindle"
)

const (
	screenW = 1236
	screenH = 1648
)

// events is everything the app reacts to: gestures, wake-ups and signals.
// The device and the simulator fill it from different sources.
type events struct {
	gestures <-chan input.Gesture
	power    <-chan string    // com.lab126.powerd events (device only)
	wake     <-chan struct{}  // simulated wake (sim only)
	settle   <-chan time.Time // post-launch redraws (device only)
	sigs     chan os.Signal
	idle     time.Duration
	// onInput runs on every gesture (the device resets powerd's idle timer).
	onInput func()
}

func newSignals() chan os.Signal {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	return sigs
}

// deviceEvents opens the touchscreen and powerd's event stream.
func deviceEvents(idle time.Duration) (*events, func(), error) {
	touch, err := input.FindByName("pt_mt")
	if err != nil {
		return nil, nil, err
	}
	ax, _ := touch.Abs(input.AbsMTPosX)
	ay, _ := touch.Abs(input.AbsMTPosY)
	mapping := input.Mapping{MaxX: int(ax.Maximum), MaxY: int(ay.Maximum), Width: screenW, Height: screenH}
	log.Printf("touch device %s %q, mapping %+v", touch.Path, touch.Name, mapping)

	ev := &events{gestures: input.Gestures(input.Touches(touch, mapping)), sigs: newSignals(), idle: idle}
	settle := make(chan time.Time, 2)
	ev.settle = settle
	for _, d := range []time.Duration{2 * time.Second, 5 * time.Second} {
		time.AfterFunc(d, func() { settle <- time.Now() })
	}
	stopPower := func() {}
	if power, stop, err := kindle.PowerEvents(); err != nil {
		log.Printf("power events unavailable: %v", err)
	} else {
		ev.power, stopPower = power, stop
	}
	var lastReset time.Time
	ev.onInput = func() {
		if time.Since(lastReset) > 30*time.Second {
			lastReset = time.Now()
			go kindle.ResetIdleTimer()
		}
	}
	return ev, func() { stopPower(); touch.Close() }, nil
}

// loop dispatches events until onGesture returns true, a signal arrives, or
// the idle timeout passes.
func (e *events) loop(onGesture func(input.Gesture) bool, onWake, onSettle func()) error {
	timer := time.NewTimer(e.idle)
	power := e.power
	for {
		select {
		case g, ok := <-e.gestures:
			if !ok {
				return fmt.Errorf("touch input closed")
			}
			timer.Reset(e.idle)
			if e.onInput != nil {
				e.onInput()
			}
			if onGesture(g) {
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
		case <-e.wake:
			onWake()
		case <-e.settle:
			if onSettle != nil {
				onSettle()
			}
		case sig := <-e.sigs:
			log.Printf("signal %v", sig)
			return nil
		case <-timer.C:
			log.Printf("idle for %v, exiting", e.idle)
			return nil
		}
	}
}
