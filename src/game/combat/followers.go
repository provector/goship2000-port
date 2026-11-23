package combat

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

// FollowerFuelContext reads live control/physical records at each ascending burn.
// A remembered launch Active[] cannot represent later disabled/control changes.
func (s *State) FollowerFuelContext(random func() uint16) *flight.FollowerFuelContext {
	m := s.init
	return &flight.FollowerFuelContext{
		Random: random,
		Notify: func(actor int, kind uint16) {
			if s.feedbackEnabled {
				s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: kind, Source: "0350/0366:nonplayer old fuel"})
			}
		},
		Read: func(actor int) flight.FuelActor {
			if actor < 0 || actor >= 5 || actor >= len(m.Entities) || actor >= len(m.Rows) {
				return flight.FuelActor{}
			}
			e := &m.Entities[actor]
			a := flight.FuelActor{Prefix: m.Rows[actor].Raw[10]&0x20 != 0, Control: e.ControlRaw[1], Height: int16(e.Pose.Height)}
			if actor == m.PlayerRow {
				a.Known = true
				return a
			}
			if m.Stores == nil || actor >= len(m.Stores.Damage) || actor >= len(m.Stores.Ships) {
				return a
			}
			a.Damage603 = m.Stores.Damage[actor][3]
			if err := s.followerAdmission(actor); err != nil {
				return a
			}
			if m.Stores.Ships[actor].Proficiency == nil {
				return a
			}
			a.Proficiency = *m.Stores.Ships[actor].Proficiency
			a.Known = true
			return a
		},
		Set609: func(actor int, value byte) { m.FollowerFailureKnown[actor] = true; m.Stores.Damage[actor][9] = value },
	}
}

func (s *State) followerProvenance(actor int) error {
	m := s.init
	if actor < 1 || actor > 4 || m.PlayerRow != 0 || len(m.FlightRows) != 5 || len(m.Entities) < 9 || len(m.Rows) < 5 || m.Targets == nil || m.Stores == nil || !m.Stores.FreshShipPadding || len(m.Stores.Damage) != 5 || len(m.Stores.Blueprint) != 5 {
		return fmt.Errorf("follower source actor/prefix/TARGET/SHIP provenance missing")
	}
	for i := 0; i <= actor; i++ {
		if m.FlightRows[i] != i || m.Entities[i].RuntimeRow != i || m.Rows[i].Raw[10]&0x20 == 0 {
			return fmt.Errorf("follower source flight prefix missing")
		}
	}
	return nil
}

func (s *State) followerAdmission(actor int) error {
	if err := s.followerProvenance(actor); err != nil {
		return err
	}
	if s.logisticsEnabled {
		return s.logisticsAdmission(actor)
	}
	m := s.init
	e := &m.Entities[actor]
	if !m.FollowerFailureKnown[actor] && (e.Kind != 0x20 && !(s.formationEnabled && e.Kind == 0x21) || e.Participation != 1) {
		return fmt.Errorf("follower kind%02x/P%d is outside admitted disabled caller", e.Kind, e.Participation)
	}
	if e.Kind != 0x20 && e.Kind != 0x26 && !(s.formationEnabled && e.Kind == 0x21) {
		return fmt.Errorf("follower changed to unsupported kind%02x", e.Kind)
	}
	if binary.LittleEndian.Uint16(e.ControlRaw[6:]) > 3 {
		return fmt.Errorf("follower altitude selector is unsupported")
	}
	h, err := s.assets.ContactHeight(m.World, deathQuery(e.Pose), s.deathQueryObjects())
	if err != nil {
		return err
	}
	if h != 0 {
		return fmt.Errorf("follower support%d is outside source query0 admission", h)
	}
	return nil
}

