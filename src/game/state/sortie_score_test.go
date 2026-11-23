package state

import (
	"encoding/binary"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func TestSortieCapturedReturnFixture(t *testing.T) {
	var fixture struct {
		Before        []byte `json:"before_pilot_base64"`
		Flags         byte   `json:"source_flags"`
		Raw           int16  `json:"raw_score"`
		Elapsed       uint16 `json:"elapsed_da"`
		Coefficient   int16  `json:"coefficient"`
		SelectorMatch bool   `json:"selector_match"`
		Expected      struct {
			Flags          byte  `json:"flags"`
			Penalty        int16 `json:"penalty"`
			Last           int32 `json:"last_score"`
			Best           int32 `json:"best_score"`
			Career         int32 `json:"career"`
			Counter        int32 `json:"counter"`
			Qualifications byte  `json:"qualifications"`
		} `json:"expected"`
	}
	raw, err := os.ReadFile(testfixture.Path("flight-return/natural-return.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	p := nResultPilot(RankCaptain)
	p.LegacyRaw = fixture.Before
	p.Statistics = statisticsFromLegacy(fixture.Before)
	f := nResultFacts()
	f.SourceFlags = fixture.Flags
	f.RawScore = fixture.Raw
	f.ElapsedDA = fixture.Elapsed
	f.Coefficient = fixture.Coefficient
	f.SelectorMultisetMatch = fixture.SelectorMatch
	f.AcceptedSelectors = []byte{0}
	f.RecommendedSelectors = []byte{0}
	out := nProposal(t, p, f)
	q := out.AcceptedPilot
	if out.Score.NormalizedFlags != fixture.Expected.Flags || sortieWord(q, 0x59) != fixture.Expected.Penalty || q.Statistics.LastMissionScore != fixture.Expected.Last || q.Statistics.BestMissionScore != fixture.Expected.Best || q.Statistics.CareerScore != fixture.Expected.Career || q.Statistics.MissionCampaignCounter != fixture.Expected.Counter || q.LegacyRaw[0x55] != fixture.Expected.Qualifications {
		t.Fatalf("independent captured output fields mismatch: score%+v stats%+v penalty%d qualifications%02x", out.Score, q.Statistics, sortieWord(q, 0x59), q.LegacyRaw[0x55])
	}
}

func nResultTables() SortieTables {
	// Literal operands independently retained in Step36/check_results.py.
	return SortieTables{
		SourceProfile:       assetdata.IslandIce469085.Name,
		RankThresholds:      [12]int16{0, 100, 1000, 3000, 6000, 1000, 3000, 6000, 10000, 15000, 21000, 28000},
		MedalThresholds:     [6]int16{1700, 1300, 1000, 750, 500, 250},
		MedalPenaltyCredits: [6]int16{1500, 1000, 500, 200, 0, 0},
		RankNames:           [12]string{"WOC", "Warrant Officer1", "Chief Warrant Officer2", "Chief Warrant Officer3", "Chief Warrant Officer4", "2nd Lieutenant", "1st Lieutenant", "Captain", "Major", "Lieutenant Colonel", "Colonel", "Brigadier General"},
		MedalNames:          [8]string{"CMOH", "DSC", "SS", "BSV", "AMV", "ACV", "PH", "NDS"},
	}
}

func nResultPilot(rank Rank) *Pilot {
	return &Pilot{ID: "n-supplied-pilot", Name: "Supplied Result", Rank: rank, Status: StatusActive, MissionType: MissionSingle, Theatre: TheatrePersianGulf, LegacyRaw: make([]byte, LegacyRecordSize), Statistics: &PilotStatistics{AwardCounts: make([]int8, 8)}}
}

func nResultFacts() SortieFacts {
	// Retained records in these arithmetic fixtures are explicit supplied data;
	// their zero bytes are not claimed as a naturally generated launch default.
	return SortieFacts{SortieID: "n-supplied-sortie", SourceProfile: assetdata.IslandIce469085.Name, MissionType: MissionSingle, ActorCount: 1, Reason: "base", SourceFlags: 0x80, Coefficient: 10, LaunchSwitches: 2, ObjectCount: 80, Targets: make([][62]byte, 5), LaunchObjectiveKinds: [2]int16{3, 3}, RecommendedSelectors: []byte{2}, AcceptedSelectors: []byte{0}, InactiveHeaderPolicy: SortieFreshInactiveHeaders}
}

func nProposal(t *testing.T, p *Pilot, f SortieFacts) *SortieProposal {
	t.Helper()
	out, err := ProposeSortie(p, f, nResultTables())
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSortieOriginalReturnArithmetic(t *testing.T) {
	p := nResultPilot(RankCaptain)
	sortiePutWord(p, 0x59, 300)
	f := nResultFacts()
	f.SourceFlags = 0xc0
	f.Coefficient = 6
	f.ElapsedDA = 128
	f.AcceptedSelectors = []byte{0}
	f.RecommendedSelectors = []byte{0}
	f.SelectorMultisetMatch = true
	out := nProposal(t, p, f)
	if out.Score.NormalizedFlags != 0x80 || out.Score.Score != 30 || out.Success || out.Awards != [8]int8{} || out.AcceptedPilot.Rank != RankCaptain {
		t.Fatalf("captured neither-objective return: %+v", out)
	}
	q, err := ResolveSortieProposal(out, true)
	if err != nil {
		t.Fatal(err)
	}
	if sortieWord(q, 0x59) != 500 || q.Statistics.LastMissionScore != 30 || q.Statistics.BestMissionScore != 30 || q.Statistics.CareerScore != 30 || q.Statistics.MissionCampaignCounter != 1 {
		t.Fatalf("original handoff: %+v penalty%d", q.Statistics, sortieWord(q, 0x59))
	}
	if q.LegacyRaw[0x55] != 5 {
		t.Fatalf("source main/exit qualification writes: %02x", q.LegacyRaw[0x55])
	}
	if sortieWord(p, 0x59) != 300 || p.Statistics.CareerScore != 0 {
		t.Fatal("proposal mutated original pilot")
	}
}

func TestSortieObjectiveAndEndingDomains(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flags   byte
		reason  string
		score   int32
		penalty int16
		success bool
	}{
		{"neither", 0x80, "base", 0, 200, false},
		{"primary", 0x81, "carrier", 100, 0, true},
		{"secondary", 0x82, "base", 50, 0, true},
		{"both", 0x83, "base", 150, 0, true},
		{"voluntary_neither", 0, "voluntary", 0, 400, false},
		{"voluntary_primary", 1, "voluntary", 100, 200, true},
		{"corridor_primary", 1, "corridor", 100, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := nResultFacts()
			f.SourceFlags = tc.flags
			f.Reason = tc.reason
			out := nProposal(t, nResultPilot(RankCaptain), f)
			if out.Score.Score != tc.score || sortieWord(out.AcceptedPilot, 0x59) != tc.penalty || out.Success != tc.success {
				t.Fatalf("score%+v penalty%d success%v", out.Score, sortieWord(out.AcceptedPilot, 0x59), out.Success)
			}
		})
	}
	for _, old := range []int16{2900, 2999, 3000, -20} {
		p := nResultPilot(RankCaptain)
		sortiePutWord(p, 0x59, old)
		f := nResultFacts()
		f.Reason = "voluntary"
		f.SourceFlags = 0
		out := nProposal(t, p, f)
		want := map[int16]int16{2900: 3100, 2999: 3199, 3000: 3000, -20: 380}[old]
		if sortieWord(out.AcceptedPilot, 0x59) != want {
			t.Fatalf("pre-add gate%d →%d want%d", old, sortieWord(out.AcceptedPilot, 0x59), want)
		}
	}
}

func TestSortieSignedCreditsAndAircraftTables(t *testing.T) {
	for _, tc := range []struct {
		value   int16
		credit  byte
		want    int32
		success bool
	}{{-101, 7, 150, true}, {101, 7, 50, true}, {-101, 1, 100, false}, {101, 1, 100, true}} {
		f := nResultFacts()
		f.SourceFlags = 0x81
		f.ObjectValues[0] = tc.value
		f.ObjectCredits[0] = tc.credit
		out := nProposal(t, nResultPilot(RankCaptain), f)
		if out.Score.Score != tc.want || out.Success != tc.success {
			t.Fatalf("credit%d/value%d: score%d success%v", tc.credit, tc.value, out.Score.Score, out.Success)
		}
	}
	for _, tc := range []struct {
		quality  uint16
		own      int8
		inactive [4]int8
		flags    byte
		reason   string
		want     int32
	}{
		{2, 50, [4]int8{-50, 25, 50, 0}, 0x81, "base", 150},
		{1, 50, [4]int8{-50, 25, 50, 0}, 0x81, "base", 125},
		{0, 50, [4]int8{-50, 25, 50, 0}, 0x81, "base", 100},
		{2, -50, [4]int8{}, 0x81, "base", 50},
		{2, 50, [4]int8{-50}, 1, "voluntary", 50},
	} {
		f := nResultFacts()
		f.SourceFlags = tc.flags
		f.Reason = tc.reason
		f.PlayerAircraftValue = tc.own
		f.InactiveAircraftValues = tc.inactive
		f.InactiveHeaderPolicy = SortieSuppliedInactiveHeaders
		f.LaunchSwitches = tc.quality
		out := nProposal(t, nResultPilot(RankCaptain), f)
		if out.Score.Score != tc.want {
			t.Fatalf("aircraft case%+v →%+v", tc, out.Score)
		}
	}
	f := nResultFacts()
	f.SourceFlags = 0x81
	f.RawScore = 32760
	out := nProposal(t, nResultPilot(RankCaptain), f)
	if out.Score.AfterObjectiveBonus != -32676 || out.Score.Score != -32676 {
		t.Fatalf("word overflow: %+v", out.Score)
	}
}

func TestSortieElapsedBoundaries(t *testing.T) {
	for _, tc := range []struct {
		elapsed uint16
		factor  int16
		score   int32
	}{{10799, 100, 1000}, {10800, 100, 1000}, {10801, 100, 1000}, {11088, 99, 990}, {25200, 50, 500}, {65535, 50, 500}} {
		f := nResultFacts()
		f.SourceFlags = 0x83
		f.RawScore = 850
		f.ElapsedDA = tc.elapsed
		out := nProposal(t, nResultPilot(RankCaptain), f)
		if out.Score.Score != tc.score || out.Score.TimeFactor != tc.factor {
			t.Fatalf("elapsed%d→%+v", tc.elapsed, out.Score)
		}
	}
	f := nResultFacts()
	f.RawScore = -11
	f.Coefficient = 3
	f.ElapsedDA = 65535
	out := nProposal(t, nResultPilot(RankCaptain), f)
	if out.Score.Score != -3 || out.Score.TimeFactor != 100 {
		t.Fatalf("negative truncation/time: %+v", out.Score)
	}
}

func TestSortieMedalThresholdsRepeatAndOrder(t *testing.T) {
	for _, tc := range []struct {
		score    int16
		flags    byte
		quality  uint16
		count    int8
		repeat   byte
		medal    int
		deferred bool
		credit   int16
	}{
		{1700, 0x83, 2, 0, 0, 0, false, 1500},
		{1699, 0x83, 2, 0, 0, 1, false, 1000},
		{1700, 0x81, 2, 0, 0, 1, false, 1000},
		{1700, 0x83, 1, 0, 0, 2, false, 500},
		{249, 0x81, 2, 0, 0, -1, false, 0},
		{250, 0x81, 2, 0, 0, 5, false, 0},
		{250, 0x81, 2, 2, 0, 5, true, 0},
		{250, 0x81, 2, 2, 1, 5, false, 0},
	} {
		p := nResultPilot(RankCaptain)
		sortiePutWord(p, 0x59, 300)
		if tc.medal >= 0 {
			p.Statistics.AwardCounts[tc.medal] = tc.count
			p.LegacyRaw[0x2c+tc.medal] = tc.repeat
		}
		f := nResultFacts()
		f.SourceFlags = tc.flags
		f.LaunchSwitches = tc.quality
		bonus := int16(0)
		if tc.flags&1 != 0 {
			bonus += 100
		}
		if tc.flags&2 != 0 {
			bonus += 50
		}
		f.RawScore = tc.score - bonus
		out := nProposal(t, p, f)
		if out.MedalIndex != tc.medal || out.MedalDeferred != tc.deferred || out.PenaltyCredit != tc.credit {
			t.Fatalf("medal case%+v → index%d deferred%v credit%d", tc, out.MedalIndex, out.MedalDeferred, out.PenaltyCredit)
		}
		if sortieWord(out.AcceptedPilot, 0x59) != 300-tc.credit {
			t.Fatalf("unclamped medal credit: %d", sortieWord(out.AcceptedPilot, 0x59))
		}
	}
	// An award's credit is applied after the main rank query, not before it.
	p := nResultPilot(RankCaptain)
	p.Statistics.CareerScore = 9700
	sortiePutWord(p, 0x59, 1000)
	f := nResultFacts()
	f.SourceFlags = 0x81
	f.RawScore = 900
	out := nProposal(t, p, f)
	if out.AcceptedPilot.Statistics.CareerScore != 10700 || out.AcceptedPilot.Rank != RankCaptain || out.MedalIndex != 2 || sortieWord(out.AcceptedPilot, 0x59) != 500 {
		t.Fatalf("award/promotion ordering: %+v", out)
	}
	// Negative-value player credit disqualifies the ordinary medal, not credit7.
	f.RawScore = 2000
	f.ObjectCredits[0] = 1
	f.ObjectValues[0] = -1
	out = nProposal(t, nResultPilot(RankCaptain), f)
	if out.MedalIndex != -1 || out.Success {
		t.Fatal("negative-object player credit failed to disqualify")
	}
}

func TestSortiePreCommanderSpecialFlag(t *testing.T) {
	for _, tc := range []struct {
		raw     int16
		flags   byte
		special bool
	}{{0, 0x81, false}, {1550, 0x83, true}, {1549, 0x83, false}} {
		f := nResultFacts()
		f.RawScore = tc.raw
		f.SourceFlags = tc.flags
		out := nProposal(t, nResultPilot(RankCaptain), f)
		if out.SpecialOutcome != tc.special {
			t.Fatalf("pre-commander861c raw%d/flags%02x=%v", tc.raw, tc.flags, out.SpecialOutcome)
		}
	}
	p := nResultPilot(RankColonel)
	p.Statistics.CareerScore = 27900
	f := nResultFacts()
	f.SourceFlags = 0x81
	if out := nProposal(t, p, f); out.InitialRankRecommendation != 11 || !out.SpecialOutcome {
		t.Fatal("initial rank11 source special arm")
	}
}

func TestSortieTrainingProtectionAndNoCareerReplay(t *testing.T) {
	p := nResultPilot(RankWOC)
	p.MissionType = MissionTraining
	p.Statistics.LastMissionScore = 40
	p.Statistics.CareerScore = 99
	p.Statistics.MissionCampaignCounter = 3
	f := nResultFacts()
	f.MissionType = MissionTraining
	f.LaunchObjectiveKinds = [2]int16{}
	f.RawScore = 100
	f.LaunchSwitches = 0x400
	out := nProposal(t, p, f)
	if !out.TrainingSuccess || !out.Success || out.Score.CareerApplied || out.Score.ExportedScoreWord != 0 || out.AcceptedPilot.Statistics.CareerScore != 99 || out.AcceptedPilot.Statistics.MissionCampaignCounter != 3 || out.Awards[7] != 1 || out.AcceptedPilot.Rank != RankWO1 || out.AcceptedPilot.MissionType != MissionSingle {
		t.Fatalf("Training commander/NDS phase: %+v", out)
	}
	if out.AcceptedPilot.LegacyRaw[0x33] != 1 || out.AcceptedPilot.LegacyRaw[0x55] != 5 {
		t.Fatal("later Training helper / qualification writes missing")
	}
	f.RawScore = 99
	out = nProposal(t, p, f)
	if out.TrainingSuccess || out.Awards[7] != 0 || out.AcceptedPilot.Rank != RankWOC {
		t.Fatal("Training100 threshold")
	}
	f.RawScore = 100
	f.LaunchSwitches = 2
	out = nProposal(t, p, f)
	if !out.Score.CareerApplied || out.AcceptedPilot.Statistics.CareerScore != 199 || out.AcceptedPilot.Statistics.MissionCampaignCounter != 4 {
		t.Fatal("Training mission type conflated with0400")
	}
	p = nResultPilot(RankCaptain)
	p.Statistics.LastMissionScore = 50
	f = nResultFacts()
	f.LaunchSwitches = 0x400
	f.SourceFlags = 0x81
	f.RawScore = 1000
	out = nProposal(t, p, f)
	if out.Score.CareerApplied || out.Score.ExportedScoreWord != 50 || out.AcceptedPilot.Statistics.LastMissionScore != 50 {
		t.Fatal("protected nonTraining controller export ownership")
	}
	for _, mode := range []byte{1, 2, 3} {
		f.SourceMode = mode
		if _, err := ProposeSortie(p, f, nResultTables()); err == nil {
			t.Fatalf("replay/mode%d accepted", mode)
		}
	}
	if NormalizeSortieFlags(MissionTraining, 0xff) != 0xcb || NormalizeSortieFlags(MissionSingle, 0xc0) != 0x80 {
		t.Fatal("literal source normalization masks")
	}
}

func TestSortieCommanderChoiceAndOrdinaryPromotion(t *testing.T) {
	p := nResultPilot(RankCW2)
	p.Statistics.CareerScore = 2850
	f := nResultFacts()
	f.SourceFlags = 0x81
	f.RawScore = 250
	out := nProposal(t, p, f)
	if !out.Commission || out.CommanderRank != 6 || out.AcceptedPilot.Rank != RankFirstLieutenant || out.AcceptedPilot.MissionType != MissionFlight {
		t.Fatalf("commission recommendation: %+v", out)
	}
	refused, err := ResolveSortieProposal(out, false)
	if err != nil {
		t.Fatal(err)
	}
	if refused.Rank != RankCW3 || refused.MissionType != MissionSingle || sortieLong(refused, 0x7a) != 3200 {
		t.Fatalf("refusal then ordinary promotion: %+v", refused)
	}
	// Source callback writes rank/snapshot only; a Single captain stays Single.
	p = nResultPilot(RankCaptain)
	p.Statistics.CareerScore = 10200
	sortiePutWord(p, 0x59, 300)
	f = nResultFacts()
	f.SourceFlags = 0x81
	out = nProposal(t, p, f)
	if out.AcceptedPilot.Rank != RankMajor || out.AcceptedPilot.MissionType != MissionSingle {
		t.Fatal("ordinary promotion changed mission type")
	}
	p = nResultPilot(RankCaptain)
	p.Statistics.CareerScore = 10199
	sortiePutWord(p, 0x59, 300)
	out = nProposal(t, p, f)
	if out.AcceptedPilot.Rank != RankCaptain {
		t.Fatal("rank threshold boundary")
	}
	// Source16-bit penalty arithmetic crosses early career after medal credit.
	p = nResultPilot(RankWO1)
	p.Statistics.CareerScore = 1000
	sortiePutWord(p, 0x59, 300)
	f = nResultFacts()
	f.SourceFlags = 0x83
	f.RawScore = 1550
	out = nProposal(t, p, f)
	if out.CommanderRank != 5 || sortieWord(out.AcceptedPilot, 0x59) != -500 {
		t.Fatalf("early commission negative penalty rounding: rank%d penalty%d", out.CommanderRank, sortieWord(out.AcceptedPilot, 0x59))
	}
}

func TestSortieRetirementAndExplicitUnsupportedStates(t *testing.T) {
	p := nResultPilot(RankColonel)
	p.Statistics.CareerScore = 27900
	f := nResultFacts()
	f.SourceFlags = 0x81
	out := nProposal(t, p, f)
	if !out.Retirement || out.AcceptedPilot.Rank != RankBrigadierGeneral || out.AcceptedPilot.Status != StatusRetired || out.AcceptedPilot.LegacyRaw[0x56] != 2 {
		t.Fatal("rank11 retirement")
	}
	s := EmptySaveState()
	s.Slots[0] = out.AcceptedPilot
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	p = nResultPilot(RankCaptain)
	p.Statistics.MissionCampaignCounter = 4999
	out = nProposal(t, p, nResultFacts())
	if out.AcceptedPilot.Status != StatusRetired {
		t.Fatal("packedcounter5000 gate")
	}
	for _, mutate := range []func(*Pilot, *SortieFacts){
		func(p *Pilot, f *SortieFacts) { f.MissionType = MissionFlight; p.MissionType = MissionFlight },
		func(p *Pilot, f *SortieFacts) { f.ActorCount = 5 },
		func(p *Pilot, f *SortieFacts) { f.SourceFlags |= 4 },
		func(p *Pilot, f *SortieFacts) { f.PlayerTerminal = true },
		func(p *Pilot, f *SortieFacts) { f.Reason = "voluntary" },
		func(p *Pilot, f *SortieFacts) { f.Targets = nil },
		func(p *Pilot, f *SortieFacts) { f.ObjectCount = 79 },
		func(p *Pilot, f *SortieFacts) { f.LaunchObjectiveKinds[0] = 2 },
		func(p *Pilot, f *SortieFacts) { f.SelectorMultisetMatch = true },
		func(p *Pilot, f *SortieFacts) { f.InactiveAircraftValues[0] = -50 },
		func(p *Pilot, f *SortieFacts) { p.Statistics.AwardCounts = nil },
		func(p *Pilot, f *SortieFacts) { p.Statistics.MissionCampaignCounter = -1 },
		func(p *Pilot, f *SortieFacts) { p.Statistics.MissionCampaignCounter = 5000 },
		func(p *Pilot, f *SortieFacts) { binary.LittleEndian.PutUint16(p.LegacyRaw[0xa3:], 16) },
	} {
		p = nResultPilot(RankCaptain)
		f = nResultFacts()
		mutate(p, &f)
		if _, err := ProposeSortie(p, f, nResultTables()); err == nil {
			t.Fatal("unsupported/incomplete facts accepted")
		}
	}
}

func TestSortieProposalIsolationAndTamperDetection(t *testing.T) {
	p := nResultPilot(RankCaptain)
	f := nResultFacts()
	f.SourceFlags = 0x81
	before, _ := json.Marshal(p)
	out := nProposal(t, p, f)
	after, _ := json.Marshal(p)
	if string(before) != string(after) {
		t.Fatal("proposal mutated pilot")
	}
	out.Facts.Targets[0][61] = 123
	if f.Targets[0][61] != 0 {
		t.Fatal("proposal aliases TARGET tail")
	}
	if err := ValidateSortieProposal(out); err == nil {
		t.Fatal("edited retained source facts accepted")
	}
	out = nProposal(t, p, f)
	one, err := ResolveSortieProposal(out, true)
	if err != nil {
		t.Fatal(err)
	}
	two, err := ResolveSortieProposal(out, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(one, two) || p.Statistics.MissionCampaignCounter != 0 || one.Statistics.MissionCampaignCounter != 1 {
		t.Fatal("repeated resolution applied progression twice")
	}
	one.Statistics.AwardCounts[0] = 99
	if two.Statistics.AwardCounts[0] == 99 || out.AcceptedPilot.Statistics.AwardCounts[0] == 99 {
		t.Fatal("resolved pilot aliases pending proposal")
	}
	out.ScoredPilot.Statistics.CareerScore++
	if err := ValidateSortieProposal(out); err == nil {
		t.Fatal("edited scored pilot accepted")
	}
}

func TestSortiePendingFactsRequireCompleteExtents(t *testing.T) {
	full, _ := json.Marshal(nResultFacts())
	for _, field := range []string{"object_values", "object_credit_nibbles", "inactive_aircraft_table_values", "launch_display_objective_kinds", "targets", "final_objects", "raw_score", "source_mode"} {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(full, &fields)
		delete(fields, field)
		bad, _ := json.Marshal(fields)
		var parsed SortieFacts
		if err := json.Unmarshal(bad, &parsed); err == nil {
			t.Fatalf("missing%s accepted", field)
		}
	}
	for _, field := range []string{"object_values", "object_credit_nibbles", "inactive_aircraft_table_values", "launch_display_objective_kinds"} {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(full, &fields)
		var row []json.RawMessage
		_ = json.Unmarshal(fields[field], &row)
		row = row[:len(row)-1]
		fields[field], _ = json.Marshal(row)
		bad, _ := json.Marshal(fields)
		var parsed SortieFacts
		if err := json.Unmarshal(bad, &parsed); err == nil {
			t.Fatalf("short%s accepted", field)
		}
	}
	for _, field := range []string{"targets", "final_objects"} {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(full, &fields)
		var rows [][]json.RawMessage
		_ = json.Unmarshal(fields[field], &rows)
		rows[0] = rows[0][:len(rows[0])-1]
		fields[field], _ = json.Marshal(rows)
		bad, _ := json.Marshal(fields)
		var parsed SortieFacts
		if err := json.Unmarshal(bad, &parsed); err == nil {
			t.Fatalf("short%s row accepted", field)
		}
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(full, &fields)
	var values []json.RawMessage
	_ = json.Unmarshal(fields["object_values"], &values)
	values[0] = json.RawMessage("null")
	fields["object_values"], _ = json.Marshal(values)
	bad, _ := json.Marshal(fields)
	var parsed SortieFacts
	if err := json.Unmarshal(bad, &parsed); err == nil {
		t.Fatal("null known-zero scalar accepted")
	}
}
