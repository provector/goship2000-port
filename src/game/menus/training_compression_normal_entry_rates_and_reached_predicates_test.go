package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/diagnostics"
	"gunship2000-assets/game/flightcontrol"
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func freshCompressionController(t *testing.T, typ gamestate.MissionType, disabled bool, cpg ...int) *FrontEnd {
	t.Helper()
	assets := os.Getenv("GS2000_TEST_MISSIONS")
	if assets == "" {
		t.Skip("prepared assets required")
	}
	a, e := assetdata.LoadFrontEndAssets(assets)
	if e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	store, e := gamestate.OpenSaveStore(filepath.Join(root, "saves"), filepath.Join(root, "no-original"))
	if e != nil {
		t.Fatal(e)
	}
	pilot, e := gamestate.NewPilot()
	if e != nil {
		t.Fatal(e)
	}
	pilot.Name = "R231 Training"
	pilot.MissionType = typ
	pilot.Theatre = gamestate.TheatrePersianGulf
	pilot.Preflight = &gamestate.PreflightSettings{CPGControls: 2, EnemyQuality: 3}
	if len(cpg) != 0 {
		pilot.Preflight.CPGControls = cpg[0]
	}
	if typ != gamestate.MissionTraining {
		pilot.Rank = gamestate.RankCaptain
	}
	state := store.State()
	state.Slots[0] = pilot
	state.SelectedPilotID = pilot.ID
	if e = store.Commit(state); e != nil {
		t.Fatal(e)
	}
	c, e := NewFrontEnd(a, store)
	if e != nil {
		t.Fatal(e)
	}
	c.Runtime.DisableEnemyAI = disabled
	c.Runtime.DisableWingmenAI = true
	c.FilmDirectory = filepath.Join(root, "films")
	if e = c.startMission(1, nil); e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 100 && c.Screen != ScreenBriefing; n++ {
		if e = c.Tick(); e != nil {
			t.Fatal(e)
		}
	}
	if c.Screen != ScreenBriefing {
		t.Fatal(c.Screen, c.Message)
	}
	startRendering(t, c)
	jFinishFade(t, c)
	c.Handle(Actions{})
	t.Cleanup(c.AbandonFilmRecording)
	return c
}

func TestTrainingCompressionNormalEntryRatesAndReachedPredicates(t *testing.T) {
	var rows []map[string]any
	for _, typ := range []gamestate.MissionType{gamestate.MissionTraining, gamestate.MissionSingle} {
		for _, disabled := range []bool{false, true} {
			c := freshCompressionController(t, typ, disabled)
			p, r := c.Rendering, c.Rendering.Flight
			for _, rate := range []int16{2, 4, 6, 16} {
				c.Handle(Actions{CompressionReset: true})
				c.Handle(Actions{})
				for n := int16(0); n < rate/2; n++ {
					if e := c.Handle(Actions{CompressionIncrease: true}); e != nil {
						t.Fatal(e)
					}
					c.Handle(Actions{})
				}
				requested := r.Compression.Request
				if requested != rate {
					t.Fatal("accepted bracket", typ, rate, requested)
				}
				before := r.Steps
				if e := c.advanceFlight(.1); e != nil {
					t.Fatal(e)
				}
				rows = append(rows, map[string]any{"type": typ, "enemy_disabled": disabled, "requested": rate, "after": r.Compression.Request, "steps": r.Steps - before, "hold": r.State.HeightControl, "route": r.State.WaypointIndex, "flight_limit": r.State.CompressionLimit, "flight_reset": r.State.CompressionReset, "contact_limit": r.Avionics.CompressionLimited, "world": p.Initialized.World, "switches": p.Initialized.Switches})
				if r.Compression.Request != rate || r.Steps-before != uint64(rate) {
					t.Fatal("clear transit unexpectedly limited", rows[len(rows)-1])
				}
				if p.Error != nil || r.Failure != "" {
					t.Fatal(p.Error, r.Failure)
				}
			}
		}
	}
	if dir := os.Getenv("GS2000_R231_FEEDBACK_OUTPUT"); dir != "" {
		os.MkdirAll(dir, 0700)
		b, _ := json.MarshalIndent(rows, "", "  ")
		os.WriteFile(filepath.Join(dir, "training-rates.json"), b, 0600)
	}
	for _, row := range rows {
		t.Log(fmt.Sprint(row))
	}
}

