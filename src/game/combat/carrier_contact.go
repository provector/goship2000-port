package combat

import (
	"gunship2000-assets/game/render3d"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"math"
)

// The raised first contact region of carrier model 64 is its rectangular island.
// Keep source geometry and ship rows immutable. A destroyed/replaced ship no
// longer supplies a wall. Ordinary bases never enter this contact path.
type carrierRotor struct {
	radius, height int32
	modelID        int
}
type carrierIsland struct {
	padding                      int32
	rotors                       [5]carrierRotor
	rows                         []int
	left, right, aft, fore, roof int32
}

func (s *State) EnableCarrierClearance() {
	const carrierModel = 64
	if s.init.Switches&0x1000 == 0 || len(s.assets.Models) <= carrierModel {
		return
	}
	g := s.assets.Geometries[s.assets.Models[carrierModel]]
	if g == nil || len(g.Heights) < 2 {
		return
	}
	r := g.Heights[0]
	if len(r.Points) != 5 || r.Points[0] != r.Points[4] || r.Plane[0] != 0 || r.Plane[2] != 0 || r.Plane[1] == 0 {
		return
	}
	left, right := r.Points[0][0], r.Points[0][0]
	aft, fore := r.Points[0][1], r.Points[0][1]
	for i, v := range r.Points[:4] {
		next := r.Points[i+1]
		if (next[0] == v[0]) == (next[1] == v[1]) {
			return
		}
		left, right = min(left, v[0]), max(right, v[0])
		aft, fore = min(aft, v[1]), max(fore, v[1])
	}
	x, z := left+(right-left)/2, aft+(fore-aft)/2
	roof, err := world.RegionHeight(g.Heights, x, z, 32767)
	deck, e := world.RegionHeight(g.Heights[1:], x, z, 32767)
	if err != nil || e != nil || roof <= deck || deck <= 0 {
		return
	}
	island := &carrierIsland{left: int32(left), right: int32(right), aft: int32(aft), fore: int32(fore), roof: int32(roof)}
	for i, e := range s.init.Entities {
		if e.ModelID == carrierModel {
			island.rows = append(island.rows, i)
			continue
		}
		if e.FlightSlot < 1 || e.FlightSlot >= len(island.rotors) || e.ModelID < 0 || e.ModelID >= len(s.assets.Models) {
			continue
		}
		mesh := s.assets.Geometries[s.assets.Models[e.ModelID]]
		if mesh == nil {
			continue
		}
		// Extent is the painter's metric, measured in different units.
		// A vertex's Manhattan length bounds its projection at every attitude.
		rotorPoints := 0
		switch e.Object.Raw[1] {
		case 1:
			rotorPoints = 8
		case 2:
			rotorPoints = 10
		}
		for j, v := range mesh.Vertices {
			x, y, z := int32(v[0]), int32(v[1]), int32(v[2])
			island.padding = max(island.padding, max(x, -x)+max(y, -y)+max(z, -z))
			if j < rotorPoints {
				rotor := &island.rotors[e.FlightSlot]
				rotor.modelID, rotor.height = e.ModelID, y
				rotor.radius = max(rotor.radius, int32(math.Ceil(math.Hypot(float64(x), float64(z)))))
			}
		}
	}
	if len(island.rows) > 0 {
		s.carrierIsland = island
	}
}

func (s *State) clearCarrierContact(e *gamestate.InitializedEntity, before gamestate.InitializedPose) {
	island := s.carrierIsland
	if island == nil || e.FlightSlot <= 0 || e.FlightSlot >= len(island.rotors) || !e.Visible || e.Kind < 0x20 || e.Kind > 0x26 || e.Participation >= 2 || e.ModelID < 0 || e.ModelID >= len(s.assets.Models) {
		return
	}
	g := s.assets.Geometries[s.assets.Models[e.ModelID]]
	if g == nil || len(g.Vertices) == 0 {
		return
	}
	for _, row := range island.rows {
		ship := &s.init.Entities[row]
		if !ship.Visible || ship.ModelID != 64 {
			continue
		}
		x, z := int32(e.Pose.H0-ship.Pose.H0), int32(e.Pose.H1-ship.Pose.H1)
		y := int32(int16(e.Pose.Height)) - int32(int16(ship.Pose.Height))
		extent := island.padding
		if x+extent < island.left || x-extent > island.right || z+extent < island.aft || z-extent > island.fore || y-extent > island.roof {
			continue
		}
		// Use the same fixed-point model orientation as the aircraft display.
		// Its origin alone can be clear while a rotor tip or tail enters the island.
		m := render3d.Rotation(e.Pose.Angles, s.assets.Sine, false)
		left, right, aft, fore, bottom := x, x, z, z, y
		for _, v := range g.Vertices {
			offset := func(axis int) int32 {
				return (int32(v[0])*int32(m[axis][0]) + int32(v[1])*int32(m[axis][1]) + int32(v[2])*int32(m[axis][2])) >> 14
			}
			vx, vz := x+offset(0), z+offset(2)
			left, right = min(left, vx), max(right, vx)
			aft, fore = min(aft, vz), max(fore, vz)
			bottom = min(bottom, y+offset(1))
		}
		// Include all visual blade phases without reading presentation state.
		if rotor := island.rotors[e.FlightSlot]; rotor.radius > 0 && rotor.modelID == e.ModelID {
			cx := x + (rotor.height * int32(m[0][1]) >> 14)
			cz := z + (rotor.height * int32(m[2][1]) >> 14)
			left, right = min(left, cx-rotor.radius), max(right, cx+rotor.radius)
			aft, fore = min(aft, cz-rotor.radius), max(fore, cz+rotor.radius)
		}
		if right <= island.left || left >= island.right || fore <= island.aft || aft >= island.fore || bottom >= island.roof {
			continue
		}
		if bottom+int32(int16(before.Height)-int16(e.Pose.Height)) >= island.roof {
			e.Pose.Height += uint16(island.roof - bottom)
		} else {
			// Resolve just the penetrating axis; tangential movement, controls
			// and existing route/engagement decisions remain with their owners.
			west, east, south, north := right-island.left, island.right-left, fore-island.aft, island.fore-aft
			switch min(west, east, south, north) {
			case west:
				e.Pose.H0 -= uint32(west)
			case east:
				e.Pose.H0 += uint32(east)
			case south:
				e.Pose.H1 -= uint32(south)
			case north:
				e.Pose.H1 += uint32(north)
			}
		}
		s.syncDeath(e)
	}
}
