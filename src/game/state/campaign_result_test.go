package state

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"strings"
	"testing"
)

func campaignInput(t *testing.T) CampaignResultInput {
	t.Helper()
	s := &CampaignState{Version: 1, SourceProfile: assetdata.IslandIce469085.Name,
		Phase: CampaignActive, Theatre: TheatrePhilippines, TargetCounter: 5,
		SelectedScenario: 1, PriorScenario: 0, ScenarioOrdinal: 2,
		Retained: CampaignRetained{Reserved: make([][32]byte, 5), Provenance: make([]byte, 80)},
		Caches: []CampaignFormationCache{{Scenario: "PHIL_01", World: "PHIL2", Ordinal: 2, BeforeCount: 7,
			Objects: make([]MissionObject, 2), Provenance: []byte{42, 43}}}}
	for i := range s.Caches[0].Objects {
		for j := range s.Caches[0].Objects[i].Raw {
			s.Caches[0].Objects[i].Raw[j] = byte(j)
		}
		s.Caches[0].Objects[i].Formation = 42 + i
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	return CampaignResultInput{LaunchDraft: s, Scenario: "PHIL_01", World: "PHIL2", Ordinal: 2,
		SourceStage: 1, LaunchHigh: 0x80, CurrentMission: MissionCampaign}
}

func TestCampaignQualityOriginalCases(t *testing.T) {
	var objects [80][32]byte
	var credits [40]byte
	if got := CampaignQuality(objects, credits, 0); got != 100 {
		t.Fatalf("empty denominator: %d", got)
	}
	for i := 0; i < 17; i++ {
		objects[i][10], objects[i][11] = 15, 1
	}
	// Five credited of seventeen: IDIV100*5/17 truncates to29.
	credits[0], credits[1], credits[2] = 0x71, 0xf2, 3
	if got := CampaignQuality(objects, credits, 0); got != 29 {
		t.Fatalf("any credit owner and unsigned class15: %d", got)
	}
	objects[0][10] = 0xff // unsigned255 is excluded, not a signed negative class.
	if got := CampaignQuality(objects, credits, 0); got != 25 {
		t.Fatalf("classFF exclusion,4/16: %d", got)
	}
	objects[0][0] = 0x10 // flag70 independently admits the same row.
	if got := CampaignQuality(objects, credits, 0); got != 29 {
		t.Fatal("flag70 did not restore eligible row")
	}
	if got := CampaignQuality(objects, [40]byte{}, 3); got != 200 {
		t.Fatalf("both objective flags short-circuit: %d", got)
	}
}

func TestCampaignCacheWriterHeaderAndExactMutation(t *testing.T) {
	in := campaignInput(t)
	before, _ := json.Marshal(in.LaunchDraft)
	// Cache row0 is credited at writer header7, not row0 or retained metadata.
	in.Credits[3] = 0x70 // nonplayer credit7 in odd nibble7.
	out, err := PrepareCampaignResult(in)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := hex.DecodeString("000102030405060708091d0b0e0e0e434d50474e20575245434b001b1c1d1e1f")
	if !bytes.Equal(out.Campaign.Caches[0].Objects[0].Raw[:], want) {
		t.Fatalf("original32-byte update differs: %x", out.Campaign.Caches[0].Objects[0].Raw)
	}
	if out.Campaign.Caches[0].Objects[1] != in.LaunchDraft.Caches[0].Objects[1] ||
		!bytes.Equal(out.Campaign.Caches[0].Provenance, []byte{42, 43}) {
		t.Fatal("uncredited object/provenance was changed")
	}
	after, _ := json.Marshal(in.LaunchDraft)
	if !bytes.Equal(before, after) {
		t.Fatal("immutable launch draft was changed")
	}
	in.LaunchDraft.Caches[0].BeforeCount = 79
	before, _ = json.Marshal(in.LaunchDraft)
	if _, err = PrepareCampaignResult(in); err == nil || !strings.Contains(err.Error(), "outside80") {
		t.Fatal("out-of-range credit row accepted")
	}
	after, _ = json.Marshal(in.LaunchDraft)
	if !bytes.Equal(before, after) {
		t.Fatal("failed result partially changed launch")
	}
}

func TestCampaignCounterThresholdsAndCallGates(t *testing.T) {
	for _, tc := range []struct {
		name            string
		quality29       bool
		success         bool
		high            byte
		threshold, a, b int
		wantA, wantB    int
		wantVerdict     int8
		bonus           int16
	}{
		{"quality29", true, true, 0x80, 5, 0, 0, 0, 1, 0, 0},
		{"quality30", false, true, 0x80, 5, 0, 0, 1, 0, 0, 0},
		{"success-required", false, false, 0x80, 5, 0, 0, 0, 1, 0, 0},
		{"launch-high-gate", false, true, 0x60, 5, 0, 0, 0, 0, 0, 0},
		{"terminal-win", false, true, 0x80, 5, 4, 0, 5, 0, 1, 100},
		{"terminal-tie-defeat", false, true, 0x80, 5, 4, 5, 5, 5, -1, 0},
		{"signed-wrap127", false, true, 0x80, 127, 127, 0, 128, 0, 0, 0},
		{"signed-threshold128", false, true, 0x80, 128, 0, 0, 1, 0, 1, 100},
		{"byte-wrap255", false, true, 0x80, 5, 255, 0, 0, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := campaignInput(t)
			in.LaunchDraft.TargetCounter, in.LaunchDraft.ResultCounters = tc.threshold, [2]int{tc.a, tc.b}
			in.LaunchHigh, in.ScoredSuccess = tc.high, tc.success
			// Independent hand cases:5/17=29,3/10=30.
			n, credit := 10, 3
			if tc.quality29 {
				n, credit = 17, 5
			}
			for i := 0; i < n; i++ {
				in.FinalObjects[i][11] = 1
			}
			for i := 0; i < credit; i++ {
				in.Credits[i/2] |= 1 << uint((i&1)*4)
			}
			out, err := PrepareCampaignResult(in)
			if err != nil {
				t.Fatal(err)
			}
			if out.Campaign.ResultCounters != [2]int{tc.wantA, tc.wantB} || out.Verdict != tc.wantVerdict || out.ScoreBonus != tc.bonus {
				t.Fatalf("counters/verdict/bonus: %+v", out)
			}
		})
	}
}

