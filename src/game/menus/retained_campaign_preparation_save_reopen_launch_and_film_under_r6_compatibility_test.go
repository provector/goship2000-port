package menus

import (
	"encoding/json"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"testing"
)

func TestRetainedCampaignPreparationSaveReopenLaunchAndFilmUnderR6Compatibility(t *testing.T) {
	retainedCampaignProfileContract(t, replay.R6Compatibility, "GS2000_R6_OUTPUT", "r6-gulf-private")
}

func TestRetainedCampaignPreparationSaveReopenLaunchAndFilmUnderR8Compatibility(t *testing.T) {
	retainedCampaignProfileContract(t, replay.R8Compatibility, "GS2000_R8_OUTPUT", "r8-gulf-private")
}

func TestRetainedCampaignPreparationSaveReopenLaunchAndFilmUnderR83Compatibility(t *testing.T) {
	retainedCampaignProfileContract(t, replay.R83Compatibility, "GS2000_R83_OUTPUT", "r83-ordinary-campaign")
}

// Exercise each historical constructor and the current normal launch on
// separate fresh copies of the same complete retained preparation.
func retainedCampaignProfileContract(t *testing.T, profile, outputEnv, title string) {
	path := os.Getenv("GS2000_TEST_R6_CAMPAIGN")
	if path == "" {
		t.Skip("owned exact retained Gulf input required")
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var pilot gamestate.Pilot
	if e = json.Unmarshal(raw, &pilot); e != nil {
		t.Fatal(e)
	}
	c := authoredUIController(t)
	c.simulationProfile = profile
	if profile == replay.R86Compatibility {
		c.simulationProfile = "" // Exercise the ordinary current launch, not a replay constructor.
	}
	state := c.Store.State()
	state.Slots[0] = &pilot
	state.SelectedPilotID = pilot.ID
	if e = c.Store.Commit(state); e != nil {
		t.Fatal(e)
	}
	c.RefreshOffice()
	seed := int64(1)
	c.MissionSeed = &seed
	c.Runtime.DisableEnemyAI = true
	c.Runtime.DisableWingmenAI = true
	readyMission(t, c)
	m := c.Mission.Data
	if m.PlacementPlan == nil || len(m.Objects) != 80 || m.Inputs.PlacementPolicy != gamestate.RequiredFARPPlacementV1 {
		t.Fatal("normal preparation lost plan")
	}
	objectHash := m.ObjectHash
	copyPilot := c.Store.State().SelectedPilot()
	if e = campaignSuspend(copyPilot, c.Mission); e != nil {
		t.Fatal(e)
	}
	state = c.Store.State()
	state.Slots[0] = copyPilot
	if e = c.Store.Commit(state); e != nil {
		t.Fatal(e)
	}
	store, e := gamestate.OpenSaveStore(c.Store.Directory, "")
	if e != nil {
		t.Fatal(e)
	}
	next, e := NewFrontEnd(c.Office.Assets, store)
	if e != nil {
		t.Fatal(e)
	}
	next.simulationProfile = c.simulationProfile
	seed = 99
	next.MissionSeed = &seed
	next.Runtime.DisableEnemyAI = true
	next.Runtime.DisableWingmenAI = true
	next.FilmDirectory = t.TempDir()
	readyMission(t, next)
	if next.Mission.Data.ObjectHash != objectHash || next.Mission.Data.PlacementPlan == nil {
		t.Fatal("reopen regenerated repaired layout")
	}
	if next.simulationProfile != "" {
		// An explicit historical constructor requires the frozen launch bank;
		// the ordinary current launch obtains this at its normal boundary.
		switches, err := gamestate.LaunchSwitches(next.Mission.Data, next.Mission.Settings, next.Office.Assets.Missions.Generation.Tables.Initialization.CPGMapping)
		if err != nil {
			t.Fatal(err)
		}
		next.enemyLaunchRules, e = freezePreparedEnemyRules(next.Office.Assets.Missions, next.Mission.Data, switches, next.Runtime.DisableEnemyAI)
		if e != nil {
			t.Fatal(e)
		}
	}
	startRendering(t, next)
	jFinishFade(t, next)
	if !next.Rendering.Initialized.Stores.VictimWingStores || next.Mission.ResultLaunch == nil {
		t.Fatal("R6 launch/result owner", next.Mission.ResultUnavailable)
	}
	wantR8 := profile == replay.R8Compatibility || profile == replay.R83Compatibility || profile == replay.R86Compatibility
	if next.Rendering.Flight.State.CorrectedAvoidance != (profile == replay.R83Compatibility || profile == replay.R86Compatibility) {
		t.Fatal("wrong player avoidance interpretation", profile)
	}
	if next.Rendering.r8Mechanics != wantR8 || next.Rendering.Initialized.Stores.VictimCriticalProficiency != wantR8 {
		t.Fatal("wrong critical/target interpretation", profile)
	}
	before, _ := json.Marshal(next.Store.State().SelectedPilot())
	for i := 0; i < 128; i++ {
		if e = next.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
	}
	if next.Rendering.filmRecorder.operationCount() <= 80 {
		t.Fatalf("short witness requires >80 operations for retained Resume helper: steps%d operations%d", next.Rendering.Flight.Steps, next.Rendering.filmRecorder.operationCount())
	}
	finishAndReplay(t, next, before, title, profile)
	f, e := next.Library.Store.Load(next.Library.Entries[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	q, e := NewFilmPlayback(next.Office.Assets, f, next.Runtime)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 2 && !q.Done; i++ {
		if e = q.Step(); e != nil {
			t.Fatal(e)
		}
	}
	if e = q.Resume(); e != nil || q.Engine.Store != nil {
		t.Fatal("isolated layout Resume", e)
	}
	if q.Engine.Mission.Data.ObjectHash != objectHash || q.Engine.Mission.Data.Inputs.PlacementPolicy != gamestate.RequiredFARPPlacementV1 {
		t.Fatal("recorded plan lost")
	}
	if out := os.Getenv(outputEnv); out != "" {
		b, e := replay.Marshal(f)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(out, title+".g2r"), b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	unchanged, e := os.ReadFile(path)
	if e != nil || string(raw) != string(unchanged) {
		t.Fatal("retained input changed")
	}
}
