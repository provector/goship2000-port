package combat

import (
	"encoding/binary"
	"fmt"
	gamestate "gunship2000-assets/game/state"
)

// P4's source5d0c visits the actual flight actors in ascending order. This is
// after reached movement/service, never a UI/camera or host-time consumer.
func (s *State) advanceFlightObjectives(in Input) {
	trace := primaryScanTrace{}
	s.dispatchFlightObjective(0, in, &trace)
	if s.advanceSecondary(in, trace) {
		s.dispatchFlightObjective(1, in, &trace)
	}
	s.advanceFlightReservePickup(in)
}

func (s *State) dispatchFlightObjective(role int, in Input, trace *primaryScanTrace) {
	m := s.init
	r := &m.Targets.Records[role+2]
	o := s.objective(role)
	o.DisplayKind = int(binary.LittleEndian.Uint16(r[32:]))
	o.Required = int16(binary.LittleEndian.Uint16(r[50:]))
	if o.DisplayKind == 0 {
		return
	}
	if o.DisplayKind == 3 {
		if o.Destroyed >= o.Required {
			s.completeFlightObjective(role, m.PlayerRow, in, false)
		}
		return
	}
	if m.Stores == nil {
		return
	}
	for actor := 0; actor < 5; actor++ {
		// Secondary delivery alone omits the prefix test in source64BC. Its
		// actual SHIP/CONTROL rows still must exist; no fabricated padding rows.
		if actor >= len(m.Entities) || actor >= len(m.Stores.Ships) {
			break
		}
		if !(role == 1 && o.DisplayKind == 1) && (actor >= len(m.Rows) || m.Rows[actor].Raw[10]&0x20 == 0) {
			break
		}
		e := &m.Entities[actor]
		ship := &m.Stores.Ships[actor]
		if e.ControlRaw[1] == 0 {
			continue
		}
		metric := gamestate.ObjectiveDistance(*r, e.Pose)
		if o.DisplayKind == 4 {
			if ship.Flags&0xf8 == 0x20 && metric < 1024 {
				s.completeFlightObjective(role, actor, in, true)
				return
			}
			continue
		}
		if o.DisplayKind != 1 && o.DisplayKind != 2 {
			return
		}
		if ship.Flags&7 != 6 || metric >= 128 {
			continue
		}
		if o.DisplayKind == 1 && ship.Flags&0xf8 == 0 || o.DisplayKind == 2 && ship.Flags&0xf8 != 0 {
			continue
		}
		if int16(e.Pose.Height) >= 16 {
			if s.orderLogisticsLanding(actor, role) {
				return
			}
			continue
		}
		if o.DisplayKind == 1 {
			if row := s.matchCargoObject(role, trace); row >= 0 {
				s.convertCargoObject(row)
			}
			s.dropCargo(actor)
			s.completeFlightObjective(role, actor, in, true)
			s.cleanupLogistics(actor)
			return
		}
		speed := e.RawMotion
		if actor == m.PlayerRow {
			speed = in.PlayerIndicatedSpeed
		}
		if speed >= 16 {
			continue
		}
		row := s.matchCargoObject(role, trace)
		if row < 0 {
			s.limitObjective(role, "pickup has no matching OBJECT row0..79; original row80 read is unsupported")
			continue
		}
		participation := m.Entities[row].Participation
		s.convertCargoObject(row)
		if participation != 1 {
			continue
		}
		ship.Flags = ship.Flags&7 | 0x18
		m.ObjectiveAssignments[role] = uint16(actor + 1)
		if actor != m.PlayerRow && actor < len(m.ActorFlags) {
			m.ActorFlags[actor] |= 1 << uint(role)
		}
		s.completeFlightObjective(role, actor, in, true)
		s.cleanupLogistics(actor)
		return
	}
}

// The nearby high-height arm changes the controller; it does not move the
// aircraft or perform descent in this visit. Kind26 owns the next visit.
func (s *State) orderLogisticsLanding(actor, role int) bool {
	e := &s.init.Entities[actor]
	if e.Participation != 1 {
		return false
	}
	eligible := e.Kind == 0x20
	if e.Kind == 0x21 {
		leader := int(int8(e.ControlRaw[5]))
		if leader < 0 || leader >= len(s.init.Entities) {
			s.limitObjective(role, fmt.Sprintf("nearby logistics actor%d has unsupported parent%d", actor, leader))
			return false
		}
		eligible = s.init.Entities[leader].Kind == 0x20
	}
	if !eligible {
		return false
	}
	e.ControlRaw[1] = 7
	e.Kind = 0x26
	s.syncDeath(e)
	return true
}