func TestCampaignConclusionOrderingAndQualificationBits(t *testing.T) {
	for _, tc := range []struct {
		name       string
		old, flags byte
		success    bool
		a, b       int
		want       byte
		conclude   bool
	}{
		{"win-clears-unretained-high", 0xf2, 0x80, true, 4, 0, 27, true},
		{"win-low3-retained", 0xf3, 0x80, true, 4, 0, 0xf3, true},
		{"tie-defeat", 0xf2, 0x80, true, 4, 5, 0xf2, true},
		{"loss18-bypasses-conclusion", 0xf2, 0x18, true, 4, 0, 0xf2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := campaignInput(t)
			in.LaunchDraft.ResultCounters = [2]int{tc.a, tc.b}
			in.NormalizedFlags, in.ScoredSuccess = tc.flags|3, tc.success
			in.FollowerFlags = [4]byte{0x80, 0x18, 0x20, 0}
			out, err := PrepareCampaignResult(in)
			if err != nil {
				t.Fatal(err)
			}
			if out.Concluded != tc.conclude {
				t.Fatal("loss caller ordering")
			}
			p := campaignPilot(t)
			p.LegacyRaw[0x55] = tc.old
			p.Statistics.MissionCampaignCounter = 12
			proposalBefore, _ := json.Marshal(out)
			if err = FinishCampaignResult(p, out, in.NormalizedFlags); err != nil {
				t.Fatal(err)
			}
			if p.LegacyRaw[0x55] != tc.want {
				t.Fatalf("raw55 got%02x want%02x", p.LegacyRaw[0x55], tc.want)
			}
			wantCounter := int32(12)
			if tc.conclude {
				wantCounter += 100
			}
			if p.Statistics.MissionCampaignCounter != wantCounter || (p.Campaign == nil) != tc.conclude || (p.MissionType == MissionFlight) != tc.conclude {
				t.Fatal("conclusion counter/stage/current-mode ownership")
			}
			if binary.LittleEndian.Uint16(p.LegacyRaw[0xa5:]) != 2 || binary.LittleEndian.Uint16(p.SquadronPilots[0].LegacyRaw[39:]) != 2 {
				t.Fatal("fixed four follower mask38 and overlapping opaque owner differ")
			}
			after, _ := json.Marshal(out)
			if !bytes.Equal(proposalBefore, after) {
				t.Fatal("branch finalization mutated proposal")
			}
		})
	}
}

func campaignPilot(t *testing.T) *Pilot {
	t.Helper()
	p := campaignTestPilot(t)
	p.Theatre = TheatrePhilippines
	p.SquadronPilots = []SquadronPilot{{LegacyRaw: make([]byte, 41)}}
	return p
}

func TestCampaignTrainingCrossmodeAndSourceIdentity(t *testing.T) {
	in := campaignInput(t)
	before, _ := json.Marshal(in.LaunchDraft)
	in.CurrentMission = MissionTraining
	if _, err := PrepareCampaignResult(in); err == nil || !strings.Contains(err.Error(), "Training") {
		t.Fatal("Training-active Campaign silently changed")
	}
	in.CurrentMission, in.World = MissionCampaign, "OTHER"
	if _, err := PrepareCampaignResult(in); err == nil {
		t.Fatal("cross-world cache accepted")
	}
	after, _ := json.Marshal(in.LaunchDraft)
	if !bytes.Equal(before, after) {
		t.Fatal("failed crossmode/source changed launch")
	}
}

