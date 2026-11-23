package flightcontrol

import (
	"encoding/binary"
	"gunship2000-assets/game/flight"
	"gunship2000-assets/game/world"
	"gunship2000-assets/internal/testfixture"
	"os"
	"reflect"
	"testing"
)

func heightFixture(t *testing.T) (flight.Parameters, flight.Queries) {
	t.Helper()
	var p flight.Parameters
	raw, err := os.ReadFile(testfixture.Path("installation/APA_STAT.DAT"))
	if err != nil {
		t.Fatal(err)
	}
	copy(p.Raw[:], raw)
	exe, err := os.ReadFile(testfixture.Path("installation/GS2.GS2"))
	if err != nil {
		t.Fatal(err)
	}
	w := &world.Bundle{Sine: make([]int16, 1025)}
	for i := range w.Sine {
		w.Sine[i] = int16(binary.LittleEndian.Uint16(exe[0x4bc50+2*i:]))
	}
	return p, flight.Queries{Angles: w, Height: func(flight.Pose) (int16, error) { return 0, nil }, Cell: func(flight.Pose) (byte, error) { return 0, nil }}
}
func TestHeightAssistUsesActualDamagedSourceCurve(t *testing.T) {
	p, q := heightFixture(t)
	for _, slider := range []bool{false, true} {
		var results []flight.State
		for _, newRules := range []bool{false, true} {
			f := flight.State{Pose: flight.Pose{H0: 200000, H1: 200000, Height: 500}, RadarAltitude: 498, Rotor: 1, Collective: 24, Thrust: 1486}
			s := LiveState{}
			a := Axes{}
			damage := [10]byte{}
			damage[2] = 1
			in := LiveInput{Mode: "relative_height", Bound: [4]bool{false, false, true}, Epoch: 1, Slider: slider, Lever: 32768}
			for n := 0; n < 150; n++ {
				if newRules {
					f.Collective = s.ApplyQ32Damage(in, &a, &f, false, true, damage, 0)
					s.ApplyHeightQ32Damage(in, &f, p, true, false, damage, 0)
				} else {
					f.Collective = s.ApplyQ32(in, &a, f.Collective, false, true)
					s.ApplyHeightQ32(in, &f, p, true, false)
				}
				if err := flight.Step(&f, p, flight.Input{DF: 6, E0: n%2 == 0, Phase: uint16(n), Damage: damage}, q); err != nil {
					t.Fatal(err)
				}
			}
			results = append(results, f)
		}
		t.Logf("slider%t oldAGL%d/VSI%d/collective%d newAGL%d/VSI%d/collective%d", slider, results[0].RadarAltitude, results[0].Vertical, results[0].Collective, results[1].RadarAltitude, results[1].Vertical, results[1].Collective)
		if results[0].RadarAltitude > 490 || results[0].Vertical != 0 {
			t.Fatal("old prediction defect not reproduced", results[0])
		}
		if results[1].Vertical != 0 || abs32(int32(results[1].RadarAltitude-498)) > 2 {
			t.Fatal("sustainable partial engine should arrest/hold using source available power", results[1])
		}
	}
}

func TestHeightHealthyResponseIsUnchanged(t *testing.T) {
	p, q := heightFixture(t)
	for _, slider := range []bool{false, true} {
		old := flight.State{Pose: flight.Pose{H0: 200000, H1: 200000, Height: 500}, RadarAltitude: 498, Rotor: 1, Collective: 24, Thrust: 1486}
		now := old
		so, sn := LiveState{}, LiveState{}
		ao, an := Axes{}, Axes{}
		in := LiveInput{Mode: "relative_height", Bound: [4]bool{false, false, true}, Epoch: 1, Slider: slider, Lever: 32768}
		for n := 0; n < 180; n++ {
			axis := int16(0)
			if n >= 20 && n < 50 {
				axis = 64
			}
			if n >= 100 && n < 120 {
				axis = -64
			}
			in.Axes[2] = axis
			if slider {
				in.Lever = 32768
				if axis > 0 {
					in.Lever = 50000
				}
				if axis < 0 {
					in.Lever = 15000
				}
			}
			old.Collective = so.ApplyQ32(in, &ao, old.Collective, false, true)
			so.ApplyHeightQ32(in, &old, p, true, false)
			now.Collective = sn.ApplyQ32Damage(in, &an, &now, false, true, [10]byte{}, 0)
			sn.ApplyHeightQ32Damage(in, &now, p, true, false, [10]byte{}, 0)
			f := flight.Input{DF: 6, E0: n%2 == 0, Phase: uint16(n)}
			if err := flight.Step(&old, p, f, q); err != nil {
				t.Fatal(err)
			}
			if err := flight.Step(&now, p, f, q); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(old, now) || !reflect.DeepEqual(so, sn) || ao != an {
				t.Fatal("healthy accepted response changed", slider, n, old, now, so, sn)
			}
		}
	}
}

