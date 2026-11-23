package state

import (
	"encoding/binary"
	"encoding/json"
	"reflect"
	"testing"
)

// Operands/answers are hand cases from Step38's independent source models,
// GS bytes 41004..41015, normalizer326c, finalizer0c4e and captured Flight.
func TestResultCRTSourceSeed(t *testing.T) {
	r := ResultRandomState{Seed: 1, State: 1}
	if r.Draw("one", 1) != 0 || r.State != 1 || len(r.Draws) != 0 {
		t.Fatal("bound1 must not consume")
	}
	for i, want := range []struct {
		raw     uint16
		bounded int
	}{{41, 0}, {18467, 56}, {6334, 19}, {26500, 80}} {
		if got := r.Draw("case", 100); got != want.bounded || r.Draws[i].Raw15 != want.raw {
			t.Fatalf("draw%d %+v", i, r.Draws[i])
		}
	}
}

func contextFacts() SortieFacts {
	f := nResultFacts()
	f.ResultContext = &SortieResultContext{Version: 1, Seed: 1, SeedPolicy: "supplied-original-seed", Actors: []SortieActorFacts{{Actor: 0, RankRaw: 7}}, ReplacementNames: []string{}, OccupiedNames: []string{}}
	f.ResultContext.Actors[0].AcceptedSelector = f.AcceptedSelectors[0]
	for i := range f.ResultContext.DynamicKinds {
		f.ResultContext.DynamicKinds[i] = 0x40
	}
	return f
}

func TestRawFinalPointsAndCreditOriginalWidths(t *testing.T) {
	f := contextFacts()
	f.RawScore = 900 // unrelated live AE9 must not survive.
	f.ResultContext.DynamicKinds[0] = 0x1c
	f.ObjectValues[0] = 32767
	f.ResultContext.DynamicKinds[1] = 0x1c
	f.ObjectValues[1] = 1
	f.ResultContext.DynamicKinds[2] = 0x1d
	f.ObjectCredits[1] = 0x21
	f.ResultContext.DynamicKinds[3] = 15
	f.ObjectValues[3] = 80 // dying old kind.
	f.ResultContext.DynamicKinds[4] = 0x5c
	f.ObjectValues[4] = 100
	if err := FinalizeSortieFacts(&f); err != nil {
		t.Fatal(err)
	}
	if f.RawScore != -32768 || f.ObjectCredits[1] != 0x29 {
		t.Fatalf("raw%d credits%x", f.RawScore, f.ObjectCredits)
	}
}

func TestProximityHealthOriginalBoundaries(t *testing.T) {
	for _, tc := range []struct {
		d    uint32
		want byte
	}{{1023, 0x40}, {1024, 0}} {
		f := contextFacts()
		f.SourceFlags = 0
		binary.LittleEndian.PutUint32(f.Targets[0][42:], tc.d*16)
		if err := FinalizeSortieFacts(&f); err != nil {
			t.Fatal(err)
		}
		if f.SourceFlags != tc.want {
			t.Fatalf("distance%d flags%x", tc.d, f.SourceFlags)
		}
	}
	if FinalResultMetric(InitializedPose{}, InitializedPose{H0: 0x100000}) != 0 {
		t.Fatal("low word wrapped metric")
	}
	for _, tc := range []struct {
		n, ordinal int
		word       uint16
	}{{0, 0, 255}, {9, 1, 138}, {10, 1, 125}, {10, 2, 65531}} {
		var d [12]byte
		for i := 0; i < tc.n; i++ {
			d[i] = byte(tc.ordinal)
		}
		d[10], d[11] = 2, 2
		if got := sourceHealth(d); got != tc.word {
			t.Fatalf("health %+v ->%d", tc, got)
		}
	}
}

func TestProficiencyRouteAndFireOriginalOperands(t *testing.T) {
	for _, tc := range []struct {
		rank   int
		awards [8]int8
		want   byte
	}{{1, [8]int8{}, 127}, {8, [8]int8{}, 38}, {9, [8]int8{}, 25}, {10, [8]int8{}, 25}, {11, [8]int8{}, 25}, {1, [8]int8{0, -128}, 114}, {1, [8]int8{0, 0, 0, 0, 0, 0, 69}, 0}} {
		rank, _ := RankFromRaw(tc.rank)
		got, err := ComputeSourceProficiency(FlightPilot{SourceIndex: 1, Rank: rank, AwardCounts: tc.awards[:]})
		if err != nil || got != tc.want {
			t.Fatalf("%+v got%d err%v", tc, got, err)
		}
	}
	if SourceRouteErrorPoint(0, 127, [4]uint16{0x1201, 0x3409, 0xff02, 0x9908}) != 7 {
		t.Fatal("two source averages")
	}
	if SourceRouteErrorPoint(17, 0, [4]uint16{}) != 17 {
		t.Fatal("zero denominator guard")
	}
	for _, tc := range []struct {
		random byte
		want   byte
		hit    bool
	}{{60, 96, true}, {59, 159, false}} {
		b, hit := SourceAcceptedFireError(127, tc.random, 60)
		if b != tc.want || hit != tc.hit {
			t.Fatal(tc, b, hit)
		}
	}
}

