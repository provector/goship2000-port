package state

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"reflect"
	"strings"
	"testing"
)

func campaignCacheFixture(t *testing.T, k, n int) (*genericMissionPopulation, CampaignFormationCache, *CampaignRetained, [80]bool) {
	t.Helper()
	a := missionTestData(t)
	w := &a.Worlds[0]
	g, err := newGenericMissionPopulation(w, newMissionRNG(MissionInputs{Seed: 17}), 4, a.Tables.Leader)
	if err != nil {
		t.Fatal(err)
	}
	g.objects = g.objects[:k]
	r := &CampaignRetained{Reserved: make([][32]byte, 5), Provenance: bytes.Repeat([]byte{0xee}, 80)}
	c := CampaignFormationCache{Scenario: w.Scenario, World: w.WorldName, Ordinal: int(w.SNHD[0]), BeforeCount: 99, Objects: make([]MissionObject, n), Provenance: make([]byte, n)}
	var known [80]bool
	for i := 0; i < k; i++ {
		known[i] = true
		r.Provenance[i] = byte(i)
	}
	for i := range c.Objects {
		for j := range c.Objects[i].Raw {
			c.Objects[i].Raw[j] = byte(i*3 + j)
		}
		c.Objects[i].Formation = 0xfd // Deliberately differs from actual writer metadata.
		c.Provenance[i] = byte(i + 1)
	}
	return g, c, r, known
}

func TestCampaignPairedMetadataCapacityAndOwners(t *testing.T) {
	for _, tc := range []struct{ k, n int }{{9, 21}, {9, 36}, {9, 37}, {5, 75}} {
		t.Run(fmt.Sprintf("k%d_n%d", tc.k, tc.n), func(t *testing.T) {
			g, c, r, known := campaignCacheFixture(t, tc.k, tc.n)
			prefix := append([]MissionObject(nil), g.objects...)
			cacheBefore, _ := json.Marshal(c)
			retainedBefore, _ := json.Marshal(r)
			worldBefore, _ := json.Marshal(g.source)
			gridBefore := append([]byte(nil), g.grid...)
			if err := applyCampaignIntermediateKnown(g, c, r, &known, true); err != nil {
				t.Fatal(err)
			}
			if len(g.objects) != tc.k+tc.n || !reflect.DeepEqual(g.objects[:tc.k], prefix) {
				t.Fatal("append changed active prefix or count")
			}
			for i, got := range g.objects[tc.k:] {
				if got.Raw != c.Objects[i].Raw || got.Formation != int(c.Provenance[i]) {
					t.Fatalf("row%d lost its writer pair", i)
				}
			}
			cacheAfter, _ := json.Marshal(c)
			retainedAfter, _ := json.Marshal(r)
			worldAfter, _ := json.Marshal(g.source)
			if !bytes.Equal(cacheBefore, cacheAfter) || !bytes.Equal(retainedBefore, retainedAfter) || !bytes.Equal(worldBefore, worldAfter) || !bytes.Equal(gridBefore, g.grid) {
				t.Fatal("paired append changed input owners/world/resource state")
			}
			if len(g.rng.trace) != 0 || g.rng.r0(100, "after-read") != newMissionRNG(MissionInputs{Seed: 17}).r0(100, "after-read") {
				t.Fatal("paired append consumed RNG")
			}
			if len(g.decisions) != 1 || g.decisions[0].Operation != "intermediate_cache_paired" {
				t.Fatal("missing explicit pairing receipt")
			}
		})
	}
}

func TestCampaignPairedReadValidatesBeforeMutation(t *testing.T) {
	for _, tc := range []string{"short-provenance", "world", "scenario", "ordinal", "over-capacity", "unknown-prefix", "retained-framing"} {
		t.Run(tc, func(t *testing.T) {
			g, c, r, known := campaignCacheFixture(t, 9, 21)
			switch tc {
			case "short-provenance":
				c.Provenance = c.Provenance[:20]
			case "world":
				c.World = "OTHER.WRL"
			case "scenario":
				c.Scenario = "OTHER.SNR"
			case "ordinal":
				c.Ordinal++
			case "over-capacity":
				c.Objects, c.Provenance = make([]MissionObject, 72), make([]byte, 72)
			case "unknown-prefix":
				known[0] = false
			case "retained-framing":
				r.Provenance = r.Provenance[:79]
			}
			objects := append([]MissionObject(nil), g.objects...)
			if err := applyCampaignIntermediateKnown(g, c, r, &known, true); err == nil {
				t.Fatal("invalid paired input admitted")
			}
			if !reflect.DeepEqual(g.objects, objects) || len(g.rng.trace) != 0 || len(g.decisions) != 0 {
				t.Fatal("failed read changed population/decisions/RNG")
			}
		})
	}
	// The same small cache still reads old physical bytes under its old policy.
	g, c, r, known := campaignCacheFixture(t, 9, 21)
	for i := 9; i < 30; i++ {
		known[i] = true
	}
	if err := applyCampaignIntermediateKnown(g, c, r, &known); err != nil || g.objects[9].Formation != 0xee {
		t.Fatal("historical in-bounds reader changed", err)
	}
	g, c, r, _ = campaignCacheFixture(t, 9, 36)
	if err := applyCampaignIntermediate(g, c, r); err == nil || len(g.objects) != 9 {
		t.Fatal("historical spill boundary changed")
	}
}

