package menus

import (
	"encoding/json"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/controllers"
	"gunship2000-assets/game/flightcontrol"
	"gunship2000-assets/game/replay"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestJammerAggregateReleaseAndGPSReset(t *testing.T) {
	c := renderingController(t)
	startRendering(t, c)
	jFinishFade(t, c)
	g, b := controllerDevice(c)
	profile := g.live.Config.Profiles["p"]
	profile.Bindings["weapon.radar_jammer"] = controllers.Binding{Kind: "control", Control: &controllers.Control{Kind: "button", Index: 4}}
	profile.Bindings["weapon.ir_jammer"] = controllers.Binding{Kind: "control", Control: &controllers.Control{Kind: "button", Index: 5}}
	g.live.Config.Profiles["p"] = profile
	var latch inputLatch
	elapsed := time.Duration(0)
	dispatch := func(keyboard bool) {
		t.Helper()
		v := Buttons{InputTime: elapsed}
		if keyboard {
			v.DefenseKeys[2], v.DefenseKeys[3] = true, true
		}
		a := latch.Read(g.Merge(c, v, .1, 0, 0, false, nil))
		if e := c.Handle(a); e != nil {
			t.Fatal(e)
		}
		if e := c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
		elapsed += 100 * time.Millisecond
	}
	dispatch(false)
	dispatch(false)
	state := b.s.States[9]
	state.Buttons[4], state.Buttons[5] = true, true
	b.s.States[9] = state
	dispatch(false)
	r := c.Rendering.Flight
	if r.Defenses.IRActive == 0 || r.Defenses.RadarSignature == 0 {
		t.Fatal("controller press not active", r.Defenses)
	}
	dispatch(true)
	state.Buttons[4], state.Buttons[5] = false, false
	b.s.States[9] = state
	dispatch(true)
	if r.Defenses.IRActive == 0 || r.Defenses.RadarSignature == 0 {
		t.Fatal("controller release lost held keyboard")
	}
	dispatch(false)
	if r.Defenses.IRActive != 0 || r.Defenses.RadarSignature != 0 {
		t.Fatal("last release latched", r.Defenses)
	}
	dispatch(true)
	if e := c.openGPS(); e != nil {
		t.Fatal(e)
	}
	if r.Defenses.IRActive != 0 || r.Defenses.RadarSignature != 0 || c.Rendering.Cockpit.Defense.IRActive != 0 {
		t.Fatal("GPS left jammer/indication latched")
	}
}

func TestTrainingResolvedModesRouteJammersStrictAndPractice(t *testing.T) {
	c := renderingController(t)
	startRendering(t, c)
	jFinishFade(t, c)
	c.Handle(Actions{})
	var latch inputLatch
	var now time.Duration
	step := func(b Buttons) {
		t.Helper()
		b.InputTime = now
		now += 100 * time.Millisecond
		if e := c.Handle(latch.Read(b)); e != nil {
			t.Fatal(e)
		}
		if e := c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
	}
	step(Buttons{FlightKeys: [11]bool{false, false, true}})
	step(Buttons{}) // Existing rotor command.
	for n := 0; n < 24; n++ {
		step(Buttons{FlightKeys: [11]bool{true}})
		step(Buttons{})
	}
	for tick := 0; tick < 60; tick++ {
		axis := int16(0)
		if tick < 20 {
			axis = 48
		}
		if tick >= 40 && tick < 50 {
			axis = -48
		}
		step(Buttons{Live: &flightcontrol.LiveInput{Mode: "relative_height", Bound: [4]bool{false, false, true}, Axes: [4]int16{0, 0, axis}, Epoch: 1}, DefenseKeys: [4]bool{false, false, tick < 10, tick >= 10 && tick < 20}})
	}
	if e := c.openGPS(); e != nil {
		t.Fatal(e)
	}
	c.Rendering.Flight.gpsNeedsRelease = false
	if e := c.handleGPS(Actions{Selector1: true, Selector1Down: true, SelectorHoldValid: true, Inside: true, X: 180, Y: 45}); e != nil {
		t.Fatal(e)
	}
	if c.Rendering.Flight.Navigation[0].Index() != 0 {
		t.Fatal("mode entry placed a point")
	}
	c.handleGPS(Actions{SelectorHoldValid: true})
	for _, point := range [][2]int{{100, 70}, {110, 80}, {120, 90}} {
		if e := c.handleGPS(Actions{Selector1: true, Selector1Down: true, SelectorHoldValid: true, Inside: true, X: point[0], Y: point[1]}); e != nil {
			t.Fatal(e)
		}

		c.handleGPS(Actions{SelectorHoldValid: true})
	}
	if c.Rendering.GPS.Editing {
		t.Fatal("third point not committed")
	}
	if e := c.handleGPS(Actions{Selector2: true}); e != nil {
		t.Fatal(e)
	}
	if c.Rendering.GPS != nil {
		t.Fatal("selector2 did not exit")
	}
	for n := 0; n < 10; n++ {
		step(Buttons{})
	}
	// This diagnostic ends via the existing controlled QoL ending helper. The
	// separately required connected sortie is recorded through OS input.
	endFilm(t, c)
	f := verifyFilm(t, c)
	if f.Header.InputSchema != replay.R1InputCompatibility {
		t.Fatal("wrong profile")
	}
	before, _ := json.Marshal(c.Store.State())
	bytesBefore, e := replay.Marshal(f)
	if e != nil {
		t.Fatal(e)
	}
	q, e := NewFilmPlayback(c.Office.Assets, f, configuration.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	for n := 0; n < 10 && !q.Done; n++ {
		if e = q.Step(); e != nil {
			t.Fatal(e)
		}
	}
	if e = q.Resume(); e != nil {
		t.Fatal(e)
	}
	if e = q.HandlePractice(Actions{Live: &flightcontrol.LiveInput{Mode: "relative", Slider: true, Lever: 30000, Bound: [4]bool{false, false, true}, Epoch: 5}}); e != nil {
		t.Fatal(e)
	}
	after, _ := json.Marshal(c.Store.State())
	bytesAfter, _ := replay.Marshal(f)
	if string(before) != string(after) || string(bytesBefore) != string(bytesAfter) {
		t.Fatal("practice changed career/film")
	}
	if out := os.Getenv("GS2000_Q31_FILM_OUTPUT"); out != "" {
		if e = os.MkdirAll(out, 0700); e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(out, "resolved-training-model.g2r"), bytesBefore, 0600); e != nil {
			t.Fatal(e)
		}
	}
}