// AdvanceFollowers replaces AdvanceDeaths in the existing ascending source157f
// entity phase. It performs exactly one old-kind/P visit; kind changes are never
// immediately redispatched. No healthy NPC route, target or firing AI is called.
func (s *State) AdvanceFollowers(in Input, systems *flight.Systems) ([]int, error) {
	if s.logisticsEnabled {
		return s.advanceLogisticsFollowers(in, systems)
	}
	var observed []int
	m := s.init
	for row := range m.Entities {
		e := &m.Entities[row]
		if s.formationEnabled && row >= 1 && row <= 4 && e.Kind == 0x21 {
			if m.Stores != nil && row < len(m.Stores.Damage) && m.Stores.Damage[row][9] != 0 {
				if err := s.followerAdmission(row); err != nil {
					systems.UnsupportedFollowerEmptyBurn |= 1 << uint(row)
					m.FollowerLimits[row] = err.Error()
					continue
				}
			}
			if err := s.advanceFormation(row, in); err != nil {
				systems.UnsupportedFollowerEmptyBurn |= 1 << uint(row)
				m.FollowerLimits[row] = err.Error()
			}
			continue
		}
		if row >= 1 && row <= 4 && m.FollowerFailureKnown[row] && e.Kind == 0x20 && e.Participation == 0 {
			if err := s.followerAdmission(row); err != nil {
				systems.UnsupportedFollowerEmptyBurn |= 1 << uint(row)
				m.FollowerLimits[row] = err.Error()
				continue
			}
			// SourceDE0a's P0 successor owns initialization, not a second P1
			// callback. This closes the post-contact ground0 continuation.
			e.RawMotion = 0
			e.Pose.Angles[0], e.Pose.Angles[2] = 0, 0
			e.Participation = 1
			if int16(e.Pose.Height) < 2 {
				e.Pose.Height = 2
			}
			s.syncDeath(e)
			continue
		}
		if row >= 1 && row <= 4 && m.Stores != nil && row < len(m.Stores.Damage) && m.Stores.Damage[row][9] != 0 && e.Participation == 1 && (e.Kind == 0x20 || e.Kind == 0x26) {
			if err := s.followerAdmission(row); err != nil {
				systems.UnsupportedFollowerEmptyBurn |= 1 << uint(row)
				m.FollowerLimits[row] = err.Error()
				continue
			}
			switch e.Kind {
			case 0x20:
				s.disableFollower(row)
				// DEEF continues the OLD kind20 geometry even after its callback changes
				// kind/P. DATA272d:2fb2 is the source altitude selector0/26/192/448 table.
				height := [4]int16{0, 26, 192, 448}[binary.LittleEndian.Uint16(e.ControlRaw[6:])]
				if int16(e.Pose.Height) < 2 {
					e.Pose.Height = 2
				}
				e.Pose.Height += uint16((height - int16(e.Pose.Height) + 2) >> 3)
				s.syncDeath(e)
			case 0x26:
				if err := s.descendFollower(row, systems); err != nil {
					return observed, err
				}
			}
			continue
		}
		if row >= 1 && row <= 4 && m.FollowerFailureKnown[row] && e.Kind == 0x26 && e.Participation == 1 {
			if err := s.followerAdmission(row); err != nil {
				systems.UnsupportedFollowerEmptyBurn |= 1 << uint(row)
				m.FollowerLimits[row] = err.Error()
				continue
			}
			if err := s.descendFollower(row, systems); err != nil {
				return observed, err
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

func (s *State) disableFollower(actor int) {
	m := s.init
	e := &m.Entities[actor]
	d := m.Stores.Damage[actor][9]
	if d == 0 || e.ControlRaw[1] == 0 {
		return
	}
	if int16(e.Pose.Height) > 2 {
		s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: 3, Source: "A7e6:disabled"})
	}
	if d == 1 {
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
		e.Participation = 2
		e.RawMotion = 4
	} else {
		e.Kind = 0x26
		e.ControlRaw[1] = 6
	}
	e.ControlRaw[0], e.ControlRaw[3] = 0, 0
	for _, off := range []int{10, 20, 30} {
		clear(e.ControlRaw[off : off+8])
	} // Source tails49a/4a4/4ae survive.
	for j := actor + 1; j < 5 && j < len(m.Entities); j++ {
		child := &m.Entities[j]
		if child.ControlRaw[1] == 1 && int(int8(child.ControlRaw[5])) == actor {
			child.ControlRaw[1] = 2
			if s.safeFlightOrders {
				// A8FF uses the matching leader left in AX for A902's DYN
				// write, although A90C changes this child's CONTROL. R5 keeps
				// the parent's failure state and gives the held child its own
				// kind20 controller. Historical profiles retain the DOS quirk.
				child.Kind = 0x20
			} else {
				e.Kind = 0x20
			}
		} // Source AX is leader a, not dependent j.
	}
}

func (s *State) descendFollower(actor int, systems *flight.Systems) error {
	m := s.init
	e := &m.Entities[actor]
	e.RawMotion = 0
	// F4d2 NEG/SAR3/INC and word storage; no DF coefficient.
	d := int16(-int16(0 - int16(e.Pose.Height)))
	d = (d >> 3) + 1
	e.Pose.Height -= uint16(d)
	if int16(e.Pose.Height) > 0 {
		s.syncDeath(e)
		return nil
	}
	e.Pose.Height = 0
	e.Participation = 0
	e.Kind = 0x20
	e.ControlRaw[1] = 2
	binary.LittleEndian.PutUint16(e.ControlRaw[6:], 0)
	damage := &m.Stores.Damage[actor]
	if damage[9] == 2 {
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
	} else if damage[9] == 0 {
		s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: 8, Source: "F677:landed"})
	}
	s.syncDeath(e)
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
func targetAnchor(t [62]byte) gamestate.InitializedPose {
	return gamestate.InitializedPose{H0: binary.LittleEndian.Uint32(t[42:]), H1: binary.LittleEndian.Uint32(t[46:])}
}

func (s *State) reserveIdentity(row int) error {
	m := s.init
	if m.PlayerRow != 0 || len(m.FlightRows) != 5 || row < 5 || row > 8 || row >= len(m.Entities) || m.FlightRows[row-4] != row-4 || m.Entities[row].RuntimeRow != row || m.Entities[row].Object.Raw[10] != 0x1b {
		return fmt.Errorf("reserve row%d lacks known actor+4 source identity", row)
	}
	return nil
}
func (s *State) advanceReservedPose(row int) error {
	m := s.init
	e := &m.Entities[row]
	// Non-Flight missions can contain an unrelated kind1b; do not manufacture a
	// survivor from it. Existing inactive source objects retain their old behavior.
	if err := s.reserveIdentity(row); err != nil {
		if e.Participation == 0 {
			return nil
		}
		return err
	}
	if e.Participation == 0 {
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
		e.RawMotion = 30
		e.Pose.Height = 0
		s.syncDeath(e)
		return nil
	}
	p := m.Entities[m.PlayerRow].Pose
	dx, dz := delta(e.Pose.H0, p.H0), delta(e.Pose.H1, p.H1)
	desired := world.SourceAngle(dx, -dz, s.assets)
	difference := e.Pose.Angles[1] - desired
	heading := e.Pose.Angles[1] + uint16(int16(int8(difference>>8)))
	e.Pose.Angles[1] = heading
	basis := m.Entities[row-4].Pose
	e.Pose.H0 = basis.H0 + uint32(int32(s.sin(heading, 256)))
	e.Pose.H1 = basis.H1 - uint32(int32(s.cos(heading, 256)))
	e.Exclusion = ""
	s.syncDeath(e)
	return nil
}

func (s *State) advancePlayerReservePickup(in Input) {
	m := s.init
	actor := m.PlayerRow
	if actor != 0 || len(m.FlightRows) != 5 || m.Stores == nil || len(m.Stores.Ships) != 5 || m.Stores.Ships[actor].Flags&7 != 6 || len(m.Entities) < 9 {
		return
	}
	carrier := &m.Entities[actor]
	// The bounded manual case is fresh playerP0. P1's additional order-choice
	// successors are not silently interpreted as the player branch.
	if carrier.Participation != 0 || int16(carrier.Pose.Height) >= 16 || in.PlayerIndicatedSpeed >= 16 {
		return
	}
	for a := 1; a < 5; a++ {
		if a >= len(m.Rows) || m.Rows[a].Raw[10]&0x20 == 0 {
			return
		}
		row := a + 4
		e := &m.Entities[row]
		if e.Kind != 0x1b || e.Participation != 1 || s.reserveIdentity(row) != nil {
			continue
		}
		if gamestate.FinalResultMetric(e.Pose, carrier.Pose) >= 128 {
			continue
		}
		e.Kind, e.DynamicRaw[0], e.DynamicRaw[3] = 0x41, 0x41, byte(actor)
		s.syncDeath(e)
		s.emit("objective-contact-kind", -1, actor, row, 0x41, "6b53:reserve pickup")
		s.appendResult(Result{Kind: 6, Actor: byte(actor), Target: byte(a), Timestamp: in.Phase})
		s.Events = append(s.Events, Event{Kind: "notification", Actor: a, Notification: 21, Source: "6b99:player recovered crew"})
		// Source actor is advanced after one pickup; no cargo/mass/TARGET mutation.
		return
	}
}
