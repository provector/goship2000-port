package state

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
)

// Expected operands are independently derived in Step46/career/oracle.json.
func TestNewNamedWOCKnownSourceProficiency(t *testing.T) {
	p := &Pilot{Name: "Leader", Rank: RankCaptain, SquadronPilots: make([]SquadronPilot, 4)}
	for i, name := range []string{"Adams", "Allen", "Andrews", "Anderson"} {
		p.SquadronPilots[i] = SquadronPilot{Name: name, Rank: RankWOC, Portrait: i + 1, LegacyRaw: make([]byte, 41)}
	}
	before, _ := json.Marshal(p)
	f, e := InitFlightPilots(p, &MissionData{Inputs: MissionInputs{MissionType: MissionFlight}})
	if e != nil {
		t.Fatal(e)
	}
	s := &RuntimeStores{Ships: make([]RuntimeAircraft, 5), Blueprint: make([]RuntimeAircraft, 5), Damage: make([][10]byte, 5)}
	performance, e := InitializeCrewPerformance(s, f, 5)
	if e != nil {
		t.Fatal(e)
	}
	for i, row := range f {
		want := byte(25)
		if i == 0 {
			want = 0
		}
		if row.Incomplete || row.SourceIndex != i || !performance[i].SkillKnown || performance[i].RawA != 0 || performance[i].RawB != want || s.Ships[i].Proficiency == nil || *s.Ships[i].Proficiency != want || s.Blueprint[i].Proficiency == nil || *s.Blueprint[i].Proficiency != want {
			t.Fatalf("actor%d %+v performance%+v", i, row, performance[i])
		}
		var d [10]byte
		if e = s.Refill(i, &d); e != nil || s.Ships[i].Proficiency == nil || *s.Ships[i].Proficiency != want {
			t.Fatal("refill lost known proficiency", i, e)
		}
	}
	after, _ := json.Marshal(p)
	if !bytes.Equal(before, after) {
		t.Fatal("preparation changed stored history")
	}
	for _, tc := range []struct {
		rank       Rank
		counts     []int8
		incomplete bool
		want       byte
		valid      bool
	}{{RankWOC, make([]int8, 8), false, 25, true}, {RankWO1, make([]int8, 8), false, 127, true}, {RankWO1, []int8{0, 0, 0, 0, 0, 0, 69, 0}, false, 0, true}, {RankWO1, []int8{0, -128, 0, 0, 0, 0, 0, 0}, false, 114, true}, {RankWOC, make([]int8, 8), true, 0, false}, {RankWOC, nil, false, 0, false}} {
		got, e := ComputeSourceProficiency(FlightPilot{SourceIndex: 1, Rank: tc.rank, AwardCounts: tc.counts, Incomplete: tc.incomplete})
		if (e == nil) != tc.valid || got != tc.want {
			t.Fatal(tc, got, e)
		}
		if tc.valid {
			stores := &RuntimeStores{FreshShipPadding: true, Ships: make([]RuntimeAircraft, 2), Blueprint: make([]RuntimeAircraft, 2)}
			crew := []FlightPilot{{SourceIndex: 0}, {SourceIndex: 1, Rank: tc.rank, AwardCounts: tc.counts}}
			known, e := InitializeCrewPerformance(stores, crew, 2)
			if e != nil || !known[1].SkillKnown || stores.Ships[1].Proficiency == nil || *stores.Ships[1].Proficiency != tc.want {
				t.Fatal("known arithmetic result lost", tc, e)
			}
			var d [10]byte
			if e = stores.Refill(1, &d); e != nil || stores.Ships[1].Proficiency == nil || *stores.Ships[1].Proficiency != tc.want {
				t.Fatal("known arithmetic refill lost", tc, e)
			}
		}
	}
	for _, tc := range []struct {
		name      string
		raw       []byte
		malformed bool
	}{{"", make([]byte, 41), false}, {"Missing", nil, false}, {"Malformed", make([]byte, 40), true}} {
		p.SquadronPilots[0].Name = tc.name
		p.SquadronPilots[0].LegacyRaw = tc.raw
		f, e := InitFlightPilots(p, &MissionData{Inputs: MissionInputs{MissionType: MissionFlight}})
		if tc.malformed {
			if e == nil {
				t.Fatal("malformed raw admitted")
			}
			continue
		}
		if e != nil {
			t.Fatal(e)
		}
		if _, e = ComputeSourceProficiency(f[1]); e == nil {
			t.Fatal("unknown history acquired proficiency", tc.name)
		}
	}
}

