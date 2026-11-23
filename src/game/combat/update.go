package combat

import (
	"fmt"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

// Step is86e6..99ab: ascending24 slots, independent of draw count. Natural
// hostile allocation belongs to the reached actor caller, before this pass.
func (s *State) Step(in Input) error {
	if in.Target != 0 {
		if _, err := s.entity(int(in.Target) - 1); err != nil {
			return err
		}
	}
	if !s.persistentPlayerThreat {
		s.IncomingThreat = 0
	}
	for slot := range s.Slots {
		p := &s.Slots[slot]
		if p.Lifetime == 0 {
			if p.Packed != 0 {
				p.Packed--
			}
			continue
		}
		if p.Flags&3 == 3 && (!s.incomingAdmitted[slot] || in.Incoming == nil) {
			return fmt.Errorf("family3 requires an admitted source slot and synchronous incoming consumer")
		}
		if _, err := s.entity(int(p.Shooter)); err != nil {
			return err
		}
		old := p.Lifetime
		p.Lifetime--
		var err error
		switch p.Flags & 3 {
		case 0:
			err = s.stepGuided(slot, old, in)
		case 1:
			err = s.stepRocket(slot, old, in)
		case 2:
			err = s.stepCannon(slot, old, in)
		case 3:
			err = s.stepIncoming(slot, old, in)
		}
		if err != nil {
			return err
		}
		// Common8836 signed32 inclusive world window. Preserve any inactive
		// tail byte the particular termination arm left, rather than clearing it.
		if int32(p.Pose.H0) < 0x4000 || int32(p.Pose.H0) > 0x7c000 || int32(p.Pose.H1) < 0x4000 || int32(p.Pose.H1) > 0x7c000 {
			p.Lifetime = 0
		}
	}
	s.Updates++
	return nil
}
func (s *State) accelerate(p *Projectile, in Input) int16 {
	shooter := s.init.Entities[p.Shooter]
	// Guided8920/cannon8ced/rocket8f0d all read dynamic DATA2588+24*row.
	// Player E2 is a separate spawn operand; late3eca only publishes pose.
	speed := shooter.RawMotion
	if p.Speed < 600 || p.Speed < int16(speed*2) {
		p.Speed += p.Speed >> 1
	}
	// Source IMUL's DX is discarded by CWD before IDIV10.
	return int16(int16(int32(p.Speed)*int32(in.DF)) / 10)
}
func (s *State) terrain(p *Projectile, previous gamestate.InitializedPose) (int16, error) {
	return s.assets.PairedTerrainHeight(s.init.World, world.QueryPose{H0: p.Pose.H0, H1: p.Pose.H1, Height: int16(p.Pose.Height)}, world.QueryPose{H0: previous.H0, H1: previous.H1, Height: int16(previous.Height)})
}
func (s *State) impact(slot, target int, count int16, in Input, command uint16, source string) error {
	p := &s.Slots[slot]
	e, err := s.entity(target)
	if err != nil {
		return err
	}
	if target == s.init.PlayerRow && e.Participation == 1 {
		// Family2 really reads row0, not an absent-target sentinel. The M
		// stationary-hit scope cannot turn that unresolved player branch into
		// a fake successful explosion or subsystem damage. Explicitly discard
		// this unsupported active-player contact, with no hit RNG/audio/credit
		// or tail. The normal source player row remains participation0: its
		// targetless-cannon contact is an ordinary inert hit/tail, and9a9c
		// returns at its participation guard before any RNG or damage.
		p.Lifetime, p.Packed = 0, 0
		s.emit("unsupported", slot, int(p.Shooter), target, 0, "family2 row0/player contact outside stationary-target scope")
		return nil
	}
	s.sound(in, slot, command, source)
	p.Pose.H0, p.Pose.H1, p.Pose.Height = e.Pose.H0, e.Pose.H1, e.Pose.Height
	p.Lifetime = 0
	s.Events = append(s.Events, Event{Kind: "impact", Slot: slot, Actor: int(p.Shooter), Target: target, Value: count, Source: source, Pose: p.Pose})
	if err = s.hit(target, int(p.Shooter), p.Packed>>2, count, in); err != nil {
		return err
	}
	p.Packed = 4
	return nil
}
func (s *State) stepGuided(slot int, old byte, in Input) error {
	p := &s.Slots[slot]
	if old > 127 {
		if p.Lifetime == 130 {
			s.sound(in, slot, 0x18, "86e6:8724")
		}
		p.Pose = s.mount(s.init.Entities[p.Shooter].Pose, p.Packed&3, slot, in.Damage)
		return nil
	}
	if p.Lifetime == 126 {
		s.sound(in, slot, 8, "86e6:88df")
	}
	previous := p.Pose
	s.move(&p.Pose, s.accelerate(p, in))
	target := p.Target
	if int(p.Shooter) == s.init.PlayerRow && p.Flags&8 == 0 {
		target = 0
		if in.Target != 0 && in.TargetAvailable {
			target = byte(in.Target - 1)
		}
	}
	if target != 0 {
		e, err := s.entity(int(target))
		if err != nil {
			return err
		}
		dx, dz := delta(e.Pose.H0, p.Pose.H0), delta(e.Pose.H1, p.Pose.H1)
		d := metric(dx, dz)
		if d < uint16(p.Speed>>4) && uint16(abs(int16(p.Pose.Height-e.Pose.Height))) < 64 {
			return s.impact(slot, int(target), 1, in, 0x11, "86e6:8aca")
		}
		yaw := s.angle(-dx, dz)
		errorAngle := int16(p.Pose.Angles[1] - yaw)
		if uint16(abs(errorAngle)) < 0x4000 {
			p.Pose.Angles[1] -= uint16(int16((int32(errorAngle) * int32(p.Parameter)) >> 8))
			p.Pose.Angles[0] = s.angle((int16(e.Pose.Height-p.Pose.Height)+32)>>4, int16(d))
		} else if p.Lifetime != 0 && uint16(abs(errorAngle)) > 0x4000 {
			p.Lifetime = 1
		}
	}
	h, err := s.terrain(p, previous)
	if err != nil {
		return err
	}
	if int16(p.Pose.Height) < h {
		if target != 0 {
			p.Pose.Height = uint16(h)
		} else {
			p.Lifetime = 0
			p.Packed = 4
			s.emit("terrain-impact", slot, int(p.Shooter), -1, 0, "86e6:8c3f")
		}
	}
	return nil
}
func (s *State) stepRocket(slot int, old byte, in Input) error {
	p := &s.Slots[slot]
	if old == 131 {
		p.Lifetime = 49
		command := uint16(7)
		if p.Parameter == 0 || p.Flags&0x1000 != 0 {
			command = 9
		}
		s.sound(in, slot, command, "86e6:8e60")
		return nil
	}
	previous := p.Pose
	distance := s.accelerate(p, in)
	if p.Target != 0 {
		e, err := s.entity(int(p.Target))
		if err != nil {
			return err
		}
		dx, dz := Difference(e.Pose.H0, p.Pose.H0), Difference(e.Pose.H1, p.Pose.H1)
		d := metric(dx, dz)
		if e.Kind > 1 {
			p.Pose.Angles[1] = s.angle(-dx, dz)
			p.Pose.Angles[0] = s.angle(int16(e.Pose.Height-p.Pose.Height)+32, int16(d))
		}
		if d < uint16(p.Speed) && uint16(abs(int16(p.Pose.Height-e.Pose.Height))) < 64 {
			return s.impact(slot, int(p.Target), p.Parameter, in, 0x10, "86e6:8fcb")
		}
	}
	for row, e := range s.init.Entities {
		if row >= 80 {
			break
		}
		if s.init.Entities[p.Shooter].Kind == 0x19 && int16(p.Pose.Angles[0]) > 0 || row == int(p.Shooter) || row == int(p.Target) || e.Kind&0x60 != 0 || e.Kind == 0x1c || e.Kind == 0x1d || e.Participation != 1 {
			continue
		}
		d := metric(Difference(p.Pose.H0, e.Pose.H0), Difference(p.Pose.H1, e.Pose.H1))
		hit := d < 1024
		count := int16(2)
		if p.Parameter != 0 {
			// 90fc JGE is signed after word ABS. Unlike the designated-target
			// branches, the -32768 ABS overflow therefore passes this bound.
			hit = d < uint16(p.Speed>>1) && abs(int16(p.Pose.Height-e.Pose.Height)) < 64
			count = p.Parameter
		}
		if hit {
			if err := s.impact(slot, row, count, in, 0x10, "86e6:917c"); err != nil {
				return err
			}
			break
		}
	}
	// Source even moves a just-hit family1 payload before its terrain check.
	s.move(&p.Pose, distance)
	h, err := s.terrain(p, previous)
	if err != nil {
		return err
	}
	if int16(p.Pose.Height) < h {
		p.Pose.H0, p.Pose.H1, p.Pose.Height = previous.H0, previous.H1, previous.Height
		p.Lifetime = 0
		p.Packed = 1
		s.emit("terrain-impact", slot, int(p.Shooter), -1, 0, "86e6:9200")
	}
	return nil
}
func (s *State) stepCannon(slot int, old byte, in Input) error {
	p := &s.Slots[slot]
	if old == 131 {
		p.Lifetime >>= 2
		s.emit("shake", slot, int(p.Shooter), -1, 4, "86e6:8c68")
		s.sound(in, slot, 7, "86e6:8ca9")
		return nil
	}
	previous := p.Pose
	s.move(&p.Pose, s.accelerate(p, in))
	// Unlike family0/1, the actual8d4c consumer reads row0 even for byte0.
	// Preserve that spatial access; hit() bounds player-damage outside M.
	e, err := s.entity(int(p.Target))
	if err != nil {
		return err
	}
	d := metric(Difference(e.Pose.H0, p.Pose.H0), Difference(e.Pose.H1, p.Pose.H1))
	if d < uint16(p.Speed) && uint16(abs(int16(p.Pose.Height-e.Pose.Height))) < 64 {
		return s.impact(slot, int(p.Target), 1, in, 0x10, "86e6:8df8")
	}
	h, err := s.terrain(p, previous)
	if err != nil {
		return err
	}
	if int16(p.Pose.Height) < h {
		p.Pose = previous
		p.Lifetime = 0
		s.emit("terrain-impact", slot, int(p.Shooter), -1, 0, "86e6:8e3c")
	}
	return nil
}