func TestNormalizerOriginalThresholdAndOwnership(t *testing.T) {
	// Seed1 bounded100 draws0,56,19,80. 80>threshold10 gives MIA;
	// safe80 and near40 bypass that draw even with severe display damage.
	f := contextFacts()
	f.SourceFlags = 0x20
	f.FinalPose.H0 = 100 * 32768
	r, err := normalizeResult(f)
	if err != nil {
		t.Fatal(err)
	}
	if r.NormalizedFlags[0] != 0x20 || r.Threshold != 10 || len(r.RNG.Draws) != 1 {
		t.Fatalf("seed1 draw0 <=10 %+v", r)
	}
	f.ResultContext.Seed = 415139642 // Original seed1's third retained CRT state; next raw15=26500 ->80.
	r, err = normalizeResult(f)
	if err != nil {
		t.Fatal(err)
	}
	if r.NormalizedFlags[0] != 8 {
		t.Fatal(r.NormalizedFlags)
	}
	for _, flags := range []byte{0xc0, 0x44, 0x30} {
		f.SourceFlags = flags
		r, _ = normalizeResult(f)
		if r.NormalizedFlags[0] != flags&0x9f || len(r.RNG.Draws) != 0 {
			t.Fatalf("bypass%x %+v", flags, r)
		}
	}
	f.MissionType = MissionTraining
	f.SourceFlags = 0x7f
	r, _ = normalizeResult(f)
	if r.NormalizedFlags[0] != 0x4b {
		t.Fatal("Training ANDCB")
	}
}

func TestSourceNormalizerStrictComparisonsAndDrawOrder(t *testing.T) {
	// First-draw seeds independently enumerated by the retained CRT reference:
	// 9020->90,9120->91,5909->59,6009->60,992->10,1093->11.
	for _, tc := range []struct {
		seed     uint32
		distance uint32
		want     byte
	}{{9020, 2, 0x20}, {9120, 2, 8}, {992, 100, 0x20}, {1093, 100, 8}} {
		f := contextFacts()
		f.SourceFlags = 0x20
		f.FinalPose.H0 = tc.distance * 32768
		f.ResultContext.Seed = tc.seed
		r, err := normalizeResult(f)
		if err != nil || r.NormalizedFlags[0] != tc.want {
			t.Fatalf("strict player threshold %+v result%+v err%v", tc, r, err)
		}
	}
	for _, tc := range []struct {
		seed  uint32
		flags byte
		want  byte
		draws int
	}{{5909, 0x20, 0x20, 1}, {6009, 0x20, 8, 1}, {6009, 0xa0, 0x20, 0}} {
		_, f := flightFixture(t)
		f.ResultContext.Seed = tc.seed
		f.ActorCount = 2
		f.ResultContext.Actors = f.ResultContext.Actors[:2]
		f.SourceFlags = 0x80
		f.ResultContext.Actors[1].Flags = tc.flags
		r, err := normalizeResult(f)
		if err != nil || r.NormalizedFlags[1] != tc.want || len(r.RNG.Draws) != tc.draws {
			t.Fatalf("follower %+v result%+v err%v", tc, r, err)
		}
	}
	// Counting mask38 excludes risk20, but enumeration mask18 can select it.
	_, f := flightFixture(t)
	f.SourceFlags = 0x80
	f.ResultContext.Seed = 5909
	f.ActorCount = 3
	f.ResultContext.Actors = f.ResultContext.Actors[:3]
	f.ResultContext.Actors[1].Flags = 0x20
	f.ResultContext.Actors[1].AcceptedSelector = 5
	f.ResultContext.Actors[2].Flags = 0x80
	f.ResultContext.Actors[2].AcceptedSelector = 5
	r, err := normalizeResult(f)
	if err != nil || r.RescuerActor != 1 || len(r.RNG.Draws) != 1 {
		t.Fatalf("literal rescue predicate asymmetry %+v err%v", r, err)
	}
}

