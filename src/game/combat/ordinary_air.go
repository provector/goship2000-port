package combat

import (
	"encoding/binary"
	"fmt"
)

// advanceOrdinaryHelicopter is source0975:B048..B6DD, kind0. R23 admits its
// two A070 callers; historical launches keep their earlier false-decision arm.
func (s *State) advanceOrdinaryHelicopter(row int, in Input) error {
	e := &s.init.Entities[row]
	if s.route == nil || e.Kind != 0 {
		return fmt.Errorf("ordinary helicopter row%d has no P5 route owner", row)
	}
	if e.Participation >= 2 {
		_, err := s.advanceDeathRow(row, in)
		return err
	}
	if e.Participation == 0 {
		s.route.Queued[row] = -1
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
		e.Participation, e.Pose.Height = 1, 0x400
		if e.DynamicRaw[2] != 0 {
			leader, x, z, err := s.groundDesired(e)
			if err != nil {
				return err
			}
			e.RawMotion, leader.RawMotion = 40, 40
			e.Pose.Angles, e.Pose.H0, e.Pose.H1, e.Pose.Height = leader.Pose.Angles, x, z, leader.Pose.Height
			s.syncDeath(leader)
		} else {
			e.RawMotion, s.route.Cursor[row] = 100, 0
		}
		s.syncDeath(e)
		return nil
	}
	old := e.Pose
	queued := s.route.Queued[row]
	if queued != -1 {
		leaderRow := e.LeaderRow
		if leaderRow < 0 || leaderRow >= row || s.init.Entities[leaderRow].Kind != 0x40 {
			return fmt.Errorf("queued helicopter row%d lacks earlier PATH header", row)
		}
		leader := &s.init.Entities[leaderRow]
		leader.DynamicRaw[2] &= 0x7f
		s.syncDeath(leader)
		p, _, err := s.routePoint(int(leader.DynamicRaw[2]), uint16(queued))
		if err != nil {
			return err
		}
		// B21D/B237 deliberately subtract only horizontal low words.
		dx, dz := int16(uint32(p.X)-e.Pose.H0), int16(uint32(p.Y)-e.Pose.H1)
		distance := (uint16(abs(dx)) >> 1) + (uint16(abs(dz)) >> 1)
		if uint16(e.RawMotion) > distance {
			e.Pose.H0, e.Pose.H1 = uint32(p.X), uint32(p.Y)
			s.route.Queued[row] = -1
		} else {
			e.Pose.Angles[1] = s.angle(-dx, dz)
			s.move(&e.Pose, int16(uint16(e.RawMotion)*uint16(in.DF))/10)
			q, err := s.assets.PairedTerrainHeight(s.init.World, deathQuery(e.Pose), deathQuery(old))
			if err != nil {
				return err
			}
			e.Pose.Height = uint16(q + 256)
		}
		s.syncDeath(e)
		return nil
	}
	if e.DynamicRaw[2] == 0 {
		if s.enemyBehaviorEnabled {
			_, selected, err := s.enemyDecision(row, in)
			if err != nil {
				return err
			}
			if selected {
				// B5DE..B68F: A070 has already selected and steered. A
				// target without a shot still reaches integration. The height
				// owner is literal CONTACT observer, not A070's fallback target.
				s.move(&e.Pose, int16(uint16(e.RawMotion)*uint16(in.DF))/10)
				h, err := s.assets.PairedTerrainHeight(s.init.World, deathQuery(e.Pose), deathQuery(old))
				if err != nil {
					return err
				}
				contacts := &in.Contacts
				if in.ContactState != nil {
					contacts = in.ContactState
				}
				observer := int(contacts[row].Flags >> 3)
				if observer >= len(s.init.Entities) {
					return fmt.Errorf("ordinary helicopter row%d observer%d has no original height owner", row, observer)
				}
				e.Pose.Height = uint16(h + (int16(s.init.Entities[observer].Pose.Height) >> 1) + 128)
				s.syncDeath(e)
				return nil
			}
		}
		// B697, including a disabled/new no-target decision: damping only.
		e.Pose.Angles[2] = uint16(int16(e.Pose.Angles[2]) >> 1)
		s.syncDeath(e)
		return nil
	}
	if s.enemyBehaviorEnabled {
		// B3A6 uses the preceding PATH header before A070. Do not demand
		// the formation-offset span until B3DD: an odd selected member can
		// detach without ever reading that table on this visit.
		leaderRow := e.LeaderRow
		if leaderRow < 0 || leaderRow >= row || leaderRow >= len(s.init.Entities) || s.init.Entities[leaderRow].Kind != 0x40 {
			return fmt.Errorf("ordinary helicopter row%d lacks preceding PATH header%d", row, leaderRow)
		}
		leader := &s.init.Entities[leaderRow]
		leader.DynamicRaw[2] &= 0x7f
		s.syncDeath(leader)
		_, selected, err := s.enemyDecision(row, in)
		if err != nil {
			return err
		}
		if selected && e.DynamicRaw[2]&1 != 0 {
			// B3D4: even an unarmed/cooldown/busy selected result detaches
			// an odd member. This visit skips formation and both jitter draws.
			e.DynamicRaw[2] = 0
			s.syncDeath(e)
			return nil
		}
	}
	leader, x, z, err := s.groundDesired(e)
	if err != nil {
		return err
	}
	leader.DynamicRaw[2] &= 0x7f
	s.syncDeath(leader)
	if in.Random == nil {
		return fmt.Errorf("ordinary helicopter requires shared two formation draws")
	}
	dx := int16((int32(x-e.Pose.H0) + int32(in.Random()&7) - 4) >> 2)
	dz := int16((int32(z-e.Pose.H1) + int32(in.Random()&7) - 4) >> 2)
	if int16(abs(dx)) > 16 || int16(abs(dz)) > 16 {
		e.Pose.Angles[1] = s.angle(-dx, dz)
	} else {
		e.Pose.Angles[1] = uint16((int16(e.Pose.Angles[1]) >> 1) + (int16(leader.Pose.Angles[1]) >> 1))
	}
	e.Pose.H0 += uint32(int32(dx))
	e.Pose.H1 += uint32(int32(dz))
	e.RawMotion = leader.RawMotion
	// The formation arm keeps the inherited height; it does not re-query
	// terrain or add a body/mast sensor offset.
	s.syncDeath(e)
	return nil
}

