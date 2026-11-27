package menus

import (
	"encoding/json"
	"fmt"
	assetdata "gunship2000-assets/game/assets"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/replay"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActualCleanLaunchLogicalFilm(t *testing.T) {
	c := renderingController(t)
	startRendering(t, c)
	jFinishFade(t, c)
	p := c.Rendering
	if p.filmRecorder == nil {
		t.Fatal("real launch did not attach recording")
	}
	c.Handle(Actions{})
	c.Handle(Actions{FlightCommands: []int{2}}) // ordinary rotor engagement command9
	for i := 0; i < 20; i++ {
		if i == 2 {
			c.Handle(Actions{FlightCommands: []int{256}})
		}
		if i == 3 {
			c.Handle(Actions{ViewKey: 4})
		}
		if i == 5 {
			c.Handle(Actions{ViewKey: 1})
		}
		if i == 8 {
			c.Handle(Actions{SpeechToggle: true})
		}
		if e := c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
		if c.Screen == ScreenQuit {
			c.cancelNaturalSortieEnd()
			c.Screen = ScreenScene
			p.clearFlightInput()
		}
	}
	c.Rendering.Initialized.EndSortieForQoL()
	if e := c.finishSortieResult(); e != nil {
		t.Fatal(e)
	}
	f, e := c.SealRecordedFilm()
	if e != nil {
		t.Fatal(e)
	}
	if f == nil {
		t.Fatal("film missing")
	}
	b, e := replay.Marshal(f)
	if e != nil {
		t.Fatal(e)
	}
	f, e = replay.Unmarshal(b)
	if e != nil {
		t.Fatal(e)
	}
	for _, scale := range []int{1, 2} {
		opt := configuration.Defaults()
		opt.SimulationRenderScale = scale
		q, e := NewFilmPlayback(c.Office.Assets, f, opt)
		if e != nil {
			t.Fatal(e)
		}
		if q.Engine.Store != nil {
			t.Fatal("playback acquired career capability")
		}
		for !q.Done {
			if e = q.Step(); e != nil {
				t.Fatal(scale, e)
			}
		}
		if q.Divergence != "" {
			t.Fatal(q.Divergence)
		}
	}
	if path := t.TempDir(); path != "" {
		s := replay.Store{Root: path}
		if e = s.PublishLast(f); e != nil {
			t.Fatal(e)
		}
		g, e := s.Load(f.Header.ID)
		if e != nil {
			t.Fatal(e)
		}
		if g.Header.ID != f.Header.ID {
			t.Fatal("film changed")
		}
	}
}

