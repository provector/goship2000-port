package combat

import (
	"encoding/binary"
	"fmt"
)

// EnemyFilmState contains the source ACB allocation cursor. Definitions and the
// disable policy are frozen launch inputs. An absent field preserves old films.
type EnemyFilmState struct {
	NextSlot uint16
}

// ConfigureStationaryEnemyAI admits only the C056 stationary ground caller.
// It must run before the launch checkpoint, including when autonomy is disabled.
func (s *State) ConfigureStationaryEnemyAI(definitions [16][8]byte, disabled bool) error {
	for i, row := range definitions {
		if row[0] != byte(i) {
			return fmt.Errorf("hostile definition row%d has source ID%d", i, row[0])
		}
	}
	if s.stationaryEnemy != nil {
		return fmt.Errorf("stationary hostile interpretation was already configured")
	}
	s.stationaryEnemy = &EnemyFilmState{NextSlot: 20} // cold DATA272D:0ACB
	s.enemyDefinitions, s.enemyAutonomyDisabled = definitions, disabled
	return nil
}

func (s *State) filmEnemy() *EnemyFilmState {
	if s.stationaryEnemy == nil {
		return nil
	}
	r := *s.stationaryEnemy
	return &r
}

// stationaryEnemyAttack is the source A070 decision for its C056 caller.
// A nonzero result means a target exists, not that a payload was released.
func (s *State) stationaryEnemyAttack(actor int, in Input) (bool, error) {
	if s.enemyBehaviorEnabled {
		_, selected, err := s.enemyDecision(actor, in)
		return selected, err
	}
	if s.stationaryEnemy == nil || s.enemyAutonomyDisabled {
		return false, nil
	}
	e, err := s.entity(actor)
	if err != nil {
		return false, err
	}
	if e.Kind < 2 || e.Kind > 7 || e.Participation != 1 || e.DynamicRaw[2] != 0 {
		return false, fmt.Errorf("stationary hostile row%d is outside C056", actor)
	}
	contacts := &in.Contacts
	if in.ContactState != nil {
		contacts = in.ContactState
	}
	c := &contacts[actor]
	target := -1
	observer := int(c.Flags >> 3)
	// A08E..A0B9: only observer0..4 with live CONTROL+1 receives the
	// retaliation shortcut. The mast token31 is excluded from this arm.
	if c.Flags&2 != 0 && observer < 5 && observer < len(s.init.Entities) && s.init.Entities[observer].ControlRaw[1] != 0 {
		target = observer
	} else if c.Flags&2 != 0 && c.Kind == 0x11 {
		// A1E8..A215 is an independent exact-kind11 alternative, not a
		// general permission to treat every contact observer as the player.
		target = observer
	}
	if target < 0 {
		c.AttackFlags = 0
		return false, nil
	}
	t, err := s.entity(target)
	if err != nil {
		return false, fmt.Errorf("stationary hostile row%d target%d: %w", actor, target, err)
	}
	if in.Random == nil {
		return false, fmt.Errorf("stationary hostile decisions require shared RNG")
	}
	// A221 is unconditional after selection, including cooldown/wait visits.
	choice := e.Object.Raw[11] >> 4
	if in.Random()&1 != 0 {
		choice = e.Object.Raw[11] & 15
	}
	definition := s.enemyDefinitions[choice]
	attack := binary.LittleEndian.Uint16(definition[4:])
	c.AttackFlags = attack
	if in.HostileTarget != nil {
		in.HostileTarget(actor, target)
	}
	if e.DynamicRaw[3] != 0 {
		if in.E0 {
			e.DynamicRaw[3]--
		}
		return true, nil
	}
	if choice == 0 {
		return true, nil
	}
	dx, dz := delta(t.Pose.H0, e.Pose.H0), delta(t.Pose.H1, e.Pose.H1)
	yaw := s.angle(-dx, dz)
	distance := metric(dx, dz)
	cone := uint16(0x800)
	if attack&1 != 0 {
		cone = 0x400
	}
	if distance>>5 >= binary.LittleEndian.Uint16(definition[6:]) || e.Kind != 2 && uint16(abs(int16(e.Pose.Angles[1]-yaw))) >= cone {
		// A792..A7B9: selected but no shot, including definition/range/cone
		// rejection, turns the stationary actor by the source quality divisor.
		divisor := [4]int16{50, 20, 10, 5}[s.init.Switches&3]
		e.Pose.Angles[1] += uint16(int16(yaw-e.Pose.Angles[1]) / divisor)
		s.syncDeath(e)
		return true, nil
	}
	// A418/A433 happen before ACB advance and slot occupancy rejection.
	e.Pose.Angles[1] = yaw
	e.DynamicRaw[3] = definition[2]
	s.syncDeath(e)
	s.stationaryEnemy.NextSlot++
	if s.stationaryEnemy.NextSlot == 24 {
		s.stationaryEnemy.NextSlot = 20
	}
	slot := int(s.stationaryEnemy.NextSlot)
	if slot < 20 || slot >= SlotCount {
		return false, fmt.Errorf("hostile ring cursor%d is outside20..23", slot)
	}
	if s.Slots[slot].Lifetime != 0 {
		s.emit("hostile-busy", slot, actor, target, int16(choice), "0975:A450..A462")
		return true, nil
	}
	// A465..A50F writes the tagged family3 constructor. Preserve the old
	// unused roll word: the original writes only pitch and yaw here.
	p := &s.Slots[slot]
	roll := p.Pose.Angles[2]
	p.Pose = e.Pose
	p.Pose.Height += 32
	p.Pose.Angles = [3]uint16{s.angle((int16(t.Pose.Height-e.Pose.Height)+32)>>4, int16(distance)), yaw, roll}
	p.Shooter, p.Target, p.Lifetime = byte(actor), byte(target), 0x83
	p.Flags = attack | 3
	if attack&1 != 0 {
		p.Parameter, p.Packed, p.Speed, p.Template = 0, definition[1], int16(uint16(153)*uint16(in.DF)), 0
		if target == s.init.PlayerRow {
			s.sound(in, slot, 0x29, "0975:A516..A52E")
		}
	} else {
		p.Parameter, p.Packed, p.Speed, p.Template = int16(definition[1]), 0, int16(uint16(51)*uint16(in.DF)), 1
		if byte(in.Random()) > definition[3] {
			p.Flags |= 0x8000
		}
	}
	s.incomingAdmitted[slot], s.incomingWarning[slot] = true, true
	s.emit("launch", slot, actor, target, int16(choice), "0975:A465..A653 natural stationary hostile")
	return true, nil
}
