//go:build !headless

package menus

import (
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

// Optional real-window fixture. The external PID-verified helper owns every
// action after setup. No original game or personal save is opened.
func TestConnectedScenarioForWindow(t *testing.T) {
	out, name := os.Getenv("GS2000_N4_NATIVE"), os.Getenv("GS2000_N4_SCENARIO")
	if out == "" {
		t.Skip("fresh disposable connected output required")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	scale, err := strconv.Atoi(os.Getenv("GS2000_N4_SCALE"))
	if err != nil || scale != 1 && scale != 2 {
		t.Fatal("scale1/2 required")
	}
	a, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	store, err := gamestate.OpenSaveStore(filepath.Join(out, "saves"), "")
	if err != nil {
		t.Fatal(err)
	}
	pilot, err := gamestate.NewPilot()
	if err != nil {
		t.Fatal(err)
	}
	pilot.Name = "N4 disposable scenario"
	saved := store.State()
	saved.Slots[0], saved.SelectedPilotID = pilot, pilot.ID
	if err := store.Commit(saved); err != nil {
		t.Fatal(err)
	}
	c, err := NewFrontEnd(a, store)
	if err != nil {
		t.Fatal(err)
	}
	options := configuration.Defaults()
	options.SimulationRenderScale = scale
	c.Runtime = options
	meta, err := c.ConfigureDevelopmentScenario(name)
	if err != nil {
		t.Fatal(err)
	}
	metaBefore := *meta
	p, r := c.Rendering, c.Rendering.Flight
	before := store.State()
	write := func(file string, value any) {
		t.Helper()
		raw, e := json.MarshalIndent(value, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(out, file), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("fixture.json", metaBefore)
	if err := RunOfficeWindow(c.Office, WindowOptions{Runtime: options, FrontEnd: c, Development: true, Width: 960, Height: 720, TickRate: 70, SmokeFrames: 4200, CaptureDirectory: filepath.Join(out, "checkpoints")}); err != nil {
		t.Fatal(err)
	}
	if !c.ExitRequested || c.Screen != ScreenQuit {
		t.Fatal("native helper did not complete owned quit", c.Screen)
	}
	checks := map[string]bool{"real_window_exit": true, "genuine_preparation": true, "initially_paused": metaBefore.Paused}
	if c.previousScreen != ScreenOffice {
		pilot := store.State().SelectedPilot()
		if pilot == nil || pilot.Status != gamestate.StatusActive || pilot.MissionType != gamestate.MissionCampaign || pilot.Campaign == nil || (c.previousScreen != ScreenBriefing && c.previousScreen != ScreenInterstitial) {
			t.Fatal("owned input did not reach the committed office/Campaign successor", c.previousScreen)
		}
	}
	final := store.State().SelectedPilot()
	if reflect.DeepEqual(before.SelectedPilot(), final) {
		t.Fatal("completed result did not publish any source career facts")
	}
	if rec, err := store.RecoveryCandidate(); err != nil || rec == nil {
		t.Fatal("committed Phoenix missing", err)
	}
	checks["real_controller_commit_and_phoenix"] = true
	if p.Initialized.Sortie == nil || p.Initialized.Sortie.Result == nil {
		t.Fatal("no actual termination fact")
	}
	checks["source_termination_frozen"] = true
	reopened, err := gamestate.OpenSaveStore(store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.State(), reopened.State()) {
		t.Fatal("restart canonical state changed")
	}
	checks["restart_identical"] = true
	write("native-summary.json", map[string]any{"checks": checks, "initial": metaBefore, "final_scenario_stage": c.DevelopmentScenario, "damage_events": r.DamageEvents, "damage12": gamestate.PlayerDamage12(&r.Systems), "terminal": r.TerminalPresentation, "transient_end": p.Initialized.Sortie, "final_pilot": store.State().SelectedPilot(), "scope": "Real native OS input after labelled supplied scenario. No original-runtime or natural attack claim."})
}
