package world

import (
	"os"
	"testing"
)

func TestHeightQuerySourceArithmetic(t *testing.T) {
	square := [][2]int16{{-100, -100}, {-100, 100}, {100, 100}, {100, -100}, {-100, -100}}
	for _, c := range []struct {
		x, z int16
		want bool
	}{{0, 0, true}, {50, 50, true}, {101, 0, false}, {-101, 0, false}} {
		got, e := SourceContains(square, c.x, c.z)
		if e != nil || got != c.want {
			t.Fatalf("contains %+v: %v %v", c, got, e)
		}
	}
	r := []HeightRegion{{Threshold: 20, Points: square, Plane: Plane{0, 2, 0, -100}}, {Points: square, Plane: Plane{0, 1, 0, -7}}}
	for _, c := range []struct{ current, want int16 }{{19, 7}, {20, 50}, {21, 50}} {
		got, e := RegionHeight(r, 0, 0, c.current)
		if e != nil || got != c.want {
			t.Fatalf("threshold %+v: %v %v", c, got, e)
		}
	}
	// Source is first-match, not maximum-height; signed IDIV truncates to zero.
	r[0].Threshold = 0
	r[0].Plane = Plane{1, 3, 0, -2}
	if got, e := RegionHeight(r, 0, 0, 0); e != nil || got != 0 {
		t.Fatalf("signed division %v %v", got, e)
	}
	r[0].Plane[1] = 0
	if _, e := RegionHeight(r, 0, 0, 0); e == nil {
		t.Fatal("zero source divisor accepted")
	}
}

func TestHeightQueryOriginalGulfCell(t *testing.T) {
	path := os.Getenv("GS2000_TEST_WORLD")
	if path == "" {
		t.Skip("set GS2000_TEST_WORLD to the separate prepared world directory containing index.json")
	}
	b, e := Load(path)
	if e != nil {
		t.Fatal(e)
	}
	// Independently worked from original GULF5 cell2/raw slope planes,
	// evidence/step29/terrain checks: n=11669812, divisor16137 =>723.
	for _, c := range []struct {
		x, z uint32
		want int16
	}{{299008, 503808, 723}, {296008, 503808, 439}, {297408, 503008, 1000}} {
		got, e := b.TerrainHeight("GULF5", QueryPose{H0: c.x, H1: c.z, Height: 1500})
		if e != nil || got != c.want {
			t.Fatalf("original cell %+v: %v %v", c, got, e)
		}
	}
	if _, e := b.TerrainHeight("GULF5", QueryPose{H0: 64 * 8192}); e == nil {
		t.Fatal("world edge silently wrapped")
	}
}
