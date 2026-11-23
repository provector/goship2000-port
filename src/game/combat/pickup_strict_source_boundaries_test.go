package combat

import (
	"encoding/binary"
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"strings"
	"testing"
)

func objectiveFixture(t *testing.T, role, kind int) (*State, Input) {
	t.Helper()
	s, in := fixture(t, 2)
	m := s.init
	for len(m.Entities) < 80 {
		m.Entities = append(m.Entities, gamestate.InitializedEntity{RuntimeRow: len(m.Entities)})
	}
	m.Rows = make([]gamestate.MissionObject, 80)
	m.Rows[0].Raw[10] = 0x20
	m.Entities[0].Object.Raw[10] = 0x20
	m.Entities[0].ControlRaw[1] = 1
	m.Entities[0].Pose.Height = 0
	m.Stores.Ships[0].Flags = 6
	m.Targets = &gamestate.MissionTargets{Origin: "labelled supplied source-boundary state"}
	r := &m.Targets.Records[role+2]
	binary.LittleEndian.PutUint16(r[32:], uint16(kind))
	binary.LittleEndian.PutUint16(r[50:], 4)
	binary.LittleEndian.PutUint16(r[52:], 10)
	binary.LittleEndian.PutUint32(r[34:], m.Entities[0].Pose.H0)
	binary.LittleEndian.PutUint32(r[38:], m.Entities[0].Pose.H1)
	m.Entities[10].Object.Raw[0] = byte(0x10 << role)
	m.Entities[10].Kind = 0x15
	m.Entities[10].Participation = 1
	in.PlayerIndicatedSpeed = 0
	in.Random = func() uint16 { t.Fatal("objective mechanism consumed RNG"); return 0 }
	return s, in
}
func TestPickupStrictSourceBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name           string
		distance       int
		height, speed  int16
		flags          byte
		active, prefix bool
		want           bool
	}{
		{"127-15-15", 127, 15, 15, 6, true, true, true}, {"distance128", 128, 15, 15, 6, true, true, false}, {"height16", 127, 16, 15, 6, true, true, false}, {"speed16", 127, 15, 16, 6, true, true, false},
		{"negativeSigned", 0, -1, -1, 6, true, true, true}, {"wrongHelicopter", 0, 0, 0, 0, true, true, false}, {"alreadyLoaded", 0, 0, 0, 0x1e, true, true, false}, {"reconEquipment", 0, 0, 0, 0x26, true, true, false},
		{"inactiveControl", 0, 0, 0, 6, false, true, false}, {"outsidePrefix", 0, 0, 0, 6, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, in := objectiveFixture(t, 0, 2)
			m := s.init
			r := &m.Targets.Records[2]
			binary.LittleEndian.PutUint32(r[34:], m.Entities[0].Pose.H0+uint32(tc.distance*16))
			m.Entities[0].Pose.Height = uint16(tc.height)
			in.PlayerIndicatedSpeed = tc.speed
			m.Stores.Ships[0].Flags = tc.flags
			if !tc.active {
				m.Entities[0].ControlRaw[1] = 0
			}
			if !tc.prefix {
				m.Rows[0].Raw[10] = 0
			}
			m.Stores.Ships[0].Capacity = 0 // No numeric capacity predicate in this source leaf.
			s.AdvanceObjectives(in)
			got := binary.LittleEndian.Uint16(r[32:]) == 0
			if got != tc.want {
				t.Fatalf("completion%t expected%t", got, tc.want)
			}
			if got && (m.Stores.Ships[0].Flags != 0x1e || m.ObjectiveAssignments[0] != 1 || m.Entities[10].Kind != 0x41 || m.Entities[10].Participation != 1 || len(events(s, "mass")) != 0) {
				t.Fatal("pickup writer mismatch")
			}
		})
	}
}
func TestPickupMatchingDomainsAndConversionBeforeParticipation(t *testing.T) {
	for role := 0; role < 2; role++ {
		s, in := objectiveFixture(t, role, 2)
		m := s.init
		m.Entities[10].Participation = 2
		s.AdvanceObjectives(in)
		if m.Entities[10].Kind != 0x41 || m.Targets.Records[role+2][32] != 2 || m.Stores.Ships[0].Flags != 6 {
			t.Fatal("P test ordering")
		}
		// The other role's flag must not supply a fabricated pickup or row80 value.
		m.Entities[10].Kind = 0x15
		m.Entities[10].Participation = 1
		m.Entities[10].Object.Raw[0] = byte(0x20 >> role)
		s.AdvanceObjectives(in)
		if m.Targets.Records[role+2][32] != 2 || !strings.Contains(m.ObjectiveLimits[role], "row0..79") {
			t.Fatal("no-match was silently admitted")
		}
	}
}
func TestCargoDropPriorityDeliveryAndRecovery(t *testing.T) {
	for _, tc := range []struct {
		upper byte
		mass  int16
	}{{8, 4000}, {16, 2000}, {24, 4000}, {32, 0}} {
		s, in := objectiveFixture(t, 0, 1)
		m := s.init
		m.Stores.Ships[0].Flags = 6 | tc.upper
		m.Entities[0].Object.Raw[12] = 0xa6
		s.AdvanceObjectives(in)
		if m.Targets.Records[2][32] != 0 || m.Stores.Ships[0].Flags != 6 || !m.CargoComponentRemoved[0] {
			t.Fatal("delivery did not clear/retain source fields")
		}
		mass := int16(0)
		for _, e := range events(s, "mass") {
			mass += e.Value
		}
		if mass != tc.mass {
			t.Fatal("08/10/18 priority", mass, tc.mass)
		}
		before := append([]Event(nil), s.Events...)
		s.AdvanceObjectives(in)
		if !reflect.DeepEqual(before, s.Events) {
			t.Fatal("repeat visit duplicated completion")
		}
	}
	s, in := objectiveFixture(t, 0, 2)
	m := s.init
	s.AdvanceObjectives(in)
	returns := func(row int) (byte, bool) { return 0x80, row == 0 }
	if !m.ObjectiveRecovered(0, returns) || m.ObjectiveRecovered(0, func(int) (byte, bool) { return 0, true }) {
		t.Fatal("pickup versus return")
	}
	if !s.DropPlayerCargo() || m.ObjectiveRecovered(0, returns) {
		t.Fatal("dropped cargo recovered")
	}
	if m.Targets.Records[2][32] != 0 {
		t.Fatal("drop fabricated a new goal")
	}
}
func TestReconExactEquipmentAndNoDwell(t *testing.T) {
	for _, tc := range []struct {
		flags  byte
		metric int
		want   bool
	}{{0x25, 1023, true}, {0x25, 1024, false}, {0x2d, 0, false}, {5, 0, false}} {
		s, in := objectiveFixture(t, 0, 4)
		m := s.init
		m.Stores.Ships[0].Flags = tc.flags
		m.Entities[0].Pose.Height = 30000
		in.PlayerIndicatedSpeed = 30000
		binary.LittleEndian.PutUint32(m.Targets.Records[2][34:], m.Entities[0].Pose.H0+uint32(tc.metric*16))
		s.AdvanceObjectives(in)
		if (m.Targets.Records[2][32] == 0) != tc.want {
			t.Fatal("recon predicate", tc)
		}
	}
}
func TestSafeRetaskCopiesOpaqueTailAndDispatchesSamePhase(t *testing.T) {
	s, in := objectiveFixture(t, 1, 3)
	m := s.init
	// No pre20/current15. Alternate becomes recon, query matches player. Its
	// countdown7 must remain7, and source notification0d precedes completion11.
	m.Entities[10].Kind = 4
	m.Entities[11].Object.Raw[0] = 0x40
	m.Entities[12].Object.Raw[0] = 0x60
	for i := range m.Targets.Records[4] {
		m.Targets.Records[4][i] = byte(i + 11)
	}
	r := &m.Targets.Records[4]
	binary.LittleEndian.PutUint16(r[32:], 4)
	binary.LittleEndian.PutUint16(r[52:], 7)
	binary.LittleEndian.PutUint32(r[34:], m.Entities[0].Pose.H0)
	binary.LittleEndian.PutUint32(r[38:], m.Entities[0].Pose.H1)
	want := *r
	binary.LittleEndian.PutUint16(want[32:], 0)
	binary.LittleEndian.PutUint16(m.Targets.Records[3][52:], 1)
	m.Stores.Ships[0].Flags = 0x26
	m.ObjectiveAssignments[1] = 1
	s.Secondary.Destroyed = 9
	s.AdvanceObjectives(in)
	if m.Targets.Records[3] != want || m.Entities[10].Object.Raw[0] != 0x40 || m.Entities[11].Object.Raw[0] != 0x20 || m.Entities[12].Object.Raw[0] != 0 {
		t.Fatal("62-byte replacement/XOR")
	}
	if m.ObjectiveAssignments[1] != 1 || s.Secondary.Destroyed != 9 {
		t.Fatal("retask invented counter/owner reset")
	}
	notices := events(s, "notification")
	if len(notices) != 2 || notices[0].Notification != 13 || notices[1].Notification != 17 {
		t.Fatal("same-phase dispatch order", notices)
	}
}
func TestRetaskZeroSuppressionAndUnsafeDomain(t *testing.T) {
	for _, mode := range []string{"zero", "suppressed", "unsafe"} {
		s, in := objectiveFixture(t, 1, 3)
		m := s.init
		r := &m.Targets.Records[3]
		binary.LittleEndian.PutUint16(r[52:], 1)
		if mode == "zero" {
			binary.LittleEndian.PutUint16(r[52:], 0)
		}
		if mode == "suppressed" {
			m.Entities[11].Object.Raw[0] = 0x40
			m.Entities[11].Kind = 0x1c
		}
		before := m.Entities[10].Object.Raw[0]
		s.AdvanceObjectives(in)
		if mode == "zero" && binary.LittleEndian.Uint16(r[52:]) != 0xffff {
			t.Fatal("zero did not wrap")
		}
		if mode == "unsafe" && (!strings.Contains(m.Targets.ProgressFailure, "BP-4") || m.Entities[10].Object.Raw[0] != before) {
			t.Fatal("unsafe local guessed or partial swap")
		}
		if mode == "suppressed" && (m.Targets.ProgressFailure != "" || r[32] != 3) {
			t.Fatal("suppression lost current goal")
		}
	}
}

