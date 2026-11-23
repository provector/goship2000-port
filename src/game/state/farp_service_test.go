package state

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"gunship2000-assets/game/flight"
	"reflect"
	"testing"
)

func fARPFixture() (*InitializedMission, *MissionData, *flight.Systems) {
	m, prepared, systems := sortieFixture()
	m.Targets = &MissionTargets{Origin: "supplied original-layout service boundary"}
	// Keep frontend and TARGET query positions at the player. Only TARGET
	// reference positions below are meaningful to source3a44/3ae9 service.
	prepared.Objectives["base"] = MissionObjective{Point: MissionPoint{int32(m.PlayerFlight.Pose.H0), int32(m.PlayerFlight.Pose.H1)}}
	for n := range m.Targets.Records {
		r := &m.Targets.Records[n]
		binary.LittleEndian.PutUint32(r[34:], m.PlayerFlight.Pose.H0)
		binary.LittleEndian.PutUint32(r[38:], m.PlayerFlight.Pose.H1)
	}
	binary.LittleEndian.PutUint32(m.Targets.Records[0][42:], 0x60000)
	binary.LittleEndian.PutUint32(m.Targets.Records[0][46:], 0x60000)
	f := &m.Targets.Records[1]
	binary.LittleEndian.PutUint16(f[32:], 1)
	binary.LittleEndian.PutUint32(f[42:], m.PlayerFlight.Pose.H0)
	binary.LittleEndian.PutUint32(f[46:], m.PlayerFlight.Pose.H1)
	binary.LittleEndian.PutUint16(f[50:], 2)
	m.Stores.Ships[0].Flags = 0x1e // selector6 with retained post-pickup18.
	m.Stores.Ships[0].Stations[0].Ammunition = 0
	systems.Damage[1] = 2
	systems.Damage[2] = 1
	return m, prepared, systems
}

func TestFARPRefillReferenceCargoDamageAndNoReturn(t *testing.T) {
	m, prepared, systems := fARPFixture()
	before, _ := json.Marshal(prepared)
	blueprint := append([]RuntimeAircraft(nil), m.Stores.Blueprint...)
	serviced, err := m.AdvanceService(prepared, systems)
	if err != nil || !serviced {
		t.Fatal("eligible FARP did not refill", err)
	}
	ship := m.Stores.Ships[0]
	if ship.Flags != 0x1b || ship.Fuel != 80 || systems.Fuel[0] != 80 || ship.Stations[0].Ammunition != 5 {
		t.Fatal("wrong accepted blueprint/damaged-wing refill", ship, systems.Fuel)
	}
	if systems.Damage[1] != 2 || systems.Damage[2] != 1 || systems.Damage[9] != 0 {
		t.Fatal("service erased other damage", systems.Damage)
	}
	if m.Sortie.Status != 0 || m.Sortie.Pending != "" || m.Sortie.Result != nil || m.Sortie.FARPLosses != 0 || m.Sortie.Services != 1 {
		t.Fatal("FARP fabricated return/loss or prompt", m.Sortie)
	}
	if !reflect.DeepEqual(blueprint, m.Stores.Blueprint) {
		t.Fatal("service mutated immutable accepted blueprint")
	}
	after, _ := json.Marshal(prepared)
	if !bytes.Equal(before, after) {
		t.Fatal("service regenerated/wrote prepared mission")
	}
	// No invented per-visit cap/cooldown: source repeats its helper while all
	// guards remain true. Ammo cannot be restored by a presentation draw instead.
	m.Stores.Ships[0].Stations[0].Ammunition = 0
	serviced, err = m.AdvanceService(prepared, systems)
	if err != nil || !serviced || m.Sortie.Services != 2 || m.PlayerFlight.FlightStage != 0 {
		t.Fatal("invented visit limit/cooldown", err, m.Sortie)
	}
}

func TestFARPSourceGuardBoundaries(t *testing.T) {
	cases := []struct {
		name      string
		configure func(*InitializedMission, *flight.Systems)
		want      bool
	}{
		{"distance511", func(m *InitializedMission, _ *flight.Systems) { m.PlayerFlight.Pose.H0 += 511 * 16 }, true},
		{"distance512", func(m *InitializedMission, _ *flight.Systems) { m.PlayerFlight.Pose.H0 += 512 * 16 }, false},
		{"no-contact", func(m *InitializedMission, _ *flight.Systems) { m.PlayerFlight.ValidContact = false }, false},
		{"rotor-running", func(m *InitializedMission, _ *flight.Systems) { m.PlayerFlight.Rotor = 1 }, false},
		{"cooldown-active", func(m *InitializedMission, _ *flight.Systems) { m.PlayerFlight.FlightStage = 1 }, false},
		{"limit-below", func(m *InitializedMission, _ *flight.Systems) { m.Sortie = &SortieState{FARPLosses: 1} }, true},
		{"limit-equal", func(m *InitializedMission, _ *flight.Systems) { m.Sortie = &SortieState{FARPLosses: 2} }, false},
		{"limit-above", func(m *InitializedMission, _ *flight.Systems) { m.Sortie = &SortieState{FARPLosses: 3} }, false},
		{"limit-zero", func(m *InitializedMission, _ *flight.Systems) {
			binary.LittleEndian.PutUint16(m.Targets.Records[1][50:], 0)
		}, false},
		{"signed-negative-counter", func(m *InitializedMission, _ *flight.Systems) {
			m.Sortie = &SortieState{FARPLosses: -1}
			binary.LittleEndian.PutUint16(m.Targets.Records[1][50:], 0)
		}, true},
		{"signed-negative-limit", func(m *InitializedMission, _ *flight.Systems) {
			binary.LittleEndian.PutUint16(m.Targets.Records[1][50:], 0xffff)
		}, false},
		{"role-absent", func(m *InitializedMission, _ *flight.Systems) {
			binary.LittleEndian.PutUint16(m.Targets.Records[1][32:], 0)
		}, false},
		{"query-is-not-reference", func(m *InitializedMission, _ *flight.Systems) {
			binary.LittleEndian.PutUint32(m.Targets.Records[1][42:], 0x50000)
		}, false},
		{"terminal-fuel-out", func(m *InitializedMission, s *flight.Systems) { s.Damage[9] = 1 }, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, p, s := fARPFixture()
			c.configure(m, s)
			ship := m.Stores.Ships[0]
			got, err := m.AdvanceService(p, s)
			if err != nil || got != c.want {
				t.Fatal("source boundary", got, c.want, err)
			}
			if !got && m.Stores.Ships[0] != ship {
				t.Fatal("rejected service changed stores")
			}
			if m.Sortie.Pending != "" || m.Sortie.Status&0x80 != 0 {
				t.Fatal("FARP opened return prompt", m.Sortie)
			}
		})
	}
}

