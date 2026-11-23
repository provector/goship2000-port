package state

import (
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func gulfHistory(t *testing.T) *Pilot {
	t.Helper()
	if path := os.Getenv("GS2000_TEST_R6_CAMPAIGN"); path != "" {
		raw, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		var p Pilot
		if e = json.Unmarshal(raw, &p); e != nil {
			t.Fatal(e)
		}
		return &p
	}
	// Reconstruct the generation owner from the retained pre-base phase. No
	// pilot identity or result is manufactured; this is a supplied-state fixture.
	raw, e := os.ReadFile(testfixture.Path("servicing/after-initial.json"))
	if os.IsNotExist(e) {
		t.Skip("retained R6 Gulf generation prefix unavailable")
	}
	if e != nil {
		t.Fatal(e)
	}
	var x struct {
		Objects         []MissionObject `json:"objects"`
		Scenario, World string
	}
	if e = json.Unmarshal(raw, &x); e != nil {
		t.Fatal(e)
	}
	data := missionTestData(t)
	ordinal := -1
	for _, w := range data.Worlds {
		if w.Scenario == x.Scenario {
			ordinal = int(w.SNHD[0])
		}
	}
	cache := CampaignFormationCache{Scenario: x.Scenario, World: x.World, Ordinal: ordinal, BeforeCount: 9, Objects: append([]MissionObject(nil), x.Objects[9:]...)}
	for _, o := range cache.Objects {
		cache.Provenance = append(cache.Provenance, byte(o.Formation))
	}
	r := CampaignRetained{Reserved: make([][32]byte, 5), Provenance: make([]byte, 80)}
	for i, o := range x.Objects {
		r.Provenance[i] = byte(o.Formation)
		if i < 5 {
			r.Reserved[i] = o.Raw
		}
	}
	p := campaignTestPilot(t)
	p.Theatre = TheatrePersianGulf
	p.Campaign = &CampaignState{Version: 2, SourceProfile: assetdata.IslandIce469085.Name, Phase: CampaignActive, Theatre: p.Theatre, TargetCounter: 6, ResultCounters: [2]int{0, 4}, SelectedScenario: 3, Caches: []CampaignFormationCache{cache}, Retained: r, CacheInventory: freshCampaignCacheInventory(), GenerationPolicy: CampaignGenerationPolicyPairedV1}
	p.Campaign.setCacheKnowledge(ordinal, CampaignCachePresent, "retained actual generation prefix")
	return p
}

func TestRequiredFARPHistoryAndOrdinaryNeighbors(t *testing.T) {
	a := &assetdata.MissionAssets{Generation: missionTestData(t)}
	p := gulfHistory(t)
	before, _ := json.Marshal(p)
	legacy, _, oldErr := CampaignStart(a, p, 1, nil, nil, true)
	if oldErr == nil {
		t.Fatalf("retained before case unexpectedly admitted: %s", legacy.ObjectHash)
	}
	m, d, e := CampaignStart(a, p, 1, nil, nil, true, true)
	if e != nil {
		t.Fatal(e)
	}
	if len(m.Objects) != 80 || m.PlacementPlan == nil || d.PlacementPolicy != RequiredFARPPlacementV1 || d.GenerationPolicy != CampaignGenerationPolicyPairedV1 || d.Version != 3 {
		t.Fatalf("incomplete plan: %+v rows%d", m.PlacementPlan, len(m.Objects))
	}
	if m.Objectives["primary"].OriginalPoint != (MissionPoint{446464, 200704}) || m.Objectives["FARP"].OriginalPoint != (MissionPoint{436772, 133668}) {
		t.Fatalf("unexpected finite selection: %+v", m.PlacementPlan)
	}
	if m.Objectives["primary"].Point != (MissionPoint{471040, 249856}) {
		t.Fatal("paper offset lost", m.Objectives["primary"])
	}
	if got, _ := json.Marshal(p); !reflect.DeepEqual(before, got) {
		t.Fatal("input Campaign mutated")
	}
	for seed := int64(2); seed <= 6; seed++ {
		old, _, e0 := CampaignStart(a, p, seed, nil, nil, true)
		now, _, e1 := CampaignStart(a, p, seed, nil, nil, true, true)
		if e0 != nil {
			t.Logf("neighbor seed%d preexisting rejection: %v", seed, e0)
			continue
		}
		if e1 != nil {
			t.Fatal(seed, e1)
		}
		if now.PlacementPlan != nil {
			t.Logf("seed%d also reaches finite fallback", seed)
			continue
		}
		if !reflect.DeepEqual(old.Objects, now.Objects) || !reflect.DeepEqual(old.Objectives, now.Objectives) || !reflect.DeepEqual(old.Paths, now.Paths) || !reflect.DeepEqual(old.Draws, now.Draws) {
			t.Fatal("successful source path changed", seed)
		}
	}
	q, _, e := CampaignStart(a, p, 1, nil, nil, true, true)
	if e != nil || q.ObjectHash != m.ObjectHash || !reflect.DeepEqual(q.Draws, m.Draws) {
		t.Fatal("plan not deterministic", e)
	}
}

func TestRequiredFARPNoPlanIsAtomic(t *testing.T) {
	data := missionTestData(t)
	g, e := newGenericMissionPopulation(&data.Worlds[0], newMissionRNG(MissionInputs{Seed: 1}), 4, data.Tables.Leader)
	if e != nil {
		t.Fatal(e)
	}
	g.boundaryMinimum = 100 // Supplied impossible locked boundary; not natural history.
	b := &missionBuilder{g: g, base: 0, refs: map[int]int{2: 0, 3: 0}, placementPrefix: append([]byte(nil), g.grid...)}
	before, _ := json.Marshal(g.objects)
	grid := append([]byte(nil), g.grid...)
	if ok, e := b.planRequiredFARP(); ok || e == nil {
		t.Fatal("impossible domain admitted", ok, e)
	}
	after, _ := json.Marshal(g.objects)
	if string(before) != string(after) || !reflect.DeepEqual(grid, g.grid) || len(g.rng.trace) != 0 {
		t.Fatal("failed plan published state")
	}
}
