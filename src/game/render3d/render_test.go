package render3d

import (
	"bytes"
	"gunship2000-assets/game/world"
	"reflect"
	"testing"
)

func testQuarter() []int16 {
	q := make([]int16, 1025)
	q[0] = 0
	q[512] = 11585
	q[1024] = 16384
	return q
}

func TestSourceCoordinateHandCases(t *testing.T) {
	q := testQuarter()
	cases := []struct {
		a           [3]uint16
		v           world.Vertex
		transformed Vector
		p           Point
	}{
		{[3]uint16{}, world.Vertex{32, 16, 128}, Vector{262144, 131072, 2097152}, Point{192, 54}},
		{[3]uint16{0, 0, 0x4000}, world.Vertex{32, 16, 128}, Vector{-131072, 262144, 2097152}, Point{144, 38}},
		{[3]uint16{0x4000, 0, 0}, world.Vertex{32, 128, 16}, Vector{262144, -131072, 2097152}, Point{192, 86}},
		{[3]uint16{0, 0, 0x2000}, world.Vertex{32, 16, 128}, Vector{92656, 278016, 2097152}, Point{171, 37}},
	}
	for _, c := range cases {
		v := Transform(c.v, Rotation(c.a, q, true))
		if v != c.transformed {
			t.Fatalf("angles%v got%v expected%v", c.a, v, c.transformed)
		}
		p, e := project(v, Point{160, 70})
		if e != nil || p != c.p {
			t.Fatalf("projection %v %v", p, e)
		}
	}
	if planeSide(world.Plane{0, 0, 0, -131072}, world.Vertex{}) != 0 || planeSide(world.Plane{0, 0, 0, -131073}, world.Vertex{}) != -1 {
		t.Fatal("plane classification dropped source20000 bias")
	}
	if got := CellIndex(299008, 503808); got != 164 {
		t.Fatalf("raw grid index%d, expected164 (RAM178)", got)
	}
}

func TestInstalledProjectionExceptionHandCases(t *testing.T) {
	// Original1ff5 installs INT0=1f8c, application-entry00f4. For
	// 308480/8 the normal word quotient38560 faults;1fd2 shifts the
	// numerator8, obtains150, and the >64 branch returns7d00=32000.
	for _, c := range []struct {
		n int32
		d int16
		q int
	}{{308480, 8, 32000}, {-308480, 8, -32000}, {32767, 1, 32767}, {-32768, 1, -32768}, {-32768, -1, 32000}} {
		q, err := projectionQuotient(c.n, c.d)
		if err != nil || q != c.q {
			t.Fatalf("projection INT0 %d/%d=%d,%v", c.n, c.d, q, err)
		}
	}
	if _, err := projectionQuotient(1, 0); err == nil {
		t.Fatal("zero divisor must remain an error")
	}
	if intoWord(32760+20) != 32767 || intoWord(-32760-20) != -32768 {
		t.Fatal("installed INT4 signed endpoints")
	}
	p, err := project(Vector{78970880, -4194304, 524288}, Point{160, 70})
	if err != nil || p != (Point{32160, 2118}) {
		t.Fatalf("first observed overflowing terrain projection=%v,%v", p, err)
	}
}

func TestOrientedPlaneCameraUsesRows(t *testing.T) {
	// Resident163c transposes the object matrix, then14e6 reads each
	// contiguous row. A quarter turn must differ from a column product.
	f := frame{r: &Renderer{Assets: &world.Bundle{Sine: testQuarter()}}, matrix: Rotation([3]uint16{}, testQuarter(), true)}
	g := &world.Geometry{Extent: 100}
	item, ok := f.item(g, world.Vertex{32, 16, 128}, [3]uint16{0, 0, 0x4000}, true, 1, "hand")
	if !ok || item.localView != (world.Vertex{16, -32, -128}) {
		t.Fatalf("oriented plane camera %v visible=%v", item.localView, ok)
	}
}

