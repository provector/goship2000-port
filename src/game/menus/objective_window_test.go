//go:build !headless

package menus

import (
	"encoding/json"
	"gunship2000-assets/game/configuration"
	gamestate "gunship2000-assets/game/state"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestConnectedPlayerPickupAndGPSDrop(t *testing.T) {
	out := os.Getenv("GS2000_N2_OBJECTIVE_NATIVE")
	if out == "" {
		t.Skip("fresh disposable connected output required")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	scale, err := strconv.Atoi(os.Getenv("GS2000_N2_OBJECTIVE_SCALE"))
	if err != nil || scale != 1 && scale != 2 {
		t.Fatal("scale1/2 required")
	}
	c := objectiveController(t, 38, 5, false)
	p := c.Rendering
	r := p.Flight
	objectiveApproach(t, c)
	options := configuration.Defaults()
	options.SimulationRenderScale = scale
	c.Runtime = options
	p.Scene.RenderScale = scale
	p.Paused, r.Paused = true, true // Supplied pre-pickup checkpoint. Real key resumes.
	if err := p.ViewControls.Tick(p.cameraInputs(), p.Renderer.Assets); err != nil {
		t.Fatal(err)
	}
	p.publishView()
	p.resetPresentation()
	if err := filepath.WalkDir(c.Store.Directory, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(c.Store.Directory, path)
		if e != nil {
			return e
		}
		dest := filepath.Join(out, "saves", rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0700)
		}
		raw, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		return os.WriteFile(dest, raw, 0600)
	}); err != nil {
		t.Fatal(err)
	}
	c.Store, err = gamestate.OpenSaveStore(filepath.Join(out, "saves"), "")
	if err != nil {
		t.Fatal(err)
	}
	before := c.Store.State()
	mass := r.MassTerm
	raw, _ := json.MarshalIndent(map[string]any{"scope": "Actual generated GulfFlight seed38/allBlackhawk accepted ORDANCE; supplied Captain/squadron and approach pose at primary query. Real host resume/GPS C/end/quit; no natural original witness.", "world": c.Mission.Data.World, "scenario": c.Mission.Data.Scenario, "object_sha256": c.Mission.Data.ObjectHash, "TARGET62": p.ResultTargets.Records[2], "pose": r.State.Pose, "initial_mass": mass, "result_unavailable": c.Mission.ResultUnavailable}, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "fixture.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := RunOfficeWindow(c.Office, WindowOptions{Runtime: options, FrontEnd: c, Development: true, Width: 960, Height: 720, TickRate: 70, SmokeFrames: 2400, CaptureDirectory: filepath.Join(out, "checkpoints")}); err != nil {
		t.Fatal(err)
	}
	if !c.ExitRequested || c.Screen != ScreenQuit || c.previousScreen != ScreenBriefing {
		t.Fatal("native unsupported Flight end/quit path", c.Screen)
	}
	if p.ResultTargets.Records[2][32] != 0 || p.Initialized.ObjectiveAssignments[0] != 1 || p.Initialized.Stores.Ships[0].Flags&0xf8 != 0 || r.MassTerm != mass-4000 {
		t.Fatal("native pickup/drop source state", p.Initialized.ObjectiveAssignments, p.Initialized.Stores.Ships[0].Flags, r.MassTerm, mass)
	}
	if p.Initialized.Sortie.Result == nil || p.Initialized.Sortie.Result.SourceStatus&0x81 != 0 {
		t.Fatal("dropped/aborted pickup granted recovery")
	}
	if !reflect.DeepEqual(before, c.Store.State()) || c.Mission.ResultLaunch != nil {
		t.Fatal("Flight awarded career")
	}
	raw, _ = json.MarshalIndent(map[string]any{"checks": map[string]bool{"genuine_Flight": true, "pickup_completed": true, "assigned_actor1": true, "GPS_drop": true, "no_pickup_mass_add": true, "no_recovered_abort": true, "Flight_result_not_admitted": true, "canonical_save_unchanged": true}, "final_mass": r.MassTerm, "initial_mass": mass, "steps": r.Steps, "owner": p.Initialized.ObjectiveAssignments, "result": p.Initialized.Sortie.Result}, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "objective-summary.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}
