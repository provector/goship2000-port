package menus

import (
	"encoding/binary"
	assetdata "gunship2000-assets/game/assets"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func missionController(t *testing.T) *FrontEnd {
	t.Helper()
	dir := os.Getenv("GS2000_TEST_MISSIONS")
	if dir == "" {
		t.Skip("set GS2000_TEST_MISSIONS for prepared mission integration")
	}
	t.Setenv("GS2000_TEST_MENUS", dir)
	c := sourceFrontEnd(t, true)
	var err error
	c.Office.Assets.Missions, err = assetdata.LoadMissionAssets(dir)
	if err != nil {
		t.Fatal(err)
	}
	seed := int64(1)
	c.MissionSeed = &seed
	return c
}

func missionClick(t *testing.T, c *FrontEnd, key int) {
	t.Helper()
	x, y, ok := missionActionPoint(c, key)
	if !ok {
		t.Fatalf("no action%d on%s", key, c.Screen)
	}
	act(t, c, Actions{X: x, Y: y, Inside: true, Selector1: true, PointerPressed: true})
	finishPaperMotion(t, c)
}

func readyMission(t *testing.T, c *FrontEnd) {
	t.Helper()
	if err := c.enterMission(); err != nil {
		t.Fatal(err)
	}
	if c.Mission == nil {
		t.Fatal(c.Message)
	}
	for i := 0; i < 36; i++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal(c.Screen)
	}
}

func TestMissionSessionReusePagesSettingsQuitAndDecline(t *testing.T) {
	c := missionController(t)
	before := c.Store.State()
	readyMission(t, c)
	s := c.Mission
	data := s.Data
	objectHash := data.ObjectHash
	draws := append([]gamestate.MissionDraw(nil), data.Draws...)
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("preparation saved progression")
	}
	for page := 0; page < 4; page++ {
		if c.Screen == ScreenBriefing {
			missionClick(t, c, 2)
		}
		if s.Page != page {
			t.Fatalf("page%d expected%d", s.Page, page)
		}
		f1, f2 := c.Frame(false), c.Frame(false)
		if !reflect.DeepEqual(f1, f2) {
			t.Fatal("a host frame rerolled paper content")
		}
		if page < 3 {
			missionClick(t, c, 3)
		}
	}
	act(t, c, Actions{Cancel: true})
	finishPaperMotion(t, c)
	if c.Screen != ScreenBriefing || s.Page != 3 {
		t.Fatal("paper cancel lost local return/page")
	}
	missionClick(t, c, 3)
	if c.Screen != ScreenSettings {
		t.Fatalf("settings blocked: %s %v %s", c.Screen, s.ReadinessError, c.Message)
	}
	initial := s.Settings
	missionClick(t, c, 1)
	if s.Settings.CPGControls != (initial.CPGControls+1)%3 {
		t.Fatal("CP/G did not advance once")
	}
	settings := s.Settings
	act(t, c, Actions{Quit: true})
	if c.Screen != ScreenQuit {
		t.Fatal(c.Screen)
	}
	act(t, c, Actions{Text: []rune{'n'}})
	if c.Screen != ScreenSettings || s.Settings != settings {
		t.Fatal("quit cancel lost settings")
	}
	missionClick(t, c, 9)
	if c.Screen != ScreenSettings || c.ExitRequested {
		t.Fatal("APU crossed execution boundary")
	}
	act(t, c, Actions{Cancel: true})
	if c.Screen != ScreenBriefing || c.Mission != s || s.Data != data || data.ObjectHash != objectHash || !reflect.DeepEqual(data.Draws, draws) {
		t.Fatal("round trip regenerated mission")
	}
	act(t, c, Actions{Cancel: true})
	if c.Screen != ScreenBriefing {
		t.Fatal("tent Escape incorrectly returned to office")
	}
	expectedReturn := cloneSaveForTest(before)
	expectedReturn.Revision++
	preferences := settings
	expectedReturn.SelectedPilot().Preflight = &preferences
	if !reflect.DeepEqual(expectedReturn, c.Store.State()) {
		t.Fatal("settings return must save only committed pilot preferences")
	}
	missionClick(t, c, 2)
	missionClick(t, c, 5)
	if c.Screen != ScreenOffice || c.Mission != nil {
		t.Fatal("Training decline did not return")
	}
	after := c.Store.State()
	p := after.SelectedPilot()
	if p.Preflight == nil || *p.Preflight != settings {
		t.Fatal("outer decline did not retain copied-back settings")
	}
	if binary.LittleEndian.Uint16(p.LegacyRaw[0x59:]) != binary.LittleEndian.Uint16(before.SelectedPilot().LegacyRaw[0x59:]) {
		t.Fatal("Training decline incurred a non-Training penalty")
	}
	if _, err := os.Stat(filepath.Join(c.Store.Directory, "pilots", p.ID+".backup.json")); !os.IsNotExist(err) {
		t.Fatal("browsing/decline created mission backup")
	}
	reopened, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil || reopened.State().SelectedPilot().Preflight == nil || *reopened.State().SelectedPilot().Preflight != settings {
		t.Fatal("settings save round trip", err)
	}
}

