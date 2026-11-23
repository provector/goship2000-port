package world

import "testing"

func TestPairedTerrainSourceThresholdAndMissing(t *testing.T) {
	square := [][2]int16{{-100, -100}, {-100, 100}, {100, 100}, {100, -100}, {-100, -100}}
	g := &Geometry{Heights: []HeightRegion{{Threshold: 20, Points: square, Plane: Plane{0, 2, 0, -100}}}}
	cells := make([]Cell, 64)
	for i := range cells {
		cells[i].Geometry = "x"
	}
	b := &Bundle{Worlds: map[string]*World{"X": {Grid: make([]byte, 4096), Cells: cells}}, Geometries: map[string]*Geometry{"x": g}}
	first := QueryPose{H0: 4096, H1: 4096, Height: 0}
	second := QueryPose{H0: 800000, H1: 800000, Height: 20}
	if got, e := b.PairedTerrainHeight("X", first, second); e != nil || got != 50 {
		t.Fatalf("firstcell/secondheight contract %d %v", got, e)
	}
	second.Height = 19
	if got, e := b.PairedTerrainHeight("X", first, second); e != nil || got != -1 {
		t.Fatalf("threshold gate/no-match %d %v", got, e)
	}
	second.Height = 20
	first.H0 += 200
	if got, e := b.PairedTerrainHeight("X", first, second); e != nil || got != -1 {
		t.Fatalf("secondpoint notgenericsegment %d %v", got, e)
	}
}