func TestSourceClippingHandCases(t *testing.T) {
	b := Rect{0, 0, 319, 199}
	for _, c := range []struct{ a, z, x, y Point }{{Point{-7, -3}, Point{8, 6}, Point{0, 1}, Point{8, 6}}, {Point{-4, -3}, Point{8, 5}, Point{0, 0}, Point{8, 5}}, {Point{0, 0}, Point{319, 199}, Point{0, 0}, Point{319, 199}}} {
		a, z, ok, _, e := ClipLine(c.a, c.z, b)
		if e != nil || !ok || a != c.x || z != c.y {
			t.Fatalf("clip %v %v =>%v %v %v %v", c.a, c.z, a, z, ok, e)
		}
	}
	points := []Vector{{-3 << 16, 0, 4 << 16}, {5 << 16, 0, 12 << 16}, {5 << 16, 4 << 16, 12 << 16}}
	got, e := clipDepthPolygon(points, 8, 8192)
	if e != nil {
		t.Fatal(e)
	}
	want := []Vector{{1 << 16, 0, 8 << 16}, {5 << 16, 0, 12 << 16}, {5 << 16, 4 << 16, 12 << 16}, {1 << 16, 2 << 16, 8 << 16}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("near-plane polygon%v", got)
	}
	dst := bytes.Repeat([]byte{0xa6}, 64000)
	_, e = FillPolygon(dst, []Point{{-4, 20}, {10, 20}, {10, 40}, {-4, 40}}, 0x3333, b, 0, nil)
	if e != nil {
		t.Fatal(e)
	}
	for y := 20; y <= 40; y++ {
		for x := 0; x <= 10; x++ {
			if dst[y*320+x] != 0x33 {
				t.Fatalf("missing closure pixel%d,%d", x, y)
			}
		}
	}
}

func TestSecondEndpointBoundaryClosure(t *testing.T) {
	// 20c5 MOV AX,[BX+4] feeds20ec: second-only clipping must add the
	// left boundary from clippedY139 to originalY2118 (clamped199).
	points := []Point{{79, 81}, {27, 90}, {-1088, 2118}, {8480, 2118}}
	spans, err := polygonSpans(points, FullView().Bounds)
	if err != nil {
		t.Fatal(err)
	}
	if spans.left[140] != 0 || spans.right[140] != 319 || spans.left[199] != 0 || spans.right[199] != 319 {
		t.Fatalf("second boundary closure140=%d..%d199=%d..%d", spans.left[140], spans.right[140], spans.left[199], spans.right[199])
	}
}

func TestMaterialOperationContexts(t *testing.T) {
	dst := bytes.Repeat([]byte{0x61}, 64000)
	fillSpan(dst, 2, 0, 5, 0x3432, 0, nil)
	if !bytes.Equal(dst[640:646], []byte{0x32, 0x34, 0x32, 0x34, 0x32, 0x34}) {
		t.Fatal("checker parity")
	}
	// K: VGA046f has the opposite phase to the ordinary checker leaf.
	fillSpan(dst, 3, 0, 3, 0x4400, 0, nil)
	if !bytes.Equal(dst[960:964], []byte{0x61, 0x44, 0x61, 0x44}) {
		t.Fatal("alternate destination preservation")
	}
	copy(dst, []byte{0, 0x5f, 0x60, 0x61, 0xff})
	fillSpan(dst, 0, 0, 4, 0, 0, nil)
	if !bytes.Equal(dst[:5], []byte{0, 0x5f, 0x61, 0x62, 0xfe}) {
		t.Fatal("0000 destination remap")
	}
	copy(dst, []byte{0x5f, 0x60, 0x6f, 0x70})
	fillSpan(dst, 0, 0, 3, 0xffff, 0, nil)
	if !bytes.Equal(dst[:4], []byte{0x5f, 0x18, 0x1f, 0x70}) {
		t.Fatal("ffff destination interval")
	}
	if !fillSpan(dst, 0, 0, 5, 0x98, 1, nil) {
		t.Fatal("missing Wreck carrier not diagnosed")
	}
	carrier := &Carrier{CS: make([]byte, 64000), DS: make([]byte, 64000)}
	if fillSpan(dst, 0, 0, 5, 0x98, 1, carrier) {
		t.Fatal("validated carrier should use source leaf")
	}
	if !bytes.Equal(dst[:6], []byte{0x9b, 0x98, 0x9b, 0x9b, 0x98, 0x9b}) {
		t.Fatalf("source feedback%02x", dst[:6])
	}
	copy(dst[3200:3205], []byte{0, 3, 0, 3, 0})
	if e := MatchPolygon(dst, []Point{{0, 10}, {4, 10}, {4, 11}, {0, 11}}, 3, FullView().Bounds); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(dst[3200:3205], []byte{3, 3, 3, 3, 3}) {
		t.Fatal("DEF opaque zero match0→3")
	}
}

