package input

import "testing"

func TestMappingApply(t *testing.T) {
	pw11 := Mapping{MaxX: 1235, MaxY: 1647, Width: 1236, Height: 1648}
	cases := []struct {
		name   string
		m      Mapping
		x, y   int
		wx, wy int
	}{
		{"identity origin", pw11, 0, 0, 0, 0},
		{"identity far corner", pw11, 1235, 1647, 1235, 1647},
		{"mirror x", Mapping{MaxX: 1235, MaxY: 1647, Width: 1236, Height: 1648, MirrorX: true}, 0, 10, 1235, 10},
		{"swap", Mapping{MaxX: 1647, MaxY: 1235, Width: 1236, Height: 1648, SwapXY: true}, 1647, 0, 0, 1647},
		{"scale", Mapping{MaxX: 4095, MaxY: 4095, Width: 1236, Height: 1648}, 4095, 4095, 1235, 1647},
	}
	for _, c := range cases {
		gx, gy := c.m.Apply(c.x, c.y)
		if gx != c.wx || gy != c.wy {
			t.Errorf("%s: Apply(%d,%d) = (%d,%d), want (%d,%d)", c.name, c.x, c.y, gx, gy, c.wx, c.wy)
		}
	}
}