// Normally saved editor mission and normal BEGIN/APU/rotor/collective input.
// The real Infantry actor and source A070 create every projectile in this test.
// This is native application evidence, not an OS-input or listening observation.
func TestNaturalGuidedThreatPublishesWarningAndTone(t *testing.T) {
	base := os.Getenv("GS2000_R23_WITNESSES")
	if base == "" {
		t.Skip("saved authored guided encounter required")
	}
	c := naturalControllerMode(t, filepath.Join(base, "guided", "try02", "app"), false, true, 0, replay.R231AuthoredCompatibility)
	defer c.AbandonFilmRecording()
	before, err := json.Marshal(c.Store.State().SelectedPilot())
	if err != nil {
		t.Fatal(err)
	}
	p, r := c.Rendering, c.Rendering.Flight
	if !p.threatFeedbackMechanics || !r.SpeechEnabled {
		t.Fatal("new profile warning gate or normal speech absent")
	}
	if p.Initialized.Switches&0x18 != 0 {
		t.Fatal("CP/G must be None", p.Initialized.Switches)
	}
	samples := []map[string]any{}
	shots, warnings, tones := 0, 0, 0
	step := func() {
		t.Helper()
		if e := c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
		for _, ev := range r.CombatEvents {
			if ev.Kind == "launch" && ev.Target == 0 && ev.Value == 9 {
				shots++
			}
		}
		r.CombatEvents = nil
		for _, ev := range r.sceneAudio {
			if ev.event == 6 {
				tones++
			}
		}
		if r.Avionics.Threat.WarningFlags != 0 {
			warnings++
			v := p.Cockpit.ContactAvionics
			if v == nil || v.WarningFlags != r.Avionics.Threat.WarningFlags {
				t.Fatal("warning lost before renderer", v, r.Avionics.Threat.WarningFlags)
			}
			samples = append(samples, map[string]any{"step": r.Steps, "phase": p.Cockpit.BlinkPhase, "flags": v.WarningFlags, "sound06_requests": tones, "active_slots": r.Combat.Slots})
		}
		r.sceneAudio = nil
		if p.Error != nil || r.Failure != "" {
			t.Fatal(p.Error, r.Failure)
		}
	}
	c.Handle(Actions{})
	c.Handle(Actions{FlightCommands: []int{2}})
	for n := 0; n < 40; n++ {
		step()
	}
	editorAction(t, c, Actions{GPSOpen: true})
	for n := 0; n < 4; n++ {
		editorAction(t, c, Actions{GPSKey: "n"})
		editorAction(t, c, Actions{GPSKey: "h"})
	}
	editorAction(t, c, Actions{ViewKey: 1})
	for n := 0; n < 40; n++ {
		c.Handle(Actions{FlightCommands: []int{0}})
		step()
	}
	c.Handle(Actions{})
	c.Handle(Actions{FlightCommands: []int{10}})
	c.Handle(Actions{})
	for n := 0; n < 2400 && warnings < 4; n++ {
		step()
	}
	if shots == 0 || warnings == 0 || tones == 0 {
		t.Fatal("missing natural chain", shots, warnings, tones, r.SecondaryMFD, p.Cockpit.CameraMode, r.Systems.DamageMasks)
	}
	if dir := os.Getenv("GS2000_R231_FEEDBACK_OUTPUT"); dir != "" {
		b, _ := json.MarshalIndent(samples, "", "  ")
		os.WriteFile(filepath.Join(dir, "natural-guided-warning.json"), b, 0600)
	}
	t.Log("real guided releases", shots, "warning phases", warnings, "source06 requests", tones)
	// A warning is not proof of a hit. This retained guided sequence misses;
	// preserve that neighboring control and complete its actual result/film.
	for n := 0; n < 200; n++ {
		step()
	}
	finishAndReplay(t, c, before, "r231-guided-warning", replay.R231AuthoredCompatibility)
}

