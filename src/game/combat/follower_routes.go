package combat

import (
	"encoding/binary"
	"fmt"
	"sort"

	"gunship2000-assets/game/avionics"
	gamestate "gunship2000-assets/game/state"
)

// SetOrdersEnabled is selected before launch. Historical profiles never visit
// the new route controller or the GPS return RNG epoch.
func (s *State) SetOrdersEnabled(enabled bool) { s.ordersEnabled = enabled }

// invalidFlightOrder carries an unusable motion instruction after the actor,
// stores and body position have been identified. It carries no result facts.
type invalidFlightOrder string

func (e invalidFlightOrder) Error() string { return string(e) }

type invalidWaypoint struct {
	kind  byte
	index int
}

func (e invalidWaypoint) Error() string {
	return fmt.Sprintf("kind%02x route index%d lacks source CONTROL cache", e.kind, e.index)
}

// A legacy route failure could skip a movement visit while leaving all result owners intact.
// Independent failures on the same aircraft remain essential even if recorded later.
func (s *State) OnlyWaypointDiagnostic(actor int) bool {
	return s.historicalRouteOnly[actor] && !s.unresolvedFollower[actor]
}
func (s *State) EnableResilientRoutes()   { s.resilientRoutes = true }
func (s *State) RouteWarnings() [5]string { return s.routeWarning }

// holdInvalidRoute preserves the reached pose. The ordinary hover order owns
// later visits, including damage and fire; arrival and recovery require their own evidence.
func (s *State) holdInvalidRoute(actor int, in Input, reason string, engagementPending bool) error {
	e := &s.init.Entities[actor]
	s.routeWarning[actor] = reason
	e.Kind, e.ControlRaw[1], e.ControlRaw[0], e.RawMotion = 0x20, 2, 0, 0
	if binary.LittleEndian.Uint16(e.ControlRaw[6:]) >= uint16(len(followerAltitude)) {
		binary.LittleEndian.PutUint16(e.ControlRaw[6:], 1)
	}
	if binary.LittleEndian.Uint16(e.ControlRaw[8:]) >= uint16(len(followerSpeed)) {
		binary.LittleEndian.PutUint16(e.ControlRaw[8:], 0)
	}
	s.ResetRouteAvoidance(actor)
	s.ResetNavigation(actor)
	s.syncDeath(e)
	if engagementPending {
		return s.advanceHealthyFollower(actor, in)
	}
	return nil
}
func (s *State) OrdersEnabled() bool { return s.ordersEnabled }
func (s *State) SetDispatcherScratchEnabled(enabled bool) {
	s.dispatcherScratchEnabled = enabled
	s.dispatcherScratchKnown = false
}

// Selected before launch; historical films retain their original guard policy.
func (s *State) SetSafeFlightOrdersEnabled(enabled bool) { s.safeFlightOrders = enabled }

// All these full-pose MOVSW writers share B01C's single BP frame across actor
// visits. Retain the reached pre-altitude height, not the previous actor's final
// pose and not a guessed initial stack value. Nothing survives a new invocation.
func (s *State) retainDispatcherHeight(height uint16) {
	if s.dispatcherScratchEnabled {
		s.dispatcherScratchKnown = true
		s.dispatcherScratchHeight = int16(height)
	}
}
func (s *State) SetWingmenAutonomyEnabled(enabled bool) { s.wingmenAutonomyDisabled = !enabled }

