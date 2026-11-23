package combat

import (
	"encoding/binary"
	gamestate "gunship2000-assets/game/state"
)

const worldEdge = 64 * 8192

func (s *State) SetCorridorNavigation(enabled bool) { s.corridorNavigation = enabled }

func (s *State) EnableExitWaypoint() { s.admitExitWaypoint = true }

func insideWorld(p gamestate.InitializedPose) bool { return p.H0 < worldEdge && p.H1 < worldEdge }

// Intersect a short probe from a valid body with the last sampleable grid line.
// Source motion stores signed offsets in unsigned pose words, including at zero.
func gridProbe(body, probe gamestate.InitializedPose) gamestate.InitializedPose {
	if insideWorld(probe) || !insideWorld(body) {
		return probe
	}
	x, z := int64(body.H0), int64(body.H1)
	dx, dz := int64(int32(probe.H0))-x, int64(int32(probe.H1))-z
	num, den := int64(1), int64(1)
	for _, axis := range [2][2]int64{{x, dx}, {z, dz}} {
		a, d := axis[0], axis[1]
		var room int64
		if d < 0 {
			room = a
			d = -d
		} else {
			room = worldEdge - 1 - a
		}
		if d > 0 && room*den < num*d {
			num, den = room, d
		}
	}
	probe.H0, probe.H1 = uint32(x+dx*num/den), uint32(z+dz*num/den)
	return probe
}

// The desired home and TARGET base identify a corridor RTB. FARP and ground
// approaches have independent destinations. Do not infer an exit from pose.
func (s *State) corridorGoal(e *gamestate.InitializedEntity) (NavigationPoint, bool) {
	if !s.corridorNavigation || e.Kind != 0x22 || e.ControlRaw[1] != 8 || e.ControlRaw[0] != 1 || s.init.Switches&0x800 == 0 || s.init.Targets == nil {
		return NavigationPoint{}, false
	}
	u32 := binary.LittleEndian.Uint32
	x, z := u32(e.ControlRaw[10:]), u32(e.ControlRaw[14:])
	base := s.init.Targets.Records[0]
	if x != u32(base[34:]) || x > worldEdge || z > worldEdge {
		return NavigationPoint{}, false
	}
	// Validate the northing too. Independently ordered ships can carry a
	// longitudinal formation offset (ResolveHome). A formed child that was
	// handed its departed parent's gate retains that exact route.
	row := e.RuntimeRow
	if row < 1 || row >= 5 || row >= len(s.init.Rows) {
		return NavigationPoint{}, false
	}
	own := s.init.Rows[row].Raw
	known := z == u32(base[38:]) || int32(u32(own[2:])) > 0x400 && x == u32(own[2:]) && z == u32(own[6:])
	parent := int(int8(e.ControlRaw[5]))
	if !known && parent >= 0 && parent < row && parent < len(s.init.Entities) {
		origin := s.init.Rows[parent].Raw
		known = int32(u32(own[2:])) <= 0x400 && x == u32(origin[2:]) && z == u32(origin[6:])+u32(own[6:])
		leader := &s.init.Entities[parent]
		known = known || leader.Kind == 0x41 && x == u32(leader.ControlRaw[10:]) && z == u32(leader.ControlRaw[14:])
	}
	if !known {
		return NavigationPoint{}, false
	}
	// A selected FARP can share the base's easting. Compare its complete destination.
	farp := s.init.Targets.Records[1]
	if binary.LittleEndian.Uint16(farp[32:]) != 0 && x == u32(farp[42:]) && z == u32(farp[46:]) {
		return NavigationPoint{}, false
	}
	switch {
	case x < 0x8000:
		x = 0
	case x > 0x78000:
		x = worldEdge
	case z < 0x8000:
		z = 0
	case z > 0x78000:
		z = worldEdge
		// An interior designated home keeps the source point-arrival semantics.
	}
	return NavigationPoint{x, z}, true
}

// Check the crossed boundary at the interpolated lateral position, using the
// source arrival tolerance (8 SAR4 units). Never credit a remote crossing.
func crossedCorridor(old, next gamestate.InitializedPose, g NavigationPoint) bool {
	if !insideWorld(old) {
		return false
	}
	axis := func(a, b, l, m, edge, goal int64) bool {
		if a == b || edge != 0 && edge != worldEdge || edge == 0 && (b > 0 || b >= a) || edge == worldEdge && (b < edge || b <= a) {
			return false
		}
		lateral := (l-goal)*(b-a) + (m-l)*(edge-a)
		if lateral < 0 {
			lateral = -lateral
		}
		delta := b - a
		if delta < 0 {
			delta = -delta
		}
		return lateral < 128*delta
	}
	x, z, nx, nz := int64(old.H0), int64(old.H1), int64(int32(next.H0)), int64(int32(next.H1))
	return axis(x, nx, z, nz, int64(g.H0), int64(g.H1)) || axis(z, nz, x, nx, int64(g.H1), int64(g.H0))
}

func (s *State) returnThroughCorridor(actor int) {
	e := &s.init.Entities[actor]
	e.Kind, e.ControlRaw[1] = 0x41, 0
	s.ResetRouteAvoidance(actor)
	s.ResetNavigation(actor)
	s.syncDeath(e)
	s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: 12, Source: "EA4A:corridor returned"})
}

// A formed or rejoining child follows the departed parent's actual return
// destination. Its own subsequent arrival (or damage transition) owns its fate.
func (s *State) followExitedSection(actor int, leader *gamestate.InitializedEntity) bool {
	if !s.corridorNavigation || s.init.Switches&0x800 == 0 || leader.Kind != 0x41 || leader.ControlRaw[0] != 1 {
		return false
	}
	departed := *leader
	departed.Kind, departed.ControlRaw[1] = 0x22, 8
	goal, ok := s.corridorGoal(&departed)
	if !ok || binary.LittleEndian.Uint32(leader.ControlRaw[40:]) != goal.H0 || binary.LittleEndian.Uint32(leader.ControlRaw[44:]) != goal.H1 {
		return false
	}
	if s.init.Stores.Damage[actor][9] != 0 {
		s.disableFollower(actor)
		return true
	}
	e := &s.init.Entities[actor]
	e.Kind, e.Participation, e.ControlRaw[0], e.ControlRaw[1] = 0x22, 0, 1, 8
	copy(e.ControlRaw[10:40], leader.ControlRaw[10:40])
	copy(e.ControlRaw[40:70], leader.ControlRaw[40:70])
	copy(e.ControlRaw[6:10], leader.ControlRaw[6:10])
	s.ResetNavigation(actor)
	s.ResetRouteAvoidance(actor)
	s.syncDeath(e)
	return true
}