func TestThreatToneDE40IsIndependentOfSpeechAndCadence(t *testing.T) {
	for _, flags := range []uint16{0, 4, 0x40, 0x80, 0x2000} {
		for _, phase := range []uint16{0, 1, 2, 3, 4, 0xffff} {
			for _, gate := range []byte{0, 0x40, 0x80, 0xc0} {
				if got, want := threatWarningTone(flags, phase, gate), flags != 0 && gate&0x40 != 0 && phase%4 == 0; got != want {
					t.Fatal(flags, phase, gate, got, want)
				}
			}
		}
	}
}

// Supplied hold and normalized-axis operands isolate the original signed32/33
// threshold inside a normally launched Training simulation. This is distinct
// from the clear-transit normal-input test and from a physical-stick check.
func TestTrainingSourceHoldLimiterAndTransitionDiagnostic(t *testing.T) {
	var buf bytes.Buffer
	logger := diagnostics.Start(t.TempDir(), configuration.Logging{ShowLog: true, Level: "DEBUG"}, diagnostics.Options{Console: &buf})
	defer logger.Close()
	c := freshCompressionController(t, gamestate.MissionTraining, true)
	c.AbandonFilmRecording() // supplied hold/axis operands are not recorded live inputs
	p, r := c.Rendering, c.Rendering.Flight
	for _, axis := range []int16{32, 33} {
		for _, rate := range []int16{2, 4, 6, 16} {
			r.State.HeightControl = 1
			r.State.WaypointIndex = 0
			r.State.FlightStage = 3
			r.State.Rotor = 1
			r.Compression.Request = rate
			c.Handle(Actions{Live: &flightcontrol.LiveInput{Epoch: 1, Axes: [4]int16{0, axis}, Bound: [4]bool{false, true}}})
			before := r.Steps
			if e := c.advanceFlight(.1); e != nil {
				t.Fatal(e)
			}
			want := rate
			if axis > 32 && rate > 4 {
				want = 0
			}
			if r.Compression.Request != want {
				t.Fatal(axis, rate, r.Compression.Request)
			}
			if axis > 32 && rate > 4 && r.Steps-before != 1 {
				t.Fatal("extra compressed work", r.Steps-before)
			}
		}
	}
	if !strings.Contains(buf.String(), "cause=hold-axis") || !strings.Contains(buf.String(), "effective_after=0") {
		t.Fatal(buf.String())
	}
	if strings.Count(buf.String(), "cause=hold-axis") != 2 {
		t.Fatal("diagnostic did not report once per changed transition", buf.String())
	}
	if dir := os.Getenv("GS2000_R231_FEEDBACK_OUTPUT"); dir != "" {
		os.WriteFile(filepath.Join(dir, "compression-debug.txt"), buf.Bytes(), 0600)
	}
	// A reached source limiter with no request change must not add a repeated line.
	n := buf.Len()
	p.limitFlightCompressionCause("hold-axis")
	if buf.Len() != n {
		t.Fatal("redundant limiter log")
	}
}

