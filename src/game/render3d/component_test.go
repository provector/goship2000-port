package render3d

import (
	"gunship2000-assets/game/world"
	"reflect"
	"testing"
)

func TestInstanceGraphEdgeSwitchDoesNotMutateModel(t *testing.T) {
	b := tinyBundle()
	r, err := New(b)
	if err != nil {
		t.Fatal(err)
	}
	g := &world.Geometry{ID: "component", Root: 0, Vertices: []world.Vertex{{0, 0, 128}}, Planes: make([]world.Plane, 2), Children: [][2]int{{1, 255}, {255, 255}}, PlaneGroups: []world.Group{{}, {Primitive: &world.Primitive{Opcode: 1, Colour: 0x3333, Radius: 1, Indices: []int{0}}}}}
	original := append([][2]int(nil), g.Children...)
	item := drawItem{geometry: g, matrix: Rotation([3]uint16{}, b.Sine, true)}
	count := func(i drawItem) int {
		f := frame{r: r, dst: make([]byte, 64000), view: FullView(), compact: map[int][]Vector{}}
		f.drawItem(i)
		return f.stats.Primitives
	}
	if count(item) != 1 {
		t.Fatal("source child did not draw")
	}
	item.graphChild = &GraphChildOverride{Node: 0, Side: 0, Expected: 1, Child: 255}
	if count(item) != 0 || count(item) != 0 {
		t.Fatal("disabled instance edge drew or changed across frames")
	}
	other := item
	other.graphChild = nil
	if count(other) != 1 {
		t.Fatal("component leaked into another instance")
	}
	item.graphChild = &GraphChildOverride{Node: 0, Side: 0, Expected: 1, Child: 1}
	if count(item) != 1 || !reflect.DeepEqual(g.Children, original) {
		t.Fatal("restoration/shared geometry ownership")
	}
}

func TestSceneRejectsMismatchedGraphComponent(t *testing.T) {
	b := tinyBundle()
	r, _ := New(b)
	g := b.Geometries[b.Models[0]]
	g.Children = [][2]int{{255, 255}}
	s := Scene{World: "GULF5", Camera: Camera{Pose: Pose{H0: 299008, H1: 503808}}, Instances: []Instance{{ID: "player", ModelID: 0, GraphChild: &GraphChildOverride{Node: 0, Side: 0, Expected: 255, Child: 255}}}}
	if err := r.validateScene(s); err != nil {
		t.Fatal(err)
	}
	for _, edge := range []*GraphChildOverride{{Node: 0, Side: 0, Expected: 41, Child: 255}, {Node: 1, Side: 0, Expected: 255, Child: 255}, {Node: 0, Side: 2, Expected: 255, Child: 255}, {Node: 0, Side: 0, Expected: 255, Child: 0}} {
		s.Instances[0].GraphChild = edge
		if err := r.validateScene(s); err == nil {
			t.Fatalf("invalid component accepted%+v", edge)
		}
	}
}
