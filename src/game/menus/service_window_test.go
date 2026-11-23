//go:build !headless

package menus

import (
	"encoding/binary"
	"encoding/json"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	gamestate "gunship2000-assets/game/state"
	"gunship2000-assets/game/world"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

// This opt-in native fixture has a real generated Flight/FARP and an explicitly
// supplied landed approach pose. Only root's PID-verified host helper supplies
// GPS/rotor/quit input; no normal-game cheat or extra source AI is introduced.
func TestConnectedFARPService(t *testing.T) {
	out := os.Getenv("GS2000_N2_SERVICE_NATIVE")
	if out == "" {
		t.Skip("fresh disposable connected output required")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	scale, err := strconv.Atoi(os.Getenv("GS2000_N2_SERVICE_SCALE"))
	if err != nil || scale != 1 && scale != 2 {
		t.Fatal("scale1/2 required")
	}
	c := mastFixture(t, 0, "pass")
	jFinishFade(t, c)
	full, err := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if err != nil {
		t.Fatal(err)
	}
	// Flight can produce a scored result. This negative fixture exercises FARP
	// service with a genuine missing result prerequisite. The ordinary scored
	// Flight case is covered by TestActualPreparedFARPLogicalServiceAndGPSFreeze.
	withoutDebrief := *full
	withoutDebrief.Debrief = nil
	c.Office.Assets, c.Menus = &withoutDebrief, full.Menus
	if err := c.beginSortieResult(c.Mission.Final); err != nil {
		t.Fatal(err)
	}
	if c.Mission.ResultUnavailable != "prepared player debrief resources unavailable" {
		t.Fatal("expected refusal for missing debrief resources", c.Mission.ResultUnavailable)
	}
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
	p := c.Rendering
	r := p.Flight
	if p.Initialized.Targets == nil || c.Mission.ResultLaunch != nil {
		t.Fatal("missing live Flight targets or fabricated result admission")
	}
	target := p.Initialized.Targets.Records[1]
	if binary.LittleEndian.Uint16(target[32:]) == 0 || int16(binary.LittleEndian.Uint16(target[50:])) <= 0 {
		t.Fatal("ordinary fixture lacks FARP")
	}
	r.State.Pose.H0 = binary.LittleEndian.Uint32(target[42:])
	r.State.Pose.H1 = binary.LittleEndian.Uint32(target[46:])
	support, err := p.Renderer.Assets.ContactHeight(p.Initialized.World, world.QueryPose{H0: r.State.Pose.H0, H1: r.State.Pose.H1, Height: 32767}, p.Initialized.ContactObjects())
	if err != nil {
		t.Fatal(err)
	}
	r.State.Pose.Height = support + 1
	r.State.Pose.Angles = [3]uint16{}
	r.State.Forward, r.State.Vertical, r.State.Lateral = 0, 0, 0
	r.State.Rotor, r.State.Collective, r.State.FlightStage, r.State.Hover, r.State.HeightControl = 1, 0, 0, 0, 0
	r.Systems.Fuel[0] = 9
	r.Defenses.Chaff, r.Defenses.Flares = 0, 0
	for i := range p.Initialized.Stores.Ships[0].Stations {
		p.Initialized.Stores.Ships[0].Stations[i].Ammunition = 0
	}
	options := configuration.Defaults()
	options.SimulationRenderScale = scale
	c.Runtime = options
	p.Scene.RenderScale = scale
	p.publishFlight()
	if err := p.ViewControls.Tick(p.cameraInputs(), p.Renderer.Assets); err != nil {
		t.Fatal(err)
	}
	p.publishView()
	p.resetPresentation()
	p.clearFlightInput()
	c.takeAudioCues()
	fixture := map[string]any{"scope": "Native fixture: ordinary Europe Flight seed10, supplied BrigadierGeneral/squadron and all-Cobra accepted loadout; supplied grounded FARP pose/low fuel/empty stores. Real host controls own rotor off, GPS and quit. Not naturally flown or original execution.", "theatre": c.Mission.Data.Inputs.Theatre, "mission_type": c.Mission.Data.Inputs.MissionType, "seed": 10, "world": c.Mission.Data.World, "scenario": c.Mission.Data.Scenario, "source_object_sha256": c.Mission.Data.ObjectHash, "FARP_TARGET62": target, "supplied_pose": r.State.Pose, "support_height": support, "initial_rotor": r.State.Rotor, "initial_fuel": r.Systems.Fuel[0], "FARP_limit": int16(binary.LittleEndian.Uint16(target[50:])), "result_admission": c.Mission.ResultUnavailable, "render_scale": scale}
	raw, _ := json.MarshalIndent(fixture, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "fixture.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeDevelopmentCheckpoint(out, 0, c); err != nil {
		t.Fatal(err)
	}
	if err := RunOfficeWindow(c.Office, WindowOptions{Runtime: options, FrontEnd: c, Development: true, Width: 960, Height: 720, TickRate: 70, SmokeFrames: 1800, CaptureDirectory: filepath.Join(out, "checkpoints")}); err != nil {
		t.Fatal(err)
	}
	if !c.ExitRequested || p.Initialized.Sortie == nil || p.Initialized.Sortie.Services == 0 || p.Initialized.Sortie.Pending != "" {
		t.Fatal("native rotor/quit path did not service without an end prompt", p.Initialized.Sortie)
	}
	if r.State.Rotor != 0 || r.Systems.Fuel[0] != p.Initialized.Stores.Blueprint[0].Fuel {
		t.Fatal("rotor not stopped or source fuel not restored")
	}
	ship := p.Initialized.Stores.Ships[0]
	want := p.Initialized.Stores.Blueprint[0]
	if ship.Stations != want.Stations || r.Defenses.Chaff != want.Chaff || r.Defenses.Flares != want.Flares {
		t.Fatal("native runtime stores/defenses not restored")
	}
	if !reflect.DeepEqual(before, c.Store.State()) || c.Mission.ResultLaunch != nil {
		t.Fatal("Flight service/quit wrote fabricated progression")
	}
	restarted, err := gamestate.OpenSaveStore(c.Store.Directory, "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, restarted.State()) {
		t.Fatal("restart applied Flight result")
	}
	raw, _ = json.MarshalIndent(map[string]any{"checks": map[string]bool{"actual_rotor_off": true, "FARP_refill": true, "no_natural_end": true, "Flight_result_not_admitted": true, "canonical_save_unchanged": true, "restart_unchanged": true}, "fixture": fixture, "final_services": p.Initialized.Sortie.Services, "final_fuel": r.Systems.Fuel[0], "final_rotor": r.State.Rotor, "FARP_status": p.Initialized.Sortie.FARPStatus}, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "service-summary.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}
