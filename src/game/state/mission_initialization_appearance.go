package state

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/world"
)

func (e *InitializedEntity) queryPose() world.QueryPose {
	return world.QueryPose{H0: e.Pose.H0, H1: e.Pose.H1, Height: int16(e.Pose.Height)}
}
func (e *InitializedEntity) motion(n int16) {
	e.RawMotion = n
	binary.LittleEndian.PutUint16(e.DynamicRaw[6:], uint16(n))
}
func (m *InitializedMission) syncEntity(i int) {
	e := &m.Entities[i]
	e.DynamicRaw[0], e.DynamicRaw[1] = e.Kind, e.Participation
	for a, v := range e.Pose.Angles {
		binary.LittleEndian.PutUint16(e.DynamicRaw[8+2*a:], v)
	}
	binary.LittleEndian.PutUint32(e.DynamicRaw[14:], e.Pose.H0)
	binary.LittleEndian.PutUint32(e.DynamicRaw[18:], e.Pose.H1)
	binary.LittleEndian.PutUint16(e.DynamicRaw[22:], e.Pose.Height)
	e.Visible = e.Exclusion == "" && e.Participation != 0 && e.Kind != 0xff && e.Kind&0x40 == 0
	if i == m.PlayerRow {
		e.Visible = true
		m.Player = e.Pose
	}
}

// relativePlacement is the one-time route/formation expansion, not movement.
// The source applies separate Q14 sine/cosine products before signed word sums.
func relativePlacement(leader InitializedPose, offset [2]int16, w *world.Bundle) InitializedPose {
	h := leader.Angles[1]
	x := world.SourceCosAmplitude(h, offset[0], w.Sine) - world.SourceSinAmplitude(h, offset[1], w.Sine)
	z := world.SourceCosAmplitude(h, offset[1], w.Sine) + world.SourceSinAmplitude(h, offset[0], w.Sine)
	return InitializedPose{H0: leader.H0 + uint32(int32(x)), H1: leader.H1 + uint32(int32(z)), Height: leader.Height, Angles: leader.Angles}
}

