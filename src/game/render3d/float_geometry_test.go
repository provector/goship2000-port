package render3d

import (
	"gunship2000-assets/game/world"
	"math"
	"slices"
	"testing"
)

func TestFractionalProjectionAndCrossings(t *testing.T) {
	f := frame{scene: Scene{Resolution: "1280x1024"}}
	f.view = f.scene.rasterView()
	f.floatView = floatRotation([3]uint16{}, [3]float64{}, true)
	var previous float64
	for i := 0; i < 5; i++ {
		f.scene.CameraFraction.Position[0] = float64(i) / 10
		p := f.projectFloat(f.floatTranslation(world.Vertex{20, 0, 128}, ""), 0)
		want := 640 + (20-float64(i)/10)*4
		if math.Abs(p[0]-want) > 1e-10 || i > 0 && previous <= p[0] {
			t.Fatal(p, want)
		}
		t.Logf("camera +%.1f world units: projected x %.3f, raster x %d", float64(i)/10, p[0], floatPixel(p).X)
		previous = p[0]
	}
	// A single face crosses BOTH depth planes and all four viewport edges.
	w := floatWorkspace{polygon: []floatVertex{{-400, -400, -2}, {400, -400, 64}, {400, 400, 9000}, {-400, 400, 64}}}
	for _, p := range []clipPlane{{2, 8, true}, {2, 8192, false}} {
		w.clip(p)
	}
	for i, v := range w.polygon {
		if v[2] < 8 || v[2] > 8192 {
			t.Fatal(v)
		}
		w.polygon[i] = f.projectFloat(v, 0)
	}
	b := f.view.Bounds
	for _, p := range []clipPlane{{0, float64(b.Left), true}, {0, float64(b.Right), false}, {1, float64(b.Top), true}, {1, float64(b.Bottom), false}} {
		w.clip(p)
	}
	if len(w.polygon) < 3 {
		t.Fatal("visible clipped polygon disappeared")
	}
	for _, v := range w.polygon {
		if !(v[0] >= float64(b.Left) && v[0] <= float64(b.Right) && v[1] >= float64(b.Top) && v[1] <= float64(b.Bottom)) {
			t.Fatal("outside or nonfinite", v)
		}
	}
}

func TestModelPanelOriginAndOrientation(t *testing.T) {
	r, scene := previewFixture(t)
	scene.Resolution = "800x600"
	scene.FloatingGeometry = true
	scene.CameraFraction.Position = [3]float64{.25, -.375, .125}
	model := ModelPreview{ModelID: 0, Translation: world.Vertex{0, 0, 32}, ID: "tracked"}
	pixels := make([]byte, scene.TargetPixels())
	draw := func() {
		clear(pixels)
		_, err := r.DrawModelPreview(scene, &VisualState{}, pixels, model)
		if err != nil {
			t.Fatal(err)
		}
	}
	draw()
	layout := scene.Layout()
	for _, x := range []int{130, 160, 190} {
		if pixels[layout.Y[70]*layout.Width+layout.X[x]] != 9 {
			t.Fatal("lost model face", x)
		}
	}
	first := slices.Clone(pixels)
	scene.CameraFraction.Position = [3]float64{-.375, .25, -.25}
	draw()
	if !slices.Equal(first, pixels) {
		t.Fatal("world translation leaked into model-only panel")
	}
	model.Angles[1] += 8192
	draw()
	if slices.Equal(first, pixels) {
		t.Fatal("model panel ignored rotation")
	}
}
