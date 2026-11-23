package render3d

import (
	"bytes"
	"errors"
	"math/rand"
	"testing"
)

// Frozen V3.2 row construction, including the original by-value workspaces.
type baselineSpans struct{ left, right [1024]int }

func baselineNewSpans(top, bottom int) baselineSpans {
	s := baselineSpans{}
	for y := top; y <= bottom; y++ {
		s.left[y] = 32768
		s.right[y] = -32768
	}
	return s
}
func (s *baselineSpans) add(x, y int) {
	if y >= 0 && y < len(s.left) {
		if x < s.left[y] {
			s.left[y] = x
		}
		if x > s.right[y] {
			s.right[y] = x
		}
	}
}
func (s *baselineSpans) vertical(x, y0, y1 int, b Rect) {
	if y0 < b.Top {
		y0 = b.Top
	}
	if y0 > b.Bottom {
		y0 = b.Bottom
	}
	if y1 < b.Top {
		y1 = b.Top
	}
	if y1 > b.Bottom {
		y1 = b.Bottom
	}
	if y0 == y1 {
		return
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	for y := y0; y <= y1; y++ {
		s.add(x, y)
	}
}

// baselinePolygonSpans preserves 2065's vertical-boundary closure for rejected edges.
// Merely clipping surviving edges leaves holes in partially offscreen polygons.

func baselinePolygonSpansWith(points []Point, b Rect, clip edgeClipper) (baselineSpans, error) {
	s := baselineNewSpans(b.Top, b.Bottom)
	if len(points) < 2 {
		return s, nil
	}
	for i, oa := range points {
		ob := points[(i+1)%len(points)]
		var a, z Point
		var ok bool
		var reason int
		var err error
		if clip == nil {
			a, z, ok, reason, err = ClipLine(oa, ob, b)
		} else {
			a, z, ok, reason, err = clip(i, oa, ob, b)
		}
		if err != nil {
			return s, err
		}
		if !ok {
			if reason >= 0 {
				s.vertical(reason, oa.Y, ob.Y, b)
			}
			continue
		}
		boundary := func(x int) bool { return x == b.Left || x == b.Right }
		if a.X != oa.X {
			if boundary(a.X) {
				s.vertical(a.X, a.Y, oa.Y, b)
			}
			if z.X != ob.X && boundary(z.X) {
				s.vertical(z.X, z.Y, ob.Y, b)
			}
		} else if z.X != ob.X && boundary(z.X) {
			// 20c5 reloads AX from the second clipped X before20ec tests
			// its boundary. Testing the unchanged first X leaves holes.
			s.vertical(z.X, z.Y, ob.Y, b)
		}
		// Source 2165 orders endpoints by Y before choosing its scan leaf.
		if a.Y > z.Y {
			a, z = z, a
		}
		visitLine(a, z, s.add)
	}
	return s, nil
}

// Convenience for legacy row-inspection tests.
func polygonSpans(points []Point, b Rect) (spans, error) {
	var s spans
	err := s.polygon(points, b, nil)
	return s, err
}

func TestPolygonWorkspaceAgainstV32(t *testing.T) {
	rng := rand.New(rand.NewSource(432))
	var got spans
	injected := errors.New("clipping failed")
	for i := 0; i < 1400; i++ {
		width := []int{320, 640, 800, 1024, 1280}[i%5]
		height := []int{200, 480, 600, 768, 1024}[i%5]
		top, left := rng.Intn(height/2), rng.Intn(width/2)
		b := Rect{left, top, left + rng.Intn(width-left), top + rng.Intn(height-top)}
		pts := make([]Point, i%11)
		for j := range pts {
			pts[j] = Point{rng.Intn(4*width) - width, rng.Intn(4*height) - height}
		}
		if i%9 == 0 {
			pts = []Point{{b.Left - 1, b.Top - 1}, {b.Left, b.Top}, {b.Left, b.Top}, {b.Right + 5, b.Bottom + 5}}
		}
		var clip edgeClipper
		if i%3 != 0 {
			clip = func(edge int, a, z Point, b Rect) (Point, Point, bool, int, error) {
				if edge == 2 && i%7 == 0 {
					return a, z, false, -1, injected
				}
				return clipLineTarget(a, z, b, width != 320)
			}
		}
		for reverse := 0; reverse < 2; reverse++ {
			want, e := baselinePolygonSpansWith(pts, b, clip)
			err := got.polygon(pts, b, clip)
			if (err == nil) != (e == nil) || err != nil && err.Error() != e.Error() {
				t.Fatalf("case %d errors %v %v", i, err, e)
			}
			for y := b.Top; y <= b.Bottom; y++ {
				if got.left[y] != want.left[y] || got.right[y] != want.right[y] {
					t.Fatalf("case %d reverse %d row %d", i, reverse, y)
				}
			}
			for a, z := 0, len(pts)-1; a < z; a, z = a+1, z-1 {
				pts[a], pts[z] = pts[z], pts[a]
			}
		}
	}
}

func TestNestedPolygonRows(t *testing.T) {
	b := FullView().Bounds
	outer := []Point{{-10, 41}, {190, 37}, {305, 130}, {30, 900}}
	inner := []Point{{80, 1}, {301, 70}, {0, 120}}
	got, want := make([]byte, Width*Height), make([]byte, Width*Height)
	// The clip callback enters a separate polygon/disc draw after the outer
	// workspace has accumulated its first edge. Both draws read destination ink.
	insert := func(dst []byte) {
		if _, err := FillPolygon(dst, inner, 0x6060, b, 0, nil); err != nil {
			t.Fatal(err)
		}
		fillDisc(dst, Point{49, 42}, 12, 0x6400, b, 0, nil)
	}
	insert(want)
	if err := MatchPolygon(want, outer, 0x601a, b); err != nil {
		t.Fatal(err)
	}
	clip := func(i int, a, z Point, b Rect) (Point, Point, bool, int, error) {
		if i == 1 {
			insert(got)
		}
		return ClipLine(a, z, b)
	}
	var rows spans
	if e := rows.polygon(outer, b, clip); e != nil {
		t.Fatal(e)
	}
	for y := b.Top; y <= b.Bottom; y++ {
		for x := max(rows.left[y], b.Left); x <= min(rows.right[y], b.Right); x++ {
			off := y*Width + x
			if got[off] == 0x60 {
				got[off] = 0x1a
			}
		}
	}
	if !bytes.Equal(got, want) {
		t.Fatal("nested draw disturbed outer row endpoints")
	}
}
