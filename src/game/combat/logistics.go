package combat

import (
	"encoding/binary"
	"errors"
	"fmt"
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

// SetLogisticsEnabled chooses the P4 source interpretation before initialization
// is frozen. Historical launches leave ActorFlags absent, preserving their hashes.
func (s *State) SetLogisticsEnabled(enabled bool) {
	s.logisticsEnabled = enabled
	if enabled && s.init.ActorFlags == nil {
		s.init.ActorFlags = make([]byte, 5)
	}
	if !enabled {
		s.init.ActorFlags = nil
	}
}
func (s *State) LogisticsEnabled() bool { return s.logisticsEnabled }

// DATA272d:2fb2/2fba, independently checked in GS2.GS2 (SHA1a1db960...)
// at file4b7a2/4b7aa. The following bytes are strings, not more selectors.
var followerAltitude = [...]int16{0, 26, 192, 448}
var followerSpeed = [...]int16{0, 48, 96, 144}

func (s *State) logisticsAdmission(actor int) error {
	if len(s.init.Stores.Ships) != 5 || len(s.init.ActorFlags) != 5 {
		return fmt.Errorf("follower SHIP/source273 actor ownership is incomplete")
	}
	e := &s.init.Entities[actor]
	if e.ControlCarrierOnly {
		return fmt.Errorf("follower row%d CONTROL carrier unavailable", actor)
	}
	switch e.Kind {
	case 0x20, 0x21, 0x24, 0x25, 0x26:
	case 0x22, 0x23:
		if !s.ordersEnabled {
			return fmt.Errorf("follower route requires R1 interpretation")
		}
	default:
		return fmt.Errorf("follower changed to unsupported kind%02x", e.Kind)
	}
	if binary.LittleEndian.Uint16(e.ControlRaw[6:]) >= uint16(len(followerAltitude)) {
		return invalidFlightOrder("follower altitude selector is unsupported")
	}
	if binary.LittleEndian.Uint16(e.ControlRaw[8:]) >= uint16(len(followerSpeed)) {
		return invalidFlightOrder("follower speed selector is unsupported")
	}
	_, err := s.assets.ContactHeight(s.init.World, deathQuery(e.Pose), s.deathQueryObjects())
	return err
}

func (s *State) advanceLogisticsFollowers(in Input, systems *flight.Systems) ([]int, error) {
	// B01C ENTER creates one frame for this entire ascending actor loop. R4's
	// ordinary reached25E2 writer donates the exact pre-easing BP-30 word. A nil
	// predecessor remains unknown; later full-pose writers/invalidation below
	// keep their source precedence. Never leak a previous native invocation.
	s.dispatcherScratchKnown = false
	if s.dispatcherScratchEnabled && in.PlayerDesiredVerticalRate != nil {
		s.dispatcherScratchKnown = true
		s.dispatcherScratchHeight = *in.PlayerDesiredVerticalRate
	}
	var observed []int
	m := s.init
	for row := range m.Entities {
		e := &m.Entities[row]
		if e.Kind != 0x22 && e.Kind != 0x23 {
			s.ResetRouteAvoidance(row)
		}
		if s.dispatcherScratchEnabled && row < 5 && e.Kind != 0xff && e.Kind != 0x41 && (e.Kind < 0x20 || e.Kind > 0x26) {
			// A non-flight controller may use the same parent-frame temporary.
			// Until its reached writer is reconstructed it cannot donate or
			// preserve a stale earlier-flight operand for a subsequent actor.
			s.dispatcherScratchKnown = false
		}
		if s.missionActorsEnabled && e.Kind == 0 {
			if err := s.advanceOrdinaryHelicopter(row, in); err != nil {
				return observed, err
			}
			continue
		}
		if s.missionActorsEnabled && e.Kind == 1 {
			if err := s.advanceOrdinaryAircraft(row, in); err != nil {
				return observed, err
			}
			continue
		}
		if s.missionActorsEnabled && e.Kind == 0x40 {
			if err := s.advanceRouteHeader(row, in); err != nil {
				return observed, err
			}
			continue
		}
		if s.enemyBehaviorEnabled && e.Kind == 0x0a {
			if err := s.advanceSubmarine(row, in); err != nil {
				return observed, err
			}
			continue
		}
		if s.missionActorsEnabled && e.Kind >= 2 && e.Kind <= 7 {
			if err := s.advanceGroundRoute(row, in); err != nil {
				return observed, err
			}
			continue
		}
		if s.trainEnabled && e.Kind == 8 {
			if err := s.advanceTrainRoute(row, in); err != nil {
				return observed, err
			}
			continue
		}
		if s.missionActorsEnabled && e.Kind == 0x11 && m.Support != nil {
			if err := s.UpdateAirSupport(row, in); err != nil {
				return observed, err
			}
			continue
		}
		if e.Kind == 0x19 && m.Support != nil {
			if err := s.UpdateArtillery(row, in); err != nil {
				return observed, err
			}
			continue
		}
		if row >= 1 && row <= 4 && (e.Kind == 0x20 || e.Kind == 0x21 || s.ordersEnabled && (e.Kind == 0x22 || e.Kind == 0x23) || e.Kind == 0x24 || e.Kind == 0x25 || e.Kind == 0x26) {
			if e.Participation == 2 {
				// DF78/E86D/EE20 call2AB8 with actor only. Its separate
				// frame cannot overwrite the caller's BP-38 temporary.
				changed, err := s.advanceDeathRow(row, in)
				if err != nil {
					return observed, err
				}
				if changed {
					observed = append(observed, row)
				}
				continue
			}
			if err := s.followerProvenance(row); err != nil {
				s.dispatcherScratchKnown = false
				s.excludeFollower(row, systems, err)
				continue
			}
			if err := s.logisticsAdmission(row); err != nil {
				var order invalidFlightOrder
				if s.resilientRoutes && e.Kind <= 0x23 && insideWorld(e.Pose) && errors.As(err, &order) {
					err = s.holdInvalidRoute(row, in, err.Error(), e.Participation == 1)
					if err == nil {
						continue
					}
				}
				s.dispatcherScratchKnown = false
				s.excludeFollower(row, systems, err)
				continue
			}
			before, orderKind := e.Pose, e.Kind
			var err error
			switch e.Kind {
			case 0x21:
				err = s.advanceFormation(row, in)
			case 0x22, 0x23:
				err = s.advanceFollowerRoute(row, in)
			case 0x24:
				err = s.advanceCleanup(row, in)
			case 0x25:
				err = s.advanceOrphan(row, in)
			case 0x26:
				if e.Participation == 1 {
					err = s.advanceApproach(row, in, systems)
				}
			case 0x20:
				err = s.advanceIdleFollower(row, in)
			}
			if err != nil {
				s.dispatcherScratchKnown = false
				var order invalidFlightOrder
				if s.resilientRoutes && orderKind == 0x21 && insideWorld(before) && errors.As(err, &order) {
					e.Pose = before
					err = s.holdInvalidRoute(row, in, err.Error(), e.Participation == 1)
					if err == nil {
						continue
					}
				}
				var outside world.OutsideGridError
				if s.corridorNavigation && orderKind >= 0x21 && orderKind <= 0x25 && insideWorld(before) && errors.As(err, &outside) && m.Stores.Damage[row][9] == 0 {
					// Only the movement/probe failed. Keep reached command, damage, RNG
					// and objective effects. Retry from the last real body position.
					if s.resilientRoutes {
						s.routeWarning[row] = "Rejected movement outside the terrain grid; retained valid aircraft pose"
					}
					e.Pose = before
					s.syncDeath(e)
					s.ResetNavigation(row)
					continue
				}
				s.excludeFollower(row, systems, err)
			}
			continue
		}
		if row >= 5 && row <= 8 && e.Kind == 0x1b && (e.Participation == 0 || e.Participation == 1) {
			if err := s.advanceReservedPose(row); err != nil {
				return observed, err
			}
			continue
		}
		changed, err := s.advanceDeathRow(row, in)
		if err != nil {
			return observed, err
		}
		if changed {
			observed = append(observed, row)
		}
	}
	return observed, nil
}

// Kind20 DE0A..DF9C owns support as well as idle height. The old ground0-only
// interpretation remains in followers.go; source q is not a fixed zero or AGL.
func (s *State) advanceIdleFollower(actor int, inputs ...Input) error {
	m := s.init
	e := &m.Entities[actor]
	phase := e.Participation
	if phase != 0 && phase != 1 {
		return nil
	}
	if phase == 0 {
		e.RawMotion = 0
		e.Pose.Angles[0], e.Pose.Angles[2] = 0, 0
		e.Participation = 1
	} else {
		// DEEF continues the old Kind20 geometry even if this callback changes
		// kind/P. No immediate second visit to the resulting Kind26/P2 occurs.
		if s.missionActorsEnabled && len(inputs) > 0 {
			if err := s.advanceHealthyFollower(actor, inputs[0]); err != nil {
				return err
			}
		} else {
			s.disableFollower(actor)
		}
	}
	s.retainDispatcherHeight(e.Pose.Height) // DE45 / DF05 full pose before minimum/idle altitude.
	q, err := s.assets.ContactHeight(m.World, deathQuery(e.Pose), s.deathQueryObjects())
	if err != nil {
		return err
	}
	minimum := q + 2
	if int16(e.Pose.Height) < minimum {
		e.Pose.Height = uint16(minimum)
	}
	if phase == 0 {
		if m.Switches&0x1000 != 0 {
			cell, err := s.assets.SourceCellID(m.World, deathQuery(e.Pose))
			if err != nil {
				return err
			}
			if cell == 0x3e && e.Pose.Height == 0x16a {
				// DEC7 reads only source row0 H0; H1 remains this actor's.
				e.Pose.H0 = binary.LittleEndian.Uint32(m.Rows[0].Raw[2:6])
				e.Pose.Angles[1] = 0
			}
		}
	} else {
		h := s.sourceFollowerAltitude(binary.LittleEndian.Uint16(e.ControlRaw[6:]))
		e.Pose.Height += uint16(int16(h-int16(e.Pose.Height)+q+2) >> 3)
	}
	s.syncDeath(e)
	return nil
}
func (s *State) excludeFollower(actor int, systems *flight.Systems, err error) {
	// Modern navigation handles recoverable movements locally. Reserve the fuel
	// witness for its actual writer; retain specific essential failures by row.
	var wp invalidWaypoint
	if errors.As(err, &wp) && insideWorld(s.init.Entities[actor].Pose) {
		s.historicalRouteOnly[actor] = true
		s.routeWarning[actor] = err.Error()
	} else {
		s.unresolvedFollower[actor] = true
	}
	if !s.corridorNavigation {
		systems.UnsupportedFollowerEmptyBurn |= 1 << uint(actor)
	}
	if s.init.FollowerLimits[actor] == "" {
		s.init.FollowerLimits[actor] = err.Error()
	}
}

// followerForward preserves the source's two separate SAR halves and old/new
// paired-query order. Kind24/25 use SAR3; Kind26 fallback uses SAR4 then DEC.
func (s *State) followerForward(actor int, in Input, shift uint, decrement bool, damagedFallback bool) error {
	e := &s.init.Entities[actor]
	old := e.Pose
	speed := binary.LittleEndian.Uint16(e.ControlRaw[8:])
	altitude := binary.LittleEndian.Uint16(e.ControlRaw[6:])
	if speed >= uint16(len(followerSpeed)) || altitude >= uint16(len(followerAltitude)) {
		return fmt.Errorf("follower movement selector outside source tables")
	}
	e.RawMotion = (s.sourceFollowerSpeed(speed) >> 1) + (e.RawMotion >> 1)
	pitch := e.Pose.Angles[0]
	e.Pose.Angles[0] = 0
	s.move(&e.Pose, e.RawMotion>>1)
	e.Pose.Angles[0] = pitch
	s.retainDispatcherHeight(e.Pose.Height) // F089/F37E/F866 full moved pose before altitude/damage.
	if damagedFallback && s.init.Stores.Damage[actor][9] != 0 {
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
		e.Participation = 2
		e.RawMotion = 4
		s.syncDeath(e)
		return nil
	}
	q, err := s.assets.PairedTerrainHeight(s.init.World, deathQuery(e.Pose), deathQuery(old))
	if err != nil {
		return err
	}
	q += in.LowLimit
	d := int16(q+s.sourceFollowerAltitude(altitude)-int16(e.Pose.Height)) >> shift
	if decrement {
		d--
	}
	e.Pose.Height += uint16(d)
	if int16(e.Pose.Height) < q {
		e.Pose.Height = uint16(q)
	}
	s.syncDeath(e)
	return nil
}

func (s *State) advanceApproach(actor int, in Input, systems *flight.Systems) error {
	m := s.init
	e := &m.Entities[actor]
	q, err := s.assets.ContactHeight(m.World, deathQuery(e.Pose), s.deathQueryObjects())
	if err != nil {
		return err
	}
	landing := q <= 0
	if q > 0 && m.Switches&0x1000 != 0 && q == 0x168 {
		cell, err := s.assets.SourceCellID(m.World, deathQuery(e.Pose))
		if err != nil {
			return err
		}
		landing = cell == 0x3e
	}
	if !landing {
		return s.followerForward(actor, in, 4, true, true)
	}
	if q == 0x168 {
		e.Pose.Angles[1] = uint16(int16(e.Pose.Angles[1]) >> 1)
	}
	e.RawMotion = 0
	d := -int16(q - int16(e.Pose.Height))
	d = (d >> 3) + 1
	e.Pose.Height -= uint16(d)
	if int16(e.Pose.Height) > q {
		s.syncDeath(e)
		return nil
	}
	e.Pose.Height = uint16(q)
	if q == -1 {
		e.Pose.Height = 8
	}
	e.Participation, e.Kind, e.ControlRaw[1] = 0, 0x20, 2
	binary.LittleEndian.PutUint16(e.ControlRaw[6:], 0)
	damage := &m.Stores.Damage[actor]
	if damage[9] != 0 && q != 0 {
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
		e.Participation = 2
		e.RawMotion = 4
		damage[9] = 1
		e.ControlRaw[1] = 0
	} else if damage[9] == 2 && q == 0 {
		damage[9] = 0
		if gamestate.FinalResultMetric(e.Pose, targetAnchor(m.Targets.Records[0])) > 768 {
			if err := s.reserveIdentity(actor + 4); err != nil {
				return err
			}
			e.ControlRaw[1] = 0
			reserve := &m.Entities[actor+4]
			reserve.Participation = 1
			reserve.Exclusion = ""
			s.syncDeath(reserve)
			s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: 22, Source: "F656:reserve/SOS"})
		}
	} else if damage[9] == 0 && q == 0 {
		s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: 8, Source: "F677:landed"})
	}
	s.syncDeath(e)
	return s.serviceFollowerContact(actor, systems)
}

