package state

import (
	"bytes"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"reflect"
	"strings"
	"testing"
)

func TestCampaignInventoryMigrationAndConsumption(t *testing.T) {
	a := missionTestData(t)
	w := &a.Worlds[0]
	for _, imported := range []bool{false, true} {
		s := campaignInput(t).LaunchDraft
		if imported {
			s.ImportSHA256 = strings.Repeat("a", 64)
		}
		old, _ := json.Marshal(s)
		clone, err := CloneCampaign(s)
		if err != nil {
			t.Fatal(err)
		}
		upgradeCampaignKnowledge(clone)
		if clone.Version != 2 || !reflect.DeepEqual(clone.Caches, s.Caches) {
			t.Fatal("migration changed exact payload")
		}
		in := campaignInputs(clone)
		// Actual source lookup ordinal, not a test-only presence accessor.
		err = checkCampaignCacheRead(in, w)
		if imported && err == nil {
			t.Fatal("omitted imported ordinal became absent")
		}
		if !imported && err != nil {
			t.Fatal("native fresh namespace lost absence", err)
		}
		clone.setCacheKnowledge(int(w.SNHD[0]), CampaignCacheAbsent, "supplied verified directory inventory")
		if err = checkCampaignCacheRead(campaignInputs(clone), w); err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(s)
		if !bytes.Equal(old, after) {
			t.Fatal("old immutable state changed during migration")
		}
	}
	legacy := campaignInput(t).LaunchDraft
	legacy.CacheInventory = []CampaignCacheKnowledge{{0, CampaignCacheAbsent, "invalid historical extension"}}
	if legacy.Validate() == nil {
		t.Fatal("arbitrary version1 JSON accepted version2 meaning")
	}
}

func TestActualPreparationRejectsUnknownOrdinalWithoutPublication(t *testing.T) {
	p := campaignTestPilot(t)
	a := &assetdata.MissionAssets{Generation: missionTestData(t)}
	_, draws := campaignHistoryCase(t, 1, 0)
	_, draft, err := CampaignStart(a, p, 1, append([]int{0}, draws...), nil)
	if err != nil {
		t.Fatal(err)
	}
	p.Campaign = draft
	p.Campaign.ImportSHA256 = strings.Repeat("b", 64)
	p.Campaign.CacheInventory = nil // Every unprovided ordinal explicitly remains unknown.
	p.Campaign.Caches = nil
	before, _ := json.Marshal(p)
	if _, _, err = CampaignStart(a, p, 1, nil, nil); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("actual unknown lookup admitted: %v", err)
	}
	after, _ := json.Marshal(p)
	if !bytes.Equal(before, after) {
		t.Fatal("failed next preparation published partial history")
	}
	// A supplied directory inventory may explicitly establish absence; the old
	// import marker must not override that new, narrower knowledge at lookup.
	p.Campaign.CacheInventory = freshCampaignCacheInventory()
	if _, _, err = CampaignStart(a, p, 1, nil, nil); err != nil {
		t.Fatalf("known-absent imported ordinal did not use actual miss/generation: %v", err)
	}
}

func TestTrainingExactKnownFileEffect(t *testing.T) {
	owner := campaignInput(t).LaunchDraft
	input := TrainingCacheEffect{RetainedOwner: owner, SourceProfile: assetdata.IslandIce469085.Name, Stage: 1, Mission: MissionTraining, Ordinal: 0, Presence: CampaignCachePresent, Raw: make([]byte, 35)}
	input.Raw[0], input.Raw[1] = 79, 1
	for i := 0; i < 32; i++ {
		input.Raw[2+i] = byte(i)
	}
	input.Raw[34] = 0xfe // unrelated writer provenance must survive even across-world alias.
	input.Credits[39] = 0x70
	before := append([]byte(nil), input.Raw...)
	oldOwner, _ := json.Marshal(owner)
	out, err := ApplyTrainingCacheEffect(input)
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte(nil), before...)
	want[2+10] = 0x1d
	want[2+12], want[2+13] = 14, 14
	copy(want[2+15:2+27], "CMPGN WRECK\x00")
	if !bytes.Equal(out, want) || !bytes.Equal(input.Raw, before) {
		t.Fatalf("exact row79 operation %x", out)
	}
	newOwner, _ := json.Marshal(owner)
	if !bytes.Equal(oldOwner, newOwner) {
		t.Fatal("pure effect changed owner/counters")
	}
	input.Credits = [40]byte{}
	if got, err := ApplyTrainingCacheEffect(input); err != nil || !bytes.Equal(got, before) {
		t.Fatal("uncredited bytes changed", err)
	}
	input.Presence, input.Raw = CampaignCacheAbsent, nil
	if got, err := ApplyTrainingCacheEffect(input); err != nil || got != nil {
		t.Fatal("absent file created", err)
	}
	input.Presence = CampaignCacheUnknown
	if _, err := ApplyTrainingCacheEffect(input); err == nil {
		t.Fatal("unknown file treated absent")
	}
	for _, raw := range [][]byte{{0xff, 0}, {80, 1}, {0, 0xff}, {0, 0, 1}} {
		input.Presence, input.Raw = CampaignCachePresent, raw
		if _, err := ApplyTrainingCacheEffect(input); err == nil {
			t.Fatalf("malformed signed/framing input %x", raw)
		}
	}
}

