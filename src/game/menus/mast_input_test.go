package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/controllers"
	"gunship2000-assets/game/flightcontrol"
	"gunship2000-assets/game/render3d"
	"gunship2000-assets/game/replay"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMastResolvedCyclicSourceAndHistoricalGate(t *testing.T) {
	for _, corrected := range []bool{false, true} {
		p := &RenderingPreview{fieldCorrectionsMechanics: corrected, Flight: &FlightRuntime{}, ViewControls: &render3d.CameraController{Mode: 7}}
		p.Flight.commandInput.Live = &flightcontrol.LiveInput{}
		var yaw uint16
		for _, v := range [][2]int16{{37, -63}, {127, -127}, {-127, 127}, {0, 0}} {
			p.Flight.Axes = flightcontrol.Axes{X: v[0], Y: v[1]}
			p.consumeMastCyclic()
			var pitch uint16
			if corrected {
				yaw -= uint16(v[0] << 4)
				pitch = uint16(v[1] << 5)
			}
			if p.ViewControls.Camera.Angles != [3]uint16{pitch, yaw, 0} || p.Flight.Axes != (flightcontrol.Axes{}) {
				t.Fatal("source shifts/clearing/historical branch", corrected, v, p.ViewControls.Camera.Angles, p.Flight.Axes)
			}
		}
		// Keyboard remains the source +/-16 reader in both interpretations.
		p.Flight.commandInput.Live = nil
		p.Flight.held = flightcontrol.Held{Right: true, Up: true}
		p.consumeMastCyclic()
		yaw -= 0x100
		if p.ViewControls.Camera.Angles != [3]uint16{0xfe00, yaw, 0} {
			t.Fatal("keyboard changed", p.ViewControls.Camera.Angles)
		}
		p.ViewControls.Mode = 0
		p.Flight.Axes = flightcontrol.Axes{X: 37, Y: 63}
		p.consumeMastCyclic()
		if p.Flight.Axes != (flightcontrol.Axes{X: 37, Y: 63}) {
			t.Fatal("F1 flight cyclic was consumed")
		}
	}
}

func TestMastMappedStickKeyboardPairsAndF1(t *testing.T) {
	c := mastProfileFixture(t, 1, "land", replay.R221Compatibility)
	jFinishFade(t, c)
	p, r := c.Rendering, c.Rendering.Flight
	if !p.fieldCorrectionsMechanics {
		t.Fatal("new interpretation not selected before launch")
	}
	g, backend := controllerDevice(c)
	profile := g.live.Config.Profiles["p"]
	profile.AxisConvention = "native-v1"
	g.live.Config.Version = 3
	g.live.Config.Profiles["p"] = profile
	var latch inputLatch
	var now time.Duration
	step := func(b Buttons) {
		t.Helper()
		b.InputTime = now
		now += 100 * time.Millisecond
		if err := c.Handle(latch.Read(g.Merge(c, b, .1, 0, 0, false, nil))); err != nil {
			t.Fatal(err)
		}
		if err := c.advanceFlight(.1); err != nil {
			t.Fatal(err)
		}
	}
	step(Buttons{}) // Fresh selected-device baseline and launch-release gate.
	step(Buttons{})
	step(Buttons{FunctionKeys: [9]bool{false, true}})
	step(Buttons{})
	pose, collective := r.State.Pose, r.State.Collective
	before := p.ViewControls.Camera.Angles
	raw := backend.s.States[9]
	raw.Axes[0], raw.Axes[1] = .5, -.5 // Physical right and forward in SDL convention.
	backend.s.States[9] = raw
	for i := 0; i < 5; i++ {
		step(Buttons{})
	}
	if p.ViewControls.Mode != 7 || p.ViewControls.Camera.Angles[1] == before[1] || int16(p.ViewControls.Camera.Angles[0]) >= 0 {
		t.Fatal("mapped stick failed to slew/elevate sight", before, p.ViewControls.Camera.Angles)
	}
	if r.Axes != (flightcontrol.Axes{}) || r.State.Pose.Angles != pose.Angles || r.State.Pose.H0 != pose.H0 || r.State.Pose.H1 != pose.H1 || r.State.Collective != collective {
		t.Fatal("mast cyclic leaked to airframe/collective", r.Axes, pose, r.State.Pose, collective, r.State.Collective)
	}
	// Neutral immediately resets sight elevation, not helicopter height.
	raw.Axes[0], raw.Axes[1] = 0, 0
	backend.s.States[9] = raw
	for i := 0; i < 3; i++ {
		step(Buttons{})
	}
	if p.ViewControls.Camera.Angles[0] != 0 {
		t.Fatal("neutral retained sight elevation")
	}
	// Saved user inversion is resolved once by the backend/role adapter,
	// never repeated by the mast consumer.
	rollBinding := profile.Bindings["flight.roll"]
	rollBinding.Invert = true
	profile.Bindings["flight.roll"] = rollBinding
	g.live.Config.Profiles["p"] = profile
	raw.Axes[0] = .5
	backend.s.States[9] = raw
	before = p.ViewControls.Camera.Angles
	for i := 0; i < 3; i++ {
		step(Buttons{})
	}
	if int16(p.ViewControls.Camera.Angles[1]-before[1]) <= 0 {
		t.Fatal("mast lost/doubled saved inversion", before, p.ViewControls.Camera.Angles)
	}
	raw.Axes[0] = 0
	backend.s.States[9] = raw
	step(Buttons{})
	// The same digital pair feeds the source +/-16 mast reader and is consumed.
	profile.Bindings["flight.roll"] = controllers.Binding{Kind: "pair", Negative: &controllers.Control{Kind: "button", Index: 6}, Positive: &controllers.Control{Kind: "button", Index: 7}}
	g.live.Config.Profiles["p"] = profile
	raw.Buttons[7] = true
	backend.s.States[9] = raw
	before = p.ViewControls.Camera.Angles
	step(Buttons{})
	step(Buttons{})
	if p.ViewControls.Camera.Angles[1] == before[1] || r.Axes != (flightcontrol.Axes{}) {
		t.Fatal("controller pair did not reach mast")
	}
	raw.Buttons[7] = false
	backend.s.States[9] = raw
	step(Buttons{})
	step(Buttons{SceneDirections: [4]bool{true}})
	step(Buttons{})
	step(Buttons{FunctionKeys: [9]bool{true}})
	step(Buttons{})
	if p.ViewControls.Mode != 0 {
		t.Fatal("F1 did not return to cockpit")
	}
	step(Buttons{SceneDirections: [4]bool{false, true}})
	step(Buttons{SceneDirections: [4]bool{false, true}})
	if r.Axes.X <= 0 {
		t.Fatal("fresh cockpit input still owned by mast", r.Axes)
	}
}

