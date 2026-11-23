package menus

import (
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"strings"
	"testing"
)

func TestCampaignPaperOriginalSignedPriorityCases(t *testing.T) {
	// Hand cases from GS P03a0..0416: signed bytes widen before the word sums.
	for _, x := range []struct{ target, success, failure, want int }{
		{1, 0, 0, -2}, {6, 0, 5, -1}, {100, 0, 3, 0}, {100, 0, 2, 1}, {100, 0, 1, 2},
		{100, 1, 1, 3}, {100, 1, 0, 4}, {100, 2, 0, 5}, {100, 3, 0, 6}, {100, 10, 0, 6},
		{128, 127, 128, 6}, {255, 128, 127, 0}, {0, 255, 255, -1}, {1, 255, 1, -2},
	} {
		if got := campaignPaperChoice(gamestate.CampaignGenerationInputs{Raw50: x.target, Raw51: x.success, Raw52: x.failure}); got != x.want {
			t.Fatalf("raw%d/%d/%d =>%d want%d", x.target, x.success, x.failure, got, x.want)
		}
	}
}

func TestCampaignPaperActualInputOwnershipAndPlacement(t *testing.T) {
	meta := assetdata.MissionPresentationData{Templates: map[int]string{0x335f: "OPERATION\n", 0x3370: "LEADER\n", 0x3395: "HOUR\n", 0x33a8: "CALLSIGN\n", 0x33b8: "CAMPAIGN STATUS:%t", 0x33cb: "INTRO\n\n", 0x344f: "DESPERATE\n\n", 0x34a2: "%t%s\n\n", 0x34a9: "PRIMARY\n", 0x372e: "S&D %s", 0x3732: "%s %s"}, CampaignStatus: []string{"S0", "S1", "S2", "S3", "S4", "S5", "S6"}}
	m := &gamestate.MissionData{Inputs: gamestate.MissionInputs{MissionType: gamestate.MissionCampaign, Campaign: &gamestate.CampaignGenerationInputs{Raw50: 6, Raw51: 0, Raw52: 4}}, Objectives: map[string]gamestate.MissionObjective{"primary": {RawKind: 3, DisplayKind: 3, Name: "target"}}}
	v := &BriefingView{Assets: &assetdata.MissionAssets{Metadata: meta, Generation: &assetdata.MissionGenerationData{Tables: assetdata.MissionTables{Callsigns: []string{"SIERRA"}}}}, Mission: m, Pilot: &gamestate.Pilot{Name: "TEST"}}
	before, _ := json.Marshal(m)
	got := v.OrdersText()
	if strings.Index(got, "CALLSIGN") > strings.Index(got, "CAMPAIGN STATUS") || strings.Index(got, "CAMPAIGN STATUS") > strings.Index(got, "PRIMARY") || !strings.Contains(got, "S0") {
		t.Fatal(got)
	}
	for n := 0; n < 3; n++ {
		if v.OrdersText() != got {
			t.Fatal("redraw changed prose")
		}
	}
	after, _ := json.Marshal(m)
	if string(before) != string(after) {
		t.Fatal("paper changed mission/RNG")
	}
	m.Inputs.Campaign.Raw52 = 5
	if !strings.Contains(v.OrdersText(), "DESPERATE") {
		t.Fatal("next committed failure not read")
	}
	m.Inputs.MissionType = gamestate.MissionSingle
	if strings.Contains(v.OrdersText(), "CAMPAIGN STATUS") {
		t.Fatal("nonCampaign paper gained status")
	}
}
