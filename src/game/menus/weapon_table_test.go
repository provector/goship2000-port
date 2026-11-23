package menus

import (
	"encoding/hex"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"os"
	"testing"
)

func TestLegal17JettisonAndAimUseSharedTable(t *testing.T) {
	a, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MENUS"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../../test-data/weapon-table/rows.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct{ Rows []string }
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	rows := make([][36]byte, len(f.Rows))
	for i, h := range f.Rows {
		b, e := hex.DecodeString(h)
		if e != nil || len(b) != 36 {
			t.Fatal(e)
		}
		copy(rows[i][:], b)
	}
	stores, err := gamestate.BuildRuntimeStores(a.Missions, &gamestate.MissionData{}, rows, true)
	if err != nil {
		t.Fatal(err)
	}
	// The shared command can consume any actual table row. This adapter
	// fixture uses the Kiowa's real accepted station without changing its ID.
	stores.Ships[0] = stores.Ships[4]
	p := &RenderingPreview{Initialized: &gamestate.InitializedMission{Stores: stores, SelectedStation: 2}, Flight: &FlightRuntime{MassTerm: 1000}}
	if flags, limit, ok := p.selectedAimDefinition(); !ok || flags != 0x0e10 || limit != 94 {
		t.Fatal("frozen upper aim record", flags, limit, ok)
	}
	if err = p.jettisonSelectedStation(); err != nil || p.Flight.MassTerm != 700 || stores.Ships[0].Stations[2].Ammunition != 0 {
		t.Fatal("actual Hellfire jettison", p.Flight.MassTerm, err)
	}
	if stores.Ships[0].Stations[1].RawWSHDID != 7 || stores.Ships[0].Stations[1].Ammunition != 7 {
		t.Fatal("unrelated station changed")
	}
	stores.Ships[0].Stations[2].Ammunition = -2
	if err = p.jettisonSelectedStation(); err != nil || p.Flight.MassTerm != 1000 {
		t.Fatal("signed jettison behavior changed", p.Flight.MassTerm, err)
	}
}