func TestCampaignWriterPairsDecodeBindAndRoundTrip(t *testing.T) {
	a := &assetdata.MissionAssets{Generation: missionTestData(t)}
	w := &a.Generation.Worlds[0]
	context := CampaignCacheReadContext{Theatre: saveTheatres[w.Theatre], Scenario: w.Scenario, World: w.WorldName, Ordinal: int(w.SNHD[0])}
	const n = 36
	raw := make([]byte, 2+33*n)
	raw[0], raw[1] = 9, n
	for i := 0; i < n; i++ {
		binary.LittleEndian.PutUint32(raw[2+32*i+2:], uint32(1000+i))
		raw[2+32*n+i] = byte(i + 2)
	}
	s := &CampaignState{Version: 2, SourceProfile: assetdata.IslandIce469085.Name, Phase: CampaignActive, Theatre: context.Theatre, TargetCounter: 7, Retained: CampaignRetained{Reserved: make([][32]byte, 5), Provenance: make([]byte, 80)}, GenerationPolicy: CampaignGenerationPolicyPairedV1}
	before, _ := json.Marshal(s)
	bound, err := BindCampaignIntermediate(s, a, context, raw)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(s)
	if !bytes.Equal(before, after) || len(bound.Caches) != 1 || bound.GenerationPolicy != CampaignGenerationPolicyPairedV1 {
		t.Fatal("binding changed retained input or policy")
	}
	encoded, _ := json.Marshal(bound)
	var reopened CampaignState
	if err = json.Unmarshal(encoded, &reopened); err != nil || reopened.Validate() != nil {
		t.Fatal("paired owner did not reopen", err)
	}
	g, _, r, known := campaignCacheFixture(t, 9, 1)
	if err = checkCampaignCacheRead(campaignInputs(&reopened), w); err != nil {
		t.Fatal(err)
	}
	if err = applyCampaignIntermediateKnown(g, reopened.Caches[0], r, &known, true); err != nil {
		t.Fatal(err)
	}
	for i, o := range g.objects[9:] {
		if o.Formation != int(raw[2+32*n+i]) || binary.LittleEndian.Uint32(o.Raw[2:]) != uint32(1000+i) {
			t.Fatal("writer pair lost through import/reopen")
		}
	}
	for _, malformed := range [][]byte{raw[:len(raw)-1], append(append([]byte(nil), raw...), 0)} {
		if _, err := BindCampaignIntermediate(s, a, context, malformed); err == nil {
			t.Fatal("malformed writer framing admitted")
		}
	}
	if err := checkCampaignCacheRead(campaignInputs(s), w); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatal("paired policy invented absent cache", err)
	}
	s.setCacheKnowledge(context.Ordinal, CampaignCacheAbsent, "supplied verified absence")
	if err := checkCampaignCacheRead(campaignInputs(s), w); err != nil {
		t.Fatal("known absence rejected", err)
	}
}

func TestCampaignPolicyUpgradesOnlyAcceptedDraft(t *testing.T) {
	p := campaignTestPilot(t)
	a := &assetdata.MissionAssets{Generation: missionTestData(t)}
	before, _ := json.Marshal(p)
	m, draft, err := CampaignStart(a, p, 1, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(p)
	if !bytes.Equal(before, after) || draft.GenerationPolicy != CampaignGenerationPolicyPairedV1 || m.Inputs.GenerationPolicy != draft.GenerationPolicy || m.Inputs.Campaign.GenerationPolicy != draft.GenerationPolicy {
		t.Fatal("policy upgrade changed pilot or lost launch ownership")
	}
	legacy, _, err := CampaignStart(a, p, 1, nil, nil)
	if err != nil || legacy.Inputs.GenerationPolicy != "" {
		t.Fatal("old API call changed default", err)
	}
	legacyJSON, _ := json.Marshal(legacy.Inputs)
	if bytes.Contains(legacyJSON, []byte("generation_policy")) {
		t.Fatal("empty new field changed historical serialized inputs")
	}
	p.Campaign = draft
	_, next, err := CampaignStart(a, p, 2, nil, nil)
	if err != nil || next.GenerationPolicy != CampaignGenerationPolicyPairedV1 {
		t.Fatal("persisted paired interpretation downgraded", err)
	}
	p.Campaign.GenerationPolicy = "future-policy"
	if _, _, err = CampaignStart(a, p, 2, nil, nil, true); err == nil {
		t.Fatal("unknown existing policy silently overwritten")
	}
	bad := MissionInputs{MissionType: MissionCampaign, Theatre: p.Theatre, Rank: p.Rank, GenerationPolicy: CampaignGenerationPolicyPairedV1, Campaign: &CampaignGenerationInputs{Stage: 1, Raw50: 7}}
	if _, err = GenerateMission(a.Generation, bad); err == nil || !strings.Contains(err.Error(), "policies disagree") {
		t.Fatal("conflicting launch policy admitted", err)
	}
}