func (s *State) serviceFollowerContact(actor int, systems *flight.Systems) error {
	m := s.init
	e := &m.Entities[actor]
	near := gamestate.FinalResultMetric(e.Pose, targetAnchor(m.Targets.Records[0])) < 128
	farp := m.Targets.Records[1]
	losses := int16(0)
	if m.Sortie != nil {
		losses = m.Sortie.FARPLosses
	}
	if !near && binary.LittleEndian.Uint16(farp[32:]) != 0 && losses < int16(binary.LittleEndian.Uint16(farp[50:])) {
		near = gamestate.FinalResultMetric(e.Pose, targetAnchor(farp)) < 128
	}
	if near {
		return m.RefillServiceActor(actor, systems)
	}
	return nil
}

func (s *State) advanceCleanup(actor int, in Input) error {
	e := &s.init.Entities[actor]
	if e.Participation == 0 {
		e.Pose.Angles[0] = uint16(uint32(0xff00) << (e.ControlRaw[8] & 31))
		e.Participation = 1
		s.syncDeath(e)
		return nil
	}
	if e.Participation != 1 {
		return nil
	}
	_, leader, _, err := s.formationOwners(actor)
	if err != nil {
		return err
	}
	if s.followExitedSection(actor, leader) {
		return nil
	}
	dx := int16(int32(leader.Pose.H0)>>4) - int16(int32(e.Pose.H0)>>4)
	dz := int16(int32(leader.Pose.H1)>>4) - int16(int32(e.Pose.H1)>>4)
	desired := s.angle(-dx, dz)
	diff := int16(e.Pose.Angles[1]-desired) >> 2
	e.Pose.Angles[2] = uint16(diff)
	e.Pose.Angles[1] -= uint16(diff)
	x, z := abs(dx), abs(dz)
	metric := x + (z >> 1)
	if z > x {
		metric = z + (x >> 1)
	}
	if uint16(metric) < 64 {
		if s.init.Switches&0x800 != 0 && leader.Kind == 0x41 {
			e.Kind = 0x41
			e.ControlRaw[1] = 0
		} else {
			e.Kind = 0x21
			e.ControlRaw[1] = 1
			binary.LittleEndian.PutUint16(e.ControlRaw[6:], 0)
			binary.LittleEndian.PutUint16(e.ControlRaw[8:], 0)
			s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: 7, Source: "EFDF:formation resumed"})
		}
		s.syncDeath(e)
		return nil
	}
	return s.followerForward(actor, in, 3, false, false)
}

