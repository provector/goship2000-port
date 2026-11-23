package world

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestSubmarineMutableWordsAndIsolation(t *testing.T) {
	g := &Geometry{Vertices: []Vertex{{1, 0, 3}, {4, 1, 6}, {7, 2, 9}, {10, 3, 12}, {13, -1, 15}}, Planes: make([]Plane, 32), Children: make([][2]int, 32), Root: 13, Extent: 99}
	for i := range g.Children {
		g.Children[i] = [2]int{255, 255}
	}
	g.Children[13] = [2]int{24, 0}
	b := &Bundle{Models: []string{"sub"}, Geometries: map[string]*Geometry{"sub": g}}
	a, err := NewSubmarineModel(b, 0)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := NewSubmarineModel(b, 0)
	before, _ := json.Marshal(other)
	for _, c := range []struct {
		counter int16
		wantY   []int16
		child   [2]int
	}{{-129, []int16{0, 0, 0, 1, -3}, [2]int{24, 0}}, {-128, []int16{0, 0, 0, 1, -3}, [2]int{24, 255}}, {-101, []int16{0, 0, 0, 0, -5}, [2]int{24, 255}}, {-100, []int16{0, 0, 0, 0, -5}, [2]int{31, 255}}, {-99, []int16{0, 0, 0, 0, -7}, [2]int{31, 255}}} {
		if err = a.SubmergeVisit(c.counter); err != nil {
			t.Fatal(err)
		}
		for i, v := range a.Vertices {
			if v[1] != c.wantY[i] || v[0] != g.Vertices[i][0] || v[2] != g.Vertices[i][2] {
				t.Fatal("word mutation", c.counter, a.Vertices)
			}
		}
		if a.Children[13] != c.child {
			t.Fatal(c.counter, a.Children[13])
		}
	}
	after, _ := json.Marshal(other)
	if string(before) != string(after) || !reflect.DeepEqual(g.Vertices, []Vertex{{1, 0, 3}, {4, 1, 6}, {7, 2, 9}, {10, 3, 12}, {13, -1, 15}}) || g.Children[13] != ([2]int{24, 0}) {
		t.Fatal("one submarine mutated another/shared geometry")
	}
	copy, err := a.Apply(g)
	if err != nil || copy == g || copy.Root != g.Root || copy.Extent != 99 || copy.Vertices[4][1] != -7 || copy.Children[13] != ([2]int{31, 255}) {
		t.Fatal("private header", copy, err)
	}
	a.Children[13][0] = 999
	if _, err = a.Apply(g); err == nil {
		t.Fatal("bad child escaped source geometry")
	}
}

func TestActualType209ModelGraphAndCollision(t *testing.T) {
	path := os.Getenv("GS2000_TEST_WORLD")
	if path == "" {
		t.Skip("set GS2000_TEST_WORLD to prepared world directory")
	}
	b, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	g := b.Geometries[b.Models[219]]
	if len(g.Vertices) != 45 || len(g.Planes) != 32 || len(g.Children) != 32 || g.Root != 13 || g.Children[13] != ([2]int{24, 0}) || len(g.Heights) != 0 {
		t.Fatal("actual Type209 source operands changed", len(g.Vertices), len(g.Planes), g.Root, g.Children[13], g.Heights)
	}
	immutable, _ := json.Marshal(g)
	a, _ := NewSubmarineModel(b, 219)
	other, _ := NewSubmarineModel(b, 219)
	for c := int16(-159); c < 0; c++ {
		if err = a.SubmergeVisit(c); err != nil {
			t.Fatal(c, err)
		}
	}
	copy, err := a.Apply(g)
	if err != nil || copy.Children[13] != ([2]int{31, 255}) || len(copy.Heights) != 0 || !reflect.DeepEqual(copy.Planes, g.Planes) || !reflect.DeepEqual(other.Vertices, g.Vertices) {
		t.Fatal("actual model state/independent instance/collision", err)
	}
	after, _ := json.Marshal(g)
	if string(immutable) != string(after) || !reflect.DeepEqual(other.Children, g.Children) {
		t.Fatal("prepared graph changed")
	}
	for i, v := range a.Vertices {
		if v[1] != 0 || v[0] != g.Vertices[i][0] || v[2] != g.Vertices[i][2] {
			t.Fatal("source nonzero-Y erosion", i, v)
		}
	}
	t.Log("Type209 model219:45 vertices,32planes,root13:[24,0]->[31,255]; empty solid-height block unchanged; two private copies independent")
}