func TestTrainingSuspendedAndReadContextBinding(t *testing.T) {
	a := &assetdata.MissionAssets{Generation: missionTestData(t)}
	p := campaignTestPilot(t)
	m, owner, err := CampaignStart(a, p, 1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	owner.Phase = CampaignSuspended
	owner.Suspended = &CampaignSuspension{Mission: CampaignSnapshot(m), Aircraft: make([][36]byte, 5), InitialAircraftIDs: make([]byte, 5), Flight: make([]FlightAssignment, 5)}
	for i := range owner.Suspended.Flight {
		owner.Suspended.Flight[i].SourceIndex = i
	}
	if err = owner.Validate(); err != nil {
		t.Fatal(err)
	}
	in := TrainingCacheEffect{RetainedOwner: owner, SourceProfile: owner.SourceProfile, Stage: 2, Mission: MissionTraining, Presence: CampaignCachePresent, Raw: []byte{79, 0}}
	if got, err := ApplyTrainingCacheEffect(in); err != nil || !bytes.Equal(got, in.Raw) {
		t.Fatal("valid suspended n0 effect", err)
	}
	in.LaunchHigh = 0x80
	if _, err := ApplyTrainingCacheEffect(in); err == nil {
		t.Fatal("Campaign launch masqueraded as Training")
	}
	in.Stage, in.LaunchHigh, in.RetainedOwner = 0, 0, nil
	if got, err := ApplyTrainingCacheEffect(in); err != nil || !bytes.Equal(got, in.Raw) {
		t.Fatal("stage0 should skip cache reader", err)
	}
	// Bind another exact read world; the file itself has no creator-world header.
	var context CampaignCacheReadContext
	for _, w := range a.Generation.Worlds {
		if w.Theatre == owner.Theatre.Raw() && int(w.SNHD[0]) != owner.ScenarioOrdinal && w.Suffix < 4 {
			context = CampaignCacheReadContext{owner.Theatre, w.Scenario, w.WorldName, int(w.SNHD[0])}
			break
		}
	}
	before, _ := json.Marshal(owner)
	bound, err := BindCampaignIntermediate(owner, a, context, []byte{99, 0})
	if err != nil {
		t.Fatal(err)
	}
	if bound.Version != 2 || len(bound.Caches) != len(owner.Caches)+1 {
		t.Fatal("association missing")
	}
	if _, err := BindCampaignIntermediate(bound, a, context, []byte{99, 0}); err == nil {
		t.Fatal("silent duplicate replacement")
	}
	context.World = "guessed creator world"
	if _, err := BindCampaignIntermediate(owner, a, context, []byte{99, 0}); err == nil {
		t.Fatal("unprepared read context admitted")
	}
	after, _ := json.Marshal(owner)
	if !bytes.Equal(before, after) {
		t.Fatal("binding changed suspended owner")
	}
}

func TestCheckedInactiveTailAndActualConsumer(t *testing.T) {
	a := missionTestData(t)
	w := &a.Worlds[0]
	for _, n := range []int{0, 16, 35, 36} {
		g, err := newGenericMissionPopulation(w, newMissionRNG(MissionInputs{Seed: 1}), 4, a.Tables.Leader)
		if err != nil {
			t.Fatal(err)
		}
		r := &CampaignRetained{Reserved: make([][32]byte, 5), Provenance: bytes.Repeat([]byte{255}, 80)}
		var known [80]bool
		for i := 0; i < 9+n && i < 80; i++ {
			known[i] = true
			r.Provenance[i] = 0
		}
		c := CampaignFormationCache{Scenario: w.Scenario, World: w.WorldName, Ordinal: int(w.SNHD[0]), BeforeCount: 99, Objects: make([]MissionObject, n), Provenance: make([]byte, n)}
		before := append([]MissionObject(nil), g.objects...)
		err = applyCampaignIntermediateKnown(g, c, r, &known)
		if n == 36 {
			if err == nil || !reflect.DeepEqual(before, g.objects) {
				t.Fatal("spill changed state")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			known[9] = false
			fresh, _ := newGenericMissionPopulation(w, newMissionRNG(MissionInputs{Seed: 1}), 4, a.Tables.Leader)
			if err := applyCampaignIntermediateKnown(fresh, c, r, &known); err == nil || !reflect.DeepEqual(before, fresh.objects) {
				t.Fatal("unknown active byte consumed")
			}
			g.objects[9].Formation = 255
			if _, err := g.objectFormation(9, "actual selected-objective lookup"); err == nil {
				t.Fatal("global reference guard removed")
			}
		}
	}
}

func TestPartialTailFullGenerationMatchesIndependentSourceInputs(t *testing.T) {
	a := missionTestData(t)
	_, draws := campaignHistoryCase(t, 1, 0)
	var expected []MissionObject
	var expectedDraws []MissionDraw
	for _, tail := range []byte{0x55, 0xfe} {
		r := &CampaignRetained{Reserved: make([][32]byte, 5), Provenance: bytes.Repeat([]byte{tail}, 80)}
		var known [80]bool
		for i := 0; i < 9; i++ {
			known[i], r.Provenance[i] = true, 0
		}
		input := MissionInputs{MissionType: MissionCampaign, Theatre: TheatrePersianGulf, Rank: RankCaptain, PilotName: "Partial carrier", DrawStream: draws, Campaign: &CampaignGenerationInputs{Stage: 1, Raw50: 7, ScenarioCacheAbsent: true, Retained: r, ProvenanceKnown: &known}}
		m, err := GenerateMission(a, input)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Objects) != 80 || m.campaignRetained == nil {
			t.Fatal("no complete final publication")
		}
		if expected == nil {
			expected, expectedDraws = m.Objects, m.Draws
		} else if !reflect.DeepEqual(expected, m.Objects) || !reflect.DeepEqual(expectedDraws, m.Draws) {
			t.Fatal("unknown inactive tail changed consumed output/RNG")
		}
		known[0] = false
		if _, err = GenerateMission(a, input); err == nil || !strings.Contains(err.Error(), "unknown") {
			t.Fatal("unknown actual reserved row admitted", err)
		}
	}
}