func TestMastContinuousFilmAndPresentationCadence(t *testing.T) {
	for _, fps := range []int{10, 60} {
		t.Run(fmt.Sprint(fps), func(t *testing.T) {
			c := mastProfileFixture(t, 1, "land", replay.R221Compatibility)
			jFinishFade(t, c)
			p, r := c.Rendering, c.Rendering.Flight
			act(t, c, Actions{})
			act(t, c, Actions{ViewKey: 2})
			act(t, c, Actions{})
			before, collective := r.State.Pose, r.State.Collective
			initialYaw := p.ViewControls.Camera.Angles[1]
			initialTicks := r.Steps
			for frame := 0; frame < fps; frame++ {
				live := &flightcontrol.LiveInput{Axes: [4]int16{-63, 37}, Bound: [4]bool{true, true}, Epoch: 1}
				act(t, c, Actions{InputTime: time.Duration(frame) * time.Second / time.Duration(fps), Live: live})
				if err := c.AdvanceRendering(1.0 / float64(fps)); err != nil {
					t.Fatal(err)
				}
			}
			steps := r.Steps - initialTicks
			if steps < 9 || steps > 11 || p.ViewControls.Camera.Angles[1] != initialYaw-uint16(steps)*uint16(37<<4) || p.ViewControls.Camera.Angles[0] != uint16(0xf820) {
				t.Fatal("presentation altered source mast update", fps, steps, initialYaw, p.ViewControls.Camera.Angles)
			}
			yawDelta := initialYaw - p.ViewControls.Camera.Angles[1]
			if r.State.Pose.Angles != before.Angles || r.State.Pose.H0 != before.H0 || r.State.Pose.H1 != before.H1 || r.State.Collective != collective {
				t.Fatal("sight input moved helicopter")
			}
			act(t, c, Actions{Live: &flightcontrol.LiveInput{Epoch: 1}})
			jStep(t, c, 2)
			act(t, c, Actions{ViewKey: 1})
			act(t, c, Actions{})
			editorAction(t, c, Actions{EndMission: true})
			editorAction(t, c, Actions{ConfirmQuit: true})
			if c.Screen != ScreenDebrief {
				t.Fatal("normal voluntary ending", c.Screen)
			}
			f := verifyFilm(t, c) // Complete strict playback at both render scales, no controller.
			if f.Header.Simulation != replay.R221Compatibility {
				t.Fatal("wrong film interpretation", f.Header.Simulation)
			}
			beforeCareer, _ := json.Marshal(c.Store.State())
			q, err := NewFilmPlayback(c.Office.Assets, f, configuration.Defaults())
			if err != nil {
				t.Fatal(err)
			}
			for !q.Done && q.Preview().ViewControls.Mode != 7 {
				if err = q.Step(); err != nil {
					t.Fatal(err)
				}
			}
			for i := 0; i < 4 && !q.Done; i++ {
				if err = q.Step(); err != nil {
					t.Fatal(err)
				}
			}
			if q.Done {
				t.Fatal("no reached mast sequence before Resume")
			}
			if err = q.Resume(); err != nil {
				t.Fatal(err)
			}
			if q.Engine.Store != nil {
				t.Fatal("Resume acquired career authority")
			}
			afterCareer, _ := json.Marshal(c.Store.State())
			if !bytes.Equal(beforeCareer, afterCareer) {
				t.Fatal("Resume changed career")
			}
			if out := os.Getenv("GS2000_R221_OUTPUT"); out != "" {
				if err = os.MkdirAll(out, 0700); err != nil {
					t.Fatal(err)
				}
				b, err := replay.Marshal(f)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(out, fmt.Sprintf("mast-%dfps.g2r", fps)), b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("mapped native input diagnostic at%d hostfps: %d logical steps, yawdelta%d, sight pitch-2016, no airframe/collective movement; strict1x/2x and isolated Resume", fps, steps, yawDelta)
		})
	}
}