func (s *State) limitObjective(role int, reason string) {
	if s.init.ObjectiveLimits[role] == "" {
		s.init.ObjectiveLimits[role] = reason
	}
	if s.init.Targets.ProgressFailure == "" {
		s.init.Targets.ProgressFailure = s.init.ObjectiveLimits[role]
	}
}

func (s *State) cleanupLogistics(first int) {
	m := s.init
	for actor := first; actor < 5 && actor < len(m.Entities); actor++ {
		if actor >= len(m.Rows) || m.Rows[actor].Raw[10]&0x20 == 0 {
			break
		}
		e := &m.Entities[actor]
		if e.ControlRaw[1] == 7 {
			e.ControlRaw[1] = 4
			e.Kind = 0x24
			s.syncDeath(e)
		}
	}
}

func (s *State) completeFlightObjective(role, actor int, in Input, notice bool) {
	r := &s.init.Targets.Records[role+2]
	if s.init.SafeGameplay {
		s.init.Targets.CompleteObjectiveRevision(role)
	}
	binary.LittleEndian.PutUint16(r[32:], 0)
	o := s.objective(role)
	o.DisplayKind, o.Achieved = 0, true
	if in.SpeechEnabled {
		s.Events = append(s.Events, Event{Kind: "sound", Slot: -1, Actor: s.init.PlayerRow, Command: uint16(0x3e + role), Source: "5d0c:objective completion"})
	}
	s.appendResult(Result{Kind: byte(4 + role), Timestamp: in.Phase})
	s.emit("objective-complete", -1, 0, 0, int16(4+role), "5d0c:objective completion")
	if notice {
		s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: uint16(0x10 + role), Source: "5d0c:actual actor cargo/recon"})
	}
}

func (s *State) advanceFlightReservePickup(in Input) {
	m := s.init
	if m.PlayerRow != 0 || len(m.FlightRows) != 5 || m.Stores == nil || len(m.Stores.Ships) != 5 || len(m.Entities) < 9 {
		return
	}
	for actor := 0; actor < 5; actor++ {
		if actor >= len(m.Rows) || m.Rows[actor].Raw[10]&0x20 == 0 {
			break
		}
		carrier := &m.Entities[actor]
		if actor != m.PlayerRow && carrier.ControlRaw[1] == 0 || m.Stores.Ships[actor].Flags&7 != 6 {
			continue
		}
		for a := 1; a < 5; a++ {
			if a >= len(m.Rows) || m.Rows[a].Raw[10]&0x20 == 0 {
				break
			}
			row := a + 4
			e := &m.Entities[row]
			if e.Kind != 0x1b || e.Participation != 1 || s.reserveIdentity(row) != nil || gamestate.FinalResultMetric(e.Pose, carrier.Pose) >= 128 {
				continue
			}
			if int16(carrier.Pose.Height) >= 16 {
				s.orderLogisticsLanding(actor, 0)
				break
			}
			speed := carrier.RawMotion
			if actor == m.PlayerRow {
				speed = in.PlayerIndicatedSpeed
			}
			if speed >= 16 {
				break
			}
			e.Kind, e.DynamicRaw[0], e.DynamicRaw[3] = 0x41, 0x41, byte(actor)
			s.syncDeath(e)
			s.emit("objective-contact-kind", -1, actor, row, 0x41, "6b53:reserve pickup")
			s.appendResult(Result{Kind: 6, Actor: byte(actor), Target: byte(a), Timestamp: in.Phase})
			if actor == m.PlayerRow {
				s.Events = append(s.Events, Event{Kind: "notification", Actor: a, Notification: 21, Source: "6b99:player recovered crew"})
			} else {
				s.Events = append(s.Events, Event{Kind: "notification", Actor: actor, Notification: 19, Source: "6b99:follower recovered crew"})
			}
			if carrier.Participation == 1 {
				s.cleanupLogistics(actor)
			}
			// This source arm advances the outer actor, not another survivor.
			break
		}
	}
}
