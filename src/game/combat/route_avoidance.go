package combat

import gamestate "gunship2000-assets/game/state"

// RouteAvoidanceLatch is native R11v3 policy, not an original source field.
// Stage1 is entered by the original blocked decision;2/3 retain it after the
// first/second clear decision. The third consecutive clear decision releases.
type RouteAvoidanceLatch struct {
	Stage       uint8  `json:",omitempty"`
	Kind, Index uint8  `json:",omitempty"`
	H0, H1      uint32 `json:",omitempty"`
	Offset      int16  `json:",omitempty"` // V4 only; zero/omitted in historical V3.
}

type RouteAvoidanceState [5]RouteAvoidanceLatch

func (s *State) SetRouteAvoidanceEnabled(enabled bool) {
	s.routeAvoidanceEnabled = enabled
	s.routeAvoidance = RouteAvoidanceState{}
}

func (s *State) SetWaypointReleaseEnabled(enabled bool) {
	s.waypointReleaseEnabled = enabled
	s.routeAvoidance = RouteAvoidanceState{}
}

func sourceAvoidanceOffset(phase uint16) int16 {
	if phase&0x200 != 0 {
		return 0x2000
	}
	return -0x2000
}

// waypointRelease is V4's heading-only release tail. Source kind22 return and
// every other controller keep the original instantaneous heading decision.
// The original blocked predicate/error is evaluated before this owner.
func (s *State) waypointRelease(actor int, e *gamestate.InitializedEntity, goal uint16, x, z uint32, blocked bool, phase uint16) (bool, int16) {
	full := sourceAvoidanceOffset(phase)
	if actor < 1 || actor > 4 || e.Kind != 0x23 || e.Participation != 1 || int16(abs(int16(e.Pose.Angles[1]-goal))) >= 0x4000 {
		s.ResetRouteAvoidance(actor)
		return blocked, full
	}
	l := &s.routeAvoidance[actor]
	if l.Kind != e.Kind || l.Index != e.ControlRaw[0] || l.H0 != x || l.H1 != z {
		*l = RouteAvoidanceLatch{}
	}
	if blocked {
		*l = RouteAvoidanceLatch{Kind: e.Kind, Index: e.ControlRaw[0], H0: x, H1: z, Offset: full}
	} else if l.Offset > 0 {
		l.Offset = max(int16(0), l.Offset-0x200)
	} else {
		l.Offset = min(int16(0), l.Offset+0x200)
	}
	if l.Offset == 0 {
		*l = RouteAvoidanceLatch{}
		return false, 0
	}
	return true, l.Offset
}

func (s *State) ResetRouteAvoidance(actor int) {
	if actor > 0 && actor < len(s.routeAvoidance) {
		s.routeAvoidance[actor] = RouteAvoidanceLatch{}
	}
}

func (s *State) filmAvoidance() *RouteAvoidanceState {
	if !s.routeAvoidanceEnabled {
		return nil
	}
	q := s.routeAvoidance
	return &q
}

// orderedAvoidance consumes the original proven decision AFTER its query/error
// path. It adds no query, RNG draw or alternate interpretation of BP-30. Only
// release is debounced; original entry and the original +/-90degree gate stay.
func (s *State) orderedAvoidance(actor int, e *gamestate.InitializedEntity, goal uint16, x, z uint32, blocked bool) bool {
	if !s.routeAvoidanceEnabled || actor < 1 || actor > 4 {
		return blocked
	}
	if e.Kind != 0x22 && e.Kind != 0x23 || e.Participation != 1 || int16(abs(int16(e.Pose.Angles[1]-goal))) >= 0x4000 {
		s.ResetRouteAvoidance(actor)
		return blocked
	}
	l := &s.routeAvoidance[actor]
	if l.Kind != e.Kind || l.Index != e.ControlRaw[0] || l.H0 != x || l.H1 != z {
		*l = RouteAvoidanceLatch{}
	}
	if blocked {
		*l = RouteAvoidanceLatch{Stage: 1, Kind: e.Kind, Index: e.ControlRaw[0], H0: x, H1: z}
		return true
	}
	if l.Stage != 0 {
		l.Stage++
		if l.Stage < 4 {
			return true
		}
	}
	*l = RouteAvoidanceLatch{}
	return false
}