func TestDeliveryNoMatchAndRetainedCountReplacement(t *testing.T) {
	for role := 0; role < 2; role++ {
		s, in := objectiveFixture(t, role, 1)
		m := s.init
		m.Stores.Ships[0].Flags = 0x1e
		m.Entities[10].Kind = 4
		in.PlayerIndicatedSpeed = 200
		s.AdvanceObjectives(in)
		if m.Targets.Records[role+2][32] != 0 || m.ObjectiveLimits[role] != "" {
			t.Fatal("delivery invented pickup-only match/speed requirements")
		}
	}
	s, in := objectiveFixture(t, 1, 3)
	m := s.init
	m.Entities[10].Kind = 4
	s.Secondary.Destroyed = 5
	binary.LittleEndian.PutUint16(m.Targets.Records[3][52:], 1)
	r := &m.Targets.Records[4]
	binary.LittleEndian.PutUint16(r[32:], 3)
	binary.LittleEndian.PutUint16(r[50:], 4)
	binary.LittleEndian.PutUint16(r[52:], 19)
	s.AdvanceObjectives(in)
	if !s.Secondary.Achieved || m.Targets.Records[3][32] != 0 || binary.LittleEndian.Uint16(m.Targets.Records[3][52:]) != 19 || s.Secondary.Destroyed != 5 {
		t.Fatal("retained count/new timer same-phase mismatch")
	}
}
func TestFinalRecoveryActorAndEndingBoundaries(t *testing.T) {
	s, in := objectiveFixture(t, 0, 2)
	m := s.init
	s.AdvanceObjectives(in)
	m.Sortie = &gamestate.SortieState{Result: &gamestate.TransientSortieResult{Reason: "Alt+E", SourceStatus: 0x80}}
	m.FinalizePlayerObjectives()
	if m.Sortie.Result.SourceStatus&0x81 != 0 {
		t.Fatal("voluntary pickup counted safe recovery")
	}
	m.Sortie.Result = &gamestate.TransientSortieResult{Reason: "base", SourceStatus: 0x80}
	m.FinalizePlayerObjectives()
	if m.Sortie.Result.SourceStatus&0x81 != 0x81 {
		t.Fatal("assigned loaded player safe return not recovered")
	}
	m.ObjectiveAssignments[0] = 2
	if m.ObjectiveRecovered(0, func(actor int) (byte, bool) { return 0x80, actor == 0 }) {
		t.Fatal("wrong actor returned")
	}
	m.ObjectiveAssignments[0] = 1
	s.DropPlayerCargo()
	m.Sortie.Result.SourceStatus = 0x80
	m.FinalizePlayerObjectives()
	if m.Sortie.Result.SourceStatus&1 != 0 {
		t.Fatal("drop before return still recovered")
	}
}
