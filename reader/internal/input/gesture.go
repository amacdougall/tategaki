package input

import "time"

// Kind is a classified touch.
type Kind int

const (
	Tap Kind = iota
	Hold
	SwipeLeft  // finger moved right-to-left
	SwipeRight // finger moved left-to-right
	SwipeUp
	SwipeDown
)

func (k Kind) String() string {
	return [...]string{"tap", "hold", "swipe-left", "swipe-right", "swipe-up", "swipe-down"}[k]
}

// Gesture is one classified touch in screen pixels.
type Gesture struct {
	Kind     Kind
	X, Y     int // where the finger went down
	X1, Y1   int // where it came up
	Duration time.Duration
}

// Classification thresholds, in screen pixels at 300 ppi.
const (
	SwipeMin = 120                    // ~1 cm of travel
	TapSlop  = 40                     // movement still counted as a tap
	HoldTime = 600 * time.Millisecond // press this long without moving
)

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Classify turns a completed touch into a gesture.
func Classify(t Touch) Gesture {
	g := Gesture{X: t.X0, Y: t.Y0, X1: t.X1, Y1: t.Y1, Duration: t.Duration}
	dx, dy := t.X1-t.X0, t.Y1-t.Y0
	switch {
	case abs(dx) >= SwipeMin && abs(dx) >= abs(dy):
		g.Kind = SwipeRight
		if dx < 0 {
			g.Kind = SwipeLeft
		}
	case abs(dy) >= SwipeMin:
		g.Kind = SwipeDown
		if dy < 0 {
			g.Kind = SwipeUp
		}
	case abs(dx) <= TapSlop && abs(dy) <= TapSlop && t.Duration >= HoldTime:
		g.Kind = Hold
	default:
		g.Kind = Tap
	}
	return g
}

// Gestures classifies a stream of touches.
func Gestures(touches <-chan Touch) <-chan Gesture {
	out := make(chan Gesture, 8)
	go func() {
		defer close(out)
		for t := range touches {
			out <- Classify(t)
		}
	}()
	return out
}
