package state

import (
	"bytes"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"reflect"
	"strings"
	"testing"
)

func TestCampaignOpaqueProvenanceOnlyValidatedAtLookup(t *testing.T) {
	data := missionTestData(t)
	w := &data.Worlds[0]
	g, err := newGenericMissionPopulation(w, newMissionRNG(MissionInputs{Seed: 1}), 4, data.Tables.Leader)
	if err != nil {
		t.Fatal(err)
	}
	r := &CampaignRetained{Reserved: make([][32]byte, 5), Provenance: bytes.Repeat([]byte{255}, 80)}
	// Step38 supersedes the old eager-append expectation: the reader copies
	// physical provenance bytes, while base/title lookups dereference FMHD.
	// cache.Provenance must not replace the retained old-slot bytes.
	r.Provenance[9], r.Provenance[10] = 0, 255
	c := CampaignFormationCache{Scenario: w.Scenario, World: w.WorldName, Ordinal: int(w.SNHD[0]), Objects: make([]MissionObject, 2), Provenance: []byte{0, 0}}
	if err = applyCampaignIntermediate(g, c, r); err != nil {
		t.Fatalf("opaque provenance must survive the cache reader: %v", err)
	}
	if g.objects[9].Formation != 0 || g.objects[10].Formation != 255 {
		t.Fatal("reader no longer retains original old-slot formation references")
	}
	before := append([]MissionObject(nil), g.objects...)
	if _, err = g.objectFormation(10, "fixture actual FMHD lookup"); err == nil || !strings.Contains(err.Error(), "object10 formation255") || !reflect.DeepEqual(g.objects, before) {
		t.Fatalf("invalid actual lookup was not rejected without mutation: %v", err)
	}
}

func TestCampaignInvalidReservedReferenceReturnsError(t *testing.T) {
	p := campaignTestPilot(t)
	p.Campaign = &CampaignState{Version: campaignSchemaVersion, SourceProfile: assetdata.IslandIce469085.Name, Phase: CampaignActive, Theatre: p.Theatre, TargetCounter: 7, Retained: CampaignRetained{Reserved: make([][32]byte, 5), Provenance: make([]byte, 80)}, CacheInventory: freshCampaignCacheInventory()}
	p.Campaign.Retained.Provenance[0] = 255
	store := saveTestStore(t)
	state := store.State()
	state.Slots[0], state.SelectedPilotID = p, p.ID
	if err := store.Commit(state); err != nil {
		t.Fatal(err)
	}
	loaded, err := OpenSaveStore(store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	p = loaded.State().SelectedPilot()
	before, _ := json.Marshal(p)
	_, _, err = CampaignStart(&assetdata.MissionAssets{Generation: missionTestData(t)}, p, 18, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "base scene selection: object0 formation255") {
		t.Fatalf("GEN-02: expected source-bound reference error, got %v", err)
	}
	after, _ := json.Marshal(p)
	if !bytes.Equal(before, after) {
		t.Fatal("failed generation mutated pilot/Campaign input")
	}
	if !reflect.DeepEqual(loaded.State(), store.State()) {
		t.Fatal("failed generation changed the disposable save")
	}
}
