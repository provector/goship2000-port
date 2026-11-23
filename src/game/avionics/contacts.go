// Package avionics owns transient contact/target observations. Rendering receives
// copies; neither visual entities nor presentation Side labels establish detection.
package avionics

import (
	"fmt"
	"gunship2000-assets/game/world"
)

const EntityCapacity, CandidateCapacity = 80, 8

type Pose struct {
	H0, H1 uint32
	Height int16
	Angles [3]uint16
}
type Entity struct {
	Present             bool
	Row                 int
	Kind, Participation byte
	Pose                Pose
	// Source raw carriers are retained for consumer-specific predicates/aliases.
	Object     [32]byte
	Control    [70]byte
	FlightSlot int
}

// Contact is source DATA642+14*row. Yaw is not a cached distance; height belongs
// to the live entity and must not be invented inside this retained observation.
type Contact struct {
	Flags, Kind byte
	AttackFlags uint16
	Yaw         uint16
	H0, H1      uint32
}
type Candidate struct {
	Row             byte
	Distance, Angle uint16
}
type Descriptor struct {
	Remaining byte
	Status    uint16
	Pose      Pose
	Angles    [3]uint16
}
type Input struct {
	Entities               [EntityCapacity]Entity
	Player                 Pose
	Head                   Pose
	Mast                   bool
	Speed                  int16
	Switches               uint16
	PlayerRow              int
	DamageMask             byte
	World                  string
	Assets                 *world.Bundle
	Random                 func() uint16
	ProgressAwareTargeting bool // R8 launch policy; false preserves the eight-attempt interpretation.
	ModernTail             bool // Launch-selected P4 interpretation; historical films keep the old tail.
	DisableAutonomy        bool // Native R1 policy: preserve sensing and shared following targets, skip independent decisions.
	SetControlTarget       func(actor int, target int8) error
	ContactOrder           func(actor, target int) error
}
type State struct {
	playerSighted             [EntityCapacity]bool // GPS memory, reconstructed by playback; never used for target selection.
	Contacts                  [EntityCapacity]Contact
	Candidates                [CandidateCapacity]Candidate
	Target                    uint16 // AB9: one-based raw entity row; zero is no player lock.
	Cursor                    uint16 // AB5: next candidate index, not an entity row.
	Suppressed                uint16
	BodyQuery, HeadQuery      Descriptor
	MFDTargetRequested        bool
	TailDraws                 [5]uint16
	Permutation               [4]uint16
	Updates                   uint64
	UnsupportedObserverTokens uint64
	TargetCache               TargetHistory
	Threat                    ThreatDisplay
	TargetPanelZoom           uint16
	QueryRetryBoundHits       uint64
	retryLoggedReasons        uint8 // Diagnostic deduplication only; not serialized/checkpointed.
	CompressionLimited        bool  // Reached source5395 contact-tail predicate; logical phase only.
}
type TargetHistory struct {
	Target uint16
	Zoom   uint16
}

func New() *State {
	s := &State{Permutation: [4]uint16{1, 2, 3, 4}}
	for i := range s.Candidates {
		s.Candidates[i].Distance = 0xffff
	}
	return s
}
func AbsWord(n int16) uint16 {
	if n < 0 {
		return uint16(-n)
	}
	return uint16(n)
}
func Delta(a, b uint32) int16 { return int16((int32(a) >> 4) - (int32(b) >> 4)) }

