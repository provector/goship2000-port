package render3d

import "testing"

func TestRejectedEdgeRetainsSourceXBoundary(t *testing.T) {
	b := Rect{0, 0, 319, 199}
	// Hand execution of resident003c. SI startsFFFF, X clip stores the
	// boundary even if its interpolated Y causes a later84/92 rejection.
	cases := []struct {
		a, z     Point
		boundary int
	}{
		{Point{-10, 190}, Point{10, 220}, 0}, // X0 ->Y205, bothbelow
		{Point{329, 190}, Point{309, 220}, 319},
		{Point{-10, 10}, Point{10, -20}, 0}, // X0 ->Y-5, bothabove
		{Point{329, 10}, Point{309, -20}, 319},
		{Point{-10752, -4858}, Point{1632, 1382}, 0}, // quotient5417 ->Y559
		{Point{5, 220}, Point{15, 250}, -1},          // no priorXclip
	}
	for _, q := range cases {
		_, _, ok, reason, err := ClipLine(q.a, q.z, b)
		if err != nil || ok || reason != q.boundary {
			t.Fatalf("%+v:ok%v reason%d error%v", q, ok, reason, err)
		}
	}
}
func TestProjectedGroundQuadClosesForeground(t *testing.T) {
	// Supplied near-clipped cell44 quad. Hand intersection atY199 spans
	// X-715..416, so original2065's closure must fill the entirebottomrow.
	dst := make([]byte, 64000)
	_, err := FillPolygon(dst, []Point{{1632, 1382}, {307, 93}, {34, -48}, {-10752, -4858}}, 0x8c8c, Rect{0, 0, 319, 199}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	for x := 0; x < 320; x++ {
		if dst[199*320+x] != 0x8c {
			t.Fatalf("foregroundhole x%d", x)
		}
	}
}