func TestFARPNeedsLiveRoleAndBaseStillWins(t *testing.T) {
	m, p, s := fARPFixture()
	// A frontend FARP label alone is not authoritative role presence/count.
	p.Objectives["base"] = MissionObjective{Point: MissionPoint{0x60000, 0x60000}}
	p.Objectives["FARP"] = MissionObjective{Point: MissionPoint{0x40000, 0x40000}, DisplayKind: 1, Count: 2}
	m.Targets = nil
	if yes, err := m.AdvanceService(p, s); err != nil || yes {
		t.Fatal("frontend FARP fabricated source carrier", yes, err)
	}
	m, p, s = fARPFixture()
	binary.LittleEndian.PutUint32(m.Targets.Records[0][42:], m.PlayerFlight.Pose.H0)
	binary.LittleEndian.PutUint32(m.Targets.Records[0][46:], m.PlayerFlight.Pose.H1)
	if yes, err := m.AdvanceService(p, s); err != nil || !yes || m.Sortie.Pending != "base" || m.Sortie.Status != 0x80 {
		t.Fatal("base precedence lost", err, m.Sortie)
	}
}

func TestServiceSelectedActorFollowerAnd609Ownership(t *testing.T) {
	m, p, s := fARPFixture()
	b := m.Stores.Blueprint[0]
	m.Stores.Blueprint = []RuntimeAircraft{b, b, b, b, b}
	m.Stores.Ships = append([]RuntimeAircraft(nil), m.Stores.Blueprint...)
	m.Stores.Damage = make([][10]byte, 5)
	m.Rows = make([]MissionObject, 5)
	m.Entities = make([]InitializedEntity, 5)
	for i := range m.Rows {
		m.Rows[i].Raw[10] = 0x20
		m.Stores.Ships[i].Fuel = 3
		m.Stores.Ships[i].Stations[0].Ammunition = 0
	}
	m.PlayerRow = 2
	m.Entities[1].ControlRaw[1] = 1
	m.Entities[1].ControlRaw[5] = 2 // selected source row2, not frontend selector.
	m.Entities[3].ControlRaw[1] = 1
	m.Entities[3].ControlRaw[5] = 1 // source accepts active leader row1 too.
	m.Entities[4].ControlRaw[1] = 2
	m.Entities[4].ControlRaw[5] = 2 // inactive/order-state gate.
	m.Stores.Damage[1][9] = 2
	m.Stores.Damage[1][2] = 1
	s.Fuel = []int16{3, 3, 3, 3, 3}
	s.Damage = [10]byte{}
	yes, err := m.AdvanceService(p, s)
	if err != nil || !yes {
		t.Fatal(err)
	}
	for i, want := range []int16{3, 80, 80, 80, 3} {
		if s.Fuel[i] != want {
			t.Fatalf("row%d fuel%d want%d", i, s.Fuel[i], want)
		}
	}
	if m.Stores.Damage[1][9] != 0 || m.Stores.Damage[1][2] != 1 {
		t.Fatal("refill did not clear only609", m.Stores.Damage[1])
	}
	// Generic visual destruction is not source9d1c/9dc4's first hit transition.
	m.Entities[4].Participation = 2
	m.Entities[4].Kind = 0x41
	m.Rows[4].Raw[0] = 0x80
	if _, err := m.AdvanceService(p, s); err != nil || m.Sortie.FARPLosses != 0 {
		t.Fatal("generic wreck consumed FARP loss ownership", err)
	}
}

func TestServiceDistanceOriginalWrappedWords(t *testing.T) {
	for _, c := range []struct {
		a    MissionPoint
		p    flight.Pose
		want uint16
	}{
		{MissionPoint{0x40000, 0x40000}, flight.Pose{H0: 0x4000f, H1: 0x4000f}, 0},
		{MissionPoint{0, 0}, flight.Pose{H0: 0xfffffff0, H1: 0}, 1},
		{MissionPoint{0x80000, 0x80000}, flight.Pose{}, 0}, // ABS16(8000)+ABS16(8000) wraps.
		{MissionPoint{0x7fff0, 0x7fff0}, flight.Pose{}, 65534},
	} {
		if got := SourceServiceDistance(c.a, c.p); got != c.want {
			t.Fatal(c, got)
		}
	}
}
