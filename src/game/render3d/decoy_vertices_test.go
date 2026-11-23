package render3d

import (
	"bytes"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func decoyGeometries(t *testing.T) [2]*world.Geometry {
	t.Helper()
	raw, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	g, err := world.PlayerDecoyGeometries(raw)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestDecoyCallbacksExactArithmeticAndCacheIsolation(t *testing.T) {
	g := decoyGeometries(t)
	matrices := []Matrix{{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}, {}, {{0, 0, 1}, {0, 1, 0}, {-1, 0, 0}}}
	wants := [][]Vector{
		{{0, 32, 0}, {8, 0, 8}, {8, 0, -8}, {-8, 0, 8}, {-9, -1, -9}},
		{{}, {}, {}, {}, {-1, -1, -1}},
		{{0, 32, 0}, {-8, 0, 8}, {8, 0, 8}, {-8, 0, -8}, {7, -1, -9}},
	}
	f := frame{compact: map[int][]Vector{1: {{12345, 67890, -77}}, 5: {{88, 99, 100}}}}
	for i, matrix := range matrices {
		if got := f.verticesForInstance(g[0], matrix, Vector{}, "flare"); !reflect.DeepEqual(got, wants[i]) {
			t.Fatalf("basis%d got%v want%v", i, got, wants[i])
		}
		translation := Vector{1000, -2000, 3000}
		got := f.verticesForInstance(g[1], matrix, translation, "chaff")
		if len(got) != 12 {
			t.Fatal("chaff count", len(got))
		}
		for n, v := range g[1].Vertices {
			for j := range v {
				if got[n][j] != int32(v[j])<<16+translation[j] {
					t.Fatal("chaff rotated or not high-word copy", n, got[n])
				}
			}
		}
	}
	if !reflect.DeepEqual(f.compact[1], []Vector{{12345, 67890, -77}}) || !reflect.DeepEqual(f.compact[5], []Vector{{88, 99, 100}}) {
		t.Fatal("embedded preparation changed world compact cache")
	}
}

func TestDecoySourceLODClippingAndRedrawPurity(t *testing.T) {
	geometries := decoyGeometries(t)
	b := tinyBundle()
	for _, g := range geometries {
		b.Geometries[g.ID] = g
	}
	r, err := New(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, scale := range []int{1, 2} {
		for _, g := range geometries {
			scene := Scene{World: "GULF5", RenderScale: scale, Camera: Camera{Pose: Pose{H0: 200000, H1: 200000, Height: 512}}, View: FullView(),
				Instances: []Instance{{ID: "decoy", GeometryID: g.ID, Pose: Pose{H0: 200000, H1: 200256, Height: 512}}}}
			baselineScene := scene
			baselineScene.Instances = nil
			plain, painted := make([]byte, scene.TargetPixels()), make([]byte, scene.TargetPixels())
			state := &VisualState{}
			if _, err = r.Draw(baselineScene, state, plain); err != nil {
				t.Fatal(err)
			}
			stats, err := r.Draw(scene, state, painted)
			if err != nil || stats.Instances != 1 || stats.Primitives < 2 || bytes.Equal(plain, painted) {
				t.Fatal("visible descriptor not drawn", scale, g.ID, stats, err)
			}
			before := *state // nil cache identity is also part of this purity check.
			for n := 0; n < 100; n++ {
				next := make([]byte, scene.TargetPixels())
				if _, err = r.Draw(scene, state, next); err != nil || !bytes.Equal(painted, next) {
					t.Fatal("redraw changed image", n, err)
				}
			}
			if !reflect.DeepEqual(state, &before) {
				t.Fatal("redraw changed visual owner")
			}
			// At near-reference equality the descriptor's far pointer is0.
			scene.Instances[0].H1 = scene.Camera.H1 + 8192
			stats, err = r.Draw(scene, state, painted)
			if err != nil || stats.Instances != 0 {
				t.Fatal("zero far-LOD not respected", stats, err)
			}
			// A source extent0 effect behind the camera is ordinarily culled.
			scene.Instances[0].H1 = scene.Camera.H1 - 256
			stats, err = r.Draw(scene, state, painted)
			if err != nil || stats.Instances != 0 {
				t.Fatal("behind camera effect forced visible", stats, err)
			}
		}
	}
}

func TestChaffSourceF000PreservesDestinationAtBothScales(t *testing.T) {
	for _, scale := range []int{1, 2} {
		dst := bytes.Repeat([]byte{0x66}, Width*Height*scale*scale)
		points := []Point{{80 * scale, 40 * scale}, {120 * scale, 40 * scale}, {100 * scale, 80 * scale}}
		fallback, err := FillPolygon(dst, points, 0xf000, Rect{0, 0, Width*scale - 1, Height*scale - 1}, 1, nil)
		if err != nil || fallback {
			t.Fatal("source low-zero material used wrong branch", scale, fallback, err)
		}
		ink, retained := 0, 0
		for y := 45 * scale; y <= 60*scale; y++ {
			for x := 96 * scale; x <= 104*scale; x++ {
				switch dst[y*Width*scale+x] {
				case 0xf0:
					ink++
				case 0x66:
					retained++
				default:
					t.Fatal("material invented an opaque low colour/hatch", scale, dst[y*Width*scale+x])
				}
			}
		}
		if ink == 0 || retained == 0 {
			t.Fatal("chaff lost source destination-preserving stipple", scale, ink, retained)
		}
	}
}

func TestDecoysUseTerrainOcclusionAndNearClipping(t *testing.T) {
	geometries := decoyGeometries(t)
	b := tinyBundle()
	cell := b.Geometries["cell"]
	cell.Vertices = []world.Vertex{{-4096, 0, -4096}, {4096, 0, -4096}, {4096, 0, 4096}, {-4096, 0, 4096}}
	cell.PlaneGroups = []world.Group{{Primitive: &world.Primitive{Opcode: 4, Colour: 0x3333, Indices: []int{0, 1, 2, 3}}}}
	for _, g := range geometries {
		b.Geometries[g.ID] = g
	}
	r, err := New(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, scale := range []int{1, 2} {
		scene := Scene{World: "GULF5", RenderScale: scale, Camera: Camera{Pose: Pose{H0: 200000, H1: 200000, Height: 512}}, View: FullView()}
		plain, with := make([]byte, scene.TargetPixels()), make([]byte, scene.TargetPixels())
		if _, err := r.Draw(scene, &VisualState{}, plain); err != nil {
			t.Fatal(err)
		}
		for _, g := range geometries {
			// Every indexed vertex remains below the ground plane. The actor
			// enters the ordinary behind-plane leaf before its terrain face.
			scene.Instances = []Instance{{ID: "below-ground", GeometryID: g.ID, Pose: Pose{H0: 200000, H1: 204096, Height: 0xff00}}}
			stats, err := r.Draw(scene, &VisualState{}, with)
			if err != nil || stats.Instances != 1 || !bytes.Equal(plain, with) {
				t.Fatal("effect bypassed ordinary terrain occlusion", g.ID, scale, stats, err)
			}
			// A close camera may intersect the source triangles. The common
			// near-plane clipper owns that path; the effect has no overlay escape.
			scene.Instances[0].ID = "near-plane"
			scene.Instances[0].Pose = Pose{H0: scene.Camera.H0, H1: scene.Camera.H1 + 32, Height: scene.Camera.Height}
			stats, err = r.Draw(scene, &VisualState{}, with)
			if err != nil || stats.Instances != 1 || stats.ArithmeticRejected != 0 {
				t.Fatal("near-plane effect did not use bounded shared clipping", g.ID, scale, stats, err)
			}
		}
	}
}
