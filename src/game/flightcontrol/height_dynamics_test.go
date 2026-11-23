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

func TestReleaseArrestsWithoutOppositeVerticalRate(t *testing.T) {
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
					in := LiveInput{Mode: "relative_height", Bound: [4]bool{false, false, true}, Epoch: 1}
					step := func(tick int) {
						t.Helper()
						f.Collective = s.Apply(in, &axes, f.Collective, false)
						s.ApplyHeight(in, &f, params, true, false)
						if e := flight.Step(&f, params, flight.Input{DF: 6, E0: (tick+phase)%2 == 0, Phase: uint16(tick), ControlFlags: 2}, queries); e != nil {
							t.Fatal(e)
						}
					}
					for tick := 0; tick < 50; tick++ {
						in.Axes[2] = axis
						step(tick)
					}
					release := f.RadarAltitude
					velocity := f.Vertical
					in.Axes[2] = 0
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

// Independent original APA table/trigonometry drive the unchanged dynamics;
// this is a supplied airborne model, not an actual flown sortie witness.
func TestHeightAssistOriginalEasyDynamics(t *testing.T) {
	raw, e := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if e != nil {
		t.Fatal(e)
	}
	stat, e := os.ReadFile(testfixture.Path("installation/APA_STAT.DAT"))
	if e != nil {
		t.Fatal(e)
	}
	var params flight.Parameters
	copy(params.Raw[:], stat)
	b := &world.Bundle{Sine: make([]int16, 1025)}
	for i := range b.Sine {
		b.Sine[i] = int16(binary.LittleEndian.Uint16(raw[0x4bc50+2*i:]))
	}
	queries := flight.Queries{Angles: b, Height: func(flight.Pose) (int16, error) { return 0, nil }, Cell: func(flight.Pose) (byte, error) { return 0, nil }}
	f := flight.State{Pose: flight.Pose{H0: 200000, H1: 200000, Height: 400}, RadarAltitude: 398, Rotor: 1, Collective: 24, Thrust: int16(binary.LittleEndian.Uint16(params.Raw[48:]))}
	in := LiveInput{Mode: "relative_height", Bound: [4]bool{false, false, true}, Epoch: 1}
	s := LiveState{}
	axes := Axes{}
	for tick := 0; tick < 150; tick++ {
		in.Axes[2] = 0
		if tick >= 30 && tick < 60 {
			in.Axes[2] = 64
		}
		if tick >= 100 && tick < 120 {
			in.Axes[2] = -64
		}
		f.Collective = s.Apply(in, &axes, f.Collective, false)
		s.ApplyHeight(in, &f, params, true, false)
		if e := flight.Step(&f, params, flight.Input{DF: 6, E0: tick%2 == 0, Phase: uint16(tick), ControlFlags: 2}, queries); e != nil {
			t.Fatal(e)
		}
		if tick%10 == 0 {
			t.Log(tick, f.Pose.Height, f.Vertical, f.Collective, s.HeightTarget)
		}
	}
	low, high := f.Pose.Height, f.Pose.Height
	for tick := 150; tick < 350; tick++ {
		in.Axes[2] = 0
		f.Collective = s.Apply(in, &axes, f.Collective, false)
		s.ApplyHeight(in, &f, params, true, false)
		if e := flight.Step(&f, params, flight.Input{DF: 6, E0: tick%2 == 0, Phase: uint16(tick), ControlFlags: 2}, queries); e != nil {
			t.Fatal(e)
		}
		if tick >= 250 {
			low = min(low, f.Pose.Height)
			high = max(high, f.Pose.Height)
		}
	}
	t.Log("settled height range", low, high, "hold target", s.HeightTarget, "collective", f.Collective)
	if high-low > 30 || abs32(int32(f.RadarAltitude-s.HeightTarget)) > 20 {
		t.Fatal("height hold oscillation/tolerance", low, high, s.HeightTarget, f)
	}
}
