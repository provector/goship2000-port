package combat

import (
	"encoding/binary"
	"gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"testing"
)

// One 8192-unit square, a level street and an actual collision roof across
// the route. The square's tallest visual vertex includes its elevated base.
func TestLowCitySquareApproachExitAndOrders(t *testing.T) {
	for _, tc := range []struct {
		alt  uint16
		city bool
	}{{3, true}, {1, true}, {2, true}, {3, false}} {
		s, in := navigationFixture(t)
		s.SetObstacleClearance(true)
		s.SetCorridorNavigation(true)
		w := s.assets.Worlds[s.init.World]
		g := *s.assets.Geometries[w.Cells[0].Geometry]
		g.Vertices = []world.Vertex{{0, 100, 0}, {0, 900, 0}}
		g.Heights = []world.HeightRegion{{Plane: world.Plane{0, 1, 0, -900}, Points: [][2]int16{{-4096, -128}, {4096, -128}, {4096, 128}, {-4096, 128}, {-4096, -128}}}, g.Heights[len(g.Heights)-1]}
		w.Cells[1].Geometry = "roof"
		s.assets.Geometries["roof"] = &g
		x, z := uint32(32*8192+4096), uint32(32*8192)
		w.Grid[32+64*(63-32)] = 1
		land := make([]byte, 64)
		// Keep a marked, unused tile even for the clear-route control.
		w.Cells[2].Geometry = "roof"
		land[2] = 4
		if tc.city {
			land[1] = 4
		}
		s.SetLowCityClearance(&gamestate.MissionData{SourceWorld: &assets.MissionWorld{TerrainName: s.init.World, LAND: land}})
		e := &s.init.Entities[1]
		e.Participation = 1
		e.Pose.H0 = x
		e.Pose.H1 = z - 3900
		e.Pose.Height = 400
		binary.LittleEndian.PutUint16(e.ControlRaw[6:], tc.alt)
		binary.LittleEndian.PutUint32(e.ControlRaw[40:], x)
		binary.LittleEndian.PutUint32(e.ControlRaw[44:], z+8192+6000)
		if !tc.city || tc.alt != 3 {
			if s.lowCityRoute(1, e) && s.city.approach(e.Pose, x, z+8192+6000) > 0 {
				t.Fatalf("wrong city activation %+v", tc)
			}
			before := *e
			if err := s.advanceFollowerRoute(1, in); err != nil {
				t.Fatal(err)
			}
			got := *e
			*e = before
			s.city = nil
			s.ResetNavigation(1)
			if err := s.advanceFollowerRoute(1, in); err != nil || *e != got {
				t.Fatalf("ordinary movement changed %+v %v", tc, err)
			}
			continue
		}
		approach, roof, exit := false, false, false
		for visit := 0; visit < 1800 && e.Kind == 0x23; visit++ {
			if err := s.advanceFollowerRoute(1, in); err != nil {
				t.Fatal(err)
			}
			if e.Pose.H1 < z && e.Pose.Height > 800 {
				approach = true
			}
			if e.Pose.H1 >= z+4096-128 && e.Pose.H1 <= z+4096+128 && e.Pose.Height >= 980 {
				roof = true
			}
			if e.Pose.H1 > z+8192+4000 && e.Pose.Height < 800 {
				exit = true
			}
		}
		if !approach || !roof || !exit || e.Kind != 0x20 {
			t.Fatalf("climb=%t crossed=%t descended=%t end=%+v kind=%x", approach, roof, exit, e.Pose, e.Kind)
		}
	}
}

// Both endpoints can be ordinary squares when a diagonal leg crosses a
// corner of a city tile. The other off-diagonal square must stay unselected.
func TestLowCityLookaheadCornerAndShortLeg(t *testing.T) {
	c := cityClearance{grid: make([]byte, 4096)}
	c.body[1] = 1096
	const edge = 32 * 8192
	c.grid[32+64*(63-31)] = 1
	p := gamestate.InitializedPose{H0: edge - 100, H1: edge - 400}
	if c.approach(p, edge+1000, edge+1000) != 1096 {
		t.Fatal("missed intermediate city square")
	}
	p.H0, p.H1 = edge-400, edge-100
	if c.approach(p, edge+1000, edge+1000) != 0 {
		t.Fatal("raised adjacent route")
	}
	p.H0, p.H1 = edge-100, edge-100
	if c.approach(p, edge+1000, edge+1000) != 0 {
		t.Fatal("city touched only at exact corner")
	}
	if c.approach(p, edge-10, edge-100) != 0 {
		t.Fatal("looked beyond end of leg")
	}
}
