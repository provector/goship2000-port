package render3d

import (
	"bytes"
	"encoding/json"
	"gunship2000-assets/game/world"
	"reflect"
	"testing"
)

func opaqueWallFixture(t *testing.T) (*Renderer, Scene, Instance) {
	t.Helper()
	b := tinyBundle()
	g := &world.Geometry{ID: "wall", Root: -1, Extent: 144,
		Vertices: []world.Vertex{{-100, -100, 0}, {100, -100, 0}, {100, 100, 0}, {-100, 100, 0}},
		// Metadata admits a scenery mesh; the ray must use the actual face,
		// not infer an enclosing volume from this unrelated support footprint.
		Heights:      []world.HeightRegion{{Points: [][2]int16{{-10, -10}, {-10, 10}, {10, 10}, {10, -10}, {-10, -10}}, Plane: world.Plane{0, 1, 0, -200}}},
		DirectGroups: []world.Group{{Primitive: &world.Primitive{Opcode: 4, Colour: 0x3333, Indices: []int{0, 1, 2, 3}}}},
	}
	b.Models = append(b.Models, "wall")
	b.Geometries["wall"] = g
	r, err := New(b)
	if err != nil {
		t.Fatal(err)
	}
	base := uint32(200000)
	focus := Instance{ID: "viewed", Pose: Pose{H0: base, H1: base - 200, Height: 100}}
	w := Instance{ID: "wall-instance", ModelID: 1, Pose: Pose{H0: base, H1: base, Height: 100}}
	s := Scene{World: "GULF5", View: FullView(), Camera: Camera{Pose: Pose{H0: base, H1: base + 200, Height: 100}, Angles: [3]uint16{0, 0x8000, 0}}, Instances: []Instance{w}}
	return r, s, focus
}

func TestObserverActualOpaqueFaceAndSourcePurity(t *testing.T) {
	r, scene, focus := opaqueWallFixture(t)
	g := r.Assets.Geometries["wall"]
	beforeGeometry, _ := json.Marshal(g)
	beforeVertices := append([]world.Vertex(nil), g.Vertices...)
	beforeScene := scene
	beforeScene.Instances = append([]Instance(nil), scene.Instances...)
	got := r.ObserverCamera(scene, focus)
	// Both endpoints are outside the ten-unit height footprint, but the actual
	// wall crosses the sightline halfway. Leave32 units before its z=200000.
	if got.H0 != 200000 || got.H1 != 200000-32 || got.Height != scene.Camera.Height || got.Angles != scene.Camera.Angles {
		t.Fatal("opaque face segment did not preserve32-unit clearance", got)
	}
	afterGeometry, _ := json.Marshal(g)
	if !bytes.Equal(beforeGeometry, afterGeometry) || !reflect.DeepEqual(beforeVertices, g.Vertices) || !reflect.DeepEqual(beforeScene, scene) {
		t.Fatal("private observer changed source geometry or scene")
	}
	// Explicit aliases and world translations must keep the same ownership.
	scene.Instances[0].GeometryID, scene.Instances[0].ModelID = "wall", 999
	if r.ObserverCamera(scene, focus) != got {
		t.Fatal("explicit geometry alias changed the actual face")
	}
	shift := func(p Pose) Pose { p.H0 += 30000; p.H1 += 40000; p.Height += 100; return p }
	scene.Camera.Pose = shift(scene.Camera.Pose)
	scene.Instances[0].Pose, focus.Pose = shift(scene.Instances[0].Pose), shift(focus.Pose)
	got.Pose = shift(got.Pose)
	if moved := r.ObserverCamera(scene, focus); moved != got {
		t.Fatal("world origin/altitude changed observer clearance", moved, got)
	}
}

func TestObserverUsesActualDrawOrientation(t *testing.T) {
	for _, callback := range []uint8{0, 1, 2, 3, 4, 5, 6, 7} {
		r, scene, focus := opaqueWallFixture(t)
		scene.Instances[0].Angles[1] = 0x4000 // stale for ordinary unoriented0
		scene.Instances[0].Callback = callback
		// A rotated local-Z wall becomes worldX=base. The path crosses it;
		// the ordinary unrotated mesh instead lies coplanar with this path.
		focus.H0, focus.H1 = 200000-200, 200000
		scene.Camera.H0, scene.Camera.H1 = 200000+200, 200000
		got := r.ObserverCamera(scene, focus)
		forced := callback == 1 || callback == 2 || callback == 4 || callback == 6
		if forced && (got.H0 != 200000-32 || got.H1 != scene.Camera.H1) {
			t.Fatal("draw-forced callback rotation omitted", callback, got)
		}
		if !forced && got != scene.Camera {
			t.Fatal("unoriented stale heading rotated the actual mesh", callback, got)
		}
	}
	// On the original Z crossing, stale heading must likewise leave the raw
	// wall in place, rather than incorrectly rotating it out of the ray.
	r, scene, focus := opaqueWallFixture(t)
	scene.Instances[0].Angles[1] = 0x4000
	if got := r.ObserverCamera(scene, focus); got.H1 != 200000-32 {
		t.Fatal("ordinary mesh used raw angles despite disabled orientation", got)
	}
}