// Metric preserves signed comparisons and low-word overflow, not Euclidean range.
func Metric(dx, dz int16) uint16 {
	a, b := int16(AbsWord(dx)), int16(AbsWord(dz))
	if a > b {
		return uint16(a + (b >> 1))
	}
	return uint16(b + (a >> 1))
}
func Envelope(height, speed int16, grey, night, mast bool) uint16 {
	v := uint16(height << 3)
	if mast {
		v = uint16((height - int16(AbsWord(speed))) << 3)
	}
	if grey {
		v >>= 1
	} else if night {
		v -= v >> 2
	}
	if v < 0x400 {
		return 0x400
	}
	if v > 0x1000 {
		return 0x1000
	}
	return v
}
func Budget(switches uint16) byte {
	if switches&0x100 != 0 {
		return 32
	}
	if switches&0x200 != 0 {
		return 48
	}
	return 64
}
func bearing(a, b Pose, t *world.Bundle) uint16 {
	return world.SourceAngle(-Delta(b.H0, a.H0), Delta(b.H1, a.H1), t)
}
func blocked(c *Contact) {
	if c.Flags&7 == 2 {
		c.Flags = c.Flags&0xf9 | 1
	}
}
func observe(c *Contact, e Entity, observer byte) {
	c.Flags = (c.Flags&7&0xfa | 2) | observer<<3
	c.Kind = e.Kind
	c.Yaw = e.Pose.Angles[1]
	c.H0 = e.Pose.H0
	c.H1 = e.Pose.H1
}

// SelectManual consumes the preceding scan. The caller queues this semantic edge
// before Advance; it must not refresh candidates first merely for responsiveness.
func (s *State) SelectManual() bool {
	old := s.Target
	s.Suppressed = old
	s.Target, s.Cursor = s.selectTarget(int(s.Cursor))
	if s.Target != 0 && s.Target == old {
		s.Target = 0
	} else if s.Target != 0 {
		s.Suppressed = 0
		s.MFDTargetRequested = true
	}
	return s.Target != 0
}
func (s *State) selectTarget(start int) (uint16, uint16) {
	start &= 7
	i := start
	valid := func(i int) bool {
		c := s.Candidates[i]
		return c.Distance < 0x1000 && c.Angle < 0x2000 && int(c.Row) < EntityCapacity && s.Contacts[c.Row].Flags&7 == 2
	}
	target := uint16(0)
	if valid(i) {
		target = uint16(s.Candidates[i].Row) + 1
	} else {
		for n := 0; n < 8; n++ {
			i = (i + 1) & 7
			if valid(i) {
				target = uint16(s.Candidates[i].Row) + 1
				break
			}
		}
	}
	if target != 0 && s.Contacts[target-1].Kind&0x10 != 0 {
		for n := 0; n < 8; n++ {
			i = (i + 1) & 7
			if valid(i) && s.Contacts[s.Candidates[i].Row].Kind&0x10 == 0 {
				target = uint16(s.Candidates[i].Row) + 1
				break
			}
		}
	}
	if target != 0 {
		return target, uint16((i + 1) & 7)
	}
	return 0, s.Cursor
}
func (s *State) scan(in Input) {
	for i := range s.Candidates {
		s.Candidates[i].Distance = 0xffff
	}
	limit := Envelope(in.Player.Height, in.Speed, in.Switches&0x100 != 0, in.Switches&0x200 != 0, false)
	origin, yaw := in.Player, in.Player.Angles[1]
	if in.Mast {
		limit = Envelope(in.Head.Height, in.Speed, in.Switches&0x100 != 0, in.Switches&0x200 != 0, true)
		yaw = in.Head.Angles[1]
	}
	for row := 1; row < EntityCapacity; row++ {
		e := in.Entities[row]
		if !e.Present || e.Kind&0x60 != 0 || e.Participation != 1 {
			continue
		}
		score := Metric(Delta(e.Pose.H0, origin.H0), Delta(e.Pose.H1, origin.H1))
		angle := AbsWord(int16(bearing(origin, e.Pose, in.Assets) - yaw))
		if score < 4 || score > limit || e.Kind&0x10 != 0 && angle > 0x2000 {
			continue
		}
		for at, c := range s.Candidates {
			if c.Distance > score {
				copy(s.Candidates[at+1:], s.Candidates[at:7])
				s.Candidates[at] = Candidate{byte(row), score, angle}
				break
			}
		}
	}
}

