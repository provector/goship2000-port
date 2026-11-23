package combat

import (
	"encoding/binary"
	"fmt"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
)

// AdvanceDeaths is the non-tactical part of source157f's entity dispatcher.
// It runs once per logical update before projectiles. Returned rows are the
// new1c wreck observations (DD92), for the authoritative contact owner to apply.
func (s *State) AdvanceDeaths(in Input) ([]int, error) {
	var observed []int
	for row := range s.init.Entities {
		changed, err := s.advanceDeathRow(row, in)
		if err != nil {
			return observed, err
		}
		if changed {
			observed = append(observed, row)
		}
	}
	return observed, nil
}

// advanceDeathRow is one source dispatcher visit, shared with follower failure.
// A newly changed kind/P never receives another visit during the same update.
func (s *State) advanceDeathRow(row int, in Input) (bool, error) {
	e := &s.init.Entities[row]
	if row == s.init.PlayerRow || e.Kind == 0xff {
		return false, nil
	}
	if e.Kind == 0x1c || e.Kind == 0x1d {
		if e.Participation != 0 {
			return false, nil
		}
		h, err := s.assets.TerrainHeight(s.init.World, deathQuery(e.Pose))
		if err != nil {
			return false, fmt.Errorf("wreck row%d terrain: %w", row, err)
		}
		e.Pose.Height, e.Participation = uint16(h), 4
		s.syncDeath(e)
		return true, nil
	}
	oldTest, countdown, fall := deathFamily(e.Kind)
	if countdown {
		if e.Participation == 2 {
			e.Participation = 3
			s.syncDeath(e)
			return false, nil // The source does not decrement on the2→3 update.
		}
		if e.Participation != 3 {
			return false, nil
		}
		old := e.RawMotion
		e.RawMotion-- // Signed word wraps; the branch tests equality, not <=0.
		ready := e.RawMotion == 0
		if oldTest {
			ready = old == 0
		}
		if !ready {
			s.syncDeath(e)
			return false, nil
		}
		if e.DynamicRaw[2] != 0 && !oldTest {
			// The route-header dispatcher supplies BP-2. Initialization has
			// already recovered that source row; it is not a target/flight ID.
			if e.LeaderRow < 0 || e.LeaderRow >= len(s.init.Entities) {
				return false, fmt.Errorf("destroyed row%d group%02x has no source leader", row, e.DynamicRaw[2])
			}
			leader := &s.init.Entities[e.LeaderRow]
			leader.Participation = 4
			s.syncDeath(leader)
		}
		e.DynamicRaw[2] = 0
		if err := s.replaceDestroyed(row, oldTest); err != nil {
			return false, err
		}
		return false, nil
	}
	if fall == 0 || e.Participation != 2 {
		return false, nil
	}
	// The source has two distinct pitch rules. 29e8 assignsFE00;
	// 2ab8 subtracts0200 until the signedE800 boundary is reached.
	if int16(e.Pose.Angles[0]) > -6144 {
		if fall == 1 {
			e.Pose.Angles[0] = 0xfe00
		} else {
			e.Pose.Angles[0] -= 0x200
		}
	}
	// SHL AH,1 truncates to8bits before CWD/IDIV10.
	step := int16(uint16(byte(in.DF<<1))<<8) / 10
	s.move(&e.Pose, step)
	h, err := s.assets.ContactHeight(s.init.World, deathQuery(e.Pose), s.deathQueryObjects())
	if err != nil {
		return false, fmt.Errorf("destroyed row%d contact: %w", row, err)
	}
	if int16(e.Pose.Height) > h {
		s.syncDeath(e)
		return false, nil
	}
	if fall == 1 {
		e.Pose.Height = uint16(h)
	}
	e.DynamicRaw[2] = 0
	if err := s.replaceDestroyed(row, true); err != nil {
		return false, err
	}
	if fall == 2 {
		e.ControlRaw[1] = 0
		if n := int(int8(e.ControlRaw[3])); n > 0 {
			// Source634+14*n is642+14*(n-1): this is one-based.
			s.emit("contact-stale", -1, row, n-1, 0, "2ab8:a014")
		}
		e.ControlRaw[3] = 0
		s.appendResult(Result{Kind: 3, Actor: byte(row), Timestamp: in.Phase})
		if row&1 != 0 {
			s.Credit[row>>1] = s.Credit[row>>1]&15 | 0x60
		} else {
			// Literal sourceF6 mask retains bit1; do not normalize it toF0.
			s.Credit[row>>1] = s.Credit[row>>1]&0xf6 | 6
		}
	}

	return false, nil
}

func deathFamily(kind byte) (oldTest, countdown bool, fall byte) {
	switch kind {
	case 2, 3, 4, 5, 6, 7, 0xa:
		return true, true, 0 // C07A/C817 tests pre-decrement value.
	case 8, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1b:
		return false, true, 0 // D59D/DA07/DBE8 tests post-decrement value.
	case 0, 1, 0x10, 0x11:
		return false, false, 1
	case 0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26:
		return false, false, 2
	}
	return false, false, 0
}

func (s *State) replaceDestroyed(row int, accumulator bool) error {
	e := &s.init.Entities[row]
	if e.ReplacementModelID < 0 || e.ReplacementModelID >= len(s.assets.Models) {
		return fmt.Errorf("destroyed row%d replacement model%d outside prepared directory", row, e.ReplacementModelID)
	}
	// The source copies272d:1682+38*row into currentDS:460a+38*row.
	// Object.Raw retains the original gameplay definition; these are visual IDs.
	e.ModelID, e.Kind, e.Participation = e.ReplacementModelID, 0x1c, 0
	if s.enemyBehaviorEnabled {
		s.init.ClearSubmarineModel(row)
	}
	if accumulator {
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0xff9c)
	} else {
		e.RawMotion = 4
	}
	s.syncDeath(e)
	s.Events = append(s.Events, Event{Kind: "wreckmodel-ready", Actor: s.init.PlayerRow, Target: row, Source: "157f:replacement38", Pose: e.Pose})
	return nil
}

func deathQuery(p gamestate.InitializedPose) world.QueryPose {
	return world.QueryPose{H0: p.H0, H1: p.H1, Height: int16(p.Height)}
}
func (s *State) deathQueryObjects() []world.QueryObject {
	return s.init.ContactObjects()
}
func (s *State) syncDeath(e *gamestate.InitializedEntity) {
	e.DynamicRaw[0], e.DynamicRaw[1] = e.Kind, e.Participation
	binary.LittleEndian.PutUint16(e.DynamicRaw[6:], uint16(e.RawMotion))
	for i, angle := range e.Pose.Angles {
		binary.LittleEndian.PutUint16(e.DynamicRaw[8+2*i:], angle)
	}
	binary.LittleEndian.PutUint32(e.DynamicRaw[14:], e.Pose.H0)
	binary.LittleEndian.PutUint32(e.DynamicRaw[18:], e.Pose.H1)
	binary.LittleEndian.PutUint16(e.DynamicRaw[22:], e.Pose.Height)
	// Current header remains registered through the1c initialization update.
	e.Visible = e.Exclusion == "" && e.Kind != 0xff && e.Kind&0x40 == 0
}
