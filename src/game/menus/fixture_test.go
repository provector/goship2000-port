package menus

import (
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"os"
	"strings"
	"testing"
)

func TestGeneratedMechanismFixturePreservesAirGuard(t *testing.T) {
	a, e := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if e != nil {
		t.Fatal(e)
	}
	in := gamestate.MissionInputs{MissionType: gamestate.MissionFlight, Theatre: gamestate.TheatreCentralEurope, Rank: gamestate.RankBrigadierGeneral, PilotName: "Test Pilot", Seed: 1, PreviousScenario: 0}
	old, e := gamestate.GenerateMission(a.Missions.Generation, in)
	if e != nil {
		t.Fatal(e)
	}
	if old.Support == nil || old.Support.Mode != 2 {
		t.Fatal("source seed1 air Support witness changed")
	}
	if e = gamestate.ValidateSupportLaunch(old); e == nil || !strings.Contains(e.Error(), "air Support") {
		t.Fatal("active air Support guard bypassed", e)
	}
	in.Seed = 10
	current, e := gamestate.GenerateMission(a.Missions.Generation, in)
	if e != nil {
		t.Fatal(e)
	}
	if current.Support != nil || current.Conditions.Flags&0x800 == 0 || current.Objectives["FARP"].RawKind != 1 || current.Objectives["FARP"].Count != 3 {
		t.Fatal("source None/corridor/FARP witness changed")
	}
	if e = gamestate.ValidateSupportLaunch(current); e != nil {
		t.Fatal(e)
	}
	if current.ObjectHash != "1d183f7d1dc557c7a678862d2713d929d88dc7cb7dae053e45914f96009911f7" {
		t.Fatal("independent generator witness changed", current.ObjectHash)
	}
}