// ownerAlive implements admitted SHIP0..4 and the explicit token31 OBJECT13+25
// alias. Unknown active-NPC aliases are counted and never invented as IFF.
func ownerAlive(token byte, in Input) (bool, bool) {
	if token < 5 {
		for _, e := range in.Entities {
			if e.Present && e.FlightSlot == int(token) {
				return int8(e.Control[3]) < 0, true
			}
		}
		return false, true
	}
	if token == 31 && in.Entities[13].Present {
		return int8(in.Entities[13].Object[25]) < 0, true
	}
	return false, false
}
func (s *State) Advance(in Input) error {
	s.CompressionLimited = false
	if in.Assets == nil || len(in.Assets.Sine) != 1025 || len(in.Assets.Atan) != 4096 || len(in.Assets.AngleBases) != 8 || in.Random == nil {
		return fmt.Errorf("contacts require source angle tables and shared logical RNG")
	}
	if s.Target > EntityCapacity {
		return fmt.Errorf("contact target%d outside source one-based0..80", s.Target)
	}
	if in.PlayerRow != 0 {
		return fmt.Errorf("contacts currently admit source player row0")
	}
	for row := range s.Contacts {
		c := &s.Contacts[row]
		if c.Flags&2 != 0 {
			alive, known := ownerAlive(c.Flags>>3, in)
			if !known {
				s.UnsupportedObserverTokens++
				continue
			}
			if !alive {
				c.Flags = c.Flags&0xf9 | 1
			}
		}
		e := in.Entities[row]
		if e.Present && e.Kind == 0x1c {
			c.Flags = c.Flags&0xf9 | 1
			c.Kind = e.Kind
			c.H0, c.H1 = e.Pose.H0, e.Pose.H1
		}
	}
	for row, e := range in.Entities {
		if e.Present && e.Row != row {
			return fmt.Errorf("contact entity row%d carries inconsistent identity%d", row, e.Row)
		}
	}
	s.scan(in)
	// Nonselected candidates use the body ray, and mast adds a separate head ray.
	// Source head-only success carries observer31; do not call it another pilot.
	for _, candidate := range s.Candidates {
		if candidate.Distance == 0xffff {
			continue
		}
		row := int(candidate.Row)
		if uint16(row+1) == s.Target {
			continue
		}
		e := in.Entities[row]
		if err := s.query(&s.BodyQuery, in.Player, e, in, 0); err != nil {
			return err
		}
		if in.Mast && s.Contacts[row].Flags&2 == 0 {
			if err := s.query(&s.HeadQuery, in.Head, e, in, 31); err != nil {
				return err
			}
		}
	}
	if s.Target == 0 || in.Entities[s.Target-1].Kind == 0x41 || in.Entities[s.Target-1].Kind == 0x1c {
		s.Cursor = 0
		s.Target, s.Cursor = s.selectTarget(0)
		if s.Target == s.Suppressed {
			s.Target = 0
		} else if s.Target != 0 {
			s.Suppressed = 0
			s.MFDTargetRequested = true
		}
	}
	if err := s.revalidateSelected(in, s.validateSelected); err != nil {
		return err
	}
	for i := range s.TailDraws {
		s.TailDraws[i] = in.Random()
	}
	draw := s.TailDraws[4]
	for bit, pair := range [][2]int{{0, 1}, {2, 3}, {0, 3}, {0, 2}, {1, 3}} {
		if draw&(1<<bit) != 0 {
			s.Permutation[pair[0]], s.Permutation[pair[1]] = s.Permutation[pair[1]], s.Permutation[pair[0]]
		}
	}
	if in.ModernTail {
		if err := s.advanceActorTail(in); err != nil {
			return err
		}
	}
	s.Updates++
	return nil
}
func (s *State) query(d *Descriptor, from Pose, e Entity, in Input, observer byte, forwardGate ...uint16) error {
	d.Remaining = Budget(in.Switches)
	d.Pose = from
	metric := Metric(Delta(e.Pose.H0, from.H0), Delta(e.Pose.H1, from.H1))
	step := int16(metric) >> 1
	if int16(metric) >= 0x800 {
		step = 0x400
	}
	for d.Remaining != 0 {
		dx, dz := Delta(e.Pose.H0, d.Pose.H0), Delta(e.Pose.H1, d.Pose.H1)
		distance := Metric(dx, dz)
		d.Angles[1] = world.SourceAngle(-dx, dz, in.Assets)
		if len(forwardGate) > 0 && AbsWord(int16(d.Angles[1]-forwardGate[0])) > 0x2000 {
			d.Status = 0
			d.Remaining = 0
			s.Target = 0
			return nil
		}
		d.Angles[0] = 0
		if int16(AbsWord(from.Height-e.Pose.Height)) >= 64 {
			d.Angles[0] = world.SourceAngle((e.Pose.Height-d.Pose.Height+32)>>4, int16(distance), in.Assets)
		}
		result, err := s.queryBatch(d, e, step, in, observer)
		if err != nil {
			return err
		}
		if result == 1 {
			s.playerSighted[e.Row] = true
		}
	}
	return nil
}