func TestMissionUnsupportedStatesLeaveOfficeUnchanged(t *testing.T) {
	for _, mission := range []gamestate.MissionType{gamestate.MissionCampaign, gamestate.MissionSimulated} {
		c := missionController(t)
		next := c.Store.State()
		next.SelectedPilot().MissionType = mission
		if mission == gamestate.MissionCampaign {
			next.SelectedPilot().LegacyRaw[0x4f] = 1
		}
		if err := c.Store.Commit(next); err != nil {
			t.Fatal(err)
		}
		if err := c.RefreshOffice(); err != nil {
			t.Fatal(err)
		}
		before := c.Store.State()
		if err := c.enterMission(); mission == gamestate.MissionCampaign && err == nil {
			t.Fatal("unproven retained Campaign accepted")
		}
		if c.Screen != ScreenOffice || c.Mission != nil || c.Message == "" || !reflect.DeepEqual(before, c.Store.State()) {
			t.Fatal("unsupported state mutated or substituted")
		}
	}
}

func TestPreflightPreferenceOwnershipAndRanges(t *testing.T) {
	p, _ := gamestate.NewPilot()
	p.Preflight = nil // Explicitly exercise imported/old-save fallback, not new-pilot defaults.
	for i, offset := range [7]int{0x5f, 0x5d, 0x5e, 0x60, 0x61, 0x62, 0x63} {
		p.LegacyRaw[offset] = byte(i % 2)
	}
	s, err := gamestate.PreflightFromPilot(p)
	if err != nil || s.Values() != [7]int{0, 1, 0, 1, 0, 1, 0} {
		t.Fatal(s, err)
	}
	p.Preflight = &gamestate.PreflightSettings{EnemyQuality: 3}
	loaded, err := gamestate.PreflightFromPilot(p)
	if err != nil || loaded.EnemyQuality != 3 {
		t.Fatal("raw witness overrode typed preference")
	}
	p.Preflight.CPGControls = 3
	if _, err := gamestate.PreflightFromPilot(p); err == nil {
		t.Fatal("invalid choice accepted")
	}
	p.Preflight = nil
	p.LegacyRaw = nil
	if _, err := gamestate.PreflightFromPilot(p); err == nil {
		t.Fatal("missing fields became fabricated defaults")
	}
}

func TestMissionInputLatchDoesNotCarryOpeningPress(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	x, y, _ := missionActionPoint(c, 2)
	l := inputLatch{}
	act(t, c, l.Read(Buttons{X: x, Y: y, Inside: true, Space: true}))
	finishPaperMotion(t, c)
	if c.Screen != ScreenPapers {
		t.Fatal(c.Screen)
	}
	act(t, c, l.Read(Buttons{X: x, Y: y, Inside: true, Space: true}))
	if c.Mission.Page != 0 {
		t.Fatal("held opening Space also advanced page")
	}
	act(t, c, l.Read(Buttons{X: x, Y: y, Inside: true}))
	act(t, c, l.Read(Buttons{X: x, Y: y, Inside: true, Space: true}))
	finishPaperMotion(t, c)
	if c.Mission.Page != 1 {
		t.Fatal("new Space press did not advance")
	}
}

func TestPreflightAllChoicesAndGlobalQuitKeepSession(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	s := c.Mission
	for _, screen := range []Screen{ScreenInterstitial, ScreenBriefing, ScreenPapers, ScreenSettings} {
		c.Screen = screen
		before := append([]byte(nil), c.Frame(false)...)
		act(t, c, Actions{Quit: true})
		act(t, c, Actions{Text: []rune{'n'}})
		if c.Screen != screen || c.Mission != s || !reflect.DeepEqual(c.Frame(false), before) {
			t.Fatalf("quit cancellation changed %s", screen)
		}
	}
	c.Screen = ScreenSettings
	initial := s.Settings
	for key, count := range []int{3, 4, 2, 2, 2, 2, 2} {
		for i := 1; i <= count; i++ {
			missionClick(t, c, key+1)
			if got := s.Settings.Values()[key]; got != (initial.Values()[key]+i)%count {
				t.Fatalf("control%d step%d got%d", key, i, got)
			}
		}
	}
	if s.Settings != initial || s.Data != c.Mission.Data {
		t.Fatal("switch cycle changed another input")
	}
}

func TestSingleDeclineOpaqueCounterAndScenarioHistory(t *testing.T) {
	// Check the outer save branch using actual Single generation, including
	// the source's conditional addition (2999 becomes3299, not a clamp3000).
	for _, tc := range []struct{ before, after int16 }{{-5, 295}, {2699, 2999}, {2999, 3299}, {3000, 3000}} {
		c := missionController(t)
		state := c.Store.State()
		p := state.SelectedPilot()
		p.MissionType = gamestate.MissionSingle
		p.LegacyRaw[0x5b] = 4
		binary.LittleEndian.PutUint16(p.LegacyRaw[0x59:], uint16(tc.before))
		if err := c.Store.Commit(state); err != nil {
			t.Fatal(err)
		}
		if err := c.RefreshOffice(); err != nil {
			t.Fatal(err)
		}
		readyMission(t, c)
		suffix, header := c.Mission.Data.SourceWorld.Suffix, c.Mission.Data.SourceWorld.SNHD[0]
		if err := c.declineMission(); err != nil {
			t.Fatal(err)
		}
		p = c.Store.State().SelectedPilot()
		if got := int16(binary.LittleEndian.Uint16(p.LegacyRaw[0x59:])); got != tc.after {
			t.Fatalf("before%d got%d want%d", tc.before, got, tc.after)
		}
		if p.LegacyRaw[0x5c] != 4 || p.LegacyRaw[0x5b] != byte(suffix) || p.LegacyRaw[0x58] != header {
			t.Fatal("source scenario history/header copy differs")
		}
		if _, err := os.Stat(filepath.Join(c.Store.Directory, "pilots", p.ID+".backup.json")); !os.IsNotExist(err) {
			t.Fatal("decline created a recovery backup")
		}
	}
}
