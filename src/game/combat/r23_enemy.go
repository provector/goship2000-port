package combat

import (
	"encoding/binary"
	"fmt"
)

// SetEnemyBehaviorEnabled is an interpretation selector, not the AI preference.
// Disabled autonomy still leaves movement and already-live incoming slots active.
func (s *State) SetEnemyBehaviorEnabled(enabled bool) { s.enemyBehaviorEnabled = enabled }

// enemyDecision is original0975:A070 at all six callers. Its true return means
// a target was selected; cooldown, unarmed, range/cone and busy-ring visits can
// all return true without releasing a projectile. No model/mission-name gates.
func (s *State) enemyDecision(actor int, in Input) (int, bool, error) {
	if !s.enemyBehaviorEnabled || s.stationaryEnemy == nil || s.enemyAutonomyDisabled {
		return -1, false, nil
	}
	e, err := s.entity(actor)
	if err != nil {
		return -1, false, err
	}
	contacts := &in.Contacts
	if in.ContactState != nil {
		contacts = in.ContactState
	}
	if actor < 0 || actor >= len(contacts) {
		return -1, false, fmt.Errorf("hostile contact row%d unavailable", actor)
	}
	c := &contacts[actor]
	target, observer := -1, int(c.Flags>>3)
	if c.Flags&2 != 0 && observer < 5 && observer < len(s.init.Entities) && s.init.Entities[observer].ControlRaw[1] != 0 {
		target = observer // A08E..A0B9; mast token31 cannot enter this arm.
	} else if e.Kind == 0 || e.Kind == 1 {
		// A0D7..A1E6 scans actual dynamic rows, not display Side/AB9. Each
		// signed16 ABS/half/sum is retained before unsigned strict comparison.
		nearest := uint16(0x800)
		for row := 0; row < len(s.init.Entities) && row < 80; row++ {
			t := &s.init.Entities[row]
			if t.Kind&0x60 != 0 || int16(binary.LittleEndian.Uint16(t.Object.Raw[27:])) >= 0 || t.Participation != 1 || t.Kind&0x10 == 0 {
				continue
			}
			distance := metric(delta(t.Pose.H0, e.Pose.H0), delta(t.Pose.H1, e.Pose.H1))
			if distance < nearest {
				nearest, target = distance, row
			}
		}
	} else if c.Flags&2 != 0 && c.Kind == 0x11 {
		target = observer // A1E8 exact-kind11 alternative belongs to non-air callers.
	}
	if target < 0 {
		c.AttackFlags = 0
		return -1, false, nil
	}
	t, err := s.entity(target)
	if err != nil {
		return -1, false, fmt.Errorf("hostile actor%d target%d: %w", actor, target, err)
	}
	if in.Random == nil {
		return -1, false, fmt.Errorf("hostile decisions require shared RNG")
	}
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
		return target, true, nil
	}
	if choice == 0 {
		return target, true, nil
	}
	dx, dz := delta(t.Pose.H0, e.Pose.H0), delta(t.Pose.H1, e.Pose.H1)
	yaw, distance := s.angle(-dx, dz), metric(dx, dz)
	cone := uint16(0x800)
	if attack&1 != 0 {
		cone = 0x400
	}
	if distance>>5 >= binary.LittleEndian.Uint16(definition[6:]) || e.Kind != 2 && uint16(abs(int16(e.Pose.Angles[1]-yaw))) >= cone {
		s.enemyNoReleaseSteering(actor, target, yaw, distance, in)
		s.syncDeath(e)
		return target, true, nil
	}
	e.Pose.Angles[1], e.DynamicRaw[3] = yaw, definition[2] // A418/A433 precede occupancy.
	s.syncDeath(e)
	s.stationaryEnemy.NextSlot++
	if s.stationaryEnemy.NextSlot == 24 {
		s.stationaryEnemy.NextSlot = 20
	}
	slot := int(s.stationaryEnemy.NextSlot)
	if slot < 20 || slot >= SlotCount {
		return target, true, fmt.Errorf("hostile ring cursor%d outside20..23", slot)
	}
	if s.Slots[slot].Lifetime != 0 {
		s.emit("hostile-busy", slot, actor, target, int16(choice), "0975:A450..A462")
		return target, true, nil
	}
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
		if e.Kind == 1 {
			p.Speed = int16(uint16(p.Speed) << 1)
		} // A5F9..A607 only kind1 guided speed.
	}
	s.incomingAdmitted[slot], s.incomingWarning[slot] = true, true
	s.emit("launch", slot, actor, target, int16(choice), "0975:A465..A653 natural hostile")
	return target, true, nil
}

// A656..A7BE is reached only on range/cone rejection, never simply because a
// target exists. All subtractions wrap as source words before arithmetic shifts.
func (s *State) enemyNoReleaseSteering(actor, target int, yaw, distance uint16, in Input) {
	e, t := &s.init.Entities[actor], &s.init.Entities[target]
	quality := s.init.Switches & 3
	if e.Kind == 0 && distance < 0x2000 {
		if uint16(abs(int16(e.Pose.Angles[1]-t.Pose.Angles[1]))) > 0x4000 && in.Phase&0xc0 == 0 && distance > 0x200 {
			e.Pose.Angles[0] += uint16(int16(uint16(0xf800)-e.Pose.Angles[0]) >> 2)
			if distance > 0x800 {
				turn := uint16(0x4000)
				if distance < 0x1000 {
					turn = 0x6000
				}
				if in.Phase&0x100 != 0 {
					turn += 0x8000
				}
				roll := int16(yaw-e.Pose.Angles[1]+turn) >> 3
				e.Pose.Angles[2] = uint16(roll)
				e.Pose.Angles[1] += uint16(roll)
			} else {
				e.Pose.Angles[2] = uint16(int16(e.Pose.Angles[2]) >> 1)
			}
			e.RawMotion += (160 - e.RawMotion) >> 2
			return
		}
		if distance > 0x100 {
			roll := int16(e.Pose.Angles[1]-yaw) >> ((5 - quality) >> 1)
			e.Pose.Angles[2] = uint16(roll)
			e.Pose.Angles[1] -= uint16(roll)
			if abs(roll) > 0x100 {
				e.RawMotion = (e.RawMotion >> 1) + 32
				return
			}
		}
		e.RawMotion += (160 - e.RawMotion) >> 2
		return
	}
	if e.Kind == 1 {
		if distance > 0x400 {
			roll := int16(e.Pose.Angles[1]-yaw) >> 1
			e.Pose.Angles[2] = uint16(roll)
			e.Pose.Angles[1] -= uint16(roll >> (5 - quality))
		}
		return
	}
	e.Pose.Angles[1] += uint16(int16(yaw-e.Pose.Angles[1]) / [4]int16{50, 20, 10, 5}[quality])
}