func (m *InitializedMission) initializeAppearance(w *world.Bundle) error {
	if len(w.Sine) != 1025 || len(w.AngleBases) != 8 || len(w.Atan) != 4096 {
		return fmt.Errorf("initialization needs prepared source angle tables")
	}
	objects := func() []world.QueryObject {
		out := []world.QueryObject{}
		for _, e := range m.Entities {
			flags := binary.LittleEndian.Uint16(e.Object.Raw[:])
			// Height-prefix records are loader-owned, independent of later
			// participation/drawing. Only route markers skip primary loading.
			if flags&2 == 0 || flags&4 != 0 {
				out = append(out, world.QueryObject{ModelID: e.ModelID, Pose: e.queryPose()})
			}
		}
		return out
	}
	// Source mainloop1543 invokes25e2 before the first object update/frame.
	// Apply its contact/height initialization without integrating motion or
	// autonomous controls; source motion/target words remain separate inputs.
	if err := m.initializePlayerContact(w, objects()); err != nil {
		return err
	}
	m.Entities[m.PlayerRow].Pose = m.Player
	// The player loader activates Control+1, not dynamic Participation+1.
	// Keep initialized P0: Kindff has separate ownship rendering, and cannon's
	// direct row0 hit must return at9a9c before stationary damage/RNG/credit.
	// Cold dynamic+6 remains zero for Kindff: loader08d5 activates Control+1
	// without writing motion. Pass-through E2=120 is a separate main-state
	// initializer; late3eca publishes only pose/angles, not this speed carrier.
	// 0afe:0267 builds PATH heads after each flag40 record. Index0 is
	// intentionally absent; generated formation routes begin at group2.
	pathHeads := map[int]int{}
	pathGroup := 0
	for i, p := range m.Paths {
		if p.Flag&0x40 != 0 {
			pathGroup++
			pathHeads[pathGroup] = i + 1
		}
	}
	for i := range m.Entities {
		e := &m.Entities[i]
		if e.Kind == 0x40 {
			g := int(e.DynamicRaw[2])
			at, ok := pathHeads[g]
			if !ok || at >= len(m.Paths) {
				return fmt.Errorf("row%d route group%d lacks consumed PATH head", i, g)
			}
			e.Pose.H0, e.Pose.H1 = uint32(m.Paths[at].Point.X), uint32(m.Paths[at].Point.Y)
			e.Participation = 1
			e.motion(20)
			m.syncEntity(i)
			continue
		}
		if e.Exclusion != "" {
			continue
		}
		if i == m.PlayerRow {
			m.syncEntity(i)
			continue
		}
		if e.Kind == 0xff {
			e.Exclusion = "non-primary FF row has no ordinary dynamic submission"
			m.syncEntity(i)
			continue
		}
		if e.Kind&0x40 != 0 {
			e.Exclusion = "source type40/41 has special route/effect presentation, no ordinary instance"
			m.syncEntity(i)
			continue
		}
		if e.Kind == 0x21 {
			leader := e.LeaderRow
			if leader < 0 || leader >= i {
				return fmt.Errorf("row%d follower leader%d is not an earlier initialized entity", i, leader)
			}
			code := e.DynamicRaw[2]
			g, slot := int(code>>4), int(code&15)
			if slot == 0 || slot > len(m.Offsets[g]) {
				return fmt.Errorf("row%d flight offset%02x outside initialized group", i, code)
			}
			e.Pose = relativePlacement(m.Entities[leader].Pose, m.Offsets[g][slot-1], w)
			if m.Switches&0x1000 != 0 && e.Pose.Height == 0x16a {
				// 6bb3..6c09 carrier followers retain the raw leader line;
				// this branch uses offsetX along H1, not the normal rotation.
				e.Pose.H0 = uint32(m.Rows[0].Point().X)
				e.Pose.H1 = uint32(m.Rows[leader].Point().Y) + uint32(int32(m.Offsets[g][slot-1][0]))
			}
			e.Participation = 1
			e.motion(0)
			m.syncEntity(i)
			continue
		}
		if e.DynamicRaw[2] != 0 && (e.Kind == 0 || e.Kind >= 2 && e.Kind <= 8 || e.Kind == 0xa || e.Kind == 0x12 || e.Kind == 0x13 || e.Kind == 0x14 || e.Kind == 0x16 || e.Kind == 0x17 || e.Kind == 0x18) {
			// Source group-relative records use the preceding leader and the
			// offset indexed directly by the low nibble (flight21 uses n−1).
			code := e.DynamicRaw[2]
			g, slot := int(code>>4), int(code&15)
			if e.LeaderRow < 0 || e.LeaderRow >= i || slot >= len(m.Offsets[g]) {
				return fmt.Errorf("row%d unresolved formation offset%02x/leader%d", i, code, e.LeaderRow)
			}
			e.Pose = relativePlacement(m.Entities[e.LeaderRow].Pose, m.Offsets[g][slot], w)
		}
		terrain := func() error {
			y, err := w.TerrainHeight(m.World, e.queryPose())
			if err != nil {
				return err
			}
			e.Pose.Height = uint16(y)
			return nil
		}
		switch e.Kind {
		case 0:
			e.Participation = 1
			e.motion(100)
			if e.DynamicRaw[2] != 0 {
				e.motion(40)
				m.Entities[e.LeaderRow].motion(40)
			} else {
				e.Pose.Height = 0x400
			}
		case 1:
			e.Participation = 1
			e.Pose.Height = 0x400
			e.motion(400)
		case 2, 3, 4, 5, 6, 7:
			e.Participation = 1
			e.motion(0)
			if e.DynamicRaw[2] != 0 {
				e.motion(20)
				m.Entities[e.LeaderRow].motion(20)
			} else {
				if err := terrain(); err != nil {
					return fmt.Errorf("row%d:%w", i, err)
				}
			}
			if e.Kind == 7 && int16(e.Pose.Height) < 0 {
				e.Pose.Height = 0
			}
		case 0xa:
			e.Participation = 1
			e.motion(0)
			if e.DynamicRaw[2] != 0 {
				e.motion(50)
				m.Entities[e.LeaderRow].motion(50)
			}
		case 0x10, 0x11:
			e.Pose.Height = 0x600
			e.motion(400)
			e.Participation = 0
			e.Exclusion = "source airborne reserve awaits AA2 activation; autonomous activation suppressed"
		case 8, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18:
			e.Participation = 1
			e.motion(30)
			if e.DynamicRaw[2] != 0 {
				e.motion(25)
				m.Entities[e.LeaderRow].motion(25)
			} else {
				if err := terrain(); err != nil {
					return fmt.Errorf("row%d:%w", i, err)
				}
			}
			if e.Kind == 0x17 && int16(e.Pose.Height) < 0 {
				e.Pose.Height = 0
			}
			if e.Kind == 0x15 {
				dx := int16((int32(m.Player.H0) >> 4) - (int32(e.Pose.H0) >> 4))
				dz := int16((int32(m.Player.H1) >> 4) - (int32(e.Pose.H1) >> 4))
				e.Pose.Angles[1] = world.SourceAngle(-dx, dz, w)
			}
		case 0x19:
			e.Participation = 1
			e.motion(30)
		case 0x1b:
			e.Participation = 0
			e.motion(30)
			e.Pose.Height = 0
			e.Exclusion = "source follower-status reserve has no initial participation"
		case 0x1c, 0x1d:
			e.Participation = 4
			e.motion(0)
			if err := terrain(); err != nil {
				return fmt.Errorf("row%d:%w", i, err)
			}
		case 0x20:
			e.Participation = 1
			e.motion(0)
			e.Pose.Angles[0], e.Pose.Angles[2] = 0, 0
			y, err := w.ContactHeight(m.World, e.queryPose(), objects())
			if err != nil {
				return fmt.Errorf("row%d contact:%w", i, err)
			}
			if int16(e.Pose.Height) < y+2 {
				e.Pose.Height = uint16(y + 2)
			}
			if m.Switches&0x1000 != 0 && int16(e.Pose.Height) == 0x16a {
				cell, err := w.SourceCellID(m.World, e.queryPose())
				if err != nil {
					return err
				}
				if cell == 0x3e {
					e.Pose.H0 = m.Player.H0
					e.Pose.Angles[1] = 0
				}
			}
		case 0x22, 0x23, 0x24:
			if e.ControlCarrierOnly {
				return fmt.Errorf("row%d transient kind%02x needs its live control+8 producer; static carrier overlaps launch DATA", i, e.Kind)
			}
			e.Participation = 1
			// 10bb:744e uses byte control+8 as x86's masked shift count.
			e.Pose.Angles[0] = uint16(uint32(0xff00) << (e.ControlRaw[8] & 31))
		case 0x25:
			e.Participation = 1
			e.DynamicRaw[3] = 30
		default:
			// No ordinary source dispatch exists for these initial kinds. Retain
			// their bytes for future systems rather than drawing a fabricated unit.
			e.Participation = 0
			e.Exclusion = fmt.Sprintf("source kind%02x has no supported initial ordinary participation", e.Kind)
		}
		m.syncEntity(i)
	}
	if m.Switches&0x800 != 0 {
		m.Status = append(m.Status, "Pass-through uses source pitchF400/motion120/height-target300 and first height-control/contact operations; motion is retained without integration.")
	}
	m.Status = append(m.Status, "Autonomous decisions and motion integration are inactive; source initial motion and routes are retained.")
	for i := range m.Entities {
		e := &m.Entities[i]
		// Preserve the actual enemy-air target predicate independently of a
		// broad allegiance label (10bb:2cad..2ce9). Zero is not enemy by default.
		e.EnemyAirTarget = e.Kind&0x60 == 0 && e.SourceDisposition < 0 && e.Participation == 1 && e.Kind&0x10 != 0
		switch {
		case e.FlightSlot >= 0 || i == m.PlayerRow:
			e.Side, e.SideEvidence = "friendly", "accepted primary/flight assignment"
		case e.EnemyAirTarget:
			e.Side, e.SideEvidence = "friendly", "negative OBJECT1b/type10 accepted by enemy-air target consumer2cad"
		case e.Object.Raw[11] != 0 && e.Kind < 2:
			e.Side, e.SideEvidence = "enemy", "original briefing enemy-air population consumer"
		case e.Object.Raw[11] != 0 && e.Object.Raw[0]&0x30 == 0 && e.Kind > 2 && e.Kind < 16:
			e.Side, e.SideEvidence = "enemy", "original briefing enemy-ground population consumer"
		default:
			e.SideEvidence = "no universal source allegiance predicate established; raw kind/flags/disposition retained"
		}
	}
	return nil
}

