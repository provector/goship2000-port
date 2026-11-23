package menus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
)

func jFinishFade(t *testing.T, c *FrontEnd) {
	t.Helper()
	for i := 0; i < 100 && c.fade != nil; i++ {
		if err := c.Tick(); err != nil {
			t.Fatal(err)
		}
	}
}
func jStep(t *testing.T, c *FrontEnd, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := c.advanceFlight(.1); err != nil {
			t.Fatal(err)
		}
		if c.Rendering.Flight.Failure != "" {
			t.Fatal(c.Rendering.Flight.Failure)
		}
	}
}
func jCommand(t *testing.T, c *FrontEnd, key int) {
	t.Helper()
	if err := c.Handle(Actions{FlightCommands: []int{key}}); err != nil {
		t.Fatal(err)
	}
	jStep(t, c, 1)
}

func TestJGeneratedFlightStartsAndPublication(t *testing.T) {
	var results []map[string]any
	for _, start := range []string{"land", "carrier", "pass"} {
		for aircraft := 0; aircraft < 8; aircraft++ {
			t.Run(fmt.Sprintf("%s-%d", start, aircraft), func(t *testing.T) {
				c := mastFixture(t, aircraft, start)
				jFinishFade(t, c)
				p := c.Rendering
				r := p.Flight
				mission, _ := json.Marshal(c.Mission.Data)
				saved := c.Store.State()
				other := make([]any, 0)
				for i, e := range p.Initialized.Entities {
					if i != p.Initialized.PlayerRow {
						other = append(other, e)
					}
				}
				before := *r.State
				if r.State.HeightControl != 0 {
					jCommand(t, c, 10)
				} // Release source pass-through altitude hold before commanding climb.
				if r.State.Rotor == 0 {
					jCommand(t, c, 2)
				}
				for i := 0; i < 8; i++ {
					jCommand(t, c, 0x100)
				} // source Shift+=, bounded31.
				jStep(t, c, 30)
				if r.State.Terminal != nil {
					t.Fatalf("light accepted start unexpectedly terminal%+v", r.State.Terminal)
				}
				if r.State.Pose.Height <= before.Pose.Height {
					t.Fatalf("source max-collective did not climb: before%+v after%+v", before, *r.State)
				}
				if p.Initialized.PlayerFlight != r.State || p.Initialized.Player.H0 != r.State.Pose.H0 || p.Initialized.Entities[0].Pose.Height != uint16(r.State.Pose.Height) {
					t.Fatal("split authoritative player publication")
				}
				if p.Cockpit.Vertical != r.State.IndicatedSpeed || p.Cockpit.Lateral != r.State.Vertical || p.Cockpit.HUDScale != r.State.RadarAltitude || p.Cockpit.Rotor != r.State.Rotor {
					t.Fatal("instrument domain mismatch")
				}
				c.Frame(false)
				if p.Error != nil {
					t.Fatalf("actual cockpit composition: %v", p.Error)
				}
				if dir := os.Getenv("GS2000_J_TRACES"); dir != "" {
					mastCapture(t, c, dir, fmt.Sprintf("flight-%s-%d", start, aircraft))
				}
				at := 0
				for i, e := range p.Initialized.Entities {
					if i != p.Initialized.PlayerRow {
						expected := other[at].(gamestate.InitializedEntity)
						if i >= 1 && i <= 4 && expected.Kind == 0x21 && p.Flight.Combat.FormationEnabled() {
							// P.3 supersedes Step J's stationary kind21 exclusion.
							// This publication test still checks every identity,
							// OBJECT, CONTROL and non-motion byte. Exact geometry
							// and shared draws have independent formation tests.
							expected.Pose, expected.RawMotion = e.Pose, e.RawMotion
							copy(expected.DynamicRaw[6:], e.DynamicRaw[6:])
						}
						if !reflect.DeepEqual(expected, e) {
							t.Fatalf("unadmitted entity/owner advanced row%d: before%+v after%+v", i, expected, e)
						}
						at++
					}
				}
				got, _ := json.Marshal(c.Mission.Data)
				if string(got) != string(mission) || !reflect.DeepEqual(saved, c.Store.State()) {
					t.Fatal("flight edited mission generation or pilot state")
				}
				if backup, err := c.Store.RecoveryCandidate(); err != nil || backup != nil {
					t.Fatal("flight created backup", err)
				}
				results = append(results, map[string]any{"start": start, "aircraft": aircraft, "before": before, "after": *r.State, "steps": r.Steps, "mass": r.MassTerm, "systems": r.Systems, "clock_policy": flight.NativeClockPolicy})
			})
		}
	}
	if dir := os.Getenv("GS2000_J_TRACES"); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		b, _ := json.MarshalIndent(results, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "generated-starts.json"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestJFlightPauseModalAndOrdinaryKeys(t *testing.T) {
	c := mastFixture(t, 6, "land")
	jFinishFade(t, c)
	p := c.Rendering
	r := p.Flight
	jCommand(t, c, 2)
	jCommand(t, c, 0x100)
	var w windowInput
	feed := func(b Buttons) {
		t.Helper()
		if err := c.Handle(w.ReadViewport(b, officeViewport(960, 720, false))); err != nil {
			t.Fatal(err)
		}
	}
	feed(Buttons{FlightPause: true, AnyKeyPressed: true})
	before := *r.State
	steps := r.Steps
	phase := p.Visual.Phase
	if !r.Paused {
		t.Fatal("AltP not paused")
	}
	if err := c.AdvanceRendering(10); err != nil {
		t.Fatal(err)
	}
	if *r.State != before || steps != r.Steps || phase != p.Visual.Phase {
		t.Fatal("pause advanced flight/presentation")
	}
	feed(Buttons{FlightKeys: [11]bool{true}, AnyKeyPressed: true})
	feed(Buttons{})
	if r.Paused || len(r.commandInput.queue) != 0 || len(r.commandInput.samples) != 0 || r.Axes.X != 0 {
		t.Fatal("resume key leaked")
	}
	jStep(t, c, 1)
	feed(Buttons{AltQ: true, SceneDirections: [4]bool{true}})
	feed(Buttons{})
	before = *r.State
	steps = r.Steps
	feed(Buttons{Text: []rune{'N'}, AnyKeyPressed: true})
	feed(Buttons{})
	if c.Screen != ScreenScene || *r.State != before || r.Steps != steps || r.held.Left {
		t.Fatal("modal mutated or retained flight input")
	}
	feed(Buttons{Enter: true})
	feed(Buttons{})
	feed(Buttons{Escape: true})
	feed(Buttons{})
	if c.Screen != ScreenScene {
		t.Fatal("temporary preview return intercepted flight key")
	}
	feed(Buttons{AltE: true})
	feed(Buttons{})
	feed(Buttons{ConfirmQuit: true})
	waitForResultVerification(t, c)
	if c.Screen != ScreenBriefing {
		t.Fatal("AltE safe return missing")
	}
}

func TestJLogicalFlightIndependentOfDrawing(t *testing.T) {
	run := func(frames int) (flight.State, []int16) {
		c := mastFixture(t, 0, "land")
		jFinishFade(t, c)
		jCommand(t, c, 2)
		for i := 0; i < 5; i++ {
			jCommand(t, c, 0x100)
		}
		for i := 0; i < frames; i++ {
			if err := c.Handle(Actions{SceneDirections: [4]bool{false, false, true, false}}); err != nil {
				t.Fatal(err)
			}
			if err := c.advanceFlight(3.0 / float64(frames)); err != nil {
				t.Fatal(err)
			}
			if i%7 == 0 {
				before := *c.Rendering.Flight.State
				c.Frame(false)
				c.Frame(false)
				if before != *c.Rendering.Flight.State {
					t.Fatal("draw integrated state")
				}
			}
		}
		return *c.Rendering.Flight.State, c.Rendering.Flight.Systems.Fuel
	}
	a, af := run(90)
	b, bf := run(432)
	if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(af, bf) {
		t.Fatalf("render partitions differ: %+v vs%+v", a, b)
	}
}

func TestJFourTheatresAndSettingsSnapshot(t *testing.T) {
	for _, theatre := range []gamestate.Theatre{gamestate.TheatreCentralEurope, gamestate.TheatrePersianGulf, gamestate.TheatreAntarctica, gamestate.TheatrePhilippines} {
		t.Run(fmt.Sprint(theatre), func(t *testing.T) {
			c := renderingController(t)
			next := c.Store.State()
			pilot := next.SelectedPilot()
			pilot.Theatre = theatre
			pilot.MissionType = gamestate.MissionSingle
			if err := c.Store.Commit(next); err != nil {
				t.Fatal(err)
			}
			if err := c.RefreshOffice(); err != nil {
				t.Fatal(err)
			}
			seed := int64(1)
			c.MissionSeed = &seed
			readyMission(t, c)
			c.Mission.Settings.FlightModel = 1
			c.Mission.Settings.Wind = 1
			c.Mission.Settings.Avoidance = 1
			c.Mission.Settings.Landings = 1
			startRendering(t, c)
			jFinishFade(t, c)
			// Source CMP1/SBB/NEG sets20/40 for ordinal0 only; realistic
			// landings/avoidanceOFF1 retain neither. Wind80/realisticFlight4 set.
			if c.Rendering.Initialized.Switches&0xe4 != 0x84 {
				t.Fatal("settings launch bits not preserved")
			}
			before := c.Mission.Data.ObjectHash
			sourceWind := c.Mission.Data.Conditions.WindSpeed
			if c.Rendering.Flight.WindStrength != int16(sourceWind)/2 {
				t.Fatal("wind launch conversion changed source condition")
			}
			jStep(t, c, 20)
			c.Frame(false)
			if c.Rendering.Error != nil || c.Mission.Data.ObjectHash != before {
				t.Fatal("world/conditions changed", c.Rendering.Error)
			}
		})
	}
}

func TestJTerminalDiscardsRemainingHostQuanta(t *testing.T) {
	c := mastFixture(t, 0, "land")
	jFinishFade(t, c)
	p := c.Rendering
	r := p.Flight
	p.Initialized.Switches &^= 0x20
	r.State.Pose.Height = 3
	r.State.Vertical, r.State.Rotor = -20, 1
	before := r.Steps
	if err := c.advanceFlight(.25); err != nil {
		t.Fatal(err)
	}
	if r.State.Terminal == nil || r.Steps != before+1 {
		t.Fatalf("contact did not stop remaining host quanta: terminal%+v steps%d", r.State.Terminal, r.Steps-before)
	}
	saved, systems := *r.State, r.Systems.Countdown
	if err := c.advanceFlight(.25); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved, *r.State) || systems != r.Systems.Countdown || r.Steps != before+1 {
		t.Fatal("terminal sortie continued updating")
	}
}