func TestHeightEmergencyManualFallbackRearmAndKeyboard(t *testing.T) {
	for _, reason := range []string{"rotor", "609", "avionics"} {
		for _, slider := range []bool{false, true} {
			f := flight.State{Rotor: 1, Collective: 18}
			damage := [10]byte{}
			av := byte(0)
			if reason == "rotor" {
				f.Rotor = 0
			}
			if reason == "609" {
				damage[9] = 1
			}
			if reason == "avionics" {
				av = 0xf2
			}
			in := LiveInput{Mode: "relative_height", Bound: [4]bool{false, false, true}, Axes: [4]int16{0, 0, 127}, Epoch: 1, Slider: slider, Lever: 50000}
			s := LiveState{}
			a := Axes{}
			for n := 0; n < 4; n++ {
				f.Collective = s.ApplyQ32Damage(in, &a, &f, false, true, damage, av)
				if f.Collective != 18 {
					t.Fatal("entry/parked throttle jumped", reason, slider, n, f.Collective)
				}
			}
			if slider {
				in.Lever = 65535
				f.Collective = s.ApplyQ32Damage(in, &a, &f, false, true, damage, av)
				if f.Collective <= 18 {
					t.Fatal("slider movement lost", reason, f)
				}
			} else {
				in.Axes[2] = 0
				f.Collective = s.ApplyQ32Damage(in, &a, &f, false, true, damage, av)
				in.Axes[2] = 127
				f.Collective = s.ApplyQ32Damage(in, &a, &f, false, true, damage, av)
				if f.Collective != 19 {
					t.Fatal("fresh centered movement lost", reason, f)
				}
			}
			// A real keyboard change has priority; a parked slider cannot undo it.
			f.Collective = 22
			f.Collective = s.ApplyQ32Damage(in, &a, &f, true, true, damage, av)
			if f.Collective != 22 {
				t.Fatal("keyboard precedence", reason, slider, f)
			}
			if slider {
				f.Collective = s.ApplyQ32Damage(in, &a, &f, false, true, damage, av)
				if f.Collective != 22 {
					t.Fatal("parked slider undid keyboard", f)
				}
			}
			// Reconnect consumes a new baseline rather than resurrecting a held input.
			in.Epoch++
			f.Collective = s.ApplyQ32Damage(in, &a, &f, false, true, damage, av)
			if f.Collective != 22 {
				t.Fatal("reconnect jump", f)
			}
			saved := in.Mode
			if saved != "relative_height" {
				t.Fatal("persistent preference mutated")
			}
		}
	}
}

func TestPartialEngineClimbRequestUsesAvailablePower(t *testing.T) {
	p, q := heightFixture(t)
	var rates []int16
	for _, current := range []bool{false, true} {
		f := flight.State{Pose: flight.Pose{H0: 200000, H1: 200000, Height: 500}, RadarAltitude: 498, Rotor: 1, Collective: 24, Thrust: 1486}
		s := LiveState{}
		a := Axes{}
		d := [10]byte{}
		d[7] = 1
		in := LiveInput{Mode: "relative_height", Bound: [4]bool{false, false, true}, Axes: [4]int16{0, 0, 64}, Epoch: 1}
		for n := 0; n < 100; n++ {
			if current {
				f.Collective = s.ApplyQ32Damage(in, &a, &f, false, true, d, 0)
				s.ApplyHeightQ32Damage(in, &f, p, true, false, d, 0)
			} else {
				f.Collective = s.ApplyQ32(in, &a, f.Collective, false, true)
				s.ApplyHeightQ32(in, &f, p, true, false)
			}
			if err := flight.Step(&f, p, flight.Input{DF: 6, E0: n%2 == 0, Phase: uint16(n), Damage: d}, q); err != nil {
				t.Fatal(err)
			}
		}
		rates = append(rates, f.Vertical)
		t.Log("damage-aware", current, "rate", f.Vertical, "collective", f.Collective, "thrust", f.Thrust)
	}
	// Axis64 requests floor(64*8/127)=4. Healthymax1765 -> partialmax1655
	// still permits this source Easy rate; the old healthy prediction undershoots.
	if rates[0] >= rates[1] || abs32(int32(rates[1]-4)) > 1 {
		t.Fatal(rates)
	}
}

func TestPoweredHeightConflictingHoldNeedsDeliberateMovement(t *testing.T) {
	p, _ := heightFixture(t)
	power := flight.Input{PoweredEngineCount: 2}
	power.Damage[2] = 2
	if power.EngineThrust(1765, 1765) != 1765 {
		t.Fatal("healthy second engine lost maximum thrust")
	}
	s := LiveState{}
	f := flight.State{Rotor: 1, Hover: 1, HeightControl: 1, Collective: 27}
	in := LiveInput{Mode: "relative_height"}
	in.Bound[2] = true
	in.Axes[2] = 18
	for _, axis := range []int16{18, 18, 19} {
		in.Axes[2] = axis
		s.ApplyHeightWithPower(in, &f, p, true, false, power, 0)
	}
	if f.Hover == 0 || f.HeightControl == 0 {
		t.Fatal("parked axis cancelled hold")
	}
	in.Axes[2] = 48
	s.ApplyHeightWithPower(in, &f, p, true, false, power, 0)
	if f.Hover != 0 || f.HeightControl != 0 {
		t.Fatal("moving axis lost to hold")
	}
}
