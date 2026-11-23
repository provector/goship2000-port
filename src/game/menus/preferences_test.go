package menus

import (
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPilotSettingsReturnSelectionAndRestart(t *testing.T) {
	c := missionController(t)
	next := c.Store.State()
	b, err := gamestate.NewPilot()
	if err != nil {
		t.Fatal(err)
	}
	b.Name = "Other Pilot"
	bSettings := gamestate.PreflightSettings{EnemyQuality: 2, Wind: 1}
	if err = gamestate.CopyPilotPreflight(b, bSettings); err != nil {
		t.Fatal(err)
	}
	next.Slots[1] = b
	if err = c.Store.Commit(next); err != nil {
		t.Fatal(err)
	}
	if err = c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	readyMission(t, c)
	s := c.Mission
	objects := s.Data.ObjectHash
	missionClick(t, c, 3)
	before := c.Store.State()
	missionClick(t, c, 1)
	want := s.Settings
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("switch draft committed before leaving settings")
	}
	if err = c.Handle(Actions{Quit: true}); err != nil {
		t.Fatal(err)
	}
	if err = c.Handle(Actions{Text: []rune{'N'}}); err != nil || c.Screen != ScreenSettings {
		t.Fatal("quit cancellation lost settings draft", err)
	}
	if err = c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenBriefing || c.Mission != s || objects != s.Data.ObjectHash || s.SettingsChanged {
		t.Fatal("settings commit replaced mission or left a dirty preference flag")
	}
	loaded, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	a := loaded.State().SelectedPilot()
	if a.Preflight == nil || *a.Preflight != want || *loaded.State().Slots[1].Preflight != bSettings {
		t.Fatal("selected/other pilot preferences did not survive restart independently")
	}
	// Actual roster posting branch, using a newly loaded controller/backend.
	c, err = NewFrontEnd(c.Office.Assets, loaded)
	if err != nil {
		t.Fatal(err)
	}
	c.Screen, c.Highlight = ScreenRoster, 1
	if err = c.Handle(Actions{Selector1: true}); err != nil || c.Screen != ScreenOffice {
		t.Fatal("other pilot posting failed", err)
	}
	readyMission(t, c)
	if c.Mission.Settings != bSettings {
		t.Fatal("selected pilot inherited another pilot's preferences")
	}
	if backup, err := loaded.RecoveryCandidate(); err != nil || backup != nil {
		t.Fatal("preference commit created gameplay backup", err)
	}
}

func TestCampaignSnapshotAndPilotPreferencesRemainSeparate(t *testing.T) {
	c := missionController(t)
	setPreflightPilot(t, c, gamestate.MissionCampaign)
	readyMission(t, c)
	c.Mission.Settings.EnemyQuality = 3
	c.Mission.SettingsChanged = true
	if err := c.campaignResult("suspend"); err != nil {
		t.Fatal(err)
	}
	next := c.Store.State()
	p := next.SelectedPilot()
	preferences := gamestate.PreflightSettings{EnemyQuality: 1, Visibility: 1}
	if err := gamestate.CopyPilotPreflight(p, preferences); err != nil {
		t.Fatal(err)
	}
	snapshot := p.Campaign.Suspended.Settings
	if err := c.Store.Commit(next); err != nil {
		t.Fatal(err)
	}
	var err error
	c.Store, err = gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	readyMission(t, c)
	s := c.Mission
	if s.Settings != snapshot || *c.Store.State().SelectedPilot().Preflight != preferences {
		t.Fatal("resume conflated mission snapshot and pilot preferences")
	}
	// This test isolates preference ownership, not equipment readiness. Opening
	// the local editor cannot silently mark the restored snapshot as edited.
	c.Screen = ScreenSettings
	s.Cockpit.Enter()
	before := c.Store.State()
	if err = c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, c.Store.State()) || s.Settings != snapshot {
		t.Fatal("unchanged snapshot return overwrote newer preferences")
	}
	c.Screen = ScreenSettings
	s.Settings.CPGControls = (s.Settings.CPGControls + 1) % 3
	s.SettingsChanged = true
	if err = c.Handle(Actions{Cancel: true}); err != nil {
		t.Fatal(err)
	}
	after := c.Store.State().SelectedPilot()
	if *after.Preflight != s.Settings || after.Campaign.Suspended.Settings != snapshot {
		t.Fatal("committed preference changed independent suspended snapshot")
	}
}

func TestSettingsSaveFailureCannotLeaveActionableMission(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	missionClick(t, c, 3)
	missionClick(t, c, 1)
	before := c.Store.State()
	if err := os.Mkdir(filepath.Join(c.Store.Directory, ".pending-commit.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := c.Handle(Actions{Cancel: true}); err == nil {
		t.Fatal("failed preference save was accepted")
	}
	if c.Mission != nil || c.Screen != ScreenOffice || c.missionSaveError == nil || !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("settings save failure published or retained an actionable session")
	}
	if err := c.Handle(Actions{Quit: true}); err != nil || c.Screen != ScreenQuit {
		t.Fatal("failed preferences save prevented quit", err)
	}
}
