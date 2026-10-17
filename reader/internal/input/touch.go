package input

import "time"

// Touch is a completed single-finger contact, in screen pixels.
type Touch struct {
	X0, Y0, X1, Y1 int // where the finger went down and came up
	RawX0, RawY0   int // device coordinates at touch-down, before mapping
	Duration       time.Duration
}

// Mapping converts raw touch coordinates to screen pixels.
type Mapping struct {
	MaxX, MaxY       int // raw axis maxima
	Width, Height    int // screen size
	SwapXY           bool
	MirrorX, MirrorY bool
}

// Apply maps a raw point to the screen.
func (m Mapping) Apply(x, y int) (int, int) {
	if m.SwapXY {
		x, y = y, x
	}
	maxX, maxY := m.MaxX, m.MaxY
	if m.SwapXY {
		maxX, maxY = maxY, maxX
	}
	if m.MirrorX {
		x = maxX - x
	}
	if m.MirrorY {
		y = maxY - y
	}
	if maxX > 0 && maxX != m.Width-1 {
		x = x * (m.Width - 1) / maxX
	}
	if maxY > 0 && maxY != m.Height-1 {
		y = y * (m.Height - 1) / maxY
	}
	return x, y
}

// Touches reads a multitouch (protocol B) device and emits one Touch per
// contact of the first finger. Other fingers are ignored for now.
func Touches(d *Device, m Mapping) <-chan Touch {
	ch := make(chan Touch, 8)
	go func() {
		defer close(ch)
		var (
			slot           int32
			down, wasDown  bool
			x, y           int
			startX, startY int
			rawX0, rawY0   int
			startT         time.Time
			haveStart      bool
		)
		for {
			ev, err := d.Read()
			if err != nil {
				return
			}
			switch {
			case ev.Type == EvSyn && ev.Code == SynReport:
				if down && !wasDown {
					rawX0, rawY0 = x, y
					startX, startY = m.Apply(x, y)
					startT = time.Now()
					haveStart = true
				}
				if !down && wasDown && haveStart {
					ex, ey := m.Apply(x, y)
					ch <- Touch{X0: startX, Y0: startY, X1: ex, Y1: ey, RawX0: rawX0, RawY0: rawY0, Duration: time.Since(startT)}
					haveStart = false
				}
				wasDown = down
			case ev.Type == EvAbs && ev.Code == AbsMTSlot:
				slot = ev.Value
			case slot != 0:
				// ignore secondary fingers
			case ev.Type == EvAbs && ev.Code == AbsMTTrack:
				down = ev.Value >= 0
			case ev.Type == EvAbs && ev.Code == AbsMTPosX:
				x = int(ev.Value)
			case ev.Type == EvAbs && ev.Code == AbsMTPosY:
				y = int(ev.Value)
			}
		}
	}()
	return ch
}