// followerRouteBlocked evaluates EB9B/EBF4 using ONE incoming BP-30 word.
// Partition on the union of both queries' real height thresholds. This proves
// the combined wrapped comparison without inventing either numeric query value.
func (s *State) followerRouteBlocked(a, b gamestate.InitializedPose, limit int16) (bool, error) {
	if s.dispatcherScratchEnabled && s.dispatcherScratchKnown || s.safeFlightOrders {
		qa, qb := deathQuery(a), deathQuery(b)
		height := int16(a.Height) // One queried-aircraft snapshot for both probes; never donated.
		if s.dispatcherScratchEnabled && s.dispatcherScratchKnown {
			height = s.dispatcherScratchHeight
		}
		qa.Height, qb.Height = height, height
		objects := s.deathQueryObjects()
		x, err := s.assets.ContactHeight(s.init.World, qa, objects)
		if err != nil {
			return false, err
		}
		y, err := s.assets.ContactHeight(s.init.World, qb, objects)
		if err != nil {
			return false, err
		}
		return limit < int16(x+y), nil
	}
	thresholds := map[int]bool{}
	for _, p := range []gamestate.InitializedPose{a, b} {
		h, err := s.formationCornerThresholds(p)
		if err != nil {
			return false, err
		}
		for _, v := range h {
			thresholds[v] = true
		}
	}
	values := make([]int, 0, len(thresholds))
	for h := range thresholds {
		values = append(values, h)
	}
	sort.Ints(values)
	var decision bool
	objects := s.deathQueryObjects()
	for n, h := range values {
		qa, qb := deathQuery(a), deathQuery(b)
		qa.Height, qb.Height = int16(h), int16(h)
		x, err := s.assets.ContactHeight(s.init.World, qa, objects)
		if err != nil {
			return false, err
		}
		y, err := s.assets.ContactHeight(s.init.World, qb, objects)
		if err != nil {
			return false, err
		}
		v := limit < int16(x+y)
		if n > 0 && v != decision {
			return false, fmt.Errorf("kind22/23 avoidance depends on shared unwritten BP-30 height")
		}
		decision = v
	}
	return decision, nil
}