func TestSourceDiscMidpointSpans(t *testing.T) {
	// 2275 radius8: pad1, d=-13; at x2 d=3 emits outer row8
	// with half-width2+1. This differs from a radius8 Euclidean circle.
	dst := bytes.Repeat([]byte{0xa6}, 64000)
	fillDisc(dst, Point{100, 100}, 8, 0x3333, FullView().Bounds, 0, nil)
	for _, p := range []Point{{91, 100}, {109, 100}, {97, 108}, {103, 108}, {94, 106}, {106, 106}} {
		if dst[p.Y*320+p.X] != 0x33 {
			t.Fatalf("missing source midpoint pixel %v", p)
		}
	}
	if dst[108*320+104] != 0xa6 || dst[100*320+110] != 0xa6 {
		t.Fatal("disc extended beyond source spans")
	}
}

func TestSharedEdgeCacheKeepsFirstRoundingAndOrientation(t *testing.T) {
	f := frame{edges: map[byte]screenEdge{}}
	b := FullView().Bounds
	first := f.edgeClipper([]byte{7}, []int{3})
	a, z, ok, _, err := first(0, Point{-4, -3}, Point{8, 5}, b)
	if err != nil || !ok || a != (Point{0, 0}) || z != (Point{8, 5}) {
		t.Fatalf("initial edge %v %v %v", a, z, err)
	}
	reverse := f.edgeClipper([]byte{7}, []int{9})
	a, z, ok, _, err = reverse(0, Point{8, 5}, Point{-4, -3}, b)
	if err != nil || !ok || a != (Point{8, 5}) || z != (Point{0, 0}) {
		t.Fatalf("cache orientation %v %v %v", a, z, err)
	}
	if len(f.edges) != 1 {
		t.Fatal("shared source edge identity duplicated")
	}
	uncached := f.edgeClipper([]byte{255}, []int{3})
	_, _, _, _, _ = uncached(0, Point{0, 0}, Point{1, 1}, b)
	if len(f.edges) != 1 {
		t.Fatal("FF sentinel created cache entry")
	}
}

func tinyBundle() *world.Bundle {
	tri := world.Group{Primitive: &world.Primitive{Opcode: 3, Colour: 0x3333, Indices: []int{0, 1, 2}}}
	g := &world.Geometry{ID: "cell", Vertices: []world.Vertex{{-4096, 0, -4096}, {4096, 0, -4096}, {0, 0, 4096}}, Planes: []world.Plane{{0, 16384, 0, 0}}, Children: [][2]int{{255, 255}}, Root: 0, PlaneGroups: []world.Group{tri}, Extent: 2500}
	w := &world.World{ID: "GULF5", Grid: make([]byte, 4096), Cells: make([]world.Cell, 64)}
	for i := range w.Cells {
		w.Cells[i] = world.Cell{ID: i, Geometry: "cell"}
	}
	model := &world.Geometry{ID: "model", Vertices: []world.Vertex{{-8, 0, 0}, {8, 0, 0}, {0, 16, 0}, {0, 0, 4}, {0, 0, 5}, {0, 0, 6}, {0, 0, 7}, {0, 0, 8}}, DirectGroups: []world.Group{tri}, Root: -1, Extent: 32}
	return &world.Bundle{Models: []string{"model"}, Geometries: map[string]*world.Geometry{"cell": g, "model": model}, Worlds: map[string]*world.World{"GULF5": w}, Sine: testQuarter()}
}

