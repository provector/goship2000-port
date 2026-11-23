package menus

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/replay"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// Q32 diagnostic: replay ONLY individually checkpointed operations from an owned
// unsealed prefix. No ending/summary/complete bit is supplied or fabricated.
// The ordinary public reader remains strict. Never publish this diagnostic state.
func TestGroundRetainedPrefixViews(t *testing.T) {
	path, out := os.Getenv("GS2000_Q32_GROUND_PREFIX"), os.Getenv("GS2000_Q32_GROUND_OUTPUT")
	if path == "" || out == "" {
		t.Skip("explicit owned unsealed prefix and disposable frame output")
	}
	data, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var f replay.Film
	if e = json.Unmarshal(data, &f); e != nil {
		t.Fatal(e)
	}
	if f.Header.Complete || len(f.Operations) < 2 {
		t.Fatal("expected incomplete retained prefix")
	}
	fixtureID := fmt.Sprintf("%x", sha256.Sum256([]byte(f.Header.ID)))
	var launch filmLaunch
	if e = json.Unmarshal(f.Launch, &launch); e != nil {
		t.Fatal(e)
	}
	a, e := assetdata.LoadFrontEndAssets(os.Getenv("GS2000_TEST_MISSIONS"))
	if e != nil {
		t.Fatal(e)
	}
	final, e := cloneFilmFinal(&launch.Final)
	if e != nil {
		t.Fatal(e)
	}
	engine := &FrontEnd{simulationProfile: f.Header.Simulation, Office: &Office{Assets: a}, Runtime: launch.Runtime, filmIsolation: &filmSessionIsolation{Mode: "playback"}, Screen: ScreenScene, Mission: &MissionSession{Data: final.Mission, Final: final, Flight: final.Flight, ReadinessRows: final.Aircraft, PilotID: final.PilotID, Settings: final.Settings}}
	if e = engine.constructSimulation(final); e != nil {
		t.Fatal(e)
	}
	p := engine.Rendering
	p.Flight.Combat.SetFormationEnabled(f.FormationMechanics())
	p.Flight.Combat.SetLogisticsEnabled(f.P4Mechanics())
	p.p4Mechanics = f.P4Mechanics()
	p.p5Mechanics = f.P5Mechanics()
	p.Flight.Combat.SetMissionActorsEnabled(p.p5Mechanics)
	p.p6Mechanics = f.P6Mechanics()
	p.Flight.Combat.SetTrainEnabled(p.p6Mechanics)
	p.p7Mechanics = f.P7Mechanics()
	p.Flight.Combat.SetFeedbackEnabled(p.p7Mechanics)
	p.filmLegacy = f.Legacy()
	p.Flight.Notifications.LegacyO = f.Legacy()
	if launch.Adjustment != nil {
		if e = p.applyFilmInitial(*launch.Adjustment, a); e != nil {
			t.Fatal(e)
		}
	}
	if group := compareFilmCheckpoint(launch.Initial, p.FilmCheckpoint()); group != "" {
		t.Fatal("initial", group)
	}
	scale, _ := strconv.Atoi(os.Getenv("GS2000_Q32_GROUND_SCALE"))
	if scale != 2 {
		scale = 1
	}
	opts := configuration.Defaults()
	opts.SimulationRenderScale = scale
	applyReplayPresentationOptions(p, engine, opts)
	pb := &FilmPlayback{Engine: engine, Film: &f, initialKinds: p.filmInitialKinds()}
	p.filmPlayback = pb
	p.installFilmObserver()
	if e = os.MkdirAll(out, 0700); e != nil {
		t.Fatal(e)
	}
	records := []map[string]any{}
	// The public complete-film reader finalizes after its last operation. This
	// diagnostic has no End. An unexecuted zero slot is only a test bounds guard;
	// it is never serialized or applied, and supplies no event/outcome/ending.
	recordedOperations := len(f.Operations)
	f.Operations = append(f.Operations, replay.Operation{})
	for pb.Cursor < recordedOperations {
		o := f.Operations[pb.Cursor]
		if o.Kind == "end" || o.Kind == "terminal" {
			break
		}
		if e = pb.Step(); e != nil {
			t.Fatal("prefix", pb.Cursor, e)
		}
		if o.Kind != "step" {
			continue
		}
		step := p.Flight.Steps
		chosen := fixtureID == "3900b84a8855fe465b62dfa3ee931db2270b03ef4adeb3f27c31e34ac16c407f" && step >= 480 && step <= 560 && step%5 == 0 || fixtureID == "024fb2a302af87266d2dad518114eb82badf5657d8fed4e9b72cd943c2a900e5" && step >= 120 && step <= 200 && step%5 == 0
		if !chosen {
			continue
		}
		pix := engine.Frame(false)
		rgba, e := engine.RenderRGBA(false)
		if e != nil {
			t.Fatal(e)
		}
		_ = pix
		name := fmt.Sprintf("%s-step%04d", f.Header.ID, step)
		file, e := os.Create(filepath.Join(out, name+".png"))
		if e != nil {
			t.Fatal(e)
		}
		e = png.Encode(file, rgba)
		file.Close()
		if e != nil {
			t.Fatal(e)
		}
		drawScene, _ := p.presentationInputs()
		points := []map[string]any{}
		for _, xy := range [][2]int{{80, 90}, {81, 90}, {150, 85}, {110, 55}} {
			at := xy[1]*scale*(320*scale) + xy[0]*scale
			r, g, b, _ := engine.CurrentPalette()[pix[at]].RGBA()
			points = append(points, map[string]any{"xy": xy, "index": pix[at], "rgb": []uint32{r >> 8, g >> 8, b >> 8}, "owner": p.skyMask[at]})
		}
		records = append(records, map[string]any{"draw_scene": drawScene, "sample_points": points, "display_palette_rgb6": p.PaletteState.RGB6(), "name": name, "step": step, "scale": scale, "pose": p.Flight.State.Pose, "camera": p.Scene.Camera, "view": p.ViewControls, "scene": p.Scene, "runtime": p.Flight.State, "palette": p.Scene.PaletteBank, "checkpoint": p.FilmCheckpoint()})
	}
	// Scene includes render-only masks/slices; compact receipts retain pose/view and
	// full scene metadata as diagnostic output, never a new replay/checkpoint.
	b, _ := json.MarshalIndent(records, "", "  ")
	if e = os.WriteFile(filepath.Join(out, "views.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	t.Log("validated retained prefix operations", pb.Cursor, "steps", p.Flight.Steps, "not a sealed/complete film")
	_ = render3d.FullView
}
