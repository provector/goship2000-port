package menus

import (
	"encoding/binary"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"os"
	"reflect"
	"testing"
)

func TestActualPreparedFARPLogicalServiceAndGPSFreeze(t *testing.T) {
	c := mastFixture(t, 0, "pass") // Ordinary generated Europe Flight seed10; AI stays disabled.
	jFinishFade(t, c)
	full, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	c.Office.Assets, c.Menus = full, full.Menus
	if err := c.beginSortieResult(c.Mission.Final); err != nil {
		t.Fatal(err)
	}
	if c.Mission.Data.Inputs.MissionType != gamestate.MissionFlight || c.Mission.ResultLaunch == nil {
		t.Fatal("fixture did not retain genuine Flight admission", c.Mission.ResultUnavailable)
	}
	p := c.Rendering
	r := p.Flight
	if p.Initialized.Targets == nil {
		t.Fatal("accepted Flight lacks authoritative TARGET carrier")
	}
	target := p.Initialized.Targets.Records[1]
	if binary.LittleEndian.Uint16(target[32:]) == 0 || int16(binary.LittleEndian.Uint16(target[50:])) <= 0 {
		t.Fatal("fixture has no original FARP presence/limit")
	}
	if c.Mission.ResultLaunch == nil {
		t.Fatal("Flight source admission missing")
	}
	beforeSave := c.Store.State()
	objectHash := c.Mission.Data.ObjectHash
	blueprint := append([]int16(nil), r.Systems.Fuel...)
	for i := range blueprint {
		blueprint[i] = p.Initialized.Stores.Blueprint[i].Fuel
	}
	// Supplied service approach pose at the genuine FARP reference point. It is
	// a player-mechanical integration case, not a naturally flown/original witness.
	r.State.Pose.H0 = binary.LittleEndian.Uint32(target[42:])
	r.State.Pose.H1 = binary.LittleEndian.Uint32(target[46:])
	height, err := p.Renderer.Assets.ContactHeight(p.Initialized.World, world.QueryPose{H0: r.State.Pose.H0, H1: r.State.Pose.H1, Height: 32767}, p.Initialized.ContactObjects())
	if err != nil {
		t.Fatal(err)
	}
	r.State.Pose.Height = height + 2
	r.State.Pose.Angles = [3]uint16{}
	r.State.Rotor, r.State.FlightStage, r.State.Forward, r.State.Vertical = 0, 0, 0, 0
	r.Systems.Fuel[0] = 9
	r.Defenses.Chaff, r.Defenses.Flares = 0, 0
	for i := range p.Initialized.Stores.Ships[0].Stations {
		p.Initialized.Stores.Ships[0].Stations[i].Ammunition = 0
	}
	if path := os.Getenv("GS2000_N2_SERVICE_FIXTURE"); path != "" {
		fixture := map[string]any{"scope": "Actual generated EuropeFlightseed10/pass FARP; supplied service approach pose, not natural runtime flight.", "theatre": c.Mission.Data.Inputs.Theatre, "mission_type": c.Mission.Data.Inputs.MissionType, "seed": 10, "world": c.Mission.Data.World, "scenario": c.Mission.Data.Scenario, "FARP_TARGET62": target, "reference_H0": r.State.Pose.H0, "reference_H1": r.State.Pose.H1, "support_height": height, "FARP_limit": int16(binary.LittleEndian.Uint16(target[50:])), "base_TARGET62": p.Initialized.Targets.Records[0], "object_sha256": objectHash, "result_unavailable": c.Mission.ResultUnavailable}
		raw, _ := json.MarshalIndent(fixture, "", "  ")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.Write(raw)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := c.advanceFlight(.1); err != nil {
		t.Fatal(err)
	}
	if !r.State.ValidContact || p.Initialized.Sortie == nil || p.Initialized.Sortie.Services != 1 || p.Initialized.Sortie.Pending != "" || c.Screen != ScreenScene {
		t.Fatal("actual contact/FARP integration", r.State, p.Initialized.Sortie, c.Screen)
	}
	accepted := p.Initialized.Stores.Blueprint[0]
	if r.Systems.Fuel[0] != blueprint[0] || r.Defenses.Chaff != accepted.Chaff || r.Defenses.Flares != accepted.Flares {
		t.Fatal("refill did not publish accepted fuel/defense counts")
	}
	if !reflect.DeepEqual(beforeSave, c.Store.State()) || c.Mission.Data.ObjectHash != objectHash {
		t.Fatal("FARP wrote saved/generated mission")
	}
	count, fuel := p.Initialized.Sortie.Services, r.Systems.Fuel[0]
	act(t, c, Actions{GPSOpen: true})
	if p.GPS == nil {
		t.Fatal("GPS did not open at service point")
	}
	if err := c.AdvanceRendering(2); err != nil {
		t.Fatal(err)
	}
	if p.Initialized.Sortie.Services != count || r.Systems.Fuel[0] != fuel {
		t.Fatal("GPS advanced service/fuel")
	}
	act(t, c, Actions{Cancel: true})
	if p.GPS != nil {
		t.Fatal("GPS did not release")
	}
}