// GS2 section1:E894..EE44, one old-kind/phase visit. Source record40 is
// waypoint1; record10 remains the desired user route, never used as a cache.
func (s *State) advanceFollowerRoute(actor int, in Input) error {
	e := &s.init.Entities[actor]
	if e.Participation != 1 {
		s.ResetRouteAvoidance(actor)
		s.ResetNavigation(actor)
	}
	if e.Participation == 0 {
		e.Pose.Angles[0] = uint16(uint32(0xff00) << (e.ControlRaw[8] & 31))
		e.Participation = 1
		s.syncDeath(e)
		return nil
	}
	if e.Participation != 1 {
		return nil
	}
	index := int(int8(e.ControlRaw[0]))
	if index < 1 || index > 3 {
		if s.resilientRoutes && insideWorld(e.Pose) {
			return s.holdInvalidRoute(actor, in, fmt.Sprintf("invalid waypoint index %d; holding position", index), true)
		}
		return invalidWaypoint{e.Kind, index}
	}
	off := 30 + 10*index
	x, z := binary.LittleEndian.Uint32(e.ControlRaw[off:]), binary.LittleEndian.Uint32(e.ControlRaw[off+4:])
	goal, corridor := s.corridorGoal(e)
	if corridor && (goal.H0 != x || goal.H1 != z) {
		corridor = false
	}
	if s.resilientRoutes && insideWorld(e.Pose) && (x >= worldEdge || z >= worldEdge) && !(s.admitExitWaypoint && corridor) {
		return s.holdInvalidRoute(actor, in, "route destination outside the usable grid; holding position", true)
	}
	dx := int16((int32(x) >> 4) - (int32(e.Pose.H0) >> 4))
	dz := int16((int32(z) >> 4) - (int32(e.Pose.H1) >> 4))
	heading := s.angle(-dx, dz)
	ax, az := int16(abs(dx)), int16(abs(dz))
	distance := int16(ax + (az >> 1))
	if az > ax {
		distance = az + (ax >> 1)
	}
	canExit := corridor && s.init.Stores != nil && actor < len(s.init.Stores.Damage) && s.init.Stores.Damage[actor][9] == 0
	if uint16(distance) < 8 && (!corridor || canExit) {
		s.ResetRouteAvoidance(actor)
		s.ResetNavigation(actor)
		if index < 3 && binary.LittleEndian.Uint32(e.ControlRaw[off+10:]) != 0 {
			e.ControlRaw[0]++
		} else {
			if e.Kind == 0x22 {
				if canExit {
					s.returnThroughCorridor(actor)
					return nil
				}
				if !s.corridorNavigation && s.init.Switches&0x800 != 0 && binary.LittleEndian.Uint32(e.ControlRaw[10:]) == binary.LittleEndian.Uint32(s.init.Targets.Records[0][34:]) {
					e.Kind, e.ControlRaw[1] = 0x41, 0
					s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: 12, Source: "EA4A:corridor returned"})
					s.syncDeath(e)
					return nil
				}
				e.Pose.H0, e.Pose.H1 = x, z
				e.Kind, e.ControlRaw[1] = 0x26, 6
			} else {
				e.Kind, e.Participation, e.ControlRaw[1] = 0x20, 0, 2
			}
			e.ControlRaw[0] = 0
			s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: 1, Source: "EB1B:route complete"})
		}
		s.syncDeath(e)
		return nil
	}
	// A7E6 changes the turn owner if a usable assignment was reached. Damage
	// successors still complete the old controller's remaining source visit.
	if err := s.advanceHealthyFollower(actor, in); err != nil {
		return err
	}
	holdPosition := false
	if !s.healthyFollowerDecision {
		p, q := e.Pose, e.Pose
		p.H0 -= uint32(int32(s.sin(heading, 0x1000)))
		p.H1 += uint32(int32(s.cos(heading, 0x1000)))
		q.H0 -= uint32(int32(s.sin(heading, 0x400)))
		q.H1 += uint32(int32(s.cos(heading, 0x400)))
		if s.corridorNavigation {
			p = gridProbe(e.Pose, p)
			q = gridProbe(e.Pose, q)
		}
		blocked, err := s.followerRouteBlocked(p, q, int16(e.Pose.Height+16))
		if err != nil {
			return err
		}
		originalBlocked := blocked
		navHeading, navActive := s.navigationHeading(actor, e, heading, x, z, blocked, in)
		holdPosition = s.corridorNavigation && (originalBlocked || s.navigation[actor].Clearance > 0) && s.navigation[actor].Failed
		var offset int16
		if navActive {
			blocked, offset = originalBlocked, sourceAvoidanceOffset(in.Phase)
		} else if s.waypointReleaseEnabled {
			blocked, offset = s.waypointRelease(actor, e, heading, x, z, blocked, in.Phase)
		} else {
			blocked = s.orderedAvoidance(actor, e, heading, x, z, blocked)
		}
		if blocked && int16(abs(int16(e.Pose.Angles[1]-heading))) < 0x4000 {
			turn := uint16(0xe000)
			if in.Phase&0x200 != 0 {
				turn = 0x2000
			}
			if s.waypointReleaseEnabled {
				heading -= uint16(offset)
			} else {
				heading -= turn
			}
			// A native release tail owns heading only. The original strict
			// blocked decision still exclusively owns this first speed arm.
			if !s.waypointReleaseEnabled || originalBlocked {
				e.RawMotion = (e.RawMotion >> 1) + 32
			}
		}
		if navActive {
			heading = navHeading
		}
		diff := int16(e.Pose.Angles[1]-heading) >> 2
		e.Pose.Angles[2] = uint16(diff)
		e.Pose.Angles[1] -= uint16(diff)
	} else {
		s.ResetRouteAvoidance(actor)
		s.ResetNavigation(actor)
	}
	if int16(abs(int16(e.Pose.Angles[2]))) < 64 {
		e.RawMotion = (s.sourceFollowerSpeed(binary.LittleEndian.Uint16(e.ControlRaw[8:])) >> 1) + (e.RawMotion >> 1)
	} else {
		e.RawMotion = (e.RawMotion >> 1) + 32
	}
	old, pitch := e.Pose, e.Pose.Angles[0]
	e.Pose.Angles[0] = 0
	if holdPosition {
		e.RawMotion = 0
	} else {
		s.move(&e.Pose, e.RawMotion>>1)
	}
	e.Pose.Angles[0] = pitch
	if canExit && e.Kind == 0x22 && crossedCorridor(old, e.Pose, goal) {
		e.Pose = gridProbe(old, e.Pose)
		s.returnThroughCorridor(actor)
		return nil
	}
	if s.corridorNavigation && insideWorld(old) && !insideWorld(e.Pose) {
		// Reject just this movement. Keep the real aircraft, heading and assignment.
		e.Pose.H0, e.Pose.H1 = old.H0, old.H1
		s.ResetNavigation(actor)
	}
	s.retainDispatcherHeight(e.Pose.Height) // ED3B..ED47 full current-pose copy before altitude owner.
	alt := s.sourceFollowerAltitude(binary.LittleEndian.Uint16(e.ControlRaw[6:]))
	if s.init.Switches&0x1000 != 0 {
		cell, err := s.assets.SourceCellID(s.init.World, deathQuery(e.Pose))
		if err != nil {
			return err
		}
		if cell == 0x3e {
			e.Pose.Height += uint16(int16(alt-int16(e.Pose.Height)+in.LowLimit+0x168) >> 3)
			s.syncDeath(e)
			return nil
		}
	}
	h, err := s.assets.PairedTerrainHeight(s.init.World, deathQuery(e.Pose), deathQuery(old))
	if err != nil {
		return err
	}
	h += in.LowLimit
	ordinary := h + alt
	delta := int16(ordinary-int16(e.Pose.Height)) >> 3
	floor := s.navigation[actor].Clearance
	if s.lowCityRoute(actor, e) {
		floor = max(floor, s.city.approach(e.Pose, x, z))
	}
	if s.obstacleClearance && floor > 0 && floor > ordinary {
		// Retain the ordinary altitude response. Extra commanded lift per visit
		// cannot exceed the response to the highest normal altitude selection.
		rise := (int32(floor) - int32(int16(e.Pose.Height))) >> 3
		delta = max(delta, int16(min(rise, int32(s.sourceFollowerAltitude(3))>>3)))
	}
	e.Pose.Height += uint16(delta)
	if int16(e.Pose.Height) < h {
		e.Pose.Height = uint16(h)
	}
	s.syncDeath(e)
	return nil
}