func TestFollowerFinalFlagsBoundaryAndLiteralLeaderIndex(t *testing.T) {
	for _, tc := range []struct {
		distance uint32
		control  byte
		want     byte
	}{{768, 0, 0x80}, {769, 0, 0x20}, {769, 2, 0}} {
		_, f := flightFixture(t)
		f.ResultContext.Actors[1].Pose.H0 = tc.distance * 16
		f.ResultContext.Actors[1].Control = tc.control
		if err := FinalizeSortieFacts(&f); err != nil || f.ResultContext.Actors[1].Flags != tc.want {
			t.Fatalf("follower boundary %+v flags%x err%v", tc, f.ResultContext.Actors[1].Flags, err)
		}
	}
	_, f := flightFixture(t)
	a := &f.ResultContext.Actors[1]
	a.Pose.H0 = 769 * 16
	a.Control = 1
	a.Leader = 2
	f.ResultContext.Actors[3].Flags = 0x80 // Literalresult274+leader reads actor3, not actor2.
	if err := FinalizeSortieFacts(&f); err != nil || a.Flags != 0x80 {
		t.Fatalf("literal leader slot flags%x err%v", a.Flags, err)
	}
}

func TestProposalRoundtripLossAndOrderedCosmetics(t *testing.T) {
	p := nResultPilot(RankCaptain)
	f := contextFacts()
	f.SourceFlags = 8
	f.Reason = "terminal"
	out := nProposal(t, p, f)
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var round SortieProposal
	if err = json.Unmarshal(raw, &round); err != nil {
		t.Fatal(err)
	}
	if err = ValidateSortieProposal(&round); err != nil {
		t.Fatalf("loss proposal must survive pending JSON roundtrip: %v", err)
	}
	p, f = flightFixture(t)
	f.SourceFlags = 0x80
	out = nProposal(t, p, f)
	if len(out.Result.Recommendations) != 0 {
		t.Fatalf("no-objective9bdaFF must not be confused with player helper AX0: %+v", out.Result.Recommendations)
	}
	if len(out.Result.RNG.Draws) != 5 {
		t.Fatalf("four healthy wound draws plus feedback failure draw %+v", out.Result.RNG.Draws)
	}
	for i := 0; i < 4; i++ {
		if out.Result.RNG.Draws[i].Site != "crew"+string(rune('1'+i))+"-wound" {
			t.Fatal("source wound loop order", out.Result.RNG.Draws)
		}
	}
}

func flightFixture(t *testing.T) (*Pilot, SortieFacts) {
	t.Helper()
	p := nResultPilot(RankCaptain)
	p.MissionType = MissionFlight
	f := contextFacts()
	f.MissionType = MissionFlight
	f.ActorCount = 5
	f.AcceptedSelectors = []byte{0, 0, 0, 0, 0}
	f.RecommendedSelectors = []byte{2, 0, 0, 0, 0}
	f.InactiveHeaderPolicy = SortieSuppliedInactiveHeaders
	p.SquadronPilots = make([]SquadronPilot, 4)
	for i := 1; i <= 4; i++ {
		raw := make([]byte, 41)
		name := string(rune('A' + i))
		copy(raw, name)
		raw[33], raw[34], raw[35], raw[36] = 1, 1, 1, byte(i)
		p.SquadronPilots[i-1] = SquadronPilot{Name: name, Rank: RankWO1, Portrait: i, LegacyRaw: raw}
		copy(p.LegacyRaw[0x55+41*i:], raw)
		a := SortieActorFacts{Actor: i, SourceIndex: i, Name: name, RankRaw: 1, RawB: 127}
		copy(a.StoredRaw[:], raw)
		a.Ship[29] = 127
		f.ResultContext.Actors = append(f.ResultContext.Actors, a)
	}
	return p, f
}

func TestNaturalIdleCrewAndSourceIndexChoices(t *testing.T) {
	p, f := flightFixture(t)
	f.SourceFlags = 0x81
	if err := FinalizeSortieFacts(&f); err != nil {
		t.Fatal(err)
	}
	out := nProposal(t, p, f)
	if !out.Result.CrewPro || len(out.Result.Crew) != 4 {
		t.Fatalf("missing real crew %+v", out.Result)
	}
	for _, m := range out.Result.Crew {
		if m.Flight != 100 || m.Attack != 50 || m.Defense != 100 || m.Mean != 83 || m.Flags != 0x80 {
			t.Fatalf("original idle baseline %+v", m)
		}
	}
	if len(out.Result.Recommendations) == 0 || out.Result.Recommendations[0].Type != 0 || out.Result.Recommendations[0].Actor != 1 {
		t.Fatalf("stable first promotion %+v", out.Result.Recommendations)
	}
	choices := DefaultSortieCrewChoices(out)
	choices.Recipients[0] = 3
	q, err := ResolveSortieProposalWithChoices(out, true, choices)
	if err != nil {
		t.Fatal(err)
	}
	if q.SquadronPilots[2].Rank.Raw() != 2 || q.SquadronPilots[0].Rank.Raw() != 1 {
		t.Fatal("recipient not copied back by real stored source")
	}
	q2, err := ResolveSortieProposalWithChoices(out, true, choices)
	if err != nil || !reflect.DeepEqual(q, q2) {
		t.Fatal("duplicate resolution changed proposal/result")
	}
	if p.SquadronPilots[2].Rank.Raw() != 1 {
		t.Fatal("proposal mutated input pilot")
	}
}

