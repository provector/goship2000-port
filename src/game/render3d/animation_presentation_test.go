package render3d

import (
	"gunship2000-assets/game/world"
	"reflect"
	"testing"
)

func TestCallback3PolarHandCasesAndFirstMutationFallback(t *testing.T) {
	for _, q := range []struct{ before, after, want world.Vertex }{
		{world.Vertex{100, 7, 0}, world.Vertex{86, 7, 50}, world.Vertex{96, 7, 26}},
		{world.Vertex{0, 7, 100}, world.Vertex{-50, 7, 86}, world.Vertex{-26, 7, 96}},
		{world.Vertex{-100, 7, 0}, world.Vertex{-87, 7, -50}, world.Vertex{-97, 7, -26}},
	} {
		base := make([]world.Vertex, 28)
		base[0] = q.before
		cur := append([]world.Vertex(nil), base...)
		cur[0] = q.after
		r := &Renderer{Assets: &world.Bundle{Geometries: map[string]*world.Geometry{"component": {ID: "component", Vertices: base}}}}
		previous := VisualState{Phase: 0} // First source mutation had no private stream yet.
		current := VisualState{Phase: 1, streams: map[string][]world.Vertex{"component": cur}, rotations: map[string]bool{"component": true}}
		if got := r.SmoothVisual(&previous, &current, .5).streams["component"][0]; got != q.want {
			t.Fatal("source30degree endpoint/polar halfway case", q, got)
		}
		if got := r.SmoothVisual(&previous, &current, 0).streams["component"][0]; got != q.before {
			t.Fatal("alpha0 not exact prior source pose")
		}
		if got := r.SmoothVisual(&previous, &current, 1).streams["component"][0]; got != q.after {
			t.Fatal("alpha1 not exact current source pose")
		}
		out := r.SmoothVisual(&previous, &current, .5)
		out.streams["component"][0][0] = 123
		if cur[0] != q.after || base[0] != q.before {
			t.Fatal("presentation mutated source or prepared geometry")
		}
	}
}

func TestDiscreteVisualFamiliesAndCloneOwnership(t *testing.T) {
	before := VisualState{Phase: 3, streams: map[string][]world.Vertex{"ring": {{10, 2, 0}, {0, 2, 10}}, "swap": {{1, 2, 3}}}}
	current := VisualState{Phase: 4, streams: map[string][]world.Vertex{"ring": {{0, 2, 10}, {10, 2, 0}}, "swap": {{4, 5, 6}}}}
	r := &Renderer{}
	out := r.SmoothVisual(&before, &current, .5)
	if !reflect.DeepEqual(out.streams, current.streams) {
		t.Fatal("ring permutation/block swap was interpolated")
	}
	out.streams["ring"][0][0] = 99
	if current.streams["ring"][0][0] != 0 {
		t.Fatal("render snapshot aliases current source stream")
	}
	clone := current.Clone()
	clone.streams["swap"][0][0] = 99
	if current.streams["swap"][0][0] != 4 {
		t.Fatal("history snapshot aliases source stream")
	}
}

func TestVisualAdvanceRegistersActualExclusiveRotation(t *testing.T) {
	b := tinyBundle()
	g := b.Geometries["model"]
	g.Vertices = append(g.Vertices, make([]world.Vertex, 28-len(g.Vertices))...)
	g.Vertices[0] = world.Vertex{100, 0, 0}
	r, _ := New(b)
	scene := Scene{World: "GULF5", Camera: Camera{Pose: Pose{H0: 200000, H1: 200000, Height: 512}}, View: FullView(), Instances: []Instance{
		{ID: "A", ModelID: 0, Pose: Pose{H0: 200000, H1: 204096}, Callback: 3},
		{ID: "B", ModelID: 0, Pose: Pose{H0: 200050, H1: 204096}, Callback: 3},
	}}
	var state VisualState
	before := state.Clone()
	if err := r.Advance(scene, &state, 1); err != nil {
		t.Fatal(err)
	}
	if !state.rotations["model"] || state.streams["model"][0] != (world.Vertex{86, 0, 50}) {
		t.Fatal("actual callback3 ownership did not register exactly once", state.streams["model"])
	}
	if got := r.SmoothVisual(&before, &state, .5).streams["model"][0]; got != (world.Vertex{96, 0, 26}) {
		t.Fatal("first actual mutation did not use prepared baseline", got)
	}
	before = state.Clone()
	scene.Instances = scene.Instances[:1]
	scene.Instances[0].Callback = 1
	if err := r.Advance(scene, &state, 1); err != nil {
		t.Fatal(err)
	}
	if len(state.rotations) != 0 {
		t.Fatal("previous frame rotation eligibility leaked into ring frame")
	}
	if got := r.SmoothVisual(&before, &state, .5); !reflect.DeepEqual(got.streams, state.streams) {
		t.Fatal("ring was smoothed using stale rotation marker")
	}
}

func TestVisualWordWrapHasNoInventedContinuousArc(t *testing.T) {
	base := make([]world.Vertex, 28)
	base[0] = world.Vertex{-32768, 0, 32767}
	cur := append([]world.Vertex(nil), base...)
	cur[0] = world.Vertex{20775, 0, 11993}
	previous := VisualState{Phase: 0, streams: map[string][]world.Vertex{"component": base}}
	current := VisualState{Phase: 1, streams: map[string][]world.Vertex{"component": cur}, rotations: map[string]bool{"component": true}}
	r := &Renderer{}
	if got := r.SmoothVisual(&previous, &current, .5).streams["component"][0]; got != cur[0] {
		t.Fatal("wordwrap discontinuity was assigned a continuous arc", got)
	}
}
