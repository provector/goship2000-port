package render3d

import "testing"

func TestPhysicalScaleClipDoesNotSaturateLogicalDelta(t *testing.T) {
	// A valid320-domain edge has DX30272. In physical640 coordinates DX60544
	// is not a source signed-word quantity. Saturating it to32767 bends the
	// left intersection toY-9949, then closes the polygon down the wrong side.
	// Hand division:12876-trunc(25024*29888/60544)=523, below bottom399;
	// bottom intersection X=-29888+trunc(60544*12477/25024)=299;
	// right intersection Y=12876-trunc(25024*30527/60544)=259.
	a, z, ok, _, err := clipLineTarget(Point{-29888, 12876}, Point{30656, -12148}, Rect{0, 0, 639, 399}, true)
	if err != nil || !ok || a != (Point{299, 399}) || z != (Point{639, 259}) {
		t.Fatalf("physical clip %v %v ok%v err%v", a, z, ok, err)
	}
	a, z, ok, _, err = clipLineTarget(Point{30656, -12148}, Point{-29888, 12876}, Rect{0, 0, 639, 399}, true)
	// Original-endpoint/truncation-toward-zero arithmetic is direction-sensitive:
	// reverse Y=-12148+trunc(25024*30017/60544)=258,
	// reverse X=30656-trunc(60544*12547/25024)=300.
	if err != nil || !ok || a != (Point{639, 258}) || z != (Point{300, 399}) {
		t.Fatalf("reverse physical clip %v %v ok%v err%v", a, z, ok, err)
	}
}

func TestPhysicalProjectionRetainsEdgeDirection(t *testing.T) {
	f := frame{scene: Scene{RenderScale: 2}, view: FullView()}
	f.view.Center = Point{320, 140}
	p, err := f.project(Vector{2000 << 16, 0, 8 << 16}, true)
	// Physical projection:320+(2000*65536*2)/(8*256)=128320.
	if err != nil || p != (Point{128320, 140}) {
		t.Fatalf("premature physical narrowing %v %v", p, err)
	}
	// The reference320 path retains the original installed overflow handler.
	p, err = project(Vector{2000 << 16, 0, 8 << 16}, Point{160, 70})
	if err != nil || p != (Point{32160, 70}) {
		t.Fatalf("source projection changed %v %v", p, err)
	}
}

func TestBankedPhysicalPolygonKeepsGroundSide(t *testing.T) {
	// The two near-plane points bracket X by60544 physical pixels. A planar
	// ground face projects below the line through its two far corners
	// (675,184),(101,422): atX320 the boundary is aboutY331. This expectation
	// comes from the input plane/hand line equation, not a saved port image.
	points := []Point{{30848, -11636}, {675, 184}, {101, 422}, {-29696, 13388}}
	dst := make([]byte, 640*400)
	if _, err := FillPolygon(dst, points, 0x3333, Rect{0, 0, 639, 399}, 0, nil); err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		x, y int
		want byte
	}{{320, 100, 0}, {320, 300, 0}, {320, 380, 0x33}, {620, 390, 0x33}, {20, 390, 0}} {
		if got := dst[q.y*640+q.x]; got != q.want {
			t.Fatalf("banked ground at%d,%d=%x want%x", q.x, q.y, got, q.want)
		}
	}
}
