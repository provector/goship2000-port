package flightcontrol

import (
	"encoding/binary"
	"gunship2000-assets/game/flight"
	"testing"
)

func TestSliderDeltaStationaryAndKeyboard(t *testing.T) {
	var state LiveState
	var axes Axes
	in := LiveInput{Mode: "relative", Slider: true, Bound: [4]bool{false, false, true}, Lever: 1000, Epoch: 1}
	value := state.Apply(in, &axes, 12, false)
	for tick := 0; tick < 100; tick++ {
		value = state.Apply(in, &axes, value, false)
	}
	if value != 12 {
		t.Fatal("parked start changed collective", value)
	}
	in.Lever = 33768
	value = state.Apply(in, &axes, value, false)
	if value != 27 {
		t.Fatal("half travel delta", value)
	}
	value = state.Apply(in, &axes, 17, true)
	for tick := 0; tick < 100; tick++ {
		value = state.Apply(in, &axes, value, false)
	}
	if value != 17 {
		t.Fatal("parked slider undid keyboard", value)
	}
	in.Epoch++
	in.Lever = 0
	if v := state.Apply(in, &axes, value, false); v != 17 {
		t.Fatal("reconnect jumped", v)
	}
	in.Lever = 65535
	if v := state.Apply(in, &axes, 17, false); v != 31 {
		t.Fatal("upper clamp", v)
	}
	in.Lever = 0
	if v := state.Apply(in, &axes, 1, false); v != 0 {
		t.Fatal("lower clamp", v)
	}
}

func TestHeightAssistUsesCollectiveOnlyAndSafeEntry(t *testing.T) {
	in := LiveInput{Mode: "relative_height", Bound: [4]bool{false, false, true}, Epoch: 1}
	f := flight.State{Pose: flight.Pose{Height: 400}, RadarAltitude: 398, Rotor: 1, Collective: 24, Thrust: 768}
	s := LiveState{}
	var params flight.Parameters
	for i := 0; i < 32; i++ {
		binary.LittleEndian.PutUint16(params.Raw[i*2:], uint16(i*32))
	}
	for i := 0; i < 30; i++ {
		s.ApplyHeight(in, &f, params, true, false)
	}
	if f.Pose.Height != 400 || abs32(int32(f.Collective-24)) > 2 {
		t.Fatal("entry jumped", f)
	}
	in.Axes[2] = 127
	for i := 0; i < 4; i++ {
		s.ApplyHeight(in, &f, params, true, false)
	}
	if f.Collective <= 24 || f.Pose.Height != 400 {
		t.Fatal("climb changed pose or not collective", f)
	}
	in.Axes[2] = 0
	f.Pose.Height = 430
	f.RadarAltitude = 428
	f.Vertical = 3
	s.ApplyHeight(in, &f, params, true, false)
	if s.HeightTarget != 428 {
		t.Fatal("release target is not underlying HUD height")
	}
	f.ValidContact = true
	f.Vertical = 0
	f.Collective = 0
	s.HeightKnown = false
	for i := 0; i < 20; i++ {
		s.ApplyHeight(in, &f, params, true, false)
	}
	if f.Collective != 0 {
		t.Fatal("neutral caused liftoff")
	}
}
