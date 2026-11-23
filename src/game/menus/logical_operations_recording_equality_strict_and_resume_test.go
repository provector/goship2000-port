package menus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gunship2000-assets/game/configuration"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/replay"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// Same normally prepared launch, with/without the optional recorder. The
// isolated comparison has no career capability; connected final acceptance
// separately covers ordinary ending/acknowledgment. No hit/outcome is injected.
func TestLogicalOperationsRecordingEqualityStrictAndResume(t *testing.T) {
	seed := developmentScenarioController(t, "o-clean-flight")
	var launch filmLaunch
	if err := json.Unmarshal(seed.Rendering.filmRecorder.Film.Launch, &launch); err != nil {
		t.Fatal(err)
	}
	seed.AbandonFilmRecording()
	var want map[string]string
	var recorded *replay.Film
	for _, record := range []bool{true, false} {
		final, err := cloneFilmFinal(&launch.Final)
		if err != nil {
			t.Fatal(err)
		}
		c := &FrontEnd{Office: seed.Office, Runtime: launch.Runtime, Screen: ScreenScene,
			simulationProfile: replay.R4Compatibility, enemyLaunchRules: launch.EnemyRules,
			filmIsolation: &filmSessionIsolation{Mode: "R4-equivalence"},
			Mission:       &MissionSession{Data: final.Mission, Final: final, Flight: final.Flight, ReadinessRows: final.Aircraft, PilotID: final.PilotID, Settings: final.Settings}}
		if err = c.constructSimulation(final); err != nil {
			t.Fatal(err)
		}
		if record {
			c.FilmDirectory = t.TempDir()
			c.filmIsolation = nil
			c.beginFilmRecording(final)
			c.filmIsolation = &filmSessionIsolation{Mode: "R4-equivalence"}
		}
		defer c.AbandonFilmRecording()
		p := c.Rendering
		if !p.r4Mechanics {
			t.Fatal("new launch did not freeze R4")
		}
		act := func(a Actions) {
			t.Helper()
			if err := c.Handle(a); err != nil {
				t.Fatal(err)
			}
		}
		act(Actions{})
		act(Actions{ViewKey: 5, ViewShift: true})
		act(Actions{ViewKey: 1})
		act(Actions{FlightCommands: []int{2}})
		for i := 0; i < 24; i++ {
			if i == 3 {
				act(Actions{WingmanOrder: "h"})
			}
			if i == 6 {
				act(Actions{Jettison: true, OrdnanceNext: true})
			}
			if i == 8 || i == 10 {
				act(Actions{LastMessage: true})
			}
			if err := c.advanceFlight(float64(flight.StepDuration()+time.Nanosecond) / float64(time.Second)); err != nil {
				t.Fatal(err)
			}
			if p.Flight.Failure != "" {
				t.Fatal(p.Flight.Failure)
			}
		}
		if p.Initialized.SelectedStation != 0 || p.Initialized.Stores.Ships[0].Stations[0].Ammunition != 0 {
			t.Fatal("jettison cycled/retained ammo")
		}
		groups := p.FilmCheckpoint()
		if want == nil {
			want = groups
		} else if !reflect.DeepEqual(want, groups) {
			t.Fatal("recording changed authoritative state", want, groups)
		}
		if record {
			recorded = p.filmRecorder.Film // Prefix only: this clone deliberately has no ending/career owner.
			if recorded.Header.Simulation != replay.R4Compatibility {
				t.Fatal(recorded.Header.Simulation)
			}

		}
	}
	// A separate fresh normal result-capable launch supplies the complete film.
	// The isolated comparison above cannot manufacture a terminating result.
	ordinary := developmentScenarioControllerProfile(t, "o-clean-flight", replay.R4Compatibility)
	defer ordinary.AbandonFilmRecording()
	for _, a := range []Actions{{Cancel: true}, {}, {ViewKey: 5, ViewShift: true}, {ViewKey: 1}, {FlightCommands: []int{2}}} {
		if err := ordinary.Handle(a); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 24; i++ {
		a := Actions{}
		switch i {
		case 3:
			a.WingmanOrder = "h"
		case 6:
			a.Jettison = true
		case 8, 10:
			a.LastMessage = true
		}
		if err := ordinary.Handle(a); err != nil {
			t.Fatal(err)
		}
		if err := ordinary.advanceFlight(float64(flight.StepDuration()+time.Nanosecond) / float64(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	endFilm(t, ordinary)
	recorded = verifyFilm(t, ordinary)
	counts := map[int]int{}
	for _, op := range recorded.Operations {
		if op.Kind == "step" {
			var step filmStep
			if err := json.Unmarshal(op.Payload, &step); err != nil {
				t.Fatal(err)
			}
			for _, aux := range step.Input.Aux {
				counts[aux]++
			}
		}
	}
	if counts[11] != 2 || counts[12] != 1 || counts[7] != 0 {
		t.Fatal("logical operations not exact", counts)
	}
	if !recorded.ReadyRepairsMechanics() {
		t.Fatal(recorded.Header)
	}
	if out := os.Getenv("GS2000_R4_OUTPUT"); out != "" {
		if err := os.MkdirAll(out, 0700); err != nil {
			t.Fatal(err)
		}
		raw, err := replay.Marshal(recorded)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(out, "r4-logical-operations.g2r"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, scale := range []int{1, 2} {
		opt := configuration.Defaults()
		opt.DisableEnemyAI = !launch.Runtime.DisableEnemyAI
		opt.DisableWingmenAI = !launch.Runtime.DisableWingmenAI
		opt.SimulationRenderScale = scale
		q, err := NewFilmPlayback(seed.Office.Assets, recorded, opt)
		if err != nil {
			t.Fatal(err)
		}
		for q.reachedSteps < 12 {
			if err = q.Step(); err != nil {
				t.Fatal(err)
			}
		}
		before := q.Preview().FilmCheckpoint()
		if err = q.Resume(); err != nil {
			t.Fatal(err)
		}
		if !q.Practice || q.Engine.Store != nil || q.Engine.Mission.ResultLaunch != nil {
			t.Fatal("Resume gained career authority")
		}
		if q.Preview().Initialized.Stores.Ships[0].Stations[0].Ammunition != 0 || !q.Preview().r4Mechanics {
			t.Fatal("Resume lost jettison/R4")
		}
		if before["combat"] != q.Preview().FilmCheckpoint()["combat"] {
			t.Fatal("Resume changed combat")
		}
	}
}

func TestDemoCreditCenteredScaledAndPresentationOnly(t *testing.T) {
	c, f := viewerFilm(t)
	openViewer(t, c, f)
	c.activateReplayControl(17)
	p := c.Viewer.Playback.Preview()
	before := p.FilmCheckpoint()
	for _, scale := range []int{1, 2} {
		p.Cockpit.MissionCounter = 0x104
		frame := bytes.Repeat([]byte{17}, 64000*scale*scale)
		c.drawReplayDemo(frame, scale)
		left, right, count := 320*scale, -1, 0
		for y := 176 * scale; y < 188*scale; y++ {
			for x := 0; x < 320*scale; x++ {
				if frame[y*320*scale+x] == 14 {
					left = min(left, x)
					right = max(right, x)
					count++
				}
			}
		}
		if count == 0 || left < 0 || right >= 320*scale || absInt((left+right)-319*scale) > 6*scale {
			t.Fatal("credit bounds/center", scale, left, right, count)
		}
		p.Cockpit.MissionCounter = 0x100
		off := bytes.Repeat([]byte{17}, len(frame))
		c.drawReplayDemo(off, scale)
		if !bytes.Equal(off, bytes.Repeat([]byte{17}, len(off))) {
			t.Fatal("credit outside original DEMO text phase")
		}
		if out := os.Getenv("GS2000_R4_OUTPUT"); out != "" {
			if err := os.MkdirAll(out, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, fmt.Sprintf("demo-credit-%dx.indices", scale)), frame, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !reflect.DeepEqual(before, p.FilmCheckpoint()) {
		t.Fatal("DEMO credit changed logical state")
	}
}

func TestNewLaunchPreservesThreeNaturalCPGModes(t *testing.T) {
	for _, choice := range []int{2, 1, 0} {
		t.Run(fmt.Sprint(choice), func(t *testing.T) {
			c := naturalCPGController(t, choice, replay.R4AuthoredCompatibility)
			p := c.Rendering
			if !p.r4Mechanics {
				t.Fatal("new interpretation not frozen")
			}
			for i := 0; i < 32; i++ {
				if err := c.advanceFlight(float64(flight.StepDuration()+time.Nanosecond) / float64(time.Second)); err != nil {
					t.Fatal(err)
				}
				if p.Flight.commandInput.Trigger {
					t.Fatal("manual trigger supplied")
				}
			}
			shots := 0
			for _, event := range p.Flight.CombatEvents {
				if event.Kind == "launch" && event.Actor == 0 {
					shots++
				}
			}
			if choice == 0 && shots == 0 || choice != 0 && shots != 0 {
				t.Fatal("automatic selected weapon", choice, shots)
			}
			if choice == 2 && p.Flight.Defenses.Flares != 30 || choice != 2 && p.Flight.Defenses.Flares >= 30 {
				t.Fatal("automatic cartridge owner", choice, p.Flight.Defenses)
			}
		})
	}
}