func routeCoordinate(v uint32, remainder byte) uint32 {
	d := int32(int16(uint16(remainder) << 8))
	if remainder&1 == 0 {
		d = -d
	}
	x := int32(v) + d
	if x > 0x7c000 {
		x = 0x7c000
	}
	if x < 0x4000 {
		x = 0x4000
	}
	return uint32(x)
}

// FinalizeFollowerRoutes is source section4:19B4..20FC, including unchanged
// and cancelled GPS returns. The caller supplies the live shared LFSR and
// source642 contacts; render redraws never call this owner.
func (s *State) FinalizeFollowerRoutes(random func() uint16, contacts *[80]avionics.Contact) error {
	if !s.ordersEnabled {
		return nil
	}
	m := s.init
	for actor := 1; actor < 5 && actor < len(m.Entities) && actor < len(m.Rows); actor++ {
		if m.Rows[actor].Raw[10]&0x20 == 0 {
			break
		}
		e := &m.Entities[actor]
		if e.ControlCarrierOnly {
			if s.resilientRoutes {
				s.routeWarning[actor] = "GPS route update skipped: flight CONTROL owner unavailable"
				continue
			}
			return fmt.Errorf("GPS return actor%d lacks CONTROL", actor)
		}
		if contacts == nil {
			if s.resilientRoutes {
				s.routeWarning[actor] = "GPS contact table unavailable; existing route retained"
				continue
			}
			return fmt.Errorf("GPS return lacks actual source642 contacts")
		}
		s.ResetRouteAvoidance(actor)
		contacts[actor].Flags &= 0xf8
		copy(e.ControlRaw[40:70], e.ControlRaw[10:40])
		if s.finalizeCorridorRoute(actor) {
			continue
		}
		if e.ControlRaw[1] != 3 {
			continue
		}
		if m.Stores == nil || actor >= len(m.Stores.Ships) || actor >= len(m.CrewPerformance) || m.Stores.Ships[actor].Proficiency == nil || !m.CrewPerformance[actor].SkillKnown {
			if s.resilientRoutes {
				s.routeWarning[actor] = "Waypoint variation skipped: crew proficiency unavailable"
				continue
			}
			return fmt.Errorf("GPS return actor%d lacks source SHIP+1D proficiency", actor)
		}
		skill := *m.Stores.Ships[actor].Proficiency
		if skill == 0 {
			continue
		}
		for point := 0; point < 3; point++ {
			off := 10 + point*10
			if s.routeTargetAvoidance(actor, point) {
				break
			}
			x, z := binary.LittleEndian.Uint32(e.ControlRaw[off:]), binary.LittleEndian.Uint32(e.ControlRaw[off+4:])
			if x == 0 {
				continue
			}
			if random == nil {
				return fmt.Errorf("GPS return requires shared LFSR")
			}
			var draws [4]uint16
			for j := range draws {
				draws[j] = random()
			}
			a, b := byte(draws[0]^draws[1])%skill, byte(draws[2]^draws[3])%skill
			m.ApplyOrdinaryCrewRoutePoint(actor, x, 3, false, draws)
			binary.LittleEndian.PutUint32(e.ControlRaw[off+30:], routeCoordinate(x, a))
			binary.LittleEndian.PutUint32(e.ControlRaw[off+34:], routeCoordinate(z, b))
			binary.LittleEndian.PutUint16(e.ControlRaw[off+38:], 0)
		}
	}
	return nil
}

