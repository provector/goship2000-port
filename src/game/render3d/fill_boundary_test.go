package render3d

import "testing"

// VGA1d47:043c/0446,04a0/04aa,0503/050d,0563/056d,05b9/05c3
// independently establish >= right / <= left rejection before the inclusive span.
func TestWorldFillBoundaryOnlySpans(t *testing.T) {
	for _, scale := range []int{1, 2} {
		width, height := Width*scale, Height*scale
		b := Rect{10 * scale, 10 * scale, 20 * scale, 20 * scale}
		for _, word := range []uint16{0x3333, 0x0033, 0xec00, 0xffff, 0x0098} {
			dst := make([]byte, width*height)
			for i := range dst {
				dst[i] = 0x64
			}
			for _, x := range []int{b.Left, b.Right} {
				if _, err := FillPolygon(dst, []Point{{x, b.Top}, {x, b.Bottom}}, word, b, 0, nil); err != nil {
					t.Fatal(err)
				}
			}
			for i, v := range dst {
				if v != 0x64 {
					t.Fatalf("scale%d word%x changed boundary-only pixel%d", scale, word, i)
				}
			}
		}
		dst := make([]byte, width*height)
		if _, err := FillPolygon(dst, []Point{{b.Left, b.Top}, {b.Right, b.Top}, {b.Right, b.Bottom}, {b.Left, b.Bottom}}, 0x3333, b, 0, nil); err != nil {
			t.Fatal(err)
		}
		for _, x := range []int{b.Left, b.Right} {
			if dst[(b.Top+1)*width+x] != 0x33 {
				t.Fatalf("accepted span lost inclusive endpoint scale%d x%d", scale, x)
			}
		}
	}
}
