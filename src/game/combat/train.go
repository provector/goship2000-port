package combat

import (
	"encoding/binary"
	"fmt"
)

// SetTrainEnabled selects the P6 source kind08 interpretation at launch, never
// from a model/formation label. Existing P5 films keep their absent visits.
func (s *State) SetTrainEnabled(v bool) { s.trainEnabled = v }

// D0B7..D62E is a separate source arm from ordinary ground BABE. In particular,
// following trains set desired coordinates directly, not the ground SAR2 delta.
func (s *State) advanceTrainRoute(row int, in Input) error {
	e := &s.init.Entities[row]
	if s.route == nil {
		return fmt.Errorf("Train row%d lacks source PATH carrier", row)
	}
	if e.Participation >= 2 {
		_, err := s.advanceDeathRow(row, in)
		return err
	}
	if e.Participation == 0 {
		s.route.Queued[row] = -1
		if e.DynamicRaw[2] != 0 {
			leader, x, z, err := s.groundDesired(e)
			if err != nil {
				return err
			}
			binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
			e.Participation = 1
			e.RawMotion = 25
			leader.RawMotion = 25
			e.Pose = leader.Pose
			e.Pose.H0, e.Pose.H1 = x, z
			s.syncDeath(leader)
		} else {
			h, err := s.assets.TerrainHeight(s.init.World, deathQuery(e.Pose))
			if err != nil {
				return err
			}
			e.Pose.Height = uint16(h)
			binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
			e.Participation = 1
			e.RawMotion = 30
			s.route.Cursor[row] = 0
		}
		s.syncDeath(e)
		return nil
	}
	if e.Participation != 1 {
		return nil
	}
	queued := s.route.Queued[row]
	if queued != -1 {
		leaderRow := e.LeaderRow
		if leaderRow < 0 || leaderRow >= row || s.init.Entities[leaderRow].Kind != 0x40 {
			return fmt.Errorf("queued Train row%d lacks preceding source PATH header", row)
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
			e.Pose.Angles[1] = leader.Pose.Angles[1]
			s.route.Queued[row] = -1
		} else {
			e.Pose.Angles[1] = s.angle(-dx, dz)
			s.move(&e.Pose, int16(uint16(e.RawMotion)*uint16(in.DF))/10)
		}
		s.syncDeath(e)
		return nil
	}
	if e.DynamicRaw[2] == 0 {
		return nil
	}
	leader, x, z, err := s.groundDesired(e)
	if err != nil {
		return err
	}
	leader.DynamicRaw[2] &= 0x7f
	s.syncDeath(leader)
	if in.Random == nil {
		return fmt.Errorf("Train row%d requires shared ordered jitter draws", row)
	}
	e.RawMotion = leader.RawMotion
	e.Pose.Angles[1] = leader.Pose.Angles[1]
	e.Pose.H0 = x + uint32(int32(in.Random()&3)-2)
	e.Pose.H1 = z + uint32(int32(in.Random()&3)-2)
	s.syncDeath(e)
	return nil
}
