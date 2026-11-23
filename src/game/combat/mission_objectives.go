package combat

import (
	"encoding/binary"
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// advanceMissionObjectives is section1:5d0c's admitted player mechanism. NPC
// orders/decisions and follower performance are separate producers. TARGET is
// shared authoritative state, including its opaque62-byte records.
func (s *State) advanceMissionObjectives(in Input) {
	if s.logisticsEnabled {
		s.advanceFlightObjectives(in)
		return
	}
	trace := primaryScanTrace{}
	primaryBefore := s.init.Targets.Records[2]
	s.dispatchPlayerObjective(0, in, &trace)
	trace.unsupportedActor = s.unimplementedPrimaryScanner(primaryBefore)
	if s.advanceSecondary(in, trace) {
		s.dispatchPlayerObjective(1, in)
	}
	s.advancePlayerReservePickup(in)
}

func (s *State) objective(role int) *Objective {
	if role == 0 {
		return &s.Primary
	}
	return &s.Secondary
}

// primaryScanTrace is the last actual BP-4 writer in this full invocation.
// It is never retained across logical updates or reconstructed from a later row.
type primaryScanTrace struct {
	executed         bool
	finalIndex       int
	unsupportedActor bool
}

func (s *State) dispatchPlayerObjective(role int, in Input, traces ...*primaryScanTrace) {
	var trace *primaryScanTrace
	if role == 0 && len(traces) != 0 {
		trace = traces[0]
	}
	m := s.init
	t := m.Targets
	r := &t.Records[role+2]
	o := s.objective(role)
	o.DisplayKind = int(binary.LittleEndian.Uint16(r[32:]))
	o.Required = int16(binary.LittleEndian.Uint16(r[50:]))
	if o.DisplayKind == 0 {
		return
	}
	if o.DisplayKind == 3 {
		if o.Destroyed >= o.Required {
			s.completeMissionObjective(role, in, false)
		}
		return
	}
	actor := m.PlayerRow
	if actor < 0 || actor >= 5 || actor >= len(m.Entities) || m.Stores == nil || actor >= len(m.Stores.Ships) {
		return
	}
	// OBJECT b56-b4c is byte10 DECIMAL, not+10h. Only the established
	// consecutive flight prefix is admitted, with the real Control489 guard.
	for row := 0; row <= actor; row++ {
		if row >= len(m.Rows) || m.Rows[row].Raw[10]&0x20 == 0 {
			return
		}
	}
	e := &m.Entities[actor]
	if e.ControlRaw[1] == 0 {
		return
	}
	ship := &m.Stores.Ships[actor]
	metric := gamestate.ObjectiveDistance(*r, e.Pose)
	if o.DisplayKind == 4 {
		if ship.Flags&0xf8 == 0x20 && metric < 1024 {
			s.completeMissionObjective(role, in, true)
		}
		return // Recon has no height, speed, contact or dwell predicate here.
	}
	if ship.Flags&7 != 6 || metric >= 128 || int16(e.Pose.Height) >= 16 {
		return
	}
	switch o.DisplayKind {
	case 1:
		if ship.Flags&0xf8 == 0 {
			return
		}
		// Source delivery converts the first role-matching dynamic15 when it
		// exists; absence does not access row80 or prevent delivery.
		if row := s.matchCargoObject(role, trace); row >= 0 {
			s.convertCargoObject(row)
		}
		s.dropCargo(actor)
		s.completeMissionObjective(role, in, true)
	case 2:
		// 0476 is the CURRENT post-player-update indicated word, not E2 or abs(speed).
		if ship.Flags&0xf8 != 0 || in.PlayerIndicatedSpeed >= 16 {
			return
		}
		row := s.matchCargoObject(role, trace)
		if row < 0 {
			m.ObjectiveLimits[role] = "pickup has no matching OBJECT row0..79; original row80 read is unsupported"
			return
		}
		participation := m.Entities[row].Participation
		s.convertCargoObject(row) // Deliberately precedes the original P==1 test.
		if participation != 1 {
			return
		}
		ship.Flags = ship.Flags&7 | 0x18
		m.ObjectiveAssignments[role] = uint16(actor + 1)
		// No immediate MassTerm addition exists in this source pickup arm.
		s.completeMissionObjective(role, in, true)
	}
}
func (s *State) matchCargoObject(role int, traces ...*primaryScanTrace) int {
	var trace *primaryScanTrace
	if role == 0 && len(traces) != 0 {
		trace = traces[0]
	}
	if trace != nil {
		trace.executed, trace.finalIndex = true, 80
	}
	flag := byte(0x10 << role) // Primary10h and secondary20h are distinct operands.
	for row := 0; row < len(s.init.Entities) && row < 80; row++ {
		e := &s.init.Entities[row]
		if e.Object.Raw[0]&flag != 0 && e.Kind == 0x15 {
			if trace != nil {
				trace.finalIndex = row
			}
			return row
		}
	}
	return -1
}
func (s *State) convertCargoObject(row int) {
	e := &s.init.Entities[row]
	e.Kind = 0x41
	e.DynamicRaw[0] = 0x41
	// CONTACTclass changes in this same phase; selection/LOS rescans retain
	// their existing later ownership. Participation is intentionally unchanged.
	s.emit("objective-contact-kind", -1, s.init.PlayerRow, row, 0x41, "5d0c:611d/688b")
}
func (s *State) dropCargo(actor int) {
	ship := &s.init.Stores.Ships[actor]
	mass := int16(0)
	if ship.Flags&8 != 0 {
		mass = 4000
	} else if ship.Flags&16 != 0 {
		mass = 2000
	}
	if actor == s.init.PlayerRow && mass != 0 {
		s.emit("mass", -1, actor, 0, mass, "5d0c:5ed0/5ee9;GPS119c")
	}
	if actor < len(s.init.Entities) && s.init.Entities[actor].Object.Raw[12] == 0xa6 {
		s.init.CargoComponentRemoved[actor] = true
	}
	ship.Flags &= 7
}

// DropPlayerCargo is only GPS C's selected-player transaction. It does not
// complete a goal, create a result, or conflate ordinary flight C's low-limit.
func (s *State) DropPlayerCargo() bool { return s.DropCargo(s.init.PlayerRow) }

func (s *State) DropCargo(actor int) bool {
	if actor < 0 || actor >= 5 || s.init.Stores == nil || actor >= len(s.init.Stores.Ships) || s.init.Stores.Ships[actor].Flags&7 != 6 {
		return false
	}
	s.dropCargo(actor)
	return true
}
func (s *State) completeMissionObjective(role int, in Input, notice bool) {
	r := &s.init.Targets.Records[role+2]
	if s.init.SafeGameplay {
		s.init.Targets.CompleteObjectiveRevision(role)
	}
	binary.LittleEndian.PutUint16(r[32:], 0)
	o := s.objective(role)
	o.DisplayKind, o.Achieved = 0, true
	s.init.ObjectiveLimits[role] = ""
	if in.SpeechEnabled {
		s.Events = append(s.Events, Event{Kind: "sound", Slot: -1, Actor: s.init.PlayerRow, Command: uint16(0x3e + role), Source: "5d0c:objective completion"})
	}
	s.appendResult(Result{Kind: byte(4 + role), Timestamp: in.Phase})
	s.emit("objective-complete", -1, 0, 0, int16(4+role), "5d0c:objective completion")
	if notice {
		s.Events = append(s.Events, Event{Kind: "notification", Actor: s.init.PlayerRow, Notification: uint16(0x10 + role), Source: "5d0c:player cargo/recon"})
	}
}
func (s *State) advanceSecondary(in Input, traces ...primaryScanTrace) bool {
	trace := primaryScanTrace{}
	if len(traces) != 0 {
		trace = traces[0]
	}
	m := s.init
	t := m.Targets
	if t.ProgressFailure != "" || binary.LittleEndian.Uint16(t.Records[3][32:]) == 0 {
		return false
	}
	r := &t.Records[3]
	n := binary.LittleEndian.Uint16(r[52:]) - 1
	binary.LittleEndian.PutUint16(r[52:], n)
	if n != 0 {
		return true
	}
	for row, e := range m.Entities {
		if m.SafeGameplay && row >= 80 {
			break
		}
		if e.Object.Raw[0]&0x40 != 0 && e.Kind == 0x1c {
			return true
		}
	}
	var oldFlags [80]byte
	var retire [80]bool
	if m.SafeGameplay {
		// R5 repairs645B's wrong primary-local index. Snapshot eligibility
		// before flag exchange and retire each actual old-secondary row.
		for row := 0; row < len(m.Entities) && row < 80; row++ {
			e := &m.Entities[row]
			oldFlags[row] = e.Object.Raw[0]
			retire[row] = oldFlags[row]&0x20 != 0 && e.Kind == 0x15
		}
	} else {
		// Historical rules validate the whole retained BP-4 domain before
		// publishing a flag swap, including the in-bounds wrong-row write.
		for _, e := range m.Entities {
			if e.Object.Raw[0]&0x20 != 0 && e.Kind == 0x15 {
				if trace.unsupportedActor || !trace.executed || trace.finalIndex < 0 || trace.finalIndex >= 80 || trace.finalIndex >= len(m.Entities) {
					t.ProgressFailure = fmt.Sprintf("secondary conversion requires defined same-invocation BP-4 row0..79 (OBJECT row%d)", e.RuntimeRow)
					return false
				}
			}
		}
	}
	for row := 0; row < len(m.Entities) && row < 80; row++ {
		e := &m.Entities[row]
		if m.SafeGameplay && retire[row] {
			s.convertCargoObject(row)
		} else if !m.SafeGameplay && e.Object.Raw[0]&0x20 != 0 && e.Kind == 0x15 {
			s.convertCargoObject(trace.finalIndex) // Source645b/6468 writes BP-4, not this loop row.
		}
		if m.SafeGameplay {
			e.Object.Raw[0] = oldFlags[row]
		}
		if e.Object.Raw[0]&0x60 != 0 {
			e.Object.Raw[0] ^= 0x60
			if row < len(m.Rows) {
				m.Rows[row].Raw[0] = e.Object.Raw[0]
			}
		}
	}
	t.Records[3] = t.Records[4] // All31 words, including opaque tail/countdown.
	if m.SafeGameplay {
		t.PromoteSecondaryRevision()
	}
	s.Secondary.DisplayKind = int(binary.LittleEndian.Uint16(t.Records[3][32:]))
	s.Secondary.Required = int16(binary.LittleEndian.Uint16(t.Records[3][50:]))
	s.Secondary.Achieved = false
	s.Events = append(s.Events, Event{Kind: "notification", Actor: 0, Notification: 0x0d, Source: "5d0c:6491..649b"})
	return true // Immediate dispatch, with NO second decrement of the new timer.
}

// unimplementedPrimaryScanner conservatively excludes only a reached actor
// that could overwrite this frame's local through an unimplemented cargo arm.
// Player-only cargo is supported; follower order/assignment producers are not
// replaced by a stale remembered player scan when their physical gates pass.
func (s *State) unimplementedPrimaryScanner(record [62]byte) bool {
	kind := binary.LittleEndian.Uint16(record[32:])
	if (kind != 1 && kind != 2) || s.init.Stores == nil {
		return false
	}
	m := s.init
	for a := 1; a < 5 && a < len(m.Entities) && a < len(m.Stores.Ships); a++ {
		if a >= len(m.Rows) || m.Rows[a].Raw[10]&0x20 == 0 {
			break
		}
		e, ship := m.Entities[a], m.Stores.Ships[a]
		if e.ControlRaw[1] == 0 || ship.Flags&7 != 6 || int16(e.Pose.Height) >= 16 || gamestate.ObjectiveDistance(record, e.Pose) >= 128 {
			continue
		}
		if kind == 1 && ship.Flags&0xf8 != 0 {
			return true
		}
		if kind == 2 && ship.Flags&0xf8 == 0 && e.RawMotion < 16 {
			return true
		}
	}
	return false
}
