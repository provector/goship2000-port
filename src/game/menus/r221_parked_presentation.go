package menus

import (
	"encoding/binary"
	"gunship2000-assets/game/navigation"
	"gunship2000-assets/game/render3d"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

// This is an explicitly native visual convenience.
// Original R13 proved coincident B homes and real same-point touchdown. This
// policy neither changes those homes nor gives simulation occupancy to models.

type parkedDisplayOffset struct {
	x, z         int32
	floor        int16
	homeX, homeZ uint32
}
type parkedDisplayState struct {
	valid   bool
	step    uint64
	offsets map[string]parkedDisplayOffset
}
type parkedFootprint struct {
	x, z   uint32
	radius int32
}
type parkedCandidate struct {
	id     string
	row    int
	x, z   uint32
	floor  int16
	radius int32
}

func (p *RenderingPreview) parkingRadius(model int) int32 {
	if model < 0 || model >= len(p.Renderer.Assets.Models) {
		return 0
	}
	g := p.Renderer.Assets.Geometries[p.Renderer.Assets.Models[model]]
	if g == nil || g.Extent <= 0 {
		return 0
	}
	return int32(g.Extent) // Original model extent is already in world/query units.
}

func parkingOverlap(x, z uint32, radius int32, occupied []parkedFootprint) bool {
	for _, a := range occupied {
		dx, dz := int64(x)-int64(a.x), int64(z)-int64(a.z)
		r := int64(radius + a.radius + 8)
		if dx*dx+dz*dz < r*r {
			return true
		}
	}
	return false
}

func (p *RenderingPreview) parkingFloor(x, z uint32, objects []world.QueryObject) (int16, bool) {
	if x < 0x4000 || x > 0x7bfff || z < 0x4000 || z > 0x7bfff {
		return 0, false
	}
	h, e := p.Renderer.Assets.ContactHeight(p.Initialized.World, world.QueryPose{H0: x, H1: z, Height: 32767}, objects)
	return h, e == nil && h >= 0
}

func (p *RenderingPreview) parkingFootprintSafe(x, z uint32, radius int32, floor int16, objects []world.QueryObject) bool {
	// Sample the complete conservative square around the original model extent.
	// A carrier edge, water or island/roof support change rejects the candidate.
	for _, dx := range []int32{-radius, 0, radius} {
		for _, dz := range []int32{-radius, 0, radius} {
			h, ok := p.parkingFloor(x+uint32(dx), z+uint32(dz), objects)
			if !ok || h != floor {
				return false
			}
		}
	}
	return true
}

func (p *RenderingPreview) parkingNearService(x, z uint32) bool {
	m := p.Initialized
	if m.Targets == nil {
		return false
	}
	pose := gamestate.InitializedPose{H0: x, H1: z}
	for i := 0; i < 2; i++ {
		r := m.Targets.Records[i]
		if i == 1 && binary.LittleEndian.Uint16(r[32:]) == 0 {
			continue
		}
		anchor := gamestate.InitializedPose{H0: binary.LittleEndian.Uint32(r[34:]), H1: binary.LittleEndian.Uint32(r[38:])}
		if gamestate.FinalResultMetric(pose, anchor) < 128 {
			return true
		}
	}
	return false
}

func (p *RenderingPreview) calculateParkedDisplay() map[string]parkedDisplayOffset {
	m := p.Initialized
	objects := m.ContactObjects()
	candidates := []parkedCandidate{}
	rows := make([][32]byte, len(m.Rows))
	for row := range rows {
		rows[row] = m.Rows[row].Raw
	}
	// Reserve the original assigned homes before any B order or touchdown.
	// A later order cannot make an already parked aircraft jump to another bay.
	// The original home resolver owns relative leader coordinates; current actor
	// positions, arrival order and presentation sampling never select a home.
	occupied := []parkedFootprint{}
	for row := 0; row < 5 && row < len(m.Entities) && row < len(rows); row++ {
		e := m.Entities[row]
		if e.ID == "" || p.carrierBerths[row].Present() {
			continue
		}
		home, err := navigation.ResolveHome(rows, e.ControlRaw, row)
		if err != nil {
			continue
		}
		x, z := home.H0, home.H1
		if !p.parkingNearService(x, z) {
			continue
		}
		radius := p.parkingRadius(int(rows[row][12]))
		floor, ok := p.parkingFloor(x, z, objects)
		if radius == 0 || !ok {
			continue
		}
		if row == m.PlayerRow {
			occupied = append(occupied, parkedFootprint{x, z, radius})
			continue
		}
		candidates = append(candidates, parkedCandidate{e.ID, row, x, z, floor, radius})
	}
	out := map[string]parkedDisplayOffset{}
	for _, a := range candidates {
		x, z := a.x, a.z
		found := !parkingOverlap(x, z, a.radius, occupied)
		if !found {
			gap := a.radius*2 + 16
			// Slot order and this fixed search order determine the layout; no RNG.
			for ring := int32(1); ring <= 3 && !found; ring++ {
				for _, v := range [][2]int32{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {1, -1}, {-1, 1}, {-1, -1}} {
					nx, nz := a.x+uint32(v[0]*ring*gap), a.z+uint32(v[1]*ring*gap)
					if !parkingOverlap(nx, nz, a.radius, occupied) && p.parkingFootprintSafe(nx, nz, a.radius, a.floor, objects) {
						x, z, found = nx, nz, true
						break
					}
				}
			}
		}
		if found && (x != a.x || z != a.z) {
			out[a.id] = parkedDisplayOffset{x: int32(x - a.x), z: int32(z - a.z), floor: a.floor, homeX: a.x, homeZ: a.z}
		}
		// If no safe offset exists preserve the authentic overlap, not an unsafe
		// deck placement or an invented new landing/approach decision.
		occupied = append(occupied, parkedFootprint{x, z, a.radius})
	}
	return out
}

// parkedPresentation returns a private instance slice only when needed. The
// canonical Scene, source rows, routes, service/recovery facts and framebuffer
// reference are untouched. Live and external replay use this same helper.
func (p *RenderingPreview) parkedPresentation(scene render3d.Scene, focusID string) (render3d.Scene, bool) {
	if p.Flight == nil || p.Initialized == nil || p.Inspection {
		return scene, false
	}
	if !p.parkedDisplay.valid || p.parkedDisplay.step != p.Flight.Steps {
		p.parkedDisplay = parkedDisplayState{true, p.Flight.Steps, p.calculateParkedDisplay()}
	}
	changed := false
	for i, in := range scene.Instances {
		o, ok := p.parkedDisplay.offsets[in.ID]
		if !ok {
			continue
		}
		// Final descent/parking uses the exact assigned home. A departing route
		// can fade the same bay within64 world units/height, so an explicit order
		// does not make its model jump. Ordinary initial lateral formation offsets
		// and already-airborne members stay outside this tiny display neighborhood.
		eligible := false
		departDistance := int32(0)
		for row := 1; row < 5 && row < len(p.Initialized.Entities); row++ {
			e := p.Initialized.Entities[row]
			if e.ID != in.ID {
				continue
			}
			if e.Pose.H0 == o.homeX && e.Pose.H1 == o.homeZ && e.Participation == 1 && ((e.Kind == 0x20 && e.ControlRaw[1] == 2) || (e.Kind == 0x26 && e.ControlRaw[1] == 6)) {
				eligible = true
			} else if e.Kind == 0x22 || e.Kind == 0x23 {
				dx, dz := int64(e.Pose.H0)-int64(o.homeX), int64(e.Pose.H1)-int64(o.homeZ)
				if dx < 0 {
					dx = -dx
				}
				if dz < 0 {
					dz = -dz
				}
				if dz > dx {
					dx = dz
				}
				if dx < 64 {
					eligible, departDistance = true, int32(dx)
				}
			}
			break
		}
		if !eligible {
			continue
		}
		above := int32(int16(in.Height) - o.floor - 2)
		if departDistance > above {
			above = departDistance
		}
		weight := int32(64) - above
		if weight <= 0 {
			continue
		}
		if weight > 64 {
			weight = 64
		}
		dx, dz := o.x*weight/64, o.z*weight/64
		if dx == 0 && dz == 0 {
			continue
		}
		if !changed {
			scene.Instances = append([]render3d.Instance(nil), scene.Instances...)
			changed = true
		}
		scene.Instances[i].H0 += uint32(dx)
		scene.Instances[i].H1 += uint32(dz)
		if in.ID == focusID {
			scene.Camera.H0 += uint32(dx)
			scene.Camera.H1 += uint32(dz)
		}
	}
	return scene, changed
}
