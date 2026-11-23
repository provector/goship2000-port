package flightcontrol

import (
	"gunship2000-assets/game/flight"
	"testing"
)

func TestKeyboardOwnsParkedCollectiveUntilFreshGesture(t *testing.T) {
	for _, slider := range []bool{false, true} {
		var owner KeyboardPickup
		var state LiveState
		input := LiveInput{Mode: "relative_height", Slider: slider, Lever: 32000, Epoch: 1}
		input.Bound[2] = true
		for i := 0; i < 12; i++ {
			input.Axes[0]++
			input.Lever += 12
			got := owner.Filter(&input, i < 3, 22, &state)
			if got.Bound[2] || got.Axes[0] != input.Axes[0] {
				t.Fatal("parked lever reacquired or cyclic dropped", i)
			}
		}
		owner.Filter(nil, false, 22, &state)
		input.Epoch++
		input.Lever = 61000
		input.Axes[2] = 95
		if owner.Filter(&input, false, 22, &state).Bound[2] {
			t.Fatal("reconnect jump")
		}
		input.Lever = 32000
		input.Axes[2] = 0
		if owner.Filter(&input, false, 22, &state).Bound[2] {
			t.Fatal("neutral poll stole ownership")
		}
		input.Lever = 46000
		input.Axes[2] = 45
		if !owner.Filter(&input, false, 22, &state).Bound[2] {
			t.Fatal("gesture did not regain configured mode")
		}
	}
}

func TestKeyboardCollectiveAgainstAbsentDevice(t *testing.T) {
	p, _ := heightFixture(t)
	for _, damaged := range []bool{false, true} {
		var pad LiveState
		var o KeyboardPickup
		idle := flight.State{Collective: 12, Rotor: 1, RadarAltitude: 55, Vertical: -3}
		actual, older := idle, idle
		var prior LiveState
		var oldAxes Axes
		witnessed := false
		var axes Axes
		in := LiveInput{Mode: "relative_height", Slider: true, Epoch: 1, Lever: 32768}
		in.Bound[2] = true
		var damage [10]byte
		if damaged {
			damage[9] = 2
		}
		for i := 0; i < 50; i++ {
			key := i%8 == 0 || i < 4
			if key {
				for _, f := range []*flight.State{&idle, &actual, &older} {
					control := State{Collective: f.Collective, Rotor: f.Rotor}
					control.Apply(CollectiveUp)
					f.Collective = control.Collective
				}
			}
			in.Lever = uint16(32768 + i%9)
			older.Collective = prior.ApplyQ32Damage(in, &oldAxes, &older, key, true, damage, 0)
			prior.ApplyHeightQ32Damage(in, &older, p, true, key, damage, 0)
			if !witnessed && older.Collective != idle.Collective {
				t.Logf("U2.5 controller at tick %d: parked slider %d, keyboard alone %d (damage=%t)", i, older.Collective, idle.Collective, damaged)
				witnessed = true
			}
			masked := o.Filter(&in, key, actual.Collective, &pad)
			actual.Collective = pad.ApplyQ32Damage(masked, &axes, &actual, key, true, damage, 0)
			pad.ApplyHeightQ32Damage(masked, &actual, p, true, key, damage, 0)
			if actual.Collective != idle.Collective {
				t.Fatalf("tick %d, damage %t: keyboard %d, parked device %d", i, damaged, idle.Collective, actual.Collective)
			}
		}
		if !damaged && !witnessed {
			t.Fatal("missing original ownership witness")
		}
	}
}
