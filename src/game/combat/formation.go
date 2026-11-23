package combat

import (
	"encoding/binary"
	"fmt"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"sort"
)

// SetFormationEnabled selects an interpretation, not mutable gameplay input.
// Historical films keep their original stationary kind21/RNG semantics.
func (s *State) SetFormationEnabled(enabled bool) { s.formationEnabled = enabled }

// FormationEnabled is the launch-selected interpretation, never a viewer toggle.
func (s *State) FormationEnabled() bool { return s.formationEnabled }

// StageDevelopmentFormation is only for explicitly supplied development
// approaches. Relocating player0 alone leaves distant P1 followers whose source
// LOW16 delta cannot represent that teleport. Rebuild eligible following poses
// through the real P0 initializer, without visiting P1 or drawing randomness.
// Live movement and historical film operations never call this staging owner.
func (s *State) StageDevelopmentFormation() (int, error) {
	if !s.formationEnabled {
		return 0, nil
	}
	// P0 only mutates the entity row. Keep this fixture transaction detached so
	// an unproved later leader/offset cannot publish a partly rebased formation.
	mission := *s.init
	mission.Entities = append([]gamestate.InitializedEntity(nil), s.init.Entities...)
	candidate := *s
	candidate.init = &mission
	var rows []int
	for actor := 1; actor <= 4 && actor < len(mission.Entities); actor++ {
		e := &mission.Entities[actor]
		if e.Kind != 0x21 || e.Participation != 1 {
			continue
		}
		if err := candidate.followerProvenance(actor); err != nil {
			return 0, err
		}
		if mission.Stores.Damage[actor][9] != 0 || mission.FollowerFailureKnown[actor] {
			continue // A supplied approach cannot revive/reclassify a failure.
		}
		if mission.FollowerLimits[actor] != "" {
			return 0, fmt.Errorf("development formation actor%d retains exclusion: %s", actor, mission.FollowerLimits[actor])
		}
		_, leader, leaderRow, err := candidate.formationOwners(actor)
		if err != nil {
			return 0, err
		}
		if leaderRow != mission.PlayerRow && (mission.FollowerFailureKnown[leaderRow] || mission.Stores.Damage[leaderRow][9] != 0 || leader.Kind != 0x20 && leader.Kind != 0x21 || leader.Participation != 1) {
			continue // Keep the real dependent failure/successor ownership.
		}
		e.Participation = 0
		if err := candidate.advanceFormation(actor, Input{}); err != nil {
			return 0, fmt.Errorf("development formation actor%d: %w", actor, err)
		}
		rows = append(rows, actor)
	}
	for _, actor := range rows {
		s.init.Entities[actor] = mission.Entities[actor]
	}
	return len(rows), nil
}

func (s *State) formationOwners(actor int) (*gamestate.InitializedEntity, *gamestate.InitializedEntity, int, error) {
	if err := s.followerProvenance(actor); err != nil {
		return nil, nil, 0, err
	}
	m := s.init
	e := &m.Entities[actor]
	leader := int(int8(e.ControlRaw[5]))
	if leader < 0 || leader >= actor || leader > 4 {
		return nil, nil, leader, invalidFlightOrder(fmt.Sprintf("formation row%d lacks its actual earlier CONTROL leader%d", actor, leader))
	}
	if e.ControlCarrierOnly || m.Entities[leader].RuntimeRow != leader || m.Entities[leader].ControlCarrierOnly {
		return nil, nil, leader, fmt.Errorf("formation row%d lacks its actual earlier CONTROL leader%d", actor, leader)
	}
	return e, &m.Entities[leader], leader, nil
}
func (s *State) formationOffset(e *gamestate.InitializedEntity) ([2]int16, error) {
	code := e.DynamicRaw[2]
	group, slot := int(code>>4), int(code&15)
	offsets, ok := s.init.Offsets[group]
	if !ok || slot == 0 || slot > len(offsets) {
		return [2]int16{}, invalidFlightOrder(fmt.Sprintf("formation row%d source offset%02x unavailable", e.RuntimeRow, code))
	}
	return offsets[slot-1], nil
}

