package menus

import (
	"bytes"
	gamestate "gunship2000-assets/game/state"
	"math/rand"
	"reflect"
	"testing"
)

func setPreflightPilot(t *testing.T, c *FrontEnd, kind gamestate.MissionType) {
	t.Helper()
	next := c.Store.State()
	p := next.SelectedPilot()
	p.Rank = gamestate.RankCaptain
	p.MissionType = kind
	rng := rand.New(rand.NewSource(1))
	var err error
	p.SquadronPilots, err = gamestate.InitializeSquadron(p, next.Slots[:], c.Office.Assets.Missions.Preflight.SquadronNames, rng.Intn)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Store.Commit(next); err != nil {
		t.Fatal(err)
	}
	if err = c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
}

func TestPreflightConnectedSourceSession(t *testing.T) {
	c := missionController(t)
	setPreflightPilot(t, c, gamestate.MissionFlight)
	readyMission(t, c)
	s := c.Mission
	data := s.Data
	objects := data.ObjectHash
	draws := append([]gamestate.MissionDraw(nil), data.Draws...)
	if len(s.Flight) != 5 || len(s.ReadinessRows) != 5 {
		t.Fatal("flight not populated")
	}
	before := c.Store.State()
	missionClick(t, c, 1)
	if c.Screen != ScreenOutfit {
		t.Fatal(c.Screen)
	}
	missionClick(t, c, 1)
	if c.Screen != ScreenOrdnance {
		t.Fatal(c.Screen)
	}
	rows := append([][36]byte(nil), s.ReadinessRows...)
	act(t, c, Actions{Quit: true})
	act(t, c, Actions{Text: []rune{'N'}})
	if c.Screen != ScreenOrdnance || !reflect.DeepEqual(rows, s.ReadinessRows) {
		t.Fatal("quit changed equipment")
	}
	// ACCEPT uses source guards; no unrestricted Escape rollback is added.
	v := s.Ordnance
	done, err := v.Activate(len(v.Choices) - 1)
	if err != nil || !done {
		t.Fatal("source default rejected", err, v.Message)
	}
	if err = c.handleLoadout(Actions{}); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenOutfit {
		t.Fatal(c.Screen)
	}
	missionClick(t, c, 6)
	missionClick(t, c, 3)
	if c.Screen != ScreenSettings {
		t.Fatal(c.Screen, c.Message)
	}
	missionClick(t, c, 10)
	missionClick(t, c, 11)
	if s.Cockpit.Knobs != [2]byte{1, 1} {
		t.Fatal("knob toggles")
	}
	act(t, c, Actions{Cancel: true})
	if s.SecondaryControl != 1 || c.Screen != ScreenBriefing {
		t.Fatal("knob copyback")
	}
	missionClick(t, c, 3)
	if s.Cockpit.Knobs != [2]byte{} {
		t.Fatal("knobs did not reset")
	}
	if s.Data != data || data.ObjectHash != objects || !reflect.DeepEqual(data.Draws, draws) {
		t.Fatal("local return regenerated mission")
	}
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("browsing saved progression")
	}
	if backup, err := c.Store.RecoveryCandidate(); err != nil || backup != nil {
		t.Fatal("browsing made recovery backup", err)
	}
}

func TestCampaignFreshSuspendRestartResumeSource(t *testing.T) {
	c := missionController(t)
	setPreflightPilot(t, c, gamestate.MissionCampaign)
	readyMission(t, c)
	s := c.Mission
	if s.CampaignDraft == nil || s.Data.Inputs.MissionType != gamestate.MissionCampaign {
		t.Fatal("campaign substituted")
	}
	original := s.Data.ObjectHash
	rows := append([][36]byte(nil), s.ReadinessRows...)
	s.Settings.EnemyQuality = 3
	s.SettingsChanged = true
	if err := c.campaignResult("suspend"); err != nil {
		t.Fatal(err)
	}
	if c.Screen != ScreenOffice {
		t.Fatal(c.Screen)
	}
	store, err := gamestate.OpenManagedCareer(c.Store.Directory, "", t.TempDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	loaded := store.State().SelectedPilot()
	if loaded.Campaign == nil || loaded.Campaign.Phase != gamestate.CampaignSuspended {
		t.Fatal("suspension not persistent")
	}
	// Training can temporarily use another theatre; reselecting an existing
	// Campaign must restore its typed theatre, not the stale import byte.
	next := store.State()
	next.SelectedPilot().MissionType = gamestate.MissionTraining
	next.SelectedPilot().Theatre = gamestate.TheatreCentralEurope
	if err = store.Commit(next); err != nil {
		t.Fatal(err)
	}
	c.Store = store
	if err = c.RefreshOffice(); err != nil {
		t.Fatal(err)
	}
	if err = c.openMission(); err != nil {
		t.Fatal(err)
	}
	act(t, c, Actions{X: 100, Y: 92, Inside: true, PointerReset: true, Selector1: true, PointerPressed: true})
	if c.Store.State().SelectedPilot().Theatre != loaded.Campaign.Theatre {
		t.Fatal("Campaign chooser used stale legacy theatre")
	}
	readyMission(t, c)
	if c.Mission.Data.ObjectHash != original || !reflect.DeepEqual(c.Mission.ReadinessRows, rows) || c.Mission.Settings.EnemyQuality != 3 {
		t.Fatal("resume changed prepared mission")
	}
	if len(c.Mission.InitialAircraftIDs) != 5 {
		t.Fatal("default aircraft history missing")
	}
	// A second local settings visit never changes the stored suspended snapshot.
	saved := c.Store.State()
	missionClick(t, c, 1)
	missionClick(t, c, 6)
	if !reflect.DeepEqual(saved, c.Store.State()) {
		t.Fatal("local return saved campaign")
	}
	if err = c.campaignResult("abort"); err != nil {
		t.Fatal(err)
	}
	p := c.Store.State().SelectedPilot()
	if p.Campaign != nil || p.MissionType != gamestate.MissionFlight {
		t.Fatal("abort failed")
	}
	if backup, err := c.Store.RecoveryCandidate(); err != nil || backup != nil {
		t.Fatal("suspension created gameplay backup", err)
	}
}

func TestPreflightFinalBoundaryIsReadOnly(t *testing.T) {
	c := missionController(t)
	readyMission(t, c)
	before := c.Store.State()
	data := c.Mission.Data
	final, err := finalPreflight(c.Mission)
	if err != nil {
		t.Fatal(err)
	}
	if final.Mission != data || !bytes.Equal(final.Aircraft[0][:], c.Mission.ReadinessRows[0][:]) {
		t.Fatal("final configuration copied wrong data")
	}
	final.Aircraft[0][0] ^= 1
	if final.Aircraft[0][0] == c.Mission.ReadinessRows[0][0] {
		t.Fatal("final aircraft aliases live rows")
	}
	if !reflect.DeepEqual(before, c.Store.State()) {
		t.Fatal("validation wrote save")
	}
}
