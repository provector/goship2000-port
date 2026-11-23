package cockpit

import (
	"bytes"
	"errors"
	"testing"

	"gunship2000-assets/internal/rasterlayout"
)

// Scalar reference for the interleaved overlays, kept independent of phase tables.
func referenceOverlay(dst, owners, plane []byte, l *rasterlayout.Layout, b Rect, draw func([]byte) error) error {
	for dy := 0; dy*200 < l.PictureHeight; dy++ {
		for dx := 0; dx*320 < l.Width; dx++ {
			for y := max(0, b.Top); y <= min(199, b.Bottom); y++ {
				for x := max(0, b.Left); x <= min(319, b.Right); x++ {
					i := y*320 + x
					at := min(l.Y[y]+dy, l.Y[y+1]-1)*l.Width + min(l.X[x]+dx, l.X[x+1]-1)
					plane[i] = dst[at]
					if len(owners) != 0 {
						plane[logicalPixels+i] = owners[at]
					}
				}
			}
			if err := draw(plane); err != nil {
				return err
			}
			for y := max(0, b.Top); y <= min(199, b.Bottom); y++ {
				for x := max(0, b.Left); x <= min(319, b.Right); x++ {
					px, py := l.X[x]+dx, l.Y[y]+dy
					if px >= l.X[x+1] || py >= l.Y[y+1] {
						continue
					}
					at := py*l.Width + px
					i := y*320 + x
					dst[at] = plane[i]
					if len(owners) != 0 {
						owners[at] = plane[logicalPixels+i]
					}
				}
			}
		}
	}
	return nil
}

func TestMappedOverlayPhasesAndClippedWrites(t *testing.T) {
	var scratch OverlayScratch
	reference := make([]byte, 2*logicalPixels)
	stop := errors.New("stop in callback")
	for _, mode := range []string{"640x480", "1024x768", "800x600", "1280x1024", "640x480"} {
		l := rasterlayout.ByName(mode)
		pixels := make([]byte, l.Width*l.Height)
		for i := range pixels {
			pixels[i] = byte(i*7 + i/l.Width)
		}
		wantPixels := bytes.Clone(pixels)
		for n, b := range []Rect{{-12, 38, 74, 65}, {316, 196, 333, 211}, {319, 199, 319, 199}, {50, 50, 49, 51}, {330, 1, 350, 2}, {11, 6, 179, 53}, Full} {
			var owner, wantOwner []byte
			plane := reference[:logicalPixels]
			if n%2 == 0 {
				owner = bytes.Clone(pixels)
				wantOwner = bytes.Clone(owner)
				plane = reference
			}
			var snapshots [][]byte
			edit := func(buf []byte, phase int) error {
				if n == 5 && phase == 2 {
					return stop
				}
				for y := max(0, b.Top); y <= min(199, b.Bottom); y++ {
					for x := max(0, b.Left); x <= min(319, b.Right); x++ {
						i := y*320 + x
						if (int(buf[i])+phase)%4 == 0 {
							PutLogical(buf, i, buf[i]^byte(phase))
						}
					}
				}
				return nil
			}
			want := referenceOverlay(wantPixels, wantOwner, plane, l, b, func(buf []byte) error {
				snapshots = append(snapshots, bytes.Clone(buf))
				return edit(buf, len(snapshots)-1)
			})
			calls := 0
			got := scratch.DrawRect(pixels, owner, 1, b, func(buf []byte) error {
				if calls >= len(snapshots) || !bytes.Equal(buf, snapshots[calls]) {
					t.Fatalf("%s bounds %v, gather %d", mode, b, calls)
				}
				calls++
				return edit(buf, calls-1)
			})
			if got != want || calls != len(snapshots) || !bytes.Equal(pixels, wantPixels) || !bytes.Equal(owner, wantOwner) {
				t.Fatalf("%s bounds %v, output mismatch after %d calls", mode, b, calls)
			}
		}
		// Use a separate warmed view so this measurement does not alter the
		// retained scratch contents in the subsequent layout-transition check.
		var measured OverlayScratch
		callback := func([]byte) error { return nil }
		if allocs := testing.AllocsPerRun(3, func() {
			if err := measured.DrawRect(pixels, nil, 1, Full, callback); err != nil {
				panic(err)
			}
		}); allocs != 0 {
			t.Fatalf("%s warmed mapper allocated %v times", mode, allocs)
		}
	}
}
