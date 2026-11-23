package state

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/flight"
)

// SortieState is transient. It owns source result bits and natural prompt state,
// never scoring, pilot records, Campaign advancement or recovery snapshots.
type SortieState struct {
	Status     byte
	Pending    string // base / corridor; distinct from the Alt+E QoL request
	Result     *TransientSortieResult
	Services   uint64
	FARPStatus string
	FARPLosses int16 // DATA272d:AF1 starts0; future specific destruction writer9dc4 owns increments.
}

type TransientSortieResult struct {
	Reason               string
	SourceStatus         byte
	Player               flight.Pose
	SourceCrashCountdown int16
	PlayerDamage         [12]byte // Frozen source5fe..609, separate from career loss interpretation.
	EncodedHealth        uint16
	DamageKnown          bool
}

// SourceDistance uses SAR32/4 followed by low-word ABS and wrapped word sum.
// It is neither world Manhattan distance nor a distance expressed in metres.
func SourceServiceDistance(anchor MissionPoint, p flight.Pose) uint16 {
	abs := func(v int16) uint16 {
		if v < 0 {
			v = -v
		}
		return uint16(v)
	}
	return abs(int16((anchor.X>>4)-(int32(p.H0)>>4))) + abs(int16((anchor.Y>>4)-(int32(p.H1)>>4)))
}

func SourceCorridorEligible(base MissionPoint, p flight.Pose, crossed byte) bool {
	return sourceCorridorEligible(base, p, crossed, p.H0)
}

func sourceCorridorEligible(base MissionPoint, p flight.Pose, crossed byte, firstH0 uint32) bool {
	abs := func(v int16) uint16 {
		if v < 0 {
			v = -v
		}
		return uint16(v)
	}
	for _, arm := range []struct {
		bit  byte
		axis int
		edge int16
	}{{1, 0, 0x400}, {2, 0, 0x7bff}, {4, 1, 0x400}, {8, 1, 0x7bff}} {
		if crossed&arm.bit == 0 {
			continue
		}
		b0, b1 := base.X>>4, base.Y>>4
		p0, p1 := int32(p.H0)>>4, int32(p.H1)>>4
		var metric uint16
		if arm.axis == 0 {
			metric = abs(int16(b0)-arm.edge) + abs(int16(b1-p1))
		} else {
			// H1 executes first in3bf9..3cf3, before H0's3d0a/3d69 clamp.
			p0 = int32(firstH0) >> 4
			metric = abs(int16(b1)-arm.edge) + abs(int16(b0-p0))
		}
		if metric < 4096 {
			return true
		}
	}
	return false
}

// AdvanceService is called after the accepted contact/boundary calculation.
// Ordinary native launches correspond to source's fresh raw3104==0 path; DOS
// /D and replay modes are outside this adapter. FARP eligibility consumes its
// retained TARGET record and specific AF1 owner; it is not a generic wreck count.
func (m *InitializedMission) AdvanceService(mission *MissionData, systems *flight.Systems) (bool, error) {
	return m.AdvanceServiceWithFuelLoss(mission, systems, false)
}

func (m *InitializedMission) AdvanceServiceWithFuelLoss(mission *MissionData, systems *flight.Systems, fuelSupplyFailed bool) (bool, error) {
	if m == nil || m.PlayerFlight == nil || mission == nil || systems == nil || m.Stores == nil {
		return false, fmt.Errorf("sortie service needs initialized flight/stores")
	}
	if m.PlayerRow < 0 || m.PlayerRow >= len(m.Stores.Ships) {
		return false, fmt.Errorf("service player runtime row outside accepted flight")
	}
	if len(m.Stores.Ships) != len(m.Stores.Blueprint) || len(m.Stores.Damage) != len(m.Stores.Ships) {
		return false, fmt.Errorf("service needs complete runtime blueprint/damage rows")
	}
	if m.Sortie == nil {
		m.Sortie = &SortieState{}
	}
	state := m.Sortie
	s := m.PlayerFlight
	if state.Result != nil || state.Pending != "" {
		return false, nil
	}
	if s.Terminal != nil {
		state.Status = s.Terminal.Status
		return false, nil
	}
	// Source39ff..3a1b tests609, not clears it. Accepted fuel-out contact
	// transfers before the rotor/cooldown/base guards, with no invented result
	// bit. The source refill helper's609 clear is a distinct operation.
	if s.ValidContact && (systems.Damage[9] != 0 || fuelSupplyFailed) {
		reason := "disabled-contact"
		if m.PlayerRow < len(systems.Fuel) && systems.Fuel[m.PlayerRow] <= 0 {
			reason = "fuel-out-contact"
		}
		//609 can also be written by rotor/engine/general-system damage. The
		// source gate is shared; the native reason must not invent fuel exhaustion.
		state.Result = &TransientSortieResult{Reason: reason, SourceStatus: state.Status, Player: s.Pose}
		return false, nil
	}
	base, ok := mission.Objectives["base"]
	if !ok && m.Targets == nil {
		return false, fmt.Errorf("sortie service lacks original base/start anchor")
	}
	// TARGET+42/+46 is the service/reference anchor. Cargo/recon uses a
	// different +34/+38 query position; frontend labels are not the carrier.
	basePoint := base.Point
	if m.Targets != nil {
		basePoint = targetServicePoint(m.Targets.Records[0])
	}
	state.FARPStatus = "unavailable: live TARGET carrier missing"
	var farp [62]byte
	farpAvailable := false
	if m.Targets != nil {
		farp = m.Targets.Records[1]
		state.FARPStatus = "not present"
		if binary.LittleEndian.Uint16(farp[32:]) != 0 {
			state.FARPStatus = "limit reached"
			if state.FARPLosses < int16(binary.LittleEndian.Uint16(farp[50:])) {
				state.FARPStatus = "available"
				farpAvailable = true
			}
		}
	}
	serviced := false
	if s.ValidContact && systems.Damage[9] == 0 && s.Rotor == 0 && s.FlightStage == 0 {
		if SourceServiceDistance(basePoint, s.Pose) < 512 {
			if err := m.refillServiceFlight(systems); err != nil {
				return false, err
			}
			state.Status = 0x80
			state.Pending = "base"
			state.Services++
			serviced = true
		} else if farpAvailable && SourceServiceDistance(targetServicePoint(farp), s.Pose) < 512 {
			// Source3b72..3b84 shares refill/mass recomputation but writes no
			// return80, prompt, new cooldown or per-visit allowance decrement.
			if err := m.refillServiceFlight(systems); err != nil {
				return false, err
			}
			state.FARPStatus = "serviced"
			state.Services++
			serviced = true
		}
	}
	if m.Switches&0x800 != 0 && s.FlightStage == 0 && sourceCorridorEligible(basePoint, s.Pose, s.BoundaryCrossed, s.BoundaryH0BeforeClamp) {
		state.Status = 0x80
		state.Pending = "corridor"
	}
	return serviced, nil
}