func (s *State) advanceOrphan(actor int, in Input) error {
	e := &s.init.Entities[actor]
	if e.Participation == 0 {
		e.Participation = 1
		e.DynamicRaw[3] = 30
		s.syncDeath(e)
		return nil
	}
	if e.Participation != 1 {
		return nil
	}
	ownTarget := int(int8(e.ControlRaw[3]))
	target := ownTarget
	if target == 0 {
		for child := actor + 1; child < 5 && child < len(s.init.Entities); child++ {
			if s.init.Rows[child].Raw[10]&0x20 == 0 {
				break
			}
			c := &s.init.Entities[child]
			if c.ControlRaw[1] == 1 && int(int8(c.ControlRaw[5])) == actor && int8(c.ControlRaw[3]) > 0 {
				target = int(int8(c.ControlRaw[3]))
				break
			}
		}
	}
	if target > 0 {
		if ownTarget == 0 {
			if !s.safeFlightOrders {
				return fmt.Errorf("kind25 parent target0/childpositive requires undefined pre-row0 target provenance")
			}
			// R5: an order to disengage is not a designation. Preserve heading
			// while the formed child remains engaged; no cache/child target pose.
			e.Pose.Angles[2] = 0
		} else if ownTarget < 1 || ownTarget > len(s.init.Entities) {
			return fmt.Errorf("kind25 actual target%d outside dynamic rows", ownTarget)
		} else {
			t := &s.init.Entities[ownTarget-1]
			dx, dz := Difference(t.Pose.H0, e.Pose.H0), Difference(t.Pose.H1, e.Pose.H1)
			desired := s.angle(-dx, dz) + 0x5000
			diff := int16(e.Pose.Angles[1]-desired) >> 2
			e.Pose.Angles[2] = uint16(diff)
			e.Pose.Angles[1] -= uint16(diff)
		}
		e.DynamicRaw[3] = 60
	} else if target == 0 {
		if e.DynamicRaw[3] == 0 {
			e.Participation, e.Kind, e.ControlRaw[1] = 0, 0x20, 2
			s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: 9, Source: "F2BF:orphan timeout"})
		} else if in.E0 {
			e.DynamicRaw[3]--
		}
	}
	if err := s.followerForward(actor, in, 3, false, false); err != nil {
		return err
	}
	// F3EA's disabled callback is source-known; its healthy attack constructor
	// is not fabricated by this bounded deterministic movement implementation.
	if s.missionActorsEnabled {
		if err := s.advanceHealthyFollower(actor, in); err != nil {
			return err
		}
	} else {
		s.disableFollower(actor)
	}
	s.syncDeath(e)
	return nil
}