// B6E0..BABB, ordinary kind1 fixed-wing pursuit/return-home controller. Source
// 593C is the actual player row, while CONTACT flags>>3 is its attributed
// observer. Neither is AB9 (the player's selected weapon target).
func (s *State) advanceOrdinaryAircraft(row int, in Input) error {
	e := &s.init.Entities[row]
	if s.route == nil || e.Kind != 1 {
		return fmt.Errorf("ordinary aircraft row%d has no P5 route owner", row)
	}
	if e.Participation >= 2 {
		_, err := s.advanceDeathRow(row, in)
		return err
	}
	if e.Participation == 0 {
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
		e.Participation, e.Pose.Height, e.RawMotion = 1, 0x400, 400
		s.route.Cursor[row] = 0
		s.syncDeath(e)
		return nil
	}
	old := e.Pose
	latched := binary.LittleEndian.Uint16(e.DynamicRaw[4:]) != 0
	selected := false
	var dx, dz, vertical int16
	if latched {
		dx = Difference(binary.LittleEndian.Uint32(e.Object.Raw[2:]), e.Pose.H0)
		dz = Difference(binary.LittleEndian.Uint32(e.Object.Raw[6:]), e.Pose.H1)
		vertical = int16(0x400 - e.Pose.Height)
	} else {
		target := s.init.PlayerRow
		contact := in.Contacts[row]
		if in.ContactState != nil {
			contact = in.ContactState[row]
		}
		if contact.Flags&2 != 0 {
			target = int(contact.Flags >> 3)
		}
		if s.enemyBehaviorEnabled {
			// B77A preserves this local observer/player before B82E. A070
			// owns its own target, steering, cooldown and release outcomes;
			// its result only gates B9B4, never the later turn/latch test.
			_, result, err := s.enemyDecision(row, in)
			if err != nil {
				return err
			}
			selected = result
		}
		if target < 0 || target >= len(s.init.Entities) {
			return fmt.Errorf("ordinary aircraft row%d observer%d outside actual rows", row, target)
		}
		t := s.init.Entities[target]
		dx, dz = Difference(t.Pose.H0, e.Pose.H0), Difference(t.Pose.H1, e.Pose.H1)
		vertical = int16(t.Pose.Height-e.Pose.Height) + 128
	}
	yawError := int16(e.Pose.Angles[1] - s.angle(-dx, dz))
	// B8DB..B90C ABS results remain signed16 for comparison and shifts.
	x, z := int16(abs(dx)), int16(abs(dz))
	distance := int16(0)
	if z <= x {
		distance = (z >> 1) + x
	} else {
		distance = (x >> 1) + z
	}
	turn := false
	if latched {
		turn = uint16(distance) < 0x400
	} else {
		turn = abs(yawError) > 0x4000
	}
	if turn {
		e.Pose.Angles[0] = uint16(int16(e.Pose.Angles[0]) >> 1)
		e.Pose.Angles[2] = uint16(int16(e.Pose.Angles[2]) >> 1)
		if latched {
			binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
		} else if uint16(distance) > 0x100 {
			binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 1)
		}
	} else if !selected {
		// B9AB: only A070=false uses this generic steering. Pitch uses the
		// unsigned horizontal half-distance and SAR5 vertical word.
		pitch := int16(s.angle(vertical>>5, int16(uint16(distance)>>1))) >> 1
		e.Pose.Angles[0] = uint16(pitch + (int16(e.Pose.Angles[0]) >> 1))
		e.Pose.Angles[2] = uint16(yawError >> 2)
		e.Pose.Angles[1] -= uint16((yawError >> 2) >> 2)
	}
	s.move(&e.Pose, int16(uint16(e.RawMotion)*uint16(in.DF))/10)
	h, err := s.assets.PairedTerrainHeight(s.init.World, deathQuery(e.Pose), deathQuery(old))
	if err != nil {
		return err
	}
	if int16(e.Pose.Height) < h+256 {
		e.Pose.Height = uint16((int16(e.Pose.Height) + h + 256) >> 1)
	}
	s.syncDeath(e)
	return nil
}