func TestInputEdgesAndResumeHaveNoStore(t *testing.T) {
	c := renderingController(t)
	startRendering(t, c)
	jFinishFade(t, c)
	c.Handle(Actions{})
	c.Handle(Actions{FlightCommands: []int{2}})
	for i := 0; i < 10; i++ {
		if e := c.advanceFlight(.1); e != nil {
			t.Fatal(e)
		}
		if c.Screen != ScreenScene {
			t.Fatal(c.Screen)
		}
	}
	c.Rendering.Initialized.EndSortieForQoL()
	if e := c.finishSortieResult(); e != nil {
		t.Fatal(e)
	}
	f, e := c.SealRecordedFilm()
	if e != nil {
		t.Fatal(e)
	}
	before, _ := json.Marshal(c.Store.State())
	filmBefore, _ := replay.Marshal(f)
	q, e := NewFilmPlayback(c.Office.Assets, f, configuration.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 4; i++ {
		if e = q.Step(); e != nil {
			t.Fatal(e)
		}
	}
	if e = q.Resume(); e != nil {
		t.Fatal(e)
	}
	if e = q.HandlePractice(Actions{}); e != nil {
		t.Fatal(e)
	}
	if e = q.AdvancePractice(.1); e != nil {
		t.Fatal(e)
	}
	if e = q.HandlePractice(Actions{GPSOpen: true}); e != nil {
		t.Fatal(e)
	}
	if q.Preview().GPS == nil {
		t.Fatal("practice GPS not available")
	}
	if e = q.HandlePractice(Actions{GPSKey: "f"}); e != nil {
		t.Fatal(e)
	}
	if e = q.HandlePractice(Actions{Inside: true, X: 112, Y: 80, Selector1: true}); e != nil {
		t.Fatal(e)
	}
	if e = q.HandlePractice(Actions{ViewKey: 1}); e != nil {
		t.Fatal(e)
	}
	if q.Preview().GPS != nil {
		t.Fatal("practice GPS could not close")
	}
	if e = q.HandlePractice(Actions{EndMission: true}); e != nil {
		t.Fatal(e)
	}
	if e = q.HandlePractice(Actions{ConfirmQuit: true}); e != nil {
		t.Fatal(e)
	}
	if !q.Engine.filmIsolation.Done || q.Engine.Store != nil || q.Engine.Mission.ResultLaunch != nil {
		t.Fatal("practice escaped capability")
	}
	after, _ := json.Marshal(c.Store.State())
	filmAfter, _ := replay.Marshal(f)
	if string(before) != string(after) || string(filmBefore) != string(filmAfter) {
		t.Fatal("practice changed career/film")
	}
}

func verifyFilm(t *testing.T, c *FrontEnd) *replay.Film {
	t.Helper()
	f, e := c.SealRecordedFilm()
	if e != nil {
		t.Fatal(e)
	}
	if f == nil {
		t.Fatal("no sealed film")
	}
	for _, scale := range []int{1, 2} {
		opt := configuration.Defaults()
		opt.SimulationRenderScale = scale
		q, e := NewFilmPlayback(c.Office.Assets, f, opt)
		if e != nil {
			t.Fatal(e)
		}
		for !q.Done {
			if e = q.Step(); e != nil {
				t.Fatal(e)
			}
		}
	}
	return f
}
func endFilm(t *testing.T, c *FrontEnd) {
	t.Helper()
	if c.Rendering != nil {
		c.Rendering.Initialized.EndSortieForQoL()
		if e := c.finishSortieResult(); e != nil {
			t.Fatal(e)
		}
	}
}
func TestTerminalActualDamageFilm(t *testing.T) {
	c := historicalCriticalFuelScenario(t, "o-terminal")
	p := c.Rendering
	if e := c.Handle(Actions{Cancel: true}); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 1000 && c.Rendering != nil; i++ {
		if e := c.AdvanceRendering(.12); e != nil {
			t.Fatal(e)
		}
	}
	if c.Rendering != nil {
		t.Fatalf("terminal did not end %+v", p.Flight.State.Terminal)
	}
	f := verifyFilm(t, c)
	found := false
	for _, o := range f.Operations {
		if o.Kind == "terminal" {
			found = true
		}
	}
	if f.Header.Simulation != replay.V45Compatibility {
		t.Fatalf("wrong recorded fuel policy: %s", f.Header.Simulation)
	}
	if !found {
		t.Fatal("critical accepted damage did not execute terminal iterations")
	}
}
func TestCargoActualPickupAndDropFilm(t *testing.T) {
	c := developmentScenarioController(t, "o-cargo")
	p := c.Rendering
	if e := c.Handle(Actions{Cancel: true}); e != nil {
		t.Fatal(e)
	}
	if e := c.AdvanceRendering(.12); e != nil {
		t.Fatal(e)
	}
	if p.Initialized.Stores.Ships[0].Flags&0x18 == 0 {
		t.Fatal("real pickup did not load cargo")
	}
	if e := c.Handle(Actions{}); e != nil {
		t.Fatal(e)
	}
	if e := c.Handle(Actions{GPSOpen: true}); e != nil {
		t.Fatal(e)
	}
	if p.GPS == nil {
		t.Fatalf("GPS did not open screen=%v terminal=%+v paused=%v pending=%+v", c.Screen, p.Flight.State.Terminal, p.Paused, p.Initialized.Sortie)
	}
	// Select the actual player detail before ordinary cargo-drop input.
	p.GPS.Selected = 0
	if e := c.Handle(Actions{GPSKey: "c"}); e != nil {
		t.Fatal(e)
	}
	if p.Initialized.Stores.Ships[0].Flags&0x18 != 0 {
		t.Fatal("real GPS drop did not unload")
	}
	if e := c.Handle(Actions{ViewKey: 1}); e != nil {
		t.Fatal(e)
	}
	endFilm(t, c)
	f := verifyFilm(t, c)
	found := false
	for _, o := range f.Operations {
		for _, v := range o.Events {
			if v.Kind == 4 || v.Kind == 6 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("pickup/objective event missing")
	}
}
func TestCombatActualFiringDestructionFilm(t *testing.T) {
	c := developmentScenarioController(t, "o-combat")
	p, r := c.Rendering, c.Rendering.Flight
	if e := c.Handle(Actions{Cancel: true}); e != nil {
		t.Fatal(e)
	}
	tick := func(a Actions) {
		t.Helper()
		if e := c.Handle(a); e != nil {
			t.Fatal(e)
		}
		if e := c.AdvanceRendering(.12); e != nil {
			t.Fatal(e)
		}
	}
	tick(Actions{WeaponTriggerValid: true})
	for i := 0; i < 4; i++ {
		s := p.Initialized.Stores.Ships[0].Stations[p.Initialized.SelectedStation]
		if strings.Contains(strings.ToUpper(s.ShortName), "HELL") {
			break
		}
		tick(Actions{OrdnanceNext: true, WeaponTriggerValid: true})
	}
	for i := 0; i < 1000 && !r.Combat.Primary.Achieved && c.Rendering != nil; i++ {
		if r.Avionics.Target == 0 || i%20 == 0 {
			tick(Actions{WeaponTriggerValid: true, Backspace: true})
		} else {
			tick(Actions{WeaponTriggerValid: true, WeaponTriggerHeld: true})
		}
	}
	if !r.Combat.Primary.Achieved {
		t.Fatalf("genuine fire failed target=%d primary=%+v", r.Avionics.Target, r.Combat.Primary)
	}
	endFilm(t, c)
	f := verifyFilm(t, c)
	found := false
	for _, o := range f.Operations {
		for _, v := range o.Events {
			if v.Kind == 1 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("real destruction event missing")
	}
}

func TestRecordingDoesNotChangeRealMechanics(t *testing.T) {
	c := renderingController(t)
	startRendering(t, c)
	jFinishFade(t, c)
	var launch filmLaunch
	if e := json.Unmarshal(c.Rendering.filmRecorder.Film.Launch, &launch); e != nil {
		t.Fatal(e)
	}
	final, e := cloneFilmFinal(&launch.Final)
	if e != nil {
		t.Fatal(e)
	}
	d := &FrontEnd{Office: &Office{Assets: c.Office.Assets}, Runtime: launch.Runtime, filmIsolation: &filmSessionIsolation{Mode: "practice"}, Screen: ScreenScene, Mission: &MissionSession{Data: final.Mission, Final: final, Flight: final.Flight, ReadinessRows: final.Aircraft, PilotID: final.PilotID, Settings: final.Settings}}
	// Recording on/off must compare the same selected launch interpretation,
	// not the retained R12 fixture against the newly defaulted R21 profile.
	d.simulationProfile = c.Rendering.filmRecorder.Film.Header.Simulation
	d.enemyLaunchRules = launch.EnemyRules
	if e = d.constructSimulation(final); e != nil {
		t.Fatal(e)
	}
	if g := compareFilmCheckpoint(c.Rendering.FilmCheckpoint(), d.Rendering.FilmCheckpoint()); g != "" {
		t.Fatal("initial", g)
	}
	for i := 0; i < 25; i++ {
		a := Actions{}
		if i == 1 {
			a.FlightCommands = []int{2}
		}
		if i == 5 {
			a.ViewKey = 4
		}
		if i == 8 {
			a.ViewKey = 1
		}
		if i == 12 {
			a.SpeechToggle = true
		}
		for _, x := range []*FrontEnd{c, d} {
			if e = x.handleFlight(a); e != nil {
				t.Fatal(e)
			}
			if e = x.advanceFlight(.1); e != nil {
				t.Fatal(e)
			}
		}
		if g := compareFilmCheckpoint(c.Rendering.FilmCheckpoint(), d.Rendering.FilmCheckpoint()); g != "" {
			t.Fatalf("recording altered step%d group%s", i, g)
		}
	}
}
func TestUntrustedFilmWitnessesStopWithoutResume(t *testing.T) {
	c := renderingController(t)
	startRendering(t, c)
	jFinishFade(t, c)
	c.Handle(Actions{})
	c.advanceFlight(.1)
	endFilm(t, c)
	f, e := c.SealRecordedFilm()
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := replay.Marshal(f)
	for _, kind := range []string{"clock", "checkpoint", "unknown-field", "nil-world", "wrong-resource", "trailing-launch"} {
		t.Run(kind, func(t *testing.T) {
			g, e := replay.Unmarshal(raw)
			if e != nil {
				t.Fatal(e)
			}
			switch kind {
			case "clock":
				for i := range g.Operations {
					if g.Operations[i].Kind == "step" {
						var v filmStep
						json.Unmarshal(g.Operations[i].Payload, &v)
						v.Timing.DF++
						g.Operations[i].Payload = filmJSON(v)
						break
					}
				}
			case "checkpoint":
				g.Operations[0].Checkpoint["flight"] = strings.Repeat("0", 64)
			case "unknown-field":
				for i := range g.Operations {
					if g.Operations[i].Kind == "step" {
						b := string(g.Operations[i].Payload)
						g.Operations[i].Payload = []byte(b[:len(b)-1] + `,"CorrectPose":true}`)
						break
					}
				}
			case "nil-world":
				var l filmLaunch
				json.Unmarshal(g.Launch, &l)
				l.Final.Mission.SourceWorld = nil
				g.Launch = filmJSON(l)
			case "wrong-resource":
				g.Header.Resources = strings.Repeat("0", 64)
			case "trailing-launch":
				g.Launch = append(g.Launch, []byte(" {}")...)
			}
			q, e := NewFilmPlayback(c.Office.Assets, g, configuration.Defaults())
			if e != nil {
				return
			}
			for !q.Done && e == nil {
				e = q.Step()
			}
			if e == nil || q.Divergence == "" {
				t.Fatal("corrupt witness played")
			}
			if q.Resume() == nil {
				t.Fatal("diverged stream resumed")
			}
		})
	}
}

func TestResourceIdentityUsesContentsNotOriginalPath(t *testing.T) {
	// P.1 supersedes the old fake-manifest success oracle. A three-field JSON
	// digest never proved that its source/output bytes or interpretation were
	// valid. Keep that original body in the P.1 evidence; real references now
	// establish location independence and the P.1 content tests guard mutation.
	a := &assetdata.OfficeAssets{Root: t.TempDir()}
	if e := os.WriteFile(filepath.Join(a.Root, "manifest.json"), []byte(`{"input_directory":"/first/original","preparation_version":24,"resources":[{"sha256":"abc"}]}`), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := filmResourceIdentity(a); e == nil {
		t.Fatal("unvalidated incomplete manifest acquired a resource interpretation")
	}
	flat, nested := os.Getenv("GS2000_P1_FLAT_ASSETS"), os.Getenv("GS2000_P1_NESTED_ASSETS")
	if flat == "" || nested == "" {
		t.Skip("unknown manifest rejection checked; explicit flat/nested fixtures required for relocation coverage")
	}
	a.Root = flat
	first, e := filmResourceIdentity(a)
	if e != nil {
		t.Fatal(e)
	}
	a.Root = nested
	second, e := filmResourceIdentity(a)
	if e != nil || first != second {
		t.Fatal("source root spelling changed validated interpretation", e)
	}
}
func TestCompressedReachedStepFilmAcrossPresentationSchedules(t *testing.T) {
	for _, fps := range []int{30, 60, 144} {
		for _, mode := range []string{"smooth", "stepped"} {
			t.Run(fmt.Sprintf("%d-%s", fps, mode), func(t *testing.T) {
				c := renderingController(t)
				startRendering(t, c)
				jFinishFade(t, c)
				p := c.Rendering
				c.Runtime.PresentationMode = mode
				p.presentation.mode = mode
				c.Handle(Actions{})
				c.Handle(Actions{CompressionIncrease: true})
				c.Handle(Actions{})
				c.Handle(Actions{CompressionIncrease: true})
				for i := 0; i < fps; i++ {
					if e := c.AdvanceRendering(1 / float64(fps)); e != nil {
						t.Fatal(e)
					}
					if i%3 == 0 {
						c.Frame(false)
					}
				}
				if p.Flight.Steps < 30 {
					t.Fatalf("compression did not advance reached steps: %d", p.Flight.Steps)
				}
				endFilm(t, c)
				f := verifyFilm(t, c)
				var last uint64
				for _, o := range f.Operations {
					if o.Kind == "step" {
						var s filmStep
						json.Unmarshal(o.Payload, &s)
						last++
						if s.Ordinal != last {
							t.Fatal("compression fictional/duplicate step")
						}
					}
				}
				if last != f.End.Steps {
					t.Fatal("step count")
				}
			})
		}
	}
}