func TestWarningActualCockpitFamiliesBothScales(t *testing.T) {
	dir := os.Getenv("GS2000_TEST_MISSIONS")
	if dir == "" {
		t.Skip("prepared cockpit artwork required")
	}
	a, e := assetdata.LoadFrontEndAssets(dir)
	if e != nil {
		t.Fatal(e)
	}
	var rows []map[string]any
	for id := 0; id < 8; id++ {
		for _, scale := range []int{1, 2} {
			family, _ := cockpit.Family(id)
			d := a.Cockpit.Definitions[family]
			frames := map[string][]byte{}
			for _, phase := range []uint16{0, 1} {
				for _, flags := range []uint16{0, 0x2000, 0xc4} {
					in := cockpit.Inputs{AircraftID: id, HUD: true, SecondaryMFD: 5, HUDScale: 128, BlinkPhase: phase, ContactAvionics: &cockpit.ContactAvionicsInputs{WarningFlags: flags}}
					pix := bytes.Repeat([]byte{0x17}, 64000*scale*scale)
					if e = cockpit.ComposeScaled(pix, a.Cockpit, in, scale); e != nil {
						t.Fatal(e)
					}
					name := fmt.Sprintf("id%d-%s-%dx-phase%d-flags%04x", id, family, scale, phase, flags)
					frames[fmt.Sprintf("%d/%d", phase, flags)] = pix
					if out := os.Getenv("GS2000_R231_FEEDBACK_OUTPUT"); out != "" {
						im := image.NewPaletted(image.Rect(0, 0, 320*scale, 200*scale), a.Missions.Palettes["FIELD.PAL"])
						copy(im.Pix, pix)
						f, e := os.Create(filepath.Join(out, name+".png"))
						if e != nil {
							t.Fatal(e)
						}
						e = png.Encode(f, im)
						f.Close()
						if e != nil {
							t.Fatal(e)
						}
					}
				}
			}
			changed := 0
			for i, b := range frames["1/196"] {
				if b != frames["1/0"][i] {
					changed++
					x, y := (i%(320*scale))/scale, (i/(320*scale))/scale
					valid := false
					for _, off := range []int{0x6c, 0x70, 0x74} {
						rx, ry := int(d.Parameters[off/2]), int(d.Parameters[(off+2)/2])
						valid = valid || x >= rx && x < rx+10 && y >= ry && y < ry+6
					}
					if !valid {
						t.Fatal("warning escaped original lamp rectangles", id, scale, x, y)
					}
				}
			}
			if changed == 0 {
				t.Fatal("no actual warning ink", id, family, scale, d.Colours)
			}
			if !bytes.Equal(frames["0/8192"], frames["0/0"]) || bytes.Equal(frames["1/8192"], frames["1/0"]) {
				t.Fatal("radar search blink", id, scale)
			}
			rows = append(rows, map[string]any{"aircraft_id": id, "family": family, "scale": scale, "changed_warning_pixels": changed, "off_colour": d.Colours[4], "on_colour": d.Colours[5]})
		}
	}
	if out := os.Getenv("GS2000_R231_FEEDBACK_OUTPUT"); out != "" {
		b, _ := json.MarshalIndent(rows, "", "  ")
		os.WriteFile(filepath.Join(out, "cockpit-family-pixels.json"), b, 0600)
	}
}

func TestTrainingAirborneNeutralHighCompressionControl(t *testing.T) {
	c := freshCompressionController(t, gamestate.MissionTraining, true)
	p, r := c.Rendering, c.Rendering.Flight
	c.Handle(Actions{FlightCommands: []int{2}})
	c.Handle(Actions{})
	for n := 0; n < 60; n++ {
		c.Handle(Actions{FlightCommands: []int{0}})
		if e := c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
	}
	c.Handle(Actions{SceneDirections: [4]bool{false, false, true, false}})
	for n := 0; n < 12; n++ {
		if e := c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
	}
	c.Handle(Actions{})
	for n := 0; n < 8; n++ {
		if e := c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
	}
	if r.State.RadarAltitude <= 0 || r.State.IndicatedSpeed == 0 {
		t.Fatal("normal input did not establish airborne transit", r.State.RadarAltitude, r.State.IndicatedSpeed, r.State.Rotor, r.State.Collective)
	}
	var samples []map[string]any
	for _, rate := range []int16{2, 4, 6, 16} {
		c.Handle(Actions{CompressionReset: true})
		c.Handle(Actions{})
		for n := int16(0); n < rate/2; n++ {
			c.Handle(Actions{CompressionIncrease: true})
			c.Handle(Actions{})
		}
		before := r.Steps
		pose := r.State.Pose
		if e := c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
		row := map[string]any{"rate": rate, "after": r.Compression.Request, "steps": r.Steps - before, "radar_altitude": r.State.RadarAltitude, "speed": r.State.IndicatedSpeed, "before_pose": pose, "after_pose": r.State.Pose, "world": p.Initialized.World}
		samples = append(samples, row)
		if r.Compression.Request != rate || r.Steps-before != uint64(rate) {
			t.Fatal("clear airborne control unexpectedly limited", row)
		}
	}
	if out := os.Getenv("GS2000_R231_FEEDBACK_OUTPUT"); out != "" {
		b, _ := json.MarshalIndent(samples, "", "  ")
		os.WriteFile(filepath.Join(out, "training-airborne-rates.json"), b, 0600)
	}
}
