package combat

import (
	"fmt"
)

func (s *State) spawn(slot int, mode byte, in Input) error {
	return s.spawnFrom(slot, mode, s.init.PlayerRow, in)
}

// spawnFrom is the shared source7EA4 body constructor. Actor identity affects
// speed and player-only acquisition filtering; mount/body/trajectory are shared.
func (s *State) spawnFrom(slot int, mode byte, actor int, in Input) error {
	p := &s.Slots[slot]
	shooter, err := s.entity(actor)
	if err != nil {
		return err
	}
	p.Lifetime, p.Shooter = 131, byte(actor)
	p.Target = 0
	if in.Target != 0 {
		if _, err = s.entity(int(in.Target) - 1); err != nil {
			return err
		}
		p.Target = byte(in.Target - 1)
	}
	p.Speed = shooter.RawMotion + 50
	if actor == s.init.PlayerRow {
		p.Speed = abs(in.PlayerSpeed) + 50
	}
	p.Pose = shooter.Pose
	switch p.Flags & 3 {
	case 0:
		p.Pose = s.mount(shooter.Pose, mode, slot, in.Damage)
		p.Template = 1
		// Source IR+0200 weapon flags select air-class contacts. Radar40
		// uses CONTACT attack highbyte20; neither is mission night0200.
		air := p.Flags&0x280 == 0x280
		radar := p.Flags&0x40 != 0
		if actor != s.init.PlayerRow {
			// Source8030 skips AB9/B0B/class reacquisition for nonplayer owners.
		} else if air || radar {
			valid := p.Target != 0
			if valid {
				if air {
					valid = in.Contacts[p.Target].Kind < 2
				} else {
					valid = in.Contacts[p.Target].AttackFlags&0x2000 != 0
				}
			}
			if !valid {
				p.Target = 0
				for row, e := range s.init.Entities {
					if row >= 80 {
						break
					}
					c := in.Contacts[row]
					if e.Participation != 1 || c.Kind&0x70 != 0 || c.Flags&7 != 2 {
						continue
					}
					if air && c.Kind < 2 || radar && c.AttackFlags&0x2000 != 0 {
						p.Target = byte(row)
						break
					}
				}
			}
		} else if !in.TargetAvailable {
			p.Target = 0
		}
		if p.Target == 0 {
			p.Parameter = 0
		}
	case 1:
		if p.Flags&0x1000 != 0 {
			p.Parameter = in.Salvo
		} else {
			p.Parameter = 1
		}
		if mode&3 == 0 {
			p.Template = 0
		} else {
			p.Pose = s.mount(shooter.Pose, mode, slot, in.Damage)
			p.Template = 0
			if p.Flags&0x1000 != 0 {
				switch in.Salvo {
				case 1:
					p.Template = 1
				case 2:
					p.Template = 2
				case 4:
					p.Template = 3
				default:
					return fmt.Errorf("invalid rocket salvo%d", in.Salvo)
				}
			}
		}
		if p.Target != 0 && in.TargetAvailable {
			target, err := s.entity(int(p.Target))
			if err != nil {
				return err
			}
			dx, dz := Difference(target.Pose.H0, shooter.Pose.H0), Difference(target.Pose.H1, shooter.Pose.H1)
			p.Pose.Angles[1] = s.angle(-dx, dz)
			p.Pose.Angles[0] = s.angle(int16(target.Pose.Height-shooter.Pose.Height)+32, int16(metric(dx, dz)))
		} else {
			p.Target = 0
		}
	case 2:
		p.Template = 0
		if p.Target != 0 {
			target, err := s.entity(int(p.Target))
			if err != nil {
				return err
			}
			dx, dz := delta(target.Pose.H0, p.Pose.H0), delta(target.Pose.H1, p.Pose.H1)
			yaw := s.angle(-dx, dz)
			if uint16(abs(int16(p.Pose.Angles[1]-yaw))) < 0x4000 {
				p.Pose.Angles[1] = yaw
			} else if int16(yaw) < 0 {
				p.Pose.Angles[1] -= 0x4000
			} else {
				p.Pose.Angles[1] += 0x4000
			}
			pitch := s.angle((int16(target.Pose.Height-p.Pose.Height)+32)>>4, int16(metric(dx, dz)))
			if uint16(abs(int16(p.Pose.Angles[0]-pitch))) < 0x1000 {
				if !in.TargetAvailable {
					pitch = (pitch - 128) * 2
				}
				p.Pose.Angles[0] = pitch
			} else if int16(pitch) < 0 {
				p.Pose.Angles[0] -= 0x1000
			} else {
				p.Pose.Angles[0] += 0x1000
			}
		}
		s.move(&p.Pose, int16((in.Phase&3)<<6))
	default:
		return fmt.Errorf("unsupported manual projectile family%d", p.Flags&3)
	}
	p.AuxiliaryAngles = p.Pose.Angles
	return nil
}
