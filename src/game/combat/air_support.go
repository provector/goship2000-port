package combat

import (
	"encoding/binary"
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// SetMissionActorsEnabled selects the source P5 controller interpretation at
// launch. P4 and earlier recordings retain their original dormant actors.
func (s *State) SetMissionActorsEnabled(v bool) {
	s.missionActorsEnabled = v
	if v && s.route == nil {
		s.route = newRouteState()
	}
}
func (s *State) MissionActorsEnabled() bool { return s.missionActorsEnabled }

// UpdateAirSupport is the literal bounded kind11 C8A3..D028 controller. Raw
// OBJECT home, AA2/target and per-actor dynamic+3 return state are distinct.
func (s *State) UpdateAirSupport(actor int, in Input) error {
	e, err := s.entity(actor)
	if err != nil {
		return err
	}
	sup := s.init.Support
	retained := sup != nil && sup.Configuration.HasRetainedPrefixPolicy()
	admitted := sup != nil && sup.Configuration.Profile == gamestate.SupportInitializationProfile && sup.Configuration.Mode == 2 && len(sup.Configuration.Rows) == 1 && sup.Configuration.Rows[0] == actor
	if retained {
		admitted = sup.Configuration.ControllerKind(actor) == 0x11
	}
	if !s.missionActorsEnabled || !admitted || actor < 1 || actor >= 80 || e.Kind != 0x11 || e.FlightSlot >= 0 {
		return fmt.Errorf("air Support row%d lacks P5 validated source owner", actor)
	}
	for id := byte(2); id <= 3; id++ {
		w, er := s.definition(id)
		if er != nil {
			return er
		}
		if !retained && w.RawWSHDID != int(sup.Configuration.PrefixRaw[id-2]) {
			return fmt.Errorf("air Support compact%d prefix mismatch", id)
		}
	}
	switch e.Participation {
	case 0:
		if s.route != nil {
			s.route.Cursor[actor] = 0
		}
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
		e.DynamicRaw[3] = 0
		e.Pose.Height = 0x600
		e.RawMotion = 0x190
		if sup.Request != 0 {
			e.Participation = 1
			if e.Exclusion == "source airborne reserve awaits AA2 activation; autonomous activation suppressed" {
				e.Exclusion = ""
			}
		}
		s.syncDeath(e)
		e.Visible = e.Visible && e.Participation != 0
		return nil
	case 2:
		_, err = s.advanceDeathRow(actor, in)
		return err
	case 1:
	default:
		return nil
	}
	old := e.Pose
	goal := sup.Target
	if e.DynamicRaw[3] != 0 {
		goal.H0 = binary.LittleEndian.Uint32(e.Object.Raw[2:])
		goal.H1 = binary.LittleEndian.Uint32(e.Object.Raw[6:])
	}
	dx, dz := delta(goal.H0, e.Pose.H0), delta(goal.H1, e.Pose.H1)
	distance := metric(dx, dz)
	yawError := int16(e.Pose.Angles[1] - s.angle(-dx, dz))
	vertical := int16(0)
	if distance > 16 {
		selected := -1
		best := uint16(0x800)
		chosenError := int16(0)
		if distance < 0x800 {
			for row := 0; row < len(s.init.Entities) && row < 80; row++ {
				t := &s.init.Entities[row]
				if t.Kind&0x70 != 0 || t.Participation != 1 {
					continue
				}
				x, z := delta(t.Pose.H0, e.Pose.H0), delta(t.Pose.H1, e.Pose.H1)
				d := metric(x, z)
				a := int16(e.Pose.Angles[1] - s.angle(-x, z))
				if int16(abs(a)) >= 0x1000 {
					continue
				}
				distance = d
				if d < best {
					selected, best, chosenError = row, d, a
					vertical = int16(t.Pose.Height-e.Pose.Height) + 128
				}
			}
		}
		if selected >= 0 {
			distance = best
			yawError = chosenError
			if in.ObserveSupport == nil {
				return fmt.Errorf("air Support source0616 contact owner missing")
			}
			visible, er := in.ObserveSupport(actor, selected)
			if er != nil {
				return er
			}
			if visible && in.E0 {
				sup.Request--
				if sup.Request == 0x40 {
					e.DynamicRaw[3] = 1
				}
				slot := int((byte(s.NextSlot) - byte((sup.Request&0x30)>>4) + 8) & 15)
				if s.Slots[slot].Lifetime == 0 {
					weapon := byte(3)
					if best < 0x200 && int16(abs(yawError)) < 0x400 {
						weapon = 2
					}
					w, er := s.definition(weapon)
					if er != nil {
						return er
					}
					p := &s.Slots[slot]
					p.Packed, p.Flags, p.Parameter = weapon<<2|3, flags(w), int16(w.Raw[7])
					shot := in
					shot.Target = uint16(selected + 1)
					shot.TargetAvailable = true
					shot.Salvo = in.Salvo
					if er = s.spawnFrom(slot, 3, actor, shot); er != nil {
						return er
					}
					s.emit("launch", slot, actor, selected, 0, "157f:CE90 air Support")
				}
			}
		} else if distance < 0x200 {
			if in.E0 {
				sup.Request--
				if sup.Request == 0x40 {
					e.DynamicRaw[3] = 1
				}
			}
			if uint16(abs(yawError)) > 0x4000 {
				e.Pose.Angles[0] = uint16(int16(e.Pose.Angles[0]) >> 1)
				e.Pose.Angles[2] = uint16(int16(e.Pose.Angles[2]) >> 1)
				if e.DynamicRaw[3] != 0 {
					e.Participation = 0
					sup.Request = 0
				} else {
					e.DynamicRaw[3] = 1
				}
			}
			yawError = 0
		}
		if yawError != 0 {
			pitch := s.angle(vertical>>5, int16(distance>>1))
			e.Pose.Angles[0] = uint16((int16(pitch) >> 1) + (int16(e.Pose.Angles[0]) >> 1))
			e.Pose.Angles[2] = uint16(yawError >> 2)
			e.Pose.Angles[1] -= uint16((yawError >> 2) >> 2)
		}
	}
	// CF7C IMUL stores lowword, CWD re-extends it before IDIV10.
	step := int16(uint16(e.RawMotion)*uint16(in.DF)) / 10
	s.move(&e.Pose, step)
	q, er := s.assets.PairedTerrainHeight(s.init.World, deathQuery(e.Pose), deathQuery(old))
	if er != nil {
		return er
	}
	if int16(e.Pose.Height) < int16(q+0x100) {
		e.Pose.Height = uint16(q + 0x100)
	}
	s.syncDeath(e)
	e.Visible = e.Visible && e.Participation != 0
	return nil
}
