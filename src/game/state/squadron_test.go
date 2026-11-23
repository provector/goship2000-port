package state

import (
	"bytes"
	"fmt"
	"testing"
)

func TestFlightInitializationPreservesBlankPromotedRecords(t *testing.T) {
	p := &Pilot{Name: "Captain JSON", Rank: RankCaptain, LegacyRaw: make([]byte, 290)}
	before := append([]byte(nil), p.LegacyRaw...)
	m := &MissionData{Inputs: MissionInputs{MissionType: MissionFlight}}
	f, err := InitFlightPilots(p, m)
	if err != nil || len(f) != 5 {
		t.Fatalf("initialization %v", err)
	}
	for _, member := range f[1:] {
		if member.Name != "" || member.Rank != RankWOC || !member.Incomplete {
			t.Fatal("manual promotion invented a squadron history")
		}
	}
	if !bytes.Equal(p.LegacyRaw, before) || len(p.SquadronPilots) != 0 {
		t.Fatal("entry changed persisted/source records")
	}
	m.Inputs.MissionType = MissionTraining
	f, err = InitFlightPilots(p, m)
	if err != nil || len(f) != 1 || f[0].Portrait != 0 {
		t.Fatalf("single-aircraft flight: %v", err)
	}
}

func TestSquadronInitializerUsesDrawsAndPreservesRawWords(t *testing.T) {
	p := &Pilot{ID: "synthetic", Name: "Test", Rank: RankCaptain, LegacyRaw: make([]byte, 290)}
	for i := 1; i < 5; i++ {
		r := p.LegacyRaw[0x55+41*i : 0x55+41*(i+1)]
		r[37], r[39] = byte(i+20), byte(i+30)
	}
	names := make([]string, 74)
	for i := range names {
		names[i] = fmt.Sprintf("Member%d", i)
	}
	draws := []int{0, 0, 1, 1, 2, 2, 3, 3}
	pos := 0
	rows, err := InitializeSquadron(p, []*Pilot{p}, names, func(bound int) int {
		if pos >= len(draws) {
			t.Fatal("unexpected initializer draw")
		}
		v := draws[pos]
		pos++
		return v
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, row := range rows {
		if row.Name != names[i] || row.Rank != RankWO1 || row.Portrait != i+1 {
			t.Fatalf("member%d initializer result", i)
		}
		if row.LegacyRaw[33] != 1 || row.LegacyRaw[35] != 1 || row.LegacyRaw[37] != byte(i+21) || row.LegacyRaw[39] != byte(i+31) {
			t.Fatal("initializer destroyed retained words or source raw defaults")
		}
	}
	if len(p.SquadronPilots) != 0 {
		t.Fatal("explicit initializer must return proposed records, not save implicitly")
	}
}

func TestSquadronTypedStateAndNameCounterBoundary(t *testing.T) {
	p := &Pilot{ID: "typed", Name: "Own", Rank: RankCaptain, LegacyRaw: make([]byte, 290), SquadronPilots: make([]SquadronPilot, 4)}
	for i := range p.SquadronPilots {
		raw := bytes.Repeat([]byte{0x41}, 41)
		copy(raw, []byte("Legacy\x00"))
		raw[34], raw[36] = 1, 2
		p.SquadronPilots[i] = SquadronPilot{Name: fmt.Sprintf("Typed%d", i), Rank: RankCaptain, Portrait: i + 1, LegacyRaw: raw}
	}
	f, err := InitFlightPilots(p, &MissionData{Inputs: MissionInputs{MissionType: MissionFlight}})
	if err != nil || f[1].Name != "Typed0" || f[1].Rank != RankCaptain || f[1].StoredRaw[26] != 0x41 {
		t.Fatalf("authoritative typed state/raw counters: %v", err)
	}
	p.SquadronPilots[0].Portrait = 36
	if _, err := InitFlightPilots(p, &MissionData{Inputs: MissionInputs{MissionType: MissionFlight}}); err == nil {
		t.Fatal("invalid portrait silently accepted")
	}
}

func TestSquadronCopybackUsesSourceIdentity(t *testing.T) {
	p, _ := NewPilot()
	p.SquadronPilots = []SquadronPilot{{Name: "One", Rank: RankWO1, Portrait: 1}, {Name: "Two", Rank: RankWO1, Portrait: 2}, {Name: "Three", Rank: RankWO1, Portrait: 3}, {Name: "Four", Rank: RankWO1, Portrait: 4}}
	flight, err := InitFlightPilots(p, &MissionData{Inputs: MissionInputs{MissionType: MissionFlight}})
	if err != nil {
		t.Fatal(err)
	}
	flight[1], flight[3] = flight[3], flight[1]
	stored, err := SquadronFromFlight(flight)
	if err != nil {
		t.Fatal(err)
	}
	for i, w := range stored {
		if w.Name != p.SquadronPilots[i].Name {
			t.Fatal("assignment order changed stored identity")
		}
	}
	flight[2].SourceIndex = flight[1].SourceIndex
	if _, err = SquadronFromFlight(flight); err == nil {
		t.Fatal("duplicate source identity accepted")
	}
}
