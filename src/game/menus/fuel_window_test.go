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
	"testing"
)

func TestConnectedOrdinaryFuel(t *testing.T) {
	out := os.Getenv("GS2000_N5_FUEL_NATIVE")
	if out == "" {
		t.Skip("fresh connected fuel output required")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
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
	pilot.Name = "N5 Ordinary Fuel"
	st := store.State()
	st.Slots[0], st.SelectedPilotID = pilot, pilot.ID
	if err = store.Commit(st); err != nil {
		t.Fatal(err)
	}
	c, err := NewFrontEnd(a, store)
	if err != nil {
		t.Fatal(err)
	}
	options := configuration.Defaults()
	c.Runtime = options
	meta, err := c.ConfigureDevelopmentScenario("fuel-empty")
	if err != nil {
		t.Fatal(err)
	}
	before := store.State()
	p, r := c.Rendering, c.Rendering.Flight
	write := func(name string, v any) {
		t.Helper()
		raw, e := json.MarshalIndent(v, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(out, name), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	write("fixture.json", map[string]any{"scenario": meta, "scratch_save": store.Directory, "scope": "Supplied fuel1/countdown0 and coherent realistic approach; real OS resume and ordinary source burn. No accepted hit or terminal preset."})
	if err = RunOfficeWindow(c.Office, WindowOptions{Runtime: options, FrontEnd: c, Development: true, DrawPointer: true, Width: 960, Height: 720, TickRate: 70, SmokeFrames: 2000, CaptureDirectory: filepath.Join(out, "checkpoints")}); err != nil {
		t.Fatal(err)
	}
	if !c.ExitRequested || c.Screen != ScreenQuit {
		t.Fatal("helper did not close owned fuel session")
	}
	if r.Steps < 200 || r.Systems.Damage[9] == 0 || len(r.DamageEvents) != 0 {
		t.Fatal("ordinary oldempty burn not reached independently of accepted hits", r.Steps, r.Systems.Damage, r.DamageEvents)
	}
	if !reflect.DeepEqual(before.Slots, store.State().Slots) || before.SelectedPilotID != store.State().SelectedPilotID {
		t.Fatal("unacknowledged fuel session changed career or status")
	}
	q, err := gamestate.OpenSaveStore(store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before.Slots, q.State().Slots) {
		t.Fatal("restart manufactured fuel loss result")
	}
	write("native-summary.json", map[string]any{"checks": map[string]bool{"ordinary_oldempty_reached": true, "no_accepted_hits": true, "real_window_exit": true, "quit_abandons_without_result": true, "restart_career_unchanged": true}, "systems": r.Systems, "flight": r.State, "damage_events": r.DamageEvents, "transient_end": p.Initialized.Sortie, "scope": "Native supplied approach with real OS resume; inspect host monitor and audio-playback separately. Actual source contact may enter terminal handling; no final result acknowledged."})
}