func TestLossStatusesAndPosthumousPurpleHeart(t *testing.T) {
	for _, tc := range []struct {
		flags  byte
		status PilotStatus
		raw    byte
		ph     int8
	}{{0x10, StatusKIA, 4, 1}, {8, StatusMIA, 3, 0}, {0x18, StatusMIA, 3, 1}, {0x84, StatusActive, 1, 1}} {
		p := nResultPilot(RankCaptain)
		f := contextFacts()
		f.SourceFlags = tc.flags
		f.Reason = "terminal"
		f.PlayerTerminal = true
		out := nProposal(t, p, f)
		q, err := ResolveSortieProposal(out, true)
		if err != nil {
			t.Fatal(err)
		}
		if q.Status != tc.status || q.LegacyStatus != tc.raw || q.Statistics.AwardCounts[6] != tc.ph {
			t.Fatalf("flags%x status%s/%d awards%v", tc.flags, q.Status, q.LegacyStatus, q.Statistics.AwardCounts)
		}
		if tc.flags&0x18 != 0 && (out.Commission || out.Result.CrewPro) {
			t.Fatal("loss entered later ordinary return controllers")
		}
	}
}

func TestFactsExtensionRejectsTruncatedOriginalArrays(t *testing.T) {
	f := contextFacts()
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	var good SortieFacts
	if err = json.Unmarshal(raw, &good); err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err = json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	c := v["result_context"].(map[string]any)
	actors := c["actors"].([]any)
	actors[0].(map[string]any)["ship"] = []any{0}
	raw, _ = json.Marshal(v)
	if json.Unmarshal(raw, &good) == nil {
		t.Fatal("short SHIP array silently zero-filled")
	}
}

func TestImportExplicitPilotStatusTable(t *testing.T) {
	for _, tc := range []struct {
		raw    byte
		status PilotStatus
	}{{2, StatusRetired}, {3, StatusMIA}, {4, StatusKIA}, {5, StatusLegacyUnknown}} {
		raw := syntheticLegacyRoster()
		raw[0x56] = tc.raw
		s, err := ParseLegacyRoster("supplied", raw)
		if err != nil {
			t.Fatal(err)
		}
		if s.Slots[0].Status != tc.status || s.SelectedPilotID != "" {
			t.Fatal(tc, s.Slots[0])
		}
	}
}

func TestActorReassignmentUsesActualStoredSource(t *testing.T) {
	p, f := flightFixture(t)
	f.SourceFlags = 0x81
	a, b := &f.ResultContext.Actors[1], &f.ResultContext.Actors[3]
	a.SourceIndex, b.SourceIndex = b.SourceIndex, a.SourceIndex
	a.StoredRaw, b.StoredRaw = b.StoredRaw, a.StoredRaw
	a.Name, b.Name = b.Name, a.Name
	if err := FinalizeSortieFacts(&f); err != nil {
		t.Fatal(err)
	}
	out := nProposal(t, p, f)
	q, err := ResolveSortieProposal(out, true)
	if err != nil {
		t.Fatal(err)
	}
	if out.Result.Recommendations[0].SourceIndex != 3 || q.SquadronPilots[2].Rank.Raw() != 2 || q.SquadronPilots[0].Rank.Raw() != 1 {
		t.Fatal("visible actor row was mistaken for persistent SourceIndex")
	}
}

func TestSeparateRosterRecoveryBranchIsBounded(t *testing.T) {
	p := nResultPilot(RankCaptain)
	f := contextFacts()
	f.SourceFlags |= 1
	binary.LittleEndian.PutUint16(f.Targets[2][32:], 2)
	f.ResultContext.SelectedScenario = 4
	mia := nResultPilot(RankCaptain)
	mia.ID = "other-MIA"
	mia.Status = StatusMIA
	mia.LegacyStatus = 3
	mia.LegacyRaw[0x56] = 3
	mia.LegacyRaw[0x5b] = 4
	rows := []*Pilot{p, mia}
	owner, err := UnsupportedMissionRecoveryOwner(p, f, rows)
	if err != nil || owner != mia.ID {
		t.Fatal("matching MIA roster consumer was not excluded", owner, err)
	}
	mia.LegacyRaw[0x5b] = 3
	owner, err = UnsupportedMissionRecoveryOwner(p, f, rows)
	if err != nil || owner != "" {
		t.Fatal("unrelated MIA blocked ordinary result")
	}
	mia.LegacyRaw[0x5b] = 4
	rows = []*Pilot{mia, p}
	owner, err = UnsupportedMissionRecoveryOwner(p, f, rows)
	if err != nil || owner != "" {
		t.Fatal("literal source skips roster row0")
	}
}
