package combat

import (
	"encoding/binary"
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// RouteState owns source DATA272D:2e72 PATH cursors and2f12 queued child
// waypoints. It exists only under P5; omission retains historical checkpoints.
type RouteState struct {
	Altitude [4]int16 // actual2FB2..2FB8, source row80..83 queued aliases
	Speed    [4]int16 // actual2FBA..2FC0, source row84..87 queued aliases
	Cursor   [80]uint16
	Queued   [80]int16
}

func newRouteState() *RouteState {
	q := &RouteState{Altitude: followerAltitude, Speed: followerSpeed}
	for i := range q.Queued {
		q.Queued[i] = -1
	}
	return q
}
func (s *State) routePoint(group int, point uint16) (gamestate.MissionPoint, byte, error) {
	head := -1
	g := 0
	for i, p := range s.init.Paths {
		if p.Flag&0x40 != 0 {
			g++
			if g == group {
				head = i + 1
				break
			}
		}
	}
	at := head + int(point)
	if head < 0 || at < 0 || at >= len(s.init.Paths) {
		return gamestate.MissionPoint{}, 0, fmt.Errorf("source PATH group%d point%d lacks actual nine-byte span", group, point)
	}
	p := s.init.Paths[at]
	return p.Point, p.Flag, nil
}
func (s *State) groundOffset(e *gamestate.InitializedEntity) (*gamestate.InitializedEntity, [2]int16, error) {
	code := e.DynamicRaw[2]
	group, slot := int(code>>4), int(code&15)
	offsets, ok := s.init.Offsets[group]
	leader := e.LeaderRow
	if !ok || slot >= len(offsets) || leader < 0 || leader >= e.RuntimeRow || leader >= len(s.init.Entities) || s.init.Entities[leader].Kind != 0x40 {
		return nil, [2]int16{}, fmt.Errorf("ground row%d offset%02x lacks actual earlier PATH leader%d", e.RuntimeRow, code, leader)
	}
	return &s.init.Entities[leader], offsets[slot], nil
}
func (s *State) groundDesired(e *gamestate.InitializedEntity) (*gamestate.InitializedEntity, uint32, uint32, error) {
	leader, offset, err := s.groundOffset(e)
	if err != nil {
		return nil, 0, 0, err
	}
	h := leader.Pose.Angles[1]
	// BB98..BBBA andBBFA..BC14 sign-extend each16-bit trig result
	// separately into the full32-bit coordinate add/subtract chain.
	x := leader.Pose.H0 - uint32(int32(s.sin(h, offset[1]))) + uint32(int32(s.cos(h, offset[0])))
	z := leader.Pose.H1 + uint32(int32(s.cos(h, offset[1]))) + uint32(int32(s.sin(h, offset[0])))
	return leader, x, z, nil
}

// F934..FC55. A header runs before its children in the ascending dispatcher.
// Its stored speed is passed directly to0c6c; child movement separately uses DF.
func (s *State) advanceRouteHeader(row int, in Input) error {
	e := &s.init.Entities[row]
	if s.route == nil {
		return fmt.Errorf("P5 PATH carrier absent")
	}
	code := e.DynamicRaw[2]
	switch e.Participation {
	case 0:
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
		e.Participation = 1
		e.RawMotion = 20
		s.route.Cursor[row] = 0
		p, _, err := s.routePoint(int(code), 0)
		if err != nil {
			return err
		}
		e.Pose.H0, e.Pose.H1 = uint32(p.X), uint32(p.Y)
		s.syncDeath(e)
		return nil
	case 1:
		if code&0x80 != 0 {
			e.Participation = 4
			s.syncDeath(e)
			return nil
		}
	default:
		return nil
	}
	e.DynamicRaw[2] |= 0x80
	p, flag, err := s.routePoint(int(code), s.route.Cursor[row])
	if err != nil {
		return err
	}
	dx, dz := Difference(uint32(p.X), e.Pose.H0), Difference(uint32(p.Y), e.Pose.H1)
	distance := (uint16(abs(dx)) >> 1) + (uint16(abs(dz)) >> 1)
	if distance < uint16(e.RawMotion) {
		e.Pose.H0, e.Pose.H1 = uint32(p.X), uint32(p.Y)
		// Source checks the next seven physical rows, not an abstract group list.
		for n := 1; n < 8; n++ {
			child := row + n
			if s.init.SafeGameplay && child >= 80 {
				break // The typed actor pool ends here; following globals are not group members.
			}
			if child >= 80 {
				codeByte, er := s.routeTailByte(child, in)
				if er != nil {
					return er
				}
				if codeByte>>4 == code {
					if flag&0x40 != 0 {
						if er = s.clearRouteTailByte(child, in); er != nil {
							return er
						}
					} else {
						if child < 84 {
							s.route.Altitude[child-80] = int16(s.route.Cursor[row])
						} else if child < 88 {
							s.route.Speed[child-84] = int16(s.route.Cursor[row])
						} else {
							return fmt.Errorf("PATH leader%d tail queue row%d has no source table owner", row, child)
						}
					}
				}
				continue
			}
			if child >= len(s.init.Entities) {
				return fmt.Errorf("PATH leader%d missing actual row%d", row, child)
			}
			c := &s.init.Entities[child]
			if c.DynamicRaw[2]>>4 != code {
				continue
			}
			if flag&0x40 != 0 {
				c.DynamicRaw[2] = 0
				s.syncDeath(c)
			} else {
				s.route.Queued[child] = int16(s.route.Cursor[row])
			}
		}
		if flag&0x40 != 0 {
			s.syncDeath(e)
			return nil
		}
		s.route.Cursor[row]++
		p, _, err = s.routePoint(int(code), s.route.Cursor[row])
		if err != nil {
			return err
		}
		dx, dz = Difference(uint32(p.X), e.Pose.H0), Difference(uint32(p.Y), e.Pose.H1)
	}
	e.Pose.Angles[1] = s.angle(-dx, dz)
	s.move(&e.Pose, e.RawMotion)
	s.syncDeath(e)
	return nil
}

// BABE..C106 ordinary ground movement. R2.1 owns only the stationary C056 A070
// caller; moving/formation decision branches preserve their older no-attack
// interpretation and never invent a tactical stop.
func (s *State) advanceGroundRoute(row int, in Input) error {
	e := &s.init.Entities[row]
	if s.route == nil {
		return fmt.Errorf("P5 ground PATH carriers absent")
	}
	if e.Participation >= 2 {
		_, err := s.advanceDeathRow(row, in)
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
			e.RawMotion = 20
			leader.RawMotion = 20
			e.Pose.Angles = leader.Pose.Angles
			e.Pose.H0, e.Pose.H1, e.Pose.Height = x, z, leader.Pose.Height
			s.syncDeath(leader)
		} else {
			h, err := s.assets.TerrainHeight(s.init.World, deathQuery(e.Pose))
			if err != nil {
				return err
			}
			if e.Kind == 7 && h < 0 {
				h = 0
			}
			e.Pose.Height = uint16(h)
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
		// The source retains BP-18 from the preceding actual header. A final
		// point can already have cleared this child's DYN+2; its queued point
		// still belongs to that header until the queue is consumed.
		leaderRow := e.LeaderRow
		if leaderRow < 0 || leaderRow >= row || s.init.Entities[leaderRow].Kind != 0x40 {
			return fmt.Errorf("queued ground row%d lacks preceding actual PATH header", row)
		}
		leader := &s.init.Entities[leaderRow]
		leader.DynamicRaw[2] &= 0x7f
		s.syncDeath(leader)
		p, _, err := s.routePoint(int(leader.DynamicRaw[2]), uint16(queued))
		if err != nil {
			return err
		}
		// BCd0 truncates the raw horizontal subtraction to a word, not SAR4 or1EE6.
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
		// C053/C056 is the stationary kinds2..7 caller. Moving/formation
		// callers retain their older no-hostile-decision interpretation.
		_, err := s.stationaryEnemyAttack(row, in)
		return err
	}
	if s.enemyBehaviorEnabled {
		// BE2E reads the preceding PATH header before A070; the formation
		// offset table isn't read until BE65, after the odd detach successor.
		leaderRow := e.LeaderRow
		if leaderRow < 0 || leaderRow >= row || s.init.Entities[leaderRow].Kind != 0x40 {
			return fmt.Errorf("grouped ground row%d lacks earlier PATH header", row)
		}
		leader := &s.init.Entities[leaderRow]
		leader.DynamicRaw[2] &= 0x7f
		s.syncDeath(leader)
		_, selected, er := s.enemyDecision(row, in)
		if er != nil {
			return er
		}
		if selected && e.DynamicRaw[2]&1 != 0 {
			// BE3D AX!=0 is target selection, including no-shot outcomes.
			// BE5C detaches the odd member before ordinary position/jitter draws.
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
		return fmt.Errorf("ground formation requires the shared two LFSR draws")
	}
	// BF5F..BFB7: full signed32 subtraction, SAR2, then retain lowword.
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

func (s *State) sourceFollowerAltitude(index uint16) int16 {
	if s.missionActorsEnabled && s.route != nil {
		return s.route.Altitude[index]
	}
	return followerAltitude[index]
}

// The source's physical seven-row scan crosses DYN80 into other *defined*
// DATA272D owners. Read the reached owners; never synthesize additional actors.
func (s *State) routeTailByte(row int, in Input) (byte, error) {
	switch row {
	case 80:
		if in.HoldHeightTarget != nil {
			return byte(*in.HoldHeightTarget), nil
		}
	case 81:
		if in.TranslationHistory0 != nil {
			return byte(in.TranslationHistory0[2]), nil
		}
	case 82:
		if in.ContactCandidates != nil {
			return byte(in.ContactCandidates[0].Angle >> 8), nil
		}
	case 83:
		if in.ContactCandidates != nil {
			return byte(in.ContactCandidates[5].Angle), nil
		}
	case 84, 85, 86:
		at := 2 + (row-84)*24
		if len(in.WeatherRemap) == 256 {
			return in.WeatherRemap[at], nil
		}
	}
	return 0, fmt.Errorf("PATH tail row%d needs its actual DATA272D byte owner", row)
}
func (s *State) clearRouteTailByte(row int, in Input) error {
	switch row {
	case 80:
		*in.HoldHeightTarget = int16(uint16(*in.HoldHeightTarget) & 0xff00)
	case 81:
		in.TranslationHistory0[2] = int16(uint16(in.TranslationHistory0[2]) & 0xff00)
	case 82:
		in.ContactCandidates[0].Angle &= 0xff
	case 83:
		in.ContactCandidates[5].Angle &= 0xff00
	default:
		return fmt.Errorf("PATH tail row%d reaches a mutable weather-remap alias; per-session presentation owner is not admitted", row)
	}
	return nil
}
func (s *State) sourceFollowerSpeed(index uint16) int16 {
	if s.missionActorsEnabled && s.route != nil {
		return s.route.Speed[index]
	}
	return followerSpeed[index]
}