func TestObserverDoesNotInventOpaqueScenery(t *testing.T) {
	for _, name := range []string{"no-height-admission", "transparent-zero", "remap", "line", "unknown-model", "unknown-alias", "far-wrap", "viewed-model", "particle", "explosion", "generator", "compact", "invalid-index"} {
		t.Run(name, func(t *testing.T) {
			r, scene, focus := opaqueWallFixture(t)
			g := r.Assets.Geometries["wall"]
			switch name {
			case "no-height-admission":
				g.Heights = nil
			case "transparent-zero":
				g.DirectGroups[0].Primitive.Colour = 0x0100
			case "remap":
				g.DirectGroups[0].Primitive.Colour = 0xffff
			case "line":
				g.DirectGroups[0].Primitive.Opcode = 2
			case "unknown-model":
				scene.Instances[0].ModelID = 999
			case "unknown-alias":
				scene.Instances[0].GeometryID = "missing"
			case "far-wrap":
				scene.Instances[0].H0 += 65536
			case "viewed-model":
				scene.Instances[0].ID = focus.ID
			case "particle":
				scene.Instances[0].Impact = true
			case "explosion":
				scene.Instances[0].Explosion = true
			case "generator":
				g.Generator = &world.Generator{}
			case "compact":
				g.CompactCallback = 1
			case "invalid-index":
				g.DirectGroups[0].Primitive.Indices[1] = 999
			}
			if got := r.ObserverCamera(scene, focus); got != scene.Camera {
				t.Fatal("unproven/nonopaque mesh changed the view", got)
			}
		})
	}
}

func TestObserverClosestFaceAndNoCollapse(t *testing.T) {
	r, scene, focus := opaqueWallFixture(t)
	nearer := scene.Instances[0]
	nearer.ID, nearer.H1 = "nearer-to-focus", 200000-80
	farther := scene.Instances[0]
	scene.Instances = []Instance{farther, nearer}
	a := r.ObserverCamera(scene, focus)
	scene.Instances = []Instance{nearer, farther}
	b := r.ObserverCamera(scene, focus)
	if a != b || a.H1 != 200000-112 {
		t.Fatal("nearest actual face depends on registration order", a, b)
	}
	scene.Instances = []Instance{farther}
	focus.H1 = 200000 - 16 // only16 units before the face, cannot leave32
	if got := r.ObserverCamera(scene, focus); got != scene.Camera {
		t.Fatal("insufficient positive orbit must retain established camera", got)
	}
}

func TestObserverUnobstructedCloseGeometryPixelsUnchanged(t *testing.T) {
	r, scene, focus := opaqueWallFixture(t)
	g := r.Assets.Geometries["wall"]
	for i := range g.Vertices {
		if g.Vertices[i][0] < 0 {
			g.Vertices[i][0] = -20
		} else {
			g.Vertices[i][0] = 20
		}
	}
	scene.Instances[0].H0 += 40 // close visible face beside, not across the ray
	a := make([]byte, scene.TargetPixels())
	b := make([]byte, len(a))
	stats, err := r.Draw(scene, &VisualState{}, a)
	if err != nil || stats.Instances == 0 {
		t.Fatal("close geometry control did not draw", stats, err)
	}
	without := scene
	without.Instances = nil
	omitted := make([]byte, len(a))
	if _, err = r.Draw(without, &VisualState{}, omitted); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, omitted) {
		t.Fatal("legitimate close face is not represented in control pixels")
	}
	clearScene := scene
	clearScene.Camera = r.ObserverCamera(scene, focus)
	if _, err = r.Draw(clearScene, &VisualState{}, b); err != nil {
		t.Fatal(err)
	}
	if clearScene.Camera != scene.Camera || !bytes.Equal(a, b) {
		t.Fatal("legitimate unobstructed close-up changed")
	}
}
