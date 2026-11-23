package state

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func campaignTestPilot(t *testing.T) *Pilot {
	t.Helper()
	p := saveTestPilot(t, "Campaign Pilot")
	p.Rank = RankCaptain
	p.MissionType = MissionCampaign
	return p
}

func campaignHistoryCase(t *testing.T, theatre, index int) ([]byte, []int) {
	t.Helper()
	path := fmt.Sprintf(testfixture.Path("campaign-preparation/theatre%d-preparation%d.json"), theatre, index)
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("Step24 independent source-history witness unavailable")
	}
	if err != nil {
		t.Fatal(err)
	}
	var ref struct{ Draws []struct{ Value int } }
	if err = json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	draws := make([]int, len(ref.Draws))
	for i, d := range ref.Draws {
		draws[i] = d.Value
	}
	return raw, draws
}

func TestCampaignIndependentSourceHistories(t *testing.T) {
	a := &assetdata.MissionAssets{Generation: missionTestData(t)}
	for theatre := 0; theatre < 4; theatre++ {
		retained := &CampaignRetained{Reserved: make([][32]byte, 5), Provenance: make([]byte, 80)}
		caches := []CampaignFormationCache{}
		previous := 0
		for index := 0; index < 4; index++ {
			t.Run(fmt.Sprintf("theatre%d-preparation%d", theatre, index), func(t *testing.T) {
				raw, draws := campaignHistoryCase(t, theatre, index)
				th, _ := TheatreFromRaw(theatre)
				input := MissionInputs{MissionType: MissionCampaign, Theatre: th, Rank: RankCaptain, PilotName: "Campaign Pilot", DrawStream: draws, PreviousScenario: previous, Campaign: &CampaignGenerationInputs{Stage: 1, Raw50: 7 + index, Raw52: index, ScenarioCacheAbsent: true, Caches: caches, Retained: retained}}
				m, err := GenerateMission(a.Generation, input)
				if err != nil {
					t.Fatal(err)
				}
				var ref struct {
					Population struct {
						Objects []struct {
							RawHex    string `json:"raw_hex"`
							Formation int    `json:"formation_index"`
						}
						ObjectHash string `json:"objects_sha256"`
					} `json:"population"`
				}
				if err = json.Unmarshal(raw, &ref); err != nil {
					t.Fatal(err)
				}
				if m.ObjectHash != ref.Population.ObjectHash || len(m.Objects) != len(ref.Population.Objects) {
					t.Fatal("active population/hash differs from independent source history")
				}
				for i, o := range m.Objects {
					if hex.EncodeToString(o.Raw[:]) != ref.Population.Objects[i].RawHex || o.Formation != ref.Population.Objects[i].Formation {
						t.Fatalf("object%d differs", i)
					}
				}
				if len(m.Draws) != len(draws) {
					t.Fatal("source bounded draw count differs")
				}
				checkMissionCanonicalFields(t, m, raw, false)
				if m.campaignInitialCache != nil {
					caches = append(caches, *m.campaignInitialCache)
				}
				previous = m.SourceWorld.Suffix
				retained = m.campaignRetained
			})
		}
	}
}

func TestCampaignFreshThresholdsAndNoPilotMutation(t *testing.T) {
	a := &assetdata.MissionAssets{Generation: missionTestData(t)}
	_, draws := campaignHistoryCase(t, 1, 0)
	for initDraw := 0; initDraw < 4; initDraw++ {
		p := campaignTestPilot(t)
		before, _ := json.Marshal(p)
		m, draft, err := CampaignStart(a, p, 1, append([]int{initDraw}, draws...), nil)
		if err != nil {
			t.Fatal(err)
		}
		if draft.TargetCounter != 5+initDraw || len(m.Draws) != len(draws)+1 || m.Draws[0].Site != "A1b342" {
			t.Fatal("initializer threshold/draw order differs")
		}
		after, _ := json.Marshal(p)
		if string(before) != string(after) {
			t.Fatal("preparation committed the pilot")
		}
		if len(draft.Caches) != 1 || draft.Phase != CampaignActive {
			t.Fatal("fresh Campaign metadata was not retained")
		}
	}
}

func TestCampaignIntermediateAsymmetryAndSpill(t *testing.T) {
	a := missionTestData(t)
	w := &a.Worlds[0]
	g, err := newGenericMissionPopulation(w, newMissionRNG(MissionInputs{Seed: 1}), 4, a.Tables.Leader)
	if err != nil {
		t.Fatal(err)
	}
	r := &CampaignRetained{Reserved: make([][32]byte, 5), Provenance: make([]byte, 80)}
	r.Provenance[9], r.Provenance[10] = 1, 2
	cache := CampaignFormationCache{Scenario: w.Scenario, World: w.WorldName, Ordinal: int(w.SNHD[0]), BeforeCount: 99, Objects: make([]MissionObject, 2), Provenance: []byte{7, 8}}
	// Header99 is ignored by the reader; the active-slot old bytes1/2 survive.
	if err = applyCampaignIntermediate(g, cache, r); err != nil {
		t.Fatal(err)
	}
	if g.objects[9].Formation != 1 || g.objects[10].Formation != 2 {
		t.Fatal("cache metadata was incorrectly normalized to the object interval")
	}
	cache.Objects = make([]MissionObject, 36)
	cache.Provenance = make([]byte, 36)
	cache.BeforeCount = 9
	before := len(g.objects)
	if err = applyCampaignIntermediate(g, cache, r); err == nil || len(g.objects) != before {
		t.Fatal("adjacent-state spill was not rejected before mutation")
	}
}
