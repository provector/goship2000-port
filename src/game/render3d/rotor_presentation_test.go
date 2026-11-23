package render3d

import (
	"encoding/json"
	"gunship2000-assets/game/world"
	"reflect"
	"testing"
)

func TestRingRotorPresentationIsInstanceLocalAndCanonicalPure(t *testing.T) {
	b := tinyBundle()
	g := b.Geometries["model"]
	g.Vertices = make([]world.Vertex, 12)
	g.Vertices[0] = world.Vertex{100, 7, 0}
	g.Vertices[8] = world.Vertex{17, 23, 42}
	r, _ := New(b)
	scene := Scene{Instances: []Instance{{ID: "running", ModelID: 0, Callback: 1}, {ID: "stopped", ModelID: 0, Callback: 1}, {ID: "scenery", ModelID: 0, Callback: 1}}}
	source := VisualState{Phase: 9, streams: map[string][]world.Vertex{"model": append([]world.Vertex(nil), g.Vertices...)}}
	source.streams["model"][0] = world.Vertex{-99, 7, 0}
	before, _ := json.Marshal(source)
	saved := source.Clone()
	immutable := append([]world.Vertex(nil), g.Vertices...)
	for _, phase := range []float64{0, 1, 1.5, 2, 12} {
		out, ok := r.NativeRingRotors(scene, &source, phase, map[string]bool{"running": true, "stopped": false})
		if !ok || len(out.instanceVertices) != 2 {
			t.Fatal("missing instance-local presentation")
		}
		if got := out.instanceVertices["stopped"]; !reflect.DeepEqual(got, g.Vertices) {
			t.Fatal("stopped rotor inherited running model arena")
		}
		if out.instanceVertices["running"][8] != g.Vertices[8] {
			t.Fatal("non-rotor vertex changed")
		}
		if phase == 1 && out.instanceVertices["running"][0] != (world.Vertex{87, 7, 50}) {
			t.Fatal("named thirty-degree step", out.instanceVertices["running"][0])
		}
		if phase == 1.5 && out.instanceVertices["running"][0] != (world.Vertex{71, 7, 71}) {
			t.Fatal("smooth fractional step")
		}
		if phase == 12 && out.instanceVertices["running"][0] != g.Vertices[0] {
			t.Fatal("periodic native pose drift")
		}
		out.instanceVertices["running"][0][0] = 1234
	}
	after, _ := json.Marshal(source)
	if string(before) != string(after) || !reflect.DeepEqual(source.streams, saved.streams) || !reflect.DeepEqual(g.Vertices, immutable) {
		t.Fatal("presentation changed canonical state or prepared vertices")
	}
}

func TestNativeRotorPrefixAndUnknownBoundaries(t *testing.T) {
	b := tinyBundle()
	g := b.Geometries["model"]
	g.Vertices = make([]world.Vertex, 12)
	g.Vertices[9] = world.Vertex{100, 7, 0}
	g.Vertices[10] = world.Vertex{100, 7, 0}
	r, _ := New(b)
	source := VisualState{}
	scene := Scene{Instances: []Instance{{ID: "ten", ModelID: 0, Callback: 2}, {ID: "disabled", ModelID: 0, Callback: 0}, {ID: "bad", ModelID: 999, Callback: 1}}}
	out, ok := r.NativeRingRotors(scene, &source, 1, map[string]bool{"ten": true, "disabled": true, "bad": true})
	if !ok || len(out.instanceVertices) != 1 || out.instanceVertices["ten"][9] != (world.Vertex{87, 7, 50}) || out.instanceVertices["ten"][10] != g.Vertices[10] {
		t.Fatal("source ten-vertex prefix or disabled/invalid gate")
	}
}