// PlayerSighted reports a successful body or mast observation during flight.
// Administrative GPS refreshes and reports from other observers bypass query.
func (s *State) PlayerSighted(row int) bool {
	return row >= 0 && row < len(s.playerSighted) && s.playerSighted[row]
}

// queryBatch keeps the source status-retention edge at the eighth sample.
func (s *State) queryBatch(d *Descriptor, e Entity, step int16, in Input, observer byte) (uint16, error) {
	for i := 0; i < 8; i++ {
		if d.Remaining == 0 {
			d.Status = 0
			blocked(&s.Contacts[e.Row])
			return 0, nil
		}
		d.Remaining--
		old := d.Pose
		d.Pose.H0 += uint32(int32(world.SourceSinAmplitude(-d.Angles[1], step, in.Assets.Sine)))
		d.Pose.H1 += uint32(int32(world.SourceCosAmplitude(-d.Angles[1], step, in.Assets.Sine)))
		d.Pose.Height += world.SourceSinAmplitude(d.Angles[0], step, in.Assets.Sine)
		distance := Metric(Delta(d.Pose.H0, e.Pose.H0), Delta(d.Pose.H1, e.Pose.H1))
		if distance > 0x1000 {
			d.Remaining, d.Status = 0, 0
			blocked(&s.Contacts[e.Row])
			return 0, nil
		}
		if uint16((step>>3)+8) > distance {
			d.Remaining, d.Status = 0, 1
			observe(&s.Contacts[e.Row], e, observer)
			return 1, nil
		}
		surface, err := in.Assets.PairedTerrainHeight(in.World, world.QueryPose{H0: d.Pose.H0, H1: d.Pose.H1, Height: d.Pose.Height}, world.QueryPose{H0: old.H0, H1: old.H1, Height: old.Height})
		if err != nil {
			return 0, err
		}
		if d.Pose.Height < surface {
			d.Remaining, d.Status = 0, 0
			blocked(&s.Contacts[e.Row])
			return 0, nil
		}
	}
	return 0xffff, nil
}

// GPS entry administratively refreshes active flight rows. This is not a sensor
// result and does not populate every world entity as a detected contact.
func (s *State) GPSRefresh(entities [EntityCapacity]Entity) {
	for row := 1; row <= 4; row++ {
		e := entities[row]
		if e.Present && e.FlightSlot >= 1 && e.Participation != 0 {
			observe(&s.Contacts[row], e, 0)
		}
	}
}

// ReachedContactOrderTail represents source5363..53aa, after the actual
// flight-actor contact/order phase. Player LOS success is NOT that invocation.
// That actor-order producer remains dormant with autonomous orders disabled;
// supplied consumers may exercise the exact predicate without inventing a scan.
func (s *State) ReachedContactOrderTail(kind, objectByte4 byte) {
	if kind == 0x15 && objectByte4&0x30 != 0 {
		s.CompressionLimited = true
	}
}
