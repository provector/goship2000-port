package menus

import (
	"bytes"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func outfitTestView(t *testing.T) *OutfitView {
	t.Helper()
	a := &assetdata.MissionAssets{Preflight: &assetdata.PreflightAssets{SystemOrder: []int{3, 4, 5}, OutfitRanks: []string{"WOC", "WO1", "CW2", "CW3", "CW4", "2LT", "1LT", "CPT", "MAJ", "LTC", "COL", "BG"}, AwardNames: []string{"CMOH", "DSC", "SS", "BSV", "AMV", "ACV", "PH", "NDS"}},
		Images:     map[string][]byte{"POINTER.PIC": make([]byte, 64000), "PILOT.PIC": make([]byte, 64000), "MENU3037.PIC": make([]byte, 64000)},
		Generation: &assetdata.MissionGenerationData{Tables: assetdata.MissionTables{Ranks: make([]string, 12), Callsigns: []string{"Alpha", "Bravo"}}},
		Metadata:   assetdata.MissionPresentationData{Screens: map[string]assetdata.MissionScreen{}}, Fonts: map[int]*assetdata.BitmapFont{}}
	a.Generation.Tables.Ranks[0], a.Generation.Tables.Ranks[1], a.Generation.Tables.Ranks[7] = "WOC", "WO1", "Capt."
	screen := assetdata.MissionScreen{}
	for i := 0; i < 5; i++ {
		screen.Areas = append(screen.Areas, assetdata.OfficeArea{Key: i + 1, Rectangle: [4]int{(i % 3) * 107, (i / 3) * 96, 106, 95}})
	}
	screen.Areas = append(screen.Areas, assetdata.OfficeArea{Key: 6, Rectangle: [4]int{222, 174, 98, 17}})
	a.Metadata.Screens["OUTFIT.SCN"] = screen
	p := &gamestate.Pilot{ID: "synthetic", Name: "Test Pilot", Rank: gamestate.RankCaptain, LegacyRaw: make([]byte, 290)}
	m := &gamestate.MissionData{Inputs: gamestate.MissionInputs{MissionType: gamestate.MissionFlight}, Conditions: gamestate.MissionConditions{Callsign: 0, SecondCallsign: 1}}
	f, err := gamestate.InitFlightPilots(p, m)
	if err != nil {
		t.Fatal(err)
	}
	for i := range f {
		if i > 0 {
			f[i].Name, f[i].Rank, f[i].Portrait = "Member"+string(rune('A'+i)), gamestate.RankWO1, i
			f[i].StoredRaw[37] = byte(i)
		}
	}
	rows := make([][36]byte, 5)
	for i := range rows {
		rows[i][0] = byte(i)
	}
	v, err := NewOutfitView(a, m, f, rows)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestOutfitSourceAssetsAndSquadronTable(t *testing.T) {
	directory := os.Getenv("GS2000_TEST_MISSIONS")
	if directory == "" {
		t.Skip("set GS2000_TEST_MISSIONS to a prepared preflight tree")
	}
	a, err := assetdata.LoadMissionAssets(directory)
	if err != nil {
		t.Fatal(err)
	}
	if a.Preflight == nil || len(a.Preflight.SquadronNames) != 74 || a.Preflight.SquadronNames[0] != "Arthur" || a.Preflight.SquadronNames[73] != "Zimmer" {
		t.Fatal("source-backed squadron table is incomplete")
	}
	p := &gamestate.Pilot{ID: "source-test", Name: "Test Pilot Name", Rank: gamestate.RankCaptain, LegacyRaw: make([]byte, 290)}
	rng := rand.New(rand.NewSource(1))
	p.SquadronPilots, err = gamestate.InitializeSquadron(p, []*gamestate.Pilot{p}, a.Preflight.SquadronNames, rng.Intn)
	if err != nil {
		t.Fatal(err)
	}
	m, err := gamestate.GenerateMission(a.Generation, gamestate.MissionInputs{MissionType: gamestate.MissionFlight, Theatre: gamestate.TheatrePersianGulf, Rank: gamestate.RankCaptain, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	flight, err := gamestate.InitFlightPilots(p, m)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := gamestate.PrepareReadinessRows(a, m)
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewOutfitView(a, m, flight, rows)
	if err != nil {
		t.Fatal(err)
	}
	if a.Preflight.OutfitRanks[1] != "WO1" || a.Preflight.OutfitRanks[7] != "CPT" || a.Preflight.AwardNames[0] != "CMOH" || a.Preflight.AwardNames[7] != "NDS" {
		t.Fatal("short-rank/award source tables differ")
	}
	if !strings.HasPrefix(assetdata.OfficeCString(flight[0].Raw[4:30]), "CPT ") || outfitAwardText(flight[1].AwardCounts, a.Preflight.AwardNames) != "NDS" {
		t.Fatal("source label or initial NDS award is missing")
	}
	frame := v.Frame()
	captureOutfitObjectiveRepair(t, v, frame)
	if frame[90*320+100] != 234 || frame[3*320+80] != 9 || frame[99*320+60] != 7 || frame[11*320+3] != 9 {
		t.Fatal("source grey/bevel/group-title/portrait-border presentation changed")
	}
	if len(frame) != 64000 || !strings.Contains(flight[0].Callsign, "1") {
		t.Fatal("source-backed five-panel composition failed")
	}
	if project := os.Getenv("GS2000_FIXTURE_ROOT"); project != "" {
		before, err := os.ReadFile(filepath.Join(project, "captain-roster.dat"))
		if err != nil {
			t.Fatal(err)
		}
		expected, err := os.ReadFile(filepath.Join(project, "captain-squadron.dat"))
		if err != nil {
			t.Fatal(err)
		}
		p.LegacyRaw, p.SquadronPilots = append([]byte(nil), before[:290]...), nil
		draw := 0
		members, err := gamestate.InitializeSquadron(p, []*gamestate.Pilot{p}, a.Preflight.SquadronNames, func(bound int) int {
			value := draw / 2
			draw++
			return value
		})
		if err != nil {
			t.Fatal(err)
		}
		for i, member := range members {
			start := 0x7e + 41*i
			if string(member.LegacyRaw) != string(expected[start:start+41]) {
				t.Fatalf("Go initializer differs from independent source fixture at wingman%d", i+1)
			}
		}
	}
}

func outfitPanelAction(panel int, text string, selector, cancel bool) Actions {
	x, y := (panel%3)*107+20, (panel/3)*96+20
	return Actions{X: x, Y: y, Inside: true, Text: []rune(text), Selector1: selector, Cancel: cancel}
}

func TestOutfitPilotSwapRetainsSlotWordsAndPrimary(t *testing.T) {
	v := outfitTestView(t)
	before := append([]gamestate.FlightPilot(nil), v.Flight...)
	_, _ = v.Handle(outfitPanelAction(0, "P", false, false))
	if v.Mode != "" {
		t.Fatal("primary pilot must not enter swap mode")
	}
	_, _ = v.Handle(outfitPanelAction(1, "P", false, false))
	_, err := v.Handle(outfitPanelAction(4, "", true, false))
	if err != nil || v.Flight[1].Name != before[4].Name || v.Flight[4].Name != before[1].Name {
		t.Fatalf("pilot swap: %v", err)
	}
	if v.Flight[1].StoredRaw[37] != 1 || v.Flight[4].StoredRaw[37] != 4 || v.Flight[1].SourceIndex != 1 || v.Flight[1].Group != before[1].Group {
		t.Fatal("slot fields moved with a pilot")
	}
	stored, err := gamestate.SquadronFromFlight(v.Flight)
	if err != nil || stored[0].Name != before[4].Name {
		t.Fatalf("typed copyback: %v", err)
	}
}

func TestOutfitHelicopterSwapDuplicateAndCancel(t *testing.T) {
	v := outfitTestView(t)
	_, _ = v.Handle(outfitPanelAction(0, "H", false, false))
	_, _ = v.Handle(outfitPanelAction(2, "", true, false))
	if v.Rows[0][0] != 2 || v.Rows[2][0] != 0 {
		t.Fatal("whole-aircraft swap failed")
	}
	_, _ = v.Handle(outfitPanelAction(2, "D", false, false))
	_, _ = v.Handle(outfitPanelAction(4, "", true, false))
	if v.Rows[2] != v.Rows[4] {
		t.Fatal("duplicate did not retain source")
	}
	_, _ = v.Handle(outfitPanelAction(3, "H", false, false))
	_, _ = v.Handle(Actions{Cancel: true})
	if v.Mode != "" || v.Rows[0][0] != 2 {
		t.Fatal("Escape should cancel mode without reverting completed changes")
	}
	result, _ := v.Handle(Actions{X: 240, Y: 180, Inside: true, Selector1: true})
	if result.Kind != "return" {
		t.Fatal("RETURN action did not return")
	}
}

func TestOutfitLeaderAndAvailability(t *testing.T) {
	v := outfitTestView(t)
	_, _ = v.Handle(outfitPanelAction(2, "L", false, false))
	want := []int{1, 1, 0, 0, 0}
	for i := range v.Flight {
		if v.Flight[i].Group != want[i] || v.Flight[i].Leader != (i == 0 || i == 2) {
			t.Fatal("leader grouping differs from source branch")
		}
	}
	if v.Flight[0].Callsign != "Alpha 1" || v.Flight[2].Callsign != "Bravo 3" {
		t.Fatal("callsigns did not follow section grouping")
	}
	v.Mission.CampaignUnavailableAircraft = []int{0, -1, -1, -1}
	result, _ := v.Handle(outfitPanelAction(0, "D", false, false))
	if v.Mode != "" || !strings.Contains(result.Message, "unavailable") {
		t.Fatal("restricted duplication was accepted")
	}
	result, _ = v.Handle(outfitPanelAction(1, "", true, false))
	if result.Kind != "ordnance" || result.Index != 1 {
		t.Fatal("panel did not select ordnance")
	}
}

func TestOutfitShortRankAwardsAndCaptionBounds(t *testing.T) {
	v := outfitTestView(t)
	v.Flight[1].Name = "Christopher Longlastname"
	v.Flight[1].AwardCounts = []int8{1, 2, 0, -1, 0, 0, 0, 1}
	if err := gamestate.RefreshFlightPresentation(v.Flight, v.Assets, v.Mission); err != nil {
		t.Fatal(err)
	}
	label := assetdata.OfficeCString(v.Flight[1].Raw[4:30])
	if !strings.HasPrefix(label, "WO1 ") || strings.Contains(label, "Warrant") {
		t.Fatalf("OUTFIT must use original short rank table: %q", label)
	}
	awards := outfitAwardText(v.Flight[1].AwardCounts, v.Assets.Preflight.AwardNames)
	if awards != "CMOH DSC2 NDS" {
		t.Fatalf("signed count/order/one-count formatting: %q", awards)
	}
	// Original OUTFIT shows rank/name, never the distinct squadron awards
	// formatter. Compare both the actual panel and selected-member hover help.
	v.Assets.Fonts[3] = briefingTestFont()
	v.Hover, v.Selected = 2, 1
	counts := append([]int8(nil), v.Flight[1].AwardCounts...)
	decorated := v.Frame()
	v.Flight[1].AwardCounts = nil
	undecorated := v.Frame()
	if !bytes.Equal(decorated, undecorated) {
		t.Fatal("squadron awards leaked into OUTFIT rank/name or hover help")
	}
	v.Flight[1].AwardCounts = counts
	v.Frame()
	if !reflect.DeepEqual(v.Flight[1].AwardCounts, counts) || assetdata.OfficeCString(v.Flight[1].Raw[4:30]) != label {
		t.Fatal("OUTFIT draw mutated member rank/name or award data")
	}
	if outfitAwardText(nil, v.Assets.Preflight.AwardNames) != "" {
		t.Fatal("unknown awards must not be displayed as established zero counts")
	}
	_, _ = v.Handle(outfitPanelAction(1, "P", false, false))
	_, _ = v.Handle(outfitPanelAction(4, "", true, false))
	if got := outfitAwardText(v.Flight[4].AwardCounts, v.Assets.Preflight.AwardNames); got != awards {
		t.Fatalf("awards did not follow member swap: %q", got)
	}
}

func TestOutfitAwardsFromAuthoritativeAndStoredState(t *testing.T) {
	p := &gamestate.Pilot{Name: "Authoritative", Rank: gamestate.RankCaptain, LegacyRaw: make([]byte, 290), Statistics: &gamestate.PilotStatistics{AwardCounts: []int8{0, 0, 3, 0, 0, 0, 0, 1}}}
	p.LegacyRaw[0x7e+26], p.LegacyRaw[0x7e+33] = 2, 1
	f, err := gamestate.InitFlightPilots(p, &gamestate.MissionData{Inputs: gamestate.MissionInputs{MissionType: gamestate.MissionFlight}})
	if err != nil {
		t.Fatal(err)
	}
	if len(f[0].AwardCounts) != 8 || f[0].AwardCounts[2] != 3 || f[1].AwardCounts[0] != 2 || f[1].AwardCounts[7] != 1 {
		t.Fatal("primary statistics and stored wingman counters were confused")
	}
	f[0].AwardCounts[2] = 9
	if p.Statistics.AwardCounts[2] != 3 {
		t.Fatal("session award view aliases saved statistics")
	}
}

func TestOutfitTransferBlinkRegionsAndLifecycle(t *testing.T) {
	v := outfitTestView(t)
	_, _ = v.Handle(outfitPanelAction(1, "P", false, false))
	_, _ = v.Handle(outfitPanelAction(2, "", false, false))
	before := append([][36]byte(nil), v.Rows...)
	for tick := 0; tick < 2*outfitTransferBlinkHalfTicks; tick++ {
		v.Tick()
	}
	if !v.blinkWhite {
		t.Fatal("source callback counter did not reach white phase")
	}
	r := v.transferRectangles()
	if len(r) != 2 || r[0] != [4]int{110, 11, 37, 32} || r[1] != [4]int{217, 11, 37, 32} {
		t.Fatalf("pilot blink regions: %v", r)
	}
	f := v.Frame()
	if f[11*320+110] != 15 || f[12*320+111] == 15 || f[11*320+148] == 15 {
		t.Fatal("blink must outline portraits, not fill them or mark aircraft")
	}
	for tick := 0; tick < outfitTransferBlinkHalfTicks; tick++ {
		v.Tick()
	}
	if v.blinkWhite || v.Frame()[11*320+110] == 15 {
		t.Fatal("off phase failed to restore section colour")
	}
	for i := range before {
		if before[i] != v.Rows[i] {
			t.Fatal("presentation changed aircraft state")
		}
	}
	_, _ = v.Handle(Actions{Cancel: true})
	if len(v.transferRectangles()) != 0 || v.Mode != "" {
		t.Fatal("cancel retained transfer highlight")
	}
	counter := v.blinkCounter
	v.Tick()
	if counter != v.blinkCounter {
		t.Fatal("idle screen advanced pending-transfer counter")
	}
	_, _ = v.Handle(outfitPanelAction(0, "D", false, false))
	_, _ = v.Handle(outfitPanelAction(4, "", false, false))
	r = v.transferRectangles()
	if len(r) != 2 || r[0] != [4]int{41, 11, 62, 32} || r[1] != [4]int{148, 107, 62, 32} {
		t.Fatalf("helicopter blink regions: %v", r)
	}
	_, _ = v.Handle(outfitPanelAction(4, "", true, false))
	if v.Mode != "" || len(v.transferRectangles()) != 0 {
		t.Fatal("completed duplication retained highlight")
	}
	v.Mode, v.blinkCounter = "helicopter", 255
	v.Tick()
	if v.blinkCounter != 0 || !v.blinkWhite {
		t.Fatal("original byte counter wrap changed the phase")
	}
}

func TestOutfitBlinkExcludesPrimaryPilotAndSameSource(t *testing.T) {
	v := outfitTestView(t)
	_, _ = v.Handle(outfitPanelAction(1, "P", false, false))
	_, _ = v.Handle(outfitPanelAction(0, "", false, false))
	if len(v.transferRectangles()) != 1 {
		t.Fatal("primary pilot became a valid swap destination")
	}
	_, _ = v.Handle(outfitPanelAction(1, "", false, false))
	if len(v.transferRectangles()) != 1 {
		t.Fatal("same source outlined twice")
	}
	_, _ = v.Handle(outfitPanelAction(1, "", true, false))
	if v.Mode != "" || len(v.transferRectangles()) != 0 {
		t.Fatal("same-source no-op did not end mode")
	}
}

func TestOutfitCampaignRestrictionDoesNotPreventSwapOrRetainedEquipment(t *testing.T) {
	v := outfitTestView(t)
	v.Mission.CampaignUnavailableAircraft = []int{0, -1, -1, 4}
	_, _ = v.Handle(outfitPanelAction(0, "H", false, false))
	_, err := v.Handle(outfitPanelAction(1, "", true, false))
	if err != nil || v.Rows[1][0] != 0 || v.Rows[0][0] != 1 {
		t.Fatalf("existing unavailable aircraft could not be swapped: %v", err)
	}
	_, _ = v.Handle(outfitPanelAction(4, "D", false, false))
	if v.Mode != "helicopter" || !v.Duplicate {
		t.Fatal("fourth aircraft restriction incorrectly applied to native three-entry duplication test")
	}
	_, _ = v.Handle(Actions{Cancel: true})
	v.Rows[2][3] = 4
	gamestate.RowPut(v.Rows[2][:], 12, 1)
	v.Mission.CampaignUnavailableSystems = []int{1, -1, -1, -1} // Definition ordinal1 is systemID4.
	if !v.duplicateDenied(2) {
		t.Fatal("positive restricted store was duplicable")
	}
	for _, quantity := range []int{0, -1} {
		gamestate.RowPut(v.Rows[2][:], 12, quantity)
		if v.duplicateDenied(2) {
			t.Fatal("nonpositive store incorrectly triggered availability predicate")
		}
	}
}

func TestOutfitPilotSwapPreservesNonidentitySourceMap(t *testing.T) {
	v := outfitTestView(t)
	v.Flight[1], v.Flight[4] = v.Flight[4], v.Flight[1]
	before := append([]gamestate.FlightPilot(nil), v.Flight...)
	_, _ = v.Handle(outfitPanelAction(1, "P", false, false))
	_, err := v.Handle(outfitPanelAction(4, "", true, false))
	if err != nil {
		t.Fatal(err)
	}
	if v.Flight[1].SourceIndex != before[1].SourceIndex || v.Flight[4].SourceIndex != before[4].SourceIndex || v.Flight[1].Raw[0] != byte(before[1].SourceIndex) {
		t.Fatal("pilot swap moved assignment source identity")
	}
	stored, err := gamestate.SquadronFromFlight(v.Flight)
	if err != nil || stored[before[1].SourceIndex-1].Name != before[4].Name || stored[before[4].SourceIndex-1].Name != before[1].Name {
		t.Fatalf("source-backed copyback permuted identities twice: %v", err)
	}
}

func TestOutfitReturnRemainsLocalPermissive(t *testing.T) {
	v := outfitTestView(t)
	gamestate.RowPut(v.Rows[0][:], 23, 1)
	gamestate.RowPut(v.Rows[0][:], 25, 1000)
	_, _ = v.Handle(outfitPanelAction(1, "P", false, false))
	result, err := v.Handle(Actions{X: 240, Y: 180, Inside: true, Selector1: true})
	if err != nil || result.Kind != "return" || v.Mode != "" || len(v.transferRectangles()) != 0 {
		t.Fatalf("local RETURN should not invent capacity validation: %v %#v", err, result)
	}
}

func TestOutfitSourceTransferPreviewsWhenRequested(t *testing.T) {
	output := os.Getenv("GS2000_OUTFIT_PREVIEW_OUTPUT")
	if output == "" {
		t.Skip("explicit new preview output required")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("preview output must not exist")
	}
	a, err := assetdata.LoadMissionAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	p := &gamestate.Pilot{ID: "synthetic-preview", Name: "Test Pilot Name", Rank: gamestate.RankCaptain, LegacyRaw: make([]byte, 290)}
	rng := rand.New(rand.NewSource(1))
	p.SquadronPilots, err = gamestate.InitializeSquadron(p, []*gamestate.Pilot{p}, a.Preflight.SquadronNames, rng.Intn)
	if err != nil {
		t.Fatal(err)
	}
	m, err := gamestate.GenerateMission(a.Generation, gamestate.MissionInputs{MissionType: gamestate.MissionFlight, Theatre: gamestate.TheatreCentralEurope, Rank: gamestate.RankCaptain, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	f, err := gamestate.InitFlightPilots(p, m)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := gamestate.PrepareReadinessRows(a, m)
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewOutfitView(a, m, f, rows)
	if err != nil {
		t.Fatal(err)
	}
	capture := func(name string) {
		t.Helper()
		meta := map[string]any{"mode": v.Mode, "source": v.Source, "destination": v.Selected, "rectangles": v.transferRectangles(), "counter": v.blinkCounter, "white": v.blinkWhite, "evidence_level": "headless port rendering from source assets with explicitly synthetic fixture; no new original-game observation"}
		if err := writeIndexedFrame(filepath.Join(output, name+".png"), v.Frame(), a.Palettes["FIELD.PAL"], os.Getenv("GS2000_TEST_MISSIONS"), meta); err != nil {
			t.Fatal(err)
		}
	}
	capture("idle")
	_, _ = v.Handle(outfitPanelAction(1, "P", false, false))
	_, _ = v.Handle(outfitPanelAction(4, "", false, false))
	for !v.blinkWhite {
		v.Tick()
	}
	capture("pilot-white")
	for v.blinkWhite {
		v.Tick()
	}
	capture("pilot-normal")
	_, _ = v.Handle(Actions{Cancel: true})
	capture("cancelled")
	_, _ = v.Handle(outfitPanelAction(0, "H", false, false))
	_, _ = v.Handle(outfitPanelAction(2, "", false, false))
	for !v.blinkWhite {
		v.Tick()
	}
	capture("helicopter-white")
	_, _ = v.Handle(Actions{Cancel: true})
	_, _ = v.Handle(outfitPanelAction(0, "D", false, false))
	_, _ = v.Handle(outfitPanelAction(4, "", false, false))
	capture("duplicate-white")
}
