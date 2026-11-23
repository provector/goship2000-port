package combat

import (
	"encoding/binary"
	"fmt"
)

// advanceHealthyFollower is A7E6's healthy branch, invoked at the existing
// callback phase, after the actual movement. It does not query body LOS again.
func (s *State) advanceHealthyFollower(actor int, in Input) error {
	s.healthyFollowerDecision = false
	if actor < 1 || actor >= 5 || s.init.Stores == nil || actor >= len(s.init.Stores.Ships) || actor >= len(s.init.Stores.Damage) {
		return fmt.Errorf("healthy follower%d lacks actual actor/SHIP/damage owner", actor)
	}
	e, err := s.entity(actor)
	if err != nil {
		return err
	}
	if s.init.Stores.Damage[actor][9] != 0 {
		s.disableFollower(actor)
		return nil
	}
	defer func() {
		if actor >= 1 && actor < 5 && s.Slots[actor+15].Lifetime != 0 {
			s.healthyFollowerDecision = true
		}
	}()
	if s.wingmenAutonomyDisabled {
		return nil
	}
	assigned := int(int8(e.ControlRaw[3]))
	if assigned <= 0 {
		return nil
	}
	if e.ControlRaw[4] == 0 {
		if e.DynamicRaw[3] != 0 {
			if in.E0 {
				e.DynamicRaw[3]--
			}
		} else {
			s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: 10, Source: "AFC2:assigned standby"})
			e.DynamicRaw[3] = byte((actor + 24) * 2)
		}
		s.syncDeath(e)
		return nil
	}
	target, err := s.entity(assigned - 1)
	if err != nil {
		return err
	}
	dx, dz := delta(target.Pose.H0, e.Pose.H0), delta(target.Pose.H1, e.Pose.H1)
	vertical := (int16(target.Pose.Height-e.Pose.Height) + 32) >> 4
	yawError := int16(e.Pose.Angles[1] - s.angle(-dx, dz))
	if abs(yawError) > 0x4000 {
		e.RawMotion >>= 1
	}
	distance := metric(dx, dz)
	if e.DynamicRaw[3] != 0 && in.E0 {
		e.DynamicRaw[3]--
	}
	if e.DynamicRaw[3] != 0 {
		s.syncDeath(e)
		return nil
	}
	class := int(int8(target.Object.Raw[31]))
	effectClass := class
	if effectClass > 2 {
		effectClass = 2
	}
	if effectClass < 0 {
		return fmt.Errorf("follower target%d signed class%d lacks effect operand", assigned-1, class)
	}
	ship := &s.init.Stores.Ships[actor]
	chosen := -1
	var chosenEffect byte
	for i := 0; i < 4; i++ {
		st := &ship.Stations[i]
		if i == 0 && s.init.Stores.Damage[actor][4] == 2 || distance < 64 || st.CompactID <= 1 || st.Ammunition == 0 {
			continue
		}
		w, er := s.definition(st.CompactID)
		if er != nil {
			return er
		}
		if binary.LittleEndian.Uint16(w.Raw[9:]) < distance>>5 || w.Raw[4+effectClass] == 0 {
			continue
		}
		effect := w.Raw[4+effectClass]
		if chosen >= 0 && effect <= chosenEffect {
			continue
		}
		accepted := false
		if class >= 2 {
			accepted = class == 2 && flags(w)&0x200 != 0 && flags(w)&0x11 == 0 || class == 3 && flags(w)&0x400 != 0
		} else {
			accepted = flags(w)&0x800 != 0
		}
		if !accepted {
			continue
		}
		chosen, chosenEffect = i, effect
		if chosen == 0 {
			break
		}
		if flags(w)&0x1000 != 0 && chosenEffect > 5 {
			break
		}
	}
	if chosen < 0 {
		e.ControlRaw[3] = byte(0xfd)
		s.syncDeath(e)
		return nil
	}
	// AFA5 returns1 once a weapon was selected; B014 returns2 for a
	// still-live private projectile. Early standby/cooldown/no-weapon returns0.
	s.healthyFollowerDecision = true
	if e.Kind == 0x21 {
		e.Pose.Angles[2] = 0
	} else {
		e.Pose.Angles[2] = uint16(yawError >> 2)
	}
	e.Pose.Angles[1] -= e.Pose.Angles[2]
	// The source stores the aim pitch before checking participation/chance.
	aim := e.Pose.Angles
	e.Pose.Angles[0] = s.angle(vertical, int16(distance))
	st := &ship.Stations[chosen]
	w, err := s.definition(st.CompactID)
	if err != nil {
		return err
	}
	defer func() {
		if e.RawMotion != 0 && (binary.LittleEndian.Uint16(w.Raw[9:]) > distance>>3 || binary.LittleEndian.Uint16(w.Raw[9:]) > distance>>4 && uint16(abs(yawError)) > 0x4000) {
			e.Pose.Angles = aim
		}
		s.syncDeath(e)
	}()
	if target.Participation != 1 {
		return nil
	}
	if ship.Proficiency == nil {
		return fmt.Errorf("follower%d SHIP301 proficiency absent at healthy fire", actor)
	}
	rngRange := binary.LittleEndian.Uint16(w.Raw[9:])
	if rngRange == 0 {
		return fmt.Errorf("follower weapon%d zero source range divisor", st.CompactID)
	}
	threshold := uint16(uint16(*ship.Proficiency)*(distance>>5)) / rngRange
	if int16(threshold) >= 96 {
		s.syncDeath(e)
		return nil
	}
	e.Pose.Angles[2] = 0
	cone := uint16(0x1000)
	if flags(w)&1 != 0 {
		cone = 0x800
	}
	if uint16(abs(yawError)) >= cone {
		s.syncDeath(e)
		return nil
	}
	slot := actor + 15
	if s.Slots[slot].Lifetime != 0 || s.Slots[slot].Packed != 0 {
		s.syncDeath(e)
		return nil
	}
	if in.Random == nil {
		return fmt.Errorf("healthy follower fire requires shared LFSR")
	}
	if actor >= len(s.init.CrewPerformance) || !s.init.CrewPerformance[actor].SkillKnown {
		return fmt.Errorf("healthy follower%d lacks initialized performance producer", actor)
	}
	e.DynamicRaw[3] = (*ship.Proficiency >> 2) + 1
	if chosen == 0 || flags(w)&1 != 0 && flags(w)&0x1000 == 0 {
		st.Ammunition -= 20
		if st.Ammunition < 0 {
			st.Ammunition = 0
		}
	} else {
		st.Ammunition--
		if st.Ammunition != 0 && flags(w)&0x1000 != 0 {
			st.Ammunition--
		}
	}
	p := &s.Slots[slot]
	p.Packed = st.CompactID<<2 | st.Mode&3
	p.Flags = flags(w)
	p.Parameter = int16(w.Raw[7])
	hit, err := s.init.ApplyAcceptedCrewFire(actor, byte(in.Random()&127), threshold)
	if err != nil {
		return err
	}
	shot := in
	shot.Target = 0
	shot.TargetAvailable = true
	shot.Salvo = in.Salvo
	if hit {
		shot.Target = uint16(assigned)
	}
	if err = s.spawnFrom(slot, st.Mode, actor, shot); err != nil {
		return err
	}
	// This is the actual accepted source AF21 constructor transition, not the
	// later AF3C radio notification. Historical profiles leave this policy nil.
	if in.FollowerEngagement != nil {
		in.FollowerEngagement(actor, assigned-1)
	}
	s.emit("launch", slot, actor, int(p.Target), 0, "A7E6:AF21 healthy follower")
	s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Target: assigned - 1, Notification: 2, Source: "A7E6:AF3C fired"})
	s.syncDeath(e)
	return nil
}
