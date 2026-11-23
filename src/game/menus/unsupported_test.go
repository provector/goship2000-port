package menus

import (
	"bytes"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Fault-injected unsupported-owner negative, not an admitted launch/film: a
// CONTROL leader is deliberately corrupted after the valid recorder starts.
// No pose, goal, score or completed result is supplied. The incomplete spool is
// evidence only and must never replace Last Mission or acquire career facts.
func TestUnsupportedFormationPreservesCareerAndEvidence(t *testing.T) {
	a, _, l, j := retainedCampaign(t)
	c := missionController(t)
	c.Office.Assets, c.Menus, c.Runtime = a, a.Menus, l.Runtime
	saved := c.Store.State()
	saved.Slots[0], saved.SelectedPilotID = j.Start.PilotBefore, j.Start.PilotBefore.ID
	if e := c.Store.Commit(saved); e != nil {
		t.Fatal(e)
	}
	s, e := prepareMissionSession(a.Missions, j.Start.PilotBefore, l.Final.Mission.Inputs.Seed, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	final, e := cloneFilmFinal(&l.Final)
	if e != nil {
		t.Fatal(e)
	}
	final.Mission, final.Flight, final.Settings = s.Data, s.Flight, s.Settings
	s.Final, s.ReadinessRows = final, final.Aircraft
	// This negative predates recoverable route errors. Under U2.6 the same
	// order is replaced by hold; TestRecoverableWaypointCommitsAndReports
	// separately checks that the current result can still commit.
	c.simulationProfile = replay.U25Compatibility
	switches, err := gamestate.LaunchSwitches(s.Data, s.Settings, a.Missions.Generation.Tables.Initialization.CPGMapping)
	if err != nil {
		t.Fatal(err)
	}
	c.enemyLaunchRules, err = freezePreparedEnemyRules(a.Missions, s.Data, switches, c.Runtime.DisableEnemyAI)
	if err != nil {
		t.Fatal(err)
	}
	c.Mission, c.Screen = s, ScreenScene
	if e = c.constructSimulation(final); e != nil {
		t.Fatal(e)
	}
	c.beginFilmRecording(final)
	p, r := c.Rendering, c.Rendering.Flight
	if p.filmRecorder == nil {
		t.Fatal(c.Message)
	}
	before := filmJSON(c.Store.State().SelectedPilot())
	phoenix, e := c.Store.RecoveryCandidate()
	if e != nil {
		t.Fatal(e)
	}
	sentinel := []byte("owned predecessor pointer; failure must not publish any film\n")
	last := filepath.Join(c.FilmDirectory, "last.json")
	if e = os.WriteFile(last, sentinel, 0600); e != nil {
		t.Fatal(e)
	}
	p.Initialized.Entities[2].ControlRaw[1] = 1   // Formation flight consumes the leader slot; transit order 2 does not.
	p.Initialized.Entities[2].ControlRaw[5] = 127 // Explicit fault injection, never an ordinary launch default.
	stepsBefore := r.Steps
	r.Paused, p.Paused = false, false
	for n := 0; n < 40 && p.Initialized.FollowerLimits[2] == "" && r.Combat.RouteWarnings()[2] == ""; n++ {
		if e = c.AdvanceRendering(.12); e != nil {
			t.Fatal(e)
		}
	}
	if r.Systems.UnsupportedFollowerEmptyBurn&4 != 0 || p.Initialized.FollowerLimits[2] == "" {
		t.Fatalf("unknown leader did not retain its narrow guard: updates=%d..%d screen=%s actor kind=%#x active=%d order=%d leader=%d limits=%q routeWarning=%q fuel=%#x", stepsBefore, r.Steps, c.Screen, p.Initialized.Entities[2].Kind, p.Initialized.Entities[2].Participation, p.Initialized.Entities[2].ControlRaw[1], p.Initialized.Entities[2].ControlRaw[5], p.Initialized.FollowerLimits[2], r.Combat.RouteWarnings()[2], r.Systems.UnsupportedFollowerEmptyBurn)
	}
	if e = c.Handle(Actions{EndMission: true}); e != nil {
		t.Fatal(e)
	}
	if e = c.Handle(Actions{ConfirmQuit: true}); e != nil {
		t.Fatal(e)
	}
	if c.Debrief != nil || !bytes.Equal(before, filmJSON(c.Store.State().SelectedPilot())) {
		t.Fatal("unsupported owner invented career facts")
	}
	after, e := c.Store.RecoveryCandidate()
	if e != nil || !reflect.DeepEqual(phoenix, after) {
		t.Fatal("unsupported path changed Phoenix", e)
	}
	b, e := os.ReadFile(last)
	if e != nil || !bytes.Equal(b, sentinel) {
		t.Fatal("unsupported path replaced predecessor film")
	}
	spool, e := filepath.Glob(filepath.Join(c.FilmDirectory, "*.tmp"))
	if e != nil {
		t.Fatal(e)
	}
	// The recorder's owned spool location, not an assumed extension, is retained.
	if p.filmRecorder.Spool == nil {
		t.Fatal("failure lost temporary evidence", spool)
	}
	reopened, e := gamestate.OpenSaveStore(c.Store.Directory, "")
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(before, filmJSON(reopened.State().SelectedPilot())) {
		t.Fatal("restart awarded unsupported sortie")
	}
}