func (m *InitializedMission) initializePlayerContact(w *world.Bundle, objects []world.QueryObject) error {
	h := m.Player.Angles[1]
	s, c := world.SourceSinAmplitude(h, 48, w.Sine), world.SourceCosAmplitude(h, 48, w.Sine)
	// 0975:35fe..3729 tests the four rotated footprint points in this order.
	points := [][2]int16{{-s, c}, {c, s}, {s, -c}, {-c, -s}}
	var highest int16
	for i, off := range points {
		pose := world.QueryPose{H0: m.Player.H0 + uint32(int32(off[0])), H1: m.Player.H1 + uint32(int32(off[1])), Height: int16(m.Player.Height)}
		y, err := w.ContactHeight(m.World, pose, objects)
		if err != nil {
			return fmt.Errorf("player contact probe%d:%w", i, err)
		}
		if i == 0 || y > highest {
			highest = y
		}
	}
	m.InitialContactHeight = highest
	if m.Switches&0x800 != 0 {
		// Import setsAE1/ADB and target300. 37a4 uses target directly;
		// avoidance40 instead averages target,currentheight,ground+2 at3792.
		y := m.InitialHeightTarget
		if m.Switches&0x40 != 0 {
			y = int16(uint16(y)+m.Player.Height+uint16(highest)+2) >> 1
		}
		m.Player.Height = uint16(y)
	}
	// Height control does not bypass the later contact test391e. AE1's
	// contact branch3a2e reaches3b86 and clears forward/vertical motion.
	if int16(m.Player.Height) < highest+2 {
		m.Player.Height = uint16(highest + 2)
		m.InitialForwardMotion = 0
	}
	return nil
}