func TestDrawIsPureAndAnimationUsesOneFamilyOwner(t *testing.T) {
	b := tinyBundle()
	r, e := New(b)
	if e != nil {
		t.Fatal(e)
	}
	scene := Scene{World: "GULF5", Camera: Camera{Pose: Pose{H0: 200000, H1: 200000, Height: 512}}, View: FullView(), Instances: []Instance{{ID: "A", ModelID: 0, Pose: Pose{H0: 200000, H1: 204096}, Callback: 1}, {ID: "B", ModelID: 0, Pose: Pose{H0: 200050, H1: 204096}, Callback: 1}}}
	state := &VisualState{}
	dst1, dst2 := make([]byte, 64000), make([]byte, 64000)
	if _, e = r.Draw(scene, state, dst1); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Draw(scene, state, dst2); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(dst1, dst2) || state.Phase != 0 || len(state.streams) != 0 {
		t.Fatal("drawing advanced logical state")
	}
	before := append([]world.Vertex(nil), b.Geometries["model"].Vertices...)
	if e = r.Advance(scene, state, 1); e != nil {
		t.Fatal(e)
	}
	if state.Phase != 1 {
		t.Fatal("logical visual step not recorded")
	}
	got := state.SnapshotVertices("model")
	if len(got) == 0 {
		t.Fatal("source instance not visited")
	}
	if got[0] != before[1] {
		t.Fatal("shared family advanced more than once for two instances")
	}
	if !reflect.DeepEqual(b.Geometries["model"].Vertices, before) {
		t.Fatal("immutable prepared geometry mutated")
	}
	for i := 0; i < 144; i++ {
		if _, e = r.Draw(scene, state, dst1); e != nil {
			t.Fatal(e)
		}
	}
	if !reflect.DeepEqual(state.SnapshotVertices("model"), got) || state.Phase != 1 {
		t.Fatal("renderFPS changes animation")
	}
}

func TestSourceMutationAndCompactHandCases(t *testing.T) {
	v := make([]world.Vertex, 28)
	v[0] = world.Vertex{100, 0, 200}
	v[1] = world.Vertex{-32768, 0, 32767}
	var phase uint8
	mutate(v, 3, &phase)
	if v[0] != (world.Vertex{-14, 0, 223}) || v[1] != (world.Vertex{20775, 0, 11993}) {
		t.Fatal(v[:2])
	}
	wave := []world.Vertex{{0, 0, 1024}, {}}
	mutate(wave, 5, &phase)
	if wave[1][2] != 768 || phase != 1 {
		t.Fatal("wave source phase")
	}
	r, _ := New(tinyBundle())
	f := frame{r: r, matrix: Rotation([3]uint16{}, r.Assets.Sine, true), compact: map[int][]Vector{}}
	g := &world.Geometry{ID: "generated", CompactCallback: 1, Generator: &world.Generator{Kind: "command3", Shifts: [3]int{7, 4, 4}, ClusterDeltas: [][2]int16{{16, 0}, {-32, 0}, {-16, 16}, {0, -32}}}}
	points := f.vertices(g, f.matrix, Vector{})
	want := []Vector{{0, 1048576, 0}, {131072, 0, 262144}, {131072, 0, -262144}, {-131072, 0, 262144}, {-131073, -1, -262145}}
	if len(points) != 25 || !reflect.DeepEqual(points[:5], want) {
		t.Fatalf("generated%v", points)
	}
	if points[5] != (Vector{1048576, 1048576, 0}) || points[20] != (Vector{-2097152, 1048576, -1048576}) {
		t.Fatal("incremental high-word clusters")
	}
}

func TestSourceQueueShortcutAndTieOrder(t *testing.T) {
	ordinary := &world.Geometry{Planes: make([]world.Plane, 2)}
	one := &world.Geometry{Planes: make([]world.Plane, 1)}
	got := sourceQueueOrder([]drawItem{{geometry: ordinary, metric: 300, id: "far"}, {geometry: ordinary, metric: 200, id: "tie-A"}, {geometry: one, metric: 1, id: "one"}, {geometry: ordinary, metric: 200, id: "tie-B"}})
	var ids []string
	for _, item := range got {
		ids = append(ids, item.id)
	}
	if !reflect.DeepEqual(ids, []string{"one", "far", "tie-A", "tie-B"}) {
		t.Fatal(ids)
	}
}
