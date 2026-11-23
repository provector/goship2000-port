//go:build devtools

package main

import (
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	menuui "gunship2000-assets/game/menus"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"testing"
)

func TestCommanderFixtureOnlySuppliesFreshCrewHistory(t *testing.T) {
	root := os.Getenv("GS2000_TEST_MISSIONS")
	if root == "" {
		t.Skip("GS2000_TEST_MISSIONS prepared fixture not declared")
	}
	a, err := assetdata.LoadFrontEndAssets(root)
	if err != nil {
		t.Fatal(err)
	}
	app := t.TempDir()
	c, err := newDevelopmentFrontEnd(ApplicationPaths{Directory: app}, a, gamestate.OfficeState{PilotName: "New Pilot", Active: true}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = configureCommanderAcceptance(c, app); err != nil {
		t.Fatal(err)
	}
	if c.Mission != nil || c.Rendering != nil {
		t.Fatal("fixture prepared/launched or supplied a simulation outcome")
	}
	p := c.Store.State().SelectedPilot()
	if p.Rank != gamestate.RankCaptain || p.MissionType != gamestate.MissionFlight || len(p.SquadronPilots) != 4 || p.Statistics.MissionCampaignCounter != 12 {
		t.Fatal("fresh history/crew fixture", p)
	}
	raw, err := os.ReadFile(filepath.Join(app, "dev-output", "scenario.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta struct {
		Seed   int64                   `json:"seed"`
		World  string                  `json:"world"`
		Flight []gamestate.FlightPilot `json:"flight"`
	}
	if err = json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Seed != 75 || meta.World != "GULF4.WRL" || len(meta.Flight) != 5 {
		t.Fatal("wrong reproducible scenario", meta)
	}
	if err = configureCommanderAcceptance(c, app); err == nil {
		t.Fatal("replaced retained scenario metadata")
	}
	if got, err := reopenScenarioSave(app); err != nil || got != c.Store.Directory {
		t.Fatal("owned reopen", got, err)
	}
	t.Logf("R1 assigned callsigns: %s / %s / %s / %s / %s", meta.Flight[0].Callsign, meta.Flight[1].Callsign, meta.Flight[2].Callsign, meta.Flight[3].Callsign, meta.Flight[4].Callsign)
	// Exercise the exact production reopen constructor and actual ordinary
	// briefing generation, not only the fixture metadata's preview generation.
	for _, fixed := range []bool{false, true} {
		var seed *int64
		value := int64(75)
		if fixed {
			seed = &value
		}
		reopened, err := newRuntimeFrontEnd(a, c.Store, seed)
		if err != nil {
			t.Fatal(err)
		}
		if !fixed {
			if reopened.MissionSeed != nil {
				t.Fatal("ordinary startup acquired a fixed seed")
			}
			continue
		}
		value = 1 // constructor owns a copy, not the mutable caller flag.
		reopened.FadeMilliseconds = 0
		if err = menuui.PreviewScreen(reopened, menuui.ScreenBriefing); err != nil {
			t.Fatal(err)
		}
		if reopened.Mission == nil || reopened.Mission.Data.Inputs.Seed != 75 || reopened.Mission.Data.ObjectHash != "d2d60fcb00b85efb2dc05a07c6ed2c2bc868f66364fce6c7b2a618e67c621544" {
			t.Fatal("reopened ordinary preparation did not use fixed seed75")
		}
	}
}
