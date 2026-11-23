package avionics

import (
	"fmt"
	"gunship2000-assets/game/world"
)

// ThreatPlot is source contact space after range shift/heading rotation. It is
// not a list of scene entities or proof that a raw row is an enemy.
type ThreatPlot struct {
	Row    byte
	X, Y   int16
	Colour byte
	Attack bool
}
type ThreatDisplay struct {
	UsedZoom     uint16
	Zoom         uint16
	Points       []ThreatPlot
	Projectiles  ProjectileThreatDisplay
	WarningFlags uint16
}

// AdvanceThreat is called only for a reached secondary threat MFD logical phase.
// Source3586 mutates zoom while drawing; native presentation consumes this copy.
func (s *State) AdvanceThreat(player Pose, assets *world.Bundle, phase uint16, pixelEligible ...func(ThreatPlot) bool) {
	s.advanceThreat(player, assets, phase, ProjectileThreatInputs{}, ThreatFeedback{Contact: firstContactFeedback(pixelEligible)})
}

// AdvanceThreatFrame includes the source24-slot phase after contacts, before the
// retained zoom update. Inputs are copied; draw calls only consume published plots.
func (s *State) AdvanceThreatFrame(player Pose, assets *world.Bundle, phase uint16, projectiles ProjectileThreatInputs, feedback ThreatFeedback) {
	s.advanceThreat(player, assets, phase, projectiles, feedback)
}

func firstContactFeedback(f []func(ThreatPlot) bool) func(ThreatPlot) bool {
	if len(f) != 0 {
		return f[0]
	}
	return nil
}

func (s *State) advanceThreat(player Pose, assets *world.Bundle, phase uint16, projectiles ProjectileThreatInputs, feedback ThreatFeedback) {
	zoom := s.Threat.Zoom
	s.Threat.UsedZoom = zoom
	s.Threat.Points = nil
	s.Threat.WarningFlags = 0
	var selected, visible, other uint16
	for row, c := range s.Contacts {
		if c.Flags&7 == 0 || c.Kind&0x40 != 0 || c.Kind == 0x1c {
			continue
		}
		dx, dz := Delta(c.H0, player.H0)>>zoom, Delta(c.H1, player.H1)>>zoom
		if c.Kind&0x20 == 0 {
			d := (AbsWord(dx) >> 1) + (AbsWord(dz) >> 1)
			if uint16(row+1) == s.Target {
				selected = d
			}
			if c.Kind&0x10 == 0 && c.Flags&7 == 2 {
				if d > visible {
					visible = d
				}
			} else if d > other {
				other = d
			}
		}
		if dx < -20 || dx > 20 || dz < -20 || dz > 20 {
			continue
		}
		x := world.SourceSinAmplitude(player.Angles[1], dz, assets.Sine) + world.SourceCosAmplitude(player.Angles[1], dx, assets.Sine)
		y := world.SourceSinAmplitude(player.Angles[1], dx, assets.Sine) - world.SourceCosAmplitude(player.Angles[1], dz, assets.Sine)
		colour := byte(6)
		if c.Kind&0x30 != 0 {
			colour = 7
		} else if c.Flags&7 == 2 {
			colour = 12
			if c.Kind <= 1 {
				colour = 9
			}
		} else if c.Kind <= 1 {
			colour = 1
		}
		if s.Target == uint16(row+1) && phase&1 != 0 && y < 0 {
			colour = 15
		}
		attack := c.AttackFlags&0x2000 != 0 && c.Flags&7 == 2
		s.Threat.Points = append(s.Threat.Points, ThreatPlot{byte(row), x, y, colour, attack})
		accepted := false
		if feedback.Contact != nil {
			accepted = feedback.Contact(s.Threat.Points[len(s.Threat.Points)-1])
		}
		if attack && accepted {
			s.Threat.WarningFlags |= 0x2000
		}
	}
	s.Threat.Projectiles = ProjectThreats(player, assets, zoom, phase, projectiles, feedback.Projectile)
	s.Threat.WarningFlags |= s.Threat.Projectiles.WarningFlags
	if selected == 0 {
		selected = visible
		if selected == 0 {
			selected = other
		}
	}
	if selected > 0x8000 {
		s.Threat.Zoom = 0
	} else if selected > 8 && selected < 0x8000 && zoom < 8 {
		s.Threat.Zoom++
	} else if selected < 4 && zoom > 1 {
		s.Threat.Zoom--
	}
}

// AdvanceTargetZoom is reached once per logical target display. The model
// preview shrinks only while source ClipLine FB flags are zero. The world branch
// instead stops at the source model extent limit. Feedback is a pure render probe.
func (s *State) AdvanceTargetZoom(extent int16, threeDimensional bool, clipFlags byte) {
	if threeDimensional {
		if int16(extent<<3) >= int16(s.TargetCache.Zoom) {
			return
		}
	} else if clipFlags != 0 {
		return
	}
	s.TargetCache.Zoom -= uint16(int16(s.TargetCache.Zoom) >> 3)
}

func (s *State) MapPhase(entities [EntityCapacity]Entity, mapStrikeActive bool) {
	if !mapStrikeActive {
		return
	}
	for row, e := range entities {
		if e.Present && e.Kind == 0x11 {
			s.Contacts[row].H0, s.Contacts[row].H1 = e.Pose.H0, e.Pose.H1
			s.Contacts[row].Flags = s.Contacts[row].Flags&0xf9 | 1
			s.Contacts[row].Kind = 0x11
		}
	}
}

// TargetSymbol is resident00db:01bf..0298's weapon capability/range branch.
// x/y are source projected offsets, not modern degree/physical-unit values.
func TargetSymbol(active bool, x, y int16, distance, threshold, flags uint16, kind byte, attackFlags uint16) int {
	if !active {
		return 0
	}
	xx, yy := AbsWord(x), AbsWord(y)
	eligible := false
	if flags&1 != 0 {
		eligible = int16(xx) < 5 && int16(yy) < 5
	} else {
		eligible = !(flags&0x40 != 0 && attackFlags&0x2000 == 0)
		k := kind & 15
		if k >= 3 && k < 10 && flags&0x800 == 0 {
			eligible = false
		}
		if k <= 1 && flags&0x200 == 0 {
			eligible = false
		}
		if (k == 2 || k == 10) && flags&0x400 == 0 {
			eligible = false
		}
		eligible = eligible && int16(xx) < 96 && int16(yy) < 64
	}
	if !eligible {
		return 3
	}
	if threshold > distance {
		return 1
	}
	return 2
}

// SetAttackDefinition is the A275/A3bf mechanism boundary. Only a real admitted
// ENEMY.DAT8-byte definition may supply its word+4; raw entity kind/Side do not.
func (s *State) SetAttackDefinition(row int, raw [8]byte) error {
	if row < 0 || row >= EntityCapacity {
		return fmt.Errorf("attack contact row%d outside80", row)
	}
	s.Contacts[row].AttackFlags = uint16(raw[4]) | uint16(raw[5])<<8
	return nil
}
func (s *State) ClearAttack(row int) error {
	if row < 0 || row >= EntityCapacity {
		return fmt.Errorf("attack contact row%d outside80", row)
	}
	s.Contacts[row].AttackFlags = 0
	return nil
}
