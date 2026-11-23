package menus

import (
	"bytes"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"testing"
)

func ordnanceTestView(t *testing.T, rank gamestate.Rank) *OrdnanceView {
	t.Helper()
	a := &assetdata.MissionAssets{Preflight: &assetdata.PreflightAssets{Ships: map[int]assetdata.ShipPresentation{}, SystemOrder: []int{7, 16, 18, 19}, Text: map[int]string{}}, AircraftDefinitions: map[int][]byte{}, WeaponDefinitions: map[int][]byte{}, Images: map[string][]byte{}, Fonts: map[int]*assetdata.BitmapFont{}}
	for id := 0; id < 8; id++ {
		sh := make([]byte, 252)
		sh[0], sh[3], sh[4], sh[8], sh[9] = byte(id), 16, 2, 3, 2
		gamestate.RowPut(sh, 10, 10000)
		gamestate.RowPut(sh, 12, 1000)
		copy(sh[38:], "TEST AIRCRAFT")
		for j := 0; j < 3; j++ {
			copy(sh[0x50+17*j:], "STATION")
		}
		if id == 1 {
			sh[5] = 7
		}
		a.AircraftDefinitions[id] = sh
		a.Preflight.Ships[id] = assetdata.ShipPresentation{Aircraft: id, Fits: []assetdata.ShipFit{{7, 5, 0, 1}, {7, 10, 1, 2}, {16, 1, 3, -1}, {18, 1000, 3, -1}, {19, 30, 3, -1}}}
		a.Images["SHIP_"+string([]byte{'0', byte('0' + id)})+".PIC"] = make([]byte, 64000)
	}
	for id, name := range map[int]string{7: "ROCKETS", 16: "CANNON", 18: "FUEL", 19: "CHAFF/FLARES"} {
		d := make([]byte, 214)
		d[0] = byte(id)
		if id == 7 {
			d[1] = 1
		}
		copy(d[17:], name)
		copy(d[26:], name)
		switch id {
		case 7:
			gamestate.RowPut(d, 13, 20)
		case 16:
			gamestate.RowPut(d, 2, 1200)
			gamestate.RowPut(d, 4, 2)
			gamestate.RowPut(d, 13, 15)
		case 19:
			gamestate.RowPut(d, 4, 0x100)
		}
		a.WeaponDefinitions[id] = d
	}
	a.Images["TARMC_00.PIC"], a.Images["PILOT.PIC"] = make([]byte, 64000), make([]byte, 64000)
	m := &gamestate.MissionData{SourceWorld: &assetdata.MissionWorld{THHD: make([]byte, 54)}, Conditions: gamestate.MissionConditions{Temperature: 45}}
	r, err := gamestate.InitializeReadinessRow(a, m, 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewOrdnanceView(a, m, []gamestate.FlightPilot{{Name: "TEST", Callsign: "VIPER"}}, [][36]byte{r}, 0, rank)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func ordnanceChoice(t *testing.T, v *OrdnanceView, action string, system int) int {
	t.Helper()
	for i, c := range v.Choices {
		if c.Action == action && (system < 0 || c.System == system) {
			return i
		}
	}
	t.Fatalf("missing%s system%d: %+v", action, system, v.Choices)
	return -1
}

func ordnanceActivate(t *testing.T, v *OrdnanceView, action string, system int) {
	t.Helper()
	if _, err := v.Activate(ordnanceChoice(t, v, action, system)); err != nil {
		t.Fatal(err)
	}
}

func TestOrdnanceSourceQuantityStepsAndNoLocalRollback(t *testing.T) {
	v := ordnanceTestView(t, gamestate.RankCaptain)
	before := v.Rows[0]
	ordnanceActivate(t, v, "quantity_menu", 18)
	if len(v.Choices) != 6 {
		t.Fatal("quantity caption, four exact fuel choices plus return required")
	}
	if v.Choices[3].Value != 900 {
		t.Fatalf("source fuel step wrong: %+v", v.Choices)
	}
	if _, err := v.Activate(3); err != nil {
		t.Fatal(err)
	}
	if gamestate.RowWord(v.Working[:], 29) != 900 {
		t.Fatal("fuel edit missing")
	}
	if v.Rows[0] != before {
		t.Fatal("quantity edit escaped working row before ACCEPT")
	}
	if _, err := v.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if len(v.stack) != 2 || gamestate.RowWord(v.Working[:], 29) != 900 {
		t.Fatal("Escape incorrectly rolled back/unwound ordnance")
	}
	ordnanceActivate(t, v, "back", -1)
	ordnanceActivate(t, v, "back", -1)
	if !v.Done || gamestate.RowWord(v.Rows[0][:], 29) != 900 {
		t.Fatal("ACCEPT did not copy working row")
	}
}

func TestOrdnanceMountRemovalAndOrderedCommit(t *testing.T) {
	v := ordnanceTestView(t, gamestate.RankCaptain)
	ordnanceActivate(t, v, "fits", 7)
	ordnanceActivate(t, v, "mount", 7)
	if v.Working[3] != 7 || gamestate.RowWord(v.Working[:], 12) != 10 {
		t.Fatal("source multiplied mount count missing")
	}
	if gamestate.RowWord(v.Working[:], 25) != 1380 {
		t.Fatalf("load sum mismatch%d", gamestate.RowWord(v.Working[:], 25))
	}
	ordnanceActivate(t, v, "remove", 7)
	if v.Working[3] != 255 || v.Working[18] != 255 || gamestate.RowWord(v.Working[:], 12) != 0 {
		t.Fatal("remove sentinel/count mismatch")
	}
}

func TestOrdnanceAvailabilityRankAndNoDuplicateOptionLists(t *testing.T) {
	v := ordnanceTestView(t, gamestate.RankWOC)
	seen := map[int]bool{}
	for _, c := range v.Choices {
		if c.Action == "fits" || c.Action == "quantity_menu" {
			if seen[c.System] {
				t.Fatal("system appears in multiple incompatible menus")
			}
			seen[c.System] = true
		}
	}
	// Restrictions use definition-array ordinal0 here, not system ID7.
	v.Mission.CampaignUnavailableSystems = []int{0}
	v.rebuild()
	for _, c := range v.Choices {
		if c.Action == "fits" && c.System == 7 {
			t.Fatal("Campaign unavailable weapon remained selectable")
		}
	}
	ordnanceActivate(t, v, "aircraft", -1)
	found := false
	for i, c := range v.Choices {
		if c.Action == "replace" && c.Value == 1 {
			found = true
			if c.Enabled {
				t.Fatal("rank gate missing")
			}
			v.Activate(i)
		}
	}
	if !found || v.Message != "" || v.Working[0] != 0 {
		t.Fatal("rank denial incorrectly hidden or applied")
	}
	v.Mission.CampaignUnavailableAircraft = []int{1}
	v.rebuild()
	for _, c := range v.Choices {
		if c.Action == "replace" && c.Value == 1 {
			t.Fatal("Campaign unavailable aircraft not omitted")
		}
	}
}

func TestOrdnanceReplacementResetsLiveRowAndGuard(t *testing.T) {
	v := ordnanceTestView(t, gamestate.RankCaptain)
	gamestate.RowPut(v.Working[:], 29, 0)
	if _, err := v.Activate(ordnanceChoice(t, v, "back", -1)); err != nil {
		t.Fatal(err)
	}
	if v.Done || v.Message == "" {
		t.Fatal("empty-fuel top ACCEPT allowed")
	}
	ordnanceActivate(t, v, "aircraft", -1)
	for i, c := range v.Choices {
		if c.Action == "replace" && c.Value == 1 {
			v.Activate(i)
			break
		}
	}
	if v.Rows[0][0] != 1 || v.Working[0] != 1 || gamestate.RowWord(v.Working[:], 29) != 1000 || gamestate.RowWord(v.Working[:], 21) != 1200 {
		t.Fatal("replacement not immediate full source initialization")
	}
	gamestate.RowPut(v.Working[:], 29, 0)
	ordnanceActivate(t, v, "back", -1)
	if len(v.stack) != 1 {
		t.Fatal("nested return blocked by top-level fuel check")
	}
}

func TestOrdnanceHoverPreviewIsReadOnlyAndSceneRemap(t *testing.T) {
	v := ordnanceTestView(t, gamestate.RankCaptain)
	before := v.Working
	rows := v.Rows[0]
	v.FrameForHit(ordnanceChoice(t, v, "fits", 7))
	if v.Working != before || v.Rows[0] != rows {
		t.Fatal("hover changed selected loadout")
	}
	pix := v.Assets.Images["SHIP_00.PIC"]
	copy(pix, []byte{68, 69, 79, 80, 191, 192, 223, 224})
	got := assetdata.PreflightShipPixels(v.Assets, 0, 4)
	if !bytes.Equal(got[:8], []byte{68, 232, 250, 80, 191, 224, 255, 224}) {
		t.Fatalf("remap %v", got[:8])
	}
	if !bytes.Equal(pix[:8], []byte{68, 69, 79, 80, 191, 192, 223, 224}) {
		t.Fatal("source indices mutated")
	}
}

func TestOrdnanceHoverDoesNotDrawPortraitTilesOrExtraAtlasColumns(t *testing.T) {
	v := ordnanceTestView(t, gamestate.RankCaptain)
	for i := range v.Assets.Images["TARMC_00.PIC"] {
		v.Assets.Images["TARMC_00.PIC"][i] = 7
		v.Assets.Images["PILOT.PIC"][i] = 254
	}
	// An atlas edge outside S019a's source rectangle must never become a
	// vertical line inside the viewport. These values are synthetic witnesses.
	for y := 0; y < 114; y++ {
		v.Assets.Images["TARMC_00.PIC"][320*y+219] = 252
		v.Assets.Images["TARMC_00.PIC"][320*y+220] = 253
	}
	for station := 0; station < 3; station++ {
		for anchor := 0; anchor < 2; anchor++ {
			at := 14 + 8*station + 4*anchor
			gamestate.RowPut(v.definition(), at, 30+station*50+anchor*20)
			gamestate.RowPut(v.definition(), at+2, 40)
		}
	}
	before, live := v.Working, v.Rows[0]
	plain := v.FrameForHit(-1)
	ordnanceActivate(t, v, "fits", 7)
	for i, c := range v.Choices {
		if c.Action != "mount" {
			continue
		}
		frame := v.FrameForHit(i)
		for y := 3; y < 117; y++ {
			for x := 3; x < 222; x++ {
				if got := frame[y*320+x]; got != 7 {
					t.Fatalf("hovered station%d inserted non-art index%d at%d,%d", c.Station, got, x, y)
				}
			}
			if frame[y*320+222] != 9 {
				t.Fatal("right viewport border replaced by a source-atlas column")
			}
		}
	}
	if v.Working != before || v.Rows[0] != live {
		t.Fatal("preview altered working or accepted equipment")
	}
	ordnanceActivate(t, v, "back", -1)
	if !bytes.Equal(plain, v.FrameForHit(-1)) {
		t.Fatal("leaving preview did not restore committed presentation")
	}
}

func TestOrdnanceCargoAndSquadOptionsToggleRemoval(t *testing.T) {
	v := ordnanceTestView(t, gamestate.RankCaptain)
	for _, id := range []int{20, 21} {
		d := make([]byte, 214)
		flags := 0x4000
		name := "CARGO"
		if id == 20 {
			flags |= 0x100
			name = "SQUAD"
		}
		gamestate.RowPut(d, 4, flags)
		copy(d[26:], name)
		v.Assets.WeaponDefinitions[id] = d
		s := v.ship()
		s.Fits = append(s.Fits, assetdata.ShipFit{id, 1, 2, -1})
		v.Assets.Preflight.Ships[0] = s
	}
	v.rebuild()
	live := v.Rows[0]
	for _, id := range []int{20, 21, 20} {
		ordnanceActivate(t, v, "mount", id)
		want := byte(2)
		if id == 20 {
			want = 1
		}
		if v.Working[5] != byte(id) || v.Working[35] != want || gamestate.RowWord(v.Working[:], 16) != 1 {
			t.Fatalf("special system%d did not own the shared station", id)
		}
		ordnanceActivate(t, v, "remove", id)
		if v.Working[5] != 255 || v.Working[20] != 255 || v.Working[35] != 0 || gamestate.RowWord(v.Working[:], 16) != 0 {
			t.Fatalf("special system%d removal left quantity/ordinal/role state", id)
		}
	}
	ordnanceActivate(t, v, "mount", 20)
	ordnanceActivate(t, v, "mount", 21)
	if v.Working[5] != 21 || v.Working[35] != 2 || gamestate.RowWord(v.Working[:], 16) != 1 {
		t.Fatal("cargo did not replace squad on the shared station")
	}
	if v.Rows[0] != live {
		t.Fatal("special option changed accepted state before ACCEPT")
	}
	ordnanceActivate(t, v, "back", -1)
	if !v.Done || v.Rows[0] != v.Working {
		t.Fatal("special option did not copy back on ACCEPT")
	}
}

func TestOrdnanceSourceHoverViewportAndRestoration(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_PREFLIGHT")
	if dir == "" {
		t.Skip("source-backed prepared data required")
	}
	a, err := assetdata.LoadMissionAssets(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := &gamestate.MissionData{SourceWorld: &a.Generation.Worlds[0], Scene: 2, Conditions: gamestate.MissionConditions{Temperature: 68}}
	cases := 0
	for id := 0; id < 8; id++ {
		r, err := gamestate.InitializeReadinessRow(a, m, id)
		if err != nil {
			t.Fatal(err)
		}
		v, err := NewOrdnanceView(a, m, []gamestate.FlightPilot{{Name: "TEST", Callsign: "VIPER"}}, [][36]byte{r}, 0, gamestate.RankCaptain)
		if err != nil {
			t.Fatal(err)
		}
		plain := v.FrameForHit(-1)
		for ordinal, fit := range v.ship().Fits {
			if fit.Station < 0 || fit.Station > 2 {
				continue
			}
			// This main-level synthetic row has no S0716 marker callback. Its
			// candidate operand must not replace committed SHIP composition.
			want, err := preflightComposeShip(a, id, m.Scene, r)
			if err != nil {
				t.Fatal(err)
			}
			v.Choices = []OrdnanceChoice{{Action: "mount", Value: ordinal, System: fit.System, Station: fit.Station, Enabled: true}}
			got := v.FrameForHit(0)
			for y := 0; y < 114; y++ {
				if !bytes.Equal(got[(y+3)*320+3:(y+3)*320+222], want[y*320:y*320+219]) {
					t.Fatalf("aircraft%d fit%d altered source viewport row%d", id, ordinal, y)
				}
				if got[(y+3)*320+222] != 9 {
					t.Fatalf("aircraft%d fit%d damaged viewport right frame", id, ordinal)
				}
			}
			if v.Working != r || v.Rows[0] != r {
				t.Fatal("source hover changed working or accepted aircraft")
			}
			cases++
		}
		v.rebuild()
		if !bytes.Equal(plain, v.FrameForHit(-1)) {
			t.Fatalf("aircraft%d retained a hover patch", id)
		}
		if output := os.Getenv("GS2000_TEST_ORDNANCE_OUTPUT"); output != "" && (id == 0 || id == 3) {
			ordnanceActivate(t, v, "fits", 7)
			hover := ordnanceChoice(t, v, "mount", 7)
			for _, scene := range []int{0, 2} {
				m.Scene = scene
				meta := map[string]any{"aircraft": id, "scene": m.Scene, "tarmac": fmt.Sprintf("TARMC_%02d.PIC", scene), "hover": v.Choices[hover], "working": v.Working, "scope": "committed source equipment plus source hover markers; no original-game runtime observation"}
				if err := writeIndexedFrame(filepath.Join(output, fmt.Sprintf("aircraft%d-scene%d-hover.png", id, scene)), v.FrameForHit(hover), a.Palettes["FIELD.PAL"], dir, meta); err != nil {
					t.Fatal(err)
				}
			}
			m.Scene = 2
		}
	}
	if cases != 178 {
		t.Fatalf("checked%d legal station previews, want178", cases)
	}
}

func TestOrdnanceQuantityBoundsCountermeasureAndCannon(t *testing.T) {
	v := ordnanceTestView(t, gamestate.RankCaptain)
	ordnanceActivate(t, v, "quantity_menu", 19)
	if got := []int{v.Choices[1].Value, v.Choices[2].Value, v.Choices[3].Value, v.Choices[4].Value}; !equalInts(got, []int{60, 33, 27, 0}) {
		t.Fatalf("countermeasure choices %v", got)
	}
	v.Activate(2)
	if gamestate.RowWord(v.Working[:], 31) != 33 || gamestate.RowWord(v.Working[:], 33) != 27 {
		t.Fatal("chaff/flares complement not maintained")
	}
	ordnanceActivate(t, v, "back", -1)
	ordnanceActivate(t, v, "quantity_menu", 16)
	if v.Choices[1].Value != 1200 || v.Choices[2].Value != 1200 || v.Choices[3].Value != 1100 {
		t.Fatalf("cannon limit/step %+v", v.Choices)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestOrdnancePreparedSourceCoverage(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_PREFLIGHT")
	if dir == "" {
		t.Skip("setGS2000_TEST_PREFLIGHT for source-backed prepared data")
	}
	a, err := assetdata.LoadMissionAssets(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Preflight.SquadronNames) != 74 || len(a.Preflight.SystemOrder) != 28 {
		t.Fatal("source name/definition-order tables missing")
	}
	m := &gamestate.MissionData{SourceWorld: &a.Generation.Worlds[0], Conditions: gamestate.MissionConditions{Temperature: 68}}
	fits, components := 0, 0
	stationCases := 0
	for id, s := range a.Preflight.Ships {
		fits += len(s.Fits)
		components += len(s.Components)
		r, err := gamestate.InitializeReadinessRow(a, m, id)
		if err != nil {
			t.Fatal(err)
		}
		for scene := 0; scene < 6; scene++ {
			if _, err := preflightComposeShip(a, id, scene, r); err != nil {
				t.Fatalf("aircraft%d scene%d: %v", id, scene, err)
			}
		}
		for ordinal, fit := range s.Fits {
			if fit.Station > 2 {
				continue
			}
			v, err := NewOrdnanceView(a, m, []gamestate.FlightPilot{{Rank: gamestate.RankCaptain}}, [][36]byte{r}, 0, gamestate.RankCaptain)
			if err != nil {
				t.Fatal(err)
			}
			for i, c := range v.Choices {
				if c.Action == "back" {
					if c.Rectangle != [4]int{236, 178, 78, 10} {
						t.Fatal("ACCEPT footer moved")
					}
					continue
				}
				if c.Rectangle[1] < 67 || c.Rectangle[1]+c.Rectangle[3] > 178 {
					t.Fatalf("aircraft%d menu row clipped/overlaps ACCEPT: %+v", id, c)
				}
				if c.Action != "heading" && v.Hit(c.Rectangle[0]+1, c.Rectangle[1]+1) != i {
					t.Fatalf("aircraft%d row%d cannot be activated", id, i)
				}
			}
			v.Choices = []OrdnanceChoice{{Action: "mount", Value: ordinal, Enabled: true}}
			if _, err = v.Activate(0); err != nil {
				t.Fatalf("aircraft%d fit%d: %v", id, ordinal, err)
			}
			want := int(int16(int(int8(a.AircraftDefinitions[id][9])) * fit.Quantity))
			if int(gamestate.RowWord(a.WeaponDefinitions[fit.System], 4))&0x4000 != 0 {
				want = 1
			}
			if int(gamestate.RowWord(v.Working[:], 12+2*fit.Station)) != want || v.Working[3+fit.Station] != byte(fit.System) || v.Working[18+fit.Station] != byte(ordinal) {
				t.Fatalf("aircraft%d fit%d transfer mismatch", id, ordinal)
			}
			if _, err = preflightComposeShip(a, id, 0, v.Working); err != nil {
				t.Fatalf("aircraft%d fit%d preview: %v", id, ordinal, err)
			}
			stationCases++
		}
	}
	if fits != 200 || components != 498 || stationCases != 178 {
		t.Fatalf("source coverage%d fits%d components", fits, components)
	}
}

func TestOrdnanceSourceLayoutAndDescriptionAreDataDriven(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_PREFLIGHT")
	if dir == "" {
		t.Skip("source-backed prepared data required")
	}
	a, err := assetdata.LoadMissionAssets(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := &gamestate.MissionData{SourceWorld: &a.Generation.Worlds[0], Conditions: gamestate.MissionConditions{Temperature: 68}}
	r, err := gamestate.InitializeReadinessRow(a, m, 0)
	if err != nil {
		t.Fatal(err)
	}
	v, err := NewOrdnanceView(a, m, []gamestate.FlightPilot{{Name: "TEST", Callsign: "VIPER"}}, [][36]byte{r}, 0, gamestate.RankCaptain)
	if err != nil {
		t.Fatal(err)
	}
	hover := ordnanceChoice(t, v, "fits", 7)
	before := v.Working
	frame := v.FrameForHit(hover)
	// Weapon7 is tile7 in source MENU0021, never a screenshot crop. Its whole
	// first row is unobscured by range/prose text at lower rows.
	if !bytes.Equal(frame[5*320+233:5*320+313], a.Images["MENU0021.PIC"][175*320:175*320+80]) {
		t.Fatal("hover did not use original weapon icon")
	}
	if frame[123*320+100] != 9 || frame[185*320+10] == 0 {
		t.Fatal("callsign bar/panel fill missing")
	}
	back := ordnanceChoice(t, v, "back", -1)
	rect := v.Choices[back].Rectangle
	if v.Hit(rect[0]+rect[2]/2, rect[1]+rect[3]/2) != back {
		t.Fatal("red ACCEPT cannot be activated")
	}
	if v.Hit(250, 30) != -1 {
		t.Fatal("description area accidentally activates a menu choice")
	}
	if v.Working != before {
		t.Fatal("drawing changed loadout")
	}
}

func TestOrdnanceAllSourceMenusFitAndRemainReachable(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_PREFLIGHT")
	if dir == "" {
		t.Skip("source-backed prepared data required")
	}
	a, err := assetdata.LoadMissionAssets(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := &gamestate.MissionData{SourceWorld: &a.Generation.Worlds[0], Conditions: gamestate.MissionConditions{Temperature: 68}}
	check := func(v *OrdnanceView) {
		t.Helper()
		for i, c := range v.Choices {
			if c.Action == "back" {
				if c.Rectangle != [4]int{236, 178, 78, 10} {
					t.Fatal("footer rectangle changed")
				}
				continue
			}
			if c.Rectangle[1] < 67 || c.Rectangle[1]+c.Rectangle[3] > 178 {
				t.Fatalf("aircraft%d menu%s clipped/overlapped footer: %+v", v.Working[0], v.stack[len(v.stack)-1].Kind, c)
			}
			if c.Action != "heading" && v.Hit(c.Rectangle[0]+1, c.Rectangle[1]+1) != i {
				t.Fatalf("unreachable row%d %+v", i, c)
			}
		}
	}
	for id := 0; id < 8; id++ {
		r, e := gamestate.InitializeReadinessRow(a, m, id)
		if e != nil {
			t.Fatal(e)
		}
		// Exercise both empty and populated main menuui. REMOVE WEAPONS appears
		// only in the populated state and changes the longest menu's height.
		for _, populated := range []bool{false, true} {
			v, e := NewOrdnanceView(a, m, []gamestate.FlightPilot{{Rank: gamestate.RankCaptain}}, [][36]byte{r}, 0, gamestate.RankCaptain)
			if e != nil {
				t.Fatal(e)
			}
			if populated {
				for _, fit := range v.ship().Fits {
					if fit.Station < 3 {
						v.Working[3+fit.Station] = byte(fit.System)
						gamestate.RowPut(v.Working[:], 12+2*fit.Station, int(int8(v.definition()[9]))*fit.Quantity)
					}
				}
				v.rebuild()
			}
			check(v)
			main := append([]OrdnanceChoice(nil), v.Choices...)
			for _, c := range main {
				if c.Action != "fits" && c.Action != "quantity_menu" && c.Action != "aircraft" && c.Action != "remove_menu" {
					continue
				}
				child := *v
				child.stack = append([]ordnanceMenu(nil), v.stack...)
				child.Choices = []OrdnanceChoice{c}
				if _, e = child.Activate(0); e != nil {
					t.Fatal(e)
				}
				check(&child)
			}
		}
	}
}