func TestWOCSourceMetricsAndRankMismatchStayGuarded(t *testing.T) {
	p, f := flightFixture(t)
	for i := range p.SquadronPilots {
		p.SquadronPilots[i].Rank = RankWOC
		p.SquadronPilots[i].LegacyRaw = make([]byte, 41)
		a := &f.ResultContext.Actors[i+1]
		a.RankRaw = 0
		a.StoredRaw = [41]byte{}
		a.Ship[29] = 25
		a.RawA = 0
		a.RawB = 25
	}
	f.ResultContext.ReplacementNames = make([]string, 74)
	for i := range f.ResultContext.ReplacementNames {
		f.ResultContext.ReplacementNames[i] = fmt.Sprintf("Replacement%d", i)
	}
	f.SourceFlags = 0x80
	out := nProposal(t, p, f)
	for _, m := range out.Result.Crew {
		if m.Flight != 100 || m.Attack != 90 || m.Defense != 100 || m.Mean != 96 {
			t.Fatal("independent idle WOC metrics", m)
		}
	}
	f.ResultContext.Actors[1].StoredRaw[34] = 1
	if _, e := ProposeSortie(p, f, pendingTestTables()); e == nil {
		t.Fatal("typed/raw rank mismatch accepted")
	}
	f.ResultContext.Actors[1].StoredRaw[34] = 0
	f.ResultContext.Actors[1].Name = ""
	if _, e := ProposeSortie(p, f, pendingTestTables()); e == nil {
		t.Fatal("unknown identity accepted")
	}
}

func TestHistoricalIncompleteAndRawSignedHistoryPreserved(t *testing.T) {
	p := &Pilot{Name: "Leader", Rank: RankCaptain, SquadronPilots: make([]SquadronPilot, 4)}
	raw := bytes.Repeat([]byte{0xa5}, 41)
	copy(raw[26:34], []byte{255, 128, 127, 254, 253, 252, 251, 250})
	raw[34] = 0
	for i := range p.SquadronPilots {
		p.SquadronPilots[i] = SquadronPilot{Name: fmt.Sprintf("Named%d", i), Rank: RankWOC, Portrait: i + 1, LegacyRaw: append([]byte(nil), raw...)}
	}
	f, e := InitFlightPilots(p, &MissionData{Inputs: MissionInputs{MissionType: MissionFlight}})
	if e != nil {
		t.Fatal(e)
	}
	if f[1].StoredRaw != [41]byte(raw) || !bytes.Equal(p.SquadronPilots[0].LegacyRaw, raw) {
		t.Fatal("unknown stored bytes changed")
	}
	for i, want := range []int8{-1, -128, 127, -2, -3, -4, -5, -6} {
		if f[1].AwardCounts[i] != want {
			t.Fatal("signed award changed", i)
		}
	}
	f[1].Incomplete = true // Historical FilmLaunch owns this recorded classification.
	s := &RuntimeStores{FreshShipPadding: true, Ships: make([]RuntimeAircraft, 5), Blueprint: make([]RuntimeAircraft, 5)}
	metrics, e := InitializeCrewPerformance(s, f, 5)
	if e != nil {
		t.Fatal(e)
	}
	if metrics[1].SkillKnown || s.Ships[1].Proficiency != nil || s.Blueprint[1].Proficiency != nil {
		t.Fatal("historical incomplete acquired guessed skill")
	}
	if _, e = s.SourceShipRows(); e == nil {
		t.Fatal("missing producer guard bypassed")
	}
}