// Primary is examined before secondary; only the last desired point can
// bypass random variation. Avoidance jumps to the next actor, not next point.
func (s *State) routeTargetAvoidance(actor, point int) bool {
	m := s.init
	if m.Targets == nil {
		return false
	}
	e := &m.Entities[actor]
	if point < 2 && binary.LittleEndian.Uint32(e.ControlRaw[20+point*10:]) != 0 {
		return false
	}
	for _, targetIndex := range []int{2, 3} {
		r := m.Targets.Records[targetIndex]
		kind := binary.LittleEndian.Uint16(r[32:])
		if kind != 1 && kind != 2 {
			continue
		}
		x, z := binary.LittleEndian.Uint32(r[34:]), binary.LittleEndian.Uint32(r[38:])
		off := 10 + point*10
		dx := int16((int32(x) >> 4) - (int32(binary.LittleEndian.Uint32(e.ControlRaw[off:])) >> 4))
		dz := int16((int32(z) >> 4) - (int32(binary.LittleEndian.Uint32(e.ControlRaw[off+4:])) >> 4))
		if uint16(int16(abs(dx))+int16(abs(dz))) >= 0x800 {
			continue
		}
		if int32(e.Pose.H0) < int32(x) {
			x -= 128
		} else {
			x += 128
		}
		if int32(e.Pose.H1) < int32(z) {
			z -= 128
		} else {
			z += 128
		}
		binary.LittleEndian.PutUint32(e.ControlRaw[off+30:], x)
		binary.LittleEndian.PutUint32(e.ControlRaw[off+34:], z)
		return true
	}
	return false
}

// finalizeCorridorRoute preserves the literal 20D0/19BE boundary words,
// including the raw48 tail-word test (not a reconstructed coordinate clamp).
func (s *State) finalizeCorridorRoute(actor int) bool {
	m := s.init
	e := &m.Entities[actor]
	if e.ControlRaw[1] != 8 || m.Switches&0x800 == 0 || m.Targets == nil || binary.LittleEndian.Uint32(e.ControlRaw[10:]) != binary.LittleEndian.Uint32(m.Targets.Records[0][34:]) {
		return false
	}
	if s.corridorNavigation {
		goal, ok := s.corridorGoal(e)
		if !ok {
			return false
		}
		binary.LittleEndian.PutUint32(e.ControlRaw[40:], goal.H0)
		binary.LittleEndian.PutUint32(e.ControlRaw[44:], goal.H1)
		if goal.H0 != 0 && goal.H0 != worldEdge {
			binary.LittleEndian.PutUint16(e.ControlRaw[48:], 0)
		}
		return true
	}
	x := int32(binary.LittleEndian.Uint32(e.ControlRaw[40:]))
	if x < 0x8000 {
		binary.LittleEndian.PutUint32(e.ControlRaw[40:], 0)
	} else if x > 0x78000 {
		binary.LittleEndian.PutUint32(e.ControlRaw[40:], 0x80000)
	} else {
		// CWD makes DX either0 orffff: no signed AX can satisfy the
		// subsequent unsigned8000h/7:8000h range. Every reached word clears.
		binary.LittleEndian.PutUint16(e.ControlRaw[48:], 0)
	}
	return true
}

// FinalizeDirectReturn is the separate Ctrl+B133F..13AD cache epoch. It
// does not apply ordinary F proficiency perturbations or consume random draws.
func (s *State) FinalizeDirectReturn(actor int, contacts *[80]avionics.Contact) error {
	if !s.ordersEnabled {
		return nil
	}
	if actor < 1 || actor >= 5 || actor >= len(s.init.Entities) || contacts == nil {
		return fmt.Errorf("direct return actor/contact owner unavailable")
	}
	e := &s.init.Entities[actor]
	contacts[actor].Flags &= 0xf8
	copy(e.ControlRaw[40:70], e.ControlRaw[10:40])
	s.finalizeCorridorRoute(actor)
	return nil
}