func targetServicePoint(record [62]byte) MissionPoint {
	return MissionPoint{X: int32(binary.LittleEndian.Uint32(record[42:])), Y: int32(binary.LittleEndian.Uint32(record[46:]))}
}

// Source5b6c is shared by base and FARP. The accepted blueprint is a native
// immutable copy; current upper cargo flags and physical damage have other owners.
func (m *InitializedMission) refillServiceFlight(systems *flight.Systems) error {
	return m.RefillServiceActor(m.PlayerRow, systems)
}

// RefillServiceActor is source5b6c with its actual caller actor, not a generic repair.
func (m *InitializedMission) RefillServiceActor(actor int, systems *flight.Systems) error {
	if m == nil || m.Stores == nil || systems == nil || actor < 0 || actor >= len(m.Stores.Ships) {
		return fmt.Errorf("refill source actor missing")
	}
	// Source5b6c walks OBJECT+0Ah bit20h flight rows. The primary is
	// explicit; followers need489==1 and the source leader relationship.
	for slot := 0; slot < len(m.Stores.Ships); slot++ {
		if slot >= len(m.Rows) || m.Rows[slot].Raw[10]&0x20 == 0 {
			break
		}
		if slot != actor {
			if slot >= len(m.Entities) || m.Entities[slot].ControlRaw[1] != 1 {
				continue
			}
			leader := int(int8(m.Entities[slot].ControlRaw[5]))
			if leader != actor && (leader < 0 || leader >= len(m.Entities) || m.Entities[leader].ControlRaw[1] != 1) {
				continue
			}
		}
		damage := m.Stores.Damage[slot]
		if slot == m.PlayerRow {
			damage = systems.Damage
		}
		if err := m.Stores.Refill(slot, &damage); err != nil {
			return err
		}
		if slot == m.PlayerRow {
			systems.Damage = damage
		}
		m.Stores.Damage[slot] = damage
		if slot < len(systems.Fuel) {
			systems.Fuel[slot] = m.Stores.Ships[slot].Fuel
		}
	}
	return nil
}

// AnswerNaturalEnd preserves N's original cooldown reset. Escape may be mapped
// to N by the native modal as an explicit QoL policy, not an original key claim.
func (m *InitializedMission) AnswerNaturalEnd(yes bool) bool {
	if m == nil || m.Sortie == nil || m.Sortie.Pending == "" || m.PlayerFlight == nil {
		return false
	}
	r := m.Sortie
	s := m.PlayerFlight
	if yes {
		r.Result = &TransientSortieResult{Reason: r.Pending, SourceStatus: r.Status, Player: s.Pose}
		r.Pending = ""
		return true
	}
	r.Status &^= 0x80
	r.Pending = ""
	s.FlightStage = 128
	return false
}

func (m *InitializedMission) EndSortieForQoL() {
	if m == nil || m.PlayerFlight == nil {
		return
	}
	if m.Sortie == nil {
		m.Sortie = &SortieState{}
	}
	s := m.PlayerFlight
	r := &TransientSortieResult{Reason: "Alt+E", SourceStatus: m.Sortie.Status, Player: s.Pose}
	if s.Terminal != nil {
		r.SourceStatus = s.Terminal.Status
		r.SourceCrashCountdown = s.Terminal.Countdown
	}
	m.Sortie.Result = r
}

// CompleteTerminal records only the transient original status/counter. The
// native owner returns to briefing until real damage/debriefing successors exist.
func (m *InitializedMission) CompleteTerminal() bool {
	if m == nil || m.PlayerFlight == nil || m.PlayerFlight.Terminal == nil {
		return false
	}
	s := m.PlayerFlight
	t := s.Terminal
	if t.Countdown != 0 || t.Status == 0 {
		return false
	}
	if m.Sortie == nil {
		m.Sortie = &SortieState{}
	}
	m.Sortie.Status = t.Status
	m.Sortie.Result = &TransientSortieResult{Reason: t.Kind, SourceStatus: t.Status, Player: s.Pose, SourceCrashCountdown: t.Countdown}
	return true
}