func TestCampaignPH154ActualConsumersAndSuspension(t *testing.T) {
	// Step38's complete StepE first-three-preparation history, not a fabricated
	// successful population. The expected hash came from its independent probe.
	raw, err := os.ReadFile(testfixture.Path("campaign-islands/philippines-before4.json"))
	if err != nil {
		t.Fatal(err)
	}
	var history CampaignState
	if err = json.Unmarshal(raw, &history); err != nil {
		t.Fatal(err)
	}
	a := &assetdata.MissionAssets{Generation: missionTestData(t)}
	p := campaignPilot(t)
	p.Campaign = &history
	before, _ := json.Marshal(p)
	m, next, err := CampaignStart(a, p, 4, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if m.ObjectHash != "6d9aaf1a33699014844d2d921571ae5bf54422e720667dc1f2c02bee9319715f" || m.Objects[23].Formation != 154 || len(m.SourceWorld.FMHD)/39 != 154 {
		t.Fatal("known PHIL3 source history/provenance changed")
	}
	after, _ := json.Marshal(p)
	if !bytes.Equal(before, after) {
		t.Fatal("next preparation published before acceptance")
	}
	assignments := make([]FlightAssignment, 5)
	for i := range assignments {
		assignments[i].SourceIndex = i
	}
	next.Phase = CampaignSuspended
	next.Suspended = &CampaignSuspension{Mission: CampaignSnapshot(m), Aircraft: make([][36]byte, 5), InitialAircraftIDs: make([]byte, 5), Flight: assignments, Settings: defaultPreflightSettings()}
	p.Campaign = next
	restored, _, err := CampaignStart(a, p, 5, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ObjectHash != m.ObjectHash || restored.Objects[23].Formation != 154 {
		t.Fatal("suspension discarded opaque PH provenance")
	}
	// An actually consumed objective formation must still be rejected.
	bad, err := CloneCampaign(next)
	if err != nil {
		t.Fatal(err)
	}
	objective := bad.Suspended.Mission.Objectives["base"]
	objective.Formation = 154
	bad.Suspended.Mission.Objectives["base"] = objective
	p.Campaign = bad
	badBefore, _ := json.Marshal(p)
	if _, _, err = CampaignStart(a, p, 5, nil, nil); err == nil {
		t.Fatal("invalid actual base lookup accepted")
	}
	badAfter, _ := json.Marshal(p)
	if !reflect.DeepEqual(badBefore, badAfter) {
		t.Fatal("failed restore partially changed pilot")
	}
}

func TestCampaignConsecutiveResultsUseLaunchDraft(t *testing.T) {
	a := &assetdata.MissionAssets{Generation: missionTestData(t)}
	p := campaignTestPilot(t)
	p.Theatre = TheatrePersianGulf
	for seed := int64(1); seed <= 3; seed++ {
		before, _ := json.Marshal(p)
		m, launch, err := CampaignStart(a, p, seed, nil, nil)
		if err != nil {
			t.Fatalf("prepare%d: %v", seed, err)
		}
		var objects [80][32]byte
		for i, object := range m.Objects {
			if i < 80 {
				objects[i] = object.Raw
			}
		}
		in := CampaignResultInput{LaunchDraft: launch, Scenario: m.Scenario, World: m.World, Ordinal: int(m.SourceWorld.SNHD[0]), SourceStage: 1, LaunchHigh: 0x80, CurrentMission: MissionCampaign, FinalObjects: objects, NormalizedFlags: 0x80}
		out, err := PrepareCampaignResult(in)
		if err != nil {
			t.Fatal(err)
		}
		// No credit/objectives establishes a failure-counter increment; the
		// preparation selected cache must still come from this launch draft.
		if out.Campaign.ResultCounters != [2]int{0, int(seed)} || out.Concluded {
			t.Fatalf("result%d: %+v", seed, out)
		}
		after, _ := json.Marshal(p)
		if !bytes.Equal(before, after) {
			t.Fatal("proposal published Campaign early")
		}
		if err = FinishCampaignResult(p, out, 0x80); err != nil {
			t.Fatal(err)
		}
		if p.Campaign.SelectedScenario != m.SourceWorld.Suffix || p.Campaign.ScenarioOrdinal != int(m.SourceWorld.SNHD[0]) || len(p.Campaign.Caches) < 1 {
			t.Fatal("committed Campaign used previous pilot snapshot instead of accepted launch draft")
		}
	}
}

func TestCampaignFailedNextPreparationPreservesCompletedState(t *testing.T) {
	// Exact StepE preparation8 input: failed original-local relocation domain,
	// kept as a bounded error rather than a replacement terrain/generator.
	raw, err := os.ReadFile(testfixture.Path("campaign-continuations/central_europe-continuation.json"))
	if err != nil {
		t.Fatal(err)
	}
	var history []struct {
		Input *CampaignState `json:"input_campaign"`
	}
	if err = json.Unmarshal(raw, &history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 8 || history[7].Input == nil {
		t.Fatal("retained EURO7 witness framing changed")
	}
	p := campaignTestPilot(t)
	p.Theatre, p.Campaign = TheatreCentralEurope, history[7].Input
	p.Statistics.CareerScore, p.Statistics.MissionCampaignCounter = 1234, 7
	before, _ := json.Marshal(p)
	if _, _, err = CampaignStart(&assetdata.MissionAssets{Generation: missionTestData(t)}, p, 8, nil, nil); err == nil || !strings.Contains(err.Error(), "objective relocation diagnostic retry limit") {
		t.Fatalf("known unsupported EURO7 history unexpectedly changed: %v", err)
	}
	after, _ := json.Marshal(p)
	if !bytes.Equal(before, after) {
		t.Fatal("failed next preparation undid score or partially published Campaign")
	}
}

func TestCampaignConclusionBeforeQualificationAndRetirement(t *testing.T) {
	// Original A1e48a concludes first; section3:1de4 copies qualification bits
	// later, and A1e4e2 checks retirement after the packed counter+100.
	p := nResultPilot(RankCaptain)
	p.MissionType = MissionCampaign
	p.LegacyRaw[0x55] = 0xf2
	p.Statistics.MissionCampaignCounter = 4900
	f := contextFacts()
	f.MissionType, f.SourceFlags = MissionCampaign, 0x83
	f.LaunchSwitches |= 0x8000
	in := campaignInput(t)
	in.LaunchDraft.ResultCounters = [2]int{4, 0}
	p.Campaign = in.LaunchDraft
	f.ResultContext.Campaign = &in
	if err := FinalizeSortieFacts(&f); err != nil {
		t.Fatal(err)
	}
	out, err := ProposeSortie(p, f, nResultTables())
	if err != nil {
		t.Fatal(err)
	}
	if !out.Result.Campaign.Concluded || !out.SpecialOutcome {
		t.Fatal("source positive terminal Campaign was not special/concluded")
	}
	for i := 0; i < 2; i++ {
		q, err := ResolveSortieProposal(out, true)
		if err != nil {
			t.Fatal(err)
		}
		// f2 -> 5*2+1+10h=1b -> old-low2 copied to bits2/3 gives1f.
		if q.LegacyRaw[0x55] != 0x1f {
			t.Fatalf("source qualification ordering: %02x", q.LegacyRaw[0x55])
		}
		if q.Statistics.MissionCampaignCounter != 5001 || q.Status != StatusRetired || q.LegacyRaw[0x56] != 2 || q.MissionType != MissionFlight || q.Campaign != nil {
			t.Fatal("50th completed Campaign retirement did not consume the post-conclusion state")
		}
	}
	if p.Statistics.MissionCampaignCounter != 4900 || p.LegacyRaw[0x55] != 0xf2 {
		t.Fatal("proposal/resolve changed original pilot")
	}
}

func TestMissionLimitWaitsForCampaignConclusion(t *testing.T) {
	for _, tc := range []struct {
		name         string
		start        int32
		previousWins int
		expected     int32
		retired      bool
	}{
		{"ongoing at fifty", 49, 0, 50, false},
		{"campaign count alone", 4900, 4, 5001, false},
		{"concluding fiftieth mission", 49, 4, 150, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := nResultPilot(RankCaptain)
			p.MissionType = MissionCampaign
			p.Statistics.MissionCampaignCounter = tc.start
			f := contextFacts()
			f.MissionType, f.SourceFlags = MissionCampaign, 0x83
			f.LaunchSwitches |= 0x8000
			in := campaignInput(t)
			in.LaunchDraft.ResultCounters = [2]int{tc.previousWins, 0}
			f.ResultContext.Campaign = &in
			f.ResultContext.RetireByMissions = true
			p.Campaign = in.LaunchDraft
			if e := FinalizeSortieFacts(&f); e != nil {
				t.Fatal(e)
			}
			proposal, e := ProposeSortie(p, f, nResultTables())
			if e != nil {
				t.Fatal(e)
			}
			pilot := proposal.AcceptedPilot
			if pilot.Statistics.MissionCampaignCounter != tc.expected || (pilot.Status == StatusRetired) != tc.retired {
				t.Fatal(pilot.Statistics.MissionCampaignCounter, pilot.Status)
			}
		})
	}
}
