package input

import (
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	ms := time.Millisecond
	cases := []struct {
		t    Touch
		want Kind
	}{
		{Touch{X0: 100, Y0: 100, X1: 104, Y1: 98, Duration: 120 * ms}, Tap},
		{Touch{X0: 100, Y0: 100, X1: 130, Y1: 120, Duration: 200 * ms}, Tap}, // small drift
		{Touch{X0: 100, Y0: 100, X1: 105, Y1: 100, Duration: 900 * ms}, Hold},
		{Touch{X0: 800, Y0: 500, X1: 300, Y1: 540, Duration: 250 * ms}, SwipeLeft},
		{Touch{X0: 300, Y0: 500, X1: 800, Y1: 460, Duration: 250 * ms}, SwipeRight},
		{Touch{X0: 300, Y0: 900, X1: 320, Y1: 300, Duration: 250 * ms}, SwipeUp},
		{Touch{X0: 300, Y0: 300, X1: 320, Y1: 900, Duration: 250 * ms}, SwipeDown},
		{Touch{X0: 100, Y0: 100, X1: 170, Y1: 100, Duration: 900 * ms}, Tap}, // moved too much for a hold, too little for a swipe
	}
	for _, c := range cases {
		if got := Classify(c.t).Kind; got != c.want {
			t.Errorf("Classify(%+v) = %v, want %v", c.t, got, c.want)
		}
	}
}