// Source314c reads pose+8 even though E411..E636 only writes the two dwords
// of its temporary corner pose. BP-30 has no kind21 initializer; the saved old
// pose is BP-12 and does not alias it. Admit a query only if its result is
// identical over every possible signed-word operand, rather than guessing zero.
func (s *State) formationCornerHeight(p gamestate.InitializedPose) (int16, error) {
	var result int16
	seen := false
	_, err := s.formationCornerOutputs(p, func(v int16) error {
		if seen && v != result {
			return fmt.Errorf("formation corner depends on unwritten source BP-30 height")
		}
		seen, result = true, v
		return nil
	})
	if err != nil {
		return 0, err
	}
	return result, err
}

// formationCornerBlocked returns only the boolean consumed at E453. Different
// undefined-height query outputs are admissible when this signed comparison is
// unanimous. A numeric representative would invent an original stack operand.
func (s *State) formationCornerBlocked(p gamestate.InitializedPose, limit int16) (bool, error) {
	if s.dispatcherScratchEnabled && s.dispatcherScratchKnown || s.safeFlightOrders {
		q := deathQuery(p)
		if s.dispatcherScratchEnabled && s.dispatcherScratchKnown {
			q.Height = s.dispatcherScratchHeight
		}
		h, err := s.assets.ContactHeight(s.init.World, q, s.deathQueryObjects())
		return limit < h, err
	}
	values, err := s.formationCornerOutputs(p)
	if err != nil {
		return false, err
	}
	blocked := limit < values[0]
	for _, v := range values[1:] {
		if (limit < v) != blocked {
			return false, fmt.Errorf("formation corner decision depends on unwritten source BP-30 height")
		}
	}
	return blocked, nil
}

func (s *State) formationCornerThresholds(p gamestate.InitializedPose) ([]int, error) {
	b := s.assets
	cell, err := b.SourceCellID(s.init.World, deathQuery(p))
	if err != nil {
		return nil, err
	}
	w := b.Worlds[s.init.World]
	if w == nil || int(cell) >= len(w.Cells) {
		return nil, fmt.Errorf("formation corner cell unavailable")
	}
	thresholds := map[int16]bool{-32768: true}
	add := func(g *world.Geometry, x, z int16) error {
		if g == nil {
			return fmt.Errorf("formation corner geometry unavailable")
		}
		for _, r := range g.Heights {
			if r.Threshold != 0 {
				inside, e := world.SourceContains(r.Points, x, z)
				if e != nil {
					return e
				}
				if inside {
					thresholds[r.Threshold] = true
				}
			}
		}
		return nil
	}
	objects := s.deathQueryObjects()
	for _, o := range objects {
		if o.Pose.H0>>13 != p.H0>>13 || o.Pose.H1>>13 != p.H1>>13 {
			continue
		}
		if o.ModelID < 0 || o.ModelID >= len(b.Models) {
			return nil, fmt.Errorf("formation corner model%d unavailable", o.ModelID)
		}
		x := int16((p.H0&0x1fff)|1) - int16(o.Pose.H0&0x1fff)
		z := int16((p.H1&0x1fff)|1) - int16(o.Pose.H1&0x1fff)
		if err := add(b.Geometries[b.Models[o.ModelID]], x, z); err != nil {
			return nil, err
		}
	}
	x := (int16(p.H0&0x1fff) - 0x1000) | 1
	z := (int16(p.H1&0x1fff) - 0x1000) | 1
	if err := add(b.Geometries[w.Cells[cell].Geometry], x, z); err != nil {
		return nil, err
	}
	values := make([]int, 0, len(thresholds))
	for h := range thresholds {
		values = append(values, int(h))
	}
	sort.Ints(values)
	return values, nil
}

func (s *State) formationCornerOutputs(p gamestate.InitializedPose, consumers ...func(int16) error) ([]int16, error) {
	values, err := s.formationCornerThresholds(p)
	if err != nil {
		return nil, err
	}
	objects := s.deathQueryObjects()
	b := s.assets
	results := make([]int16, 0, len(values))
	for _, h := range values {
		q := deathQuery(p)
		q.Height = int16(h)
		v, e := b.ContactHeight(s.init.World, q, objects)
		if e != nil {
			return nil, e
		}
		results = append(results, v)
		if len(consumers) != 0 {
			if err := consumers[0](v); err != nil {
				return nil, err
			}
		}
	}
	return results, nil
}

