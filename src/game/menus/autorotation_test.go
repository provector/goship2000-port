package menus

import (
	"gunship2000-assets/game/replay"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

// Rotor=0 is the source engine-disengaged/freewheel control, not a rotor RPM
// measurement. The native per-instance rotor painter must keep airborne blades
// turning. This does not alter the canonical model arena or flight equations.
func TestAirborneFreewheelUsesRunningRotorPresentation(t *testing.T) {
	c := freshCompressionController(t, gamestate.MissionTraining, true)
	p, r := c.Rendering, c.Rendering.Flight
	id := p.Initialized.PlayerID
	if p.nativeRotorRunning()[id] {
		t.Fatal("genuinely stopped pad rotor")
	}
	c.Handle(Actions{FlightCommands: []int{2}})
	c.Handle(Actions{})
	for n := 0; n < 60; n++ {
		c.Handle(Actions{FlightCommands: []int{0}})
		if err := c.advanceFlight(.1); err != nil {
			t.Fatal(err)
		}
	}
	if r.State.RadarAltitude <= 2 || !p.nativeRotorRunning()[id] {
		t.Fatal("normal takeoff", r.State.RadarAltitude, r.State.Rotor)
	}
	c.Handle(Actions{})
	c.Handle(Actions{FlightCommands: []int{2}})
	for n := 0; n < 10 && r.State.Rotor != 0; n++ {
		if err := c.advanceFlight(.1); err != nil {
			t.Fatal(err)
		}
	}
	c.Handle(Actions{})
	if r.State.Rotor != 0 || !p.nativeRotorRunning()[id] {
		t.Fatal("freewheeling rotor was painted stopped", r.State.Rotor, r.State.RadarAltitude)
	}
	// The same state in a historical reader keeps its recorded presentation
	// interpretation. No new state, random draw or physics update is performed.
	old := *p
	historical := &replay.Film{Header: replay.Header{Simulation: replay.R23Compatibility}}
	old.threatFeedbackMechanics = historical.AircraftFeedbackMechanics()
	if old.nativeRotorRunning()[id] || r.State.Rotor != 0 {
		t.Fatal("historical gate or presentation purity")
	}
}
