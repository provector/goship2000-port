package state

import (
	"gunship2000-assets/game/flight"
	"testing"
)

func sortieFixture() (*InitializedMission, *MissionData, *flight.Systems) {
	s := &flight.State{Pose: flight.Pose{H0: 0x40000, H1: 0x40000, Height: 2}, ValidContact: true}
	r := RuntimeAircraft{Flags: 3, FuelMaximum: 100, Fuel: 80, Chaff: 30, Flares: 30, Stations: [4]RuntimeStation{{Present: true, CompactID: 2, Ammunition: 11, Mode: 3}}}
	i := &InitializedMission{PlayerFlight: s, PlayerRow: 0, Rows: make([]MissionObject, 1), Stores: &RuntimeStores{Ships: []RuntimeAircraft{r}, Blueprint: []RuntimeAircraft{r}, Damage: make([][10]byte, 1)}}
	i.Rows[0].Raw[10] = 0xff
	m := &MissionData{Objectives: map[string]MissionObjective{"base": {Point: MissionPoint{0x40000, 0x40000}}}}
	sys := flight.NewSystems([]int16{3}, []bool{true})
	return i, m, &sys
}

func TestSourceServiceAndNaturalAnswers(t *testing.T) {
	i, m, sys := sortieFixture()
	i.Stores.Ships[0].Stations[0].Ammunition = 1
	service, e := i.AdvanceService(m, sys)
	if e != nil || !service || sys.Fuel[0] != 80 || i.Stores.Ships[0].Stations[0].Ammunition != 11 || i.Sortie.Pending != "base" {
		t.Fatal(service, e, i.Sortie, sys)
	}
	if i.AnswerNaturalEnd(false) || i.Sortie.Pending != "" || i.PlayerFlight.FlightStage != 128 {
		t.Fatal("N must reset cooldown without ending")
	}
	if service, e = i.AdvanceService(m, sys); e != nil || service {
		t.Fatal("N cooldown bypassed", e)
	}
	i.PlayerFlight.FlightStage = 0
	if _, e = i.AdvanceService(m, sys); e != nil {
		t.Fatal(e)
	}
	if !i.AnswerNaturalEnd(true) || i.Sortie.Result.Reason != "base" || i.Sortie.Result.SourceStatus != 0x80 {
		t.Fatal(i.Sortie)
	}
}

func TestServiceStrictSourceGuards(t *testing.T) {
	for _, tc := range []struct {
		name      string
		configure func(*InitializedMission, *flight.Systems)
	}{
		{"rotor", func(i *InitializedMission, s *flight.Systems) { i.PlayerFlight.Rotor = 1 }},
		{"cooldown", func(i *InitializedMission, s *flight.Systems) { i.PlayerFlight.FlightStage = 1 }},
		{"fuel-out", func(i *InitializedMission, s *flight.Systems) { s.Damage[9] = 1 }},
		{"not-contact", func(i *InitializedMission, s *flight.Systems) { i.PlayerFlight.ValidContact = false }},
		{"strict512", func(i *InitializedMission, s *flight.Systems) { i.PlayerFlight.Pose.H0 += 512 * 16 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i, m, s := sortieFixture()
			tc.configure(i, s)
			yes, e := i.AdvanceService(m, s)
			if e != nil || yes || i.Sortie.Pending != "" {
				t.Fatal(yes, e, i.Sortie)
			}
		})
	}
	i, m, s := sortieFixture()
	i.PlayerFlight.Pose.H0 += 511 * 16
	if yes, e := i.AdvanceService(m, s); e != nil || !yes {
		t.Fatal("511 must service", e)
	}
}

func TestSourceFourCorridorArmsUseSameAnchor(t *testing.T) {
	for _, tc := range []struct {
		base MissionPoint
		p    flight.Pose
		bit  byte
		want bool
	}{
		{MissionPoint{0x4000, 0x40000}, flight.Pose{H0: 0x4000, H1: 0x40000}, 1, true},
		{MissionPoint{0x4000, 0x40000}, flight.Pose{H0: 0x7bfff, H1: 0x40000}, 2, false},
		{MissionPoint{0x4000, 0x40000}, flight.Pose{H0: 0x4000, H1: 0x4fff0}, 1, true},
		{MissionPoint{0x4000, 0x40000}, flight.Pose{H0: 0x4000, H1: 0x50000}, 1, false},
		{MissionPoint{0x7bfff, 0x40000}, flight.Pose{H0: 0x7bfff, H1: 0x40000}, 2, true},
		{MissionPoint{0x40000, 0x4000}, flight.Pose{H0: 0x40000, H1: 0x4000}, 4, true},
		{MissionPoint{0x40000, 0x7bfff}, flight.Pose{H0: 0x40000, H1: 0x7bfff}, 8, true},
	} {
		if got := SourceCorridorEligible(tc.base, tc.p, tc.bit); got != tc.want {
			t.Fatal(tc, got)
		}
	}
}

func TestServiceDoesNotRestoreUpperOptionsOrPhysicalDamage(t *testing.T) {
	i, m, s := sortieFixture()
	i.Stores.Ships[0].Flags = 0xa6
	s.Damage[1] = 2
	s.Damage[2] = 1
	if _, e := i.AdvanceService(m, s); e != nil {
		t.Fatal(e)
	}
	if i.Stores.Ships[0].Flags != 0xa3 || s.Damage[1] != 2 || s.Damage[2] != 1 || i.Stores.Ships[0].Stations[0].Ammunition != 5 {
		t.Fatal(i.Stores.Ships[0], s)
	}
}

func TestDisabledContactEndsBeforeBaseAndReadinessGuards(t *testing.T) {
	i, m, s := sortieFixture()
	i.Sortie = &SortieState{Status: 3}
	m.Objectives = nil
	i.PlayerFlight.Rotor = 1
	i.PlayerFlight.FlightStage = 128
	s.Damage[9] = 1
	old := i.Stores.Ships[0]
	if serviced, e := i.AdvanceService(m, s); e != nil || serviced {
		t.Fatal(serviced, e)
	}
	if i.Sortie.Result == nil || i.Sortie.Result.Reason != "disabled-contact" || i.Sortie.Result.SourceStatus != 3 || s.Damage[9] != 1 || i.Stores.Ships[0] != old {
		t.Fatal(i.Sortie, s, i.Stores.Ships[0])
	}
}

func TestCorridorH1ConsumesH0BeforeLaterClamp(t *testing.T) {
	// Explicit arithmetic/wrapping witness, not a naturally initialized pose.
	// H1 first: low16(40000h>>4 -140000h>>4)=0, so metric0.
	// H0 later uses upper7bff and would yield15359. Reusing final clamped
	// H0 for both arms loses the source's earlier accepted H1 arm.
	base := MissionPoint{0x40000, 0x4000}
	final := flight.Pose{H0: 0x7bfff, H1: 0x4000}
	if !sourceCorridorEligible(base, final, 2|4, 0x140000) {
		t.Fatal("first H1 arm lost preclamp low-word input")
	}
	if SourceCorridorEligible(base, final, 2|4) {
		t.Fatal("control case should reject final-coordinate-only metrics")
	}
}