func formationDelta(current, desired uint32, draw uint16) int16 {
	// E693..E6D9: signed32 SAR2, followed by LOW WORD storage/sign extension.
	return int16(int32(desired-current+uint32(draw&15)-8) >> 2)
}
func formationHeight(own, leader, query, low int16) int16 {
	if leader < query {
		return (int16(query+low) >> 1) + (own >> 1)
	}
	return (own >> 1) + (leader >> 1)
}
func (s *State) advanceFormation(actor int, in Input) error {
	if err := s.followerProvenance(actor); err != nil {
		return err
	}
	e := &s.init.Entities[actor]
	if e.Participation == 2 {
		_, err := s.advanceDeathRow(actor, in)
		return err
	}
	if e.Participation != 0 && e.Participation != 1 {
		return nil
	}
	e, leader, leaderRow, err := s.formationOwners(actor)
	if err != nil {
		return err
	}
	m := s.init
	if s.followExitedSection(actor, leader) {
		return nil
	}
	switch e.Participation {
	case 0:
		// DFB1 initializes only this visit; it must not also run P1's RNG/callback.
		if !s.logisticsEnabled && m.Switches&0x1000 != 0 && leader.Pose.Height == 0x16a {
			return fmt.Errorf("formation P0 carrier reference pose is outside admitted ownership")
		}
		off, err := s.formationOffset(e)
		if err != nil {
			return err
		}
		if s.logisticsEnabled && m.Switches&0x1000 != 0 && leader.Pose.Height == 0x16a {
			e.Pose.H0, e.Pose.H1 = s.carrierFormationReference(leaderRow, off[0])
		} else {
			h := leader.Pose.Angles[1]
			origin := e.Pose
			e.Pose.H0 = leader.Pose.H0 + uint32(int32(s.cos(h, off[0]))) - uint32(int32(s.sin(h, off[1])))
			e.Pose.H1 = leader.Pose.H1 + uint32(int32(s.sin(h, off[0]))) + uint32(int32(s.cos(h, off[1])))
			if s.corridorNavigation && insideWorld(origin) && !insideWorld(e.Pose) {
				e.Pose.H0, e.Pose.H1 = origin.H0, origin.H1
			}
		}
		e.Pose.Height, e.Pose.Angles = leader.Pose.Height, leader.Pose.Angles
		binary.LittleEndian.PutUint16(e.DynamicRaw[4:], 0)
		e.Participation = 1
		s.syncDeath(e)
		return nil
	case 1:
	default:
		return nil
	}
	old := e.Pose
	// These tests address byte3BE5, the HIGH byte of native SWITCHES word.
	if m.Switches&0x800 != 0 && leader.Kind == 0x41 {
		e.Kind = 0x41
		e.ControlRaw[1] = 0
		s.syncDeath(e)
		return nil
	}
	if leader.ControlRaw[1] == 0 {
		binary.LittleEndian.PutUint16(e.ControlRaw[6:], 1)
		binary.LittleEndian.PutUint16(e.ControlRaw[8:], 3)
		e.ControlRaw[1] = 5
		e.Participation = 0
		e.Kind = 0x25
		s.syncDeath(e)
		if s.logisticsEnabled {
			return nil
		}
		return fmt.Errorf("formation leader control0 reached unimplemented kind25 successor")
	}
	carrier := false
	if m.Switches&0x1000 != 0 {
		cell, err := s.assets.SourceCellID(m.World, deathQuery(e.Pose))
		if err != nil {
			return err
		}
		if cell == 0x3e && (e.Pose.Height == 0x16a || leader.Pose.Height == 0x16a || leader.Kind == 0x26 || leaderRow == m.PlayerRow) {
			if !s.logisticsEnabled {
				return fmt.Errorf("formation carrier material3e requires source ship/query reference owners")
			}
			carrier = e.Pose.Height == 0x16a || leader.Pose.Height == 0x16a || leader.Kind == 0x26 || leaderRow == m.PlayerRow && in.PlayerContactHeight == 0x168
		}
	}
	if in.Random == nil {
		return fmt.Errorf("formation requires shared gameplay RNG")
	}
	off, err := s.formationOffset(e)
	if err != nil {
		return err
	}
	heading := leader.Pose.Angles[1]
	desired0 := leader.Pose.H0 + uint32(int32(s.cos(heading, off[0]))) - uint32(int32(s.sin(heading, off[1])))
	desired1 := leader.Pose.H1 + uint32(int32(s.sin(heading, off[0]))) + uint32(int32(s.cos(heading, off[1])))
	alternate := int16(int16(off[1]-64)*2) - ((off[0] &^ int16(2)) >> 1)
	if carrier {
		desired0, desired1 = s.carrierFormationReference(leaderRow, off[0])
		heading = 0
		e.Pose.Angles = [3]uint16{}
	} else {
		queryHeight := e.Pose.Height // Same local fallback for all four corners.
		for _, d := range [][2]int32{{512, 512}, {512, -512}, {-512, 512}, {-512, -512}} {
			q := gamestate.InitializedPose{H0: desired0 + uint32(d[0]), H1: desired1 + uint32(d[1])}
			if s.safeFlightOrders {
				q.Height = queryHeight
			}
			if s.corridorNavigation {
				q = gridProbe(e.Pose, q)
			}
			blocked := false
			if s.logisticsEnabled {
				blocked, err = s.formationCornerBlocked(q, int16(leader.Pose.Height+16))
			} else {
				var height int16
				height, err = s.formationCornerHeight(q)
				blocked = int16(leader.Pose.Height+16) < height
			}
			if err != nil {
				return err
			}
			if blocked {
				desired0 = leader.Pose.H0 - uint32(int32(s.sin(heading, alternate)))
				desired1 = leader.Pose.H1 + uint32(int32(s.cos(heading, alternate)))
				break
			}
		}
	}
	e.RawMotion = leader.RawMotion
	if leaderRow == m.PlayerRow {
		e.RawMotion = in.PlayerIndicatedSpeed
	}
	if !carrier {
		e.Pose.Angles[2] = (int16AngleHalf(leader.Pose.Angles[2])) + (int16AngleHalf(e.Pose.Angles[2]))
	}
	dx := formationDelta(e.Pose.H0, desired0, in.Random())
	dz := formationDelta(e.Pose.H1, desired1, in.Random())
	e.Pose.H0 += uint32(int32(dx))
	e.Pose.H1 += uint32(int32(dz))
	if s.corridorNavigation && insideWorld(old) && !insideWorld(e.Pose) {
		e.Pose.H0, e.Pose.H1 = old.H0, old.H1
	}
	if carrier {
		// E2C6 joins at E689, bypassing the ordinary attitude owner.
	} else if leader.Kind == 0x20 {
		e.Pose.Angles[0], e.Pose.Angles[2] = 0, 0
	} else {
		e.Pose.Angles[0] += uint16(int16(leader.Pose.Angles[0]-e.Pose.Angles[0]) >> 3)
		if leaderRow == m.PlayerRow && (uint16(abs(dx)) > 16 || uint16(abs(dz)) > 16) {
			if in.PlayerSpeed < 0 {
				e.Pose.Angles[1] = s.angle(dx, -dz)
			} else {
				e.Pose.Angles[1] = s.angle(-dx, dz)
			}
		} else {
			e.Pose.Angles[1] += uint16(int16(heading-e.Pose.Angles[1]) >> 3)
		}
	}
	s.retainDispatcherHeight(e.Pose.Height) // E7E1..E7ED current pose before E833 altitude mutation.
	height, err := s.assets.PairedTerrainHeight(m.World, deathQuery(e.Pose), deathQuery(old))
	if err != nil {
		return err
	}
	target := formationHeight(int16(e.Pose.Height), int16(leader.Pose.Height), height, in.LowLimit)
	// A formation member may still be over the last roof after its leader has
	// left the square. Keep its own clearance until its footprint moves on.
	if !carrier && s.lowCityRoute(leaderRow, leader) {
		floor := s.city.at(e.Pose.H0, e.Pose.H1)
		if floor > 0 && floor > target {
			own := int16(e.Pose.Height)
			target = max(target, own+min((floor-own)>>3, s.sourceFollowerAltitude(3)>>3))
		}
	}
	e.Pose.Height = uint16(target)
	if s.missionActorsEnabled {
		if err := s.advanceHealthyFollower(actor, in); err != nil {
			return err
		}
	} else {
		s.disableFollower(actor)
	}
	s.syncDeath(e)
	return nil
}

func (s *State) carrierFormationReference(leaderRow int, offset int16) (uint32, uint32) {
	// E013/E2C6 use the retained OBJECT epoch, not live dynamic poses.
	h0 := binary.LittleEndian.Uint32(s.init.Rows[0].Raw[2:6])
	h1 := binary.LittleEndian.Uint32(s.init.Rows[leaderRow].Raw[6:10]) + uint32(int32(offset))
	return h0, h1
}
func int16AngleHalf(v uint16) uint16 { return uint16(int16(v) >> 1) }
