package flightcontrol

import (
	"encoding/binary"
	"fmt"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"testing"
)

func TestSliderRelativeSlowMotionKeyboardAndEntry(t *testing.T) {
	s := LiveState{}
	a := Axes{}
	in := LiveInput{Mode: "relative", Slider: true, Bound: [4]bool{false, false, true}, Lever: 32000, Epoch: 1}
	c := s.ApplyQ32(in, &a, 17, false, true)
	if c != 17 {
		t.Fatal("entry jumped", c)
	}
	for i := 0; i < 200; i++ {
		in.Lever += 20
		c = s.ApplyQ32(in, &a, c, false, true)
	}
	if c <= 17 {
		t.Fatal("slow movement lost", c)
	}
	c = s.ApplyQ32(in, &a, 22, true, true)
	for i := 0; i < 100; i++ {
		c = s.ApplyQ32(in, &a, c, false, true)
	}
	if c != 22 {
		t.Fatal("parked lever undid keyboard", c)
	}
	in.Epoch++
	in.Lever = 65535
	if c = s.ApplyQ32(in, &a, c, false, true); c != 22 {
		t.Fatal("reconnect jumped")
	}
}
func TestHeightSliderParkedEntryAndKeyboard(t *testing.T) {
	s := LiveState{}
	a := Axes{}
	in := LiveInput{Mode: "relative_height", Slider: true, Bound: [4]bool{false, false, true}, Lever: 65535, Epoch: 1}
	f := flight.State{Collective: 18, Rotor: 1, RadarAltitude: 200}
	p := flight.Parameters{}
	for i := 0; i < 20; i++ {
		f.Collective = s.ApplyQ32(in, &a, f.Collective, false, true)
		s.ApplyHeightQ32(in, &f, p, true, false)
	}
	if f.Collective != 18 || s.HeightSliderReady {
		t.Fatal("parked entry commanded", f.Collective, s)
	}
	in.Lever = 32768
	f.Collective = s.ApplyQ32(in, &a, f.Collective, false, true)
	s.ApplyHeightQ32(in, &f, p, true, false)
	if !s.HeightSliderReady {
		t.Fatal("midpoint did not arm")
	}
	in.Lever = 60000
	f.Collective = s.ApplyQ32(in, &a, 19, true, true)
	s.ApplyHeightQ32(in, &f, p, true, true)
	if s.HeightSliderReady || f.Collective != 19 {
		t.Fatal("manual not preserved")
	}
	if sliderHeightAxis(0) != -127 || sliderHeightAxis(65535) != 127 || sliderHeightAxis(32768) != 0 {
		t.Fatal("midpoint range")
	}
}
func TestSliderHeightReleaseRetainsBDAArrest(t *testing.T) {
	raw, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	b := &world.Bundle{Sine: make([]int16, 1025)}
	for i := range b.Sine {
		b.Sine[i] = int16(binary.LittleEndian.Uint16(raw[0x4bc50+2*i:]))
	}
	queries := flight.Queries{Angles: b, Height: func(flight.Pose) (int16, error) { return 0, nil }, Cell: func(flight.Pose) (byte, error) { return 0, nil }}
	for _, aircraft := range []string{"APA", "BLK", "COB", "COM", "DEF", "KIA"} {
		stat, e := os.ReadFile(testfixture.Path("installation/") + aircraft + "_STAT.DAT")
		if e != nil {
			t.Fatal(e)
		}
		var params flight.Parameters
		copy(params.Raw[:], stat)
		for _, axis := range []int16{-64, 64} {
			for phase := 0; phase < 2; phase++ {
				t.Run(fmt.Sprintf("%s-axis%d-phase%d", aircraft, axis, phase), func(t *testing.T) {
					f := flight.State{Pose: flight.Pose{H0: 200000, H1: 200000, Height: 500}, RadarAltitude: 498, Rotor: 1, Collective: 24, Thrust: int16(binary.LittleEndian.Uint16(params.Raw[48:]))}
					s := LiveState{}
					axes := Axes{}
					in := LiveInput{Mode: "relative_height", Bound: [4]bool{false, false, true}, Epoch: 1, Slider: true, Lever: 32768}
					step := func(tick int) {
						t.Helper()
						f.Collective = s.ApplyQ32(in, &axes, f.Collective, false, true)
						s.ApplyHeightQ32(in, &f, params, true, false)
						if e := flight.Step(&f, params, flight.Input{DF: 6, E0: (tick+phase)%2 == 0, Phase: uint16(tick), ControlFlags: 2}, queries); e != nil {
							t.Fatal(e)
						}
					}
					step(-1) // initial midpoint arms slider without commanding a jump
					for tick := 0; tick < 50; tick++ {
						in.Lever = uint16(32768 + int32(axis)*240)
						step(tick)
					}
					release := f.RadarAltitude
					velocity := f.Vertical
					in.Lever = 32768
					zeroAt := -1
					for tick := 50; tick < 180; tick++ {
						step(tick)
						if velocity > 0 && f.Vertical < 0 || velocity < 0 && f.Vertical > 0 {
							t.Fatal("release crossed VSI neutral", release, velocity, tick, f.Vertical, f.Collective)
						}
						if f.Vertical == 0 && zeroAt < 0 {
							zeroAt = tick
						}
					}
					t.Log("release", release, "stopping AGL", f.RadarAltitude, "VSI", f.Vertical, "arrest ticks", zeroAt-50)
					if zeroAt < 0 || abs32(int32(f.RadarAltitude-release)) > 24 || abs32(int32(f.RadarAltitude-s.HeightTarget)) > 2 {
						t.Fatal("unbounded arrest/hold", release, f, s)
					}
				})
			}
		}
	}
}
