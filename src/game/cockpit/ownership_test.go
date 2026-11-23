package cockpit

import (
	"gunship2000-assets/internal/rasterlayout"
	"testing"
)

func TestFractionalOverlayCoverage(t *testing.T) {
	var scratch OverlayScratch
	for _, name := range []string{"1280x1024", "800x600", "1024x768", "800x600"} {
		l := rasterlayout.ByName(name)
		dst := make([]byte, l.Width*l.Height)
		owner := make([]byte, len(dst))
		for i := range dst {
			dst[i] = byte(i & 1)
			owner[i] = 0xb4
		}
		b := Rect{Left: 80, Top: 90, Right: 85, Bottom: 94}
		if err := scratch.DrawRect(dst, owner, 1, b, func(p []byte) error {
			for y := b.Top; y <= b.Bottom; y++ {
				for x := b.Left; x <= b.Right; x++ {
					i := y*320 + x
					if p[i] == 1 {
						PutLogical(p, i, 1)
					}
				}
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		for i, v := range dst {
			x, y := i%l.Width, i/l.Width
			want := byte(0xb4)
			if x >= l.X[b.Left] && x < l.X[b.Right+1] && y >= l.Y[b.Top] && y < l.Y[b.Bottom+1] && i&1 == 1 {
				want = 0
			}
			if v != byte(i&1) || owner[i] != want {
				t.Fatal(name, x, y, v, owner[i])
			}
		}
	}
}
