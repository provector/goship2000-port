package menus

import (
	"gunship2000-assets/game/flight"
	gamestate "gunship2000-assets/game/state"
	"testing"
)

func TestNativeRotorUsesActualPlayerSwitchAndFlightRows(t *testing.T) {
	p := RenderingPreview{Flight: &FlightRuntime{State: &flight.State{}}}
	p.Initialized = &gamestate.InitializedMission{PlayerID: "player", FlightRows: []int{0, 1}, Entities: []gamestate.InitializedEntity{{ID: "player", Participation: 1}, {ID: "follower", Participation: 1}, {ID: "scenery", Participation: 1}}}
	got := p.nativeRotorRunning()
	if got["player"] || !got["follower"] || len(got) != 2 {
		t.Fatal("stopped player/shared-model follower gate", got)
	}
	p.Flight.State.Rotor = 1
	got = p.nativeRotorRunning()
	if !got["player"] || !got["follower"] {
		t.Fatal("running player/follower gate", got)
	}
	p.Initialized.Entities[1].Participation = 0
	if p.nativeRotorRunning()["follower"] {
		t.Fatal("removed follower remains running")
	}
}
