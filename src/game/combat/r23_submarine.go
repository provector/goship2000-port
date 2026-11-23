package combat

import (
	"encoding/binary"
	"fmt"
)

// advanceSubmarine is source C106..C8A0, kind0A. RawMotion is the source speed
// while nonnegative and the submergence counter while negative, not world depth.
func (s *State) advanceSubmarine(row int, in Input) error {
	e := &s.init.Entities[row]
	if !s.enemyBehaviorEnabled || s.route == nil || e.Kind != 0x0a {
		return fmt.Errorf("submarine row%d lacks the R23 controller", row)
	}
	if e.Participation >= 2 {
		_, err := s.advanceDeathRow(row, in)
		if e.Kind != 0x0a {
			s.init.ClearSubmarineModel(row)
		}
		return err
	}
	model, err := s.init.EnsureSubmarineModel(row, s.assets)
	if err != nil {
		return err
	}
	if e.Participation == 0 {
		s.route.Queued[row] = -1
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
		e.Participation = 1
		if e.DynamicRaw[2] != 0 {
			leader, x, z, err := s.groundDesired(e)
			if err != nil {
				return err
			}
			e.RawMotion, leader.RawMotion = 50, 50
			e.Pose.Angles, e.Pose.H0, e.Pose.H1, e.Pose.Height = leader.Pose.Angles, x, z, leader.Pose.Height
			s.syncDeath(leader)
		} else {
			e.RawMotion, s.route.Cursor[row] = 0, 0
		}
		s.syncDeath(e)
		return nil
	}
	if e.RawMotion < 0 {
		// C2B2 moves even when E0 is false; the geometry/countdown is slower.
		s.move(&e.Pose, int16(uint16(in.DF)*2))
		if in.E0 {
			e.RawMotion++
			if e.RawMotion == 0 {
				e.Kind = 0x41
				if in.ContactState != nil {
					in.ContactState[row].Kind = 0x41
				}
			} else if err := model.SubmergeVisit(e.RawMotion); err != nil {
				return err
			}
		}
		s.syncDeath(e)
		return nil
	}
	contacts := &in.Contacts
	if in.ContactState != nil {
		contacts = in.ContactState
	}
	if contacts[row].Flags&2 != 0 {
		// Mechanical contact response stays active with autonomous decisions off,
		// as victim defenses do. This visit neither moves nor consumes RNG.
		e.RawMotion = -160
		s.syncDeath(e)
		return nil
	}
	if queued := s.route.Queued[row]; queued != -1 {
		leaderRow := e.LeaderRow
		if leaderRow < 0 || leaderRow >= row || s.init.Entities[leaderRow].Kind != 0x40 {
			return fmt.Errorf("queued submarine row%d lacks earlier PATH header", row)
		}
		leader := &s.init.Entities[leaderRow]
		leader.DynamicRaw[2] &= 0x7f
		s.syncDeath(leader)
		p, _, err := s.routePoint(int(leader.DynamicRaw[2]), uint16(queued))
		if err != nil {
			return err
		}
		dx, dz := int16(uint32(p.X)-e.Pose.H0), int16(uint32(p.Y)-e.Pose.H1)
		distance := (uint16(abs(dx)) >> 1) + (uint16(abs(dz)) >> 1)
		if uint16(e.RawMotion) > distance {
			e.Pose.H0, e.Pose.H1 = uint32(p.X), uint32(p.Y)
			s.route.Queued[row] = -1
		} else {
			e.Pose.Angles[1] = s.angle(-dx, dz)
			s.move(&e.Pose, int16(uint16(e.RawMotion)*uint16(in.DF))/10)
		}
		s.syncDeath(e)
		return nil
	}
	if e.DynamicRaw[2] == 0 {
		return nil // C5CE/C5D6: independent has no A070 or ordinary motion.
	}
	return s.advanceSubmarineFormation(row, in)
}

// advanceSubmarineFormation is C5C6..C7FB. The normal controller reaches it only
// after the earlier contact-triggered dive predicate was false. Keeping that
// entry boundary explicit also permits a supplied mid-routine selected/no-shot
// fixture without mislabelling it a naturally reached submarine engagement.
func (s *State) advanceSubmarineFormation(row int, in Input) error {
	e := &s.init.Entities[row]
	leader, x, z, err := s.groundDesired(e)
	if err != nil {
		return err
	}
	leader.DynamicRaw[2] &= 0x7f
	s.syncDeath(leader)
	_, selected, err := s.enemyDecision(row, in)
	if err != nil {
		return err
	}
	if selected && e.DynamicRaw[2]&1 != 0 {
		e.DynamicRaw[2] = 0
		s.syncDeath(e)
		return nil // C607: no ordinary movement or two jitter draws this visit.
	}
	if in.Random == nil {
		return fmt.Errorf("submarine formation needs shared two LFSR draws")
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
	s.syncDeath(e)
	return nil
}
