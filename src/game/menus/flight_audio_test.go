package menus

import (
	"reflect"
	"testing"

	"gunship2000-assets/game/cockpit"
	"gunship2000-assets/game/flight"
)

func TestFlightRotorSourceSemanticRequests(t *testing.T) {
	p := &RenderingPreview{Flight: &FlightRuntime{State: &flight.State{Rotor: 1, Collective: 16}, Systems: flight.NewSystems([]int16{100}, []bool{true})}}
	a := &cockpit.Assets{Definitions: map[string]cockpit.Definition{}}
	// Original keyboard07f1..0817: collective17 SAR1=8, then fast
	//collective21 SAR1=10. Neither uses speed, rotor-indicator or frame count.
	p.applyFlightCommand(0, a)
	p.applyFlightCommand(0x100, a)
	if got := p.TakeRotorCommands(); !reflect.DeepEqual(got, []uint16{8, 10}) {
		t.Fatalf("collective source arguments%v", got)
	}
	if got := p.TakeRotorCommands(); len(got) != 0 {
		t.Fatal("audio events replayed")
	}
	p.applyFlightCommand(3, a)  // Hover command changes its own source state only.
	p.applyFlightCommand(10, a) // Altitude hold is not a rotor sound caller.
	if len(p.TakeRotorCommands()) != 0 {
		t.Fatal("assistance fabricated rotor requests")
	}
	p.applyFlightCommand(2, a) // Rotor OFF clears AAF and requests0.
	p.applyFlightCommand(2, a) // Rotor ON restarts the retained idle sound.
	if got := p.TakeRotorCommands(); !reflect.DeepEqual(got, []uint16{0, 1}) {
		t.Fatalf("rotor toggle arguments%v", got)
	}
	p.Flight.State.Collective = 31
	p.applyFlightCommand(0, a)
	p.applyFlightCommand(0, a)
	if got := p.TakeRotorCommands(); !reflect.DeepEqual(got, []uint16{15, 15}) {
		t.Fatalf("clamped accepted input must preserve repeated source requests:%v", got)
	}
	p.Flight.Systems.Fuel[0] = 0
	p.applyFlightCommand(1, a)
	if len(p.TakeRotorCommands()) != 0 {
		t.Fatal("fuel-empty collective emitted positive request")
	}
}

func TestFlightFuelAudioUsesOriginalOldValueBranch(t *testing.T) {
	p := &RenderingPreview{Flight: &FlightRuntime{State: &flight.State{}, Systems: flight.NewSystems([]int16{1}, []bool{true})}}
	r := p.Flight
	r.Systems.Countdown = 1
	p.fuelAudio(r.Systems.Advance(true, 16))
	if len(p.TakeRotorCommands()) != 0 {
		t.Fatal("postdecrementzero is not source burn gate")
	}
	p.fuelAudio(r.Systems.Advance(true, 16))
	if got := p.takeSceneAudio(); len(got) != 0 || r.Systems.Fuel[0] != 0 || r.Systems.Damage[9] != 0 {
		t.Fatalf("positive oldfuel1->0 is not the empty caller:%v", got)
	}
	r.Systems.Countdown = 0
	p.fuelAudio(r.Systems.Advance(true, 16))
	if got := p.TakeRotorCommands(); !reflect.DeepEqual(got, []uint16{0}) {
		t.Fatalf("empty player fuel must emit0:%v", got)
	}
	p.fuelAudio(r.Systems.Advance(false, 16))
	if len(p.TakeRotorCommands()) != 0 {
		t.Fatal("odd E0 phase emitted fuel request")
	}
	r.Systems.Active[0], r.Systems.Countdown = false, 0
	p.fuelAudio(r.Systems.Advance(true, 16))
	if len(p.TakeRotorCommands()) != 0 {
		t.Fatal("inactive slot emitted fuel request")
	}
}

func TestFuelWarningsPreserveSourceDE80AndCallOrder(t *testing.T) {
	for _, tc := range []struct {
		fuel      int16
		clearance int16
		speech    bool
		want      []sceneAudioCommand
		wantGate  bool
	}{
		{24, 3, true, []sceneAudioCommand{{event: 0x33}}, true},
		{10, 3, true, []sceneAudioCommand{{event: 0x3c}}, true},
		{25, 3, true, nil, true},
		{11, 3, true, nil, true},
		{24, 3, false, nil, false},
		{0, 3, true, []sceneAudioCommand{{event: 0x3d}, {event: 0x0b}}, false},
		{-1, 3, true, []sceneAudioCommand{{event: 0x3d}, {event: 0x0b}}, false},
		{0, 2, true, []sceneAudioCommand{{event: 0x0b}}, true},
		{0, 3, false, []sceneAudioCommand{{event: 0x0b}}, false},
	} {
		p := &RenderingPreview{Flight: &FlightRuntime{SpeechEnabled: tc.speech,
			State: &flight.State{Rotor: 1, RadarAltitude: tc.clearance}, Systems: flight.NewSystems([]int16{tc.fuel}, []bool{true})}}
		p.Flight.Systems.Countdown = 0
		p.fuelAudio(p.Flight.Systems.Advance(true, 16))
		if got := p.takeSceneAudio(); !reflect.DeepEqual(got, tc.want) || p.Flight.SpeechEnabled != tc.wantGate || p.Flight.State.Rotor != 1 || p.Flight.State.Terminal != nil {
			t.Fatalf("oldfuel%d clearance%d gate%t: audio%v gate%t", tc.fuel, tc.clearance, tc.speech, got, p.Flight.SpeechEnabled)
		}
		if len(p.takeSceneAudio()) != 0 {
			t.Fatal("repeated draw/drain replayed a logical fuel warning")
		}
	}
}

func TestEmptyFuelWarningRemainsSuppressedAcrossBurns(t *testing.T) {
	p := &RenderingPreview{Flight: &FlightRuntime{SpeechEnabled: true,
		State: &flight.State{Rotor: 1, RadarAltitude: 3}, Systems: flight.NewSystems([]int16{0}, []bool{true})}}
	for pass := 0; pass < 2; pass++ {
		p.Flight.Systems.Countdown = 0
		p.fuelAudio(p.Flight.Systems.Advance(true, 16))
		want := []sceneAudioCommand{{event: 0x0b}}
		if pass == 0 {
			want = []sceneAudioCommand{{event: 0x3d}, {event: 0x0b}}
		}
		if got := p.takeSceneAudio(); !reflect.DeepEqual(got, want) {
			t.Fatalf("source empty pass%d audio%v want%v", pass, got, want)
		}
	}
}

func TestRunningRotorFirstCollectiveRetainsIdleVoice(t *testing.T) {
	p := &RenderingPreview{Flight: &FlightRuntime{State: &flight.State{}, Systems: flight.NewSystems([]int16{100}, []bool{true})}}
	a := &cockpit.Assets{}
	for _, key := range []int{2, 0, 1, 2, 2} {
		p.applyFlightCommand(key, a)
	}
	if got := p.TakeRotorCommands(); !reflect.DeepEqual(got, []uint16{1, 1, 1, 0, 1}) {
		t.Fatal(got)
	}
}
