package combat

import (
	"encoding/binary"
	gamestate "gunship2000-assets/game/state"
)

// LAND bit 2 selects built-up cells in the supported mission worlds. Their
// terrain vertices already include ground elevation and roof/steeple heights.
type cityClearance struct {
	grid []byte
	body [64]int16
}

func (s *State) SetLowCityClearance(m *gamestate.MissionData) {
	s.city = nil
	if m == nil || m.SourceWorld == nil || m.SourceWorld.TerrainName != s.init.World {
		return
	}
	w := s.assets.Worlds[s.init.World]
	if w == nil || len(w.Grid) != 4096 || len(w.Cells) != 64 || len(m.SourceWorld.LAND) < 64 {
		return
	}
	var c cityClearance
	for id, cell := range w.Cells {
		if m.SourceWorld.LAND[id]&4 == 0 {
			continue
		}
		g := s.assets.Geometries[cell.Geometry]
		if g == nil {
			continue
		}
		var top int16
		for _, v := range g.Vertices {
			top = max(top, v[1])
		}
		// Up to 80 native units below the origin on the cargo Blackhawk, plus
		// 16 for the two integer altitude controllers' convergence.
		if top > 0 && top <= 32767-96 {
			c.body[id] = top + 96
			c.grid = w.Grid
		}
	}
	if c.grid != nil {
		s.city = &c
	}
}

func (s *State) lowCityRoute(actor int, e *gamestate.InitializedEntity) bool {
	if s.city == nil || actor < 1 || actor > 4 || actor == s.init.PlayerRow || e.Participation != 1 || (e.Kind != 0x22 && e.Kind != 0x23) || binary.LittleEndian.Uint16(e.ControlRaw[6:]) != 3 {
		return false
	}
	for i := 1; i < len(s.init.Entities) && i <= 4; i++ {
		member := &s.init.Entities[i]
		if (i == actor || member.Kind == 0x21 && int(member.ControlRaw[5]) == actor) && (s.init.Stores == nil || i >= len(s.init.Stores.Damage) || s.init.Stores.Damage[i][9] != 0) {
			return false
		}
	}
	return true
}

func (c *cityClearance) at(x, z uint32) int16 {
	if x >= worldEdge || z >= worldEdge {
		return 0
	}
	id := c.grid[(x>>13)+64*(63-(z>>13))]
	if id >= 64 {
		return 0
	}
	return c.body[id]
}

// Look at the current square and the next half-square along the actual leg,
// stopping at the goal. At most one grid line per axis can be crossed.
func (c *cityClearance) approach(p gamestate.InitializedPose, x, z uint32) int16 {
	dx, dz := int64(x)-int64(p.H0), int64(z)-int64(p.H1)
	ax, az := max(dx, -dx), max(dz, -dz)
	if length := max(ax, az); length > 4096 {
		x = uint32(int64(p.H0) + dx*4096/length)
		z = uint32(int64(p.H1) + dz*4096/length)
	}
	floor := max(c.at(p.H0, p.H1), c.at(x, z))
	if x>>13 != p.H0>>13 && z>>13 != p.H1>>13 {
		// Compare parametric crossing times. At an exact corner there is no
		// intervening square on the segment.
		edgeX, edgeZ := int64(p.H0&8191), int64(p.H1&8191)
		if dx > 0 {
			edgeX = 8192 - edgeX
		}
		if dz > 0 {
			edgeZ = 8192 - edgeZ
		}
		if edgeX*az < edgeZ*ax {
			floor = max(floor, c.at(x, p.H1))
		} else if edgeX*az > edgeZ*ax {
			floor = max(floor, c.at(p.H0, z))
		}
	}
	return floor
}
