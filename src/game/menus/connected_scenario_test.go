//go:build !headless

package menus

import (
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

// Optional real-window fixture. The external PID-verified helper owns every
// action after setup. No original game or personal save is opened.
func TestConnectedScenario(t *testing.T) {
	out, name := os.Getenv("GS2000_N3_NATIVE"), os.Getenv("GS2000_N3_SCENARIO")
	if out == "" {
		t.Skip("fresh disposable connected output required")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	scale, err := strconv.Atoi(os.Getenv("GS2000_N3_SCALE"))
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
	pilot.Name = "N3 disposable scenario"
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
	if name == "crash" {
		anchor := developmentScenarioController(t, name)
		c.simulationProfile = replay.V45Compatibility
		c.enemyLaunchRules = anchor.Rendering.Initialized.EnemyRules
		anchor.AbandonFilmRecording()
	}
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
	if err := RunOfficeWindow(c.Office, WindowOptions{Runtime: options, FrontEnd: c, Development: true, Width: 960, Height: 720, TickRate: 70, SmokeFrames: 2400, CaptureDirectory: filepath.Join(out, "checkpoints")}); err != nil {
		t.Fatal(err)
	}
	if !c.ExitRequested || c.Screen != ScreenQuit {
		t.Fatal("native helper did not complete owned quit", c.Screen)
	}
	checks := map[string]bool{"real_window_exit": true, "genuine_preparation": true, "initially_paused": metaBefore.Paused}
	switch name {
	case "damage":
		if r.Systems.Damage[2] != 1 || r.Systems.Damage[8] != 1 || r.Systems.DamageMasks[0] != 0x21 || len(r.DamageEvents) != 3 || r.DamageHealth != 229 {
			t.Fatal("native damage/GPS input path did not reach real mechanics", r.Systems, r.DamageEvents)
		}
		checks["engine_avionics_crack_damage"] = true
	case "power-loss":
		if r.Systems.Damage[2] != 2 || r.Systems.Damage[7] != 2 || r.Systems.Damage[9] != 1 || r.SpeechEnabled || len(r.DamageEvents) != 4 {
			t.Fatal("native dualengine/609/warning path", r.Systems)
		}
		checks["dual_engine_and_source_warning"] = true
	case "crash":
		if p.Initialized.Sortie.Result == nil || p.Initialized.Sortie.Result.Reason != "damage-fuel" || r.TerminalPresentation.Frames != 4 || r.TerminalPresentation.RNGCalls != 68263 || !p.Initialized.Sortie.Result.DamageKnown {
			t.Fatal("native terminal sequence not frozen", p.Initialized.Sortie, r.TerminalPresentation)
		}
		checks["terminal_four_updates_unscored"] = true
	case "farp":
		if p.Initialized.Sortie == nil || p.Initialized.Sortie.Services == 0 || p.Initialized.Sortie.Pending != "" || r.Systems.Fuel[0] < 99 || r.State.Rotor != 0 {
			t.Fatal("native FARP source guards", p.Initialized.Sortie, r.Systems.Fuel)
		}
		checks["real_rotor_contact_refill"] = true
	case "pickup-return":
		end := p.Initialized.Sortie.Result
		if end == nil || end.SourceStatus&0x81 != 0x81 || p.Initialized.Stores.Ships[0].Flags&0xf8 != 0x18 || p.Initialized.ObjectiveAssignments[0] != 1 {
			t.Fatal("native real pickup plus supported return facts", end)
		}
		checks["pickup_then_loaded_safe_return"] = true
	case "return":
		stats := store.State().SelectedPilot().Statistics
		old := before.SelectedPilot().Statistics
		if stats.MissionCampaignCounter != old.MissionCampaignCounter+1 || c.previousScreen != ScreenOffice {
			t.Fatal("native healthy Single result not committed once", stats, c.previousScreen)
		}
		if rec, err := store.RecoveryCandidate(); err != nil || rec == nil {
			t.Fatal("completed Single Phoenix unavailable", err)
		}
		checks["healthy_single_result_and_phoenix"] = true
	default:
		t.Fatal("unrecognized connected scenario", name)
	}
	if name != "return" {
		if !reflect.DeepEqual(before, store.State()) {
			t.Fatal("unsupported/unfinished scenario changed career")
		}
		checks["canonical_career_unchanged"] = true
	}
	if r.State.Terminal != nil || p.Initialized.Sortie != nil && p.Initialized.Sortie.Result != nil {
		if p.Initialized.Sortie != nil && p.Initialized.Sortie.Result != nil && p.Initialized.Sortie.Result.SourceStatus&0x3c != 0 {
			checks["no_survival_classification_or_persistent_loss"] = name != "return"
		}
	}
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
