package menus

import (
	gamestate "gunship2000-assets/game/state"
	"reflect"
	"testing"
)

func TestSalvoOriginalStrictThresholdsAndNoStoreMutation(t *testing.T) {
	for _, tc := range []struct {
		size, ammo int16
		flags      uint16
		want       int16
	}{
		{1, 0, 0, 1}, {2, 3, 0x1000, 1}, {2, 4, 0x1000, 2}, {4, 8, 0x1000, 1}, {4, 9, 0x1000, 4},
		{2, 100, 0, 1}, {4, -1, 0x1000, 1}, {3, 100, 0x1000, 1},
	} {
		stores := &gamestate.RuntimeStores{Ships: []gamestate.RuntimeAircraft{{Stations: [4]gamestate.RuntimeStation{{Ammunition: tc.ammo, Flags: tc.flags}}}}}
		p := &RenderingPreview{Flight: &FlightRuntime{RocketSalvo: 1}, Initialized: &gamestate.InitializedMission{Stores: stores}}
		before := stores.Ships[0]
		p.selectRocketSalvo(tc.size)
		if p.Flight.RocketSalvo != tc.want || stores.Ships[0] != before {
			t.Fatal(tc, p.Flight.RocketSalvo, stores.Ships[0])
		}
	}
}

func TestSalvoPhysicalLatchAndTimestampedSelection(t *testing.T) {
	var latch inputLatch
	q := flightCommandInput{Capacity: 64}
	for i, want := range []int{1, 2, 4} {
		keys := [3]bool{}
		keys[i] = true
		a := latch.Read(Buttons{RocketSalvoKeys: keys})
		if a.RocketSalvo != want {
			t.Fatal(i, a)
		}
		q.SampleAt(0, a)
		q.NextAt(0)
		if got := q.TakeAux(); !reflect.DeepEqual(got, []int{8 + i}) {
			t.Fatal(i, got)
		}
		if a = latch.Read(Buttons{RocketSalvoKeys: keys}); a.RocketSalvo != 0 {
			t.Fatal("held row key repeated without a new activation", a)
		}
		latch.Read(Buttons{})
	}
	// Backspace keeps its expansion target/standby ownership, not salvo cycle.
	a := latch.Read(Buttons{Backspace: true})
	q.SampleAt(0, a)
	q.NextAt(0)
	if got := q.TakeAux(); !reflect.DeepEqual(got, []int{4}) {
		t.Fatal(got)
	}
}
